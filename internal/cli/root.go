// Package cli 只负责命令交互；源连接、凭据和文件变更交给共享客户端内核。
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/provenance"
	"github.com/skillhubhq/skillhub/internal/provenancewire"
	"github.com/spf13/cobra"
)

var ErrPartial = errors.New("one or more sources failed; successful results are included")
var ErrPlanOnly = errors.New("review the plan and repeat with --yes to apply, or use --dry-run")

type Options struct{ Client client.ClientOptions }
type application struct {
	options   Options
	store     *client.Store
	core      *client.Client
	data      string
	json      bool
	noColor   bool
	requestID string
}

func Execute(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, options Options) error {
	a := &application{options: options}
	root := a.command()
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	err := root.ExecuteContext(ctx)
	if a.store != nil {
		err = errors.Join(err, a.store.Close())
	}
	return err
}

func (a *application) command() *cobra.Command {
	root := &cobra.Command{Use: "skillhub", Short: "Manage skills across independent registries and local tools", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&a.data, "data", "", "local state directory (default: SKILLHUB_HOME or OS user config)")
	root.PersistentFlags().BoolVar(&a.noColor, "no-color", false, "disable terminal colors")
	root.PersistentFlags().BoolVar(&a.json, "json", false, "emit machine-readable JSON")
	root.PersistentFlags().StringVar(&a.requestID, "request-id", "", "idempotency key for a retryable remote mutation")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Annotations["local-authoring"] == "true" {
			return nil
		}
		// 补全脚本只描述命令，不应初始化数据库或访问凭据。
		for c := cmd; c != nil; c = c.Parent() {
			if c.Name() == "completion" || c.Name() == "__complete" || c.Name() == "__completeNoDesc" {
				return nil
			}
		}
		var err error
		if a.data == "" {
			a.data, err = client.DataDir()
			if err != nil {
				return err
			}
		}
		a.store, err = client.OpenStore(a.data)
		if err != nil {
			return err
		}
		if err = a.store.Recover(cmd.Context()); err != nil {
			return err
		}
		a.core = client.NewClient(a.store, a.options.Client)
		return nil
	}
	root.AddCommand(a.evaluationCommand(), a.initCommand(), a.validateCommand(), a.passwordCommand(), a.sourceCommand(), a.loginCommand(), a.searchCommand(), a.publishCommand(), a.reviewCommand(), a.adminCommand(), a.governanceCommand(), a.projectCommand())
	root.AddCommand(&cobra.Command{Use: "logout SOURCE", Short: "Revoke the current session and remove its saved credential", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.core.Logout(cmd.Context(), args[0]); err != nil {
			return err
		}
		return a.output(cmd, map[string]bool{"logged_out": true})
	}})
	root.AddCommand(a.remoteCommand("whoami", "Show current source identity", cobra.NoArgs, func(cmd *cobra.Command, r *client.Remote, args []string) error {
		response, err := r.Auth.GetIdentity(cmd.Context(), connect.NewRequest(&authv1.GetIdentityRequest{}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	}))
	root.AddCommand(&cobra.Command{Use: "list", Short: "List local installations shared with the desktop App", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		items, err := a.store.ListInstallations(cmd.Context())
		if err != nil {
			return err
		}
		return a.output(cmd, items)
	}})
	root.AddCommand(a.configCommand(), a.environmentCommand(), a.installCommand(), a.removeCommand(), a.updatesCommand(), a.updateCommand(), a.cacheCommand())
	root.AddCommand(&cobra.Command{Use: "info SOURCE/NAMESPACE/NAME", Short: "Show a visible skill", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		coordinate, err := client.ParseCoordinate(args[0], false)
		if err != nil {
			return err
		}
		remote, err := a.core.Remote(cmd.Context(), coordinate.Source)
		if err != nil {
			return err
		}
		response, err := remote.Catalog.GetSkill(cmd.Context(), connect.NewRequest(&catalogv1.GetSkillRequest{Namespace: coordinate.Namespace, Name: coordinate.Name}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	}})
	versions := &cobra.Command{Use: "versions SOURCE/NAMESPACE/NAME", Short: "List downloadable versions", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		coordinate, err := client.ParseCoordinate(args[0], false)
		if err != nil {
			return err
		}
		token, _ := cmd.Flags().GetString("page-token")
		result, err := a.core.Releases(cmd.Context(), coordinate, token)
		if err != nil {
			return err
		}
		return a.output(cmd, result)
	}}
	versions.Flags().String("page-token", "", "cursor returned by this source")
	root.AddCommand(versions)
	yank := a.remoteCommand("yank RELEASE_ID", "Withdraw a published or pending version", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		reason, _ := cmd.Flags().GetString("reason")
		response, err := r.Publishing.YankRelease(cmd.Context(), connect.NewRequest(&publishingv1.YankReleaseRequest{Mutation: a.mutation(), ReleaseId: args[0], Reason: reason}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	})
	yank.Flags().String("reason", "", "withdrawal reason")
	_ = yank.MarkFlagRequired("reason")
	root.AddCommand(yank)
	a.configureHelp(root)
	return root
}

func (a *application) mutation() *commonv1.Mutation {
	key := a.requestID
	if key == "" {
		key = uuid.NewString()
	}
	return &commonv1.Mutation{IdempotencyKey: key}
}

func (a *application) remoteCommand(use, short string, args cobra.PositionalArgs, run func(*cobra.Command, *client.Remote, []string) error) *cobra.Command {
	command := &cobra.Command{Use: use, Short: short, Args: args, RunE: func(cmd *cobra.Command, args []string) error {
		source, _ := cmd.Flags().GetString("source")
		remote, err := a.core.Remote(cmd.Context(), source)
		if err != nil {
			return err
		}
		return run(cmd, remote, args)
	}}
	command.Flags().String("source", "", "source alias")
	_ = command.MarkFlagRequired("source")
	return command
}

func (a *application) sourceCommand() *cobra.Command {
	command := &cobra.Command{Use: "source", Short: "Manage independent Hub connections"}
	command.AddCommand(&cobra.Command{Use: "add ALIAS URL", Short: "Connect a Hub and remember its stable identity", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		source, err := a.core.AddSource(cmd.Context(), args[0], args[1])
		if err != nil {
			return err
		}
		return a.output(cmd, source)
	}})
	command.AddCommand(&cobra.Command{Use: "list", Short: "List configured sources", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		sources, err := a.core.ListSources(cmd.Context())
		if err != nil {
			return err
		}
		return a.output(cmd, sources)
	}})
	command.AddCommand(&cobra.Command{Use: "remove SOURCE", Short: "Remove the local connection and its saved credential", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.core.RemoveSource(cmd.Context(), args[0]); err != nil {
			return err
		}
		return a.output(cmd, map[string]bool{"removed": true})
	}})
	return command
}

