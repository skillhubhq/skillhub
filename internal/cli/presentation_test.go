package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
)

func render(t *testing.T, value any, machine bool) string {
	t.Helper()
	var out bytes.Buffer
	cmd := &cobra.Command{Use: "publish"}
	cmd.SetOut(&out)
	if err := (&application{json: machine}).output(cmd, value); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestHelpAndCompletionDoNotCreateLocalState(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}, {"install", "--help"}, {"completion", "bash"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "absent")
			var out, stderr bytes.Buffer
			if err := Execute(context.Background(), append([]string{"--data", dir}, args...), strings.NewReader(""), &out, &stderr, Options{}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("help or completion created state")
			}
			if out.Len() == 0 || strings.Contains(out.String(), "\x1b") {
				t.Fatal("missing or decorated redirected help")
			}
			if len(args) <= 1 && !strings.Contains(out.String(), "Discover skills:") {
				t.Fatal("missing command groups")
			}
		})
	}
}

func TestPlanPresentationAndMachineContract(t *testing.T) {
	before := &client.Installation{Namespace: "team", Name: "review", Version: "1.0.0", Source: "work", Tool: "codex", Scope: "project", Digest: "private-digest"}
	after := *before
	after.Version = "1.1.0"
	plan := &client.InstallPlan{Action: "update", Before: before, After: &after, Target: "/project/.agents/skills/review"}
	text := render(t, plan, false)
	for _, want := range []string{"Change preview", "team/review", "work", "1.0.0 → 1.1.0", plan.Target, "No files changed", "--yes"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "private-digest") || strings.Contains(text, "\x1b") {
		t.Fatal("unexpected internal fields or terminal codes")
	}
	applied := render(t, appliedPlan{true, plan}, false)
	if !strings.Contains(applied, "Change applied") || strings.Contains(applied, "No files changed") {
		t.Fatal(applied)
	}
	for _, value := range []any{plan, appliedPlan{true, plan}} {
		got := render(t, value, true)
		want, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want)+"\n" {
			t.Fatalf("JSON contract changed: %s", got)
		}
	}
}

func TestEmptyPartialAndUnavailablePresentation(t *testing.T) {
	for _, test := range []struct {
		value any
		want  string
	}{
		{[]client.Source{}, "skillhub source add"},
		{[]client.Installation{}, "No skills installed"},
		{[]client.SearchResult{}, "skillhub source add"},
		{[]client.SearchResult{{Source: "work"}}, "No matching skills"},
		{[]client.SearchResult{{Source: "work", Error: "sign in required"}}, "Unavailable"},
		{[]client.UpdateCheck{{VersionUnavailable: true}}, "no longer available"},
		{[]client.UpdateCheck{{Error: "offline", ConnectionRequired: true}}, "Choose a connection"},
		{[]client.UpdateCheck{{AvailableVersion: "1.2.0"}}, "1.2.0"},
		{client.CacheInfo{Entries: 3, LimitEntries: 128, LimitBytes: 256 << 20}, "3 / 128"},
	} {
		got := render(t, test.value, false)
		if !strings.Contains(got, test.want) {
			t.Fatalf("want %q in %s", test.want, got)
		}
		if strings.Contains(got, "offline") && strings.Contains(got, "Up to date") {
			t.Fatal("failed check presented as current")
		}
	}
	got := render(t, []client.SearchResult{{Source: "offline", Error: "unavailable"}, {Source: "work", NextPage: "cursor", Skills: []*catalogv1.Skill{{Namespace: "team", Name: "review", LatestVersion: "1.0.0"}}}}, false)
	for _, want := range []string{"1 skill", "work/team/review@1.0.0", "offline", "--page-token cursor"} {
		if !strings.Contains(got, want) {
			t.Fatalf("partial result lost: %s", got)
		}
	}
}

func TestHumanOutputSanitizesControlsWithoutChangingJSON(t *testing.T) {
	description := "First\nforged\tcolumn\x1b[2J\u202ehidden"
	values := []client.SearchResult{{Source: "work", Skills: []*catalogv1.Skill{{Name: "example", Description: description}}}}
	human := render(t, values, false)
	if strings.ContainsAny(human, "\x1b\t\u202e") || strings.Contains(human, "\nforged") {
		t.Fatal("terminal content was not sanitized")
	}
	var decoded []client.SearchResult
	if err := json.Unmarshal([]byte(render(t, values, true)), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[0].Skills[0].Description != description {
		t.Fatal("JSON payload was changed")
	}
}

func TestPublicationStatusAndLargeNumbersRemainAccurate(t *testing.T) {
	response := &publishingv1.SubmitReleaseResponse{Release: &catalogv1.Release{ReleaseId: "release-1", Status: taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING}}
	human := render(t, response, false)
	if !strings.Contains(human, "Pending") || !strings.Contains(human, "release-1") || strings.Contains(human, "Published") {
		t.Fatal(human)
	}
	machine := render(t, response, true)
	if !strings.Contains(machine, "RELEASE_STATUS_PENDING") || !json.Valid([]byte(machine)) {
		t.Fatal(machine)
	}
	if got := render(t, map[string]uint64{"revision": 18446744073709551615}, false); !strings.Contains(got, "18446744073709551615") {
		t.Fatal("integer precision lost")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed output") }

func TestOutputFailuresAreReturned(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.SetOut(failingWriter{})
	for _, machine := range []bool{false, true} {
		if err := (&application{json: machine}).output(cmd, []client.Source{}); err == nil {
			t.Fatal("write failure ignored")
		}
	}
}

func TestErrorsHaveSafeActionableMessages(t *testing.T) {
	var out bytes.Buffer
	PrintError(&out, client.ErrModified)
	if !strings.Contains(out.String(), "back up your local edits") {
		t.Fatal(out.String())
	}
	out.Reset()
	PrintError(&out, ErrPlanOnly)
	if strings.HasPrefix(out.String(), "Error:") || !strings.Contains(out.String(), "--yes") {
		t.Fatal(out.String())
	}
	out.Reset()
	PrintError(&out, errors.New("remote\x1b[2J\nforged"))
	if strings.Contains(out.String(), "\x1b") || strings.Contains(out.String(), "\nforged") {
		t.Fatal("unsafe error")
	}
}

func TestColorPolicy(t *testing.T) {
	var output bytes.Buffer
	if terminalColor(&output, false) {
		t.Fatal("redirected output must not use colors")
	}
	p := presentation{color: true}
	p.heading("SkillHub")
	if !strings.Contains(p.String(), "\x1b[1;34m") || !strings.Contains(p.String(), "\x1b[0m") {
		t.Fatal("missing color reset")
	}
	t.Setenv("NO_COLOR", "1")
	if terminalColor(os.Stdout, false) {
		t.Fatal("NO_COLOR ignored")
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if terminalColor(os.Stdout, false) || terminalColor(os.Stdout, true) {
		t.Fatal("disabled colors ignored")
	}
}
