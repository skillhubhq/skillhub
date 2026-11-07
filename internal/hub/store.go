// Package hub 实现独立 Hub 的领域逻辑和持久化。
package hub

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/hubpolicy"
	"golang.org/x/crypto/bcrypt"
)

const currentSchema = 1

var (
	ErrUnauthenticated = errors.New("authentication required")
	ErrInvalid         = errors.New("invalid request")
	ErrNotFound        = errors.New("resource not found")
	ErrConflict        = errors.New("resource already exists or changed")
	ErrForbidden       = errors.New("permission denied")
	ErrPendingApproval = errors.New("enterprise access is awaiting administrator approval")
	ErrScanRequired    = errors.New("a current scan without blocking findings is required; inspect the scan report")
	ErrRateLimited     = errors.New("too many attempts; try again later")
)

type Store struct {
	policy        hubpolicy.Policy
	db            *hubdb.DB
	root          string
	registryID    string
	secret        []byte
	dummyPassword []byte
	now           func() time.Time
}

// Identity 不包含密码或凭据摘要，也不依赖 API 消息。
type Identity struct {
	Federated     bool
	ID            string
	Username      string
	Administrator bool
}

type Session struct {
	Token     string
	Identity  Identity
	ExpiresAt time.Time
}

func Open(root string) (*Store, error) { return OpenDatabase(root, hubdb.Config{}) }

func OpenDatabase(root string, config hubdb.Config) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	for _, marker := range []string{maintenanceMarker, backupManifestName} {
		if _, err = os.Lstat(filepath.Join(abs, marker)); err == nil {
			return nil, errors.New("data directory is a backup or unfinished recovery; restore into a new directory before serving")
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	db, err := hubdb.Open(abs, config)
	if err != nil {
		return nil, err
	}
	if err = db.CheckMaintenance(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db, root: abs, now: time.Now}
	if err = s.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if db.Driver == "sqlite" {
		if err = os.Chmod(filepath.Join(abs, "hub.db"), 0600); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	s.dummyPassword, err = bcrypt.GenerateFromPassword([]byte(randomSecret()), bcrypt.DefaultCost)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error       { return s.db.Close() }
func (s *Store) RegistryID() string { return s.registryID }

func (s *Store) initialize() error { return s.initializeContext(context.Background()) }

func (s *Store) initializeContext(ctx context.Context) error {
	if s.db.Driver == "sqlite" {
		if _, err := s.db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.db.Driver == "postgres" {
		if _, err = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS hub_schema (id BIGINT PRIMARY KEY CHECK(id=1), version BIGINT NOT NULL); INSERT INTO hub_schema(id,version) VALUES(1,0) ON CONFLICT DO NOTHING;"); err != nil {
			return err
		}
	}
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 0 && version != currentSchema {
		return errors.New("unsupported hub database schema")
	}
	if version == 0 {
		if _, err = tx.ExecContext(ctx, initialSchema); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO instance(id,secret) VALUES(?,?)", uuid.NewString(), []byte(randomSecret())); err != nil {
			return err
		}
	}
	// 拒绝结构不完整的数据库，不能用 CREATE IF NOT EXISTS 掩盖损坏。
	for _, table := range []string{"instance", "users", "sessions", "devices", "audit", "groups", "group_members", "namespaces", "namespace_grants", "categories", "releases", "release_groups", "release_tags", "mutations", "profiles", "team_profiles", "comments", "reactions", "access_requests", "platform_roles", "release_scans", "review_notifications", "featured_skills", "evaluation_evidence", "personal_namespaces", "community_reports", "external_identities"} {
		rows, e := tx.QueryContext(ctx, "SELECT * FROM "+table+" LIMIT 0")
		if e != nil {
			return errors.New("incomplete hub database schema")
		}
		rows.Close()
	}
	if err = tx.QueryRowContext(ctx, "SELECT id,secret FROM instance").Scan(&s.registryID, &s.secret); err != nil {
		return err
	}

	return tx.Commit()
}

func randomSecret() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func secretHash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func audit(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, actor, action, resource string, at time.Time) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO audit(actor_id,action,resource,created_at) VALUES(?,?,?,?)", actor, action, resource, at.Unix())
	return err
}

// Bootstrap 只允许空实例创建首个管理员；并发初始化由唯一事务保护。
func (s *Store) Bootstrap(ctx context.Context, username, password string) (Identity, error) {
	if !validUsername(username) || len(password) < 12 || len(password) > 72 {
		return Identity{}, ErrInvalid
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Identity{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Identity{}, err
	}
	defer tx.Rollback()
	// 首先取得写锁，避免两个进程同时读到空实例。
	if _, err = tx.ExecContext(ctx, "UPDATE instance SET id=id"); err != nil {
		return Identity{}, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
		return Identity{}, err
	}
	if count != 0 {
		return Identity{}, ErrConflict
	}
	user := Identity{ID: uuid.NewString(), Username: username, Administrator: true}
	if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,username,password_hash,administrator) VALUES(?,?,?,1)", user.ID, user.Username, hash); err != nil {
		return Identity{}, err
	}
	if err = audit(ctx, tx, user.ID, "instance.bootstrap", s.registryID, s.now()); err != nil {
		return Identity{}, err
	}
	return user, tx.Commit()
}

