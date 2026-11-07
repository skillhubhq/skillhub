// Package sqliteutil 集中处理本地 SQLite 的跨平台路径和连接约束。
package sqliteutil

import (
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(filename string) (*sql.DB, error) {
	return open(filename, false)
}

// OpenSnapshot 仅用于已封存且不带 WAL 的数据库副本，避免验证备份时写入源目录。
func OpenSnapshot(filename string) (*sql.DB, error) {
	return open(filename, true)
}

func open(filename string, snapshot bool) (*sql.DB, error) {
	abs, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	path := filepath.ToSlash(abs)
	if strings.HasPrefix(path, "//") {
		return nil, errors.New("SQLite requires a local filesystem path")
	}
	// SQLite 的 Windows URI 需要 /C:/ 而非转义反斜线的 C:\ 路径。
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "trusted_schema(0)")
	if snapshot {
		q.Set("mode", "ro")
		q.Set("immutable", "1")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}
