package hub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	PlatformRoles   []taxonomy.PlatformRole
	OIDCIssuer      string
	OIDCSubject     string
	ApprovalPending bool
	Federated       bool
	DisplayName     string
	ID              string
	Username        string
	Administrator   bool
	Disabled        bool
}
type Group struct {
	ID   string
	Name string
}
type Namespace struct {
	Name           string
	ReviewRequired bool
	Revision       uint64
}
type NamespaceGrant struct {
	Namespace string
	Kind      taxonomy.PrincipalKind
	SubjectID string
	Role      taxonomy.NamespaceRole
}
type Category struct {
	Name     string
	Title    string
	Archived bool
	Revision uint64
}
type AuditEvent struct {
	Sequence  uint64
	ActorID   string
	Action    string
	Resource  string
	CreatedAt time.Time
	Detail    string
}
type CreateUser struct {
	Username      string
	Password      string
	Administrator bool
}
type SetUserDisabled struct {
	ID       string
	Disabled bool
}
type SetGroupMember struct {
	GroupID string
	UserID  string
	Member  bool
}
type PutNamespace struct {
	Name             string
	ReviewRequired   bool
	ExpectedRevision uint64
}
type SetNamespaceGrant struct {
	NamespaceGrant
	Granted bool
}
type PutCategory struct {
	Name             string
	Title            string
	Archived         bool
	ExpectedRevision uint64
}

func (s *Store) CreateUser(ctx context.Context, actor Identity, key string, cmd CreateUser) (User, error) {
	if !validUsername(cmd.Username) || len(cmd.Password) < 12 || len(cmd.Password) > 72 {
		return User{}, ErrInvalid
	}
	return mutate(s, ctx, actor, "user.create", key, cmd, userAdministrator, func(tx *hubdb.Tx, actor Identity) (User, error) {
		if cmd.Administrator && !actor.Administrator {
			return User{}, ErrForbidden
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(cmd.Password), bcrypt.DefaultCost)
		if err != nil {
			return User{}, err
		}
		user := User{ID: uuid.NewString(), Username: cmd.Username, Administrator: cmd.Administrator}
		_, err = tx.ExecContext(ctx, "INSERT INTO users(id,username,password_hash,administrator) VALUES(?,?,?,?)", user.ID, user.Username, hash, user.Administrator)
		if isUnique(err) {
			return User{}, ErrConflict
		}
		if err != nil {
			return User{}, err
		}
		return user, s.auditChange(ctx, tx, actor.ID, "user.create", user.ID, user)
	})
}

func (s *Store) SetUserDisabled(ctx context.Context, actor Identity, key string, cmd SetUserDisabled) (User, error) {
	return mutate(s, ctx, actor, "user.disable", key, cmd, userAdministrator, func(tx *hubdb.Tx, actor Identity) (User, error) {
		if err := ordinaryAccount(ctx, tx, actor, cmd.ID); err != nil {
			return User{}, err
		}
		if cmd.ID == actor.ID && cmd.Disabled {
			return User{}, ErrForbidden
		}
		result, err := tx.ExecContext(ctx, "UPDATE users SET disabled=? WHERE id=?", cmd.Disabled, cmd.ID)
		if err != nil {
			return User{}, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return User{}, err
		}
		if n == 0 {
			return User{}, ErrNotFound
		}
		var pending bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM access_requests WHERE user_id=?)", cmd.ID).Scan(&pending); err != nil {
			return User{}, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM access_requests WHERE user_id=?", cmd.ID); err != nil {
			return User{}, err
		}
		if cmd.Disabled {
			if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", cmd.ID); err != nil {
				return User{}, err
			}
			if _, err = tx.ExecContext(ctx, "DELETE FROM devices WHERE user_id=?", cmd.ID); err != nil {
				return User{}, err
			}
		}
		var user User
		var roles string
		err = tx.QueryRowContext(ctx, "SELECT id,username,administrator,disabled,EXISTS(SELECT 1 FROM external_identities e WHERE e.user_id=users.id),COALESCE((SELECT p.display_name FROM profiles p WHERE p.user_id=users.id),''),EXISTS(SELECT 1 FROM access_requests a WHERE a.user_id=users.id),COALESCE((SELECT issuer FROM external_identities e WHERE e.user_id=users.id),''),COALESCE((SELECT subject FROM external_identities e WHERE e.user_id=users.id),''),COALESCE((SELECT json_group_array(role) FROM platform_roles p WHERE p.user_id=users.id),'[]') FROM users WHERE id=?", cmd.ID).Scan(&user.ID, &user.Username, &user.Administrator, &user.Disabled, &user.Federated, &user.DisplayName, &user.ApprovalPending, &user.OIDCIssuer, &user.OIDCSubject, &roles)
		if err == nil {
			err = json.Unmarshal([]byte(roles), &user.PlatformRoles)
		}
		if err != nil {
			return user, err
		}
		action := "user.disable"
		if pending {
			if cmd.Disabled {
				action = "user.reject_oidc_access"
			} else {
				action = "user.approve_oidc_access"
			}
		}
		return user, s.auditChange(ctx, tx, actor.ID, action, user.ID, cmd)
	})
}

