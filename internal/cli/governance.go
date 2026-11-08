package cli

import (
	"errors"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
)

func (a *application) governanceCommand() *cobra.Command {
	root := &cobra.Command{Use: "governance", Short: "Inspect package scans, review notifications and platform duties"}
	source := func(cmd *cobra.Command) string { v, _ := cmd.Flags().GetString("source"); return v }
	for _, action := range []string{"scan-report", "scan-run"} {
		root.AddCommand(a.remoteCommand(action+" RELEASE_ID", "Read or run an offline package scan", cobra.ExactArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
			var v client.ScanInfo
			var e error
			if action == "scan-run" {
				v, e = a.core.ScanRelease(cmd.Context(), source(cmd), args[0], a.mutation().IdempotencyKey)
			} else {
				v, e = a.core.ScanReport(cmd.Context(), source(cmd), args[0])
			}
			if e != nil {
				return e
			}
			return a.output(cmd, v)
		}))
	}
	notices := a.remoteCommand("notifications", "List review outcomes and replies for your Hub account", cobra.NoArgs, func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		token, _ := cmd.Flags().GetString("page-token")
		v, e := a.core.Notifications(cmd.Context(), source(cmd), token)
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	})
	notices.Flags().String("page-token", "", "notification cursor")
	root.AddCommand(notices)
	root.AddCommand(a.remoteCommand("notifications-read-all", "Mark all your notifications as read", cobra.NoArgs, func(cmd *cobra.Command, _ *client.Remote, _ []string) error {
		return a.core.MarkAllNotificationsRead(cmd.Context(), source(cmd))
	}))
	root.AddCommand(a.remoteCommand("notification-read NOTIFICATION_ID", "Mark your notification as read", cobra.ExactArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		return a.core.MarkNotificationRead(cmd.Context(), source(cmd), args[0])
	}))
	root.AddCommand(a.remoteCommand("access [USER_ID]", "Read platform duties (other identities require Hub administration)", cobra.MaximumNArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		id := ""
		if len(args) > 0 {
			id = args[0]
		}
		v, e := a.core.PlatformAccess(cmd.Context(), source(cmd), id)
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	}))
	role := a.remoteCommand("role USER_ID ROLE", "Grant people-admin or auditor duties; Hub administrator only", cobra.ExactArgs(2), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		roles := map[string]taxonomy.PlatformRole{"people-admin": taxonomy.PlatformRole_PLATFORM_ROLE_USER_ADMIN, "auditor": taxonomy.PlatformRole_PLATFORM_ROLE_AUDITOR}
		r, ok := roles[args[1]]
		if !ok {
			return errors.New("role must be people-admin or auditor")
		}
		remove, _ := cmd.Flags().GetBool("remove")
		return a.core.SetPlatformRole(cmd.Context(), source(cmd), args[0], r, !remove, a.mutation().IdempotencyKey)
	})
	role.Flags().Bool("remove", false, "revoke duty and active sessions")
	root.AddCommand(role)
	tasks := a.remoteCommand("review-tasks", "List actionable pending reviews, oldest first", cobra.NoArgs, func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		token, _ := cmd.Flags().GetString("page-token")
		v, e := a.core.ReviewTasks(cmd.Context(), source(cmd), token)
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	})
	tasks.Flags().String("page-token", "", "review task cursor")
	root.AddCommand(tasks)
	featured := a.remoteCommand("feature NAMESPACE NAME", "Curate a skill; Hub administrator only", cobra.ExactArgs(2), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		remove, _ := cmd.Flags().GetBool("remove")
		return a.core.SetFeaturedSkill(cmd.Context(), source(cmd), args[0], args[1], !remove, a.mutation().IdempotencyKey)
	})
	featured.Flags().Bool("remove", false, "remove from featured")
	root.AddCommand(featured)
	return root
}
