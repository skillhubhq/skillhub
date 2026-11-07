package client

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

var (
	ErrUnmanaged      = errors.New("target already exists and is not managed")
	ErrModified       = errors.New("installed files have local modifications")
	ErrStalePlan      = errors.New("installation changed after planning")
	ErrSourceConflict = errors.New("target belongs to a different skill source")
)

type InstallPlan struct {
	Action string        `json:"action"`
	Target string        `json:"target"`
	Before *Installation `json:"before,omitempty"`
	After  *Installation `json:"after,omitempty"`
}

type installJournal struct {
	Plan      InstallPlan `json:"plan"`
	Directory string      `json:"directory"`
	Committed bool        `json:"committed"`
}

func canonicalTarget(target string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if abs == filepath.Dir(abs) {
		return "", errors.New("installation cannot target filesystem root")
	}
	parent := filepath.Dir(abs)
	suffix := []string{filepath.Base(abs)}
	for {
		_, err := os.Lstat(parent)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		suffix = append([]string{filepath.Base(parent)}, suffix...)
		parent = filepath.Dir(parent)
	}
	real, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	return physicalSpelling(filepath.Join(append([]string{real}, suffix...)...))
}

// 读取已存在分量的实际拼写，使大小写不敏感卷中的别名归于同一路径。
func physicalSpelling(absolute string) (string, error) {
	root := filepath.VolumeName(absolute) + string(filepath.Separator)
	rel, err := filepath.Rel(root, absolute)
	if err != nil {
		return "", err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	current := root
	for i, part := range parts {
		candidate := filepath.Join(current, part)
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return filepath.Join(append([]string{current}, parts[i:]...)...), nil
		}
		if err != nil {
			return "", err
		}
		entries, err := os.ReadDir(current)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if !strings.EqualFold(entry.Name(), part) {
				continue
			}
			actual, err := entry.Info()
			if err != nil {
				return "", err
			}
			if os.SameFile(info, actual) {
				candidate = filepath.Join(current, entry.Name())
				break
			}
		}
		current = candidate
	}
	return current, nil
}

// Windows 不保留 Unix 执行位；安装指纹反映目标平台实际能够保存的状态。
func installationFingerprint(bundle *packageformat.Bundle) string {
	if runtime.GOOS != "windows" {
		return bundle.Fingerprint
	}
	files := append([]packageformat.File(nil), bundle.Files...)
	for i := range files {
		files[i].Executable = false
	}
	return packageformat.Fingerprint(files)
}

func (s *Store) withTarget(ctx context.Context, target string, fn func() error) error {
	// 在大小写敏感卷上保守串行同形路径，确保未知卷上也不会并发创建同一目录。
	sum := sha256.Sum256([]byte(strings.ToLower(target)))
	if err := os.MkdirAll(filepath.Join(s.root, "locks"), 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(s.root, "locks", hex.EncodeToString(sum[:])+".lock"))
	ok, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("cannot acquire installation lock")
	}
	defer lock.Unlock()
	return fn()
}

func fingerprintAt(target string) (string, bool, error) {
	_, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	files, err := packageformat.ReadDirectory(target)
	if err != nil {
		return "", true, err
	}
	return packageformat.Fingerprint(files), true, nil
}

func (s *Store) checkedInstallation(ctx context.Context, target string) (*Installation, error) {
	record, err := s.installation(ctx, target)
	if err != nil {
		return nil, err
	}
	fingerprint, exists, err := fingerprintAt(target)
	if err != nil {
		return nil, fmt.Errorf("inspect target: %w", err)
	}
	if exists && record == nil {
		return nil, ErrUnmanaged
	}
	if record != nil && (!exists || record.Fingerprint != fingerprint) {
		return nil, ErrModified
	}
	return record, nil
}

func (s *Store) PlanInstall(ctx context.Context, desired Installation, archive []byte) (*InstallPlan, error) {
	bundle, err := packageformat.Inspect(archive)
	if err != nil {
		return nil, err
	}
	if desired.Name != bundle.Metadata.Name || desired.Digest != bundle.Digest || desired.RegistryID == "" || desired.Namespace == "" || desired.Version == "" {
		return nil, errors.New("package does not match release identity")
	}
	target, err := canonicalTarget(desired.Target)
	if err != nil {
		return nil, err
	}
	desired.Target = target
	desired.Fingerprint = installationFingerprint(bundle)
	var plan *InstallPlan
	err = s.withTarget(ctx, target, func() error {
		if err := s.recoverTarget(ctx, target); err != nil {
			return err
		}
		before, err := s.checkedInstallation(ctx, target)
		if err != nil {
			return err
		}
		desired.ID = uuid.NewString()
		action := "install"
		if before != nil {
			if before.RegistryID != desired.RegistryID || before.Namespace != desired.Namespace || before.Name != desired.Name {
				return ErrSourceConflict
			}
			desired.ID = before.ID
			action = "update"
		}
		plan = &InstallPlan{Action: action, Target: target, Before: before, After: &desired}
		return nil
	})
	return plan, err
}

