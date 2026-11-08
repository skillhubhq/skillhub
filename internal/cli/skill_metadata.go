package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
)

func (a *application) skillMetadataCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "edit-details SOURCE", Short: "Update skill presentation from a JSON document with expected_revision", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("file")
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		var change client.SkillMetadataChange
		data, err := io.ReadAll(io.LimitReader(f, 32769))
		if err != nil {
			return err
		}
		if len(data) > 32768 {
			return errors.New("metadata exceeds size limit")
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&change); err != nil {
			return err
		}
		if decoder.Decode(new(any)) != io.EOF {
			return errors.New("expected one metadata document")
		}
		return a.core.UpdateSkillMetadata(cmd.Context(), args[0], change)
	}}
	cmd.Flags().String("file", "", "metadata JSON document")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}
