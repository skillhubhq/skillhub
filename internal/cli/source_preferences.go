package cli

import (
	"errors"
	"github.com/spf13/cobra"
)

func (a *application) addSourcePreferences(parent *cobra.Command) {
	var name, note string
	edit := &cobra.Command{Use: "edit SOURCE", Short: "Edit this device’s connection name and note without changing its identity", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("note") {
			return errors.New("specify --name or --note; use an empty value to clear")
		}
		current, err := a.core.Source(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		label, description := current.Label, current.Note
		if cmd.Flags().Changed("name") {
			label = name
		}
		if cmd.Flags().Changed("note") {
			description = note
		}
		result, err := a.core.UpdateSourcePreferences(cmd.Context(), args[0], label, description)
		if err != nil {
			return err
		}
		return a.output(cmd, result)
	}}
	edit.Flags().StringVar(&name, "name", "", "local display name (empty uses Hub name)")
	edit.Flags().StringVar(&note, "note", "", "local note, never sent to the Hub")
	parent.AddCommand(edit, &cobra.Command{Use: "refresh SOURCE", Short: "Refresh the Hub name and access mode after verifying its identity", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		source, err := a.core.RefreshSource(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return a.output(cmd, source)
	}})
}
