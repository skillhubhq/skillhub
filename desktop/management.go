package main

import (
	"errors"
	"github.com/skillhubhq/skillhub/internal/hub"
	"strconv"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
)

// 管理列表是桌面表格投影，远端请求始终使用生成的 Connect 契约。
type AdminRow struct {
	PlatformRoles   []taxonomy.PlatformRole `json:"platform_roles"`
	OIDCIssuer      string                  `json:"oidc_issuer"`
	OIDCSubject     string                  `json:"oidc_subject"`
	ApprovalPending bool                    `json:"approval_pending"`
	ID              string                  `json:"id"`
	Name            string                  `json:"name"`
	Title           string                  `json:"title"`
	Revision        string                  `json:"revision"`
	Enabled         bool                    `json:"enabled"`
	Administrator   bool                    `json:"administrator"`
	Namespace       string                  `json:"namespace"`
	SubjectKind     taxonomy.PrincipalKind  `json:"subject_kind"`
	Role            taxonomy.NamespaceRole  `json:"role"`
	Detail          string                  `json:"detail"`
	CreatedAt       string                  `json:"created_at"`
}
type AdminPage struct {
	Rows     []AdminRow `json:"rows"`
	NextPage string     `json:"next_page"`
}

func (s *DesktopService) AdminList(source, kind, filter, token string) (AdminPage, error) {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return AdminPage{}, err
	}
	result := AdminPage{Rows: []AdminRow{}}
	page := &commonv1.PageRequest{PageSize: 50, PageToken: token}
	switch kind {
	case "users", "members":
		var users []*adminv1.User
		var next string
		if kind == "users" {
			r, e := remote.Admin.ListUsers(s.ctx, connect.NewRequest(&adminv1.ListUsersRequest{Page: page}))
			if e != nil {
				return result, e
			}
			users = r.Msg.Users
			next = r.Msg.GetPage().GetNextPageToken()
		} else {
			r, e := remote.Admin.ListGroupMembers(s.ctx, connect.NewRequest(&adminv1.ListGroupMembersRequest{Page: page, GroupId: filter}))
			if e != nil {
				return result, e
			}
			users = r.Msg.Users
			next = r.Msg.GetPage().GetNextPageToken()
		}
		result.NextPage = next
		for _, row := range users {
			result.Rows = append(result.Rows, AdminRow{ID: row.UserId, Name: row.Username, Title: row.DisplayName, Detail: strconv.FormatBool(row.Federated), Enabled: !row.Disabled, Administrator: row.Administrator, ApprovalPending: row.ApprovalPending, OIDCIssuer: row.OidcIssuer, OIDCSubject: row.OidcSubject, PlatformRoles: row.PlatformRoles})
		}
	case "groups":
		r, e := remote.Admin.ListGroups(s.ctx, connect.NewRequest(&adminv1.ListGroupsRequest{Page: page}))
		if e != nil {
			return result, e
		}
		result.NextPage = r.Msg.GetPage().GetNextPageToken()
		for _, row := range r.Msg.Groups {
			result.Rows = append(result.Rows, AdminRow{ID: row.GroupId, Name: row.Name})
		}
	case "namespaces", "submission-namespaces":
		r, e := remote.Admin.ListNamespaces(s.ctx, connect.NewRequest(&adminv1.ListNamespacesRequest{Page: page, ForSubmission: kind == "submission-namespaces"}))
		if e != nil {
			return result, e
		}
		result.NextPage = r.Msg.GetPage().GetNextPageToken()
		for _, row := range r.Msg.Namespaces {
			result.Rows = append(result.Rows, AdminRow{ID: row.Name, Name: row.Name, Enabled: row.ReviewRequired, Revision: strconv.FormatUint(row.Revision, 10)})
		}
	case "grants":
		r, e := remote.Admin.ListNamespaceGrants(s.ctx, connect.NewRequest(&adminv1.ListNamespaceGrantsRequest{Page: page, Namespace: filter}))
		if e != nil {
			return result, e
		}
		result.NextPage = r.Msg.GetPage().GetNextPageToken()
		for _, row := range r.Msg.Grants {
			result.Rows = append(result.Rows, AdminRow{ID: row.SubjectId, Namespace: row.Namespace, SubjectKind: row.SubjectKind, Role: row.Role})
		}
	case "categories":
		r, e := remote.Admin.ListCategories(s.ctx, connect.NewRequest(&adminv1.ListCategoriesRequest{Page: page}))
		if e != nil {
			return result, e
		}
		result.NextPage = r.Msg.GetPage().GetNextPageToken()
		for _, row := range r.Msg.Categories {
			result.Rows = append(result.Rows, AdminRow{ID: row.Name, Name: row.Name, Title: row.Title, Enabled: !row.Archived, Revision: strconv.FormatUint(row.Revision, 10)})
		}
	case "audit":
		r, e := remote.Admin.ListAudit(s.ctx, connect.NewRequest(&adminv1.ListAuditRequest{Page: page}))
		if e != nil {
			return result, e
		}
		result.NextPage = r.Msg.GetPage().GetNextPageToken()
		for _, row := range r.Msg.Events {
			created := ""
			if row.CreatedAt != nil {
				created = row.CreatedAt.AsTime().UTC().Format("2006-01-02 15:04:05Z")
			}
			result.Rows = append(result.Rows, AdminRow{ID: strconv.FormatUint(row.Sequence, 10), Name: row.Action, Title: row.Resource, Detail: row.Detail, Namespace: row.ActorId, CreatedAt: created})
		}
	default:
		return result, errors.New("unknown management list")
	}
	return result, nil
}

