package hub

import (
	"context"
	"errors"
	"testing"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/hubpolicy"
)

func TestMemberSubmissions(t *testing.T) { testMemberSubmissions(t, hubdb.Config{}) }
func TestPostgresMemberSubmissions(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testMemberSubmissions(t, cfg)
}

func testMemberSubmissions(t *testing.T, cfg hubdb.Config) {
	ctx := context.Background()
	s, err := OpenDatabase(t.TempDir(), cfg)
	mustGovernance(t, err)
	defer s.Close()
	admin, err := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, err)
	member := governanceUser(t, s, admin, "member")
	_, err = s.PutNamespace(ctx, admin, "create-team", PutNamespace{Name: "team"})
	mustGovernance(t, err)
	cmd := SubmitRelease{Namespace: "team", Version: "1.0.0", Archive: governanceArchive(t, "Review the proposed change."), Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}
	if _, err = s.SubmitRelease(ctx, member, "default-denied", cmd); !errors.Is(err, ErrForbidden) {
		t.Fatal("default policy allowed submission", err)
	}
	policy := hubpolicy.Policy{Mode: hubpolicy.Enterprise, Admission: hubpolicy.AdmissionAutomatic, Submission: hubpolicy.SubmissionMembers}
	mustGovernance(t, s.ConfigurePolicy(policy))
	newcomer, err := s.FederatedLogin(ctx, "https://identity.example", "new-member", "New member", true)
	mustGovernance(t, err)
	for _, user := range []Identity{member, newcomer.Identity} {
		access, err := s.NamespaceAccess(ctx, user, "team")
		mustGovernance(t, err)
		if !access.CanPublish || access.CanManage || access.CanReview || access.CanMaintain || user.Administrator {
			t.Fatalf("incorrect member access: %+v", access)
		}
		for _, submission := range []bool{false, true} {
			spaces, err := s.listNamespaces(ctx, user, Page{}, submission)
			mustGovernance(t, err)
			if len(spaces.Items) != 1 || spaces.Items[0].Name != "team" || spaces.Items[0].ReviewRequired != submission {
				t.Fatalf("missing submission space: %+v", spaces)
			}
		}
	}
	if _, err = s.SubmitRelease(ctx, Identity{}, "anonymous", cmd); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("anonymous submission", err)
	}
	r, err := s.SubmitRelease(ctx, member, "member-submit", cmd)
	mustGovernance(t, err)
	if r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING {
		t.Fatal("member bypassed review in direct-publish namespace")
	}
	if _, _, err = s.GetSubmission(ctx, member, r.ID); err != nil {
		t.Fatal("cannot read own submission", err)
	}
	if _, err = s.GetScanReport(ctx, member, r.ID); err != nil {
		t.Fatal("cannot read own scan", err)
	}
	if _, err = s.ImportEvaluation(ctx, member, "own-evidence", r.ID, evaluationFixture(r.Digest).Canonical()); err != nil {
		t.Fatal("cannot import own evidence", err)
	}
	if evidence, err := s.ListEvaluations(ctx, member, r.ID); err != nil || !evidence.CanManage || len(evidence.Evidence) != 1 {
		t.Fatal("cannot read own evidence", err)
	}
	_, _, err = s.CreateAccessToken(ctx, member, "Submit skills", "publish", "team", 1)
	mustGovernance(t, err)
	queue, err := s.ListSubmissions(ctx, member, "team", Page{}, SubmissionFilter{Mine: true})
	mustGovernance(t, err)
	if len(queue.Items) != 1 {
		t.Fatal("missing own submission")
	}
	other := newcomer.Identity
	if _, _, err = s.GetSubmission(ctx, other, r.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("another member read unpublished content", err)
	}
	if _, err = s.GetScanReport(ctx, other, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("another member read scan", err)
	}
	if _, err = s.ListEvaluations(ctx, other, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("another member read evidence", err)
	}
	queue, err = s.ListSubmissions(ctx, other, "team", Page{}, SubmissionFilter{})
	mustGovernance(t, err)
	if len(queue.Items) != 0 {
		t.Fatal("another member read queue")
	}
	for _, version := range []string{"1.0.0", "1.1.0"} {
		attempt := cmd
		attempt.Version = version
		if _, err = s.SubmitRelease(ctx, other, "takeover-"+version, attempt); !errors.Is(err, ErrForbidden) {
			t.Fatal("member took over another skill", err)
		}
	}
	if _, err = s.ReviewRelease(ctx, member, "self-review", ReviewRelease{ReleaseID: r.ID, Approve: true}); !errors.Is(err, ErrForbidden) {
		t.Fatal("self-review allowed", err)
	}
	if _, err = s.ReviewRelease(ctx, other, "peer-review", ReviewRelease{ReleaseID: r.ID, Approve: true}); !errors.Is(err, ErrForbidden) {
		t.Fatal("member reviewed another submission", err)
	}
	if err = s.SetNamespaceGrant(ctx, member, "escalate", SetNamespaceGrant{Namespace: "team", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: member.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN, Granted: true}); !errors.Is(err, ErrForbidden) {
		t.Fatal("member escalated", err)
	}
	if _, err = s.PutNamespace(ctx, member, "edit-team", PutNamespace{Name: "team", ExpectedRevision: 1, ReviewRequired: false}); !errors.Is(err, ErrForbidden) {
		t.Fatal("member changed team", err)
	}
	_, err = s.ReviewRelease(ctx, admin, "approve-member", ReviewRelease{ReleaseID: r.ID, Approve: true})
	mustGovernance(t, err)
	if _, _, err = s.DownloadRelease(ctx, other, r.Namespace, r.Name, r.Version); err != nil {
		t.Fatal("published member skill inaccessible", err)
	}
	notifications, err := s.ListNotifications(ctx, member, Page{})
	mustGovernance(t, err)
	if len(notifications.Items) != 1 {
		t.Fatal("missing approval feedback")
	}
	cmd.Version = "1.1.0"
	updated, err := s.SubmitRelease(ctx, member, "update-own", cmd)
	mustGovernance(t, err)
	if _, err = s.CompareReleases(ctx, member, r.Namespace, r.Name, r.ID, updated.ID); err != nil {
		t.Fatal("cannot compare own update", err)
	}
	if _, err = s.CompareReleases(ctx, other, r.Namespace, r.Name, r.ID, updated.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("comparison exposed pending update", err)
	}
	if _, err = s.YankRelease(ctx, other, "withdraw-other", YankRelease{ReleaseID: updated.ID, Reason: "Withdraw"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("member withdrew another submission", err)
	}
	_, err = s.YankRelease(ctx, member, "withdraw-own", YankRelease{ReleaseID: updated.ID, Reason: "Revise"})
	mustGovernance(t, err)
	deleteCmd := DeleteRelease{ReleaseID: updated.ID, ConfirmCoordinate: updated.Namespace + "/" + updated.Name + "@" + updated.Version}
	if _, err = s.DeleteRelease(ctx, other, "delete-other", deleteCmd); !errors.Is(err, ErrForbidden) {
		t.Fatal("member deleted another submission", err)
	}
	_, err = s.DeleteRelease(ctx, member, "delete-own", deleteCmd)
	mustGovernance(t, err)
	_, err = s.DeleteRelease(ctx, member, "delete-own", deleteCmd)
	mustGovernance(t, err)
	// 成员投稿不改变受众读取规则，也不授予维护他人内容的权限。
	group, err := s.CreateGroup(ctx, admin, "private-group", "private")
	mustGovernance(t, err)
	_, err = s.PutNamespace(ctx, admin, "private-team", PutNamespace{Name: "private-team"})
	mustGovernance(t, err)
	private := cmd
	private.Namespace = "private-team"
	private.Visibility = taxonomy.Visibility_VISIBILITY_GROUPS
	private.GroupIDs = []string{group.ID}
	privateRelease, err := s.SubmitRelease(ctx, admin, "private-release", private)
	mustGovernance(t, err)
	if _, _, err = s.DownloadRelease(ctx, member, privateRelease.Namespace, privateRelease.Name, privateRelease.Version); !errors.Is(err, ErrNotFound) {
		t.Fatal("member policy bypassed audience", err)
	}
	// 个人空间的隔离独立于共享团队投稿策略。
	communityPolicy := policy
	communityPolicy.Mode = hubpolicy.Community
	communityPolicy.Limits = hubpolicy.Defaults()
	mustGovernance(t, s.ConfigurePolicy(communityPolicy))
	personalOwner, err := s.FederatedLogin(ctx, "https://identity.example", "personal-owner", "Personal author", true)
	mustGovernance(t, err)
	participation, err := s.GetParticipation(ctx, personalOwner.Identity)
	mustGovernance(t, err)
	mustGovernance(t, s.ConfigurePolicy(policy))
	personal := cmd
	personal.Namespace = participation.PersonalNamespace
	if _, err = s.SubmitRelease(ctx, member, "personal-denied", personal); !errors.Is(err, ErrForbidden) {
		t.Fatal("member entered personal space", err)
	}
	spaces, err := s.listNamespaces(ctx, member, Page{}, true)
	mustGovernance(t, err)
	for _, space := range spaces.Items {
		if space.Name == personal.Namespace {
			t.Fatal("personal space offered for submission")
		}
	}
	_, err = s.SetUserDisabled(ctx, admin, "disable-member", SetUserDisabled{ID: member.ID, Disabled: true})
	mustGovernance(t, err)
	if _, err = s.SubmitRelease(ctx, member, "disabled-submit", cmd); err == nil {
		t.Fatal("disabled member submitted")
	}
	_, err = s.SetUserDisabled(ctx, admin, "enable-member", SetUserDisabled{ID: member.ID})
	mustGovernance(t, err)
	_, err = s.SetNamespaceArchived(ctx, admin, "archive-team", ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: 1, Archived: true})
	mustGovernance(t, err)
	if _, err = s.SubmitRelease(ctx, member, "archived-submit", cmd); !errors.Is(err, ErrNotFound) {
		t.Fatal("archived team accepted submission", err)
	}
	_, err = s.SetNamespaceArchived(ctx, admin, "restore-team", ChangeNamespaceLifecycle{Name: "team", ExpectedRevision: 2, Archived: false})
	mustGovernance(t, err)
	policy.Submission = hubpolicy.SubmissionGranted
	mustGovernance(t, s.ConfigurePolicy(policy))
	if _, err = s.SubmitRelease(ctx, member, "member-submit", cmd); !errors.Is(err, ErrForbidden) {
		t.Fatal("revocation bypassed by replay", err)
	}
	if _, _, err = s.GetSubmission(ctx, member, r.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("revoked submission access persisted", err)
	}
}
