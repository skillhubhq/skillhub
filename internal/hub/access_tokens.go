package hub

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
)

type AccessToken struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Scope      string `json:"scope"`
	Namespace  string `json:"namespace"`
	ExpiresAt  int64  `json:"expires_at"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt int64  `json:"last_used_at"`
	Revoked    bool   `json:"revoked"`
}

func (s *Store) CreateAccessToken(ctx context.Context, actor Identity, name, scope, namespace string, days uint32) (AccessToken, string, error) {
	v := AccessToken{}
	name = strings.TrimSpace(name)
	if actor.AutomationID != "" {
		return v, "", ErrForbidden
	}
	if !validDisplayName(name) || name == "" || utf8.RuneCountInString(name) > 64 || strings.ContainsAny(name, "\r\n\t") || days < 1 || days > 90 || (scope != "read" && scope != "publish") || (scope == "read" && namespace != "") {
		return v, "", ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return v, "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE instance SET id=id"); err != nil {
		return v, "", err
	}
	u, err := currentIdentity(ctx, tx, actor)
	if err != nil {
		return v, "", err
	}
	if err = authenticatedOnly(tx, u); err != nil {
		return v, "", err
	}
	if scope == "publish" {
		if err = requireRole(ctx, tx, u, namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER); err != nil {
			return v, "", err
		}
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM access_tokens WHERE user_id=? AND revoked=0 AND expires_at>?", u.ID, s.now().Unix()).Scan(&count); err != nil {
		return v, "", err
	}
	if count >= 20 {
		return v, "", ErrRateLimited
	}
	v = AccessToken{ID: uuid.NewString(), Name: name, Scope: scope, Namespace: namespace, CreatedAt: s.now().Unix(), ExpiresAt: s.now().Unix() + int64(days)*86400}
	secret := "sht_" + randomSecret()
	_, err = tx.ExecContext(ctx, "INSERT INTO access_tokens(id,token_hash,user_id,name,scope,namespace,expires_at,created_at) VALUES(?,?,?,?,?,?,?,?)", v.ID, secretHash(secret), u.ID, v.Name, v.Scope, v.Namespace, v.ExpiresAt, v.CreatedAt)
	if err != nil {
		return AccessToken{}, "", err
	}
	if err = audit(ctx, tx, u.ID, "token.create", v.ID, s.now()); err != nil {
		return AccessToken{}, "", err
	}
	if err = tx.Commit(); err != nil {
		return AccessToken{}, "", err
	}
	return v, secret, nil
}
func (s *Store) ListAccessTokens(ctx context.Context, actor Identity) ([]AccessToken, error) {
	result := []AccessToken{}
	if actor.AutomationID != "" {
		return nil, ErrForbidden
	}
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = authenticatedOnly(tx, u); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,name,scope,namespace,expires_at,created_at,last_used_at,revoked FROM access_tokens WHERE user_id=? ORDER BY revoked,expires_at DESC,id LIMIT 100", u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v AccessToken
		if err = rows.Scan(&v.ID, &v.Name, &v.Scope, &v.Namespace, &v.ExpiresAt, &v.CreatedAt, &v.LastUsedAt, &v.Revoked); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) RevokeAccessToken(ctx context.Context, actor Identity, id string) error {
	if actor.AutomationID != "" {
		return ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE instance SET id=id"); err != nil {
		return err
	}
	u, err := currentIdentity(ctx, tx, actor)
	if err != nil {
		return err
	}
	if err = authenticatedOnly(tx, u); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE access_tokens SET revoked=1 WHERE id=? AND user_id=?", id, u.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if err = audit(ctx, tx, u.ID, "token.revoke", id, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) authenticateAccessToken(ctx context.Context, token string) (Identity, error) {
	var u Identity
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.username,u.administrator,t.id,t.scope,t.namespace FROM access_tokens t JOIN users u ON u.id=t.user_id WHERE t.token_hash=? AND t.revoked=0 AND t.expires_at>? AND u.disabled=0 AND NOT EXISTS(SELECT 1 FROM access_requests a WHERE a.user_id=u.id)`, secretHash(token), s.now().Unix()).Scan(&u.ID, &u.Username, &u.Administrator, &u.AutomationID, &u.AutomationScope, &u.AutomationNamespace)
	if errors.Is(err, sql.ErrNoRows) {
		return Identity{}, ErrUnauthenticated
	}
	if err != nil {
		return Identity{}, err
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE access_tokens SET last_used_at=? WHERE id=?", s.now().Unix(), u.AutomationID); err != nil {
		return Identity{}, err
	}
	return u, nil
}
