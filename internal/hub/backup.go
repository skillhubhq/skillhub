package hub

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/sqliteutil"
)

const (
	backupManifestName = "backup.json"
	maintenanceMarker  = ".skillhub-maintenance"
)

// BackupInfo 不含密码、会话或包内容；摘要用来检测备份损坏，不代表来源签名。
type BackupInfo struct {
	Format         int    `json:"format"`
	Schema         int    `json:"schema"`
	RegistryID     string `json:"registry_id"`
	CreatedAt      string `json:"created_at"`
	DatabaseDigest string `json:"database_digest"`
	DatabaseBytes  int64  `json:"database_bytes"`
	Packages       int    `json:"packages"`
}

// Backup 先建立 SQLite 一致快照，再复制其引用的不可变包；无需停止服务。
func (s *Store) Backup(ctx context.Context, destination string) (info BackupInfo, err error) {
	if s.db.Driver != "sqlite" {
		return info, errors.New("PostgreSQL backup requires pg_dump and a matching package-directory snapshot; see docs/database.md")
	}
	destination, err = newBackupDestination(s.root, destination)
	if err != nil {
		return info, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(destination)
		}
	}()
	database := filepath.Join(destination, "hub.db")
	if _, err = s.db.ExecContext(ctx, "VACUUM INTO ?", database); err != nil {
		return info, err
	}
	if err = os.Chmod(database, 0600); err != nil {
		return info, err
	}
	info = BackupInfo{Format: 1, Schema: currentSchema, RegistryID: s.registryID, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if info.DatabaseDigest, info.DatabaseBytes, err = hashBackupFile(ctx, database); err != nil {
		return info, err
	}
	db, err := openCheckedSnapshot(ctx, destination, info)
	if err != nil {
		return info, err
	}
	info.Packages, err = copySnapshotPackages(ctx, db, s.root, destination)
	err = errors.Join(err, db.Close())
	if err != nil {
		return info, err
	}
	raw, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return info, err
	}
	if err = writeBackupFile(filepath.Join(destination, backupManifestName), append(raw, '\n')); err != nil {
		return info, err
	}
	if err = finishBackupDirectory(destination); err != nil {
		return info, err
	}
	return info, nil
}

func VerifyBackup(ctx context.Context, root string) (BackupInfo, error) {
	info, err := readBackupInfo(root)
	if err != nil {
		return info, err
	}
	db, err := openCheckedSnapshot(ctx, root, info)
	if err != nil {
		return info, err
	}
	defer db.Close()
	count, err := copySnapshotPackages(ctx, db, root, "")
	if err != nil {
		return info, err
	}
	if count != info.Packages {
		return info, errors.New("backup package count does not match manifest")
	}
	return info, nil
}

// RestoreBackup 只写入新目录；中断标记阻止服务启动到半恢复状态。
func RestoreBackup(ctx context.Context, source, destination string) (info BackupInfo, err error) {
	info, err = readBackupInfo(source)
	if err != nil {
		return info, err
	}
	destination, err = newBackupDestination(source, destination)
	if err != nil {
		return info, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(destination)
		}
	}()
	// 先复制后验证副本，使被验证的数据就是将要恢复的数据。
	if err = copyBackupDatabase(ctx, filepath.Join(source, "hub.db"), filepath.Join(destination, "hub.db"), info); err != nil {
		return info, err
	}
	db, err := openCheckedSnapshot(ctx, destination, info)
	if err != nil {
		return info, err
	}
	count, copyErr := copySnapshotPackages(ctx, db, source, destination)
	err = errors.Join(copyErr, db.Close())
	if err != nil {
		return info, err
	}
	if count != info.Packages {
		return info, errors.New("backup package count does not match manifest")
	}
	db, err = sqliteutil.Open(filepath.Join(destination, "hub.db"))
	if err != nil {
		return info, err
	}
	err = resetRestoredSessions(ctx, db, info.RegistryID)
	err = errors.Join(err, db.Close())
	if err != nil {
		return info, err
	}
	if err = finishBackupDirectory(destination); err != nil {
		return info, err
	}
	return info, nil
}

func resetRestoredSessions(ctx context.Context, db *sql.DB, id string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions; DELETE FROM devices;"); err != nil {
		return err
	}
	// 保留实例密钥，维持已有幂等操作指纹；删除会话防止历史凭据复活。
	if err = audit(ctx, tx, "system", "instance.restore", id, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

func newBackupDestination(source, destination string) (string, error) {
	if destination == "" {
		return "", errors.New("destination is required")
	}
	abs, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", errors.New("destination parent must already exist")
	}
	abs = filepath.Join(parent, filepath.Base(abs))
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(source, abs)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("destination must be outside the source directory")
	}
	if err = os.Mkdir(abs, 0700); err != nil {
		return "", fmt.Errorf("destination must be a new directory: %w", err)
	}
	if err = writeBackupFile(filepath.Join(abs, maintenanceMarker), []byte("operation incomplete\n")); err != nil {
		_ = os.RemoveAll(abs)
		return "", err
	}
	if err = syncPackageDirectory(abs); err != nil {
		_ = os.RemoveAll(abs)
		return "", err
	}
	return abs, nil
}

