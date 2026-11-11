package client

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/provenance"
)

func TestAttributedInstallationCanUpdateAndRemove(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	archive, digest := testArchive(t, "first")
	desired := record(filepath.Join(t.TempDir(), "check"), digest)
	desired.Provenance = &provenance.Record{AuthorName: "Example author", SourceURL: "https://example.com/skills", License: "Apache-2.0"}
	plan, err := s.PlanInstall(ctx, desired, archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *plan, archive); err != nil {
		t.Fatal(err)
	}
	next, digest := testArchive(t, "updated")
	desired.Digest = digest
	desired.Version = "2.0.0"
	plan, err = s.PlanInstall(ctx, desired, next)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *plan, next); err != nil {
		t.Fatalf("update attributed installation: %v", err)
	}
	plan, err = s.PlanRemove(ctx, desired.Target)
	if err != nil {
		t.Fatal(err)
	}
	plan.Before.Provenance.License = "MIT"
	if err = s.Apply(ctx, *plan, nil); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("changed attribution must invalidate plan: %v", err)
	}
	plan, err = s.PlanRemove(ctx, desired.Target)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *plan, nil); err != nil {
		t.Fatalf("remove attributed installation: %v", err)
	}
	if _, err = os.Stat(desired.Target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("installation remains: %v", err)
	}
}

func TestExecutableArchiveCanBeUpdatedOnHostPlatform(t *testing.T) {
	var raw bytes.Buffer
	writer := zip.NewWriter(&raw)
	for name, content := range map[string]string{"SKILL.md": "---\nname: check\ndescription: test\n---\n", "scripts/run.sh": "echo test\n"} {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0755)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s := testStore(t)
	desired := record(filepath.Join(t.TempDir(), "check"), packageformat.Digest(raw.Bytes()))
	plan, err := s.PlanInstall(ctx, desired, raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *plan, raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PlanRemove(ctx, desired.Target); err != nil {
		t.Fatalf("cross-platform fingerprint: %v", err)
	}
}

func TestPhysicalPathAliasesShareOneOwner(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	data, digest := testArchive(t, "first")
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skip(err)
	}
	p, err := s.PlanInstall(ctx, record(filepath.Join(real, "check"), digest), data)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *p, data); err != nil {
		t.Fatal(err)
	}
	q, err := s.PlanInstall(ctx, record(filepath.Join(alias, "check"), digest), data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Target != q.Target || q.Action != "update" || q.After.ID != p.After.ID {
		t.Fatal("physical alias created another owner")
	}
	if _, err = os.Stat(filepath.Join(real, "CHECK")); err == nil {
		q, err = s.PlanInstall(ctx, record(filepath.Join(real, "CHECK"), digest), data)
		if err != nil {
			t.Fatal(err)
		}
		if p.Target != q.Target || q.After.ID != p.After.ID {
			t.Fatal("case alias created another owner")
		}
	}
}

func TestRecoveryStopsIfBackupWasModified(t *testing.T) {
	for _, stage := range []string{"old-moved", "committed"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			old, digest := testArchive(t, "old")
			target := filepath.Join(t.TempDir(), "check")
			p, err := s.PlanInstall(ctx, record(target, digest), old)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Apply(ctx, *p, old); err != nil {
				t.Fatal(err)
			}
			next, digest := testArchive(t, "new")
			p, err = s.PlanInstall(ctx, record(target, digest), next)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.apply(ctx, *p, next, func(at string) error {
				if at == stage {
					return errors.New("interrupted")
				}
				return nil
			}); err == nil {
				t.Fatal("missing injected failure")
			}
			paths, err := filepath.Glob(filepath.Join(filepath.Dir(p.Target), ".skillhub-txn-*", "old", "SKILL.md"))
			if err != nil || len(paths) != 1 {
				t.Fatal("missing backup")
			}
			if err = os.WriteFile(paths[0], []byte("local recovery edit"), 0600); err != nil {
				t.Fatal(err)
			}
			if err = s.Recover(ctx); err == nil {
				t.Fatal("destroyed modified backup")
			}
			if content, err := os.ReadFile(paths[0]); err != nil || string(content) != "local recovery edit" {
				t.Fatal("backup not preserved")
			}
		})
	}
}

func testArchive(t *testing.T, body string) ([]byte, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: check\ndescription: Check a release.\n---\n"+body), 0600); err != nil {
		t.Fatal(err)
	}
	data, bundle, err := packageformat.Pack(dir)
	if err != nil {
		t.Fatal(err)
	}
	return data, bundle.Digest
}

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func record(target, digest string) Installation {
	return Installation{RegistryID: "registry-one", RegistryURL: "https://one.example", Namespace: "team", Name: "check", Version: "1.0.0", Digest: digest, Tool: "codex", Scope: "project", Target: target}
}

