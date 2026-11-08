package client

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestInstallationConnectionIsRetainedAndNeverGuessed(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	core := NewClient(store, ClientOptions{})
	source := Source{Alias: "author", RegistryID: "registry-one", URL: "https://one.example", CredentialRef: "original"}
	put := func(s Source) {
		t.Helper()
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.db.Exec("INSERT OR REPLACE INTO sources(alias,registry_id,record) VALUES(?,?,?)", s.Alias, s.RegistryID, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	put(source)
	data, digest := testArchive(t, "first")
	desired := record(filepath.Join(t.TempDir(), "check"), digest)
	plan, err := store.PlanInstall(ctx, desired, data)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Apply(ctx, *plan, data); err != nil {
		t.Fatal(err)
	}
	unbound := *plan.After
	if got, err := core.installationSource(ctx, unbound, ""); err != nil || got.Alias != "author" {
		t.Fatalf("unique unbound source: %+v %v", got, err)
	}
	alternate := source
	alternate.Alias, alternate.CredentialRef = "reviewer", "alternate"
	put(alternate)
	if _, err := core.installationSource(ctx, unbound, ""); !errors.Is(err, ErrUpdateConnection) {
		t.Fatalf("ambiguous unbound source: %v", err)
	}
	saved, err := store.rememberInstallationSource(ctx, unbound, source)
	if err != nil {
		t.Fatal(err)
	}
	// 重新读取持久记录；不是依赖 UI 内存中的临时选择。
	persisted, err := store.installation(ctx, saved.Target)
	if err != nil || persisted == nil || *persisted != saved {
		t.Fatalf("source not persisted: %+v %v", persisted, err)
	}
	if got, err := core.installationSource(ctx, *persisted, ""); err != nil || got.Alias != "author" {
		t.Fatalf("remembered source: %+v %v", got, err)
	}
	if _, err = store.rememberInstallationSource(ctx, unbound, alternate); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale metadata overwrote source: %v", err)
	}
	if _, err = store.PlanRemove(ctx, saved.Target); err != nil {
		t.Fatalf("metadata update changed installed files: %v", err)
	}
	if _, err = store.db.Exec("DELETE FROM sources WHERE alias=?", source.Alias); err != nil {
		t.Fatal(err)
	}
	if _, err = core.installationSource(ctx, saved, ""); !errors.Is(err, ErrUpdateConnection) {
		t.Fatalf("removed source fell back: %v", err)
	}
	source.CredentialRef = "replacement"
	put(source)
	if _, err = core.installationSource(ctx, saved, ""); !errors.Is(err, ErrUpdateConnection) {
		t.Fatalf("recreated alias silently reused: %v", err)
	}
	if got, err := core.installationSource(ctx, saved, "reviewer"); err != nil || got.Alias != "reviewer" {
		t.Fatalf("explicit recovery: %+v %v", got, err)
	}
}
