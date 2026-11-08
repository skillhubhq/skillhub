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
)

func TestReleaseCleanup(t *testing.T) { testReleaseCleanup(t, hubdb.Config{}) }
func TestPostgresReleaseCleanup(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testReleaseCleanup(t, cfg)
}
func testReleaseCleanup(t *testing.T, cfg hubdb.Config) {
	ctx := context.Background()
	s, e := OpenDatabase(t.TempDir(), cfg)
	mustGovernance(t, e)
	defer s.Close()
	admin, e := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	author := governanceUser(t, s, admin, "author")
	reader := governanceUser(t, s, admin, "reader")
	reviewer := governanceUser(t, s, admin, "reviewer")
	_, e = s.PutNamespace(ctx, admin, "namespace", PutNamespace{Name: "team", ReviewRequired: true})
	mustGovernance(t, e)
	grant := NamespaceGrant{Namespace: "team", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: author.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER}
	mustGovernance(t, s.SetNamespaceGrant(ctx, admin, "cleanup-grant", SetNamespaceGrant{NamespaceGrant: grant, Granted: true}))
	mustGovernance(t, s.SetNamespaceGrant(ctx, admin, "reviewer-grant", SetNamespaceGrant{NamespaceGrant: NamespaceGrant{Namespace: "team", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: reviewer.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER}, Granted: true}))
	cmd := SubmitRelease{Namespace: "team", Version: "1.0.0", Archive: governanceArchive(t, "Content to clean"), Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}
	r, e := s.SubmitRelease(ctx, author, "cleanup-submit", cmd)
	mustGovernance(t, e)
	cleanup := DeleteRelease{ReleaseID: r.ID, ConfirmCoordinate: "team/governance-check@1.0.0"}
	if _, e = s.DeleteRelease(ctx, author, "pending-delete", cleanup); !errors.Is(e, ErrConflict) {
		t.Fatal("pending deletion", e)
	}
	_, e = s.ReviewRelease(ctx, reviewer, "cleanup-reject", ReviewRelease{ReleaseID: r.ID, Reason: "Needs changes"})
	mustGovernance(t, e)
	for _, actor := range []Identity{reader, reviewer} {
		if _, e = s.DeleteRelease(ctx, actor, "forbidden", cleanup); !errors.Is(e, ErrForbidden) {
			t.Fatal("unauthorized deletion", e)
		}
	}
	if _, e = s.DeleteRelease(ctx, author, "cleanup-wrong", DeleteRelease{ReleaseID: r.ID, ConfirmCoordinate: "team/governance-check@2.0.0"}); !errors.Is(e, ErrInvalid) {
		t.Fatal("wrong confirmation", e)
	}
	cmd.Version = "1.0.1"
	shared, e := s.SubmitRelease(ctx, author, "cleanup-shared", cmd)
	mustGovernance(t, e)
	current, e := s.ListSubmissions(ctx, author, "team", Page{}, SubmissionFilter{Mine: true, ActiveOnly: true})
	mustGovernance(t, e)
	if len(current.Items) != 1 || current.Items[0].ID != shared.ID {
		t.Fatal("active filter includes rejected versions")
	}
	// 使用单独的已发布版本覆盖评论、回复、评测和共享文件清理。
	_, e = s.ReviewRelease(ctx, reviewer, "cleanup-approve", ReviewRelease{ReleaseID: shared.ID, Approve: true})
	mustGovernance(t, e)
	sharedCleanup := DeleteRelease{ReleaseID: shared.ID, ConfirmCoordinate: "team/governance-check@1.0.1"}
	if _, e = s.DeleteRelease(ctx, author, "published-delete", sharedCleanup); !errors.Is(e, ErrConflict) {
		t.Fatal("published deletion", e)
	}
	comment, e := s.CreateComment(ctx, author, "cleanup-comment", c.CommentChange{ReleaseID: shared.ID, Body: "Comment"})
	mustGovernance(t, e)
	_, e = s.CreateComment(ctx, author, "cleanup-reply", c.CommentChange{ReleaseID: shared.ID, ParentID: comment.CommentID, Body: "Reply"})
	mustGovernance(t, e)
	_, e = s.SetReaction(ctx, reader, "cleanup-reaction", c.ReactionChange{ReleaseID: shared.ID, Active: true})
	mustGovernance(t, e)
	_, e = s.db.Exec("INSERT INTO evaluation_evidence(id,release_id,digest,metadata,document,retracted) VALUES(?,?,?,?,?,0)", "evidence", shared.ID, shared.Digest, "{}", "{}")
	mustGovernance(t, e)
	_, e = s.YankRelease(ctx, author, "withdraw", YankRelease{ReleaseID: shared.ID, Reason: "Outdated"})
	mustGovernance(t, e)
	_, e = s.DeleteRelease(ctx, author, "cleanup-delete", cleanup)
	mustGovernance(t, e)
	_, e = s.DeleteRelease(ctx, author, "cleanup-delete", cleanup)
	mustGovernance(t, e)
	if _, e = os.Stat(filepath.Join(s.root, "packages", r.Digest[7:]+".zip")); e != nil {
		t.Fatal("shared blob removed", e)
	}
	cmd.Version = "1.0.0"
	if _, e = s.SubmitRelease(ctx, author, "cleanup-submit", cmd); !errors.Is(e, ErrConflict) {
		t.Fatal("old submit response replayed", e)
	}
	_, e = s.DeleteRelease(ctx, author, "delete-shared", sharedCleanup)
	mustGovernance(t, e)
	for _, table := range []string{"evaluation_evidence", "review_notifications", "release_scans"} {
		var count int
		mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM "+table).Scan(&count))
		if count != 0 {
			t.Fatalf("%s retained %d", table, count)
		}
	}
	var cached, audits int
	mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM mutations WHERE invalidated=0 AND key IN ('cleanup-submit','cleanup-shared','cleanup-comment','cleanup-reply','cleanup-reject','cleanup-approve')").Scan(&cached))
	if cached != 0 {
		t.Fatal("cached content retained", cached)
	}
	mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM audit WHERE action='release.delete'").Scan(&audits))
	if audits != 2 {
		t.Fatal("audit duplicated or lost", audits)
	}
	if _, e = os.Stat(filepath.Join(s.root, "packages", r.Digest[7:]+".zip")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("orphan blob retained", e)
	}
	var comments, reactions, versions int
	mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM comments").Scan(&comments))
	mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM reactions").Scan(&reactions))
	mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM releases WHERE cleaned=1 AND markdown=''").Scan(&versions))
	if comments != 2 || reactions != 1 || versions != 2 {
		t.Fatal("cleanup lost skill history", comments, reactions, versions)
	}
	cmd.Version = "1.0.1"
	if _, e = s.SubmitRelease(ctx, author, "reuse-published", cmd); !errors.Is(e, ErrConflict) {
		t.Fatal("published coordinate reused", e)
	}
	cmd.Version = "1.0.0"
	replacement, e := s.SubmitRelease(ctx, author, "revise-rejected", cmd)
	mustGovernance(t, e)
	if replacement.ID == r.ID {
		t.Fatal("revision reused identity")
	}
	mustGovernance(t, s.SetNamespaceGrant(ctx, admin, "cleanup-revoke", SetNamespaceGrant{NamespaceGrant: grant, Granted: false}))
	if _, e = s.DeleteRelease(ctx, author, "cleanup-delete", cleanup); !errors.Is(e, ErrForbidden) {
		t.Fatal("retry bypassed revocation", e)
	}
}

