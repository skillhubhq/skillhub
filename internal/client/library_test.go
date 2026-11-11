package client

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/skillhubhq/skillhub/internal/packageformat"
)

func localFixture(t *testing.T) (*Client, *Store, string) {
	t.Helper()
	s := testStore(t)
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(s, ClientOptions{Home: home, Credentials: &configCredentials{values: map[string]string{}}, HTTPClient: &http.Client{Transport: configNoNetwork{t}}})
	return c, s, home
}
func writeLocal(t *testing.T, target, body string) string {
	t.Helper()
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	text := "---\nname: check\ndescription: Local test skill\n---\n" + body
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return text
}
func TestLocalDiscoveryDeduplicatesLinksWithoutClaimingOwnership(t *testing.T) {
	ctx := context.Background()
	c, _, home := localFixture(t)
	target := filepath.Join(home, ".agents/skills/check")
	writeLocal(t, target, "Original")
	link := filepath.Join(home, ".claude/skills/check")
	if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	project := t.TempDir()
	if _, err := c.InitProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	copy := filepath.Join(project, ".agents/skills/check")
	writeLocal(t, copy, "Project")
	inv, err := c.LocalInventory(ctx)
	if err != nil || len(inv.Skills) != 2 {
		t.Fatalf("inventory: %+v %v", inv, err)
	}
	var linked LocalSkill
	for _, v := range inv.Skills {
		if v.Target == target {
			linked = v
		}
	}
	if len(linked.Locations) != 2 || linked.Installation != nil || linked.Association != nil || linked.State != "local" {
		t.Fatalf("unexpected takeover: %+v", linked)
	}
	if err = c.SetLocalHidden(ctx, target, true); err != nil {
		t.Fatal(err)
	}
	item, err := c.LocalSkill(ctx, link)
	if err != nil || !item.Hidden {
		t.Fatalf("hide did not resolve shared target: %v", err)
	}
	root := t.TempDir()
	writeLocal(t, filepath.Join(root, "check"), "Extra")
	if err = c.SetLocalRoot(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	inv, err = c.LocalInventory(ctx)
	if err != nil || len(inv.Skills) != 3 {
		t.Fatalf("extra folder: %+v %v", inv, err)
	}
	if err = c.SetLocalRoot(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "check/SKILL.md")); err != nil {
		t.Fatal("stop scanning deleted files", err)
	}
}
func TestLocalEditBackupRestoreAndStalePreview(t *testing.T) {
	ctx := context.Background()
	c, s, home := localFixture(t)
	target := filepath.Join(home, ".agents/skills/check")
	original := writeLocal(t, target, "Original")
	doc, err := c.ReadLocalSkill(ctx, target, "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := c.PlanLocalEdit(ctx, target, "SKILL.md", doc.Fingerprint, original+"\nEdited")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ApplyLocalPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(target, "SKILL.md"))
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatal("edit changed file permissions", err)
	}
	if err = c.ApplyLocalPlan(ctx, plan); !errors.Is(err, ErrStalePlan) {
		t.Fatal("replayed stale edit", err)
	}
	backups, err := s.LocalBackups(ctx)
	if err != nil || len(backups) != 1 {
		t.Fatalf("backup missing: %v %v", backups, err)
	}
	restore, err := c.PlanLocalRestore(ctx, backups[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ApplyLocalPlan(ctx, restore); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if err != nil || string(raw) != original {
		t.Fatal("restore did not recover original", err)
	}
	remove, err := c.PlanLocalRemove(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ApplyLocalPlan(ctx, remove); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("uninstall left target", err)
	}
	backups, _ = s.LocalBackups(ctx)
	var removed LocalBackup
	for _, b := range backups {
		if b.Action == "remove" {
			removed = b
		}
	}
	restore, err = c.PlanLocalRestore(ctx, removed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ApplyLocalPlan(ctx, restore); err != nil {
		t.Fatal(err)
	}
	if raw, err = os.ReadFile(filepath.Join(target, "SKILL.md")); err != nil || string(raw) != original {
		t.Fatal("uninstall recovery", err)
	}
}
func TestLocalCrashRecoveryPreservesOriginalOrCommittedEdit(t *testing.T) {
	for _, stage := range []string{"prepared", "old-moved", "new-moved", "committed"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			c, s, home := localFixture(t)
			target := filepath.Join(home, ".agents/skills/check")
			original := writeLocal(t, target, "Original")
			doc, _ := c.ReadLocalSkill(ctx, target, "SKILL.md")
			p, e := c.PlanLocalEdit(ctx, target, "SKILL.md", doc.Fingerprint, original+"Changed")
			if e != nil {
				t.Fatal(e)
			}
			stop := errors.New("simulated interruption")
			e = s.applyLocalPlan(ctx, p, func(step string) error {
				if step == stage {
					return stop
				}
				return nil
			})
			if !errors.Is(e, stop) {
				t.Fatal(e)
			}
			if e = s.Recover(ctx); e != nil {
				t.Fatal(e)
			}
			raw, e := os.ReadFile(filepath.Join(target, "SKILL.md"))
			want := original
			if stage == "committed" {
				want += "Changed"
			}
			if e != nil || string(raw) != want {
				t.Fatalf("recovery content %q: %v", raw, e)
			}
			if e = s.Recover(ctx); e != nil {
				t.Fatal("recovery not idempotent", e)
			}
		})
	}
}
func TestLocalRemoveLinkPreservesSharedFilesAndRestoresLink(t *testing.T) {
	ctx := context.Background()
	c, s, home := localFixture(t)
	target := filepath.Join(home, ".agents/skills/check")
	writeLocal(t, target, "Shared")
	link := filepath.Join(home, ".claude/skills/check")
	_ = os.MkdirAll(filepath.Dir(link), 0700)
	if e := os.Symlink(target, link); e != nil {
		t.Skip(e)
	}
	p, e := c.PlanLocalRemove(ctx, link)
	if e != nil {
		t.Fatal(e)
	}
	if p.Target != link || p.Link != target {
		t.Fatalf("wrong link plan: %+v", p)
	}
	if e = c.ApplyLocalPlan(ctx, p); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(target, "SKILL.md")); e != nil {
		t.Fatal("shared files removed", e)
	}
	b, _ := s.LocalBackups(ctx)
	r, e := c.PlanLocalRestore(ctx, b[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.ApplyLocalPlan(ctx, r); e != nil {
		t.Fatal(e)
	}
	if got, e := os.Readlink(link); e != nil || got != target {
		t.Fatal("link not restored", got, e)
	}
}
func TestLocalCopyAndExternalOwnership(t *testing.T) {
	ctx := context.Background()
	c, _, home := localFixture(t)
	target := filepath.Join(home, ".agents/skills/check")
	writeLocal(t, target, "External")
	if e := os.Mkdir(filepath.Join(target, ".git"), 0700); e != nil {
		t.Fatal(e)
	}
	doc, e := c.ReadLocalSkill(ctx, target, "SKILL.md")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.PlanLocalEdit(ctx, target, "SKILL.md", doc.Fingerprint, "bad"); e == nil {
		t.Fatal("edited externally managed files")
	}
	if _, e = c.PlanLocalRemove(ctx, target); e == nil {
		t.Fatal("removed externally managed files")
	}
	// 独立副本不包含原目录的 Git 管理标记。
	destination := filepath.Join(t.TempDir(), "check")
	p, e := c.PlanLocalCopy(ctx, target, destination)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.ApplyLocalPlan(ctx, p); e != nil {
		t.Fatal(e)
	}
	if _, e = c.PlanLocalCopy(ctx, target, destination); e == nil {
		t.Fatal("overwrote copy")
	}
	inv, e := c.LocalInventory(ctx)
	if e != nil || len(inv.Skills) != 2 {
		t.Fatal("copy not discoverable", inv, e)
	}
}
func TestLocalConfigurationUsesCredentialStoreOffline(t *testing.T) {
	ctx := context.Background()
	c, _, home := localFixture(t)
	target := filepath.Join(home, ".agents/skills/check")
	writeLocal(t, target, "Local")
	if e := os.WriteFile(filepath.Join(target, SkillConfigFile), []byte(configFixture), 0600); e != nil {
		t.Fatal(e)
	}
	if e := c.SetSkillConfiguration(ctx, target, "DEMO_TOKEN", "keychain", "local-secret-fixture"); e != nil {
		t.Fatal(e)
	}
	if e := c.SetSkillConfiguration(ctx, target, "DEMO_REGION", "value", "test"); e != nil {
		t.Fatal(e)
	}
	state, e := c.SkillConfiguration(ctx, target)
	if e != nil || !state.Ready {
		t.Fatal("local config unusable", state, e)
	}
	doc, _ := c.ReadLocalSkill(ctx, target, "SKILL.md")
	p, e := c.PlanLocalEdit(ctx, target, "SKILL.md", doc.Fingerprint, doc.File.Content+"Changed")
	if e != nil {
		t.Fatal(e)
	}
	if e = c.ApplyLocalPlan(ctx, p); e != nil {
		t.Fatal(e)
	}
	state, e = c.SkillConfiguration(ctx, target)
	if e != nil || !state.Changed {
		t.Fatal("changed content kept old credential authorization", state, e)
	}
	for _, f := range mustLocalFiles(t, target) {
		if strings.Contains(string(f.Data), "local-secret-fixture") {
			t.Fatal("secret persisted in skill")
		}
	}
}
func mustLocalFiles(t *testing.T, path string) []packageformat.File {
	t.Helper()
	f, e := packageformat.ReadDirectory(path)
	if e != nil {
		t.Fatal(e)
	}
	return f
}

func TestLocalBackupCleanupRejectsChangedContents(t *testing.T) {
	ctx := context.Background()
	c, s, home := localFixture(t)
	target := filepath.Join(home, ".agents/skills/check")
	original := writeLocal(t, target, "Original")
	doc, _ := c.ReadLocalSkill(ctx, target, "SKILL.md")
	plan, e := c.PlanLocalEdit(ctx, target, "SKILL.md", doc.Fingerprint, original+"Edit")
	if e != nil {
		t.Fatal(e)
	}
	if e = c.ApplyLocalPlan(ctx, plan); e != nil {
		t.Fatal(e)
	}
	backups, _ := s.LocalBackups(ctx)
	b := backups[0]
	file := filepath.Join(b.Directory, "old", "SKILL.md")
	if e = os.WriteFile(file, []byte("Changed outside SkillHub"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteLocalBackup(ctx, b.ID); e == nil {
		t.Fatal("deleted modified backup")
	}
	if e = os.WriteFile(file, []byte(original), 0600); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteLocalBackup(ctx, b.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(b.Directory); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("backup not removed", e)
	}
	if raw, e := os.ReadFile(filepath.Join(target, "SKILL.md")); e != nil || string(raw) != original+"Edit" {
		t.Fatal("cleanup changed current files", e)
	}
}

func TestLocalDocumentsRejectPathsOutsideSkill(t *testing.T) {
	ctx := context.Background()
	c, _, home := localFixture(t)
	target := filepath.Join(home, ".agents/skills/check")
	writeLocal(t, target, "Original")
	outside := filepath.Join(home, "private.txt")
	if err := os.WriteFile(outside, []byte("private fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := c.ReadLocalSkill(ctx, target, "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{outside, "../../../private.txt", "../check/../private.txt"} {
		if _, err := c.ReadLocalSkill(ctx, target, path); err == nil {
			t.Fatalf("read escaped skill: %s", path)
		}
		if _, err := c.PlanLocalEdit(ctx, target, path, doc.Fingerprint, "replacement"); err == nil {
			t.Fatalf("edit escaped skill: %s", path)
		}
	}
	if content, err := os.ReadFile(outside); err != nil || string(content) != "private fixture" {
		t.Fatal("outside file changed", err)
	}
}

func TestLocalBackupRejectsRedirectedDirectory(t *testing.T) {
	ctx := context.Background()
	c, s, home := localFixture(t)
	target := filepath.Join(home, ".agents/skills/check")
	original := writeLocal(t, target, "Original")
	doc, err := c.ReadLocalSkill(ctx, target, "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := c.PlanLocalEdit(ctx, target, "SKILL.md", doc.Fingerprint, original+"Edit")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ApplyLocalPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	backups, err := s.LocalBackups(ctx)
	if err != nil || len(backups) != 1 {
		t.Fatal("backup missing", err)
	}
	b := backups[0]
	saved := b.Directory + "-saved"
	if err = os.Rename(b.Directory, saved); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(saved, b.Directory); err != nil {
		t.Skip(err)
	}
	if _, err = c.PlanLocalRestore(ctx, b.ID); err == nil {
		t.Fatal("accepted redirected backup")
	}
	if err = s.DeleteLocalBackup(ctx, b.ID); err == nil {
		t.Fatal("deleted redirected backup")
	}
	if content, err := os.ReadFile(filepath.Join(saved, "old", "SKILL.md")); err != nil || string(content) != original {
		t.Fatal("backup target changed", err)
	}
}