func (s *Store) CreateGroup(ctx context.Context, actor Identity, key, name string) (Group, error) {
	if !packageformat.ValidName(name) {
		return Group{}, ErrInvalid
	}
	return mutate(s, ctx, actor, "group.create", key, name, adminOnly, func(tx *hubdb.Tx, actor Identity) (Group, error) {
		group := Group{ID: uuid.NewString(), Name: name}
		_, err := tx.ExecContext(ctx, "INSERT INTO groups(id,name) VALUES(?,?)", group.ID, name)
		if isUnique(err) {
			return Group{}, ErrConflict
		}
		if err != nil {
			return Group{}, err
		}
		return group, s.auditChange(ctx, tx, actor.ID, "group.create", group.ID, group)
	})
}

func (s *Store) SetGroupMember(ctx context.Context, actor Identity, key string, cmd SetGroupMember) error {
	_, err := mutate(s, ctx, actor, "group.member", key, cmd, adminOnly, func(tx *hubdb.Tx, actor Identity) (struct{}, error) {
		var exists bool
		err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM groups g,users u WHERE g.id=? AND u.id=?)", cmd.GroupID, cmd.UserID).Scan(&exists)
		if err != nil {
			return struct{}{}, err
		}
		if !exists {
			return struct{}{}, ErrNotFound
		}
		if cmd.Member {
			_, err = tx.ExecContext(ctx, "INSERT INTO group_members(group_id,user_id) VALUES(?,?) ON CONFLICT DO NOTHING", cmd.GroupID, cmd.UserID)
		} else {
			_, err = tx.ExecContext(ctx, "DELETE FROM group_members WHERE group_id=? AND user_id=?", cmd.GroupID, cmd.UserID)
		}
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, s.auditChange(ctx, tx, actor.ID, "group.member", cmd.GroupID+"/"+cmd.UserID, cmd)
	})
	return err
}

func (s *Store) PutNamespace(ctx context.Context, actor Identity, key string, cmd PutNamespace) (Namespace, error) {
	if !packageformat.ValidName(cmd.Name) {
		return Namespace{}, ErrInvalid
	}
	authorize := func(tx *hubdb.Tx, actor Identity) error {
		if cmd.ExpectedRevision == 0 {
			return adminOnly(tx, actor)
		}
		return requireRole(ctx, tx, actor, cmd.Name)
	}
	return mutate(s, ctx, actor, "namespace.put", key, cmd, authorize, func(tx *hubdb.Tx, actor Identity) (Namespace, error) {
		value := Namespace{Name: cmd.Name, ReviewRequired: cmd.ReviewRequired, Revision: cmd.ExpectedRevision + 1}
		var result sql.Result
		var err error
		if cmd.ExpectedRevision == 0 {
			result, err = tx.ExecContext(ctx, "INSERT INTO namespaces(name,review_required,revision) VALUES(?,?,1)", cmd.Name, cmd.ReviewRequired)
		} else {
			result, err = tx.ExecContext(ctx, "UPDATE namespaces SET review_required=?,revision=revision+1 WHERE name=? AND revision=?", cmd.ReviewRequired, cmd.Name, cmd.ExpectedRevision)
		}
		if isUnique(err) {
			return Namespace{}, ErrConflict
		}
		if err != nil {
			return Namespace{}, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return Namespace{}, err
		}
		if n != 1 {
			return Namespace{}, ErrConflict
		}
		return value, s.auditChange(ctx, tx, actor.ID, "namespace.put", cmd.Name, value)
	})
}

func (s *Store) SetNamespaceGrant(ctx context.Context, actor Identity, key string, cmd SetNamespaceGrant) error {
	if cmd.Kind != taxonomy.PrincipalKind_PRINCIPAL_KIND_USER && cmd.Kind != taxonomy.PrincipalKind_PRINCIPAL_KIND_GROUP {
		return ErrInvalid
	}
	if cmd.Role < taxonomy.NamespaceRole_NAMESPACE_ROLE_READER || cmd.Role > taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN {
		return ErrInvalid
	}
	_, err := mutate(s, ctx, actor, "namespace.grant", key, cmd, func(tx *hubdb.Tx, user Identity) error { return requireRole(ctx, tx, user, cmd.Namespace) }, func(tx *hubdb.Tx, actor Identity) (struct{}, error) {
		table := "users"
		if cmd.Kind == taxonomy.PrincipalKind_PRINCIPAL_KIND_GROUP {
			table = "groups"
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE id=?)", cmd.SubjectID).Scan(&exists); err != nil {
			return struct{}{}, err
		}
		if !exists {
			return struct{}{}, ErrNotFound
		}
		var err error
		if cmd.Granted {
			_, err = tx.ExecContext(ctx, "INSERT INTO namespace_grants(namespace,kind,subject_id,role) VALUES(?,?,?,?) ON CONFLICT DO NOTHING", cmd.Namespace, cmd.Kind, cmd.SubjectID, cmd.Role)
		} else {
			_, err = tx.ExecContext(ctx, "DELETE FROM namespace_grants WHERE namespace=? AND kind=? AND subject_id=? AND role=?", cmd.Namespace, cmd.Kind, cmd.SubjectID, cmd.Role)
		}
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, s.auditChange(ctx, tx, actor.ID, "namespace.grant", cmd.Namespace+"/"+cmd.SubjectID, cmd)
	})
	return err
}

