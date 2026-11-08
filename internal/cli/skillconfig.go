package cli

import (
	"errors"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func (a *application) configCommand() *cobra.Command {
	root := &cobra.Command{Use: "config", Short: "Manage local skill requirements and system-store secrets", Long: "Configure installed skills locally. Values are never uploaded, printed by diagnostics, or automatically injected into agents."}
	root.AddCommand(&cobra.Command{Use: "list", Short: "List installations and retained local configurations", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		items, err := a.core.ConfigurationTargets(cmd.Context())
		if err != nil {
			return err
		}
		return a.output(cmd, items)
	}})
	doctor := &cobra.Command{Use: "doctor TARGET", Short: "Check requirements without displaying values", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		report, err := a.core.SkillConfiguration(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if err = a.output(cmd, report); err != nil {
			return err
		}
		if !report.Ready {
			return errors.New("configuration needs attention; values were not displayed")
		}
		return nil
	}}
	root.AddCommand(doctor)
	set := &cobra.Command{Use: "set TARGET KEY", Short: "Save a value or reference the current process environment", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		mode, _ := cmd.Flags().GetString("mode")
		stdin, _ := cmd.Flags().GetBool("value-stdin")
		var value string
		if mode == "environment" {
			if stdin {
				return errors.New("environment references do not accept a value")
			}
		} else {
			if !stdin {
				return errors.New("use --value-stdin; values must not appear in command arguments")
			}
			raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 4099))
			if err != nil {
				return errors.New("could not read configuration input")
			}
			value = strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
			if len(raw) > 4098 || len(value) > 4096 {
				return errors.New("configuration input exceeds 4096 bytes")
			}
		}
		if err := a.core.SetSkillConfiguration(cmd.Context(), args[0], args[1], mode, value); err != nil {
			return err
		}
		return a.output(cmd, map[string]bool{"saved_locally": true})
	}}
	set.Flags().String("mode", "keychain", "keychain, environment, or value (non-secret only)")
	set.Flags().Bool("value-stdin", false, "read the value from stdin; never use a literal in shell history")
	root.AddCommand(set)
	clear := &cobra.Command{Use: "clear TARGET", Short: "Delete saved values and system-store secrets for this installation", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			return errors.New("repeat with --yes to clear this installation's saved configuration")
		}
		if err := a.core.ClearSkillConfiguration(cmd.Context(), args[0]); err != nil {
			return err
		}
		return a.output(cmd, map[string]bool{"cleared": true})
	}}
	clear.Flags().Bool("yes", false, "confirm deleting saved configuration and secrets")
	root.AddCommand(clear)
	unset := &cobra.Command{Use: "unset TARGET KEY", Short: "Delete one saved value or environment reference", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			return errors.New("repeat with --yes to remove this saved value")
		}
		if err := a.core.UnsetSkillConfiguration(cmd.Context(), args[0], args[1]); err != nil {
			return err
		}
		return a.output(cmd, map[string]bool{"removed": true})
	}}
	unset.Flags().Bool("yes", false, "confirm removing this saved value")
	root.AddCommand(unset)
	return root
}
