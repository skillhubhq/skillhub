package client

import (
	"context"
	"database/sql"
	"errors"
)

// 启动迁移先取得 SQLite 写锁，再判断版本；不向未来版本追加旧表。
func migrateClient(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, "ROLLBACK")
	var exists int
	if err = conn.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='client_schema'").Scan(&exists); err != nil {
		return err
	}
	version := 0
	if exists != 0 {
		var count int
		if err = conn.QueryRowContext(ctx, "SELECT count(*), coalesce(min(version),0) FROM client_schema").Scan(&count, &version); err != nil {
			return err
		}
		if count != 1 || version < 1 || version > 2 {
			return errors.New("unsupported client database schema")
		}
	}
	if version == 0 {
		_, err = conn.ExecContext(ctx, `CREATE TABLE client_schema (version INTEGER NOT NULL);
INSERT INTO client_schema(version) VALUES(1);
CREATE TABLE installations (target TEXT PRIMARY KEY, id TEXT UNIQUE NOT NULL, record TEXT NOT NULL);
CREATE TABLE install_journals (target TEXT PRIMARY KEY, record TEXT NOT NULL);
CREATE TABLE sources (alias TEXT PRIMARY KEY, registry_id TEXT NOT NULL, record TEXT NOT NULL);`)
		if err != nil {
			return err
		}
		version = 1
	}
	if version == 1 {
		if _, err = conn.ExecContext(ctx, `CREATE TABLE projects (root TEXT PRIMARY KEY);
UPDATE client_schema SET version=2;`); err != nil {
			return err
		}
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}