func TestCurrentSubmissionsStayWithinPublisher(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	session, e := s.Login(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	admin := session.Identity
	_, e = s.PutNamespace(ctx, admin, "namespace-current", PutNamespace{Name: "team"})
	mustGovernance(t, e)
	other := governanceUser(t, s, admin, "other")
	mustGovernance(t, s.SetNamespaceGrant(ctx, admin, "grant-other", SetNamespaceGrant{NamespaceGrant: NamespaceGrant{Namespace: "team", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: other.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER}, Granted: true}))
	cmd := SubmitRelease{Namespace: "team", Version: "1.0.0", Archive: governanceArchive(t, "Current submission"), Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}
	own, e := s.SubmitRelease(ctx, admin, "own-release", cmd)
	mustGovernance(t, e)
	cmd.Version = "1.0.1"
	_, e = s.SubmitRelease(ctx, other, "other-release", cmd)
	mustGovernance(t, e)
	result, e := s.ListSubmissions(ctx, admin, "team", Page{}, SubmissionFilter{Mine: true, ActiveOnly: true})
	mustGovernance(t, e)
	if len(result.Items) != 1 || result.Items[0].ID != own.ID {
		t.Fatal("another publisher hides my latest release")
	}
}

func TestReleaseCleanupCanRetryStorageFailure(t *testing.T) {
	s, session, _, r := backupFixture(t)
	ctx := context.Background()
	_, e := s.YankRelease(ctx, session.Identity, "withdraw-fixture", YankRelease{ReleaseID: r.ID, Reason: "Obsolete"})
	mustGovernance(t, e)
	path := filepath.Join(s.root, "packages", r.Digest[7:]+".zip")
	mustGovernance(t, os.Remove(path))
	mustGovernance(t, os.Mkdir(path, 0700))
	mustGovernance(t, os.WriteFile(filepath.Join(path, "block"), []byte("fixture"), 0600))
	cmd := DeleteRelease{ReleaseID: r.ID, ConfirmCoordinate: r.Namespace + "/" + r.Name + "@" + r.Version}
	if _, e = s.DeleteRelease(ctx, session.Identity, "cleanup-retry", cmd); e == nil {
		t.Fatal("storage failure hidden")
	}
	mustGovernance(t, os.Remove(filepath.Join(path, "block")))
	_, e = s.DeleteRelease(ctx, session.Identity, "cleanup-retry", cmd)
	mustGovernance(t, e)
	var pending int
	mustGovernance(t, s.db.QueryRow("SELECT count(*) FROM package_cleanup").Scan(&pending))
	if pending != 0 {
		t.Fatal("cleanup retry remained queued")
	}
}
