package hub

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/provenance"
)

func TestProvenanceReviewAndIdentitySeparation(t *testing.T) {
	testProvenance(t, hubdb.Config{Driver: "sqlite"})
}
func TestPostgresProvenanceReviewAndIdentitySeparation(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testProvenance(t, cfg)
}
func testProvenance(t *testing.T, cfg hubdb.Config) {
	s, e := OpenDatabase(t.TempDir(), cfg)
	mustGovernance(t, e)
	defer s.Close()
	ctx := context.Background()
	admin, e := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	curator := governanceUser(t, s, admin, "curator")
	_, e = s.PutNamespace(ctx, admin, "provenance-space", PutNamespace{Name: "curated", ReviewRequired: false})
	mustGovernance(t, e)
	e = s.SetNamespaceGrant(ctx, admin, "provenance-grant", SetNamespaceGrant{NamespaceGrant: NamespaceGrant{Namespace: "curated", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: curator.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER}, Granted: true})
	mustGovernance(t, e)
	dir := t.TempDir()
	mustGovernance(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: upstream-example\ndescription: A curated example\n---\nOriginal skill content."), 0600))
	mustGovernance(t, os.WriteFile(filepath.Join(dir, "LICENSE.txt"), []byte("Example license notice"), 0600))
	archive, _, e := packageformat.Pack(dir)
	mustGovernance(t, e)
	p := &provenance.Record{AuthorName: "Original Authors", AuthorURL: "https://example.org/authors", SourceURL: "https://example.org/source/" + strings.Repeat("a", 40), Revision: strings.Repeat("a", 40), License: "Apache-2.0", LicenseFile: "LICENSE.txt"}
	cmd := SubmitRelease{Namespace: "curated", Version: "0.0.0-upstream.aaaaaaaaaaaa", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC, Provenance: p}
	r, e := s.SubmitRelease(ctx, curator, "provenance-submit", cmd)
	mustGovernance(t, e)
	if r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING || r.SubmittedBy != curator.ID || r.Namespace != "curated" || *r.Provenance != *p {
		t.Fatal("identity attribution or forced review lost")
	}
	if _, e = s.ReviewRelease(ctx, curator, "provenance-self", ReviewRelease{ReleaseID: r.ID, Approve: true, Reason: "Self"}); !errors.Is(e, ErrForbidden) {
		t.Fatal("self review accepted", e)
	}
	if _, e = s.GetSkill(ctx, Identity{}, "curated", "upstream-example"); !errors.Is(e, ErrNotFound) {
		t.Fatal("pending attribution exposed", e)
	}
	_, e = s.ReviewRelease(ctx, admin, "provenance-approve", ReviewRelease{ReleaseID: r.ID, Approve: true, Reason: "Reviewed source, license and content"})
	mustGovernance(t, e)
	skill, e := s.GetSkill(ctx, Identity{}, "curated", "upstream-example")
	mustGovernance(t, e)
	if skill.Provenance == nil || *skill.Provenance != *p {
		t.Fatal("catalog lost provenance")
	}
	contributions, e := s.ListContributions(ctx, Identity{}, "", "curated", Page{}, false)
	mustGovernance(t, e)
	if len(contributions.Items) != 1 || contributions.Items[0].Author.UserID != curator.ID || contributions.Items[0].Provenance.AuthorName != p.AuthorName {
		t.Fatal("operator was replaced with upstream identity")
	}
	got, data, e := s.DownloadRelease(ctx, Identity{}, "curated", "upstream-example", cmd.Version)
	mustGovernance(t, e)
	if *got.Provenance != *p || packageformat.Digest(data) != packageformat.Digest(archive) {
		t.Fatal("archive or attribution changed")
	}
	changed := *p
	changed.AuthorName = "Different Author"
	cmd.Provenance = &changed
	if _, e = s.SubmitRelease(ctx, curator, "provenance-submit", cmd); !errors.Is(e, ErrConflict) {
		t.Fatal("idempotency ignored attribution", e)
	}
	if _, e = s.SubmitRelease(ctx, curator, "provenance-new-key", cmd); !errors.Is(e, ErrConflict) {
		t.Fatal("existing version attribution overwritten", e)
	}
	var actor string
	mustGovernance(t, s.db.QueryRow("SELECT actor_id FROM audit WHERE action='release.review' AND resource=?", r.ID).Scan(&actor))
	if actor != admin.ID {
		t.Fatal("review audit identity lost")
	}
}
func TestProvenanceSchemaUpgradePreservesIdentityAndBackup(t *testing.T) {
	testProvenanceUpgrade(t, hubdb.Config{Driver: "sqlite"})
}
func TestPostgresProvenanceSchemaUpgradePreservesIdentityAndBackup(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testProvenanceUpgrade(t, cfg)
}
func testProvenanceUpgrade(t *testing.T, cfg hubdb.Config) {
	root := t.TempDir()
	s, e := OpenDatabase(root, cfg)
	mustGovernance(t, e)
	ctx := context.Background()
	admin, e := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	id := s.RegistryID()
	_, e = s.PutNamespace(ctx, admin, "provenance-space", PutNamespace{Name: "original"})
	mustGovernance(t, e)
	r, e := s.SubmitRelease(ctx, admin, "provenance-submit", SubmitRelease{Namespace: "original", Version: "1.0.0", Archive: governanceArchive(t, "Existing content"), Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC})
	mustGovernance(t, e)
	_, e = s.db.Exec("ALTER TABLE releases DROP COLUMN provenance; ALTER TABLE namespaces DROP COLUMN archived; DROP TABLE namespace_tombstones; DROP TABLE package_cleanup; DROP TABLE release_tombstones; PRAGMA user_version=1;")
	mustGovernance(t, e)
	if cfg.Driver == "sqlite" {
		snapshot := filepath.Join(t.TempDir(), "old-backup")
		mustGovernance(t, os.Mkdir(snapshot, 0700))
		_, e = s.db.Exec("VACUUM INTO ?", filepath.Join(snapshot, "hub.db"))
		mustGovernance(t, e)
		digest, bytes, e := hashBackupFile(ctx, filepath.Join(snapshot, "hub.db"))
		mustGovernance(t, e)
		info := BackupInfo{Format: 1, Schema: 1, RegistryID: id, CreatedAt: time.Now().UTC().Format(time.RFC3339), DatabaseDigest: digest, DatabaseBytes: bytes, Packages: 1}
		blob, e := s.readBlob(r.Digest)
		mustGovernance(t, e)
		writer := &Store{root: snapshot}
		mustGovernance(t, writer.writeBlob(r.Digest, blob))
		raw, _ := json.Marshal(info)
		mustGovernance(t, os.WriteFile(filepath.Join(snapshot, "backup.json"), raw, 0600))
		_, e = VerifyBackup(ctx, snapshot)
		mustGovernance(t, e)
		target := filepath.Join(t.TempDir(), "restored")
		_, e = RestoreBackup(ctx, snapshot, target)
		mustGovernance(t, e)
		restored, e := Open(target)
		mustGovernance(t, e)
		release, data, e := restored.DownloadRelease(ctx, Identity{}, "original", "governance-check", "1.0.0")
		mustGovernance(t, e)
		if release.Digest != r.Digest || packageformat.Digest(data) != r.Digest || restored.RegistryID() != id {
			t.Fatal("old backup upgrade lost data")
		}
		restored.Close()
		unchanged, e := VerifyBackup(ctx, snapshot)
		mustGovernance(t, e)
		if unchanged.Schema != 1 {
			t.Fatal("restore modified source snapshot")
		}
	}
	s.Close()
	// 旧快照保持原样；升级只在打开可写目标时进行。
	s, e = OpenDatabase(root, cfg)
	mustGovernance(t, e)
	defer s.Close()
	login, e := s.Login(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	got, data, e := s.DownloadRelease(ctx, login.Identity, "original", "governance-check", "1.0.0")
	mustGovernance(t, e)
	if s.RegistryID() != id || login.Identity.ID != admin.ID || got.ID != r.ID || got.Digest != r.Digest || got.Provenance != nil || packageformat.Digest(data) != r.Digest {
		t.Fatal("upgrade changed existing content or identity")
	}
	var schema int
	mustGovernance(t, s.db.QueryRow("PRAGMA user_version").Scan(&schema))
	if schema != currentSchema {
		t.Fatal(schema)
	}
	if cfg.Driver == "sqlite" {
		snapshot := filepath.Join(t.TempDir(), "backup")
		_, e = s.Backup(ctx, snapshot)
		mustGovernance(t, e)
		_, e = VerifyBackup(ctx, snapshot)
		mustGovernance(t, e)
	}
}
