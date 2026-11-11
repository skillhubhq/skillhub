package client

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/skillhubhq/skillhub/internal/packageformat"
)

type configCredentials struct {
	values      map[string]string
	unavailable bool
	mu          sync.Mutex
}

func (s *configCredentials) Get(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return "", errors.New("provider diagnostic must not escape")
	}
	v, ok := s.values[k]
	if !ok {
		return "", ErrCredentialMissing
	}
	return v, nil
}
func (s *configCredentials) Set(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return errors.New(v)
	}
	s.values[k] = v
	return nil
}
func (s *configCredentials) Delete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return ErrCredentialStore
	}
	delete(s.values, k)
	return nil
}

type configNoNetwork struct{ t *testing.T }

func (r configNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Error("local configuration used the network")
	return nil, errors.New("offline")
}

const configFixture = `{"version":1,"variables":[{"name":"DEMO_TOKEN","description":"Service credential","secret":true,"required":true},{"name":"DEMO_REGION","description":"Region","required":true}]}`

func installConfigFixture(t *testing.T, s *Store, target, version string) Installation {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: check\ndescription: configuration fixture\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SkillConfigFile), []byte(configFixture), 0600); err != nil {
		t.Fatal(err)
	}
	archive, bundle, err := packageformat.Pack(dir)
	if err != nil {
		t.Fatal(err)
	}
	i := record(target, bundle.Digest)
	i.Version = version
	plan, err := s.PlanInstall(context.Background(), i, archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(context.Background(), *plan, archive); err != nil {
		t.Fatal(err)
	}
	return *plan.After
}
func TestLocalConfigurationIsolationRedactionAndUpgrade(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	vault := &configCredentials{values: map[string]string{}}
	c := NewClient(s, ClientOptions{Home: t.TempDir(), Credentials: vault, HTTPClient: &http.Client{Transport: configNoNetwork{t}}})
	first := installConfigFixture(t, s, filepath.Join(t.TempDir(), "check"), "1.0.0")
	second := installConfigFixture(t, s, filepath.Join(t.TempDir(), "check"), "1.0.0")
	secret := "test-only-sensitive-config-value"
	if err := c.SetSkillConfiguration(ctx, first.Target, "DEMO_TOKEN", "value", secret); err == nil {
		t.Fatal("secret accepted as plaintext")
	}
	if err := c.SetSkillConfiguration(ctx, first.Target, "UNDECLARED", "keychain", secret); err == nil {
		t.Fatal("undeclared key accepted")
	}
	for _, v := range []struct{ name, mode, value string }{{"DEMO_TOKEN", "keychain", secret}, {"DEMO_REGION", "value", "test-region"}} {
		if err := c.SetSkillConfiguration(ctx, first.Target, v.name, v.mode, v.value); err != nil {
			t.Fatal(err)
		}
	}
	report, err := c.SkillConfiguration(ctx, first.Target)
	if err != nil || !report.Ready {
		t.Fatalf("configured: %+v %v", report, err)
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "test-region") || strings.Contains(string(raw), "skill-config:") {
		t.Fatal("diagnostic leaked values")
	}
	if err = filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			raw, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if strings.Contains(string(raw), secret) {
				t.Error("secret persisted on disk")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	other, err := c.SkillConfiguration(ctx, second.Target)
	if err != nil || other.Ready {
		t.Fatal("cross-project configuration shared")
	}
	// 新客户端复用同一系统凭据库，与 CLI/App 进程模型一致。
	reopened := NewClient(s, ClientOptions{Home: t.TempDir(), Credentials: vault, HTTPClient: &http.Client{Transport: configNoNetwork{t}}})
	if r, e := reopened.SkillConfiguration(ctx, first.Target); e != nil || !r.Ready {
		t.Fatal("configuration did not survive reopening", e)
	}
	installConfigFixture(t, s, first.Target, "1.0.1")
	report, err = c.SkillConfiguration(ctx, first.Target)
	if err != nil || !report.Changed || report.Ready {
		t.Fatal("upgrade reused authorization", err)
	}
	if err = c.SetSkillConfiguration(ctx, first.Target, "DEMO_REGION", "value", "new"); err == nil {
		t.Fatal("changed binding accepted")
	}
	if err = c.RenewSkillConfiguration(ctx, first.Target, "sha256:wrong"); err == nil {
		t.Fatal("accepted stale digest")
	}
	if err = c.RenewSkillConfiguration(ctx, first.Target, report.Installation.Digest); err != nil {
		t.Fatal(err)
	}
	if renewed, e := c.SkillConfiguration(ctx, first.Target); e != nil || !renewed.Ready || renewed.Changed {
		t.Fatal("renewal lost values", e)
	}
	if err = c.ClearSkillConfiguration(ctx, first.Target); err != nil {
		t.Fatal(err)
	}
	if len(vault.values) != 0 {
		t.Fatal("secret not removed")
	}
	t.Setenv("DEMO_TOKEN", secret)
	t.Setenv("DEMO_REGION", "test-region")
	for _, key := range []string{"DEMO_TOKEN", "DEMO_REGION"} {
		if err = c.SetSkillConfiguration(ctx, first.Target, key, "environment", ""); err != nil {
			t.Fatal(err)
		}
	}
	if r, e := c.SkillConfiguration(ctx, first.Target); e != nil || !r.Ready {
		t.Fatal("environment reference not available", e)
	}
	plan, err := s.PlanRemove(ctx, first.Target)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *plan, nil); err != nil {
		t.Fatal(err)
	}
	items, err := c.ConfigurationTargets(ctx)
	if err != nil || len(items) != 2 {
		t.Fatal("retained configuration disappeared", err)
	}
	if r, e := c.SkillConfiguration(ctx, first.Target); e != nil || r.Installed {
		t.Fatal("removed installation status", e)
	}
	if err = c.ClearSkillConfiguration(ctx, first.Target); err != nil {
		t.Fatal(err)
	}
}
func TestLocalConfigurationRejectsUnavailableStoreAndTampering(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	vault := &configCredentials{values: map[string]string{}, unavailable: true}
	c := NewClient(s, ClientOptions{Home: t.TempDir(), Credentials: vault})
	i := installConfigFixture(t, s, filepath.Join(t.TempDir(), "check"), "1.0.0")
	if err := c.SetSkillConfiguration(ctx, i.Target, "DEMO_TOKEN", "keychain", "private-fixture"); !errors.Is(err, ErrCredentialStore) || strings.Contains(err.Error(), "private-fixture") {
		t.Fatal("unsafe keychain failure", err)
	}
	vault.unavailable = false
	if err := c.SetSkillConfiguration(ctx, i.Target, "DEMO_TOKEN", "keychain", "private-fixture"); err != nil {
		t.Fatal(err)
	}
	vault.unavailable = true
	if r, e := c.SkillConfiguration(ctx, i.Target); e != nil || r.Variables[0].Status != "store-unavailable" {
		t.Fatal("unavailable store reported as success", e)
	}
	if err := c.ClearSkillConfiguration(ctx, i.Target); !errors.Is(err, ErrCredentialStore) {
		t.Fatal("failed removal lost metadata", err)
	}
	vault.unavailable = false
	if err := os.WriteFile(filepath.Join(i.Target, SkillConfigFile), []byte(`{"version":1,"variables":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if state, err := c.SkillConfiguration(ctx, i.Target); err != nil || !state.Changed || state.Ready {
		t.Fatal("modified declaration retained credential authorization", err)
	}
	if err := c.ClearSkillConfiguration(ctx, i.Target); err != nil {
		t.Fatal("cannot clear after file modification", err)
	}
}
func TestConfigDeclarationBounds(t *testing.T) {
	for _, raw := range []string{`{"version":2}`, `{"version":1,"command":"sh"}`, `{"version":1,"variables":[{"name":"TOKEN","default":"value"}]}`, `{"version":1,"variables":[{"name":"TOKEN"},{"name":"TOKEN"}]}`, `{"version":1,"variables":[{"name":"bad-name"}]}`, configFixture + ` {}`, strings.Repeat(" ", 33<<10)} {
		if _, err := ParseConfigDeclaration([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid declaration: %.80s", raw)
		}
	}
}

func TestLocalConfigurationUnsetPreservesOtherValues(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	vault := &configCredentials{values: map[string]string{}}
	c := NewClient(s, ClientOptions{Home: t.TempDir(), Credentials: vault})
	i := installConfigFixture(t, s, filepath.Join(t.TempDir(), "check"), "1.0.0")
	for _, v := range []struct{ name, mode, value string }{{"DEMO_TOKEN", "keychain", "fixture-secret"}, {"DEMO_REGION", "value", "region"}} {
		if err := c.SetSkillConfiguration(ctx, i.Target, v.name, v.mode, v.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.UnsetSkillConfiguration(ctx, i.Target, "DEMO_TOKEN"); err != nil {
		t.Fatal(err)
	}
	r, err := c.SkillConfiguration(ctx, i.Target)
	if err != nil || r.Ready || r.Variables[0].Status != "missing" || r.Variables[1].Status != "available" || len(vault.values) != 0 {
		t.Fatal("unset changed unrelated values", err)
	}
	if err := c.UnsetSkillConfiguration(ctx, i.Target, "DEMO_REGION"); err != nil {
		t.Fatal(err)
	}
	items, err := c.ConfigurationTargets(ctx)
	if err != nil || len(items) != 1 || items[0].Saved {
		t.Fatal("last value retained metadata", err)
	}
}
func TestLocalConfigurationRejectsSymlinkDirectory(t *testing.T) {
	s := testStore(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(s.root, "local-configuration")); err != nil {
		t.Skip("symlink unavailable")
	}
	c := NewClient(s, ClientOptions{})
	if _, err := c.ConfigurationTargets(context.Background()); err == nil {
		t.Fatal("accepted symlink directory")
	}
	files, err := os.ReadDir(outside)
	if err != nil || len(files) != 0 {
		t.Fatal("wrote outside local store", err)
	}
}

func TestConfigurationRenewalDoesNotCrossAccounts(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	vault := &configCredentials{values: map[string]string{}}
	core := NewClient(store, ClientOptions{Home: t.TempDir(), Credentials: vault})
	first := installConfigFixture(t, store, filepath.Join(t.TempDir(), "check"), "1.0.0")
	if e := core.SetSkillConfiguration(ctx, first.Target, "DEMO_TOKEN", "keychain", "fixture-secret"); e != nil {
		t.Fatal(e)
	}
	archive, _, e := packageformat.Pack(first.Target)
	if e != nil {
		t.Fatal(e)
	}
	first.SourceRef = "another-account"
	plan, e := store.PlanInstall(ctx, first, archive)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Apply(ctx, *plan, archive); e != nil {
		t.Fatal(e)
	}
	report, e := core.SkillConfiguration(ctx, first.Target)
	if e != nil || !report.Changed {
		t.Fatal("account change retained authorization", e)
	}
	if e = core.RenewSkillConfiguration(ctx, first.Target, first.Digest); e == nil {
		t.Fatal("credentials crossed accounts")
	}
	if len(vault.values) != 1 {
		t.Fatal("account change erased the original secret")
	}
}
