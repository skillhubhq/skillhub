package hub

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
)

type Page struct {
	Size  int
	Token string
}
type PageResult[T any] struct {
	Items []T
	Next  string
}
type cursor struct {
	Scope  string `json:"s"`
	After  string `json:"a"`
	Expiry int64  `json:"e"`
}

func (s *Store) sign(data []byte) []byte {
	h := hmac.New(sha256.New, s.secret)
	_, _ = h.Write(data)
	return h.Sum(nil)
}
func scope(actor, method string, filters any) string {
	raw, _ := json.Marshal([]any{actor, method, filters})
	return secretHash(string(raw))
}

func (s *Store) pageAfter(page Page, scope string) (string, int, error) {
	size := page.Size
	if size == 0 {
		size = 30
	}
	if size < 1 || size > 100 || len(page.Token) > 2048 {
		return "", 0, ErrInvalid
	}
	if page.Token == "" {
		return "", size, nil
	}
	parts := strings.Split(page.Token, ".")
	if len(parts) != 2 {
		return "", 0, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", 0, ErrInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(sig, s.sign(raw)) {
		return "", 0, ErrInvalid
	}
	var value cursor
	if err = json.Unmarshal(raw, &value); err != nil || value.Scope != scope || value.Expiry <= s.now().Unix() {
		return "", 0, ErrInvalid
	}
	return value.After, size, nil
}

func (s *Store) nextPage(scope, after string) string {
	raw, _ := json.Marshal(cursor{Scope: scope, After: after, Expiry: s.now().Add(time.Hour).Unix()})
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(s.sign(raw))
}

func pageQuery[T any](s *Store, ctx context.Context, tx *hubdb.Tx, page Page, scope, query string, args []any, scan func(*sql.Rows) (T, string, error)) (PageResult[T], error) {
	result := PageResult[T]{Items: []T{}}
	after, size, err := s.pageAfter(page, scope)
	if err != nil {
		return result, err
	}
	args = append(args, sql.Named("after", after), sql.Named("limit", size+1))
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	last := ""
	for rows.Next() {
		item, key, err := scan(rows)
		if err != nil {
			return result, err
		}
		if len(result.Items) == size {
			result.Next = s.nextPage(scope, last)
			break
		}
		result.Items = append(result.Items, item)
		last = key
	}
	return result, rows.Err()
}

func currentIdentity(ctx context.Context, tx *hubdb.Tx, user Identity) (Identity, error) {
	if user.ID == "" {
		return Identity{}, nil
	}
	var current Identity
	err := tx.QueryRowContext(ctx, "SELECT id,username,administrator FROM users WHERE id=? AND disabled=0", user.ID).Scan(&current.ID, &current.Username, &current.Administrator)
	if errors.Is(err, sql.ErrNoRows) {
		return Identity{}, ErrUnauthenticated
	}
	return current, err
}

func (s *Store) read(ctx context.Context, user Identity) (*hubdb.Tx, Identity, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, Identity{}, err
	}
	current, err := currentIdentity(ctx, tx, user)
	if err != nil {
		_ = tx.Rollback()
		return nil, Identity{}, err
	}
	return tx, current, nil
}

// mutate 在同一写事务中重新授权、核对幂等摘要、保存结果，重放不能绕过权限撤销。
func mutate[T any](s *Store, ctx context.Context, user Identity, method, key string, payload any, authorize func(*hubdb.Tx, Identity) error, apply func(*hubdb.Tx, Identity) (T, error)) (T, error) {
	var zero T
	if user.ID == "" {
		return zero, ErrUnauthenticated
	}
	if len(key) < 8 || len(key) > 128 {
		return zero, ErrInvalid
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return zero, err
	}
	fingerprint := hex.EncodeToString(s.sign(raw))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE instance SET id=id"); err != nil {
		return zero, err
	}
	current, err := currentIdentity(ctx, tx, user)
	if err != nil {
		return zero, err
	}
	if err = authorize(tx, current); err != nil {
		return zero, err
	}
	var previous string
	var response []byte
	err = tx.QueryRowContext(ctx, "SELECT fingerprint,result FROM mutations WHERE actor=? AND method=? AND key=?", user.ID, method, key).Scan(&previous, &response)
	if err == nil {
		if previous != fingerprint {
			return zero, ErrConflict
		}
		var result T
		if err = json.Unmarshal(response, &result); err != nil {
			return zero, err
		}
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return zero, err
	}
	result, err := apply(tx, current)
	if err != nil {
		return zero, err
	}
	response, err = json.Marshal(result)
	if err != nil {
		return zero, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO mutations(actor,method,key,fingerprint,result) VALUES(?,?,?,?,?)", user.ID, method, key, fingerprint, response); err != nil {
		return zero, err
	}
	if err = tx.Commit(); err != nil {
		return zero, err
	}
	return result, nil
}

func adminOnly(_ *hubdb.Tx, user Identity) error {
	if !user.Administrator {
		return ErrForbidden
	}
	return nil
}
func authenticatedOnly(_ *hubdb.Tx, user Identity) error {
	if user.ID == "" {
		return ErrUnauthenticated
	}
	return nil
}

func requireRole(ctx context.Context, tx *hubdb.Tx, user Identity, namespace string, roles ...taxonomy.NamespaceRole) error {
	if user.ID == "" {
		return ErrUnauthenticated
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM namespaces WHERE name=? AND archived=0)", namespace).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if user.Administrator {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT role FROM namespace_grants g WHERE namespace=? AND ((kind=1 AND subject_id=?) OR (kind=2 AND EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.subject_id AND m.user_id=?)))`, namespace, user.ID, user.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var role taxonomy.NamespaceRole
		if err = rows.Scan(&role); err != nil {
			return err
		}
		if role == taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN {
			return nil
		}
		for _, allowed := range roles {
			if role == allowed {
				return nil
			}
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return ErrForbidden
}

func isUnique(err error) bool {
	type sqlStater interface{ SQLState() string }
	var state sqlStater
	if errors.As(err, &state) && state.SQLState() == "23505" {
		return true
	}
	type coder interface{ Code() int }
	var value coder
	return errors.As(err, &value) && (value.Code() == 1555 || value.Code() == 2067)
}
