package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/hubhttp"
)

type credentialStore struct {
	mu          sync.Mutex
	values      map[string]string
	unavailable bool
}

func (s *credentialStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return "", client.ErrCredentialStore
	}
	v, ok := s.values[key]
	if !ok {
		return "", client.ErrCredentialMissing
	}
	return v, nil
}
func (s *credentialStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return client.ErrCredentialStore
	}
	s.values[key] = value
	return nil
}
func (s *credentialStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

func handler(t *testing.T) http.Handler {
	t.Helper()
	store, err := hub.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err = store.Bootstrap(context.Background(), "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	handler, err := hubhttp.New(store, hubhttp.Config{PublicURL: "http://127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func TestSourceIdentityChangeAndRedirectNeverForwardCredentials(t *testing.T) {
	ctx := context.Background()
	a, b := handler(t), handler(t)
	var active atomic.Value
	active.Store(a)
	var received atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Store(true); b.ServeHTTP(w, r) }))
	defer target.Close()
	var redirect atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirect.Load() {
			http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
			return
		}
		active.Load().(http.Handler).ServeHTTP(w, r)
	}))
	defer server.Close()
	store, err := client.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	core := client.NewClient(store, client.ClientOptions{Credentials: &credentialStore{values: map[string]string{}}})
	if _, err = core.AddSource(ctx, "company", server.URL); err != nil {
		t.Fatal(err)
	}
	if _, err = core.LoginPassword(ctx, "company", "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	remote, err := core.Remote(ctx, "company")
	if err != nil {
		t.Fatal(err)
	}
	redirect.Store(true)
	if _, err = remote.Auth.GetIdentity(ctx, connect.NewRequest(&authv1.GetIdentityRequest{})); err == nil {
		t.Fatal("followed credential-bearing redirect")
	}
	if received.Load() {
		t.Fatal("request reached redirect target")
	}
	redirect.Store(false)
	active.Store(b)
	if _, err = core.Remote(ctx, "company"); !errors.Is(err, client.ErrRegistryChanged) {
		t.Fatalf("changed registry accepted: %v", err)
	}
}

func TestAnonymousSourceWorksWithoutSystemCredentialService(t *testing.T) {
	server := httptest.NewServer(handler(t))
	defer server.Close()
	store, err := client.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	core := client.NewClient(store, client.ClientOptions{Credentials: &credentialStore{unavailable: true}})
	if _, err = core.AddSource(context.Background(), "public", server.URL); err != nil {
		t.Fatal(err)
	}
	results, err := core.Search(context.Background(), nil, &catalogv1.SearchSkillsRequest{})
	if err != nil || len(results) != 1 || results[0].Error != "" {
		t.Fatalf("public source required keyring: %v %v", results, err)
	}
	if _, err = core.LoginPassword(context.Background(), "public", "admin", "a-strong-test-password"); !errors.Is(err, client.ErrCredentialStore) {
		t.Fatalf("silently stored secret outside keyring: %v", err)
	}
	if err = core.RemoveSource(context.Background(), "public"); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterTargetsAndReservedNames(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	for _, tool := range []string{"codex", "claude-code"} {
		for _, scope := range []string{"global", "project"} {
			dir := ""
			if scope == "project" {
				dir = project
			}
			target, owner, err := client.ResolveTarget(tool, scope, dir, home, "safe-skill")
			if err != nil || target == "" || (scope == "project" && owner == "") {
				t.Fatalf("target %s %s: %s %s %v", tool, scope, target, owner, err)
			}
		}
	}
	for _, name := range []string{"synced", "anthropic-skills"} {
		if _, _, err := client.ResolveTarget("claude-code", "global", "", home, name); err == nil {
			t.Fatal("accepted reserved Claude skill name")
		}
	}
	if _, _, err := client.ResolveTarget("codex", "global", project, home, "safe-skill"); err == nil {
		t.Fatal("ignored contradictory scope")
	}
}

func TestMissingSavedCredentialDoesNotFallBackToAnonymous(t *testing.T) {
	server := httptest.NewServer(handler(t))
	defer server.Close()
	store, err := client.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	credentials := &credentialStore{values: map[string]string{}}
	core := client.NewClient(store, client.ClientOptions{Credentials: credentials})
	source, err := core.AddSource(context.Background(), "public", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = core.LoginPassword(context.Background(), "public", "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	if err = credentials.Delete(source.CredentialRef); err != nil {
		t.Fatal(err)
	}
	if _, err = core.Remote(context.Background(), "public"); !errors.Is(err, client.ErrCredentialMissing) {
		t.Fatalf("missing saved identity silently became anonymous: %v", err)
	}
}

func TestProjectInstallationRejectsExternalToolDirectories(t *testing.T) {
	for _, tool := range []struct{ name, directory string }{{"codex", ".agents"}, {"claude-code", ".claude"}} {
		t.Run(tool.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			if err := os.Symlink(outside, filepath.Join(root, tool.directory)); err != nil {
				t.Skip(err)
			}
			if target, _, err := client.ResolveTarget(tool.name, "project", root, "", "safe-skill"); err == nil {
				t.Fatalf("accepted target outside selected project: %s", target)
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatal("modified external directory")
			}
		})
	}
}

func TestProjectInstallationAcceptsInternalAndRootAliases(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "shared")
	if err := os.Mkdir(inside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(inside, filepath.Join(root, ".agents")); err != nil {
		t.Skip(err)
	}
	alias := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	target, project, err := client.ResolveTarget("codex", "project", alias, "", "safe-skill")
	canonical, e := filepath.EvalSymlinks(root)
	if err != nil || e != nil || project != canonical || target != filepath.Join(canonical, "shared", "skills", "safe-skill") {
		t.Fatalf("valid alias rejected: %s %s %v", target, project, err)
	}
}

func TestProjectInstallationAcceptsCaseAliasOfRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "MixedProject")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "mixedproject")
	if _, err := os.Stat(alias); err != nil {
		t.Skip("host filesystem is case-sensitive")
	}
	target, project, err := client.ResolveTarget("codex", "project", alias, "", "safe-skill")
	canonical, e := filepath.EvalSymlinks(root)
	if err != nil || e != nil || project != canonical || target != filepath.Join(canonical, ".agents", "skills", "safe-skill") {
		t.Fatalf("case alias rejected: %s %s %v", target, project, err)
	}
}
