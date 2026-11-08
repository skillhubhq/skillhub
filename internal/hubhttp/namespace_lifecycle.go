package hubhttp

import (
	"connectrpc.com/connect"
	"context"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	"github.com/skillhubhq/skillhub/internal/hub"
)

func lifecycleDTO(v hub.NamespaceLifecycle) *adminv1.NamespaceLifecycle {
	return &adminv1.NamespaceLifecycle{Name: v.Name, Revision: v.Revision, Archived: v.Archived, Releases: v.Releases, Grants: v.Grants, Comments: v.Comments, Evidence: v.Evidence}
}
func (s *Server) GetNamespaceLifecycle(ctx context.Context, req *connect.Request[adminv1.GetNamespaceLifecycleRequest]) (*connect.Response[adminv1.GetNamespaceLifecycleResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.GetNamespaceLifecycle(ctx, u, req.Msg.Name)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.GetNamespaceLifecycleResponse{Lifecycle: lifecycleDTO(v)}), nil
}
func (s *Server) SetNamespaceArchived(ctx context.Context, req *connect.Request[adminv1.SetNamespaceArchivedRequest]) (*connect.Response[adminv1.SetNamespaceArchivedResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.SetNamespaceArchived(ctx, u, req.Msg.Mutation.IdempotencyKey, hub.ChangeNamespaceLifecycle{Name: req.Msg.Name, ExpectedRevision: req.Msg.ExpectedRevision, Archived: req.Msg.Archived})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.SetNamespaceArchivedResponse{Lifecycle: lifecycleDTO(v)}), nil
}
func (s *Server) PurgeNamespace(ctx context.Context, req *connect.Request[adminv1.PurgeNamespaceRequest]) (*connect.Response[adminv1.PurgeNamespaceResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.PurgeNamespace(ctx, u, req.Msg.Mutation.IdempotencyKey, hub.ChangeNamespaceLifecycle{Name: req.Msg.Name, ExpectedRevision: req.Msg.ExpectedRevision, ConfirmName: req.Msg.ConfirmName})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.PurgeNamespaceResponse{Lifecycle: lifecycleDTO(v)}), nil
}
