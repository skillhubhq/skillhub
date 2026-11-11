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

	"github.com/skillhubhq/skillhub/internal/client"
)

func TestLocalCommandsPreviewEditAndRestoreWithoutHub(t *testing.T) {
	home, data := t.TempDir(), t.TempDir()
	target := filepath.Join(home, ".agents/skills/check")
	if e := os.MkdirAll(target, 0700); e != nil {
		t.Fatal(e)
	}
	original := "---\nname: check\ndescription: Local command test\n---\nOriginal\n"
	if e := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte(original), 0600); e != nil {
		t.Fatal(e)
	}
	run := func(input string, args ...string) (string, error) {
		var out, stderr bytes.Buffer
		e := Execute(context.Background(), append([]string{"--data", data, "--json"}, args...), strings.NewReader(input), &out, &stderr, Options{Client: client.ClientOptions{Home: home}})
		return out.String(), e
	}
	out, e := run("", "local", "list")
	if e != nil || !strings.Contains(out, "Local command test") {
		t.Fatal("discovery failed", out, e)
	}
	out, e = run("", "local", "read", target)
	if e != nil {
		t.Fatal(e)
	}
	var doc client.LocalDocument
	if e = json.Unmarshal([]byte(out), &doc); e != nil {
		t.Fatal(e)
	}
	if _, e = run(original+"Edited", "local", "edit", target, "SKILL.md", "--fingerprint", doc.Fingerprint); !errors.Is(e, ErrPlanOnly) {
		t.Fatal("edit applied without consent", e)
	}
	raw, _ := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if string(raw) != original {
		t.Fatal("preview wrote files")
	}
	if _, e = run(original+"Edited", "local", "edit", target, "SKILL.md", "--fingerprint", doc.Fingerprint, "--yes"); e != nil {
		t.Fatal(e)
	}
	out, e = run("", "local", "backups")
	if e != nil {
		t.Fatal(e)
	}
	var backups []client.LocalBackup
	if e = json.Unmarshal([]byte(out), &backups); e != nil || len(backups) != 1 {
		t.Fatal(out, e)
	}
	if _, e = run("", "local", "restore", backups[0].ID, "--yes"); e != nil {
		t.Fatal(e)
	}
	raw, _ = os.ReadFile(filepath.Join(target, "SKILL.md"))
	if string(raw) != original {
		t.Fatal("restore failed")
	}
	if _, e = run("", "local", "hide", target); e != nil {
		t.Fatal(e)
	}
	out, e = run("", "local", "list")
	if e != nil || !strings.Contains(out, `"hidden": true`) {
		t.Fatal(out, e)
	}
}
