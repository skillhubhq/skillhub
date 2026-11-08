package hub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

// 格式 2 绑定 PostgreSQL dump、实例身份和完整包摘要集合，不包含部署凭据。
type PostgresBackupInfo struct {
	PostgresMajor int `json:"postgres_major"`
	BackupInfo
	Driver         string   `json:"driver"`
	DatabaseSchema string   `json:"database_schema"`
	Digests        []string `json:"digests"`
}

var pgSchemaName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`)

// PostgreSQL 维护只打开数据库并持锁，不隐式升级；升级前的备份必须保留旧 schema。
func postgresMetadata(ctx context.Context, db *hubdb.DB) (PostgresBackupInfo, error) {
	info := PostgresBackupInfo{Driver: "postgres", BackupInfo: BackupInfo{Format: 2, CreatedAt: time.Now().UTC().Format(time.RFC3339)}}
	var serverVersion int
	if err := db.QueryRowContext(ctx, "SHOW server_version_num").Scan(&serverVersion); err != nil {
		return info, err
	}
	info.PostgresMajor = serverVersion / 10000
	if err := db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&info.DatabaseSchema); err != nil || !pgSchemaName.MatchString(info.DatabaseSchema) {
		return info, errors.New("maintenance requires a simple PostgreSQL schema name")
	}
	if err := db.QueryRowContext(ctx, "SELECT version FROM hub_schema WHERE id=1").Scan(&info.Schema); err != nil || (info.Schema < 1 || info.Schema > currentSchema) {
		return info, errors.New("unsupported PostgreSQL Hub schema")
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*),min(id) FROM instance").Scan(&count, &info.RegistryID); err != nil || count != 1 {
		return info, errors.New("invalid PostgreSQL registry identity")
	}
	rows, err := db.QueryContext(ctx, "SELECT DISTINCT digest FROM releases ORDER BY digest")
	if err != nil {
		return info, err
	}
	defer rows.Close()
	for rows.Next() {
		var digest string
		if err = rows.Scan(&digest); err != nil {
			return info, err
		}
		if !validBackupDigest(digest) {
			return info, errors.New("invalid package digest")
		}
		info.Digests = append(info.Digests, digest)
	}
	info.Packages = len(info.Digests)
	return info, rows.Err()
}
func pgTool(ctx context.Context, db *hubdb.DB, c hubdb.Config, name string, args ...string) error {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	if db != nil {
		go func() {
			select {
			case <-db.Lost():
				cancel()
			case <-child.Done():
			}
		}()
	}
	cmd := exec.CommandContext(child, name, args...)
	// 不继承机器上其他 PG 连接参数；密码只通过子进程环境传递，不出现在 argv 或日志中。
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "PG") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	if c.DSN != "" {
		u, err := url.Parse(c.DSN)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" {
			return errors.New("PostgreSQL maintenance requires a postgres connection URL")
		}
		env := map[string]string{"PGHOST": u.Hostname(), "PGDATABASE": strings.TrimPrefix(u.Path, "/"), "PGCONNECT_TIMEOUT": "10"}
		if u.Port() != "" {
			env["PGPORT"] = u.Port()
		}
		if u.User != nil {
			env["PGUSER"] = u.User.Username()
			if password, ok := u.User.Password(); ok {
				env["PGPASSWORD"] = password
			}
		}
		names := map[string]string{"sslmode": "PGSSLMODE", "sslrootcert": "PGSSLROOTCERT", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY", "connect_timeout": "PGCONNECT_TIMEOUT", "application_name": "PGAPPNAME", "target_session_attrs": "PGTARGETSESSIONATTRS", "channel_binding": "PGCHANNELBINDING", "options": "PGOPTIONS"}
		for key, values := range u.Query() {
			if key == "search_path" {
				continue
			}
			name, ok := names[key]
			if !ok || len(values) != 1 {
				return errors.New("unsupported maintenance connection URL parameter")
			}
			env[name] = values[0]
		}
		for key, value := range env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	// 工具错误可能打印连接信息或数据，统一返回不含秘密的失败说明。
	if err := cmd.Run(); err != nil {
		if child.Err() != nil {
			return errors.New("PostgreSQL maintenance interrupted or ownership lost")
		}
		return errors.New(name + " failed; check tool compatibility, database access and archive integrity")
	}
	return nil
}
func copyPostgresPackages(ctx context.Context, info PostgresBackupInfo, source, dest string) error {
	reader, writer := &Store{root: source}, &Store{root: dest}
	if len(info.Digests) > 0 {
		entry, err := os.Lstat(filepath.Join(source, "packages"))
		if err != nil || !entry.IsDir() || entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("invalid package directory")
		}
	}
	for _, digest := range info.Digests {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := reader.readBlob(digest)
		if err != nil {
			return errors.New("referenced package is missing or corrupt")
		}
		if _, err = packageformat.Inspect(data); err != nil {
			return errors.New("invalid referenced package")
		}
		if dest != "" {
			if err = writer.writeBlob(digest, data); err != nil {
				return err
			}
		}
	}
	return nil
}
func BackupPostgres(ctx context.Context, c hubdb.Config, root, destination string) (info PostgresBackupInfo, err error) {
	if c.Driver != "postgres" {
		return info, errors.New("PostgreSQL configuration required")
	}
	unlock, err := AcquireServingLock(root)
	if err != nil {
		return info, err
	}
	defer unlock()
	db, err := hubdb.Open(root, c)
	if err != nil {
		return info, err
	}
	defer db.Close()
	return backupPostgresLocked(ctx, db, c, root, destination)
}

// 专用 schema 禁止混入其他应用表，避免维护工具误带非 Hub 数据。
func checkPostgresTables(ctx context.Context, db *hubdb.DB) error {
	allowed := strings.Fields("hub_schema instance users sessions devices audit groups group_members namespaces namespace_grants categories releases release_groups release_tags mutations profiles team_profiles comments reactions external_identities access_requests platform_roles release_scans review_notifications featured_skills evaluation_evidence personal_namespaces community_reports namespace_tombstones package_cleanup release_tombstones")
	rows, err := db.QueryContext(ctx, "SELECT relname,relkind FROM pg_class WHERE relnamespace=current_schema()::regnamespace AND relkind IN ('r','p','v','m','f')")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			return err
		}
		found := false
		for _, a := range allowed {
			if name == a && kind == "r" {
				found = true
				break
			}
		}
		if !found {
			return errors.New("maintenance requires a dedicated Hub schema without unrelated tables or views")
		}
	}
	return rows.Err()
}
func backupPostgresLocked(ctx context.Context, db *hubdb.DB, c hubdb.Config, root, destination string) (info PostgresBackupInfo, err error) {
	if err = db.CheckMaintenance(ctx); err != nil {
		return info, err
	}
	if err = checkPostgresTables(ctx, db); err != nil {
		return info, err
	}
	info, err = postgresMetadata(ctx, db)
	if err != nil {
		return info, err
	}
	if err = checkPostgresTools(ctx, info.PostgresMajor); err != nil {
		return info, err
	}
	destination, err = newBackupDestination(root, destination)
	if err != nil {
		return info, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(destination)
		}
	}()
	dump := filepath.Join(destination, "hub.dump")
	// table 模式不携带 CREATE SCHEMA，恢复目标必须预建同名空 schema。
	if err = pgTool(ctx, db, c, "pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--table="+info.DatabaseSchema+".*", "--file="+dump); err != nil {
		return info, err
	}
	f, openErr := os.OpenFile(dump, os.O_RDWR, 0600)
	if openErr != nil {
		return info, openErr
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil {
		return info, syncErr
	}
	if closeErr != nil {
		return info, closeErr
	}
	if err = os.Chmod(dump, 0600); err != nil {
		return info, err
	}
	if info.DatabaseDigest, info.DatabaseBytes, err = hashBackupFile(ctx, dump); err != nil {
		return info, err
	}
	if err = copyPostgresPackages(ctx, info, root, destination); err != nil {
		return info, err
	}
	after, metaErr := postgresMetadata(ctx, db)
	if metaErr != nil {
		return info, metaErr
	}
	if after.RegistryID != info.RegistryID || after.Schema != info.Schema || strings.Join(after.Digests, ",") != strings.Join(info.Digests, ",") {
		return info, errors.New("database changed during backup")
	}
	raw, _ := json.MarshalIndent(info, "", "  ")
	if len(raw) >= 16<<20 {
		return info, errors.New("backup manifest exceeds size limit")
	}
	if err = writeBackupFile(filepath.Join(destination, backupManifestName), append(raw, '\n')); err != nil {
		return info, err
	}
	if err = finishBackupDirectory(destination); err != nil {
		return info, err
	}
	return info, nil
}
func readPostgresBackup(root string) (PostgresBackupInfo, error) {
	var info PostgresBackupInfo
	if _, err := os.Lstat(filepath.Join(root, maintenanceMarker)); !errors.Is(err, os.ErrNotExist) {
		return info, errors.New("backup is incomplete")
	}
	f, err := openBackupFile(filepath.Join(root, backupManifestName))
	if err != nil {
		return info, err
	}
	defer f.Close()
	stat, statErr := f.Stat()
	if statErr != nil || stat.Size() > 16<<20 {
		return info, errors.New("backup manifest exceeds size limit")
	}
	dec := json.NewDecoder(io.LimitReader(f, 16<<20))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&info); err != nil {
		return info, errors.New("invalid PostgreSQL backup manifest")
	}
	if err = dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return info, errors.New("invalid manifest trailer")
	}
	_, timeErr := time.Parse(time.RFC3339, info.CreatedAt)
	if info.PostgresMajor < 15 || info.Driver != "postgres" || info.Format != 2 || (info.Schema < 1 || info.Schema > currentSchema) || info.RegistryID == "" || info.DatabaseBytes <= 0 || !validBackupDigest(info.DatabaseDigest) || !pgSchemaName.MatchString(info.DatabaseSchema) || info.Packages != len(info.Digests) || timeErr != nil {
		return info, errors.New("unsupported PostgreSQL backup metadata")
	}
	previous := ""
	for _, d := range info.Digests {
		if !validBackupDigest(d) || d <= previous {
			return info, errors.New("invalid package manifest")
		}
		previous = d
	}
	return info, nil
}
func VerifyPostgresBackup(ctx context.Context, root string) (PostgresBackupInfo, error) {
	info, err := readPostgresBackup(root)
	if err != nil {
		return info, err
	}
	hash, size, err := hashBackupFile(ctx, filepath.Join(root, "hub.dump"))
	if err != nil || hash != info.DatabaseDigest || size != info.DatabaseBytes {
		return info, errors.New("database archive checksum mismatch")
	}
	if err = checkPostgresTools(ctx, info.PostgresMajor); err != nil {
		return info, err
	}
	if err = pgTool(ctx, nil, hubdb.Config{}, "pg_restore", "--list", filepath.Join(root, "hub.dump")); err != nil {
		return info, err
	}
	return info, copyPostgresPackages(ctx, info, root, "")
}

// 恢复只接受新目录和同名空 schema。双重标记在进程崩溃后也阻止另一数据目录启动半成品。
func RestorePostgres(ctx context.Context, c hubdb.Config, source, destination string) (info PostgresBackupInfo, err error) {
	info, err = VerifyPostgresBackup(ctx, source)
	if err != nil {
		return info, err
	}
	if c.Driver != "postgres" {
		return info, errors.New("PostgreSQL configuration required")
	}
	db, err := hubdb.Open(destination, c)
	if err != nil {
		return info, err
	}
	defer db.Close()
	var serverVersion int
	if err = db.QueryRowContext(ctx, "SHOW server_version_num").Scan(&serverVersion); err != nil || serverVersion/10000 != info.PostgresMajor {
		return info, errors.New("restore requires the backup's PostgreSQL major version")
	}
	var schema string
	var objects int
	if err = db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil || schema != info.DatabaseSchema {
		return info, errors.New("restore requires the same schema name in a new database")
	}
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM pg_class WHERE relnamespace=current_schema()::regnamespace").Scan(&objects); err != nil || objects != 0 {
		return info, errors.New("restore requires an empty destination schema")
	}
	destination, err = newBackupDestination(source, destination)
	if err != nil {
		return info, err
	}
	// 失败保留目录与数据库标记，操作员检查后改用新的目标，绝不自动删除不明数据库对象。
	if _, err = db.ExecContext(ctx, "CREATE TABLE skillhub_recovery (id BIGINT PRIMARY KEY)"); err != nil {
		return info, err
	}
	if err = copyBackupDatabase(ctx, filepath.Join(source, "hub.dump"), filepath.Join(destination, "hub.dump"), info.BackupInfo); err != nil {
		return info, err
	}
	if err = copyPostgresPackages(ctx, info, source, destination); err != nil {
		return info, err
	}
	if err = pgTool(ctx, db, c, "pg_restore", "--single-transaction", "--exit-on-error", "--no-owner", "--no-privileges", "--dbname=", filepath.Join(destination, "hub.dump")); err != nil {
		return info, err
	}
	// 通过同一持锁连接比对 dump 内的身份、schema 和引用集合，防止错误配对。
	restored, metaErr := postgresMetadata(ctx, db)
	if metaErr != nil || restored.RegistryID != info.RegistryID || restored.Schema != info.Schema || strings.Join(restored.Digests, ",") != strings.Join(info.Digests, ",") {
		return info, errors.New("restored database does not match backup manifest")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return info, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions; DELETE FROM devices;"); err != nil {
		return info, err
	}
	if err = audit(ctx, tx, "system", "instance.restore", info.RegistryID, time.Now()); err != nil {
		return info, err
	}
	if err = tx.Commit(); err != nil {
		return info, err
	}
	if err = os.Remove(filepath.Join(destination, "hub.dump")); err != nil {
		return info, err
	}
	// 先移除文件标记，数据库标记最后删除；任何中断至少保留一种启动阻断。
	if err = finishBackupDirectory(destination); err != nil {
		return info, err
	}
	if _, err = db.ExecContext(ctx, "DROP TABLE skillhub_recovery"); err != nil {
		return info, err
	}
	return info, nil
}

// 避免较新 pg_dump 引入旧服务不支持的 SET/DDL；工具与源/目标数据库主版本必须一致。
func checkPostgresTools(ctx context.Context, major int) error {
	for _, name := range []string{"pg_dump", "pg_restore"} {
		out, err := exec.CommandContext(ctx, name, "--version").Output()
		if err != nil || len(out) > 4096 {
			return errors.New("matching pg_dump and pg_restore are required on PATH")
		}
		match := regexp.MustCompile(`PostgreSQL\) (\d+)`).FindSubmatch(out)
		if len(match) != 2 {
			return errors.New("unrecognized PostgreSQL tool version")
		}
		value, _ := strconv.Atoi(string(match[1]))
		if value != major {
			return errors.New("PostgreSQL tools must match the database major version")
		}
	}
	return nil
}
