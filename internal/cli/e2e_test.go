package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/hubhttp"
)

type memoryCredentials struct {
	mu     sync.Mutex
	values map[string]string
}

func (m *memoryCredentials) Get(ref string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[ref]
	if !ok {
		return "", client.ErrCredentialMissing
	}
	return value, nil
}
func (m *memoryCredentials) Set(ref, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[ref] = value
	return nil
}
func (m *memoryCredentials) Delete(ref string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, ref)
	return nil
}

func cliHub(t *testing.T, private bool) *httptest.Server {
	t.Helper()
	store, err := hub.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err = store.Bootstrap(context.Background(), "admin", "test-password-strong"); err != nil {
		t.Fatal(err)
	}
	handler, err := hubhttp.New(store, hubhttp.Config{PublicURL: "http://127.0.0.1", Private: private})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestTwoHubCLIReviewInstallationAndPartialFailure(t *testing.T) {
	public := cliHub(t, false)
	company := cliHub(t, true)
	state := t.TempDir()
	home := t.TempDir()
	project := t.TempDir()
	credentials := &memoryCredentials{values: map[string]string{}}
	options := Options{Client: client.ClientOptions{Credentials: credentials, Home: home}}
	run := func(input string, args ...string) ([]byte, error) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		command := append([]string{"--data", state, "--json"}, args...)
		err := Execute(context.Background(), command, strings.NewReader(input), &stdout, &stderr, options)
		if bytes.Contains(stdout.Bytes(), []byte("test-password-strong")) {
			t.Fatal("password leaked to output")
		}
		return stdout.Bytes(), err
	}
	ok := func(input string, args ...string) []byte {
		t.Helper()
		out, err := run(input, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !json.Valid(out) {
			t.Fatalf("invalid JSON output for %v: %s", args, out)
		}
		return out
	}
	for alias, url := range map[string]string{"public": public.URL, "company": company.URL} {
		ok("", "source", "add", alias, url)
		ok("test-password-strong\n", "login", alias, "--username", "admin", "--password-stdin")
	}
	if len(credentials.values) != 2 {
		t.Fatal("sources did not receive independent credentials")
	}
	ok("", "admin", "namespace-put", "team", "--source", "public", "--review-required=false")
	ok("", "admin", "namespace-put", "team", "--source", "company")
	created := ok("reviewer-password-strong\n", "admin", "user-add", "reviewer", "--source", "company", "--password-stdin")
	var createdUser struct {
		User struct {
			ID string `json:"user_id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(created, &createdUser); err != nil {
		t.Fatal(err)
	}
	ok("", "admin", "grant", "team", createdUser.User.ID, "--source", "company", "--role", "reviewer")
	source := t.TempDir()
	original := []byte("---\nname: shared-check\ndescription: Verify two independent sources.\n---\nOriginal instructions.\n")
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), original, 0600); err != nil {
		t.Fatal(err)
	}
	ok("", "publish", source, "--source", "public", "--namespace", "team", "--version", "1.0.0", "--visibility", "public")
	submitted := ok("", "publish", source, "--source", "company", "--namespace", "team", "--version", "1.0.0")
	var submission struct {
		Release struct {
			ID     string `json:"release_id"`
			Status string `json:"status"`
		} `json:"release"`
	}
	if err := json.Unmarshal(submitted, &submission); err != nil {
		t.Fatal(err)
	}
	if submission.Release.Status != "RELEASE_STATUS_PENDING" {
		t.Fatal("CLI mistook submission for publication")
	}
	if _, err := run("", "review", "approve", submission.Release.ID, "--source", "company"); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("self review: %v", err)
	}
	ok("reviewer-password-strong\n", "login", "company", "--username", "reviewer", "--password-stdin")
	ok("", "review", "inspect", submission.Release.ID, "--source", "company")
	ok("", "review", "approve", submission.Release.ID, "--source", "company", "--reason", "Reviewed immutable package")
	results := ok("", "search", "shared-check")
	var search []client.SearchResult
	if err := json.Unmarshal(results, &search); err != nil {
		t.Fatal(err)
	}
	if len(search) != 2 || len(search[0].Skills) != 1 || len(search[1].Skills) != 1 {
		t.Fatal("multi-source search lost independent skills")
	}
	if _, err := run("", "install", "company/team/shared-check@1.0.0", "--tool", "codex", "--project", project); !errors.Is(err, ErrPlanOnly) {
		t.Fatalf("missing concrete install plan: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "shared-check")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unconfirmed plan wrote files")
	}
	ok("", "install", "company/team/shared-check@1.0.0", "--tool", "codex", "--scope", "project", "--project", project, "--yes")
	if _, err := run("", "install", "public/team/shared-check@1.0.0", "--tool", "codex", "--scope", "project", "--project", project, "--yes"); !errors.Is(err, client.ErrSourceConflict) {
		t.Fatalf("same-name source replacement: %v", err)
	}
	ok("", "install", "public/team/shared-check@1.0.0", "--tool", "claude-code", "--scope", "global", "--yes")
	var installs []client.Installation
	if err := json.Unmarshal(ok("", "list"), &installs); err != nil {
		t.Fatal(err)
	}
	if len(installs) != 2 {
		t.Fatal("missing independent installation instances")
	}
	codex := filepath.Join(project, ".agents", "skills", "shared-check")
	if err := os.WriteFile(filepath.Join(codex, "SKILL.md"), []byte("local edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := run("", "remove", codex, "--yes"); !errors.Is(err, client.ErrModified) {
		t.Fatalf("local edit discarded: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codex, "SKILL.md"), original, 0600); err != nil {
		t.Fatal(err)
	}
	ok("", "remove", codex, "--yes")
	claude := filepath.Join(home, ".claude", "skills", "shared-check")
	if data, err := os.ReadFile(filepath.Join(claude, "SKILL.md")); err != nil || !bytes.Equal(data, original) {
		t.Fatal("global Claude installation missing")
	}
	updated := append(append([]byte{}, original...), []byte("Updated instructions.\n")...)
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), updated, 0600); err != nil {
		t.Fatal(err)
	}
	ok("", "publish", source, "--source", "public", "--namespace", "team", "--version", "1.1.0", "--visibility", "public")
	// 最近发布的旧分支补丁和预发行版本不应让默认更新选错稳定版本。
	ok("", "publish", source, "--source", "public", "--namespace", "team", "--version", "2.0.0-rc.1", "--visibility", "public")
	ok("", "publish", source, "--source", "public", "--namespace", "team", "--version", "1.0.1", "--visibility", "public")
	var checks []client.UpdateCheck
	if err := json.Unmarshal(ok("", "updates", claude), &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].AvailableVersion != "1.1.0" {
		t.Fatalf("selected wrong stable update: %+v", checks)
	}
	ok("", "update", claude, "--dry-run")
	if data, err := os.ReadFile(filepath.Join(claude, "SKILL.md")); err != nil || !bytes.Equal(data, original) {
		t.Fatal("update dry run wrote files")
	}
	ok("", "update", claude, "--yes")
	if out := ok("", "update", claude, "--yes"); !bytes.Contains(out, []byte("up_to_date")) {
		t.Fatal("repeated update did not report up to date")
	}
	if data, err := os.ReadFile(filepath.Join(claude, "SKILL.md")); err != nil || !bytes.Equal(data, updated) {
		t.Fatal("CLI upgrade did not replace the managed version")
	}
	ok("", "update", claude, "--version", "1.0.0", "--yes")
	if data, err := os.ReadFile(filepath.Join(claude, "SKILL.md")); err != nil || !bytes.Equal(data, original) {
		t.Fatal("explicit version selection failed")
	}
	if err := os.WriteFile(filepath.Join(claude, "SKILL.md"), []byte("local edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := run("", "update", claude, "--yes"); !errors.Is(err, client.ErrModified) {
		t.Fatalf("update discarded local changes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claude, "SKILL.md"), original, 0600); err != nil {
		t.Fatal(err)
	}
	ok("", "update", claude, "--yes")
	ok("", "install", "public/team/shared-check@1.1.0", "--tool", "codex", "--scope", "global", "--yes")
	ok("", "install", "public/team/shared-check@1.1.0", "--tool", "claude-code", "--scope", "project", "--project", project, "--yes")
	for _, path := range []string{filepath.Join(home, ".agents", "skills", "shared-check", "SKILL.md"), filepath.Join(project, ".claude", "skills", "shared-check", "SKILL.md")} {
		if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, updated) {
			t.Fatalf("tool scope installation missing at %s", path)
		}
	}
	ok("", "logout", "company")
	partial, err := run("", "search", "shared-check")
	if !errors.Is(err, ErrPartial) {
		t.Fatalf("private source failure not reported: %v", err)
	}
	if err = json.Unmarshal(partial, &search); err != nil {
		t.Fatal(err)
	}
	if len(search) != 2 || search[0].Source != "company" || search[0].Error == "" || search[1].Source != "public" || len(search[1].Skills) != 1 {
		t.Fatalf("partial results lost: %s", partial)
	}
	ok("", "source", "remove", "company")
	if len(credentials.values) != 1 {
		t.Fatal("source logout affected another credential")
	}
}
