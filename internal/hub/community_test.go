package hub

import (
	"context"
	c "github.com/skillhubhq/skillhub/internal/community"
	"path/filepath"
	"testing"
)

func TestCommunityBackupRestoresProfilesThreadsAndPrivateBookmarks(t *testing.T) {
	s, session, _, release := backupFixture(t)
	ctx := context.Background()
	actor := session.Identity
	if _, err := s.UpdateProfile(ctx, actor, "community-profile", c.ProfileChange{DisplayName: "Registry author", Bio: "Recovery check"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTeam(ctx, actor, "community-team", c.TeamChange{Namespace: "team", DisplayName: "Platform team", Bio: "Shared skills"}); err != nil {
		t.Fatal(err)
	}
	comment, err := s.CreateComment(ctx, actor, "community-comment", c.CommentChange{ReleaseID: release.ID, Body: "Preserve this discussion"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateComment(ctx, actor, "community-reply", c.CommentChange{ReleaseID: release.ID, ParentID: comment.CommentID, Body: "Preserve this reply"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetReaction(ctx, actor, "community-save", c.ReactionChange{ReleaseID: release.ID, Bookmark: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if _, err = s.Backup(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "restored")
	if _, err = RestoreBackup(ctx, snapshot, root); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	next, err := restored.Login(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := restored.GetProfile(ctx, next.Identity, "")
	if err != nil || profile.DisplayName != "Registry author" {
		t.Fatal("profile was not restored", err)
	}
	team, err := restored.GetTeam(ctx, next.Identity, "team")
	if err != nil || team.DisplayName != "Platform team" {
		t.Fatal("team was not restored", err)
	}
	replies, err := restored.ListComments(ctx, next.Identity, release.ID, comment.CommentID, Page{})
	if err != nil || len(replies.Items) != 1 {
		t.Fatal("thread was not restored", err)
	}
	bookmarks, err := restored.ListContributions(ctx, next.Identity, "", "", Page{}, true)
	if err != nil || len(bookmarks.Items) != 1 {
		t.Fatal("private bookmark was not restored", err)
	}
}
