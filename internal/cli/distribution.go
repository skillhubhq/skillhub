package cli

import (
	"errors"
	"os"

	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
)

func (a *application) compareCommand() *cobra.Command {
	return &cobra.Command{Use: "compare SOURCE/NAMESPACE/NAME BEFORE_VERSION AFTER_VERSION", Short: "Compare files in two exact skill versions", Args: cobra.ExactArgs(3), RunE: func(cmd *cobra.Command, args []string) error {
		c, e := client.ParseCoordinate(args[0], false)
		if e != nil {
			return e
		}
		ids := map[string]string{}
		token := ""
		for {
			p, e := a.core.Releases(cmd.Context(), c, token)
			if e != nil {
				return e
			}
			for _, r := range p.Releases {
				ids[r.Version] = r.ReleaseId
			}
			token = p.GetPage().GetNextPageToken()
			if token == "" {
				break
			}
		}
		if ids[args[1]] == "" || ids[args[2]] == "" {
			return errors.New("both versions must be readable published versions")
		}
		v, e := a.core.CompareReleases(cmd.Context(), c, ids[args[1]], ids[args[2]])
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	}}
}
func (a *application) subscriptionsCommand() *cobra.Command {
	root := &cobra.Command{Use: "subscriptions", Short: "Follow skill publications in a Hub"}
	list := a.remoteCommand("list", "List followed skills", cobra.NoArgs, func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		source, _ := cmd.Flags().GetString("source")
		token, _ := cmd.Flags().GetString("page-token")
		v, e := a.core.Subscriptions(cmd.Context(), source, token)
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	})
	list.Flags().String("page-token", "", "pagination cursor")
	root.AddCommand(list)
	for _, action := range []string{"follow", "unfollow"} {
		root.AddCommand(&cobra.Command{Use: action + " SOURCE/NAMESPACE/NAME", Short: action + " skill updates", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			c, e := client.ParseCoordinate(args[0], false)
			if e != nil {
				return e
			}
			return a.core.Subscribe(cmd.Context(), c, action == "follow", a.mutation().IdempotencyKey)
		}})
	}
	return root
}
func (a *application) tokensCommand() *cobra.Command {
	root := &cobra.Command{Use: "tokens", Short: "Manage scoped automation credentials"}
	root.AddCommand(a.remoteCommand("list", "List token metadata", cobra.NoArgs, func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		source, _ := cmd.Flags().GetString("source")
		v, e := a.core.AccessTokens(cmd.Context(), source)
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	}))
	root.AddCommand(a.remoteCommand("revoke ID", "Revoke a token immediately", cobra.ExactArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		source, _ := cmd.Flags().GetString("source")
		return a.core.RevokeAccessToken(cmd.Context(), source, args[0])
	}))
	create := a.remoteCommand("create NAME", "Create a token and write its secret to a new private file", cobra.ExactArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		source, _ := cmd.Flags().GetString("source")
		scope, _ := cmd.Flags().GetString("scope")
		namespace, _ := cmd.Flags().GetString("namespace")
		days, _ := cmd.Flags().GetUint32("days")
		path, _ := cmd.Flags().GetString("output")
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		success := false
		defer func() {
			f.Close()
			if !success {
				os.Remove(path)
			}
		}()
		v, e := a.core.CreateAccessToken(cmd.Context(), source, args[0], scope, namespace, days)
		if e != nil {
			return e
		}
		_, e = f.WriteString(v.Secret + "\n")
		if e == nil {
			e = f.Sync()
		}
		if e == nil {
			e = f.Close()
		}
		if e != nil {
			_ = a.core.RevokeAccessToken(cmd.Context(), source, v.Token.ID)
			return e
		}
		success = true
		return a.output(cmd, v.Token)
	})
	create.Flags().String("scope", "read", "read or publish")
	create.Flags().String("namespace", "", "namespace required for publishing")
	create.Flags().Uint32("days", 30, "expiry in 1–90 days")
	create.Flags().String("output", "", "new private file for the one-time secret")
	_ = create.MarkFlagRequired("output")
	root.AddCommand(create)
	return root
}
