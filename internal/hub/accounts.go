package hub

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"golang.org/x/crypto/bcrypt"
)

type NamespaceAccess struct {
	CanManage  bool `json:"can_manage"`
	CanPublish bool `json:"can_publish"`
	CanReview  bool `json:"can_review"`
}

func (s *Store) NamespaceAccess(ctx context.Context, actor Identity, namespace string) (NamespaceAccess, error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return NamespaceAccess{}, err
	}
	defer tx.Rollback()
	var result NamespaceAccess
	for _, check := range []struct {
		roles []taxonomy.NamespaceRole
		value *bool
	}{
		{nil, &result.CanManage}, {[]taxonomy.NamespaceRole{taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER}, &result.CanPublish}, {[]taxonomy.NamespaceRole{taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER}, &result.CanReview},
	} {
		err = requireRole(ctx, tx, user, namespace, check.roles...)
		if err != nil && !errors.Is(err, ErrForbidden) {
			return result, err
		}
		*check.value = err == nil
	}
	return result, nil
}

// 空间管理员只能按有界搜索词查找授权对象，不获得全站管理目录和外部身份信息。
func (s *Store) LookupNamespaceUsers(ctx context.Context, actor Identity, namespace, query string, page Page) (PageResult[User], error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 || len(query) > 512 {
		return PageResult[User]{}, ErrInvalid
	}
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[User]{}, err
	}
	defer tx.Rollback()
	if err = requireRole(ctx, tx, user, namespace); err != nil {
		return PageResult[User]{}, err
	}
	return pageQuery(s, ctx, tx, page, scope(user.ID, "namespace-users", []string{namespace, query}), `SELECT u.id,u.username,COALESCE(p.display_name,''),u.disabled FROM users u LEFT JOIN profiles p ON p.user_id=u.id WHERE u.id>@after AND (u.id=@query OR instr(lower(u.username||' '||COALESCE(p.display_name,'')),lower(@query))>0) ORDER BY u.id LIMIT @limit`, []any{sql.Named("query", query)}, func(rows *sql.Rows) (User, string, error) {
		var v User
		err := rows.Scan(&v.ID, &v.Username, &v.DisplayName, &v.Disabled)
		return v, v.ID, err
	})
}

func revokeUserSessions(ctx context.Context, tx *hubdb.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM devices WHERE user_id=?", id)
	return err
}
func localPassword(ctx context.Context, tx *hubdb.Tx, id, password string) error {
	if len(password) < 12 || len(password) > 72 {
		return ErrInvalid
	}
	var external bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM external_identities WHERE user_id=?)", id).Scan(&external); err != nil {
		return err
	}
	if external {
		return ErrForbidden
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE users SET password_hash=? WHERE id=?", hash, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	return revokeUserSessions(ctx, tx, id)
}
func (s *Store) ChangePassword(ctx context.Context, actor Identity, current, password string) error {
	if actor.ID == "" || len(current) == 0 || len(current) > 72 {
		return ErrUnauthenticated
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE instance SET id=id"); err != nil {
		return err
	}
	user, err := currentIdentity(ctx, tx, actor)
	if err != nil {
		return err
	}
	var hash []byte
	if err = tx.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id=?", user.ID).Scan(&hash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(current)) != nil {
		return ErrUnauthenticated
	}
	if err = localPassword(ctx, tx, user.ID, password); err != nil {
		return err
	}
	if err = audit(ctx, tx, user.ID, "user.password_change", user.ID, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ResetUserPassword(ctx context.Context, actor Identity, key, id, password string) error {
	_, err := mutate(s, ctx, actor, "user.password_reset", key, struct{ ID, Password string }{id, password}, adminOnly, func(tx *hubdb.Tx, user Identity) (struct{}, error) {
		if id == user.ID {
			return struct{}{}, ErrForbidden
		}
		if err := localPassword(ctx, tx, id, password); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, audit(ctx, tx, user.ID, "user.password_reset", id, s.now())
	})
	return err
}
func (s *Store) SetUserAdministrator(ctx context.Context, actor Identity, key, id string, enabled bool) (User, error) {
	return mutate(s, ctx, actor, "user.administrator", key, struct {
		ID      string
		Enabled bool
	}{id, enabled}, adminOnly, func(tx *hubdb.Tx, user Identity) (User, error) {
		var result User
		err := tx.QueryRowContext(ctx, "SELECT id,username,administrator,disabled FROM users WHERE id=?", id).Scan(&result.ID, &result.Username, &result.Administrator, &result.Disabled)
		if err != nil {
			return result, assertExists(err)
		}
		if !enabled && result.Administrator && !result.Disabled {
			var count int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE administrator=1 AND disabled=0").Scan(&count); err != nil {
				return result, err
			}
			if count <= 1 {
				return result, ErrForbidden
			}
		}
		if enabled && result.Disabled {
			return result, ErrForbidden
		}
		if _, err = tx.ExecContext(ctx, "UPDATE users SET administrator=? WHERE id=?", enabled, id); err != nil {
			return result, err
		}
		result.Administrator = enabled
		if err = revokeUserSessions(ctx, tx, id); err != nil {
			return result, err
		}
		return result, s.auditChange(ctx, tx, user.ID, "user.administrator", id, struct{ Administrator bool }{enabled})
	})
}

// 恢复仅由持有数据目录服务锁的本机运维命令调用，不能经 HTTP 调用。
func (s *Store) RecoverAdministrator(ctx context.Context, username, password string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE instance SET id=id"); err != nil {
		return err
	}
	var id string
	if err = tx.QueryRowContext(ctx, "SELECT id FROM users WHERE username=?", username).Scan(&id); err != nil {
		return assertExists(err)
	}
	if err = localPassword(ctx, tx, id, password); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE users SET administrator=1,disabled=0 WHERE id=?", id); err != nil {
		return err
	}
	if err = audit(ctx, tx, id, "user.recover_offline", id, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}
