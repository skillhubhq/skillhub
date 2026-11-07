// Package client 是 CLI 与桌面共同使用的客户端内核。
package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"os"
	"path/filepath"

	"github.com/skillhubhq/skillhub/internal/sqliteutil"
)

type Store struct {
	db   *sql.DB
	root string
}

// Installation 以实际目录为唯一物理所有权边界。
type Installation struct {
	DisplayNameEN string             `json:"display_name_en"`
	DisplayNameZH string             `json:"display_name_zh"`
	Icon          taxonomy.SkillIcon `json:"icon"`
	ID            string             `json:"id"`
	Source        string             `json:"source,omitempty"`
	SourceRef     string             `json:"source_ref,omitempty"`
	RegistryID    string             `json:"registry_id"`
	RegistryURL   string             `json:"registry_url"`
	Namespace     string             `json:"namespace"`
	Name          string             `json:"name"`
	Version       string             `json:"version"`
	Digest        string             `json:"digest"`
	Fingerprint   string             `json:"fingerprint"`
	Tool          string             `json:"tool"`
	Scope         string             `json:"scope"`
	Project       string             `json:"project,omitempty"`
	Target        string             `json:"target"`
}

func OpenStore(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	db, err := sqliteutil.Open(filepath.Join(abs, "client.db"))
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = initializeClient(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = os.Chmod(filepath.Join(abs, "client.db"), 0600); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, root: abs}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) installation(ctx context.Context, target string) (*Installation, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT record FROM installations WHERE target=?", target).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record Installation
	if err = json.Unmarshal([]byte(raw), &record); err != nil {
		return nil, err
	}
	if record.Target != target {
		return nil, errors.New("invalid installation record")
	}
	return &record, nil
}

func (s *Store) ListInstallations(ctx context.Context) ([]Installation, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT record FROM installations ORDER BY target")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Installation{}
	for rows.Next() {
		var raw string
		var record Installation
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}
