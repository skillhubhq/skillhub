// Package hubdb 隔离数据库连接、参数绑定与受支持的 SQL 方言。
package hubdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/skillhubhq/skillhub/internal/sqliteutil"
)

type Config struct {
	Driver string
	DSN    string
}
type DB struct {
	pool      *sql.DB
	Driver    string
	conn      *sql.Conn
	gate      chan struct{}
	lost      chan struct{}
	cancel    context.CancelFunc
	loseOnce  sync.Once
	closeOnce sync.Once
}
type Tx struct {
	*sql.Tx
	driver  string
	release func()
	once    sync.Once
}
type Row struct {
	row     *sql.Row
	err     error
	release func()
	once    sync.Once
}

func (r *Row) Scan(dest ...any) error {
	if r.release != nil {
		defer r.once.Do(r.release)
	}
	if r.err != nil {
		return r.err
	}
	return r.row.Scan(dest...)
}
func Open(root string, c Config) (*DB, error) {
	if c.Driver == "" {
		c.Driver = "sqlite"
	}
	var db *sql.DB
	var err error
	switch c.Driver {
	case "sqlite":
		if c.DSN != "" {
			return nil, errors.New("SQLite uses the data directory, not a DSN")
		}
		db, err = sqliteutil.Open(filepath.Join(root, "hub.db"))
	case "postgres":
		if c.DSN == "" {
			return nil, errors.New("PostgreSQL DSN is required")
		}
		cfg, e := pgx.ParseConfig(c.DSN)
		if e != nil {
			return nil, errors.New("invalid PostgreSQL connection configuration")
		}
		cfg.ConnectTimeout = 10 * time.Second
		db = stdlib.OpenDB(*cfg)
		db.SetMaxOpenConns(8)
		db.SetMaxIdleConns(4)
		db.SetConnMaxLifetime(30 * time.Minute)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if e = db.PingContext(ctx); e != nil {
			_ = db.Close()
			return nil, errors.New("cannot connect to PostgreSQL; check connection settings and server availability")
		}
	default:
		return nil, errors.New("unsupported database driver; choose sqlite or postgres")
	}
	if err != nil {
		return nil, err
	}
	d := &DB{pool: db, Driver: c.Driver}
	if c.Driver == "postgres" {
		if err := d.acquire(); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return d, nil
}
func (d *DB) Begin() (*Tx, error) { return d.BeginTx(context.Background(), nil) }
func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	if d.Driver == "postgres" && opts != nil && opts.ReadOnly {
		copy := *opts
		copy.Isolation = sql.LevelRepeatableRead
		opts = &copy
	}
	release, err := d.enter(ctx)
	if err != nil {
		return nil, err
	}
	var tx *sql.Tx
	if d.conn != nil {
		tx, err = d.conn.BeginTx(ctx, opts)
	} else {
		tx, err = d.pool.BeginTx(ctx, opts)
	}
	if err != nil {
		release()
		return nil, err
	}
	if d.Driver == "postgres" && (opts == nil || !opts.ReadOnly) {
		// 所有 Hub 写事务共用数据库内锁，保持跨连接的幂等、准入与最后管理员约束。
		if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(735071204)"); err != nil {
			_ = tx.Rollback()
			release()
			return nil, err
		}
	}
	return &Tx{Tx: tx, driver: d.Driver, release: release}, nil
}
func (d *DB) Exec(q string, args ...any) (sql.Result, error) {
	return d.ExecContext(context.Background(), q, args...)
}
func (d *DB) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	q, args, err := bind(d.Driver, q, args)
	if err != nil {
		return nil, err
	}
	release, err := d.enter(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if d.conn != nil {
		return d.conn.ExecContext(ctx, q, args...)
	}
	return d.pool.ExecContext(ctx, q, args...)
}
func (d *DB) Query(q string, args ...any) (*Rows, error) {
	return d.QueryContext(context.Background(), q, args...)
}
func (d *DB) QueryContext(ctx context.Context, q string, args ...any) (*Rows, error) {
	q, args, err := bind(d.Driver, q, args)
	if err != nil {
		return nil, err
	}
	release, err := d.enter(ctx)
	if err != nil {
		return nil, err
	}
	var rows *sql.Rows
	if d.conn != nil {
		rows, err = d.conn.QueryContext(ctx, q, args...)
	} else {
		rows, err = d.pool.QueryContext(ctx, q, args...)
	}
	if err != nil {
		release()
		return nil, err
	}
	return &Rows{Rows: rows, release: release}, nil
}
func (d *DB) QueryRow(q string, args ...any) *Row {
	return d.QueryRowContext(context.Background(), q, args...)
}
func (d *DB) QueryRowContext(ctx context.Context, q string, args ...any) *Row {
	q, args, err := bind(d.Driver, q, args)
	if err != nil {
		return &Row{err: err}
	}
	release, err := d.enter(ctx)
	if err != nil {
		return &Row{err: err}
	}
	if d.conn != nil {
		return &Row{row: d.conn.QueryRowContext(ctx, q, args...), release: release}
	}
	return &Row{row: d.pool.QueryRowContext(ctx, q, args...), release: release}
}
func (t *Tx) Exec(q string, args ...any) (sql.Result, error) {
	return t.ExecContext(context.Background(), q, args...)
}
func (t *Tx) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	q, args, err := bind(t.driver, q, args)
	if err != nil {
		return nil, err
	}
	return t.Tx.ExecContext(ctx, q, args...)
}
func (t *Tx) Query(q string, args ...any) (*sql.Rows, error) {
	return t.QueryContext(context.Background(), q, args...)
}
func (t *Tx) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	q, args, err := bind(t.driver, q, args)
	if err != nil {
		return nil, err
	}
	return t.Tx.QueryContext(ctx, q, args...)
}
func (t *Tx) QueryRow(q string, args ...any) *Row {
	return t.QueryRowContext(context.Background(), q, args...)
}
func (t *Tx) QueryRowContext(ctx context.Context, q string, args ...any) *Row {
	q, args, err := bind(t.driver, q, args)
	if err != nil {
		return &Row{err: err}
	}
	return &Row{row: t.Tx.QueryRowContext(ctx, q, args...)}
}

