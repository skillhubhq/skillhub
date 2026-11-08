package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"

	"connectrpc.com/connect"
	"github.com/mattn/go-isatty"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type appliedPlan struct {
	Applied bool                `json:"applied"`
	Plan    *client.InstallPlan `json:"plan"`
}

// 仅终端使用颜色；机器输出和重定向始终保持无装饰。
func terminalColor(w io.Writer, disabled bool) bool {
	file, ok := w.(interface{ Fd() uintptr })
	return !disabled && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" && ok && isatty.IsTerminal(file.Fd())
}

// Hub 文案、路径和错误不允许携带终端控制序列或伪造新行。
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, s)
}

type presentation struct {
	bytes.Buffer
	color bool
}

func (p *presentation) heading(s string) {
	s = clean(s)
	if p.color {
		s = "\x1b[1;34m" + s + "\x1b[0m"
	}
	fmt.Fprintln(&p.Buffer, s)
}
func (p *presentation) field(key, value string) {
	if value != "" {
		fmt.Fprintf(&p.Buffer, "  %-12s %s\n", key, clean(value))
	}
}
func (p *presentation) hint(s string) { fmt.Fprintf(&p.Buffer, "\n%s\n", clean(s)) }
func (p *presentation) gap()          { p.WriteByte('\n') }

func encodeOutput(value any) ([]byte, error) {
	if message, ok := value.(proto.Message); ok {
		return (protojson.MarshalOptions{Indent: "  ", UseProtoNames: true}).Marshal(message)
	}
	return json.MarshalIndent(value, "", "  ")
}