func passwordInput(cmd *cobra.Command) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1025))
	if err != nil {
		return "", err
	}
	if len(raw) > 1024 {
		return "", errors.New("password input too large")
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r"), nil
}

func (a *application) loginCommand() *cobra.Command {
	command := &cobra.Command{Use: "login SOURCE", Short: "Authorize this client through the source's browser page", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		stdin, _ := cmd.Flags().GetBool("password-stdin")
		username, _ := cmd.Flags().GetString("username")
		if stdin {
			if username == "" {
				return errors.New("--username is required with --password-stdin")
			}
			password, err := passwordInput(cmd)
			if err != nil {
				return err
			}
			identity, err := a.core.LoginPassword(cmd.Context(), args[0], username, password)
			if err != nil {
				return err
			}
			return a.output(cmd, identity)
		}
		if username != "" {
			return errors.New("--username requires --password-stdin")
		}
		login, err := a.core.StartLogin(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Sign in to %s\n\n  Open  %s\n  Code  %s\n\nWaiting for approval. Press Ctrl+C to cancel.\n", clean(args[0]), clean(login.VerificationURI), clean(login.UserCode))
		for time.Now().Before(login.ExpiresAt) {
			timer := time.NewTimer(login.Interval)
			select {
			case <-cmd.Context().Done():
				timer.Stop()
				return cmd.Context().Err()
			case <-timer.C:
			}
			identity, pending, err := a.core.CompleteLogin(cmd.Context(), login)
			if connect.CodeOf(err) == connect.CodeResourceExhausted {
				continue
			}
			if err != nil {
				return err
			}
			if !pending {
				return a.output(cmd, identity)
			}
		}
		return errors.New("device authorization expired")
	}}
	command.Flags().Bool("password-stdin", false, "read a local account password from stdin instead of browser authorization")
	command.Flags().String("username", "", "source-local account name")
	return command
}

