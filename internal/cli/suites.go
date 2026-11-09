package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
)

func (a *application) suitesCommand() *cobra.Command {
	root := &cobra.Command{Use: "suites", Short: "Publish and inspect exact-version skill suites"}
	root.AddCommand(&cobra.Command{Use: "list", Short: "List installed suites and their members", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		v, e := a.store.Suites(cmd.Context())
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	}})
	publish := a.remoteCommand("publish FILE", "Submit a suite definition through normal review", cobra.ExactArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		f, e := os.Open(args[0])
		if e != nil {
			return e
		}
		defer f.Close()
		d := json.NewDecoder(io.LimitReader(f, 65537))
		d.DisallowUnknownFields()
		var form client.PublishSuite
		if e = d.Decode(&form); e != nil {
			return e
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return errors.New("invalid suite definition")
		}
		form.Source, _ = cmd.Flags().GetString("source")
		form.RequestID = a.mutation().IdempotencyKey
		if form.Visibility == 0 {
			form.Visibility = taxonomy.Visibility_VISIBILITY_MEMBERS
		}
		v, e := a.core.PublishSuite(cmd.Context(), form)
		if e != nil {
			return e
		}
		return a.output(cmd, v)
	})
	root.AddCommand(publish)
	return root
}
