package hub

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	c "github.com/skillhubhq/skillhub/internal/community"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

func TestNamespaceLifecycle(t *testing.T) { testNamespaceLifecycle(t, hubdb.Config{}) }
func TestPostgresNamespaceLifecycle(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testNamespaceLifecycle(t, cfg)
}
func testNamespaceLifecycle(t *testing.T, cfg hubdb.Config) {
	ctx := context.Background()
	s, e := OpenDatabase(t.TempDir(), cfg)
	mustGovernance(t, e)
	defer s.Close()
	admin, e := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	member := governanceUser(t, s, admin, "member")
	_, e = s.PutNamespace(ctx, admin, "namespace-create", PutNamespace{Name: "team"})
	mustGovernance(t, e)
	_, e = s.PutNamespace(ctx, admin, "namespace-shared", PutNamespace{Name: "shared"})
	mustGovernance(t, e)
	group, e := s.CreateGroup(ctx, admin, "shared-group", "shared")
	mustGovernance(t, e)
	mustGovernance(t, s.SetGroupMember(ctx, admin, "shared-member", SetGroupMember{GroupID: group.ID, UserID: member.ID, Member: true}))
	mustGovernance(t, s.SetNamespaceGrant(ctx, admin, "namespace-access", SetNamespaceGrant{NamespaceGrant: NamespaceGrant{Namespace: "team", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_GROUP, SubjectID: group.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN}, Granted: true}))
	d := t.TempDir()
	mustGovernance(t, os.WriteFile(filepath.Join(d, "SKILL.md"), []byte("---\nname: lifecycle-check\ndescription: Check lifecycle.\n---\nSummarize supplied text.\n"), 0600))
	archive, _, e := packageformat.Pack(d)
	mustGovernance(t, e)
	cmd := SubmitRelease{Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC}
	release, e := s.SubmitRelease(ctx, member, "member-publish", cmd)
	mustGovernance(t, e)
	cmd.Namespace = "shared"
	_, e = s.SubmitRelease(ctx, admin, "shared-publish", cmd)
	mustGovernance(t, e)
	comment, e := s.CreateComment(ctx, member, "comment-root", c.CommentChange{ReleaseID: release.ID, Body: "Keep until purge"})
	mustGovernance(t, e)
	_, e = s.CreateComment(ctx, member, "comment-reply", c.CommentChange{ReleaseID: release.ID, ParentID: comment.CommentID, Body: "Reply"})
	mustGovernance(t, e)
	_, e = s.SetReaction(ctx, member, "save-release", c.ReactionChange{ReleaseID: release.ID, Bookmark: true, Active: true})
	mustGovernance(t, e)
	_, e = s.db.Exec("INSERT INTO evaluation_evidence(id,release_id,digest,metadata,document,retracted) VALUES(?,?,?,?,?,0)", "fixture-evidence", release.ID, release.Digest, "{}", "{}")
	mustGovernance(t, e)
	_, e = s.GetNamespaceLifecycle(ctx, member, "team")
	if !errors.Is(e, ErrForbidden) {
		t.Fatal("non-Hub admin preview", e)
	}
	change := ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: 1, Archived: true}
	_, e = s.SetNamespaceArchived(ctx, member, "member-archive", change)
	if !errors.Is(e, ErrForbidden) {
		t.Fatal("namespace admin archived", e)
	}
	_, e = s.PurgeNamespace(ctx, admin, "active-purge", ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: 1, ConfirmName: "team"})
	if !errors.Is(e, ErrConflict) {
		t.Fatal("purged active namespace", e)
	}
	v, e := s.SetNamespaceArchived(ctx, admin, "archive-team", change)
	mustGovernance(t, e)
	if !v.Archived || v.Revision != 2 || v.Comments != 2 || v.Grants != 1 || v.Evidence != 1 {
		t.Fatalf("bad preview: %+v", v)
	}
	_, e = s.SetNamespaceArchived(ctx, admin, "stale-archive", change)
	if !errors.Is(e, ErrConflict) {
		t.Fatal("stale revision", e)
	}
	for _, actor := range []Identity{{}, member, admin} {
		_, _, e = s.DownloadRelease(ctx, actor, "team", "lifecycle-check", "1.0.0")
		if !errors.Is(e, ErrNotFound) {
			t.Fatal("archived download", e)
		}
		_, e = s.GetTeam(ctx, actor, "team")
		if !errors.Is(e, ErrNotFound) {
			t.Fatal("archived team", e)
		}
		_, e = s.GetDiscussion(ctx, actor, release.ID)
		if !errors.Is(e, ErrNotFound) {
			t.Fatal("archived discussion", e)
		}
	}
	_, _, e = s.GetSubmission(ctx, admin, release.ID)
	if !errors.Is(e, ErrNotFound) {
		t.Fatal("archived submission bypass", e)
	}
	cmd.Namespace = "team"
	cmd.Version = "1.0.1"
	_, e = s.SubmitRelease(ctx, admin, "archived-publish", cmd)
	if !errors.Is(e, ErrNotFound) {
		t.Fatal("archived publish", e)
	}
	choices, e := s.ListSubmissionNamespaces(ctx, admin, Page{})
	mustGovernance(t, e)
	for _, n := range choices.Items {
		if n.Name == "team" {
			t.Fatal("archived publication choice")
		}
	}
	v, e = s.SetNamespaceArchived(ctx, admin, "restore-team", ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: 2})
	mustGovernance(t, e)
	_, _, e = s.DownloadRelease(ctx, member, "team", "lifecycle-check", "1.0.0")
	mustGovernance(t, e)
	v, e = s.SetNamespaceArchived(ctx, admin, "archive-again", ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: v.Revision, Archived: true})
	mustGovernance(t, e)
	purge := ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: v.Revision, ConfirmName: "wrong"}
	_, e = s.PurgeNamespace(ctx, admin, "wrong-confirm", purge)
	if !errors.Is(e, ErrInvalid) {
		t.Fatal("confirmation not enforced", e)
	}
	purge.ConfirmName = "team"
	_, e = s.PurgeNamespace(ctx, member, "member-purge", purge)
	if !errors.Is(e, ErrForbidden) {
		t.Fatal("namespace admin purge", e)
	}
	_, e = s.PurgeNamespace(ctx, admin, "purge-team", purge)
	mustGovernance(t, e)
	_, e = s.PurgeNamespace(ctx, admin, "purge-team", purge)
	mustGovernance(t, e)
	_, _, e = s.DownloadRelease(ctx, admin, "shared", "lifecycle-check", "1.0.0")
	mustGovernance(t, e)
	for table, want := range map[string]int{"users": 2, "groups": 1, "group_members": 1, "namespace_grants": 0, "comments": 0, "reactions": 0, "evaluation_evidence": 0, "namespace_tombstones": 1} {
		var n int
		mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM "+table).Scan(&n))
		if n != want {
			t.Fatalf("%s=%d want %d", table, n, want)
		}
	}
	_, e = s.PutNamespace(ctx, admin, "reuse-deleted", PutNamespace{Name: "team"})
	if !errors.Is(e, ErrConflict) {
		t.Fatal("deleted coordinate reused", e)
	}
	var cached int
	mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM mutations WHERE key IN ('member-publish','comment-root','comment-reply')").Scan(&cached))
	if cached != 0 {
		t.Fatal("deleted content retained in response cache")
	}
	var audits int
	mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM audit WHERE action='namespace.purge'").Scan(&audits))
	if audits != 1 {
		t.Fatal("audit not retained or replay duplicated", audits)
	}
	v, e = s.SetNamespaceArchived(ctx, admin, "archive-shared", ChangeNamespaceLifecycle{Name: "shared", ExpectedRevision: 1, Archived: true})
	mustGovernance(t, e)
	_, e = s.PurgeNamespace(ctx, admin, "purge-shared", ChangeNamespaceLifecycle{Name: "shared", ExpectedRevision: v.Revision, ConfirmName: "shared"})
	mustGovernance(t, e)
	if _, e = os.Stat(filepath.Join(s.root, "packages", release.Digest[7:]+".zip")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("unreferenced package retained", e)
	}
}
func TestNamespaceCleanupFailureCanRetry(t *testing.T) {
	s, session, _, r := backupFixture(t)
	ctx := context.Background()
	v, e := s.SetNamespaceArchived(ctx, session.Identity, "archive-failure", ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: 1, Archived: true})
	mustGovernance(t, e)
	p := filepath.Join(s.root, "packages", r.Digest[7:]+".zip")
	mustGovernance(t, os.Remove(p))
	mustGovernance(t, os.Mkdir(p, 0700))
	mustGovernance(t, os.WriteFile(filepath.Join(p, "block"), []byte("fixture"), 0600))
	cmd := ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: v.Revision, ConfirmName: "team"}
	_, e = s.PurgeNamespace(ctx, session.Identity, "purge-retry", cmd)
	if e == nil {
		t.Fatal("cleanup error hidden")
	}
	var pending int
	mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM package_cleanup").Scan(&pending))
	if pending != 1 {
		t.Fatal("lost retry queue")
	}
	mustGovernance(t, os.Remove(filepath.Join(p, "block")))
	_, e = s.PurgeNamespace(ctx, session.Identity, "purge-retry", cmd)
	mustGovernance(t, e)
	mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM package_cleanup").Scan(&pending))
	if pending != 0 {
		t.Fatal("retry did not finish")
	}
}

