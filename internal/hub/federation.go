package hub

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// FederatedLogin 只接受服务端第三方身份适配器确认的 issuer/sub，不按邮箱合并本地账号。
func (s *Store) FederatedLogin(ctx context.Context, issuer, subject, displayName string, allowCreate bool) (Session, error) {
	if issuer == "" || subject == "" || len(issuer) > 2048 || len(subject) > 255 {
		return Session{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	// 先取得写锁，串行化首次登录与禁用操作。
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<=? AND refresh_expires_at<=?", s.now().Unix(), s.now().Unix()); err != nil {
		return Session{}, err
	}
	var user Identity
	var disabled bool
	err = tx.QueryRowContext(ctx, `SELECT u.id,u.username,u.administrator,u.disabled FROM external_identities e JOIN users u ON u.id=e.user_id WHERE e.issuer=? AND e.subject=?`, issuer, subject).Scan(&user.ID, &user.Username, &user.Administrator, &disabled)
	if errors.Is(err, sql.ErrNoRows) {
		if s.policy.IsCommunity() {
			var count int
			if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count); e != nil {
				return Session{}, e
			}
			if count >= s.policy.Limits.Users {
				return Session{}, ErrQuota
			}
		}
		if !allowCreate {
			var pendingCount int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM access_requests").Scan(&pendingCount); err != nil {
				return Session{}, err
			}
			if pendingCount >= 1000 {
				return Session{}, ErrRateLimited
			}
		}
		user = Identity{ID: uuid.NewString()}
		user.Username = "sso-" + user.ID
		if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,username,password_hash,administrator,disabled) VALUES(?,?,?,0,?)", user.ID, user.Username, []byte{}, !allowCreate); err != nil {
			return Session{}, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO external_identities(issuer,subject,user_id) VALUES(?,?,?)", issuer, subject, user.ID); err != nil {
			return Session{}, err
		}
		displayName = strings.TrimSpace(displayName)
		if !utf8.ValidString(displayName) {
			displayName = ""
		}
		runes := []rune(displayName)
		if len(runes) > 80 {
			displayName = string(runes[:80])
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO profiles(user_id,display_name) VALUES(?,?)", user.ID, displayName); err != nil {
			return Session{}, err
		}
		action := "user.provision_external"
		if !allowCreate {
			if _, err = tx.ExecContext(ctx, "INSERT INTO access_requests(user_id,created_at) VALUES(?,?)", user.ID, s.now().Unix()); err != nil {
				return Session{}, err
			}
			disabled = true
			action = "user.request_external_access"
		}
		if err = audit(ctx, tx, user.ID, action, user.ID, s.now()); err != nil {
			return Session{}, err
		}
	} else if err != nil {
		return Session{}, err
	}
	if disabled {
		var pending bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM access_requests WHERE user_id=?)", user.ID).Scan(&pending); err != nil {
			return Session{}, err
		}
		if pending {
			if err = tx.Commit(); err != nil {
				return Session{}, err
			}
			return Session{}, ErrPendingApproval
		}
		return Session{}, ErrUnauthenticated
	}
	session, err := s.newSession(ctx, tx, user)
	if err != nil {
		return Session{}, err
	}
	return session, tx.Commit()
}

// 外部身份会话较短；企业禁用的即时同步另由管理员停用或后续 SCIM 完成。
const federatedSessionDuration = 8 * time.Hour