func (s *Store) PutCategory(ctx context.Context, actor Identity, key string, cmd PutCategory) (Category, error) {
	if !packageformat.ValidName(cmd.Name) || len(strings.TrimSpace(cmd.Title)) == 0 || utf8.RuneCountInString(cmd.Title) > 128 {
		return Category{}, ErrInvalid
	}
	return mutate(s, ctx, actor, "category.put", key, cmd, adminOnly, func(tx *hubdb.Tx, actor Identity) (Category, error) {
		value := Category{Name: cmd.Name, Title: cmd.Title, Archived: cmd.Archived, Revision: cmd.ExpectedRevision + 1}
		var result sql.Result
		var err error
		if cmd.ExpectedRevision == 0 {
			result, err = tx.ExecContext(ctx, "INSERT INTO categories(name,title,archived,revision) VALUES(?,?,?,1)", cmd.Name, cmd.Title, cmd.Archived)
		} else {
			result, err = tx.ExecContext(ctx, "UPDATE categories SET title=?,archived=?,revision=revision+1 WHERE name=? AND revision=?", cmd.Title, cmd.Archived, cmd.Name, cmd.ExpectedRevision)
		}
		if isUnique(err) {
			return Category{}, ErrConflict
		}
		if err != nil {
			return Category{}, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return Category{}, err
		}
		if n != 1 {
			return Category{}, ErrConflict
		}
		return value, s.auditChange(ctx, tx, actor.ID, "category.put", cmd.Name, value)
	})
}

func (s *Store) ListUsers(ctx context.Context, actor Identity, groupID string, page Page) (PageResult[User], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[User]{}, err
	}
	defer tx.Rollback()
	if err = userAdministrator(tx, user); err != nil {
		return PageResult[User]{}, err
	}
	return pageQuery(s, ctx, tx, page, scope(user.ID, "users", groupID), `SELECT id,username,administrator,disabled,EXISTS(SELECT 1 FROM external_identities e WHERE e.user_id=users.id),COALESCE((SELECT p.display_name FROM profiles p WHERE p.user_id=users.id),''),EXISTS(SELECT 1 FROM access_requests a WHERE a.user_id=users.id),COALESCE((SELECT issuer FROM external_identities e WHERE e.user_id=users.id),''),COALESCE((SELECT subject FROM external_identities e WHERE e.user_id=users.id),''),COALESCE((SELECT json_group_array(role) FROM platform_roles p WHERE p.user_id=users.id),'[]') FROM users WHERE id>@after AND (@group='' OR EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=@group AND m.user_id=users.id)) ORDER BY id LIMIT @limit`, []any{sql.Named("group", groupID)}, func(rows *sql.Rows) (User, string, error) {
		var v User
		var roles string
		err := rows.Scan(&v.ID, &v.Username, &v.Administrator, &v.Disabled, &v.Federated, &v.DisplayName, &v.ApprovalPending, &v.OIDCIssuer, &v.OIDCSubject, &roles)
		if err == nil {
			err = json.Unmarshal([]byte(roles), &v.PlatformRoles)
		}
		return v, v.ID, err
	})
}

func (s *Store) ListGroups(ctx context.Context, actor Identity, page Page) (PageResult[Group], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[Group]{}, err
	}
	defer tx.Rollback()
	if err = authenticatedOnly(tx, user); err != nil {
		return PageResult[Group]{}, err
	}
	return pageQuery(s, ctx, tx, page, scope(user.ID, "groups", nil), "SELECT id,name FROM groups WHERE id>@after ORDER BY id LIMIT @limit", nil, func(rows *sql.Rows) (Group, string, error) {
		var v Group
		err := rows.Scan(&v.ID, &v.Name)
		return v, v.ID, err
	})
}

func (s *Store) ListNamespaces(ctx context.Context, actor Identity, page Page) (PageResult[Namespace], error) {
	return s.listNamespaces(ctx, actor, page, false)
}

