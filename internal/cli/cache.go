package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func (a *application) cacheCommand() *cobra.Command {
	command := &cobra.Command{Use: "cache", Short: "Inspect or clear the shared local package cache"}
	command.AddCommand(&cobra.Command{Use: "info", Short: "Show cached package count, size and capacity", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		info, err := a.store.CacheInfo(cmd.Context())
		if err != nil {
			return err
		}
		return a.output(cmd, info)
	}})
	clear := &cobra.Command{Use: "clear", Short: "Remove cached downloads while preserving installations and credentials", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			info, err := a.store.CacheInfo(cmd.Context())
			if err != nil {
				return err
			}
			if err = a.output(cmd, info); err != nil {
				return err
			}
			return errors.New("repeat cache clear with --yes to remove cached downloads")
		}
		info, err := a.store.ClearCache(cmd.Context())
		if err != nil {
			return err
		}
		return a.output(cmd, info)
	}}
	clear.Flags().Bool("yes", false, "clear cached package downloads")
	command.AddCommand(clear)
	return command
}