var jsonAggregate = regexp.MustCompile(`json_group_array\(([^()]*)\)`)
var jsonElements = regexp.MustCompile(`json_each\(([@a-zA-Z0-9_.]+)\) ([a-zA-Z_]+)`)

var schemaVersion = regexp.MustCompile(`PRAGMA user_version=(\d+);`)

// 方言转换仅接受仓库内固定 SQL，不是用户 SQL 翻译器；新增方言必须通过真实行为测试。
func postgresSQL(q string) string {
	q = strings.ReplaceAll(q, "INTEGER PRIMARY KEY AUTOINCREMENT", "BIGSERIAL PRIMARY KEY")
	q = strings.ReplaceAll(q, "INTEGER", "BIGINT")
	q = strings.ReplaceAll(q, "BLOB", "BYTEA")
	q = schemaVersion.ReplaceAllString(q, "UPDATE hub_schema SET version=$1 WHERE id=1;")
	q = strings.ReplaceAll(q, "PRAGMA user_version", "SELECT version FROM hub_schema WHERE id=1")
	q = jsonAggregate.ReplaceAllString(q, "CAST(json_agg($1) AS TEXT)")
	q = jsonElements.ReplaceAllString(q, "json_array_elements_text(CAST($1 AS json)) AS $2(value)")
	q = strings.ReplaceAll(q, "instr(", "strpos(")
	q = strings.ReplaceAll(q, "AND cursor_key>@after", "AND (kind||':'||subject_id||':'||role)>@after")
	if strings.HasPrefix(strings.TrimSpace(q), "INSERT OR IGNORE ") {
		q = strings.Replace(q, "INSERT OR IGNORE ", "INSERT ", 1) + " ON CONFLICT DO NOTHING"
	}
	return q
}
func bind(driver, q string, args []any) (string, []any, error) {
	if driver != "postgres" {
		return q, args, nil
	}
	q = postgresSQL(q)
	named := map[string]any{}
	positional := []any{}
	normalize := func(v any) any {
		if b, ok := v.(bool); ok {
			if b {
				return int64(1)
			}
			return int64(0)
		}
		return v
	}
	for _, arg := range args {
		if n, ok := arg.(sql.NamedArg); ok {
			named[n.Name] = normalize(n.Value)
		} else {
			positional = append(positional, normalize(arg))
		}
	}
	var out strings.Builder
	bound := []any{}
	names := map[string]int{}
	at := 0
	// 参数标记只在 SQL 代码中识别，保留字符串与带引号的标识符。
	for i := 0; i < len(q); {
		if q[i] == '\'' || q[i] == '"' {
			quote := q[i]
			out.WriteByte(quote)
			i++
			for i < len(q) {
				ch := q[i]
				out.WriteByte(ch)
				i++
				if ch == quote {
					if i < len(q) && q[i] == quote {
						out.WriteByte(q[i])
						i++
						continue
					}
					break
				}
			}
			continue
		}
		if q[i] == '?' {
			if at >= len(positional) {
				return "", nil, errors.New("missing positional SQL parameter")
			}
			bound = append(bound, positional[at])
			at++
			fmt.Fprintf(&out, "$%d", len(bound))
			i++
			continue
		}
		if q[i] == '@' {
			j := i + 1
			for j < len(q) && ((q[j] >= 'a' && q[j] <= 'z') || (q[j] >= 'A' && q[j] <= 'Z') || (q[j] >= '0' && q[j] <= '9') || q[j] == '_') {
				j++
			}
			name := q[i+1 : j]
			v, ok := named[name]
			if !ok {
				return "", nil, errors.New("missing named SQL parameter")
			}
			n, exists := names[name]
			if !exists {
				bound = append(bound, v)
				n = len(bound)
				names[name] = n
			}
			fmt.Fprintf(&out, "$%d", n)
			i = j
			continue
		}
		out.WriteByte(q[i])
		i++
	}
	if at != len(positional) {
		return "", nil, errors.New("unused positional SQL parameter")
	}
	return out.String(), bound, nil
}
