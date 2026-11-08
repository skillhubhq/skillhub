package hub

import (
	"context"
	"errors"
	"fmt"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	c "github.com/skillhubhq/skillhub/internal/community"
	"github.com/skillhubhq/skillhub/internal/hubpolicy"
	"path/filepath"
	"sync"
	"testing"
)

func TestCommunityPolicyAndModeration(t *testing.T) {
	s, session, cmd, _ := backupFixture(t)
	testCommunityPolicy(t, s, session.Identity, cmd)
}
func testCommunityPolicy(t *testing.T, s *Store, admin Identity, cmd SubmitRelease) {
	ctx := context.Background()
	p := hubpolicy.Policy{Mode: hubpolicy.Community, Limits: hubpolicy.Defaults()}
	p.Limits.SubmissionsPerDay = 1
	p.Limits.CommentsPerDay = 1
	p.Limits.ReportsPerDay = 1
	mustGovernance(t, s.ConfigurePolicy(p))
	session, e := s.FederatedLogin(ctx, "https://identity.example", "community-author", "Community author", true)
	mustGovernance(t, e)
	u := session.Identity
	participation, e := s.GetParticipation(ctx, u)
	mustGovernance(t, e)
	if participation.PersonalNamespace == "" || u.Administrator {
		t.Fatal("missing personal namespace or excessive privilege")
	}
	again, e := s.FederatedLogin(ctx, "https://identity.example", "community-author", "New upstream name", true)
	mustGovernance(t, e)
	if again.Identity.ID != u.ID {
		t.Fatal("identity changed")
	}
	profile, e := s.GetProfile(ctx, u, u.ID)
	mustGovernance(t, e)
	if profile.DisplayName != "Community author" {
		t.Fatal("upstream overwrote profile")
	}
	if e = s.SetNamespaceGrant(ctx, u, "self-admin", SetNamespaceGrant{Namespace: participation.PersonalNamespace, Kind: 1, SubjectID: u.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN, Granted: true}); !errors.Is(e, ErrForbidden) {
		t.Fatal("personal maintainer escalated", e)
	}
	cmd.Namespace = participation.PersonalNamespace
	cmd.Visibility = taxonomy.Visibility_VISIBILITY_PUBLIC
	cmd.GroupIDs = nil
	cmd.Version = "1.0.0"
	r, e := s.SubmitRelease(ctx, u, "community-submit", cmd)
	mustGovernance(t, e)
	if r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING {
		t.Fatal("community publication bypassed review")
	}
	replay, e := s.SubmitRelease(ctx, u, "community-submit", cmd)
	mustGovernance(t, e)
	if replay.ID != r.ID {
		t.Fatal("quota broke idempotent retry")
	}
	cmd.Version = "1.0.1"
	if _, e = s.SubmitRelease(ctx, u, "community-over-quota", cmd); !errors.Is(e, ErrQuota) {
		t.Fatal("daily quota bypass", e)
	}
	if _, e = s.ReviewRelease(ctx, u, "self-approve", ReviewRelease{ReleaseID: r.ID, Approve: true}); e == nil {
		t.Fatal("self review allowed")
	}
	_, e = s.ReviewRelease(ctx, admin, "independent-approve", ReviewRelease{ReleaseID: r.ID, Approve: true})
	mustGovernance(t, e)
	comment, e := s.CreateComment(ctx, u, "community-comment", c.CommentChange{ReleaseID: r.ID, Body: "Please check this example."})
	mustGovernance(t, e)
	if _, e = s.CreateComment(ctx, u, "community-comment-more", c.CommentChange{ReleaseID: r.ID, Body: "Another comment"}); !errors.Is(e, ErrQuota) {
		t.Fatal("comment quota bypass", e)
	}
	id, e := s.ReportContent(ctx, u, "community-report", ReportContent{ReleaseID: r.ID, CommentID: comment.CommentID, Reason: "Inappropriate content"})
	mustGovernance(t, e)
	duplicate, e := s.ReportContent(ctx, u, "duplicate-report", ReportContent{ReleaseID: r.ID, CommentID: comment.CommentID, Reason: "Same target"})
	mustGovernance(t, e)
	if duplicate != id {
		t.Fatal("duplicate report")
	}
	if _, e = s.ListReports(ctx, u, false, Page{}); !errors.Is(e, ErrForbidden) {
		t.Fatal("reporter disclosed private moderation", e)
	}
	if e = s.ResolveReport(ctx, u, "unauthorized-resolve", ResolveReport{ID: id, Action: 1, Note: "Ignore"}); !errors.Is(e, ErrForbidden) {
		t.Fatal("reporter resolved own report", e)
	}
	queue, e := s.ListReports(ctx, admin, false, Page{Size: 1})
	mustGovernance(t, e)
	if len(queue.Items) != 1 || queue.Items[0].Content != comment.Body {
		t.Fatal("moderation queue", queue)
	}
	mustGovernance(t, s.ResolveReport(ctx, admin, "resolve-comment", ResolveReport{ID: id, Action: 2, Note: "Removed after review"}))
	mustGovernance(t, s.ResolveReport(ctx, admin, "resolve-comment", ResolveReport{ID: id, Action: 2, Note: "Removed after review"}))
	comments, e := s.ListComments(ctx, u, r.ID, "", Page{})
	mustGovernance(t, e)
	if len(comments.Items) != 1 || !comments.Items[0].Deleted || comments.Items[0].Body != "" {
		t.Fatal("comment still visible")
	}
	if _, e = s.ReportContent(ctx, u, "community-report-more", ReportContent{ReleaseID: r.ID, Reason: "Too many reports"}); !errors.Is(e, ErrQuota) {
		t.Fatal("report quota bypass", e)
	}
	id, e = s.ReportContent(ctx, admin, "report-release", ReportContent{ReleaseID: r.ID, Reason: "Invalid instructions"})
	mustGovernance(t, e)
	mustGovernance(t, s.ResolveReport(ctx, admin, "resolve-release", ResolveReport{ID: id, Action: 2, Note: "Withdrawn after review"}))
	preview, _, e := s.GetSubmission(ctx, u, r.ID)
	mustGovernance(t, e)
	if preview.ReviewReason != "Withdrawn by a Hub administrator following a content report." {
		t.Fatal("moderation leaked private decision note to author", preview.ReviewReason)
	}
	resolvedReports, e := s.ListReports(ctx, admin, true, Page{})
	mustGovernance(t, e)
	if len(resolvedReports.Items) != 2 || resolvedReports.Items[0].Resolution != "remove_content: Withdrawn after review" {
		t.Fatal("private decision note was not retained for administrators")
	}
	if _, e = s.GetDiscussion(ctx, Identity{}, r.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("withdrawn content visible", e)
	}
	participation, e = s.GetParticipation(ctx, u)
	mustGovernance(t, e)
	if participation.Versions != 1 || participation.StorageBytes != int64(len(cmd.Archive)) {
		t.Fatal("withdrawal reset quota", participation)
	}
	// 模式切换不抹除账号、授权、配额记录或待审状态。
	mustGovernance(t, s.ConfigurePolicy(hubpolicy.Policy{Mode: hubpolicy.Enterprise}))
	if _, e = s.FederatedLogin(ctx, "https://identity.example", "new-enterprise", "Pending", false); !errors.Is(e, ErrPendingApproval) {
		t.Fatal(e)
	}
	existing, e := s.FederatedLogin(ctx, "https://identity.example", "community-author", "Author", false)
	mustGovernance(t, e)
	if existing.Identity.ID != u.ID {
		t.Fatal("mode switch replaced account")
	}
	mustGovernance(t, s.ConfigurePolicy(p))
	if _, e = s.FederatedLogin(ctx, "https://identity.example", "new-enterprise", "Pending", true); !errors.Is(e, ErrPendingApproval) {
		t.Fatal("community mode approved an existing pending request", e)
	}
	p.Limits.Users = 1
	mustGovernance(t, s.ConfigurePolicy(p))
	if _, e = s.FederatedLogin(ctx, "https://identity.example", "over-account-limit", "More", true); !errors.Is(e, ErrQuota) {
		t.Fatal("account limit bypass", e)
	}
	if _, e = s.FederatedLogin(ctx, "https://identity.example", "community-author", "Author", true); e != nil {
		t.Fatal("account limit locked out existing member", e)
	}
}
func TestCommunityBackupPreservesParticipation(t *testing.T) {
	s, session, _, r := backupFixture(t)
	ctx := context.Background()
	root := s.root
	registry := s.RegistryID()
	var e error
	mustGovernance(t, s.Close())
	s, e = Open(root)
	mustGovernance(t, e)
	defer s.Close()
	p := hubpolicy.Policy{Mode: hubpolicy.Community, Limits: hubpolicy.Defaults()}
	mustGovernance(t, s.ConfigurePolicy(p))
	user, e := s.FederatedLogin(ctx, "https://identity.example", "recover", "Recover", true)
	mustGovernance(t, e)
	part, e := s.GetParticipation(ctx, user.Identity)
	mustGovernance(t, e)
	_, e = s.ReportContent(ctx, session.Identity, "backup-report", ReportContent{ReleaseID: r.ID, Reason: "Review fixture"})
	mustGovernance(t, e)
	backup := filepath.Join(t.TempDir(), "backup")
	_, e = s.Backup(ctx, backup)
	mustGovernance(t, e)
	restored := filepath.Join(t.TempDir(), "restored")
	_, e = RestoreBackup(ctx, backup, restored)
	mustGovernance(t, e)
	copy, e := Open(restored)
	mustGovernance(t, e)
	defer copy.Close()
	mustGovernance(t, copy.ConfigurePolicy(p))
	if copy.RegistryID() != registry {
		t.Fatal("registry changed")
	}
	recovered, e := copy.FederatedLogin(ctx, "https://identity.example", "recover", "Recover", false)
	mustGovernance(t, e)
	if recovered.Identity.ID != user.Identity.ID {
		t.Fatal("identity changed")
	}
	next, e := copy.GetParticipation(ctx, recovered.Identity)
	mustGovernance(t, e)
	if next.PersonalNamespace != part.PersonalNamespace {
		t.Fatal("namespace lost")
	}
	reports, e := copy.ListReports(ctx, session.Identity, false, Page{})
	mustGovernance(t, e)
	if len(reports.Items) != 1 {
		t.Fatal("reports lost")
	}
}
func TestCommunityConcurrentIdentityAndQuota(t *testing.T) {
	s, session, cmd, _ := backupFixture(t)
	ctx := context.Background()
	p := hubpolicy.Policy{Mode: hubpolicy.Community, Limits: hubpolicy.Defaults()}
	p.Limits.SubmissionsPerDay = 1
	mustGovernance(t, s.ConfigurePolicy(p))
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := s.FederatedLogin(ctx, "https://identity.example", "one-person", "Person", true)
			if e != nil {
				t.Error(e)
				return
			}
			ids <- v.Identity.ID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for v := range ids {
		if id != "" && id != v {
			t.Fatal("concurrent duplicate identities")
		}
		id = v
	}
	part, e := s.GetParticipation(ctx, Identity{ID: id})
	mustGovernance(t, e)
	cmd.Namespace = part.PersonalNamespace
	cmd.Visibility = 1
	cmd.GroupIDs = nil
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v := cmd
			v.Version = fmt.Sprintf("1.0.%d", i)
			_, e := s.SubmitRelease(ctx, Identity{ID: id}, fmt.Sprintf("parallel-%d", i), v)
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if !errors.Is(e, ErrQuota) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("quota race", success)
	}
	_ = session
}

