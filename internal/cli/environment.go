package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed shell/environment.sh
var environmentShell string

//go:embed shell/environment.ps1
var environmentPowerShell string

func (a *application) environmentCommand() *cobra.Command {
	root := &cobra.Command{Use: "env", Short: "Load local skill settings into your own terminal; never launch an agent"}
	init := &cobra.Command{Use: "init SHELL", Short: "Print the value-free Bash, Zsh or PowerShell integration", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		script := environmentShell
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
		switch args[0] {
		case "bash", "zsh":
		case "powershell":
			script = environmentPowerShell
			quote = func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
		default:
			return errors.New("supported shells: bash, zsh, powershell")
		}
		script = strings.ReplaceAll(script, "@EXECUTABLE@", quote(exe))
		script = strings.ReplaceAll(script, "@DATA@", quote(a.data))
		_, err = fmt.Fprint(cmd.OutOrStdout(), script)
		return err
	}}
	doctor := &cobra.Command{Use: "doctor TARGET [TARGET...]", Short: "Check this process environment without displaying values", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := a.core.CheckEnvironment(cmd.Context(), args)
		if err != nil {
			return err
		}
		if err = a.output(cmd, result); err != nil {
			return err
		}
		for _, v := range result {
			if v.Status != "present" {
				return errors.New("current process environment is not ready; no values were displayed")
			}
		}
		return nil
	}}
	read := &cobra.Command{Use: "_read TARGET [TARGET...]", Hidden: true, Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		f, ok := cmd.OutOrStdout().(*os.File)
		if !ok {
			return errors.New("environment transfer requires a local pipe")
		}
		info, err := f.Stat()
		if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
			return errors.New("environment transfer refuses terminals and regular files; use the shell integration")
		}
		format, _ := cmd.Flags().GetString("format")
		return a.core.WriteEnvironment(cmd.Context(), args, format, f)
	}}
	read.Flags().String("format", "nul", "internal pipe framing")
	root.AddCommand(init, doctor, read)
	return root
}
