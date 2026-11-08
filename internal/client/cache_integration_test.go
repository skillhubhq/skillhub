package client_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"google.golang.org/protobuf/proto"
)

type observedCacheHub struct {
	server           *httptest.Server
	known            atomic.Value
	cacheUnsupported atomic.Bool
}

func cacheHub(t *testing.T) *observedCacheHub {
	t.Helper()
	f := &observedCacheHub{}
	f.known.Store("")
	backend := handler(t)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api.catalog.v1.CatalogService/DownloadRelease" {
			body, err := io.ReadAll(r.Body)
			_ = r.Body.Close()
			var request catalogv1.DownloadReleaseRequest
			if err != nil || proto.Unmarshal(body, &request) != nil {
				http.Error(w, "invalid fixture request", 400)
				return
			}
			f.known.Store(request.KnownDigest)
			if f.cacheUnsupported.Load() {
				request.KnownDigest = ""
				body, err = proto.Marshal(&request)
				if err != nil {
					http.Error(w, "invalid fixture serialization", 500)
					return
				}
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
		backend.ServeHTTP(w, r)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func TestSharedCacheUsesConditionalDownloadsWithoutGrantingOfflineOrRevokedAccess(t *testing.T) {
	ctx := context.Background()
	core, store := newProjectClient(t)
	a, b := cacheHub(t), cacheHub(t)
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: cache-check\ndescription: Exercise authenticated cached downloads.\n---\nExact content.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	archive, _, err := packageformat.Pack(directory)
	if err != nil {
		t.Fatal(err)
	}
	releaseID := ""
	for alias, hub := range map[string]*observedCacheHub{"cache-a": a, "cache-b": b} {
		if _, err = core.AddSource(ctx, alias, hub.server.URL); err != nil {
			t.Fatal(err)
		}
		if _, err = core.LoginPassword(ctx, alias, "admin", "a-strong-test-password"); err != nil {
			t.Fatal(err)
		}
		remote, err := core.Remote(ctx, alias)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = remote.Admin.PutNamespace(ctx, connect.NewRequest(&adminv1.PutNamespaceRequest{Mutation: projectMutation(), Name: "team"})); err != nil {
			t.Fatal(err)
		}
		submitted, err := remote.Publishing.SubmitRelease(ctx, connect.NewRequest(&publishingv1.SubmitReleaseRequest{Mutation: projectMutation(), Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}))
		if err != nil {
			t.Fatal(err)
		}
		if alias == "cache-a" {
			releaseID = submitted.Msg.Release.ReleaseId
		}
	}
	coordinate := client.Coordinate{Source: "cache-a", Namespace: "team", Name: "cache-check", Version: "1.0.0"}
	plan := func() []byte {
		t.Helper()
		_, data, err := core.PlanRemoteInstall(ctx, coordinate, "codex", "project", t.TempDir())
		if err != nil || !bytes.Equal(data, archive) {
			t.Fatalf("exact cached download failed: %v", err)
		}
		return data
	}
	plan()
	if a.known.Load().(string) != "" {
		t.Fatal("first download sent a cached digest")
	}
	plan()
	if a.known.Load().(string) != packageformat.Digest(archive) {
		t.Fatal("second download did not use verified cached bytes")
	}
	a.cacheUnsupported.Store(true)
	plan()
	a.cacheUnsupported.Store(false)
	info, err := store.CacheInfo(ctx)
	if err != nil || info.Entries != 1 {
		t.Fatal("cache was not shared across requests")
	}
	entries, err := os.ReadDir(info.Directory)
	if err != nil || len(entries) != 1 {
		t.Fatal("unexpected cache files")
	}
	if err = os.WriteFile(filepath.Join(info.Directory, entries[0].Name()), []byte("modified local cache"), 0600); err != nil {
		t.Fatal(err)
	}
	plan()
	if cached, err := os.ReadFile(filepath.Join(info.Directory, entries[0].Name())); err != nil || !bytes.Equal(cached, archive) {
		t.Fatal("corrupt cache did not recover through a fresh authorized download")
	}
	coordinate.Source = "cache-b"
	plan()
	if b.known.Load().(string) != "" {
		t.Fatal("another Hub received the first Hub's cached digest")
	}
	coordinate.Source = "cache-a"
	if err = core.Logout(ctx, "cache-a"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = core.PlanRemoteInstall(ctx, coordinate, "codex", "project", t.TempDir()); err == nil {
		t.Fatal("cached bytes bypassed logout")
	}
	if _, err = core.LoginPassword(ctx, "cache-a", "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	remote, err := core.Remote(ctx, "cache-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = remote.Publishing.YankRelease(ctx, connect.NewRequest(&publishingv1.YankReleaseRequest{Mutation: projectMutation(), ReleaseId: releaseID, Reason: "withdraw cached version"})); err != nil {
		t.Fatal(err)
	}
	if _, _, err = core.PlanRemoteInstall(ctx, coordinate, "codex", "project", t.TempDir()); err == nil {
		t.Fatal("cached bytes bypassed withdrawal")
	}
	coordinate.Source = "cache-b"
	b.server.Close()
	if _, _, err = core.PlanRemoteInstall(ctx, coordinate, "codex", "project", t.TempDir()); err == nil {
		t.Fatal("cached bytes bypassed offline authorization")
	}
}
