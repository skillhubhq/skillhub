package cli

import (
	"connectrpc.com/connect"
	"errors"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
	"io"
	"strings"
)

func (a *application) passwordCommand() *cobra.Command {
	cmd := a.remoteCommand("password-change", "Change your local password and revoke all sessions", cobra.NoArgs, func(cmd *cobra.Command, r *client.Remote, _ []string) error {
		stdin, _ := cmd.Flags().GetBool("passwords-stdin")
		if !stdin {
			return errors.New("use --passwords-stdin: current and new password on separate lines")
		}
		raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1025))
		if err != nil {
			return err
		}
		lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		if len(raw) > 1024 || len(lines) != 2 {
			return errors.New("stdin must contain exactly two password lines")
		}
		_, err = r.Auth.ChangePassword(cmd.Context(), connect.NewRequest(&authv1.ChangePasswordRequest{CurrentPassword: strings.TrimSuffix(lines[0], "\r"), NewPassword: strings.TrimSuffix(lines[1], "\r")}))
		if err != nil {
			return err
		}
		return a.output(cmd, map[string]bool{"password_changed": true, "sign_in_required": true})
	})
	cmd.Flags().Bool("passwords-stdin", false, "read current and new passwords from two stdin lines")
	return cmd
}
func (a *application) addAccountCommands(parent *cobra.Command) {
	reset := a.remoteCommand("user-password-reset USER_ID", "Reset another local account's password and revoke its sessions", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		stdin, _ := cmd.Flags().GetBool("password-stdin")
		if !stdin {
			return errors.New("use --password-stdin")
		}
		password, err := passwordInput(cmd)
		if err != nil {
			return err
		}
		response, err := r.Admin.ResetUserPassword(cmd.Context(), connect.NewRequest(&adminv1.ResetUserPasswordRequest{Mutation: a.mutation(), UserId: args[0], Password: password}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	})
	reset.Flags().Bool("password-stdin", false, "read new password from stdin")
	parent.AddCommand(reset)
	admin := a.remoteCommand("user-administrator USER_ID", "Grant or revoke Hub administrator access", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		enabled, _ := cmd.Flags().GetBool("enabled")
		response, err := r.Admin.SetUserAdministrator(cmd.Context(), connect.NewRequest(&adminv1.SetUserAdministratorRequest{Mutation: a.mutation(), UserId: args[0], Administrator: enabled}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	})
	admin.Flags().Bool("enabled", false, "grant when true, revoke when false")
	_ = admin.MarkFlagRequired("enabled")
	parent.AddCommand(admin)
}
