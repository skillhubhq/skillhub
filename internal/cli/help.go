package cli

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func (a *application) configureHelp(root *cobra.Command) {
	root.Long = "SkillHub\nDiscover, share and install skills from your team's Hubs."
	root.Example = `  skillhub source add work https://skills.example.com
  skillhub login work
  skillhub search review --source work
  skillhub install work/team/review@1.0.0 --tool codex --dry-run`
	root.AddGroup(
		&cobra.Group{ID: "discover", Title: "Discover skills:"},
		&cobra.Group{ID: "local", Title: "Use on this device:"},
		&cobra.Group{ID: "publish", Title: "Publish and govern:"},
		&cobra.Group{ID: "account", Title: "Connections and accounts:"},
		&cobra.Group{ID: "other", Title: "More:"},
	)
	root.SetHelpCommandGroupID("other")
	root.SetCompletionCommandGroupID("other")
	summaries := map[string]string{
		"search":          "Search one or all connected Hubs",
		"info":            "Read a skill's details",
		"versions":        "List available versions",
		"install":         "Preview or install an exact version",
		"list":            "Show installed skills",
		"update":          "Preview or apply a skill update",
		"updates":         "Check installed skills for updates",
		"remove":          "Preview or remove a managed skill",
		"project":         "Pin and restore a project's skills",
		"cache":           "Inspect or clear downloaded packages",
		"publish":         "Submit a skill for publication or review",
		"review":          "Inspect submissions and leave decisions",
		"governance":      "Review tasks, scans and notifications",
		"admin":           "Manage Hub accounts and permissions",
		"yank":            "Withdraw a skill version",
		"source":          "Manage Hub connections",
		"login":           "Sign in to a Hub",
		"logout":          "Sign out of a Hub",
		"whoami":          "Show your identity on a Hub",
		"password-change": "Change a local account's password",
	}
	for _, c := range root.Commands() {
		if summary := summaries[c.Name()]; summary != "" {
			if c.Long == "" {
				c.Long = c.Short
			}
			c.Short = summary
		}
		switch c.Name() {
		case "search", "info", "versions":
			c.GroupID = "discover"
		case "install", "list", "update", "updates", "remove", "project", "cache":
			c.GroupID = "local"
		case "publish", "review", "yank", "admin", "governance":
			c.GroupID = "publish"
		case "source", "login", "logout", "whoami", "password-change":
			c.GroupID = "account"
		default:
			c.GroupID = "other"
		}
	}
	examples := map[string]string{
		"source":  "  skillhub source add work https://skills.example.com\n  skillhub source list",
		"search":  "  skillhub search review\n  skillhub search review --source work --order recent\n  skillhub search --source work --featured --json",
		"install": "  skillhub install work/team/review@1.0.0 --tool codex --dry-run\n  skillhub install work/team/review@1.0.0 --tool codex --yes\n  skillhub install work/team/review@1.0.0 --tool claude-code --scope global --yes",
		"publish": "  skillhub publish ./my-skill --source work --namespace team --version 1.0.0",
		"review":  "  skillhub governance review-tasks --source work\n  skillhub review inspect RELEASE_ID --source work\n  skillhub review approve RELEASE_ID --source work --reason \"Reviewed package contents\"",
		"project": "  skillhub project init\n  skillhub project add work/team/review@1.0.0 --tool codex\n  skillhub project lock\n  skillhub project restore --dry-run",
		"login":   "  skillhub login work\n  skillhub login work --username reader --password-stdin",
	}
	for _, c := range root.Commands() {
		if example := examples[c.Name()]; example != "" {
			c.Example = example
		}
	}
	baseHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()
		if !terminalColor(out, a.noColor || a.json) {
			baseHelp(cmd, args)
			return
		}
		var buffer bytes.Buffer
		cmd.SetOut(&buffer)
		baseHelp(cmd, args)
		cmd.SetOut(out)
		for _, line := range strings.SplitAfter(buffer.String(), "\n") {
			text := strings.TrimSuffix(line, "\n")
			if text != "" && !strings.HasPrefix(text, " ") && (text == "SkillHub" || strings.HasSuffix(text, ":")) {
				fmt.Fprint(out, "\x1b[1;34m"+text+"\x1b[0m\n")
			} else {
				fmt.Fprint(out, line)
			}
		}
	})
}