// ListSubmissionNamespaces 按实际提交角色过滤，包含群组继承和管理员权限。
func (s *Store) ListSubmissionNamespaces(ctx context.Context, actor Identity, page Page) (PageResult[Namespace], error) {
	return s.listNamespaces(ctx, actor, page, true)
}

func (s *Store) listNamespaces(ctx context.Context, actor Identity, page Page, forSubmission bool) (PageResult[Namespace], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[Namespace]{}, err
	}
	defer tx.Rollback()
	if err = authenticatedOnly(tx, user); err != nil {
		return PageResult[Namespace]{}, err
	}
	return pageQuery(s, ctx, tx, page, scope(user.ID, "namespaces", forSubmission), `SELECT name,review_required,revision FROM namespaces n WHERE name>@after AND (@admin=1 OR EXISTS(SELECT 1 FROM namespace_grants g WHERE g.namespace=n.name AND (@submission=0 OR g.role IN (@maintainer,@namespace_admin)) AND ((g.kind=1 AND g.subject_id=@actor) OR (g.kind=2 AND EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.subject_id AND m.user_id=@actor))))) ORDER BY name LIMIT @limit`, []any{sql.Named("admin", user.Administrator), sql.Named("actor", user.ID), sql.Named("submission", forSubmission), sql.Named("maintainer", taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER), sql.Named("namespace_admin", taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN)}, func(rows *sql.Rows) (Namespace, string, error) {
		var v Namespace
		err := rows.Scan(&v.Name, &v.ReviewRequired, &v.Revision)
		return v, v.Name, err
	})
}

func (s *Store) ListNamespaceGrants(ctx context.Context, actor Identity, namespace string, page Page) (PageResult[NamespaceGrant], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[NamespaceGrant]{}, err
	}
	defer tx.Rollback()
	if err = requireRole(ctx, tx, user, namespace); err != nil {
		return PageResult[NamespaceGrant]{}, err
	}
	return pageQuery(s, ctx, tx, page, scope(user.ID, "grants", namespace), `SELECT namespace,kind,subject_id,role,kind||':'||subject_id||':'||role AS cursor_key FROM namespace_grants WHERE namespace=@namespace AND cursor_key>@after ORDER BY cursor_key LIMIT @limit`, []any{sql.Named("namespace", namespace)}, func(rows *sql.Rows) (NamespaceGrant, string, error) {
		var v NamespaceGrant
		var key string
		err := rows.Scan(&v.Namespace, &v.Kind, &v.SubjectID, &v.Role, &key)
		return v, key, err
	})
}

func (s *Store) ListCategories(ctx context.Context, actor Identity, page Page) (PageResult[Category], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[Category]{}, err
	}
	defer tx.Rollback()
	if err = authenticatedOnly(tx, user); err != nil {
		return PageResult[Category]{}, err
	}
	return pageQuery(s, ctx, tx, page, scope(user.ID, "categories", nil), "SELECT name,title,archived,revision FROM categories WHERE name>@after ORDER BY name LIMIT @limit", nil, func(rows *sql.Rows) (Category, string, error) {
		var v Category
		err := rows.Scan(&v.Name, &v.Title, &v.Archived, &v.Revision)
		return v, v.Name, err
	})
}

func (s *Store) ListAudit(ctx context.Context, actor Identity, page Page) (PageResult[AuditEvent], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[AuditEvent]{}, err
	}
	defer tx.Rollback()
	if err = hasPlatformRole(ctx, tx, user, taxonomy.PlatformRole_PLATFORM_ROLE_AUDITOR); err != nil {
		return PageResult[AuditEvent]{}, err
	}
	return pageQuery(s, ctx, tx, page, scope(user.ID, "audit", nil), "SELECT sequence,actor_id,action,resource,created_at,detail FROM audit WHERE sequence>CAST(COALESCE(NULLIF(@after,''),'0') AS INTEGER) ORDER BY sequence LIMIT @limit", nil, func(rows *sql.Rows) (AuditEvent, string, error) {
		var v AuditEvent
		var timestamp int64
		err := rows.Scan(&v.Sequence, &v.ActorID, &v.Action, &v.Resource, &timestamp, &v.Detail)
		v.CreatedAt = time.Unix(timestamp, 0)
		return v, strconv.FormatUint(v.Sequence, 10), err
	})
}

// assertExists 将不存在的数据库记录转换为稳定领域错误。
func assertExists(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// auditChange 仅接收已筛选的公开管理状态；密码和包内容不能作为审计值传入。
func (s *Store) auditChange(ctx context.Context, tx *hubdb.Tx, actor, action, resource string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO audit(actor_id,action,resource,created_at,detail) VALUES(?,?,?,?,?)", actor, action, resource, s.now().Unix(), string(raw))
	return err
}
