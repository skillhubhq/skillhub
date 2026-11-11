package cli

import (
	"errors"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
	"io"
)

func (a *application) localCommand() *cobra.Command {
	root := &cobra.Command{Use: "local", Short: "Discover, inspect and manage skills already on this device"}
	root.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, Short: "Scan tool folders, projects and additional folders", RunE: func(cmd *cobra.Command, _ []string) error {
		v, e := a.core.LocalInventory(cmd.Context())
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	}})
	for _, action := range []string{"add", "remove"} {
		add := action == "add"
		root.AddCommand(&cobra.Command{Use: action + "-folder PATH", Args: cobra.ExactArgs(1), Short: "Change additional discovery folders without moving skills", RunE: func(cmd *cobra.Command, args []string) error {
			if e := a.core.SetLocalRoot(cmd.Context(), args[0], add); e != nil {
				return e
			}
			v, e := a.core.LocalRoots(cmd.Context())
			if e != nil {
				return e
			}
			return a.output(cmd, v)
		}})
	}
	for _, action := range []string{"hide", "show"} {
		hidden := action == "hide"
		root.AddCommand(&cobra.Command{Use: action + " TARGET", Args: cobra.ExactArgs(1), Short: "Change local list visibility without changing files", RunE: func(cmd *cobra.Command, args []string) error {
			return a.core.SetLocalHidden(cmd.Context(), args[0], hidden)
		}})
	}
	root.AddCommand(&cobra.Command{Use: "read TARGET [FILE]", Args: cobra.RangeArgs(1, 2), Short: "Inspect a local skill and its file inventory", RunE: func(cmd *cobra.Command, args []string) error {
		path := "SKILL.md"
		if len(args) > 1 {
			path = args[1]
		}
		v, e := a.core.ReadLocalSkill(cmd.Context(), args[0], path)
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	}})
	source := &cobra.Command{Use: "associate TARGET SOURCE/NAMESPACE/NAME@VERSION", Args: cobra.ExactArgs(2), Short: "Compare and associate a Hub version; optionally replace with a backup", RunE: func(cmd *cobra.Command, args []string) error {
		coord, e := client.ParseCoordinate(args[1], true)
		if e != nil {
			return e
		}
		replace, _ := cmd.Flags().GetBool("replace")
		p, e := a.core.PlanLocalSource(cmd.Context(), args[0], coord, replace)
		if e != nil {
			return e
		}
		return a.applyLocal(cmd, p)
	}}
	source.Flags().Bool("replace", false, "back up existing files and use the selected version")
	planFlags(source)
	root.AddCommand(source)
	for _, action := range []string{"detach", "remove", "restore", "copy"} {
		action := action
		use := action + " TARGET"
		n := 1
		if action == "restore" {
			use = "restore BACKUP_ID"
		}
		if action == "copy" {
			use += " DESTINATION"
			n = 2
		}
		cmd := &cobra.Command{Use: use, Args: cobra.ExactArgs(n), Short: "Preview a local change; file changes retain a recovery point", RunE: func(cmd *cobra.Command, args []string) error {
			var p client.LocalPlan
			var e error
			switch action {
			case "detach":
				p, e = a.core.PlanLocalDetach(cmd.Context(), args[0])
			case "remove":
				p, e = a.core.PlanLocalRemove(cmd.Context(), args[0])
			case "restore":
				p, e = a.core.PlanLocalRestore(cmd.Context(), args[0])
			case "copy":
				p, e = a.core.PlanLocalCopy(cmd.Context(), args[0], args[1])
			}
			if e != nil {
				return e
			}
			return a.applyLocal(cmd, p)
		}}
		planFlags(cmd)
		root.AddCommand(cmd)
	}
	edit := &cobra.Command{Use: "edit TARGET FILE", Args: cobra.ExactArgs(2), Short: "Read replacement text from stdin; require the inspected fingerprint", RunE: func(cmd *cobra.Command, args []string) error {
		fp, _ := cmd.Flags().GetString("fingerprint")
		raw, e := io.ReadAll(io.LimitReader(cmd.InOrStdin(), (1<<20)+1))
		if e != nil {
			return e
		}
		if len(raw) > 1<<20 {
			return errors.New("text exceeds 1 MiB")
		}
		p, e := a.core.PlanLocalEdit(cmd.Context(), args[0], args[1], fp, string(raw))
		if e != nil {
			return e
		}
		return a.applyLocal(cmd, p)
	}}
	edit.Flags().String("fingerprint", "", "fingerprint returned by local read")
	_ = edit.MarkFlagRequired("fingerprint")
	planFlags(edit)
	root.AddCommand(edit)
	root.AddCommand(&cobra.Command{Use: "backups", Args: cobra.NoArgs, Short: "List recovery points for replacement, editing and removal", RunE: func(cmd *cobra.Command, _ []string) error {
		v, e := a.store.LocalBackups(cmd.Context())
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	}})
	cleanup := &cobra.Command{Use: "backup-delete BACKUP_ID", Args: cobra.ExactArgs(1), Short: "Permanently remove a selected recovery point", RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			return ErrPlanOnly
		}
		return a.store.DeleteLocalBackup(cmd.Context(), args[0])
	}}
	cleanup.Flags().Bool("yes", false, "confirm permanently deleting this recovery point")
	root.AddCommand(cleanup)
	return root
}
func (a *application) applyLocal(cmd *cobra.Command, p client.LocalPlan) error {
	yes, _ := cmd.Flags().GetBool("yes")
	dry, _ := cmd.Flags().GetBool("dry-run")
	if !yes {
		if e := a.output(cmd, p); e != nil {
			return e
		}
		if dry {
			return nil
		}
		return ErrPlanOnly
	}
	if e := a.core.ApplyLocalPlan(cmd.Context(), p); e != nil {
		return e
	}
	return a.output(cmd, map[string]any{"applied": true, "target": p.Target, "action": p.Action})
}
