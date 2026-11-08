package hubhttp

import (
	"connectrpc.com/connect"
	"context"
	common "github.com/skillhubhq/skillhub/gen/api/common/v1"
	api "github.com/skillhubhq/skillhub/gen/api/community/v1"
	"github.com/skillhubhq/skillhub/gen/api/community/v1/communityv1connect"
	publishing "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"testing"
)

func TestCommunityAudienceRepliesReactionsAndProfiles(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	apiClient := communityv1connect.NewCommunityServiceClient(f.server.Client(), f.server.URL)
	memberID, member := f.user(t, "member")
	_, outsider := f.user(t, "outsider")
	group := f.group(t, "private-team")
	f.member(t, group, memberID, true)
	release, err := f.publishing.SubmitRelease(ctx, authenticated(&publishing.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "community-check", "# Private"), Visibility: taxonomy.Visibility_VISIBILITY_GROUPS, GroupIds: []string{group}}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	id := release.Msg.Release.ReleaseId
	for _, token := range []string{"", outsider} {
		if _, err = apiClient.GetDiscussion(ctx, rpcRequest(&api.GetDiscussionRequest{ReleaseId: id}, token)); connect.CodeOf(err) != connect.CodeNotFound {
			t.Fatalf("private discussion leaked: %v", err)
		}
	}
	profile, err := apiClient.UpdateProfile(ctx, authenticated(&api.UpdateProfileRequest{Mutation: mutation(), DisplayName: "Member One", Bio: "Product writing"}, member))
	if err != nil || profile.Msg.Profile.UserId != memberID {
		t.Fatalf("profile: %v", err)
	}
	if _, err = apiClient.UpdateProfile(ctx, authenticated(&api.UpdateProfileRequest{Mutation: mutation(), DisplayName: "Stale"}, member)); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("stale revision: %v", err)
	}
	if _, err = apiClient.GetProfile(ctx, connect.NewRequest(&api.GetProfileRequest{UserId: memberID})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("private author leaked: %v", err)
	}
	if _, err = apiClient.ListMembers(ctx, authenticated(&api.ListMembersRequest{Namespace: "team"}, outsider)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("member roster leaked: %v", err)
	}
	f.grant(t, group, taxonomy.PrincipalKind_PRINCIPAL_KIND_GROUP, taxonomy.NamespaceRole_NAMESPACE_ROLE_READER, true)
	members, err := apiClient.ListMembers(ctx, authenticated(&api.ListMembersRequest{Namespace: "team"}, member))
	if err != nil || len(members.Msg.Members) != 1 || members.Msg.Members[0].Profile.DisplayName != "Member One" {
		t.Fatalf("group-derived membership: %v %v", members, err)
	}
	f.grant(t, group, taxonomy.PrincipalKind_PRINCIPAL_KIND_GROUP, taxonomy.NamespaceRole_NAMESPACE_ROLE_READER, false)
	req := &api.CreateCommentRequest{Mutation: mutation(), ReleaseId: id, Body: "A useful example <script>alert(1)</script>"}
	first, err := apiClient.CreateComment(ctx, authenticated(req, member))
	if err != nil {
		t.Fatal(err)
	}
	again, err := apiClient.CreateComment(ctx, authenticated(req, member))
	if err != nil || again.Msg.Comment.CommentId != first.Msg.Comment.CommentId {
		t.Fatal("comment replay not idempotent", err)
	}
	reply, err := apiClient.CreateComment(ctx, authenticated(&api.CreateCommentRequest{Mutation: mutation(), ReleaseId: id, ParentId: first.Msg.Comment.CommentId, Body: "Thanks for the feedback"}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = apiClient.CreateComment(ctx, authenticated(&api.CreateCommentRequest{Mutation: mutation(), ReleaseId: id, ParentId: reply.Msg.Comment.CommentId, Body: "nested"}, member)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatal("nested reply accepted", err)
	}
	for _, bookmark := range []bool{false, true} {
		for i := 0; i < 2; i++ {
			if _, err = apiClient.SetReaction(ctx, authenticated(&api.SetReactionRequest{Mutation: mutation(), ReleaseId: id, Bookmark: bookmark, Active: true}, member)); err != nil {
				t.Fatal(err)
			}
		}
	}
	d, err := apiClient.GetDiscussion(ctx, authenticated(&api.GetDiscussionRequest{ReleaseId: id}, member))
	if err != nil || d.Msg.Discussion.LikeCount != 1 || d.Msg.Discussion.CommentCount != 2 || !d.Msg.Discussion.Bookmarked {
		t.Fatal("reaction counts", d, err)
	}
	private, err := apiClient.GetDiscussion(ctx, authenticated(&api.GetDiscussionRequest{ReleaseId: id}, f.owner))
	if err != nil || private.Msg.Discussion.Bookmarked {
		t.Fatal("bookmark is not private", err)
	}
	if _, err = apiClient.DeleteComment(ctx, authenticated(&api.DeleteCommentRequest{Mutation: mutation(), CommentId: reply.Msg.Comment.CommentId}, member)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatal("deleted another author's reply", err)
	}
	if _, err = apiClient.DeleteComment(ctx, authenticated(&api.DeleteCommentRequest{Mutation: mutation(), CommentId: first.Msg.Comment.CommentId}, member)); err != nil {
		t.Fatal(err)
	}
	replay, replayErr := apiClient.CreateComment(ctx, authenticated(req, member))
	if replayErr != nil || replay.Msg.Comment.Body != "" || !replay.Msg.Comment.Deleted {
		t.Fatal("deleted text returned by idempotent replay", replayErr)
	}
	comments, err := apiClient.ListComments(ctx, authenticated(&api.ListCommentsRequest{ReleaseId: id}, member))
	if err != nil || len(comments.Msg.Comments) != 1 || comments.Msg.Comments[0].Body != "" || !comments.Msg.Comments[0].Deleted {
		t.Fatal("deleted comment body exposed", err)
	}
	replies, err := apiClient.ListComments(ctx, authenticated(&api.ListCommentsRequest{ReleaseId: id, ParentId: first.Msg.Comment.CommentId}, member))
	if err != nil || len(replies.Msg.Comments) != 1 {
		t.Fatal("replies lost after parent removal", err)
	}
	for i := 0; i < 2; i++ {
		_, err = apiClient.CreateComment(ctx, authenticated(&api.CreateCommentRequest{Mutation: mutation(), ReleaseId: id, Body: "pagination"}, member))
		if err != nil {
			t.Fatal(err)
		}
	}
	pg, err := apiClient.ListComments(ctx, authenticated(&api.ListCommentsRequest{ReleaseId: id, Page: &common.PageRequest{PageSize: 1}}, member))
	if err != nil || pg.Msg.Page.NextPageToken == "" {
		t.Fatal("missing cursor", err)
	}
	if _, err = apiClient.ListComments(ctx, authenticated(&api.ListCommentsRequest{ReleaseId: id, ParentId: first.Msg.Comment.CommentId, Page: &common.PageRequest{PageToken: pg.Msg.Page.NextPageToken}}, member)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatal("cursor crossed thread", err)
	}
	f.grant(t, memberID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_READER, true)
	f.member(t, group, memberID, false)
	if _, err = apiClient.CreateComment(ctx, authenticated(req, member)); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatal("replay bypassed revocation", err)
	}
	bookmarks, err := apiClient.ListBookmarks(ctx, authenticated(&api.ListBookmarksRequest{}, member))
	if err != nil || len(bookmarks.Msg.Contributions) != 1 || !bookmarks.Msg.Contributions[0].Unavailable || bookmarks.Msg.Contributions[0].ReleaseId != "" || bookmarks.Msg.Contributions[0].Description != "" {
		t.Fatal("revoked bookmark leaked", err)
	}
}
