package hub

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/internal/hubdb"
)

const renewableSessionSchema = `
ALTER TABLE sessions ADD COLUMN previous_access_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN previous_access_expires_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN refresh_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN previous_refresh_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN refresh_request TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN refresh_expires_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN absolute_expires_at INTEGER NOT NULL DEFAULT 0;
CREATE INDEX sessions_refresh ON sessions(refresh_hash);
CREATE INDEX sessions_previous_refresh ON sessions(previous_refresh_hash);
ALTER TABLE devices ADD COLUMN renewable INTEGER NOT NULL DEFAULT 0;
`

const accessDuration = 15 * time.Minute
const refreshDuration = 30 * 24 * time.Hour
const sessionDuration = 90 * 24 * time.Hour

func (s *Store) makeRenewable(ctx context.Context, tx *hubdb.Tx, value Session) (Session, error) {
	value.RefreshToken = randomSecret()
	value.ExpiresAt = s.now().Add(accessDuration)
	value.RefreshExpiresAt = s.now().Add(refreshDuration)
	_, err := tx.ExecContext(ctx, `UPDATE sessions SET expires_at=?,refresh_hash=?,refresh_expires_at=?,absolute_expires_at=? WHERE token_hash=?`, value.ExpiresAt.Unix(), secretHash(value.RefreshToken), value.RefreshExpiresAt.Unix(), s.now().Add(sessionDuration).Unix(), secretHash(value.Token))
	return value, err
}

// 同一刷新请求的响应可重建，网络中断后不需要保存明文令牌或放宽旧凭据的权限。
func (s *Store) rotatedSecret(secret, request, purpose string) string {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte("session-refresh\x00" + purpose + "\x00" + secret + "\x00" + request))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func (s *Store) RefreshSession(ctx context.Context, secret, request string) (Session, error) {
	if len(secret) != 43 {
		return Session{}, ErrUnauthenticated
	}
	id, err := uuid.Parse(request)
	if err != nil || id.String() != request {
		return Session{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	hash := secretHash(secret)
	// 先取得写锁；并发刷新和撤销必须在同一行上串行。
	if _, err = tx.ExecContext(ctx, `UPDATE sessions SET refresh_request=refresh_request WHERE refresh_hash=? OR previous_refresh_hash=?`, hash, hash); err != nil {
		return Session{}, err
	}
	var current, previous, priorRequest, access string
	var expires, refreshExpires, absolute int64
	var user Identity
	err = tx.QueryRowContext(ctx, `SELECT s.token_hash,s.refresh_hash,s.previous_refresh_hash,s.refresh_request,s.expires_at,s.refresh_expires_at,s.absolute_expires_at,u.id,u.username,u.administrator,EXISTS(SELECT 1 FROM external_identities e WHERE e.user_id=u.id) FROM sessions s JOIN users u ON u.id=s.user_id WHERE (s.refresh_hash=? OR s.previous_refresh_hash=?) AND u.disabled=0`, hash, hash).Scan(&access, &current, &previous, &priorRequest, &expires, &refreshExpires, &absolute, &user.ID, &user.Username, &user.Administrator, &user.Federated)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrUnauthenticated
	}
	if err != nil {
		return Session{}, err
	}
	if s.now().Unix() >= refreshExpires || s.now().Unix() >= absolute {
		return Session{}, ErrUnauthenticated
	}
	value := Session{Token: s.rotatedSecret(secret, request, "access"), RefreshToken: s.rotatedSecret(secret, request, "refresh"), Identity: user}
	if hash == previous {
		if request != priorRequest {
			if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", access); err != nil {
				return Session{}, err
			}
			if err = audit(ctx, tx, user.ID, "session.refresh_reuse", user.ID, s.now()); err != nil {
				return Session{}, err
			}
			if err = tx.Commit(); err != nil {
				return Session{}, err
			}
			return Session{}, ErrUnauthenticated
		}
		value.ExpiresAt = time.Unix(expires, 0)
		value.RefreshExpiresAt = time.Unix(refreshExpires, 0)
		return value, tx.Commit()
	}
	value.ExpiresAt = time.Unix(min(s.now().Add(accessDuration).Unix(), absolute), 0)
	value.RefreshExpiresAt = time.Unix(min(s.now().Add(refreshDuration).Unix(), absolute), 0)
	_, err = tx.ExecContext(ctx, `UPDATE sessions SET token_hash=?,expires_at=?,refresh_hash=?,previous_refresh_hash=?,refresh_request=?,refresh_expires_at=?,previous_access_hash=?,previous_access_expires_at=? WHERE token_hash=?`, secretHash(value.Token), value.ExpiresAt.Unix(), secretHash(value.RefreshToken), hash, request, value.RefreshExpiresAt.Unix(), access, min(expires, s.now().Add(time.Minute).Unix()), access)
	if err != nil {
		return Session{}, err
	}
	return value, tx.Commit()
}

// 刷新秘密只允许撤销所属会话，不能用作业务请求的访问凭据。
func (s *Store) RevokeRefreshSession(ctx context.Context, secret string) error {
	if len(secret) != 43 {
		return ErrUnauthenticated
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	hash := secretHash(secret)
	var user string
	err = tx.QueryRowContext(ctx, "SELECT user_id FROM sessions WHERE refresh_hash=? OR previous_refresh_hash=?", hash, hash).Scan(&user)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE refresh_hash=? OR previous_refresh_hash=?", hash, hash); err != nil {
		return err
	}
	if err = audit(ctx, tx, user, "session.revoke", user, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}
