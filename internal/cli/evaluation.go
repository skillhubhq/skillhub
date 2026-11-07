package cli

import (
	"errors"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/evaluation"
	"github.com/spf13/cobra"
	"io"
	"os"
)

func evaluationFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open evaluation file")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > evaluation.MaxBytes {
		return nil, evaluation.ErrInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(f, evaluation.MaxBytes+1))
	if err != nil || len(raw) > evaluation.MaxBytes {
		return nil, evaluation.ErrInvalid
	}
	return raw, nil
}
func (a *application) evaluationCommand() *cobra.Command {
	root := &cobra.Command{Use: "eval", Short: "Validate and import external evaluation evidence; never execute an agent"}
	validate := &cobra.Command{Use: "validate FILE", Short: "Validate a case suite or external result file offline", Args: cobra.ExactArgs(1), Annotations: map[string]string{"local-authoring": "true"}, RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := evaluationFile(args[0])
		if err != nil {
			return err
		}
		cases, _ := cmd.Flags().GetBool("cases")
		if cases {
			suite, err := evaluation.ParseSuite(raw)
			if err != nil {
				return err
			}
			return a.output(cmd, map[string]any{"valid": true, "cases": len(suite.Cases)})
		}
		report, err := evaluation.Parse(raw)
		if err != nil {
			return err
		}
		return a.output(cmd, map[string]any{"valid": true, "suite_digest": report.SuiteDigest(), "results": report.Summary(), "externally_reported": true})
	}}
	validate.Flags().Bool("cases", false, "validate a skillhub.cases.v1 suite instead of results")
	root.AddCommand(validate)
	source := func(cmd *cobra.Command) string { s, _ := cmd.Flags().GetString("source"); return s }
	root.AddCommand(a.remoteCommand("import RELEASE_ID FILE", "Attach sanitized external evidence to an exact version", cobra.ExactArgs(2), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		raw, err := evaluationFile(args[1])
		if err != nil {
			return err
		}
		id, err := a.core.ImportEvaluation(cmd.Context(), source(cmd), args[0], string(raw), a.mutation().IdempotencyKey)
		if err != nil {
			return err
		}
		return a.output(cmd, map[string]string{"id": id})
	}))
	root.AddCommand(a.remoteCommand("list RELEASE_ID", "List evidence summaries for one accessible version", cobra.ExactArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		v, err := a.core.Evaluations(cmd.Context(), source(cmd), args[0])
		if err != nil {
			return err
		}
		return a.output(cmd, v)
	}))
	root.AddCommand(a.remoteCommand("show EVIDENCE_ID", "Read one accessible evidence report", cobra.ExactArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		v, err := a.core.Evaluation(cmd.Context(), source(cmd), args[0])
		if err != nil {
			return err
		}
		return a.output(cmd, v)
	}))
	retract := a.remoteCommand("retract EVIDENCE_ID", "Retract report content while preserving its audit digest", cobra.ExactArgs(1), func(cmd *cobra.Command, _ *client.Remote, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			return errors.New("retraction removes report content; repeat with --yes after checking the evidence ID")
		}
		return a.core.RetractEvaluation(cmd.Context(), source(cmd), args[0], a.mutation().IdempotencyKey)
	})
	retract.Flags().Bool("yes", false, "confirm removal of report content")
	root.AddCommand(retract)
	return root
}
