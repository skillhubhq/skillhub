package hubhttp

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/provenancewire"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func pageDomain(page *commonv1.PageRequest) hub.Page {
	return hub.Page{Size: int(page.GetPageSize()), Token: page.GetPageToken()}
}
func pageDTO(token string) *commonv1.PageResult { return &commonv1.PageResult{NextPageToken: token} }
func mapItems[T, U any](items []T, convert func(T) *U) []*U {
	result := make([]*U, 0, len(items))
	for _, item := range items {
		result = append(result, convert(item))
	}
	return result
}
func skillDTO(v hub.Skill) *catalogv1.Skill {
	return &catalogv1.Skill{Revision: v.Revision, CanEdit: v.CanEdit, Provenance: provenancewire.Encode(v.Provenance), Namespace: v.Namespace, Name: v.Name, DisplayNameEn: v.DisplayNameEN, DisplayNameZh: v.DisplayNameZH, Icon: v.Icon, Description: v.Description, Category: v.Category, Tags: v.Tags, LatestVersion: v.LatestVersion, Featured: v.Featured}
}
func releaseDTO(v hub.Release) *catalogv1.Release {
	return &catalogv1.Release{Provenance: provenancewire.Encode(v.Provenance), ReleaseId: v.ID, Namespace: v.Namespace, Name: v.Name, DisplayNameEn: v.DisplayNameEN, DisplayNameZh: v.DisplayNameZH, Icon: v.Icon, Version: v.Version, Digest: v.Digest, Status: v.Status, Visibility: v.Visibility, GroupIds: v.GroupIDs, SubmittedBy: v.SubmittedBy, CreatedAt: timestamppb.New(v.CreatedAt), ReviewReason: v.ReviewReason, SkillMarkdown: v.Markdown}
}

func (s *Server) catalogIdentity(ctx context.Context, header http.Header) (hub.Identity, error) {
	if len(header.Values("Authorization")) > 0 || s.config.Private {
		return s.identity(ctx, header)
	}
	return hub.Identity{}, nil
}

