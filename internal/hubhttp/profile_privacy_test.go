package hubhttp

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	api "github.com/skillhubhq/skillhub/gen/api/community/v1"
	"github.com/skillhubhq/skillhub/gen/api/community/v1/communityv1connect"
	publishing "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
)

func TestProfileDiscoveryRequiresVisibleRelationshipAndHidesLoginName(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	client := communityv1connect.NewCommunityServiceClient(f.server.Client(), f.server.URL)
	authorID, author := f.user(t, "private-login")
	viewerID, viewer := f.user(t, "viewer")
	get := func(token string) (*api.Profile, error) {
		r, err := client.GetProfile(ctx, rpcRequest(&api.GetProfileRequest{UserId: authorID}, token))
		if err != nil {
			return nil, err
		}
		return r.Msg.Profile, nil
	}
	for _, token := range []string{"", viewer} {
		if _, err := get(token); connect.CodeOf(err) != connect.CodeNotFound {
			t.Fatal("unrelated profile disclosed", err)
		}
	}
	for _, token := range []string{author, f.owner} {
		p, err := get(token)
		if err != nil || p.Username != "private-login" {
			t.Fatal("self/admin profile unavailable", err)
		}
	}
	group := f.group(t, "team-access")
	f.member(t, group, authorID, true)
	f.member(t, group, viewerID, true)
	f.grant(t, group, taxonomy.PrincipalKind_PRINCIPAL_KIND_GROUP, taxonomy.NamespaceRole_NAMESPACE_ROLE_READER, true)
	p, err := get(viewer)
	if err != nil || p.Username != "" || p.DisplayName == "private-login" {
		t.Fatal("shared-team profile exposes login name", err)
	}
	f.member(t, group, viewerID, false)
	if _, err = get(viewer); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatal("removed membership retained profile discovery", err)
	}
	f.grant(t, authorID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, true)
	r, err := f.publishing.SubmitRelease(ctx, authenticated(&publishing.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "profile-check", "# Synthetic example"), Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC}, author))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", viewer} {
		p, err = get(token)
		if err != nil || p.Username != "" || p.DisplayName == "private-login" {
			t.Fatal("public profile exposes login name", err)
		}
		items, e := client.ListContributions(ctx, rpcRequest(&api.ListContributionsRequest{UserId: authorID}, token))
		if e != nil || len(items.Msg.Contributions) != 1 || items.Msg.Contributions[0].Author.Username != "" {
			t.Fatal("contribution exposes login name", e)
		}
	}
	if _, err = client.CreateComment(ctx, authenticated(&api.CreateCommentRequest{Mutation: mutation(), ReleaseId: r.Msg.Release.ReleaseId, Body: "A synthetic comment"}, author)); err != nil {
		t.Fatal(err)
	}
	comments, err := client.ListComments(ctx, connect.NewRequest(&api.ListCommentsRequest{ReleaseId: r.Msg.Release.ReleaseId}))
	if err != nil || len(comments.Msg.Comments) != 1 || comments.Msg.Comments[0].Author.Username != "" {
		t.Fatal("comment exposes login name", err)
	}
	if _, err = f.publishing.YankRelease(ctx, authenticated(&publishing.YankReleaseRequest{Mutation: mutation(), ReleaseId: r.Msg.Release.ReleaseId, Reason: "privacy regression"}, author)); err != nil {
		t.Fatal(err)
	}
	if _, err = get(viewer); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatal("withdrawn contribution retained profile discovery", err)
	}
}
