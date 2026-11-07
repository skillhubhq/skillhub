package client

import (
	"context"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
