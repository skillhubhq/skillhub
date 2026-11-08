package cli

import (
	"errors"
	"io"
	"os"

	"connectrpc.com/connect"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/spf13/cobra"
)

func (a *application) draftsCommand() *cobra.Command {
	root := &cobra.Command{Use: "draft", Short: "Read or save the device-local publishing draft for a Hub account"}
	for _, action := range []string{"show", "save", "clear"} {
		action := action
		cmd := &cobra.Command{Use: action + " SOURCE", Short: action + " the current account's publishing draft", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			r, err := a.core.Remote(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			identity, err := r.Auth.GetIdentity(cmd.Context(), connect.NewRequest(&authv1.GetIdentityRequest{}))
			if err != nil {
				return err
			}
			account := identity.Msg.GetIdentity().GetUserId()
			if action == "show" {
				payload, err := a.core.LoadPublicationDraft(cmd.Context(), args[0], account)
				if err != nil {
					return err
				}
				return a.output(cmd, map[string]string{"draft": payload})
			}
			payload := ""
			if action == "save" {
				name, _ := cmd.Flags().GetString("file")
				f, err := os.Open(name)
				if err != nil {
					return err
				}
				defer f.Close()
				data, err := io.ReadAll(io.LimitReader(f, 32<<20+1))
				if err != nil {
					return err
				}
				if len(data) > 32<<20 {
					return errors.New("draft exceeds size limit")
				}
				payload = string(data)
			}
			return a.core.SavePublicationDraft(cmd.Context(), args[0], account, payload)
		}}
		if action == "save" {
			cmd.Flags().String("file", "", "JSON publishing draft file")
			_ = cmd.MarkFlagRequired("file")
		}
		root.AddCommand(cmd)
	}
	return root
}