func (s *Server) SearchSkills(ctx context.Context, req *connect.Request[catalogv1.SearchSkillsRequest]) (*connect.Response[catalogv1.SearchSkillsResponse], error) {
	user, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.SearchSkills(ctx, user, hub.Search{Query: req.Msg.Query, Namespace: req.Msg.Namespace, Category: req.Msg.Category, Tags: req.Msg.Tags, Order: req.Msg.Order, FeaturedOnly: req.Msg.FeaturedOnly}, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&catalogv1.SearchSkillsResponse{Skills: mapItems(result.Items, skillDTO), Page: pageDTO(result.Next)}), nil
}
func (s *Server) GetSkill(ctx context.Context, req *connect.Request[catalogv1.GetSkillRequest]) (*connect.Response[catalogv1.GetSkillResponse], error) {
	user, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.GetSkill(ctx, user, req.Msg.Namespace, req.Msg.Name)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&catalogv1.GetSkillResponse{Skill: skillDTO(result)}), nil
}
func (s *Server) ListReleases(ctx context.Context, req *connect.Request[catalogv1.ListReleasesRequest]) (*connect.Response[catalogv1.ListReleasesResponse], error) {
	user, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.ListReleases(ctx, user, req.Msg.Namespace, req.Msg.Name, pageDomain(req.Msg.Page), false)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&catalogv1.ListReleasesResponse{Releases: mapItems(result.Items, releaseDTO), Page: pageDTO(result.Next)}), nil
}
func (s *Server) DownloadRelease(ctx context.Context, req *connect.Request[catalogv1.DownloadReleaseRequest]) (*connect.Response[catalogv1.DownloadReleaseResponse], error) {
	user, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, archive, err := s.store.DownloadRelease(ctx, user, req.Msg.Namespace, req.Msg.Name, req.Msg.Version)
	if err != nil {
		return nil, err
	}
	// 必须先走完整授权和包完整性检查，客户端提供的摘要不构成权限。
	notModified := req.Msg.KnownDigest != "" && req.Msg.KnownDigest == result.Digest
	if notModified {
		archive = nil
	}
	return connect.NewResponse(&catalogv1.DownloadReleaseResponse{Release: releaseDTO(result), Digest: result.Digest, Archive: archive, NotModified: notModified}), nil
}
func (s *Server) SubmitRelease(ctx context.Context, req *connect.Request[publishingv1.SubmitReleaseRequest]) (*connect.Response[publishingv1.SubmitReleaseResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.SubmitRelease(ctx, user, req.Msg.Mutation.IdempotencyKey, hub.SubmitRelease{Provenance: provenancewire.Decode(req.Msg.Provenance), Namespace: req.Msg.Namespace, Version: req.Msg.Version, Archive: req.Msg.Archive, Visibility: req.Msg.Visibility, GroupIDs: req.Msg.GroupIds, Category: req.Msg.Category, Tags: req.Msg.Tags, DisplayNameEN: req.Msg.DisplayNameEn, DisplayNameZH: req.Msg.DisplayNameZh, Icon: req.Msg.Icon})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&publishingv1.SubmitReleaseResponse{Release: releaseDTO(result)}), nil
}
func (s *Server) ListSubmissions(ctx context.Context, req *connect.Request[publishingv1.ListSubmissionsRequest]) (*connect.Response[publishingv1.ListSubmissionsResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.ListSubmissions(ctx, user, req.Msg.Namespace, pageDomain(req.Msg.Page), hub.SubmissionFilter{ActiveOnly: req.Msg.ActiveOnly, Mine: req.Msg.Mine, Status: req.Msg.Status, Query: req.Msg.Query})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&publishingv1.ListSubmissionsResponse{Releases: mapItems(result.Items, releaseDTO), Page: pageDTO(result.Next)}), nil
}
func (s *Server) GetSubmission(ctx context.Context, req *connect.Request[publishingv1.GetSubmissionRequest]) (*connect.Response[publishingv1.GetSubmissionResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, archive, err := s.store.GetSubmission(ctx, user, req.Msg.ReleaseId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&publishingv1.GetSubmissionResponse{Release: releaseDTO(result), Archive: archive}), nil
}
func (s *Server) ReviewRelease(ctx context.Context, req *connect.Request[publishingv1.ReviewReleaseRequest]) (*connect.Response[publishingv1.ReviewReleaseResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.ReviewRelease(ctx, user, req.Msg.Mutation.IdempotencyKey, hub.ReviewRelease{ReleaseID: req.Msg.ReleaseId, Approve: req.Msg.Approve, Reason: req.Msg.Reason})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&publishingv1.ReviewReleaseResponse{Release: releaseDTO(result)}), nil
}
func (s *Server) YankRelease(ctx context.Context, req *connect.Request[publishingv1.YankReleaseRequest]) (*connect.Response[publishingv1.YankReleaseResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.YankRelease(ctx, user, req.Msg.Mutation.IdempotencyKey, hub.YankRelease{ReleaseID: req.Msg.ReleaseId, Reason: req.Msg.Reason})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&publishingv1.YankReleaseResponse{Release: releaseDTO(result)}), nil
}

func (s *Server) DeleteRelease(ctx context.Context, req *connect.Request[publishingv1.DeleteReleaseRequest]) (*connect.Response[publishingv1.DeleteReleaseResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.DeleteRelease(ctx, user, req.Msg.Mutation.IdempotencyKey, hub.DeleteRelease{ReleaseID: req.Msg.ReleaseId, ConfirmCoordinate: req.Msg.ConfirmCoordinate})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&publishingv1.DeleteReleaseResponse{ReleaseId: result.ReleaseID}), nil
}

func (s *Server) UpdateSkillMetadata(ctx context.Context, req *connect.Request[catalogv1.UpdateSkillMetadataRequest]) (*connect.Response[catalogv1.UpdateSkillMetadataResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v := req.Msg
	revision, err := s.store.UpdateSkillMetadata(ctx, u, v.GetMutation().GetIdempotencyKey(), hub.SkillMetadataChange{Namespace: v.Namespace, Name: v.Name, ExpectedRevision: v.ExpectedRevision, DisplayNameEN: v.DisplayNameEn, DisplayNameZH: v.DisplayNameZh, Description: v.Description, Icon: v.Icon, Category: v.Category, Tags: v.Tags})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&catalogv1.UpdateSkillMetadataResponse{Revision: revision}), nil
}