func (a *application) searchCommand() *cobra.Command {
	command := &cobra.Command{Use: "search [QUERY]", Short: "Search selected sources (default: all configured sources)", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) > 0 {
			query = args[0]
		}
		sources, _ := cmd.Flags().GetStringSlice("source")
		category, _ := cmd.Flags().GetString("category")
		tags, _ := cmd.Flags().GetStringSlice("tag")
		token, _ := cmd.Flags().GetString("page-token")
		size, _ := cmd.Flags().GetUint32("page-size")
		orderName, _ := cmd.Flags().GetString("order")
		order := taxonomy.CatalogOrder_CATALOG_ORDER_UNSPECIFIED
		switch orderName {
		case "name":
		case "recent":
			order = taxonomy.CatalogOrder_CATALOG_ORDER_RECENT
		default:
			return errors.New("order must be name or recent")
		}
		featured, _ := cmd.Flags().GetBool("featured")
		result, err := a.core.Search(cmd.Context(), sources, &catalogv1.SearchSkillsRequest{Query: query, Category: category, Tags: tags, Order: order, FeaturedOnly: featured, Page: &commonv1.PageRequest{PageSize: size, PageToken: token}})
		if err != nil {
			return err
		}
		if err = a.output(cmd, result); err != nil {
			return err
		}
		for _, source := range result {
			if source.Error != "" {
				return ErrPartial
			}
		}
		return nil
	}}
	command.Flags().String("order", "name", "name or recent publication")
	command.Flags().Bool("featured", false, "only administrator curated skills")
	command.Flags().StringSlice("source", nil, "source aliases to search")
	command.Flags().String("category", "", "category within each selected source")
	command.Flags().StringSlice("tag", nil, "required tags")
	command.Flags().String("page-token", "", "cursor; requires exactly one source")
	command.Flags().Uint32("page-size", 30, "results per source, maximum 100")
	return command
}

func (a *application) installCommand() *cobra.Command {
	command := &cobra.Command{Use: "install SOURCE/NAMESPACE/NAME@VERSION", Short: "Plan or install an exact version; also updates an existing installation", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		coordinate, err := client.ParseCoordinate(args[0], true)
		if err != nil {
			return err
		}
		tool, _ := cmd.Flags().GetString("tool")
		scope, _ := cmd.Flags().GetString("scope")
		project, _ := cmd.Flags().GetString("project")
		plan, archive, err := a.core.PlanRemoteInstall(cmd.Context(), coordinate, tool, scope, project)
		if err != nil {
			return err
		}
		return a.applyPlan(cmd, plan, archive)
	}}
	command.Flags().String("tool", "", "target tool: codex or claude-code")
	_ = command.MarkFlagRequired("tool")
	command.Flags().String("scope", "project", "installation scope: global or project")
	command.Flags().String("project", "", "project directory (default: working directory)")
	planFlags(command)
	return command
}

func planFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("yes", false, "apply the displayed file change plan")
	cmd.Flags().Bool("dry-run", false, "show the plan without applying it")
	cmd.MarkFlagsMutuallyExclusive("yes", "dry-run")
}
func (a *application) applyPlan(cmd *cobra.Command, plan *client.InstallPlan, data []byte) error {
	yes, _ := cmd.Flags().GetBool("yes")
	dry, _ := cmd.Flags().GetBool("dry-run")
	if !yes {
		if err := a.output(cmd, plan); err != nil {
			return err
		}
		if dry {
			return nil
		}
		return ErrPlanOnly
	}
	if err := a.store.Apply(cmd.Context(), *plan, data); err != nil {
		return err
	}
	return a.output(cmd, appliedPlan{true, plan})
}

func (a *application) removeCommand() *cobra.Command {
	command := &cobra.Command{Use: "remove TARGET", Short: "Plan or remove a managed installation without discarding local edits", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		plan, err := a.store.PlanRemove(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return a.applyPlan(cmd, plan, nil)
	}}
	planFlags(command)
	return command
}