func (a *application) output(cmd *cobra.Command, value any) error {
	if a.json {
		data, err := encodeOutput(value)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}
	p := &presentation{color: terminalColor(cmd.OutOrStdout(), a.noColor)}
	switch v := value.(type) {
	case client.SkillConfiguration:
		p.heading("Local configuration · " + v.Installation.Name)
		p.field("Target", v.Installation.Target)
		if !v.Installed {
			p.hint("Not installed. Saved configuration can be cleared with config clear TARGET --yes.")
		}
		if !v.Declared {
			p.hint("No optional skillhub.config.json requirements declared.")
		}
		if v.Changed {
			p.hint("Skill version or origin changed. Clear saved configuration and review the current requirements.")
		}
		for _, item := range v.Variables {
			p.field(item.Name, item.Status+" · "+item.Mode)
		}
		p.hint("Local checks only. No values displayed, uploaded or injected into an agent.")
	case []client.ConfigTarget:
		p.heading("Local configurations")
		for _, item := range v {
			p.gap()
			p.installation(item.Installation)
			p.field("Installed", fmt.Sprint(item.Installed))
			p.field("Saved configuration", fmt.Sprint(item.Saved))
		}
	case client.EvaluationPage:
		p.heading(fmt.Sprintf("Evaluation evidence · %d", len(v.Evidence)))
		if !v.Supported {
			p.hint("This Hub does not support evaluation evidence.")
		} else if len(v.Evidence) == 0 {
			p.hint("No external results for this version.")
		}
		for _, e := range v.Evidence {
			p.gap()
			p.evidence(e)
		}
	case client.EvaluationEvidence:
		p.heading("Evaluation evidence")
		p.evidence(v)
		if !v.Retracted {
			p.hint("Use --json to read the complete case inputs and observed results.")
		}
	case authoringResult:
		p.authoring(v)
	case []client.Source:
		p.heading(fmt.Sprintf("Connections · %d", len(v)))
		if len(v) == 0 {
			p.hint("Connect a Hub: skillhub source add work https://skills.example.com")
		}
		for _, s := range v {
			p.gap()
			p.source(s)
		}
	case client.Source:
		p.heading("Hub connected")
		p.gap()
		p.source(v)
		p.hint("Sign in: skillhub login " + v.Alias)
	case []client.Installation:
		p.heading(fmt.Sprintf("Installed skills · %d", len(v)))
		if len(v) == 0 {
			p.hint("No skills installed. Find one with: skillhub search <query>")
		}
		for _, item := range v {
			p.gap()
			p.installation(item)
		}
	case []client.SearchResult:
		count, failures := 0, 0
		for _, result := range v {
			count += len(result.Skills)
			if result.Error != "" {
				failures++
			}
		}
		p.heading("Search · " + quantity(count, "skill") + " · " + quantity(len(v), "Hub"))
		for _, result := range v {
			for _, skill := range result.Skills {
				p.gap()
				p.heading(result.Source + "/" + skill.Namespace + "/" + skill.Name + "@" + skill.LatestVersion)
				p.field("Name", displayName(skill.DisplayNameEn, skill.DisplayNameZh))
				p.field("", skill.Description)
			}
			if result.Error != "" {
				p.gap()
				p.field("Unavailable", result.Source+": "+result.Error)
			}
			if result.NextPage != "" {
				p.field("Next page", result.Source+": --page-token "+result.NextPage)
			}
		}
		if count == 0 && failures == 0 {
			if len(v) == 0 {
				p.hint("Connect a Hub: skillhub source add work https://skills.example.com")
			} else {
				p.hint("No matching skills. Try a broader query or another Hub.")
			}
		}
		if count > 0 {
			p.hint("Inspect: skillhub info SOURCE/NAMESPACE/NAME")
		}
	case *client.InstallPlan:
		p.plan(*v, false)
	case appliedPlan:
		p.plan(*v.Plan, v.Applied)
	case []client.UpdateCheck:
		p.heading(fmt.Sprintf("Update checks · %d", len(v)))
		if len(v) == 0 {
			p.hint("No installed skills to check.")
		}
		for _, check := range v {
			p.gap()
			p.installation(check.Installation)
			switch {
			case check.Error != "":
				p.field("Check failed", check.Error)
			case check.VersionUnavailable:
				p.field("Status", "Installed version is no longer available")
			case check.AvailableVersion != "":
				p.field("Available", check.AvailableVersion)
			default:
				p.field("Status", "Up to date")
			}
			if check.ConnectionRequired {
				p.field("Next step", "Choose a connection to the same Hub with --source")
			}
		}
	case client.CacheInfo:
		p.heading("Download cache")
		p.field("Packages", fmt.Sprintf("%d / %d", v.Entries, v.LimitEntries))
		p.field("Storage", fmt.Sprintf("%.1f / %.0f MiB", float64(v.Bytes)/(1<<20), float64(v.LimitBytes)/(1<<20)))
		p.field("Directory", v.Directory)
		p.hint("Shared with the App. Clearing the cache keeps installed skills and accounts.")
	case []string:
		p.heading(fmt.Sprintf("Projects · %d", len(v)))
		for _, root := range v {
			p.field("Project", root)
		}
		if len(v) == 0 {
			p.hint("Track this project: skillhub project init")
		}
	case *client.ProjectState:
		p.project(*v)
	case client.ProjectState:
		p.project(v)
	case *client.ProjectRestorePlan:
		p.heading("Project restore preview")
		p.field("Project", v.Project)
		for _, plan := range v.Plans {
			p.gap()
			p.plan(plan, false)
		}
		if len(v.Plans) == 0 {
			p.hint("No locked skills to restore.")
		}
	case []client.ProjectRestoreResult:
		p.heading("Project restore results")
		for _, result := range v {
			p.gap()
			p.field("Target", result.Target)
			if result.Applied {
				p.field("Status", "Applied")
			} else {
				p.field("Failed", result.Error)
			}
		}
	default:
		data, err := encodeOutput(value)
		if err != nil {
			return err
		}
		var tree any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err = decoder.Decode(&tree); err != nil {
			return err
		}
		p.heading(label(strings.TrimPrefix(cmd.CommandPath(), "skillhub ")))
		p.tree(tree, "  ")
	}
	_, err := io.Copy(cmd.OutOrStdout(), &p.Buffer)
	return err
}

func (p *presentation) source(s client.Source) {
	p.heading(s.Alias)
	p.field("Community", s.DisplayName)
	p.field("URL", s.URL)
	status := "No saved login"
	if s.HasCredential {
		status = "Credentials saved"
	} else if !s.Private {
		status = "Public access · no saved login"
	}
	p.field("Account", status)
}
func (p *presentation) installation(v client.Installation) {
	p.heading(v.Namespace + "/" + v.Name + "@" + v.Version)
	p.field("Name", displayName(v.DisplayNameEN, v.DisplayNameZH))
	p.field("Source", v.Source)
	p.field("Install", v.Tool+" · "+v.Scope)
	p.field("Target", v.Target)
}
func (p *presentation) plan(v client.InstallPlan, applied bool) {
	title := "Change preview · " + v.Action
	if applied {
		title = "Change applied · " + v.Action
	}
	p.heading(title)
	item := v.After
	if item == nil {
		item = v.Before
	}
	if item != nil {
		p.field("Skill", item.Namespace+"/"+item.Name)
		p.field("Name", displayName(item.DisplayNameEN, item.DisplayNameZH))
		p.field("Source", item.Source)
		p.field("Install", item.Tool+" · "+item.Scope)
		version := item.Version
		if v.Before != nil && v.After != nil {
			version = v.Before.Version + " → " + v.After.Version
		}
		p.field("Version", version)
	}
	p.field("Target", v.Target)
	if !applied {
		p.hint("No files changed. Repeat with --yes to apply this plan.")
	}
}
func (p *presentation) project(v client.ProjectState) {
	p.heading("Project skills")
	p.field("Directory", v.Root)
	state := "Ready to restore"
	if v.NeedsLock {
		state = "Needs lock · run skillhub project lock"
	}
	p.field("Lock", state)
	for _, s := range v.Manifest.Skills {
		p.gap()
		p.heading(s.Source + "/" + s.Namespace + "/" + s.Name + "@" + s.Version)
		p.field("Tool", s.Tool)
	}
	if len(v.Manifest.Skills) == 0 {
		p.hint("Add a skill: skillhub project add SOURCE/NAMESPACE/NAME@VERSION --tool codex")
	}
}

