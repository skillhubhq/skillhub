package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentHandoffScopeIntegrityAndDiagnostics(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	vault := &configCredentials{values: map[string]string{}}
	c := NewClient(s, ClientOptions{Credentials: vault, HTTPClient: &http.Client{Transport: configNoNetwork{t}}})
	i := installConfigFixture(t, s, filepath.Join(t.TempDir(), "selected"), "1.0.0")
	secret := "synthetic-'\"$(touch SHOULD_NOT_EXIST)\n雪\\value"
	for _, v := range []struct{ name, mode, value string }{{"DEMO_TOKEN", "keychain", secret}, {"DEMO_REGION", "value", "test-region"}} {
		if err := c.SetSkillConfiguration(ctx, i.Target, v.name, v.mode, v.value); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := c.WriteEnvironment(ctx, []string{i.Target}, "json", &out); err != nil {
		t.Fatal(err)
	}
	var decoded []environmentValue
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 || decoded[1].Value != secret {
		t.Fatal("values did not round-trip")
	}
	report, err := c.CheckEnvironment(ctx, []string{i.Target})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(report)
	if bytes.Contains(raw, []byte(secret)) || report[1].Status != "missing" {
		t.Fatal("diagnostic leaked or misreported values")
	}
	t.Setenv("DEMO_TOKEN", secret)
	t.Setenv("DEMO_REGION", "test-region")
	report, err = c.CheckEnvironment(ctx, []string{i.Target})
	if err != nil || report[1].Status != "present" {
		t.Fatal("child environment not recognized", err)
	}
	out.Reset()
	if err = c.WriteEnvironment(ctx, []string{i.Target}, "nul", &out); err != nil || !strings.HasSuffix(out.String(), "__SKILLHUB_COMPLETE__\x00") {
		t.Fatal(err)
	}
	t.Setenv("DEMO_TOKEN", "different")
	out.Reset()
	if err = c.WriteEnvironment(ctx, []string{i.Target}, "nul", &out); err == nil || out.Len() != 0 {
		t.Fatal("conflict produced partial output")
	}
	t.Setenv("DEMO_TOKEN", secret)
	second := installConfigFixture(t, s, filepath.Join(t.TempDir(), "other"), "1.0.0")
	if err = c.SetSkillConfiguration(ctx, second.Target, "DEMO_TOKEN", "keychain", secret); err != nil {
		t.Fatal(err)
	}
	if err = c.SetSkillConfiguration(ctx, second.Target, "DEMO_REGION", "value", "test-region"); err != nil {
		t.Fatal(err)
	}
	if err = c.WriteEnvironment(ctx, []string{i.Target, second.Target}, "nul", &out); err == nil {
		t.Fatal("duplicate owners accepted")
	}
	instructions, err := c.EnvironmentInstructions(ctx, i.Target, "zsh")
	if err != nil || strings.Contains(instructions, secret) || !strings.Contains(instructions, "skillhub_env use") {
		t.Fatal("invalid value-free instructions", err)
	}
	if err = os.WriteFile(filepath.Join(i.Target, "SKILL.md"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err = c.WriteEnvironment(ctx, []string{i.Target}, "nul", &out); err == nil || out.Len() != 0 {
		t.Fatal("modified installation exported values")
	}
}

func TestEnvironmentReservedNames(t *testing.T) {
	for _, name := range []string{"PATH", "HOME", "BASH_ENV", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES", "NODE_OPTIONS", "SKILLHUB_HOME", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GIT_CONFIG_COUNT", "HTTPS_PROXY", "JAVA_TOOL_OPTIONS"} {
		if safeEnvironmentName(name) {
			t.Errorf("allowed runtime variable %s", name)
		}
	}
	for _, name := range []string{"SERVICE_TOKEN", "AWS_ACCESS_KEY_ID", "SERVICE_REGION"} {
		if !safeEnvironmentName(name) {
			t.Errorf("blocked service variable %s", name)
		}
	}
}
