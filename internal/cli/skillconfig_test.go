package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

func TestConfigurationCLIAndSharedClientNeverPrintSecrets(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := client.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "config-check")
	for name, text := range map[string]string{"SKILL.md": "---\nname: config-check\ndescription: Test configuration.\n---\n", client.SkillConfigFile: `{"version":1,"variables":[{"name":"EXAMPLE_TOKEN","secret":true,"required":true}]}`} {
		if err = os.WriteFile(filepath.Join(source, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	archive, bundle, err := packageformat.Pack(source)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.PlanInstall(ctx, client.Installation{RegistryID: "example-hub", RegistryURL: "https://skills.example.com", Namespace: "team", Name: "config-check", Version: "1.0.0", Digest: bundle.Digest, Tool: "codex", Scope: "project", Target: target}, archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Apply(ctx, *plan, archive); err != nil {
		t.Fatal(err)
	}
	target = plan.Target
	vault := &memoryCredentials{values: map[string]string{}}
	options := client.ClientOptions{Credentials: vault}
	core := client.NewClient(store, options)
	secret := "synthetic-private-input"
	run := func(input string, args ...string) (string, error) {
		var out, stderr bytes.Buffer
		err := Execute(ctx, append([]string{"--data", root, "--json"}, args...), strings.NewReader(input), &out, &stderr, Options{Client: options})
		text := out.String() + stderr.String()
		if strings.Contains(text, secret) || (err != nil && strings.Contains(err.Error(), secret)) {
			t.Fatal("secret leaked to CLI output")
		}
		return text, err
	}
	if _, err = run("", "config", "doctor", target); err == nil {
		t.Fatal("missing required secret passed")
	}
	if _, err = run("", "config", "set", target, "EXAMPLE_TOKEN", secret); err == nil {
		t.Fatal("value accepted in argv")
	}
	if _, err = run(secret+"\n", "config", "set", target, "EXAMPLE_TOKEN", "--value-stdin"); err != nil {
		t.Fatal(err)
	}
	if report, e := core.SkillConfiguration(ctx, target); e != nil || !report.Ready {
		t.Fatal("App core could not observe CLI secret", e)
	}
	if _, err = run("", "config", "doctor", target); err != nil {
		t.Fatal(err)
	}
	if _, err = run(strings.Repeat("a", 4096)+"\r\nextra", "config", "set", target, "EXAMPLE_TOKEN", "--value-stdin"); err == nil {
		t.Fatal("oversized stdin was truncated and accepted")
	}
	if _, err = run("", "config", "clear", target); err == nil {
		t.Fatal("clear did not require explicit confirmation")
	}
	if err = core.ClearSkillConfiguration(ctx, target); err != nil {
		t.Fatal(err)
	}
	if _, err = run("", "config", "doctor", target); err == nil {
		t.Fatal("CLI did not observe App cleanup")
	}
}
