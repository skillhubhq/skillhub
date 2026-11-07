package hub

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

func postgresFixture(t *testing.T) (hubdb.Config, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("SKILLHUB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "skillhub_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE " + name + " WITH (FORCE)"); _ = admin.Close() })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	return hubdb.Config{Driver: "postgres", DSN: u.String()}, admin
}
func TestPostgresOwnershipBeforeMigrationAndDisconnect(t *testing.T) {
	cfg, admin := postgresFixture(t)
	ctx := context.Background()
	first, err := OpenDatabase(t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err = first.db.Exec("DROP TABLE featured_skills; DROP INDEX releases_review_queue; DROP TABLE review_notifications; DROP TABLE release_scans; DROP TABLE platform_roles; DROP TABLE access_requests; ALTER TABLE releases DROP COLUMN display_name_en; ALTER TABLE releases DROP COLUMN display_name_zh; ALTER TABLE releases DROP COLUMN icon; PRAGMA user_version=5;"); err != nil {
		t.Fatal(err)
	}
	if second, err := OpenDatabase(t.TempDir(), cfg); err == nil {
		second.Close()
		t.Fatal("second store opened before ownership check")
	}
	var schema int
	if err = first.db.QueryRow("PRAGMA user_version").Scan(&schema); err != nil || schema != 5 {
		t.Fatal("rejected process migrated active schema", err)
	}
	var pid int
	if err = first.db.QueryRow("SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec("SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.DatabaseLeaseLost():
	case <-time.After(5 * time.Second):
		t.Fatal("lost ownership not signalled")
	}
	second, err := OpenDatabase(t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err = second.db.QueryRow("PRAGMA user_version").Scan(&schema); err != nil || schema != currentSchema {
		t.Fatal("normal migration failed", err)
	}
	if _, err = first.db.ExecContext(ctx, "UPDATE instance SET id=id"); err == nil {
		t.Fatal("old store reconnected for write")
	}
	if err = first.db.QueryRow("SELECT count(*) FROM instance").Scan(&schema); err == nil {
		t.Fatal("old store reconnected for read")
	}
	if _, err = first.db.BeginTx(ctx, nil); err == nil {
		t.Fatal("old store began transaction")
	}
}
func TestPostgresPairedBackupRestoreAndFailureGuards(t *testing.T) {
	ctx := context.Background()
	cfg, _ := postgresFixture(t)
	destCfg, _ := postgresFixture(t)
	root := t.TempDir()
	s, err := OpenDatabase(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.Login(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutNamespace(ctx, actor, "namespace", PutNamespace{Name: "team", ReviewRequired: true}); err != nil {
		t.Fatal(err)
	}
	pkg := t.TempDir()
	if err = os.WriteFile(filepath.Join(pkg, "SKILL.md"), []byte("---\nname: recovery-check\ndescription: Synthetic recovery fixture.\n---\n# Recovery\n"), 0600); err != nil {
		t.Fatal(err)
	}
	archive, _, err := packageformat.Pack(pkg)
	if err != nil {
		t.Fatal(err)
	}
	release, err := s.SubmitRelease(ctx, actor, "publish-release", SubmitRelease{Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS})
	if err != nil {
		t.Fatal(err)
	}
	reviewer := governanceUser(t, s, actor, "reviewer")
	mustGovernance(t, s.SetNamespaceGrant(ctx, actor, "reviewer-role", SetNamespaceGrant{NamespaceGrant: NamespaceGrant{Namespace: "team", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: reviewer.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER}, Granted: true}))
	mustGovernance(t, s.SetPlatformRole(ctx, actor, "audit-role", reviewer.ID, taxonomy.PlatformRole_PLATFORM_ROLE_AUDITOR, true))
	_, err = s.ReviewRelease(ctx, reviewer, "review-before-backup", ReviewRelease{ReleaseID: release.ID, Approve: true})
	mustGovernance(t, err)
	mustGovernance(t, s.SetFeaturedSkill(ctx, actor, "feature-before-backup", "team", "recovery-check", true))
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err = BackupPostgres(ctx, cfg, root, backup); err == nil {
		t.Fatal("backup allowed while database serving")
	}
	id := s.RegistryID()
	s.Close()
	info, err := BackupPostgres(ctx, cfg, root, backup)
	if err != nil {
		t.Fatal(err)
	}
	if info.RegistryID != id || info.Packages != 1 {
		t.Fatal("backup metadata mismatch")
	}
	if _, err = VerifyPostgresBackup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if _, err = RestorePostgres(ctx, destCfg, backup, restored); err != nil {
		t.Fatal(err)
	}
	copy, err := OpenDatabase(restored, destCfg)
	if err != nil {
		t.Fatal(err)
	}
	if copy.RegistryID() != id {
		t.Fatal("identity changed")
	}
	if _, err = copy.Authenticate(ctx, session.Token); err == nil {
		t.Fatal("restored session revived")
	}
	if _, err = copy.Login(ctx, "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	r, bytes, err := copy.DownloadRelease(ctx, actor, "team", "recovery-check", "1.0.0")
	if err != nil || r.ID != release.ID || packageformat.Digest(bytes) != release.Digest {
		t.Fatal("restored package changed", err)
	}
	report, err := copy.GetScanReport(ctx, reviewer, release.ID)
	mustGovernance(t, err)
	if report.Digest != release.Digest || report.Engine == "" {
		t.Fatal("restored scan evidence missing")
	}
	access, err := copy.GetPlatformAccess(ctx, reviewer, "")
	mustGovernance(t, err)
	if !access.CanReadAudit || access.CanManageUsers {
		t.Fatal("restored platform duties changed")
	}
	notices, err := copy.ListNotifications(ctx, actor, Page{})
	mustGovernance(t, err)
	if len(notices.Items) != 1 || notices.Items[0].ReleaseID != release.ID {
		t.Fatal("restored notification missing")
	}
	picks, err := copy.SearchSkills(ctx, actor, Search{FeaturedOnly: true}, Page{})
	mustGovernance(t, err)
	if len(picks.Items) != 1 || !picks.Items[0].Featured {
		t.Fatal("restored curated selection missing")
	}
	copy.Close()
	if _, err = RestorePostgres(ctx, destCfg, backup, filepath.Join(t.TempDir(), "must-not-write")); err == nil {
		t.Fatal("nonempty database overwritten")
	}
	// 错误配对即使 pg_restore 成功也必须保持数据库和文件启动阻断。
	manifestPath := filepath.Join(backup, backupManifestName)
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	wrong := info
	wrong.RegistryID = "wrong-registry"
	raw, _ := json.Marshal(wrong)
	if err = os.WriteFile(manifestPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	failedCfg, _ := postgresFixture(t)
	failedRoot := filepath.Join(t.TempDir(), "failed-restore")
	if _, err = RestorePostgres(ctx, failedCfg, backup, failedRoot); err == nil {
		t.Fatal("wrong identity accepted")
	}
	if _, err = os.Stat(filepath.Join(failedRoot, maintenanceMarker)); err != nil {
		t.Fatal("missing failed-restore marker", err)
	}
	if bad, err := OpenDatabase(t.TempDir(), failedCfg); err == nil {
		bad.Close()
		t.Fatal("failed restore served through another root")
	}
	if err = os.WriteFile(manifestPath, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	packages := filepath.Join(backup, "packages")
	if err = os.Rename(packages, packages+"-hidden"); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyPostgresBackup(ctx, backup); err == nil {
		t.Fatal("missing packages accepted")
	}
	if err = os.Rename(packages+"-hidden", packages); err != nil {
		t.Fatal(err)
	}
	dump := filepath.Join(backup, "hub.dump")
	f, err := os.OpenFile(dump, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("corrupt"))
	f.Close()
	if _, err = VerifyPostgresBackup(ctx, backup); err == nil {
		t.Fatal("corrupt archive accepted")
	}
	guardedCfg, _ := postgresFixture(t)
	db, err := hubdb.Open(t.TempDir(), guardedCfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TABLE skillhub_recovery(id BIGINT PRIMARY KEY)")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if st, err := OpenDatabase(t.TempDir(), guardedCfg); err == nil {
		st.Close()
		t.Fatal("recovery marker bypassed with another root")
	}
}

func TestPostgresUpgradeAndRollbackPreservePreUpgradeSchema(t *testing.T) {
	ctx := context.Background()
	cfg, _ := postgresFixture(t)
	rollbackCfg, _ := postgresFixture(t)
	root := t.TempDir()
	s, err := OpenDatabase(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	id := s.RegistryID()
	if _, err = s.db.Exec("DROP TABLE featured_skills; DROP INDEX releases_review_queue; DROP TABLE review_notifications; DROP TABLE release_scans; DROP TABLE platform_roles; DROP TABLE access_requests; PRAGMA user_version=6;"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	backup := filepath.Join(t.TempDir(), "before-upgrade")
	after, err := UpgradePostgres(ctx, cfg, root, backup)
	if err != nil {
		t.Fatal(err)
	}
	if after.Schema != currentSchema || after.RegistryID != id {
		t.Fatal("upgrade identity mismatch")
	}
	before, err := VerifyPostgresBackup(ctx, backup)
	if err != nil || before.Schema != 6 {
		t.Fatal("pre-upgrade backup was migrated", err)
	}
	if _, err = os.Stat(filepath.Join(backup, "upgrade.json")); err != nil {
		t.Fatal("missing upgrade receipt")
	}
	rollbackRoot := filepath.Join(t.TempDir(), "rollback")
	if _, err = RestorePostgres(ctx, rollbackCfg, backup, rollbackRoot); err != nil {
		t.Fatal(err)
	}
	raw, err := hubdb.Open(rollbackRoot, rollbackCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var version int
	var userID string
	if err = raw.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 6 {
		t.Fatal("rollback did not preserve old schema", err)
	}
	if err = raw.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&userID); err != nil || userID != actor.ID {
		t.Fatal("rollback identity changed", err)
	}
}

func TestPostgresFailedMigrationAndFutureSchema(t *testing.T) {
	ctx := context.Background()
	cfg, _ := postgresFixture(t)
	root := t.TempDir()
	s, err := OpenDatabase(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	id := s.RegistryID()
	// 留下同名表触发真实 DDL 冲突，版本和身份应保留，升级前备份仍可校验。
	if _, err = s.db.Exec("PRAGMA user_version=6;"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	backup := filepath.Join(t.TempDir(), "failed-upgrade")
	if _, err = UpgradePostgres(ctx, cfg, root, backup); err == nil {
		t.Fatal("conflicting migration succeeded")
	}
	if _, err = VerifyPostgresBackup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(backup, "upgrade.json")); !os.IsNotExist(err) {
		t.Fatal("failed migration has success receipt")
	}
	raw, err := hubdb.Open(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	var restoredID string
	if err = raw.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 6 {
		t.Fatal("failed migration changed version", err)
	}
	if err = raw.QueryRow("SELECT id FROM instance").Scan(&restoredID); err != nil || restoredID != id {
		t.Fatal("failed migration changed identity", err)
	}
	if _, err = raw.Exec("PRAGMA user_version=999;"); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	if future, err := OpenDatabase(root, cfg); err == nil {
		future.Close()
		t.Fatal("future schema accepted")
	}
}

func TestPostgresBackupRejectsUnrelatedTables(t *testing.T) {
	cfg, _ := postgresFixture(t)
	root := t.TempDir()
	s, err := OpenDatabase(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("CREATE TABLE unrelated_application(id TEXT)"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	target := filepath.Join(t.TempDir(), "backup")
	if _, err = BackupPostgres(context.Background(), cfg, root, target); err == nil {
		t.Fatal("unrelated data included in backup")
	}
	if _, err = os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("rejected backup wrote output")
	}
}

func TestPostgresMigrationCancellation(t *testing.T) {
	cfg, _ := postgresFixture(t)
	root := t.TempDir()
	s, err := OpenDatabase(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DROP TABLE featured_skills; DROP INDEX releases_review_queue; DROP TABLE review_notifications; DROP TABLE release_scans; DROP TABLE platform_roles; DROP TABLE access_requests; PRAGMA user_version=6;"); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	external, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer external.Close()
	blocker, err := external.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err = blocker.Exec("LOCK TABLE users IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err = s.initializeContext(ctx); err == nil {
		t.Fatal("blocked migration ignored cancellation")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("migration exceeded bounded cancellation")
	}
	blocker.Rollback()
	var version int
	if err = external.QueryRow("SELECT version FROM hub_schema").Scan(&version); err != nil || version != 6 {
		t.Fatal("cancelled migration changed schema", err)
	}
}

func TestPostgresGovernanceUpgradeAndRollback(t *testing.T) {
	ctx := context.Background()
	cfg, _ := postgresFixture(t)
	destCfg, _ := postgresFixture(t)
	root := t.TempDir()
	s, e := OpenDatabase(root, cfg)
	mustGovernance(t, e)
	actor, e := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	registry := s.RegistryID()
	_, e = s.db.Exec("DROP TABLE featured_skills; DROP INDEX releases_review_queue; DROP TABLE review_notifications; DROP TABLE release_scans; DROP TABLE platform_roles; PRAGMA user_version=7;")
	mustGovernance(t, e)
	mustGovernance(t, s.Close())
	backup := filepath.Join(t.TempDir(), "schema7")
	info, e := BackupPostgres(ctx, cfg, root, backup)
	mustGovernance(t, e)
	if info.Schema != 7 {
		t.Fatal("backup migrated source")
	}
	migrated, e := OpenDatabase(root, cfg)
	mustGovernance(t, e)
	a, e := migrated.GetPlatformAccess(ctx, actor, "")
	mustGovernance(t, e)
	if !a.Administrator || len(a.Roles) != 0 {
		t.Fatal("migration changed access")
	}
	mustGovernance(t, migrated.Close())
	recovered := filepath.Join(t.TempDir(), "rollback")
	_, e = RestorePostgres(ctx, destCfg, backup, recovered)
	mustGovernance(t, e)
	old, e := hubdb.Open(filepath.Join(recovered, "hub.db"), destCfg)
	mustGovernance(t, e)
	defer old.Close()
	var version int
	var id string
	mustGovernance(t, old.QueryRow("PRAGMA user_version").Scan(&version))
	mustGovernance(t, old.QueryRow("SELECT id FROM instance").Scan(&id))
	if version != 7 || id != registry {
		t.Fatal("rollback lost original version/identity")
	}
}

func TestPostgresDiscoveryUpgradeAndRollback(t *testing.T) {
	ctx := context.Background()
	cfg, _ := postgresFixture(t)
	destCfg, _ := postgresFixture(t)
	root := t.TempDir()
	s, e := OpenDatabase(root, cfg)
	mustGovernance(t, e)
	actor, e := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	registry := s.RegistryID()
	_, e = s.db.Exec("DROP TABLE featured_skills; DROP INDEX releases_review_queue; PRAGMA user_version=8;")
	mustGovernance(t, e)
	mustGovernance(t, s.Close())
	backup := filepath.Join(t.TempDir(), "schema8")
	info, e := BackupPostgres(ctx, cfg, root, backup)
	mustGovernance(t, e)
	if info.Schema != 8 {
		t.Fatal("backup migrated source")
	}
	migrated, e := OpenDatabase(root, cfg)
	mustGovernance(t, e)
	a, e := migrated.GetPlatformAccess(ctx, actor, "")
	mustGovernance(t, e)
	if !a.Administrator || len(a.Roles) != 0 {
		t.Fatal("migration changed access")
	}
	mustGovernance(t, migrated.Close())
	recovered := filepath.Join(t.TempDir(), "rollback")
	_, e = RestorePostgres(ctx, destCfg, backup, recovered)
	mustGovernance(t, e)
	old, e := hubdb.Open(filepath.Join(recovered, "hub.db"), destCfg)
	mustGovernance(t, e)
	defer old.Close()
	var version int
	var id string
	mustGovernance(t, old.QueryRow("PRAGMA user_version").Scan(&version))
	mustGovernance(t, old.QueryRow("SELECT id FROM instance").Scan(&id))
	if version != 8 || id != registry {
		t.Fatal("rollback lost original version/identity")
	}
}
