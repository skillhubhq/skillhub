package cli

import (
	"errors"

	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
)

func (a *application) updatesCommand() *cobra.Command {
	command := &cobra.Command{Use: "updates [TARGET]", Short: "Check managed installations for newer stable versions, reporting each source failure", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		source, _ := cmd.Flags().GetString("source")
		results, err := a.core.CheckUpdatesFrom(cmd.Context(), target, source)
		if err != nil {
			return err
		}
		if err = a.output(cmd, results); err != nil {
			return err
		}
		for _, result := range results {
			if result.Error != "" || result.VersionUnavailable {
				return errors.New("one or more update checks failed; successful results are included")
			}
		}
		return nil
	}}
	command.Flags().String("source", "", "connection alias for update authorization (successful checks remember it)")
	return command
}
func (a *application) updateCommand() *cobra.Command {
	command := &cobra.Command{Use: "update TARGET", Short: "Plan or apply a newer stable version, or an explicitly selected version", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		version, _ := cmd.Flags().GetString("version")
		source, _ := cmd.Flags().GetString("source")
		plan, archive, err := a.core.PlanRemoteUpdateFrom(cmd.Context(), args[0], version, source)
		if errors.Is(err, client.ErrNoUpdate) {
			return a.output(cmd, map[string]bool{"up_to_date": true})
		}
		if err != nil {
			return err
		}
		return a.applyPlan(cmd, plan, archive)
	}}
	command.Flags().String("version", "", "exact version (default: newest higher stable version); does not edit project manifests")
	command.Flags().String("source", "", "explicit connection alias for update authorization")
	planFlags(command)
	return command
}
