package client

import (
	"context"

	"connectrpc.com/connect"
	common "github.com/skillhubhq/skillhub/gen/api/common/v1"
	api "github.com/skillhubhq/skillhub/gen/api/community/v1"
	c "github.com/skillhubhq/skillhub/internal/community"
	"github.com/skillhubhq/skillhub/internal/provenancewire"
)

func CommunityProfile(v *api.Profile) c.Profile {
	if v == nil {
		return c.Profile{}
	}
	return c.Profile{UserID: v.UserId, Username: v.Username, DisplayName: v.DisplayName, Bio: v.Bio, Revision: v.Revision, CanEdit: v.CanEdit, Avatar: v.Avatar}
}
func CommunityTeam(v *api.Team) c.Team {
	if v == nil {
		return c.Team{}
	}
	return c.Team{Namespace: v.Namespace, DisplayName: v.DisplayName, Bio: v.Bio, Revision: v.Revision, CanEdit: v.CanEdit, CanViewMembers: v.CanViewMembers}
}
func CommunityMember(v *api.Member) c.Member {
	if v == nil {
		return c.Member{}
	}
	return c.Member{Profile: CommunityProfile(v.Profile), Roles: v.Roles}
}
func CommunityContribution(v *api.Contribution) c.Contribution {
	if v == nil {
		return c.Contribution{}
	}
	return c.Contribution{Unavailable: v.Unavailable, Provenance: provenancewire.Decode(v.Provenance), ReleaseID: v.ReleaseId, Namespace: v.Namespace, Name: v.Name, DisplayNameEN: v.DisplayNameEn, DisplayNameZH: v.DisplayNameZh, Icon: v.Icon, Version: v.Version, Description: v.Description, Author: CommunityProfile(v.Author)}
}
func CommunityComment(v *api.Comment) c.Comment {
	if v == nil {
		return c.Comment{}
	}
	return c.Comment{CommentID: v.CommentId, ReleaseID: v.ReleaseId, ParentID: v.ParentId, Author: CommunityProfile(v.Author), Body: v.Body, CreatedAt: v.CreatedAt.AsTime(), Deleted: v.Deleted, CanDelete: v.CanDelete, Version: v.Version}
}
func CommunityDiscussion(v *api.Discussion) c.Discussion {
	if v == nil {
		return c.Discussion{}
	}
	return c.Discussion{LikeCount: v.LikeCount, CommentCount: v.CommentCount, Liked: v.Liked, Bookmarked: v.Bookmarked, CanInteract: v.CanInteract}
}
func (client *Client) CommunityGetProfile(ctx context.Context, source string, id string) (c.Profile, error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Profile{}, err
	}
	response, err := remote.Community.GetProfile(ctx, connect.NewRequest(&api.GetProfileRequest{UserId: id}))
	if err != nil {
		return c.Profile{}, err
	}
	return CommunityProfile(response.Msg.Profile), nil
}
func (client *Client) CommunityUpdateProfile(ctx context.Context, source string, name, bio string, revision uint64, key string) (c.Profile, error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Profile{}, err
	}
	response, err := remote.Community.UpdateProfile(ctx, connect.NewRequest(&api.UpdateProfileRequest{DisplayName: name, Bio: bio, ExpectedRevision: revision, Mutation: &common.Mutation{IdempotencyKey: key}}))
	if err != nil {
		return c.Profile{}, err
	}
	return CommunityProfile(response.Msg.Profile), nil
}
func (client *Client) CommunityGetTeam(ctx context.Context, source string, namespace string) (c.Team, error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Team{}, err
	}
	response, err := remote.Community.GetTeam(ctx, connect.NewRequest(&api.GetTeamRequest{Namespace: namespace}))
	if err != nil {
		return c.Team{}, err
	}
	return CommunityTeam(response.Msg.Team), nil
}
func (client *Client) CommunityUpdateTeam(ctx context.Context, source string, namespace, name, bio string, revision uint64, key string) (c.Team, error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Team{}, err
	}
	response, err := remote.Community.UpdateTeam(ctx, connect.NewRequest(&api.UpdateTeamRequest{Namespace: namespace, DisplayName: name, Bio: bio, ExpectedRevision: revision, Mutation: &common.Mutation{IdempotencyKey: key}}))
	if err != nil {
		return c.Team{}, err
	}
	return CommunityTeam(response.Msg.Team), nil
}
func (client *Client) CommunityListTeams(ctx context.Context, source string, token string) (c.Page[c.Team], error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Page[c.Team]{}, err
	}
	response, err := remote.Community.ListTeams(ctx, connect.NewRequest(&api.ListTeamsRequest{Page: &common.PageRequest{PageToken: token}}))
	if err != nil {
		return c.Page[c.Team]{}, err
	}
	result := c.Page[c.Team]{Items: []c.Team{}, NextPage: response.Msg.GetPage().GetNextPageToken()}
	for _, v := range response.Msg.Teams {
		result.Items = append(result.Items, CommunityTeam(v))
	}
	return result, nil
}
func (client *Client) CommunityListMembers(ctx context.Context, source string, namespace, token string) (c.Page[c.Member], error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Page[c.Member]{}, err
	}
	response, err := remote.Community.ListMembers(ctx, connect.NewRequest(&api.ListMembersRequest{Namespace: namespace, Page: &common.PageRequest{PageToken: token}}))
	if err != nil {
		return c.Page[c.Member]{}, err
	}
	result := c.Page[c.Member]{Items: []c.Member{}, NextPage: response.Msg.GetPage().GetNextPageToken()}
	for _, v := range response.Msg.Members {
		result.Items = append(result.Items, CommunityMember(v))
	}
	return result, nil
}
func (client *Client) CommunityListContributions(ctx context.Context, source string, id, namespace, token string) (c.Page[c.Contribution], error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Page[c.Contribution]{}, err
	}
	response, err := remote.Community.ListContributions(ctx, connect.NewRequest(&api.ListContributionsRequest{UserId: id, Namespace: namespace, Page: &common.PageRequest{PageToken: token}}))
	if err != nil {
		return c.Page[c.Contribution]{}, err
	}
	result := c.Page[c.Contribution]{Items: []c.Contribution{}, NextPage: response.Msg.GetPage().GetNextPageToken()}
	for _, v := range response.Msg.Contributions {
		result.Items = append(result.Items, CommunityContribution(v))
	}
	return result, nil
}
func (client *Client) CommunityGetDiscussion(ctx context.Context, source string, id string) (c.Discussion, error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Discussion{}, err
	}
	response, err := remote.Community.GetDiscussion(ctx, connect.NewRequest(&api.GetDiscussionRequest{ReleaseId: id}))
	if err != nil {
		return c.Discussion{}, err
	}
	return CommunityDiscussion(response.Msg.Discussion), nil
}
func (client *Client) CommunityListComments(ctx context.Context, source string, id, parent, token string) (c.Page[c.Comment], error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Page[c.Comment]{}, err
	}
	response, err := remote.Community.ListComments(ctx, connect.NewRequest(&api.ListCommentsRequest{ReleaseId: id, ParentId: parent, Page: &common.PageRequest{PageToken: token}}))
	if err != nil {
		return c.Page[c.Comment]{}, err
	}
	result := c.Page[c.Comment]{Items: []c.Comment{}, NextPage: response.Msg.GetPage().GetNextPageToken()}
	for _, v := range response.Msg.Comments {
		result.Items = append(result.Items, CommunityComment(v))
	}
	return result, nil
}
func (client *Client) CommunityCreateComment(ctx context.Context, source string, id, parent, body, key string) (c.Comment, error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Comment{}, err
	}
	response, err := remote.Community.CreateComment(ctx, connect.NewRequest(&api.CreateCommentRequest{ReleaseId: id, ParentId: parent, Body: body, Mutation: &common.Mutation{IdempotencyKey: key}}))
	if err != nil {
		return c.Comment{}, err
	}
	return CommunityComment(response.Msg.Comment), nil
}
func (client *Client) CommunityDeleteComment(ctx context.Context, source string, id, key string) (bool, error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return false, err
	}
	response, err := remote.Community.DeleteComment(ctx, connect.NewRequest(&api.DeleteCommentRequest{CommentId: id, Mutation: &common.Mutation{IdempotencyKey: key}}))
	if err != nil {
		return false, err
	}
	_ = response
	return true, nil
}
func (client *Client) CommunitySetReaction(ctx context.Context, source string, id string, bookmark, active bool, key string) (c.Discussion, error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Discussion{}, err
	}
	response, err := remote.Community.SetReaction(ctx, connect.NewRequest(&api.SetReactionRequest{ReleaseId: id, Bookmark: bookmark, Active: active, Mutation: &common.Mutation{IdempotencyKey: key}}))
	if err != nil {
		return c.Discussion{}, err
	}
	return CommunityDiscussion(response.Msg.Discussion), nil
}
func (client *Client) CommunityListBookmarks(ctx context.Context, source string, token string) (c.Page[c.Contribution], error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Page[c.Contribution]{}, err
	}
	response, err := remote.Community.ListBookmarks(ctx, connect.NewRequest(&api.ListBookmarksRequest{Page: &common.PageRequest{PageToken: token}}))
	if err != nil {
		return c.Page[c.Contribution]{}, err
	}
	result := c.Page[c.Contribution]{Items: []c.Contribution{}, NextPage: response.Msg.GetPage().GetNextPageToken()}
	for _, v := range response.Msg.Contributions {
		result.Items = append(result.Items, CommunityContribution(v))
	}
	return result, nil
}

func (client *Client) CommunityUpdateAvatar(ctx context.Context, source string, image []byte, revision uint64, key string) (c.Profile, error) {
	remote, err := client.Remote(ctx, source)
	if err != nil {
		return c.Profile{}, err
	}
	response, err := remote.Community.UpdateAvatar(ctx, connect.NewRequest(&api.UpdateAvatarRequest{Image: image, ExpectedRevision: revision, Mutation: &common.Mutation{IdempotencyKey: key}}))
	if err != nil {
		return c.Profile{}, err
	}
	return CommunityProfile(response.Msg.Profile), nil
}

func (client *Client) CommunityRemoveBookmark(ctx context.Context, source, namespace, name, key string) error {
	r, err := client.Remote(ctx, source)
	if err != nil {
		return err
	}
	_, err = r.Community.RemoveBookmark(ctx, connect.NewRequest(&api.RemoveBookmarkRequest{Mutation: &common.Mutation{IdempotencyKey: key}, Namespace: namespace, Name: name}))
	return err
}
