package cli

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/skillscan"
	"github.com/spf13/cobra"
)

type authoringResult struct {
	Directory string            `json:"directory"`
	Name      string            `json:"name"`
	Files     int               `json:"files"`
	Scan      *skillscan.Report `json:"scan,omitempty"`
	Created   bool              `json:"created"`
}

func (a *application) initCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "init DIRECTORY", Short: "Create a skill directory for editing in your agent or editor", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			name = filepath.Base(filepath.Clean(args[0]))
		}
		description, _ := cmd.Flags().GetString("description")
		directory, err := packageformat.InitDirectory(args[0], name, description)
		if err != nil {
			return err
		}
		return a.output(cmd, authoringResult{Directory: directory, Name: name, Files: 1, Created: true})
	}}
	cmd.Annotations = map[string]string{"local-authoring": "true"}
	cmd.Flags().String("name", "", "stable lower-case skill name (default: directory name)")
	cmd.Flags().String("description", "", "when this skill should be used")
	_ = cmd.MarkFlagRequired("description")
	return cmd
}
func (a *application) validateCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "validate DIRECTORY", Short: "Check package structure and run an offline static scan", Long: "Validate the same package rules used by publishing, without contacting a Hub or executing files. Static findings are not a behavioral evaluation or a security guarantee.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		_, bundle, err := packageformat.Pack(args[0])
		if err != nil {
			return err
		}
		report, err := skillscan.Scan(cmd.Context(), bundle)
		if err != nil {
			return err
		}
		directory, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		if err = a.output(cmd, authoringResult{Directory: directory, Name: bundle.Metadata.Name, Files: len(bundle.Files), Scan: &report}); err != nil {
			return err
		}
		if report.Blocking {
			return errors.New("package contains blocking static findings")
		}
		return nil
	}}
	cmd.Annotations = map[string]string{"local-authoring": "true"}
	return cmd
}

func (p *presentation) authoring(result authoringResult) {
	if result.Created {
		p.heading("Skill created")
	} else {
		p.heading("Package validation")
	}
	p.field("Skill", result.Name)
	p.field("Directory", result.Directory)
	if result.Scan != nil {
		status := "No built-in rules matched"
		if !result.Scan.Complete || len(result.Scan.Findings) > 0 {
			status = "Human review required"
		}
		if result.Scan.Blocking {
			status = "Blocked"
		}
		p.field("Static scan", status)
		for _, finding := range result.Scan.Findings {
			p.field(finding.Rule, finding.Path+": "+finding.Message)
		}
		p.hint("Static checks do not measure skill effectiveness. Test representative tasks in your agent before publishing.")
	} else {
		p.hint("Open SKILL.md in your editor or ask your coding agent to refine its instructions.\nThen run: skillhub validate " + shellArgument(result.Directory))
	}
}

// 生成提示中的参数只供用户复制，不能把路径里的空格或引号解释成命令。
func shellArgument(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