func label(key string) string {
	words := strings.Fields(strings.ReplaceAll(key, "_", " "))
	for i, word := range words {
		switch word {
		case "id", "url", "uri":
			words[i] = strings.ToUpper(word)
		default:
			runes := []rune(word)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}

func displayName(en, zh string) string {
	if en != "" {
		return en
	}
	return zh
}

func quantity(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", n, noun)
}

// 不认识的管理响应也以可读字段展示；原始字段、枚举和精度由 --json 保留。
func (p *presentation) tree(value any, indent string) {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			fmt.Fprintln(&p.Buffer, indent+"Done.")
		}
		for _, k := range keys {
			if k == "credential_ref" || k == "source_ref" {
				continue
			}
			switch v[k].(type) {
			case map[string]any, []any:
				fmt.Fprintf(&p.Buffer, "%s%s\n", indent, clean(label(k)))
				p.tree(v[k], indent+"  ")
			default:
				fmt.Fprintf(&p.Buffer, "%s%-18s %s\n", indent, clean(label(k)), scalar(v[k]))
			}
		}
	case []any:
		if len(v) == 0 {
			fmt.Fprintln(&p.Buffer, indent+"None.")
		}
		for i, item := range v {
			if i > 0 {
				p.gap()
			}
			p.tree(item, indent)
		}
	default:
		fmt.Fprintln(&p.Buffer, indent+scalar(v))
	}
}
func scalar(v any) string {
	if v == nil {
		return "—"
	}
	if b, ok := v.(bool); ok {
		if b {
			return "Yes"
		}
		return "No"
	}
	s := fmt.Sprint(v)
	for _, prefix := range []string{"RELEASE_STATUS_", "VISIBILITY_", "NAMESPACE_ROLE_", "PLATFORM_ROLE_", "SUBJECT_KIND_", "SKILL_ICON_"} {
		if strings.HasPrefix(s, prefix) {
			return label(strings.ToLower(strings.TrimPrefix(s, prefix)))
		}
	}
	return clean(s)
}

// 错误只写 stderr，保留返回码与 stdout 的 JSON 契约。
func PrintError(w io.Writer, err error) {
	if errors.Is(err, ErrPlanOnly) {
		fmt.Fprintln(w, "Next: review the plan, then repeat with --yes. Use --dry-run for preview-only automation.")
		return
	}
	fmt.Fprintln(w, "Error: "+clean(err.Error()))
	hint := ""
	switch {
	case errors.Is(err, client.ErrModified):
		hint = "Save or back up your local edits before updating or removing this skill."
	case errors.Is(err, client.ErrSourceNotFound):
		hint = "Run skillhub source list to see your connections."
	case errors.Is(err, client.ErrCredentialMissing), connect.CodeOf(err) == connect.CodeUnauthenticated:
		hint = "Sign in again with skillhub login SOURCE."
	case errors.Is(err, client.ErrUnmanaged):
		hint = "Choose another destination or move the existing directory yourself."
	case errors.Is(err, client.ErrStalePlan):
		hint = "Run the command again to inspect a fresh plan."
	}
	if hint != "" {
		fmt.Fprintln(w, "\n"+hint)
	}
}

func (p *presentation) evidence(e client.EvaluationEvidence) {
	p.field("ID", e.ID)
	if e.Retracted {
		p.field("Status", "Retracted; report content removed")
		return
	}
	p.field("Suite", e.SuiteName+" · "+e.SuiteVersion)
	p.field("Agent", e.Agent)
	p.field("Model", e.Model)
	p.field("Runner", e.Runner)
	p.field("Results", fmt.Sprintf("%d/%d passed · %d failed · %d errors · %d skipped", e.Passed, e.Passed+e.Failed+e.Errors+e.Skipped, e.Failed, e.Errors, e.Skipped))
	p.field("Executed", e.ExecutedAt)
	p.field("Imported", e.ImportedAt)
	p.field("Trust", "Externally reported; not independently verified by SkillHub")
}