func validUsername(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' || r == '@') {
			return false
		}
	}
	return true
}

func (s *Store) verifyPassword(ctx context.Context, username, password string) (Identity, error) {
	var user Identity
	var hash []byte
	var disabled bool
	err := s.db.QueryRowContext(ctx, "SELECT id,username,password_hash,administrator,disabled FROM users WHERE username=?", username).Scan(&user.ID, &user.Username, &hash, &user.Administrator, &disabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Identity{}, err
	}
	if err != nil {
		hash = s.dummyPassword
	}
	passwordErr := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if err != nil || disabled || passwordErr != nil {
		return Identity{}, ErrUnauthenticated
	}
	return user, nil
}

func (s *Store) newSession(ctx context.Context, tx *hubdb.Tx, user Identity) (Session, error) {
	if err := s.provisionPersonalNamespace(ctx, tx, user); err != nil {
		return Session{}, err
	}
	value := Session{Token: randomSecret(), Identity: user, ExpiresAt: s.now().Add(30 * 24 * time.Hour)}
	var external bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM external_identities WHERE user_id=?)", user.ID).Scan(&external); err != nil {
		return Session{}, err
	}
	value.Identity.Federated = external
	if external {
		value.ExpiresAt = s.now().Add(federatedSessionDuration)
	}
	// 账号状态在写入凭据时再检查，关闭密码校验与禁用账号之间的窗口。
	result, err := tx.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) SELECT ?,id,? FROM users WHERE id=? AND disabled=0", secretHash(value.Token), value.ExpiresAt.Unix(), user.ID)
	if err != nil {
		return Session{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Session{}, err
	}
	if n != 1 {
		return Session{}, ErrUnauthenticated
	}
	if err = audit(ctx, tx, user.ID, "session.create", user.ID, s.now()); err != nil {
		return Session{}, err
	}
	return value, nil
}

func (s *Store) Login(ctx context.Context, username, password string) (Session, error) {
	user, err := s.verifyPassword(ctx, username, password)
	if err != nil {
		return Session{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<=?", s.now().Unix()); err != nil {
		return Session{}, err
	}
	// 写锁内重验密码，防止并发重置后使用旧密码签发新会话。
	var currentHash []byte
	if err = tx.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id=?", user.ID).Scan(&currentHash); err != nil {
		return Session{}, err
	}
	if bcrypt.CompareHashAndPassword(currentHash, []byte(password)) != nil {
		return Session{}, ErrUnauthenticated
	}
	value, err := s.newSession(ctx, tx, user)
	if err != nil {
		return Session{}, err
	}
	return value, tx.Commit()
}

func (s *Store) Authenticate(ctx context.Context, token string) (Identity, error) {
	if len(token) != 43 {
		return Identity{}, ErrUnauthenticated
	}
	var user Identity
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.username,u.administrator,EXISTS(SELECT 1 FROM external_identities e WHERE e.user_id=u.id) FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>? AND u.disabled=0`, secretHash(token), s.now().Unix()).Scan(&user.ID, &user.Username, &user.Administrator, &user.Federated)
	if errors.Is(err, sql.ErrNoRows) {
		return Identity{}, ErrUnauthenticated
	}
	return user, err
}

func (s *Store) Logout(ctx context.Context, token string) error {
	user, err := s.Authenticate(ctx, token)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", secretHash(token)); err != nil {
		return err
	}
	if err = audit(ctx, tx, user.ID, "session.revoke", user.ID, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DatabaseLeaseLost() <-chan struct{} { return s.db.Lost() }