func TestExternalAvatarNeverOverwritesUserEdits(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	u, e := s.FederatedLogin(ctx, "https://identity.example", "avatar-person", "Initial", true)
	mustGovernance(t, e)
	mustGovernance(t, s.SyncExternalAvatar(ctx, u.Identity, avatarImage(t, 16, 16)))
	p, e := s.GetProfile(ctx, u.Identity, u.Identity.ID)
	mustGovernance(t, e)
	if p.Avatar == "" || p.Revision != 0 {
		t.Fatal("initial avatar missing")
	}
	_, e = s.UpdateAvatar(ctx, u.Identity, "clear-avatar", c.AvatarChange{ExpectedRevision: p.Revision})
	mustGovernance(t, e)
	mustGovernance(t, s.SyncExternalAvatar(ctx, u.Identity, avatarImage(t, 32, 32)))
	p, e = s.GetProfile(ctx, u.Identity, u.Identity.ID)
	mustGovernance(t, e)
	if p.Avatar != "" {
		t.Fatal("user-cleared avatar overwritten")
	}
	if e = s.SyncExternalAvatar(ctx, u.Identity, []byte("not an image")); !errors.Is(e, ErrInvalid) {
		t.Fatal("invalid image accepted", e)
	}
}
func TestCommunityByteAndVersionQuotas(t *testing.T) {
	for _, kind := range []string{"author-bytes", "package-bytes", "versions"} {
		t.Run(kind, func(t *testing.T) {
			s, session, cmd, _ := backupFixture(t)
			ctx := context.Background()
			p := hubpolicy.Policy{Mode: hubpolicy.Community, Limits: hubpolicy.Defaults()}
			switch kind {
			case "author-bytes":
				p.Limits.BytesPerUser = int64(len(cmd.Archive))
			case "package-bytes":
				p.Limits.PackageBytes = 1
			case "versions":
				p.Limits.VersionsPerUser = 1
			}
			mustGovernance(t, s.ConfigurePolicy(p))
			cmd.Version = "2.0.0"
			if _, e := s.SubmitRelease(ctx, session.Identity, "quota-boundary", cmd); !errors.Is(e, ErrQuota) {
				t.Fatal("quota accepted", e)
			}
		})
	}
}
