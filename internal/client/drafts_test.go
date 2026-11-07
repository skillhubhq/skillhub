package client

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skillhubhq/skillhub/internal/packageformat"
)

func TestDraftCopyEditIsolationAndConflicts(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c := NewClient(store, ClientOptions{})
	ctx := context.Background()
	draft, err := c.CreateDraft(ctx, "", PreviewTarget{})
	if err != nil {
		t.Fatal(err)
	}
	original, err := store.ReadDraftFile(draft.ID, "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	copy, err := c.CreateDraft(ctx, draft.Directory, PreviewTarget{})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(original.Text, "my-skill", "edited-skill", 1)
	updated, err := store.SaveDraftFile(ctx, copy.ID, "SKILL.md", original.Digest, text)
	if err != nil || updated.Text != text {
		t.Fatalf("save: %v", err)
	}
	unchanged, _ := store.ReadDraftFile(draft.ID, "SKILL.md")
	if unchanged.Digest != original.Digest {
		t.Fatal("edited source")
	}
	if _, err = store.SaveDraftFile(ctx, copy.ID, "SKILL.md", original.Digest, "stale"); err == nil {
		t.Fatal("stale edit overwrote changes")
	}
	_, bundle, err := packageformat.Pack(copy.Directory)
	if err != nil || bundle.Metadata.Name != "edited-skill" {
		t.Fatalf("draft cannot be published: %v", err)
	}
	if _, err = c.CreateDraft(ctx, draft.Directory, PreviewTarget{Source: "other"}); err == nil {
		t.Fatal("ambiguous source")
	}
}
func TestDraftRejectsUnsafeAndNonTextFiles(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c := NewClient(store, ClientOptions{})
	ctx := context.Background()
	d, err := c.CreateDraft(ctx, "", PreviewTarget{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../SKILL.md", "/etc/passwd", "a/../SKILL.md", "a\\b", "C:secret", "missing"} {
		if _, err = store.ReadDraftFile(d.ID, name); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	if _, err = store.ReadDraftFile("../outside", "SKILL.md"); err == nil {
		t.Fatal("accepted invalid draft")
	}
	for name, data := range map[string][]byte{"binary": {0, 255}, "large": []byte(strings.Repeat("x", maxDraftText+1))} {
		if err = os.WriteFile(filepath.Join(d.Directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = store.ReadDraftFile(d.ID, name); err == nil {
			t.Fatal("accepted nontext/oversize")
		}
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err = os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, filepath.Join(d.Directory, "link")); err == nil {
		if _, err = store.ReadDraftFile(d.ID, "link"); err == nil {
			t.Fatal("followed external symlink")
		}
		if _, err = store.SaveDraftFile(ctx, d.ID, "link", "invalid", "overwrite"); err == nil {
			t.Fatal("wrote symlink")
		}
	}
	file, _ := store.ReadDraftFile(d.ID, "SKILL.md")
	for _, content := range []string{"\x00", strings.Repeat("x", maxDraftText+1)} {
		if _, err = store.SaveDraftFile(ctx, d.ID, "SKILL.md", file.Digest, content); err == nil {
			t.Fatal("accepted invalid save")
		}
	}
}

func TestDraftsRemainDiscoverableAfterRestartIncludingIncompleteContent(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	core := NewClient(store, ClientOptions{})
	draft, err := core.CreateDraft(context.Background(), "", PreviewTarget{})
	if err != nil {
		t.Fatal(err)
	}
	incomplete, err := core.CreateDraft(context.Background(), "", PreviewTarget{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := store.ReadDraftFile(incomplete.ID, "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveDraftFile(context.Background(), incomplete.ID, "SKILL.md", f.Digest, "unfinished draft"); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for directory, modified := range map[string]time.Time{draft.Directory: older, incomplete.Directory: older.Add(500 * time.Millisecond)} {
		if err := os.Chtimes(directory, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := store.ListDrafts()
	if err != nil || len(entries) != 2 {
		t.Fatalf("lost drafts: %v %v", entries, err)
	}
	if entries[0].ID != incomplete.ID {
		t.Fatal("draft list is not ordered by most recent change")
	}
	for _, id := range []string{draft.ID, incomplete.ID} {
		work, err := store.OpenDraft(id)
		if err != nil || len(work.Files) != 1 || work.Files[0] != "SKILL.md" {
			t.Fatalf("cannot reopen saved draft: %v", err)
		}
	}
	if _, err = store.OpenDraft("../outside"); err == nil {
		t.Fatal("accepted invalid draft identifier")
	}
	f, err = store.ReadDraftFile(incomplete.ID, "SKILL.md")
	if err != nil || f.Text != "unfinished draft" {
		t.Fatal("modified incomplete content")
	}
	if err = os.Mkdir(filepath.Join(root, "drafts", "not-a-draft"), 0700); err != nil {
		t.Fatal(err)
	}
	entries, err = store.ListDrafts()
	if err != nil || len(entries) != 2 {
		t.Fatal("listed unrelated directory")
	}
	if err = os.Symlink(t.TempDir(), filepath.Join(draft.Directory, "outside")); err == nil {
		if _, err = store.OpenDraft(draft.ID); err == nil {
			t.Fatal("reopened a draft containing a symlink")
		}
	}
}