func (s *Store) PlanRemove(ctx context.Context, target string) (*InstallPlan, error) {
	target, err := canonicalTarget(target)
	if err != nil {
		return nil, err
	}
	var plan *InstallPlan
	err = s.withTarget(ctx, target, func() error {
		if err := s.recoverTarget(ctx, target); err != nil {
			return err
		}
		before, err := s.checkedInstallation(ctx, target)
		if err != nil {
			return err
		}
		if before == nil {
			return errors.New("installation not found")
		}
		plan = &InstallPlan{Action: "remove", Target: target, Before: before}
		return nil
	})
	return plan, err
}

func sameInstallation(a, b *Installation) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *Store) Apply(ctx context.Context, plan InstallPlan, archive []byte) error {
	return s.apply(ctx, plan, archive, nil)
}

// hook 仅用于故障注入测试，生产调用始终为空。
func (s *Store) apply(ctx context.Context, plan InstallPlan, archive []byte, hook func(string) error) error {
	target, err := canonicalTarget(plan.Target)
	if err != nil {
		return err
	}
	if target != plan.Target {
		return ErrStalePlan
	}
	if plan.Action != "install" && plan.Action != "update" && plan.Action != "remove" {
		return errors.New("invalid install action")
	}
	if plan.Action == "remove" && (plan.Before == nil || plan.After != nil) {
		return errors.New("invalid removal plan")
	}
	var bundle *packageformat.Bundle
	if plan.Action != "remove" {
		if plan.After == nil || plan.After.Target != target {
			return errors.New("invalid installation plan")
		}
		bundle, err = packageformat.Inspect(archive)
		if err != nil {
			return err
		}
		if plan.After.Digest != bundle.Digest || plan.After.Fingerprint != installationFingerprint(bundle) || plan.After.Name != bundle.Metadata.Name {
			return errors.New("package changed after planning")
		}
	}
	return s.withTarget(ctx, target, func() error {
		if err := s.recoverTarget(ctx, target); err != nil {
			return err
		}
		before, err := s.checkedInstallation(ctx, target)
		if err != nil {
			return err
		}
		if !sameInstallation(before, plan.Before) {
			return ErrStalePlan
		}
		if plan.After != nil && before != nil && (before.RegistryID != plan.After.RegistryID || before.Namespace != plan.After.Namespace || before.Name != plan.After.Name) {
			return ErrSourceConflict
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		// 再次解析父目录，拒绝计划后发生的符号链接重定向。
		current, err := canonicalTarget(target)
		if err != nil {
			return err
		}
		if current != target {
			return ErrStalePlan
		}
		dir, err := os.MkdirTemp(filepath.Dir(target), ".skillhub-txn-")
		if err != nil {
			return err
		}
		journal := installJournal{Plan: plan, Directory: dir}
		if bundle != nil {
			stage := filepath.Join(dir, "new")
			if err = os.Mkdir(stage, 0700); err != nil {
				_ = os.RemoveAll(dir)
				return err
			}
			if err = bundle.Extract(stage); err != nil {
				_ = os.RemoveAll(dir)
				return err
			}
			if err = syncTree(stage); err != nil {
				_ = os.RemoveAll(dir)
				return err
			}
		}
		if err = s.saveJournal(ctx, journal); err != nil {
			_ = os.RemoveAll(dir)
			return err
		}
		if hook != nil {
			if err = hook("prepared"); err != nil {
				return err
			}
		}
		if before != nil {
			if err = os.Rename(target, filepath.Join(dir, "old")); err != nil {
				return err
			}
		}
		if hook != nil {
			if err = hook("old-moved"); err != nil {
				return err
			}
		}
		if plan.After != nil {
			if err = os.Rename(filepath.Join(dir, "new"), target); err != nil {
				return err
			}
		}
		if err = syncDirectory(filepath.Dir(target)); err != nil {
			return err
		}
		if err = syncDirectory(dir); err != nil {
			return err
		}
		if hook != nil {
			if err = hook("new-moved"); err != nil {
				return err
			}
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if plan.After == nil {
			_, err = tx.ExecContext(ctx, "DELETE FROM installations WHERE target=?", target)
		} else {
			raw, _ := json.Marshal(plan.After)
			_, err = tx.ExecContext(ctx, "INSERT INTO installations(target,id,record) VALUES(?,?,?) ON CONFLICT(target) DO UPDATE SET id=excluded.id,record=excluded.record", target, plan.After.ID, string(raw))
		}
		if err != nil {
			return err
		}
		journal.Committed = true
		raw, _ := json.Marshal(journal)
		if _, err = tx.ExecContext(ctx, "UPDATE install_journals SET record=? WHERE target=?", string(raw), target); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		if hook != nil {
			if err = hook("committed"); err != nil {
				return err
			}
		}
		return s.recoverTarget(ctx, target)
	})
}

func (s *Store) saveJournal(ctx context.Context, journal installJournal) error {
	raw, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO install_journals(target,record) VALUES(?,?)", journal.Plan.Target, string(raw))
	return err
}

func (s *Store) recoverTarget(ctx context.Context, target string) error {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT record FROM install_journals WHERE target=?", target).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var j installJournal
	if err = json.Unmarshal([]byte(raw), &j); err != nil {
		return err
	}
	if j.Plan.Target != target || filepath.Dir(j.Directory) != filepath.Dir(target) || !strings.HasPrefix(filepath.Base(j.Directory), ".skillhub-txn-") {
		return errors.New("invalid recovery journal")
	}
	current, err := canonicalTarget(target)
	if err != nil {
		return err
	}
	if current != target {
		return errors.New("recovery stopped: target parent changed")
	}
	if info, statErr := os.Lstat(j.Directory); statErr == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("recovery stopped: transaction directory changed")
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	// 已提交只表示新安装已生效，并不授权删除崩溃后被用户修改的备份。
	if j.Committed {
		fingerprint, exists, err := fingerprintAt(filepath.Join(j.Directory, "old"))
		if err != nil {
			return err
		}
		if exists && (j.Plan.Before == nil || fingerprint != j.Plan.Before.Fingerprint) {
			return errors.New("recovery stopped: original installation was modified")
		}
	}
	if !j.Committed {
		backup := filepath.Join(j.Directory, "old")
		_, backupErr := os.Lstat(backup)
		fingerprint, exists, err := fingerprintAt(target)
		if err != nil {
			return err
		}
		if backupErr == nil {
			backupFingerprint, _, err := fingerprintAt(backup)
			if err != nil {
				return err
			}
			if j.Plan.Before == nil || backupFingerprint != j.Plan.Before.Fingerprint {
				return errors.New("recovery stopped: original installation was modified")
			}
			if exists {
				if j.Plan.After == nil || fingerprint != j.Plan.After.Fingerprint {
					return errors.New("recovery stopped: target was modified")
				}
				if err = os.RemoveAll(target); err != nil {
					return err
				}
			}
			if err = os.Rename(backup, target); err != nil {
				return err
			}
		} else if !errors.Is(backupErr, os.ErrNotExist) {
			return backupErr
		} else if j.Plan.Before == nil {
			if exists {
				if j.Plan.After == nil || fingerprint != j.Plan.After.Fingerprint {
					return errors.New("recovery stopped: target was modified")
				}
				if err = os.RemoveAll(target); err != nil {
					return err
				}
			}
		} else if !exists || fingerprint != j.Plan.Before.Fingerprint {
			return errors.New("recovery stopped: original installation is unavailable")
		}
		if err = syncDirectory(filepath.Dir(target)); err != nil {
			return err
		}
	}
	if err = os.RemoveAll(j.Directory); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "DELETE FROM install_journals WHERE target=?", target)
	return err
}

func (s *Store) Recover(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "SELECT target FROM install_journals ORDER BY target")
	if err != nil {
		return err
	}
	var targets []string
	for rows.Next() {
		var target string
		if err = rows.Scan(&target); err != nil {
			_ = rows.Close()
			return err
		}
		targets = append(targets, target)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err = s.withTarget(ctx, target, func() error { return s.recoverTarget(ctx, target) }); err != nil {
			return err
		}
	}
	return nil
}

func syncTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return syncDirectory(path)
		}
		return nil
	})
}