func TestInstallConflictUpdateAndRemove(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	data, digest := testArchive(t, "first")
	target := filepath.Join(t.TempDir(), "check")
	plan, err := s.PlanInstall(ctx, record(target, digest), data)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *plan, data); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListInstallations(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("inventory: %v %v", list, err)
	}
	other := record(target, digest)
	other.RegistryID = "another-registry"
	if _, err = s.PlanInstall(ctx, other, data); !errors.Is(err, ErrSourceConflict) {
		t.Fatalf("source conflict: %v", err)
	}
	if err = os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PlanInstall(ctx, record(target, digest), data); !errors.Is(err, ErrModified) {
		t.Fatalf("modified: %v", err)
	}
	if _, err = s.PlanRemove(ctx, target); !errors.Is(err, ErrModified) {
		t.Fatalf("remove modified: %v", err)
	}
	if err = os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	bundle, err := packageformat.Inspect(data)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err = bundle.Extract(target); err != nil {
		t.Fatal(err)
	}
	data2, digest2 := testArchive(t, "second")
	next := record(target, digest2)
	next.Version = "1.1.0"
	plan, err = s.PlanInstall(ctx, next, data2)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *plan, data2); err != nil {
		t.Fatal(err)
	}
	remove, err := s.PlanRemove(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *remove, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("target retained")
	}
	list, err = s.ListInstallations(ctx)
	if err != nil || len(list) != 0 {
		t.Fatal("inventory retained")
	}
}

func TestUnmanagedAndStalePlan(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	data, digest := testArchive(t, "first")
	target := t.TempDir()
	marker := filepath.Join(target, "notes.txt")
	if err := os.WriteFile(marker, []byte("keep my files"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlanInstall(ctx, record(target, digest), data); !errors.Is(err, ErrUnmanaged) || !strings.Contains(err.Error(), target) {
		t.Fatalf("unmanaged: %v", err)
	}
	if content, err := os.ReadFile(marker); err != nil || string(content) != "keep my files" {
		t.Fatal("unmanaged files changed", err)
	}
	target = filepath.Join(target, "managed")
	a, err := s.PlanInstall(ctx, record(target, digest), data)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.PlanInstall(ctx, record(target, digest), data)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *a, data); err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *b, data); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale: %v", err)
	}
}

func TestRecoveryAtEachFilesystemBoundary(t *testing.T) {
	for _, point := range []string{"prepared", "old-moved", "new-moved", "committed"} {
		t.Run(point, func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			target := filepath.Join(t.TempDir(), "check")
			old, digest := testArchive(t, "old")
			p, err := s.PlanInstall(ctx, record(target, digest), old)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Apply(ctx, *p, old); err != nil {
				t.Fatal(err)
			}
			next, digest2 := testArchive(t, "new")
			desired := record(target, digest2)
			desired.Version = "2.0.0"
			p, err = s.PlanInstall(ctx, desired, next)
			if err != nil {
				t.Fatal(err)
			}
			err = s.apply(ctx, *p, next, func(at string) error {
				if at == point {
					return errors.New("simulated process exit")
				}
				return nil
			})
			if err == nil {
				t.Fatal("did not inject failure")
			}
			if err = s.Recover(ctx); err != nil {
				t.Fatal(err)
			}
			installed, err := s.checkedInstallation(ctx, p.Target)
			if err != nil {
				t.Fatal(err)
			}
			want := "1.0.0"
			if point == "committed" {
				want = "2.0.0"
			}
			if installed.Version != want {
				t.Fatalf("got %s want %s", installed.Version, want)
			}
		})
	}
}

func TestTwoClientInstancesSerializePhysicalTarget(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	a, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	data, digest := testArchive(t, "same")
	target := filepath.Join(t.TempDir(), "check")
	p, err := a.PlanInstall(ctx, record(target, digest), data)
	if err != nil {
		t.Fatal(err)
	}
	q, err := b.PlanInstall(ctx, record(target, digest), data)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, s := range []*Store{a, b} {
		wg.Add(1)
		plan := []*InstallPlan{p, q}[i]
		go func() { defer wg.Done(); errs <- s.Apply(ctx, *plan, data) }()
	}
	wg.Wait()
	close(errs)
	success, stale := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, ErrStalePlan) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("success=%d stale=%d", success, stale)
	}
}
