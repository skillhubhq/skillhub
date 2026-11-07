package hubhttp

import (
	"context"

	"connectrpc.com/connect"
	governancev1 "github.com/skillhubhq/skillhub/gen/api/governance/v1"
	"github.com/skillhubhq/skillhub/internal/hub"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Server) GetPlatformAccess(ctx context.Context, req *connect.Request[governancev1.GetPlatformAccessRequest]) (*connect.Response[governancev1.GetPlatformAccessResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	r, err := s.store.GetPlatformAccess(ctx, u, req.Msg.UserId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&governancev1.GetPlatformAccessResponse{UserId: r.UserID, Roles: r.Roles, Administrator: r.Administrator, CanManageUsers: r.CanManageUsers, CanReadAudit: r.CanReadAudit}), nil
}
func (s *Server) SetPlatformRole(ctx context.Context, req *connect.Request[governancev1.SetPlatformRoleRequest]) (*connect.Response[governancev1.SetPlatformRoleResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	err = s.store.SetPlatformRole(ctx, u, req.Msg.GetMutation().GetIdempotencyKey(), req.Msg.UserId, req.Msg.Role, req.Msg.Granted)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&governancev1.SetPlatformRoleResponse{}), nil
}
func scanDTO(r hub.ScanReport) *governancev1.ScanReport {
	if r.Engine == "" {
		return nil
	}
	v := &governancev1.ScanReport{Engine: r.Engine, Digest: r.Digest, Complete: r.Complete, Blocking: r.Blocking, ScannedAt: timestamppb.New(r.ScannedAt)}
	for _, f := range r.Findings {
		v.Findings = append(v.Findings, &governancev1.ScanFinding{Rule: f.Rule, Path: f.Path, Line: uint32(f.Line), Blocking: f.Blocking, Message: f.Message})
	}
	return v
}
func (s *Server) GetScanReport(ctx context.Context, req *connect.Request[governancev1.GetScanReportRequest]) (*connect.Response[governancev1.GetScanReportResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	r, err := s.store.GetScanReport(ctx, u, req.Msg.ReleaseId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&governancev1.GetScanReportResponse{Report: scanDTO(r)}), nil
}
func (s *Server) ScanRelease(ctx context.Context, req *connect.Request[governancev1.ScanReleaseRequest]) (*connect.Response[governancev1.ScanReleaseResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	r, err := s.store.ScanRelease(ctx, u, req.Msg.GetMutation().GetIdempotencyKey(), req.Msg.ReleaseId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&governancev1.ScanReleaseResponse{Report: scanDTO(r)}), nil
}
func (s *Server) ListNotifications(ctx context.Context, req *connect.Request[governancev1.ListNotificationsRequest]) (*connect.Response[governancev1.ListNotificationsResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	r, err := s.store.ListNotifications(ctx, u, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	result := &governancev1.ListNotificationsResponse{Page: pageDTO(r.Next)}
	for _, n := range r.Items {
		result.Notifications = append(result.Notifications, &governancev1.ReviewNotification{NotificationId: n.ID, ReleaseId: n.ReleaseID, Coordinate: n.Coordinate, Approved: n.Approved, Read: n.Read, CreatedAt: timestamppb.New(n.CreatedAt)})
	}
	return connect.NewResponse(result), nil
}
func (s *Server) MarkNotificationRead(ctx context.Context, req *connect.Request[governancev1.MarkNotificationReadRequest]) (*connect.Response[governancev1.MarkNotificationReadResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	if err = s.store.MarkNotificationRead(ctx, u, req.Msg.NotificationId); err != nil {
		return nil, err
	}
	return connect.NewResponse(&governancev1.MarkNotificationReadResponse{}), nil
}

func (s *Server) ListReviewTasks(ctx context.Context, req *connect.Request[governancev1.ListReviewTasksRequest]) (*connect.Response[governancev1.ListReviewTasksResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	r, err := s.store.ListReviewTasks(ctx, u, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	out := &governancev1.ListReviewTasksResponse{Total: r.Total, Page: pageDTO(r.Next)}
	if !r.OldestAt.IsZero() {
		out.OldestAt = timestamppb.New(r.OldestAt)
	}
	for _, t := range r.Tasks {
		out.Tasks = append(out.Tasks, &governancev1.ReviewTask{ReleaseId: t.ReleaseID, Coordinate: t.Coordinate, CreatedAt: timestamppb.New(t.CreatedAt)})
	}
	return connect.NewResponse(out), nil
}
func (s *Server) SetFeaturedSkill(ctx context.Context, req *connect.Request[governancev1.SetFeaturedSkillRequest]) (*connect.Response[governancev1.SetFeaturedSkillResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	if err = s.store.SetFeaturedSkill(ctx, u, req.Msg.GetMutation().GetIdempotencyKey(), req.Msg.Namespace, req.Msg.Name, req.Msg.Featured); err != nil {
		return nil, err
	}
	return connect.NewResponse(&governancev1.SetFeaturedSkillResponse{}), nil
}
