package cli

import (
	"errors"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
)

func (a *application) namespaceLifecycleCommands(parent *cobra.Command) {
	source := func(cmd *cobra.Command) string { v, _ := cmd.Flags().GetString("source"); return v }
	parent.AddCommand(a.remoteCommand("namespace-inspect NAME", "Preview a team's content, grants and lifecycle state", cobra.ExactArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		v, e := a.core.NamespaceLifecycle(cmd.Context(), source(cmd), args[0])
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	}))
	for _, action := range []string{"archive", "restore", "purge"} {
		command := a.remoteCommand("namespace-"+action+" NAME", "Manage team lifecycle; inspect first and supply its revision", cobra.ExactArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
			revision, _ := cmd.Flags().GetString("revision")
			confirm, _ := cmd.Flags().GetString("confirm")
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				return errors.New("inspect the team first, then pass --revision and --yes; purge also requires --confirm NAME")
			}
			if action == "purge" && confirm != args[0] {
				return errors.New("permanent cleanup requires --confirm matching the namespace")
			}
			v, e := a.core.ChangeNamespaceLifecycle(cmd.Context(), source(cmd), args[0], action, revision, confirm, a.mutation().IdempotencyKey)
			if e != nil {
				return e
			}
			return a.output(cmd, v)
		})
		command.Flags().String("revision", "", "revision from namespace-inspect")
		_ = command.MarkFlagRequired("revision")
		command.Flags().Bool("yes", false, "confirm the lifecycle change; installed files and audit are preserved")
		command.Flags().String("confirm", "", "exact namespace name for permanent cleanup")
		parent.AddCommand(command)
	}
}
