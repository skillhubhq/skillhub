package cli

import (
	"errors"

	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
)

func (a *application) projectCommand() *cobra.Command {
	command := &cobra.Command{Use: "project", Short: "Manage a reproducible project manifest and exact-version lock"}
	var directory string
	command.PersistentFlags().StringVar(&directory, "project", "", "project directory (default: working directory)")
	command.AddCommand(&cobra.Command{Use: "list", Short: "List project paths shared with the desktop App", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		roots, err := a.core.ListProjects(cmd.Context())
		if err != nil {
			return err
		}
		return a.output(cmd, roots)
	}})
	command.AddCommand(&cobra.Command{Use: "forget", Short: "Remove the local project shortcut; keep its files and installations", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.core.ForgetProject(cmd.Context(), directory); err != nil {
			return err
		}
		return a.output(cmd, map[string]bool{"forgotten": true})
	}})
	command.AddCommand(&cobra.Command{Use: "init", Short: "Create an empty manifest without overwriting an existing file", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		state, err := a.core.InitProject(cmd.Context(), directory)
		if err != nil {
			return err
		}
		return a.output(cmd, state)
	}})
	command.AddCommand(&cobra.Command{Use: "show", Short: "Read the manifest, lock and whether resolution is needed", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		state, err := a.core.OpenProject(cmd.Context(), directory)
		if err != nil {
			return err
		}
		return a.output(cmd, state)
	}})
	add := &cobra.Command{Use: "add SOURCE/NAMESPACE/NAME@VERSION", Short: "Set a desired skill for a tool; does not install files", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		coordinate, err := client.ParseCoordinate(args[0], true)
		if err != nil {
			return err
		}
		tool, _ := cmd.Flags().GetString("tool")
		state, err := a.core.SetProjectSkill(cmd.Context(), directory, client.ProjectSkill{Source: coordinate.Source, Namespace: coordinate.Namespace, Name: coordinate.Name, Version: coordinate.Version, Tool: tool})
		if err != nil {
			return err
		}
		return a.output(cmd, state)
	}}
	add.Flags().String("tool", "", "target tool (see skillhub tools)")
	_ = add.MarkFlagRequired("tool")
	command.AddCommand(add)
	remove := &cobra.Command{Use: "remove NAME", Short: "Remove a manifest entry; installed files remain until explicitly removed", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		tool, _ := cmd.Flags().GetString("tool")
		state, err := a.core.RemoveProjectSkill(cmd.Context(), directory, tool, args[0])
		if err != nil {
			return err
		}
		return a.output(cmd, state)
	}}
	remove.Flags().String("tool", "", "target tool (see skillhub tools)")
	_ = remove.MarkFlagRequired("tool")
	command.AddCommand(remove)
	command.AddCommand(&cobra.Command{Use: "lock", Short: "Resolve configured sources and pin authorized package digests", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		lock, err := a.core.LockProject(cmd.Context(), directory)
		if err != nil {
			return err
		}
		return a.output(cmd, lock)
	}})
	restore := &cobra.Command{Use: "restore", Short: "Plan or restore every locked skill after rechecking source identity and authorization", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		plan, err := a.core.PlanProjectRestore(cmd.Context(), directory)
		if err != nil {
			return err
		}
		yes, _ := cmd.Flags().GetBool("yes")
		dry, _ := cmd.Flags().GetBool("dry-run")
		if !yes {
			if err = a.output(cmd, plan); err != nil {
				return err
			}
			if dry {
				return nil
			}
			return ErrPlanOnly
		}
		results, err := a.core.ApplyProjectRestore(cmd.Context(), plan)
		if err != nil {
			return err
		}
		if err = a.output(cmd, results); err != nil {
			return err
		}
		for _, result := range results {
			if !result.Applied {
				return errors.New("one or more project targets failed; inspect the per-target results")
			}
		}
		return nil
	}}
	planFlags(restore)
	command.AddCommand(restore)
	return command
}
