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

	"github.com/skillhubhq/skillhub/internal/packageformat"
)

func TestOfflineAuthoringPreservesFilesAndDoesNotInitializeAccounts(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "api-review")
	state := filepath.Join(root, "no-client-state")
	run := func(args ...string) (string, error) {
		var out, stderr bytes.Buffer
		err := Execute(context.Background(), append([]string{"--data", state, "--json"}, args...), strings.NewReader(""), &out, &stderr, Options{})
		return out.String(), err
	}
	if _, err := run("init", directory, "--description", "Review API compatibility: inputs, outputs and errors."); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(directory, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run("init", directory, "--description", "replacement"); err == nil {
		t.Fatal("overwrote existing directory")
	}
	current, _ := os.ReadFile(filepath.Join(directory, "SKILL.md"))
	if !bytes.Equal(original, current) {
		t.Fatal("original changed")
	}
	text, err := run("validate", directory)
	if err != nil {
		t.Fatal(err)
	}
	var result authoringResult
	if err = json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatal(err)
	}
	if result.Name != "api-review" || result.Files != 1 || result.Scan == nil || !result.Scan.Complete || result.Scan.Blocking {
		t.Fatal(text)
	}
	if _, err = os.Stat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("offline authoring initialized account state")
	}
	secretShape := "gh" + "p_" + strings.Repeat("a", 36)
	if err = os.WriteFile(filepath.Join(directory, "unsafe.txt"), []byte(secretShape), 0600); err != nil {
		t.Fatal(err)
	}
	text, err = run("validate", directory)
	if err == nil || strings.Contains(text, secretShape) || !strings.Contains(text, "credential-token") {
		t.Fatal("blocking scan did not fail safely", err, text)
	}
}

func TestAuthoringRejectsInvalidNamesAndEscapesMetadata(t *testing.T) {
	for _, name := range []string{"../escape", "Upper Case", "bad_name"} {
		if _, err := packageformat.Template(name, "description"); err == nil {
			t.Fatal("accepted invalid name", name)
		}
	}
	root := t.TempDir()
	directory := filepath.Join(root, "safe-skill")
	description := "When reviewing YAML:\n---\nname: injected"
	if _, err := packageformat.InitDirectory(directory, "safe-skill", description); err != nil {
		t.Fatal(err)
	}
	_, bundle, err := packageformat.Pack(directory)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Metadata.Name != "safe-skill" || bundle.Metadata.Description != description {
		t.Fatal("metadata injection")
	}
	if _, err := packageformat.Template("valid-name", " "); err == nil {
		t.Fatal("accepted empty description")
	}
}

func TestAuthoringRejectsSymlinkDestination(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "existing")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable", err)
	}
	if _, err := packageformat.InitDirectory(link, "safe-skill", "description"); err == nil {
		t.Fatal("accepted symlink destination")
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatal("modified symlink target", err)
	}
}

func TestToolCatalogDoesNotOpenLocalState(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	var out bytes.Buffer
	if e := Execute(context.Background(), []string{"--data", state, "tools"}, strings.NewReader(""), &out, &out, Options{}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "GitHub Copilot") {
		t.Fatal(out.String())
	}
	if _, e := os.Stat(state); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("static tool list modified local state")
	}
}