func finishBackupDirectory(root string) error {
	if err := syncPackageDirectory(root); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(root, maintenanceMarker)); err != nil {
		return err
	}
	return errors.Join(syncPackageDirectory(root), syncPackageDirectory(filepath.Dir(root)))
}

func readBackupInfo(root string) (BackupInfo, error) {
	var info BackupInfo
	if err := checkSnapshotSidecars(root); err != nil {
		return info, err
	}
	if _, err := os.Lstat(filepath.Join(root, maintenanceMarker)); err == nil {
		return info, errors.New("backup is incomplete")
	} else if !errors.Is(err, os.ErrNotExist) {
		return info, err
	}
	f, err := openBackupFile(filepath.Join(root, backupManifestName))
	if err != nil {
		return info, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (16<<10)+1))
	if err != nil {
		return info, err
	}
	if len(raw) > 16<<10 {
		return info, errors.New("backup manifest too large")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&info); err != nil {
		return info, errors.New("invalid backup manifest")
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return info, errors.New("invalid backup manifest trailer")
	}
	_, timeErr := time.Parse(time.RFC3339, info.CreatedAt)
	if info.Format != 1 || info.Schema != currentSchema || info.RegistryID == "" || info.DatabaseBytes <= 0 || info.Packages < 0 || !validBackupDigest(info.DatabaseDigest) || timeErr != nil {
		return info, errors.New("unsupported or invalid backup metadata")
	}
	return info, nil
}

func openCheckedSnapshot(ctx context.Context, root string, info BackupInfo) (*sql.DB, error) {
	if err := checkSnapshotSidecars(root); err != nil {
		return nil, err
	}
	digest, size, err := hashBackupFile(ctx, filepath.Join(root, "hub.db"))
	if err != nil {
		return nil, err
	}
	if digest != info.DatabaseDigest || size != info.DatabaseBytes {
		return nil, errors.New("backup database checksum does not match manifest")
	}
	db, err := sqliteutil.OpenSnapshot(filepath.Join(root, "hub.db"))
	if err != nil {
		return nil, err
	}
	if err = checkSnapshotDatabase(ctx, db, info); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func checkSnapshotSidecars(root string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(filepath.Join(root, "hub.db"+suffix)); err == nil {
			return errors.New("backup contains live SQLite sidecar files")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func checkSnapshotDatabase(ctx context.Context, db *sql.DB, info BackupInfo) error {
	var check, id string
	var schema, instances int
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&check); err != nil || check != "ok" {
		return errors.New("backup database integrity check failed")
	}
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&schema); err != nil || schema != info.Schema {
		return errors.New("backup database schema does not match manifest")
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*),min(id) FROM instance").Scan(&instances, &id); err != nil || instances != 1 || id != info.RegistryID {
		return errors.New("backup database identity does not match manifest")
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("backup database foreign key check failed")
	}
	return rows.Err()
}

func copySnapshotPackages(ctx context.Context, db *sql.DB, source, destination string) (int, error) {
	rows, err := db.QueryContext(ctx, "SELECT DISTINCT digest FROM releases WHERE cleaned=0 ORDER BY digest")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	reader := &Store{root: source}
	writer := &Store{root: destination}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		if count == 0 {
			// 禁止通过包目录符号链接读取备份目录之外的文件。
			entry, err := os.Lstat(filepath.Join(source, "packages"))
			if err != nil || !entry.IsDir() || entry.Mode()&os.ModeSymlink != 0 {
				return count, errors.New("invalid backup package directory")
			}
		}
		var digest string
		if err := rows.Scan(&digest); err != nil {
			return count, err
		}
		data, err := reader.readBlob(digest)
		if err != nil {
			return count, errors.New("referenced backup package is missing or corrupt")
		}
		if _, err = packageformat.Inspect(data); err != nil {
			return count, errors.New("referenced backup package is invalid")
		}
		if destination != "" {
			if err := writer.writeBlob(digest, data); err != nil {
				return count, err
			}
		}
		count++
	}
	return count, rows.Err()
}

func validBackupDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(value[7:])
	return err == nil
}

func openBackupFile(path string) (*os.File, error) {
	entry, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() {
		return nil, errors.New("backup files must be regular files")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(entry, opened) {
		_ = f.Close()
		return nil, errors.New("backup file changed while opening")
	}
	return f, nil
}

type backupReader struct {
	ctx context.Context
	r   io.Reader
}

func (r backupReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func hashBackupFile(ctx context.Context, path string) (string, int64, error) {
	f, err := openBackupFile(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, backupReader{ctx, f})
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), n, err
}

func copyBackupDatabase(ctx context.Context, source, destination string, info BackupInfo) error {
	input, err := openBackupFile(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(output, hash), backupReader{ctx, io.LimitReader(input, info.DatabaseBytes+1)})
	err = errors.Join(copyErr, output.Sync(), output.Close())
	if err != nil {
		return err
	}
	if n != info.DatabaseBytes || "sha256:"+hex.EncodeToString(hash.Sum(nil)) != info.DatabaseDigest {
		return errors.New("copied backup database does not match manifest")
	}
	return nil
}

func writeBackupFile(path string, content []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(content)
	return errors.Join(writeErr, f.Sync(), f.Close())
}