func TestArchivedReviewQueueAndBackupPreserveState(t *testing.T) {
	s, session, cmd, original := backupFixture(t)
	ctx := context.Background()
	admin := session.Identity
	_, e := s.PutNamespace(ctx, admin, "enable-review", PutNamespace{Name: "team", ExpectedRevision: 1, ReviewRequired: true})
	mustGovernance(t, e)
	member := governanceUser(t, s, admin, "author")
	mustGovernance(t, s.SetNamespaceGrant(ctx, admin, "author-grant", SetNamespaceGrant{NamespaceGrant: NamespaceGrant{Namespace: "team", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: member.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER}, Granted: true}))
	cmd.Version = "1.0.1"
	pending, e := s.SubmitRelease(ctx, member, "pending-release", cmd)
	mustGovernance(t, e)
	tasks, e := s.ListReviewTasks(ctx, admin, Page{})
	mustGovernance(t, e)
	if tasks.Total != 1 {
		t.Fatal(tasks)
	}
	_, e = s.SetNamespaceArchived(ctx, admin, "archive-backup", ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: 2, Archived: true})
	mustGovernance(t, e)
	tasks, e = s.ListReviewTasks(ctx, admin, Page{})
	mustGovernance(t, e)
	if tasks.Total != 0 {
		t.Fatal("archived task remained")
	}
	_, e = s.ReviewRelease(ctx, admin, "archived-review", ReviewRelease{ReleaseID: pending.ID, Approve: true})
	if !errors.Is(e, ErrNotFound) {
		t.Fatal("archived review allowed", e)
	}
	_, e = s.SubmitRelease(ctx, member, "pending-release", cmd)
	if !errors.Is(e, ErrNotFound) {
		t.Fatal("submission cache bypass", e)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	_, e = s.Backup(ctx, backup)
	mustGovernance(t, e)
	root := filepath.Join(t.TempDir(), "recovery")
	_, e = RestoreBackup(ctx, backup, root)
	mustGovernance(t, e)
	restored, e := Open(root)
	mustGovernance(t, e)
	defer restored.Close()
	login, e := restored.Login(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	state, e := restored.GetNamespaceLifecycle(ctx, login.Identity, "team")
	mustGovernance(t, e)
	if !state.Archived || state.Revision != 3 || state.Releases != 2 || state.Grants != 1 {
		t.Fatal(state)
	}
	_, e = restored.SetNamespaceArchived(ctx, login.Identity, "restore-backup", ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: 3})
	mustGovernance(t, e)
	tasks, e = restored.ListReviewTasks(ctx, login.Identity, Page{})
	mustGovernance(t, e)
	if tasks.Total != 1 {
		t.Fatal("pending state not restored")
	}
	_, data, e := restored.DownloadRelease(ctx, login.Identity, "team", original.Name, "1.0.0")
	mustGovernance(t, e)
	if packageformat.Digest(data) != original.Digest {
		t.Fatal("archive changed package")
	}
}
