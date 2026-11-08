package hub

import (
	"context"
	"testing"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	c "github.com/skillhubhq/skillhub/internal/community"
	"github.com/skillhubhq/skillhub/internal/hubdb"
)

func TestSkillInteractionsDefaultVersionAndSharedAccess(t *testing.T) {
	testSkillInteractionsDefaultVersionAndSharedAccess(t, hubdb.Config{})
}
func TestPostgresSkillInteractionsDefaultVersionAndSharedAccess(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testSkillInteractionsDefaultVersionAndSharedAccess(t, cfg)
}
func testSkillInteractionsDefaultVersionAndSharedAccess(t *testing.T, cfg hubdb.Config) {
	s, session, cmd, first := identityFixture(t, cfg)
	ctx := context.Background()
	u := session.Identity
	_, err := s.SetReaction(ctx, u, "identity-like", c.ReactionChange{ReleaseID: first.ID, Active: true})
	mustGovernance(t, err)
	_, err = s.SetReaction(ctx, u, "identity-save", c.ReactionChange{ReleaseID: first.ID, Bookmark: true, Active: true})
	mustGovernance(t, err)
	comment, err := s.CreateComment(ctx, u, "identity-comment", c.CommentChange{ReleaseID: first.ID, Body: "Useful across updates"})
	mustGovernance(t, err)
	cmd.Version = "2.0.0"
	next, err := s.SubmitRelease(ctx, u, "identity-next", cmd)
	mustGovernance(t, err)
	cmd.Version = "1.1.0"
	_, err = s.SubmitRelease(ctx, u, "identity-older-semver", cmd)
	mustGovernance(t, err)
	cmd.Version = "3.0.0-beta.1"
	_, err = s.SubmitRelease(ctx, u, "identity-prerelease", cmd)
	mustGovernance(t, err)
	skill, err := s.GetSkill(ctx, u, first.Namespace, first.Name)
	mustGovernance(t, err)
	if skill.LatestVersion != "2.0.0" {
		t.Fatal("default is not highest stable", skill.LatestVersion)
	}
	d, err := s.GetDiscussion(ctx, u, next.ID)
	mustGovernance(t, err)
	if !d.Liked || !d.Bookmarked || d.LikeCount != 1 || d.CommentCount != 1 {
		t.Fatal("interactions lost", d)
	}
	_, err = s.SetReaction(ctx, u, "identity-like-again", c.ReactionChange{ReleaseID: next.ID, Active: true})
	mustGovernance(t, err)
	contributions, err := s.ListContributions(ctx, u, "", "", Page{}, true)
	mustGovernance(t, err)
	if len(contributions.Items) != 1 || contributions.Items[0].Version != "2.0.0" {
		t.Fatal("saved versions duplicated", contributions)
	}
	replies, err := s.ListComments(ctx, u, next.ID, "", Page{})
	mustGovernance(t, err)
	if len(replies.Items) != 1 || replies.Items[0].CommentID != comment.CommentID {
		t.Fatal("discussion not shared")
	}
	_, err = s.YankRelease(ctx, u, "identity-yank", YankRelease{ReleaseID: first.ID, Reason: "Replaced"})
	mustGovernance(t, err)
	_, err = s.DeleteRelease(ctx, u, "identity-clean", DeleteRelease{ReleaseID: first.ID, ConfirmCoordinate: first.Namespace + "/" + first.Name + "@" + first.Version})
	mustGovernance(t, err)
	d, err = s.GetDiscussion(ctx, u, next.ID)
	mustGovernance(t, err)
	if d.LikeCount != 1 || d.CommentCount != 1 {
		t.Fatal("cleanup erased community", d)
	}
	cmd.Version = "2.1.0"
	cmd.Visibility = taxonomy.Visibility_VISIBILITY_PUBLIC
	cmd.GroupIDs = nil
	public, err := s.SubmitRelease(ctx, u, "identity-public", cmd)
	mustGovernance(t, err)
	d, err = s.GetDiscussion(ctx, Identity{}, public.ID)
	mustGovernance(t, err)
	if d.CommentCount != 1 || d.LikeCount != 1 {
		t.Fatal("skill access not unified", d)
	}
	_, _, err = s.DownloadRelease(ctx, Identity{}, next.Namespace, next.Name, next.Version)
	mustGovernance(t, err)
}

