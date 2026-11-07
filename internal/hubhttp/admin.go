package hubhttp

import (
	"connectrpc.com/connect"
	"context"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	"github.com/skillhubhq/skillhub/internal/hub"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func userDTO(v hub.User) *adminv1.User {
	return &adminv1.User{UserId: v.ID, Username: v.Username, Administrator: v.Administrator, Disabled: v.Disabled, Federated: v.Federated, DisplayName: v.DisplayName, ApprovalPending: v.ApprovalPending, OidcIssuer: v.OIDCIssuer, OidcSubject: v.OIDCSubject, PlatformRoles: v.PlatformRoles}
}
func groupDTO(v hub.Group) *adminv1.Group { return &adminv1.Group{GroupId: v.ID, Name: v.Name} }
func namespaceDTO(v hub.Namespace) *adminv1.Namespace {
	return &adminv1.Namespace{Name: v.Name, ReviewRequired: v.ReviewRequired, Revision: v.Revision}
}
func namespaceGrantDTO(v hub.NamespaceGrant) *adminv1.NamespaceGrant {
	return &adminv1.NamespaceGrant{Namespace: v.Namespace, SubjectKind: v.Kind, SubjectId: v.SubjectID, Role: v.Role}
}
func categoryDTO(v hub.Category) *adminv1.Category {
	return &adminv1.Category{Name: v.Name, Title: v.Title, Archived: v.Archived, Revision: v.Revision}
}
func auditEventDTO(v hub.AuditEvent) *adminv1.AuditEvent {
	return &adminv1.AuditEvent{Sequence: v.Sequence, ActorId: v.ActorID, Action: v.Action, Resource: v.Resource, CreatedAt: timestamppb.New(v.CreatedAt), Detail: v.Detail}
}
func (s *Server) CreateUser(ctx context.Context, req *connect.Request[adminv1.CreateUserRequest]) (*connect.Response[adminv1.CreateUserResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.CreateUser(ctx, user, req.Msg.Mutation.IdempotencyKey, hub.CreateUser{Username: req.Msg.Username, Password: req.Msg.Password, Administrator: req.Msg.Administrator})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.CreateUserResponse{User: userDTO(result)}), nil
}
func (s *Server) SetUserDisabled(ctx context.Context, req *connect.Request[adminv1.SetUserDisabledRequest]) (*connect.Response[adminv1.SetUserDisabledResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.SetUserDisabled(ctx, user, req.Msg.Mutation.IdempotencyKey, hub.SetUserDisabled{ID: req.Msg.UserId, Disabled: req.Msg.Disabled})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.SetUserDisabledResponse{User: userDTO(result)}), nil
}
func (s *Server) CreateGroup(ctx context.Context, req *connect.Request[adminv1.CreateGroupRequest]) (*connect.Response[adminv1.CreateGroupResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.CreateGroup(ctx, user, req.Msg.Mutation.IdempotencyKey, req.Msg.Name)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.CreateGroupResponse{Group: groupDTO(result)}), nil
}
func (s *Server) SetGroupMember(ctx context.Context, req *connect.Request[adminv1.SetGroupMemberRequest]) (*connect.Response[adminv1.SetGroupMemberResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	err = s.store.SetGroupMember(ctx, user, req.Msg.Mutation.IdempotencyKey, hub.SetGroupMember{GroupID: req.Msg.GroupId, UserID: req.Msg.UserId, Member: req.Msg.Member})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.SetGroupMemberResponse{}), nil
}
func (s *Server) PutNamespace(ctx context.Context, req *connect.Request[adminv1.PutNamespaceRequest]) (*connect.Response[adminv1.PutNamespaceResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.PutNamespace(ctx, user, req.Msg.Mutation.IdempotencyKey, hub.PutNamespace{Name: req.Msg.Name, ReviewRequired: req.Msg.ReviewRequired, ExpectedRevision: req.Msg.ExpectedRevision})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.PutNamespaceResponse{Namespace: namespaceDTO(result)}), nil
}
func (s *Server) SetNamespaceGrant(ctx context.Context, req *connect.Request[adminv1.SetNamespaceGrantRequest]) (*connect.Response[adminv1.SetNamespaceGrantResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	err = s.store.SetNamespaceGrant(ctx, user, req.Msg.Mutation.IdempotencyKey, hub.SetNamespaceGrant{NamespaceGrant: hub.NamespaceGrant{Namespace: req.Msg.Namespace, Kind: req.Msg.SubjectKind, SubjectID: req.Msg.SubjectId, Role: req.Msg.Role}, Granted: req.Msg.Granted})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.SetNamespaceGrantResponse{}), nil
}
func (s *Server) PutCategory(ctx context.Context, req *connect.Request[adminv1.PutCategoryRequest]) (*connect.Response[adminv1.PutCategoryResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.PutCategory(ctx, user, req.Msg.Mutation.IdempotencyKey, hub.PutCategory{Name: req.Msg.Name, Title: req.Msg.Title, Archived: req.Msg.Archived, ExpectedRevision: req.Msg.ExpectedRevision})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.PutCategoryResponse{Category: categoryDTO(result)}), nil
}
func (s *Server) ListUsers(ctx context.Context, req *connect.Request[adminv1.ListUsersRequest]) (*connect.Response[adminv1.ListUsersResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.ListUsers(ctx, user, "", pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ListUsersResponse{Users: mapItems(result.Items, userDTO), Page: pageDTO(result.Next)}), nil
}
func (s *Server) ListGroupMembers(ctx context.Context, req *connect.Request[adminv1.ListGroupMembersRequest]) (*connect.Response[adminv1.ListGroupMembersResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.ListUsers(ctx, user, req.Msg.GroupId, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ListGroupMembersResponse{Users: mapItems(result.Items, userDTO), Page: pageDTO(result.Next)}), nil
}
func (s *Server) ListGroups(ctx context.Context, req *connect.Request[adminv1.ListGroupsRequest]) (*connect.Response[adminv1.ListGroupsResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.ListGroups(ctx, user, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ListGroupsResponse{Groups: mapItems(result.Items, groupDTO), Page: pageDTO(result.Next)}), nil
}
func (s *Server) ListNamespaces(ctx context.Context, req *connect.Request[adminv1.ListNamespacesRequest]) (*connect.Response[adminv1.ListNamespacesResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	list := s.store.ListNamespaces
	if req.Msg.ForSubmission {
		list = s.store.ListSubmissionNamespaces
	}
	result, err := list(ctx, user, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ListNamespacesResponse{Namespaces: mapItems(result.Items, namespaceDTO), Page: pageDTO(result.Next)}), nil
}
func (s *Server) ListNamespaceGrants(ctx context.Context, req *connect.Request[adminv1.ListNamespaceGrantsRequest]) (*connect.Response[adminv1.ListNamespaceGrantsResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.ListNamespaceGrants(ctx, user, req.Msg.Namespace, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ListNamespaceGrantsResponse{Grants: mapItems(result.Items, namespaceGrantDTO), Page: pageDTO(result.Next)}), nil
}
func (s *Server) ListCategories(ctx context.Context, req *connect.Request[adminv1.ListCategoriesRequest]) (*connect.Response[adminv1.ListCategoriesResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.ListCategories(ctx, user, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ListCategoriesResponse{Categories: mapItems(result.Items, categoryDTO), Page: pageDTO(result.Next)}), nil
}
func (s *Server) ListAudit(ctx context.Context, req *connect.Request[adminv1.ListAuditRequest]) (*connect.Response[adminv1.ListAuditResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.ListAudit(ctx, user, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ListAuditResponse{Events: mapItems(result.Items, auditEventDTO), Page: pageDTO(result.Next)}), nil
}

func (s *Server) LookupNamespaceUsers(ctx context.Context, req *connect.Request[adminv1.LookupNamespaceUsersRequest]) (*connect.Response[adminv1.LookupNamespaceUsersResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.LookupNamespaceUsers(ctx, user, req.Msg.Namespace, req.Msg.Query, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.LookupNamespaceUsersResponse{Users: mapItems(result.Items, userDTO), Page: pageDTO(result.Next)}), nil
}
func (s *Server) GetNamespaceAccess(ctx context.Context, req *connect.Request[adminv1.GetNamespaceAccessRequest]) (*connect.Response[adminv1.GetNamespaceAccessResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.NamespaceAccess(ctx, user, req.Msg.Namespace)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.GetNamespaceAccessResponse{CanManage: result.CanManage, CanPublish: result.CanPublish, CanReview: result.CanReview}), nil
}
func (s *Server) ResetUserPassword(ctx context.Context, req *connect.Request[adminv1.ResetUserPasswordRequest]) (*connect.Response[adminv1.ResetUserPasswordResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	if err = s.store.ResetUserPassword(ctx, user, req.Msg.Mutation.IdempotencyKey, req.Msg.UserId, req.Msg.Password); err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ResetUserPasswordResponse{}), nil
}
func (s *Server) SetUserAdministrator(ctx context.Context, req *connect.Request[adminv1.SetUserAdministratorRequest]) (*connect.Response[adminv1.SetUserAdministratorResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	result, err := s.store.SetUserAdministrator(ctx, user, req.Msg.Mutation.IdempotencyKey, req.Msg.UserId, req.Msg.Administrator)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.SetUserAdministratorResponse{User: userDTO(result)}), nil
}