type AdminForm struct {
	Source        string                 `json:"source"`
	Action        string                 `json:"action"`
	RequestID     string                 `json:"request_id"`
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Title         string                 `json:"title"`
	Password      string                 `json:"password"`
	Namespace     string                 `json:"namespace"`
	Subject       string                 `json:"subject"`
	Revision      string                 `json:"revision"`
	Enabled       bool                   `json:"enabled"`
	Administrator bool                   `json:"administrator"`
	SubjectKind   taxonomy.PrincipalKind `json:"subject_kind"`
	Role          taxonomy.NamespaceRole `json:"role"`
}

func (s *DesktopService) AdminChange(f AdminForm) error {
	remote, err := s.core.Remote(s.ctx, f.Source)
	if err != nil {
		return err
	}
	mutation := &commonv1.Mutation{IdempotencyKey: f.RequestID}
	switch f.Action {
	case "user-add":
		_, err = remote.Admin.CreateUser(s.ctx, connect.NewRequest(&adminv1.CreateUserRequest{Mutation: mutation, Username: f.Name, Password: f.Password, Administrator: f.Administrator}))
	case "user-password":
		_, err = remote.Admin.ResetUserPassword(s.ctx, connect.NewRequest(&adminv1.ResetUserPasswordRequest{Mutation: mutation, UserId: f.ID, Password: f.Password}))
	case "user-admin":
		_, err = remote.Admin.SetUserAdministrator(s.ctx, connect.NewRequest(&adminv1.SetUserAdministratorRequest{Mutation: mutation, UserId: f.ID, Administrator: f.Administrator}))
	case "user-state":
		_, err = remote.Admin.SetUserDisabled(s.ctx, connect.NewRequest(&adminv1.SetUserDisabledRequest{Mutation: mutation, UserId: f.ID, Disabled: !f.Enabled}))
	case "group-add":
		_, err = remote.Admin.CreateGroup(s.ctx, connect.NewRequest(&adminv1.CreateGroupRequest{Mutation: mutation, Name: f.Name}))
	case "member":
		_, err = remote.Admin.SetGroupMember(s.ctx, connect.NewRequest(&adminv1.SetGroupMemberRequest{Mutation: mutation, GroupId: f.ID, UserId: f.Subject, Member: f.Enabled}))
	case "grant":
		_, err = remote.Admin.SetNamespaceGrant(s.ctx, connect.NewRequest(&adminv1.SetNamespaceGrantRequest{Mutation: mutation, Namespace: f.Namespace, SubjectId: f.Subject, SubjectKind: f.SubjectKind, Role: f.Role, Granted: f.Enabled}))
	case "namespace", "category":
		revision, e := strconv.ParseUint(f.Revision, 10, 64)
		if e != nil {
			return errors.New("invalid revision; reload the current record")
		}
		if f.Action == "namespace" {
			_, err = remote.Admin.PutNamespace(s.ctx, connect.NewRequest(&adminv1.PutNamespaceRequest{Mutation: mutation, Name: f.Name, ReviewRequired: f.Enabled, ExpectedRevision: revision}))
		} else {
			_, err = remote.Admin.PutCategory(s.ctx, connect.NewRequest(&adminv1.PutCategoryRequest{Mutation: mutation, Name: f.Name, Title: f.Title, Archived: !f.Enabled, ExpectedRevision: revision}))
		}
	default:
		return errors.New("unknown management action")
	}
	return err
}

func (s *DesktopService) NamespaceAccess(source, namespace string) (hub.NamespaceAccess, error) {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return hub.NamespaceAccess{}, err
	}
	r, err := remote.Admin.GetNamespaceAccess(s.ctx, connect.NewRequest(&adminv1.GetNamespaceAccessRequest{Namespace: namespace}))
	if err != nil {
		return hub.NamespaceAccess{}, err
	}
	return hub.NamespaceAccess{CanManage: r.Msg.CanManage, CanPublish: r.Msg.CanPublish, CanReview: r.Msg.CanReview}, nil
}
func (s *DesktopService) LookupUsers(source, namespace, query, token string) (AdminPage, error) {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return AdminPage{}, err
	}
	r, err := remote.Admin.LookupNamespaceUsers(s.ctx, connect.NewRequest(&adminv1.LookupNamespaceUsersRequest{Namespace: namespace, Query: query, Page: &commonv1.PageRequest{PageSize: 30, PageToken: token}}))
	if err != nil {
		return AdminPage{}, err
	}
	result := AdminPage{Rows: []AdminRow{}, NextPage: r.Msg.GetPage().GetNextPageToken()}
	for _, row := range r.Msg.Users {
		result.Rows = append(result.Rows, AdminRow{ID: row.UserId, Name: row.Username, Title: row.DisplayName, Enabled: !row.Disabled})
	}
	return result, nil
}