func TestVersionOrderUsesStableAndSemanticPrecedence(t *testing.T) {
	values := []string{"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "2.0.0-beta", "1.0.0", "1.9.0", "1.10.0", "2.0.0"}
	for i := 1; i < len(values); i++ {
		if versionOrder(values[i-1]) >= versionOrder(values[i]) {
			t.Fatalf("wrong order: %s %s", values[i-1], values[i])
		}
	}
}

func TestUnpublishedRevisionMetadataAndReplyNotifications(t *testing.T) {
	testUnpublishedRevisionMetadataAndReplyNotifications(t, hubdb.Config{})
}
func TestPostgresUnpublishedRevisionMetadataAndReplyNotifications(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testUnpublishedRevisionMetadataAndReplyNotifications(t, cfg)
}
func testUnpublishedRevisionMetadataAndReplyNotifications(t *testing.T, cfg hubdb.Config) {
	s, session, cmd, first := identityFixture(t, cfg)
	ctx := context.Background()
	u := session.Identity
	reviewer := governanceUser(t, s, u, "reviewer")
	mustGovernance(t, s.SetNamespaceGrant(ctx, u, "identity-grant-review", SetNamespaceGrant{Namespace: cmd.Namespace, Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: reviewer.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, Granted: true}))
	_, err := s.PutNamespace(ctx, u, "identity-enable-review", PutNamespace{Name: cmd.Namespace, ReviewRequired: true, ExpectedRevision: 1})
	mustGovernance(t, err)
	cmd.Version = "2.0.0"
	cmd.Visibility = taxonomy.Visibility_VISIBILITY_MEMBERS
	cmd.GroupIDs = nil
	old, err := s.SubmitRelease(ctx, u, "identity-revision-one", cmd)
	mustGovernance(t, err)
	revised, err := s.SubmitRelease(ctx, u, "identity-revision-two", cmd)
	mustGovernance(t, err)
	if old.ID == revised.ID {
		t.Fatal("revision reused immutable identity")
	}
	if _, err = s.ReviewRelease(ctx, reviewer, "identity-stale-review", ReviewRelease{ReleaseID: old.ID, Approve: true}); err == nil {
		t.Fatal("superseded revision approved")
	}
	_, err = s.ReviewRelease(ctx, reviewer, "identity-approve", ReviewRelease{ReleaseID: revised.ID, Approve: true})
	mustGovernance(t, err)
	if _, err = s.SubmitRelease(ctx, u, "identity-overwrite-published", cmd); err == nil {
		t.Fatal("published version overwritten")
	}
	skill, err := s.GetSkill(ctx, u, first.Namespace, first.Name)
	mustGovernance(t, err)
	_, err = s.UpdateSkillMetadata(ctx, u, "identity-edit-details", SkillMetadataChange{Namespace: first.Namespace, Name: first.Name, ExpectedRevision: skill.Revision, DisplayNameEN: "Clear name", Description: "A better description", Icon: skill.Icon})
	mustGovernance(t, err)
	updated, err := s.GetSkill(ctx, u, first.Namespace, first.Name)
	mustGovernance(t, err)
	if updated.DisplayNameEN != "Clear name" || updated.LatestVersion != "2.0.0" {
		t.Fatal("metadata edit changed version or failed", updated)
	}
	_, err = s.UpdateSkillMetadata(ctx, reviewer, "identity-forbidden-edit", SkillMetadataChange{Namespace: first.Namespace, Name: first.Name, ExpectedRevision: updated.Revision, Description: "Forbidden"})
	if err == nil {
		t.Fatal("reviewer edited skill metadata")
	}
	_, err = s.UpdateSkillMetadata(ctx, u, "identity-stale-edit", SkillMetadataChange{Namespace: first.Namespace, Name: first.Name, ExpectedRevision: skill.Revision, Description: "Stale"})
	if err == nil {
		t.Fatal("stale metadata overwrote current details")
	}
	root, err := s.CreateComment(ctx, u, "identity-question", c.CommentChange{ReleaseID: first.ID, Body: "Question"})
	mustGovernance(t, err)
	_, err = s.CreateComment(ctx, reviewer, "identity-answer", c.CommentChange{ReleaseID: revised.ID, ParentID: root.CommentID, Body: "Answer"})
	mustGovernance(t, err)
	notes, err := s.ListNotifications(ctx, u, Page{})
	mustGovernance(t, err)
	if len(notes.Items) != 2 {
		t.Fatal("missing review or reply notice", notes)
	}
	unread, err := s.UnreadNotifications(ctx, u)
	mustGovernance(t, err)
	if unread != 2 {
		t.Fatal(unread)
	}
	mustGovernance(t, s.MarkAllNotificationsRead(ctx, u))
	unread, err = s.UnreadNotifications(ctx, u)
	mustGovernance(t, err)
	if unread != 0 {
		t.Fatal("unread not cleared")
	}
}

func TestUnavailableBookmarkCanBeRemovedWithoutContentAccess(t *testing.T) {
	testUnavailableBookmarkCanBeRemovedWithoutContentAccess(t, hubdb.Config{})
}
func TestPostgresUnavailableBookmarkCanBeRemovedWithoutContentAccess(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testUnavailableBookmarkCanBeRemovedWithoutContentAccess(t, cfg)
}
func testUnavailableBookmarkCanBeRemovedWithoutContentAccess(t *testing.T, cfg hubdb.Config) {
	s, session, cmd, _ := identityFixture(t, cfg)
	ctx := context.Background()
	admin := session.Identity
	originalGroups := cmd.GroupIDs
	cmd.Version = "2.0.0"
	cmd.Visibility = taxonomy.Visibility_VISIBILITY_PUBLIC
	cmd.GroupIDs = nil
	public, err := s.SubmitRelease(ctx, admin, "identity-public", cmd)
	mustGovernance(t, err)
	reader := governanceUser(t, s, admin, "reader")
	_, err = s.SetReaction(ctx, reader, "identity-save", c.ReactionChange{ReleaseID: public.ID, Active: true, Bookmark: true})
	mustGovernance(t, err)
	cmd.Version = "3.0.0"
	cmd.Visibility = taxonomy.Visibility_VISIBILITY_GROUPS
	cmd.GroupIDs = originalGroups
	_, err = s.SubmitRelease(ctx, admin, "identity-restrict", cmd)
	mustGovernance(t, err)
	saved, err := s.ListContributions(ctx, reader, "", "", Page{}, true)
	mustGovernance(t, err)
	if len(saved.Items) != 1 || !saved.Items[0].Unavailable || saved.Items[0].ReleaseID != "" || saved.Items[0].Description != "" {
		t.Fatal("missing or exposed unavailable bookmark", saved)
	}
	mustGovernance(t, s.RemoveBookmark(ctx, reader, "identity-remove", public.Namespace, public.Name))
	saved, err = s.ListContributions(ctx, reader, "", "", Page{}, true)
	mustGovernance(t, err)
	if len(saved.Items) != 0 {
		t.Fatal("unavailable bookmark not removed")
	}
}

func identityFixture(t *testing.T, cfg hubdb.Config) (*Store, Session, SubmitRelease, Release) {
	t.Helper()
	s, e := OpenDatabase(t.TempDir(), cfg)
	mustGovernance(t, e)
	t.Cleanup(func() { s.Close() })
	_, e = s.Bootstrap(context.Background(), "admin", "a-strong-test-password")
	mustGovernance(t, e)
	return backupFixtureStore(t, s)
}
