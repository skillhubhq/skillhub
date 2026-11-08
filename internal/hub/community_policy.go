package hub

import (
	"context"
	"database/sql"
	"errors"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/hubpolicy"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ConfigurePolicy 只在监听请求之前调用；切换模式不改写已有授权和发布状态。
func (s *Store) ConfigurePolicy(p hubpolicy.Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}

	s.policy = p
	return nil
}

func (s *Store) provisionPersonalNamespace(ctx context.Context, tx *hubdb.Tx, user Identity) error {
	if !s.policy.IsCommunity() {
		return nil
	}
	var existing string
	err := tx.QueryRowContext(ctx, "SELECT namespace FROM personal_namespaces WHERE user_id=?", user.ID).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	// 稳定内部 ID 避免第三方改名、抢注和账号合并改变内容坐标。
	name := "u-" + strings.ReplaceAll(user.ID, "-", "")
	var retired bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM namespace_tombstones WHERE name=?)", name).Scan(&retired); err != nil {
		return err
	}
	if retired {
		return nil
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO namespaces(name,review_required,revision) VALUES(?,1,1)", name); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO personal_namespaces(user_id,namespace) VALUES(?,?)", user.ID, name); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO namespace_grants(namespace,kind,subject_id,role) VALUES(?,1,?,?)", name, user.ID, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO team_profiles(namespace,display_name) VALUES(?,COALESCE((SELECT NULLIF(display_name,'') FROM profiles WHERE user_id=?),'Personal skills'))", name, user.ID); err != nil {
		return err
	}
	return audit(ctx, tx, user.ID, "namespace.personal", name, s.now())
}

func (s *Store) checkSubmissionQuota(ctx context.Context, tx *hubdb.Tx, u Identity, cmd SubmitRelease) error {
	if !s.policy.IsCommunity() {
		return nil
	}
	l := s.policy.Limits
	var count, recent int
	var size int64
	if err := tx.QueryRowContext(ctx, "SELECT count(*),COALESCE(sum(archive_bytes),0),COALESCE(sum(CASE WHEN created_at>? THEN 1 ELSE 0 END),0) FROM releases WHERE submitted_by=?", s.now().Add(-24*time.Hour).Unix(), u.ID).Scan(&count, &size, &recent); err != nil {
		return err
	}
	if count >= l.VersionsPerUser || recent >= l.SubmissionsPerDay || int64(len(cmd.Archive)) > l.BytesPerUser-size {
		return ErrQuota
	}
	// 连失败事务留下的不可变包也计量，避免用失败投稿耗尽磁盘。
	entries, err := os.ReadDir(filepath.Join(s.root, "packages"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var total int64
	for _, e := range entries {
		if !e.Type().IsRegular() {
			return ErrConflict
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > l.PackageBytes {
			return ErrQuota
		}
	}
	target := filepath.Join(s.root, "packages", strings.TrimPrefix(cmd.ArchiveDigest, "sha256:")+".zip")
	if _, err = os.Stat(target); errors.Is(err, os.ErrNotExist) {
		total += int64(len(cmd.Archive))
	} else if err != nil {
		return err
	}
	if total > l.PackageBytes {
		return ErrQuota
	}
	return nil
}

// 首次成功同步仅填充未由用户编辑过的空头像；网络失败可在下次登录重试。
func (s *Store) SyncExternalAvatar(ctx context.Context, actor Identity, raw []byte) error {
	value, err := normalizeAvatar(raw)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if actor.ID == "" {
		return ErrUnauthenticated
	}
	if _, err = currentIdentity(ctx, tx, actor); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE profiles SET avatar=? WHERE user_id=? AND revision=0 AND avatar=''", value, actor.ID); err != nil {
		return err
	}
	return tx.Commit()
}

var ErrQuota = errors.New("community quota reached; contact the hub operator")