func (a *application) publishCommand() *cobra.Command {
	command := a.remoteCommand("publish DIRECTORY", "Pack and submit a skill for publication or review", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		archive, bundle, err := packageformat.Pack(args[0])
		if err != nil {
			return err
		}
		var origin *provenance.Record
		originFile, _ := cmd.Flags().GetString("provenance")
		if originFile != "" {
			if err = r.RequireProvenance(); err != nil {
				return err
			}
			f, e := os.Open(originFile)
			if e != nil {
				return e
			}
			origin, e = provenance.Read(f)
			f.Close()
			if e != nil {
				return e
			}
			if e = provenance.Validate(origin, bundle); e != nil {
				return e
			}
		}
		namespace, _ := cmd.Flags().GetString("namespace")
		version, _ := cmd.Flags().GetString("version")
		visibility, _ := cmd.Flags().GetString("visibility")
		groups, _ := cmd.Flags().GetStringSlice("group")
		category, _ := cmd.Flags().GetString("category")
		tags, _ := cmd.Flags().GetStringSlice("tag")
		displayEN, _ := cmd.Flags().GetString("display-name-en")
		displayZH, _ := cmd.Flags().GetString("display-name-zh")
		iconName, _ := cmd.Flags().GetString("icon")
		if displayEN != "" || displayZH != "" || iconName != "unspecified" {
			if err = r.RequireSkillPresentation(); err != nil {
				return err
			}
		}
		icon, ok := taxonomy.SkillIcon_value["SKILL_ICON_"+strings.ToUpper(iconName)]
		if !ok {
			return errors.New("unknown built-in skill icon")
		}
		audience := map[string]taxonomy.Visibility{"public": taxonomy.Visibility_VISIBILITY_PUBLIC, "members": taxonomy.Visibility_VISIBILITY_MEMBERS, "groups": taxonomy.Visibility_VISIBILITY_GROUPS}[visibility]
		if audience == 0 {
			return errors.New("visibility must be public, members, or groups")
		}
		response, err := r.Publishing.SubmitRelease(cmd.Context(), connect.NewRequest(&publishingv1.SubmitReleaseRequest{Mutation: a.mutation(), Namespace: namespace, Version: version, Archive: archive, Visibility: audience, GroupIds: groups, Category: category, Tags: tags, DisplayNameEn: displayEN, DisplayNameZh: displayZH, Icon: taxonomy.SkillIcon(icon), Provenance: provenancewire.Encode(origin)}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	})
	command.Flags().String("namespace", "", "target namespace")
	_ = command.MarkFlagRequired("namespace")
	command.Flags().String("version", "", "immutable semantic version")
	_ = command.MarkFlagRequired("version")
	command.Flags().String("visibility", "members", "public, members, or groups")
	command.Flags().StringSlice("group", nil, "audience group IDs")
	command.Flags().String("category", "", "category name")
	command.Flags().String("provenance", "", "JSON upstream attribution file (requires independent review)")
	command.Flags().String("display-name-en", "", "English display name (up to 80 characters)")
	command.Flags().String("display-name-zh", "", "Simplified Chinese display name (up to 80 characters)")
	command.Flags().String("icon", "unspecified", "unspecified, document, code, chart, writing, research, design, workflow, terminal")
	command.Flags().StringSlice("tag", nil, "classification tags")
	return command
}

func (a *application) reviewCommand() *cobra.Command {
	command := &cobra.Command{Use: "review", Short: "Inspect and independently review immutable submissions"}
	list := a.remoteCommand("list", "List submissions you can maintain or review", cobra.NoArgs, func(cmd *cobra.Command, r *client.Remote, args []string) error {
		namespace, _ := cmd.Flags().GetString("namespace")
		token, _ := cmd.Flags().GetString("page-token")
		response, err := r.Publishing.ListSubmissions(cmd.Context(), connect.NewRequest(&publishingv1.ListSubmissionsRequest{Namespace: namespace, Page: &commonv1.PageRequest{PageSize: 100, PageToken: token}}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	})
	list.Flags().String("namespace", "", "optional namespace")
	list.Flags().String("page-token", "", "cursor")
	command.AddCommand(list)
	command.AddCommand(a.remoteCommand("inspect RELEASE_ID", "Preview the exact submitted files without executing them", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		response, err := r.Publishing.GetSubmission(cmd.Context(), connect.NewRequest(&publishingv1.GetSubmissionRequest{ReleaseId: args[0]}))
		if err != nil {
			return err
		}
		bundle, err := packageformat.Inspect(response.Msg.Archive)
		if err != nil {
			return err
		}
		if response.Msg.Release == nil || bundle.Digest != response.Msg.Release.Digest {
			return errors.New("submission digest mismatch")
		}
		type file struct {
			Path       string `json:"path"`
			Bytes      int    `json:"bytes"`
			Executable bool   `json:"executable"`
		}
		files := []file{}
		for _, entry := range bundle.Files {
			files = append(files, file{entry.Path, len(entry.Data), entry.Executable})
		}
		return a.output(cmd, struct {
			Release *catalogv1.Release `json:"release"`
			Files   []file             `json:"files"`
		}{response.Msg.Release, files})
	}))
	for _, action := range []string{"approve", "reject"} {
		approve := action == "approve"
		child := a.remoteCommand(action+" RELEASE_ID", action+" a submission", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
			reason, _ := cmd.Flags().GetString("reason")
			response, err := r.Publishing.ReviewRelease(cmd.Context(), connect.NewRequest(&publishingv1.ReviewReleaseRequest{Mutation: a.mutation(), ReleaseId: args[0], Approve: approve, Reason: reason}))
			if err != nil {
				return err
			}
			return a.output(cmd, response.Msg)
		})
		child.Flags().String("reason", "", "review reason")
		if !approve {
			_ = child.MarkFlagRequired("reason")
		}
		command.AddCommand(child)
	}
	return command
}
