package hubdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"time"
)

var ErrLeaseLost = errors.New("PostgreSQL ownership connection lost; restart required")

// PostgreSQL 的 DDL、查询和事务都走同一持锁连接，禁止连接池重连绕过失锁。
// gate 覆盖整个事务/结果集生命期；单节点轻量部署选择确定性隔离而非并行数据库查询。
func (d *DB) acquire() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := d.pool.Conn(ctx)
	if err != nil {
		return err
	}
	var schema string
	if err = conn.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil || schema == "" {
		_ = conn.Close()
		return errors.New("PostgreSQL requires an existing explicit schema")
	}
	var locked bool
	err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(735071205, hashtext(current_schema()))").Scan(&locked)
	if err != nil || !locked {
		_ = conn.Close()
		return errors.New("another Hub process owns this PostgreSQL schema")
	}
	d.conn = conn
	d.gate = make(chan struct{}, 1)
	d.lost = make(chan struct{})
	watch, stop := context.WithCancel(context.Background())
	d.cancel = stop
	go d.watchLease(watch)
	return nil
}
func (d *DB) Lost() <-chan struct{} { return d.lost }
func (d *DB) lose()                 { d.loseOnce.Do(func() { close(d.lost) }) }
func (d *DB) enter(ctx context.Context) (func(), error) {
	if d.conn == nil {
		return func() {}, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-d.lost:
		return nil, ErrLeaseLost
	case d.gate <- struct{}{}:
	}
	select {
	case <-d.lost:
		<-d.gate
		return nil, ErrLeaseLost
	default:
	}
	return func() { <-d.gate }, nil
}
func (d *DB) watchLease(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		// 活跃事务已在持锁连接上执行；不向事务中插入健康查询。
		select {
		case d.gate <- struct{}{}:
		default:
			continue
		}
		probe, cancel := context.WithTimeout(ctx, 3*time.Second)
		var held bool
		err := d.conn.QueryRowContext(probe, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory' AND classid=735071205 AND objid=hashtext(current_schema())::oid AND granted)`).Scan(&held)
		cancel()
		<-d.gate
		if ctx.Err() != nil {
			return
		}
		if err != nil || !held {
			d.lose()
			return
		}
	}
}
func (d *DB) Close() (err error) {
	d.closeOnce.Do(func() {
		if d.conn != nil {
			d.cancel()
			d.lose()
			// 直接关闭持锁会话，不将仍持锁的连接归还池中。
			// 等待事务结束后丢弃物理连接，防止仍持锁的连接归还池。
			d.gate <- struct{}{}
			_ = d.conn.Raw(func(any) error { return driver.ErrBadConn })
			_ = d.conn.Close()
			<-d.gate
		}
		err = d.pool.Close()
	})
	// 已关闭的会话是正常关闭，不暴露驱动内部错误。
	if errors.Is(err, sql.ErrConnDone) {
		return nil
	}
	return err
}
func (t *Tx) Commit() error   { err := t.Tx.Commit(); t.finish(); return err }
func (t *Tx) Rollback() error { err := t.Tx.Rollback(); t.finish(); return err }
func (t *Tx) finish() {
	t.once.Do(func() {
		if t.release != nil {
			t.release()
		}
	})
}

type Rows struct {
	*sql.Rows
	release func()
	once    sync.Once
}

func (r *Rows) Close() error { err := r.Rows.Close(); r.once.Do(r.release); return err }
func (r *Rows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		_ = r.Close()
	}
	return ok
}

// 数据库标记防止另一个包目录绕过中断恢复的文件标记。
func (d *DB) CheckMaintenance(ctx context.Context) error {
	if d.Driver != "postgres" {
		return nil
	}
	var incomplete bool
	if err := d.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_class WHERE relnamespace=current_schema()::regnamespace AND relname='skillhub_recovery')").Scan(&incomplete); err != nil {
		return err
	}
	if incomplete {
		return errors.New("PostgreSQL recovery is incomplete; use a new restore destination")
	}
	return nil
}
