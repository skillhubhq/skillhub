package hub

import (
	"context"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"os"
	"path/filepath"
	"testing"
)

func TestPresentationMigrationPreservesLegacyReleaseAndBackup(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutNamespace(ctx, actor, "namespace", PutNamespace{Name: "team"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: legacy-check\ndescription: A legacy package.\n---\n# Legacy\n"), 0600); err != nil {
		t.Fatal(err)
	}
	archive, _, err := packageformat.Pack(dir)
	if err != nil {
		t.Fatal(err)
	}
	original, err := s.SubmitRelease(ctx, actor, "submit-legacy", SubmitRelease{Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS})
	if err != nil {
		t.Fatal(err)
	}
	id := s.RegistryID()
	if _, err = s.db.Exec(`DROP TABLE evaluation_evidence; DROP TABLE featured_skills; DROP INDEX releases_review_queue; DROP TABLE review_notifications; DROP TABLE release_scans; DROP TABLE platform_roles; DROP TABLE access_requests; ALTER TABLE releases DROP COLUMN display_name_en; ALTER TABLE releases DROP COLUMN display_name_zh; ALTER TABLE releases DROP COLUMN icon; PRAGMA user_version=5;`); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.RegistryID() != id {
		t.Fatal("changed registry identity")
	}
	r, bytes, err := s.DownloadRelease(ctx, actor, "team", "legacy-check", "1.0.0")
	if err != nil || r.ID != original.ID || r.Digest != original.Digest || packageformat.Digest(bytes) != original.Digest || r.DisplayNameEN != "" || r.DisplayNameZH != "" || r.Icon != 0 {
		t.Fatal("legacy release changed", err)
	}
	info, err := s.Backup(ctx, filepath.Join(t.TempDir(), "backup"))
	if err != nil || info.Schema != currentSchema {
		t.Fatal("schema 6 backup failed", err)
	}
}
