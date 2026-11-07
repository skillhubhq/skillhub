package hubhttp

import (
	"connectrpc.com/connect"
	"context"
	api "github.com/skillhubhq/skillhub/gen/api/community/v1"
	c "github.com/skillhubhq/skillhub/internal/community"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func communityProfileDTO(v c.Profile) *api.Profile {
	return &api.Profile{UserId: v.UserID, Username: v.Username, DisplayName: v.DisplayName, Bio: v.Bio, Revision: v.Revision, CanEdit: v.CanEdit, Avatar: v.Avatar}
}
func communityTeamDTO(v c.Team) *api.Team {
	return &api.Team{Namespace: v.Namespace, DisplayName: v.DisplayName, Bio: v.Bio, Revision: v.Revision, CanEdit: v.CanEdit, CanViewMembers: v.CanViewMembers}
}
func communityMemberDTO(v c.Member) *api.Member {
	return &api.Member{Profile: communityProfileDTO(v.Profile), Roles: v.Roles}
}
func communityContributionDTO(v c.Contribution) *api.Contribution {
	return &api.Contribution{ReleaseId: v.ReleaseID, Namespace: v.Namespace, Name: v.Name, DisplayNameEn: v.DisplayNameEN, DisplayNameZh: v.DisplayNameZH, Icon: v.Icon, Version: v.Version, Description: v.Description, Author: communityProfileDTO(v.Author)}
}
func communityCommentDTO(v c.Comment) *api.Comment {
	return &api.Comment{CommentId: v.CommentID, ReleaseId: v.ReleaseID, ParentId: v.ParentID, Author: communityProfileDTO(v.Author), Body: v.Body, CreatedAt: timestamppb.New(v.CreatedAt), Deleted: v.Deleted, CanDelete: v.CanDelete}
}
func communityDiscussionDTO(v c.Discussion) *api.Discussion {
	return &api.Discussion{LikeCount: v.LikeCount, CommentCount: v.CommentCount, Liked: v.Liked, Bookmarked: v.Bookmarked, CanInteract: v.CanInteract}
}
func (s *Server) GetProfile(ctx context.Context, req *connect.Request[api.GetProfileRequest]) (*connect.Response[api.GetProfileResponse], error) {
	user, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.GetProfile(ctx, user, req.Msg.UserId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.GetProfileResponse{Profile: communityProfileDTO(v)}), nil
}
func (s *Server) UpdateProfile(ctx context.Context, req *connect.Request[api.UpdateProfileRequest]) (*connect.Response[api.UpdateProfileResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.UpdateProfile(ctx, user, req.Msg.Mutation.IdempotencyKey, c.ProfileChange{DisplayName: req.Msg.DisplayName, Bio: req.Msg.Bio, ExpectedRevision: req.Msg.ExpectedRevision})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.UpdateProfileResponse{Profile: communityProfileDTO(v)}), nil
}
func (s *Server) GetTeam(ctx context.Context, req *connect.Request[api.GetTeamRequest]) (*connect.Response[api.GetTeamResponse], error) {
	user, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.GetTeam(ctx, user, req.Msg.Namespace)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.GetTeamResponse{Team: communityTeamDTO(v)}), nil
}
func (s *Server) UpdateTeam(ctx context.Context, req *connect.Request[api.UpdateTeamRequest]) (*connect.Response[api.UpdateTeamResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.UpdateTeam(ctx, user, req.Msg.Mutation.IdempotencyKey, c.TeamChange{Namespace: req.Msg.Namespace, DisplayName: req.Msg.DisplayName, Bio: req.Msg.Bio, ExpectedRevision: req.Msg.ExpectedRevision})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.UpdateTeamResponse{Team: communityTeamDTO(v)}), nil
}
func (s *Server) ListTeams(ctx context.Context, req *connect.Request[api.ListTeamsRequest]) (*connect.Response[api.ListTeamsResponse], error) {
	user, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.ListTeams(ctx, user, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.ListTeamsResponse{Teams: mapItems(v.Items, communityTeamDTO), Page: pageDTO(v.Next)}), nil
}
func (s *Server) ListMembers(ctx context.Context, req *connect.Request[api.ListMembersRequest]) (*connect.Response[api.ListMembersResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.ListMembers(ctx, user, req.Msg.Namespace, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.ListMembersResponse{Members: mapItems(v.Items, communityMemberDTO), Page: pageDTO(v.Next)}), nil
}
func (s *Server) ListContributions(ctx context.Context, req *connect.Request[api.ListContributionsRequest]) (*connect.Response[api.ListContributionsResponse], error) {
	user, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.ListContributions(ctx, user, req.Msg.UserId, req.Msg.Namespace, pageDomain(req.Msg.Page), false)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.ListContributionsResponse{Contributions: mapItems(v.Items, communityContributionDTO), Page: pageDTO(v.Next)}), nil
}
func (s *Server) GetDiscussion(ctx context.Context, req *connect.Request[api.GetDiscussionRequest]) (*connect.Response[api.GetDiscussionResponse], error) {
	user, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.GetDiscussion(ctx, user, req.Msg.ReleaseId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.GetDiscussionResponse{Discussion: communityDiscussionDTO(v)}), nil
}
func (s *Server) ListComments(ctx context.Context, req *connect.Request[api.ListCommentsRequest]) (*connect.Response[api.ListCommentsResponse], error) {
	user, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.ListComments(ctx, user, req.Msg.ReleaseId, req.Msg.ParentId, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.ListCommentsResponse{Comments: mapItems(v.Items, communityCommentDTO), Page: pageDTO(v.Next)}), nil
}
func (s *Server) CreateComment(ctx context.Context, req *connect.Request[api.CreateCommentRequest]) (*connect.Response[api.CreateCommentResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.CreateComment(ctx, user, req.Msg.Mutation.IdempotencyKey, c.CommentChange{ReleaseID: req.Msg.ReleaseId, ParentID: req.Msg.ParentId, Body: req.Msg.Body})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.CreateCommentResponse{Comment: communityCommentDTO(v)}), nil
}
func (s *Server) DeleteComment(ctx context.Context, req *connect.Request[api.DeleteCommentRequest]) (*connect.Response[api.DeleteCommentResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	err = s.store.DeleteComment(ctx, user, req.Msg.Mutation.IdempotencyKey, req.Msg.CommentId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.DeleteCommentResponse{}), nil
}
func (s *Server) SetReaction(ctx context.Context, req *connect.Request[api.SetReactionRequest]) (*connect.Response[api.SetReactionResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.SetReaction(ctx, user, req.Msg.Mutation.IdempotencyKey, c.ReactionChange{ReleaseID: req.Msg.ReleaseId, Bookmark: req.Msg.Bookmark, Active: req.Msg.Active})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.SetReactionResponse{Discussion: communityDiscussionDTO(v)}), nil
}
func (s *Server) ListBookmarks(ctx context.Context, req *connect.Request[api.ListBookmarksRequest]) (*connect.Response[api.ListBookmarksResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.ListContributions(ctx, user, "", "", pageDomain(req.Msg.Page), true)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.ListBookmarksResponse{Contributions: mapItems(v.Items, communityContributionDTO), Page: pageDTO(v.Next)}), nil
}

func (s *Server) UpdateAvatar(ctx context.Context, req *connect.Request[api.UpdateAvatarRequest]) (*connect.Response[api.UpdateAvatarResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.UpdateAvatar(ctx, user, req.Msg.Mutation.IdempotencyKey, c.AvatarChange{Image: req.Msg.Image, ExpectedRevision: req.Msg.ExpectedRevision})
	if err != nil {
		return nil, publicError(err)
	}
	return connect.NewResponse(&api.UpdateAvatarResponse{Profile: communityProfileDTO(v)}), nil
}
