package hub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/skillscan"
)

type PlatformAccess struct {
	UserID         string                  `json:"user_id"`
	Roles          []taxonomy.PlatformRole `json:"roles"`
	CanManageUsers bool                    `json:"can_manage_users"`
	CanReadAudit   bool                    `json:"can_read_audit"`
	Administrator  bool                    `json:"administrator"`
}

func hasPlatformRole(ctx context.Context, tx *hubdb.Tx, user Identity, role taxonomy.PlatformRole) error {
	if user.Administrator {
		return nil
	}
	if user.ID == "" {
		return ErrUnauthenticated
	}
	var yes bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM platform_roles WHERE user_id=? AND role=?)", user.ID, role).Scan(&yes); err != nil {
		return err
	}
	if !yes {
		return ErrForbidden
	}
	return nil
}
func userAdministrator(tx *hubdb.Tx, user Identity) error {
	return hasPlatformRole(context.Background(), tx, user, taxonomy.PlatformRole_PLATFORM_ROLE_USER_ADMIN)
}

// 人员管理员不可修改管理员、其他平台职责账号或本人；密码重置另限 Hub 管理员。
func ordinaryAccount(ctx context.Context, tx *hubdb.Tx, actor Identity, id string) error {
	if actor.Administrator {
		return nil
	}
	if actor.ID == id {
		return ErrForbidden
	}
	var privileged bool
	err := tx.QueryRowContext(ctx, "SELECT administrator=1 OR EXISTS(SELECT 1 FROM platform_roles WHERE user_id=users.id) FROM users WHERE id=?", id).Scan(&privileged)
	if err != nil {
		return assertExists(err)
	}
	if privileged {
		return ErrForbidden
	}
	return nil
}
func (s *Store) GetPlatformAccess(ctx context.Context, actor Identity, id string) (PlatformAccess, error) {
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return PlatformAccess{}, err
	}
	defer tx.Rollback()
	if u.ID == "" {
		return PlatformAccess{}, ErrUnauthenticated
	}
	if id == "" {
		id = u.ID
	}
	if id != u.ID && !u.Administrator {
		return PlatformAccess{}, ErrForbidden
	}
	result := PlatformAccess{UserID: id, Roles: []taxonomy.PlatformRole{}}
	if err = tx.QueryRowContext(ctx, "SELECT administrator FROM users WHERE id=?", id).Scan(&result.Administrator); err != nil {
		return result, assertExists(err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT role FROM platform_roles WHERE user_id=? ORDER BY role", id)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var r taxonomy.PlatformRole
		if err = rows.Scan(&r); err != nil {
			return result, err
		}
		result.Roles = append(result.Roles, r)
	}
	result.CanManageUsers = result.Administrator
	result.CanReadAudit = result.Administrator
	for _, r := range result.Roles {
		result.CanManageUsers = result.CanManageUsers || r == taxonomy.PlatformRole_PLATFORM_ROLE_USER_ADMIN
		result.CanReadAudit = result.CanReadAudit || r == taxonomy.PlatformRole_PLATFORM_ROLE_AUDITOR
	}
	return result, rows.Err()
}
func (s *Store) SetPlatformRole(ctx context.Context, actor Identity, key, id string, role taxonomy.PlatformRole, granted bool) error {
	if role != taxonomy.PlatformRole_PLATFORM_ROLE_USER_ADMIN && role != taxonomy.PlatformRole_PLATFORM_ROLE_AUDITOR {
		return ErrInvalid
	}
	cmd := struct {
		ID      string
		Role    taxonomy.PlatformRole
		Granted bool
	}{id, role, granted}
	_, err := mutate(s, ctx, actor, "platform.role", key, cmd, adminOnly, func(tx *hubdb.Tx, u Identity) (struct{}, error) {
		var disabled bool
		if err := tx.QueryRowContext(ctx, "SELECT disabled FROM users WHERE id=?", id).Scan(&disabled); err != nil {
			return struct{}{}, assertExists(err)
		}
		if granted && disabled {
			return struct{}{}, ErrForbidden
		}
		var err error
		if granted {
			_, err = tx.ExecContext(ctx, "INSERT INTO platform_roles(user_id,role) VALUES(?,?) ON CONFLICT DO NOTHING", id, role)
		} else {
			_, err = tx.ExecContext(ctx, "DELETE FROM platform_roles WHERE user_id=? AND role=?", id, role)
		}
		if err != nil {
			return struct{}{}, err
		}
		if err = revokeUserSessions(ctx, tx, id); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, s.auditChange(ctx, tx, u.ID, "platform.role", id, cmd)
	})
	return err
}

type ScanReport struct {
	skillscan.Report
	ScannedAt time.Time `json:"scanned_at"`
}

func scanAccess(ctx context.Context, tx *hubdb.Tx, u Identity, id string) (Release, error) {
	r, err := releaseByID(ctx, tx, id)
	if err != nil {
		return r, err
	}
	if err = requireRole(ctx, tx, u, r.Namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER); err != nil {
		return Release{}, ErrNotFound
	}
	return r, nil
}
func latestScan(ctx context.Context, tx *hubdb.Tx, id string) (ScanReport, error) {
	var r ScanReport
	var raw, digest string
	var at int64
	err := tx.QueryRowContext(ctx, "SELECT report,digest,created_at FROM release_scans WHERE release_id=? ORDER BY sequence DESC LIMIT 1", id).Scan(&raw, &digest, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(raw), &r.Report); err != nil {
		return r, err
	}
	if r.Digest != digest {
		return r, ErrConflict
	}
	r.ScannedAt = time.Unix(at, 0)
	return r, nil
}
func (s *Store) saveScan(ctx context.Context, tx *hubdb.Tx, id string, r skillscan.Report) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO release_scans(release_id,digest,report,created_at) VALUES(?,?,?,?)", id, r.Digest, string(raw), s.now().Unix())
	return err
}
func (s *Store) GetScanReport(ctx context.Context, actor Identity, id string) (ScanReport, error) {
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return ScanReport{}, err
	}
	defer tx.Rollback()
	r, err := scanAccess(ctx, tx, u, id)
	if err != nil {
		return ScanReport{}, err
	}
	report, err := latestScan(ctx, tx, id)
	if err == nil && report.Engine != "" && report.Digest != r.Digest {
		return ScanReport{}, ErrConflict
	}
	return report, err
}
func (s *Store) ScanRelease(ctx context.Context, actor Identity, key, id string) (ScanReport, error) {
	return mutate(s, ctx, actor, "release.scan", key, id, func(tx *hubdb.Tx, u Identity) error { _, err := scanAccess(ctx, tx, u, id); return err }, func(tx *hubdb.Tx, u Identity) (ScanReport, error) {
		r, err := releaseByID(ctx, tx, id)
		if err != nil {
			return ScanReport{}, err
		}
		data, err := s.readBlob(r.Digest)
		if err != nil {
			return ScanReport{}, err
		}
		b, err := packageformat.Inspect(data)
		if err != nil {
			return ScanReport{}, err
		}
		report, err := skillscan.Scan(ctx, b)
		if err != nil {
			return ScanReport{}, err
		}
		if err = s.saveScan(ctx, tx, id, report); err != nil {
			return ScanReport{}, err
		}
		if err = audit(ctx, tx, u.ID, "release.scan", id, s.now()); err != nil {
			return ScanReport{}, err
		}
		return ScanReport{Report: report, ScannedAt: s.now()}, nil
	})
}

type ReviewNotification struct {
	Kind        string    `json:"kind"`
	CommentID   string    `json:"comment_id"`
	Unavailable bool      `json:"unavailable"`
	ID          string    `json:"id"`
	ReleaseID   string    `json:"release_id"`
	Coordinate  string    `json:"coordinate"`
	Approved    bool      `json:"approved"`
	Read        bool      `json:"read"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) ListNotifications(ctx context.Context, actor Identity, page Page) (PageResult[ReviewNotification], error) {
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[ReviewNotification]{}, err
	}
	defer tx.Rollback()
	if err = authenticatedOnly(tx, u); err != nil {
		return PageResult[ReviewNotification]{}, err
	}
	visible := `r.cleaned=0 AND ((n.kind='review' AND ` + manageableSQL + `) OR (n.kind='reply' AND ` + readableSQL + `))`
	return pageQuery(s, ctx, tx, page, scope(u.ID, "notifications", nil), `SELECT n.sequence,CASE WHEN `+visible+` THEN n.release_id ELSE '' END,CASE WHEN `+visible+` THEN r.namespace||'/'||r.name||'@'||r.version ELSE '' END,n.approved,n.is_read,n.created_at,n.kind,CASE WHEN `+visible+` THEN n.comment_id ELSE '' END,NOT (`+visible+`) FROM review_notifications n JOIN releases r ON r.id=n.release_id WHERE n.recipient=@actor AND (@after='' OR n.sequence<CAST(COALESCE(NULLIF(@after,''),'0') AS INTEGER)) ORDER BY n.sequence DESC LIMIT @limit`, principalArgs(u), func(rows *sql.Rows) (ReviewNotification, string, error) {
		var n ReviewNotification
		var seq, at int64
		err := rows.Scan(&seq, &n.ReleaseID, &n.Coordinate, &n.Approved, &n.Read, &at, &n.Kind, &n.CommentID, &n.Unavailable)
		n.ID = strconv.FormatInt(seq, 10)
		n.CreatedAt = time.Unix(at, 0)
		return n, n.ID, err
	})
}
func (s *Store) UnreadNotifications(ctx context.Context, actor Identity) (uint32, error) {
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if u.ID == "" {
		return 0, ErrUnauthenticated
	}
	var count uint32
	err = tx.QueryRowContext(ctx, "SELECT count(*) FROM review_notifications WHERE recipient=? AND is_read=0", u.ID).Scan(&count)
	return count, err
}
func (s *Store) MarkAllNotificationsRead(ctx context.Context, actor Identity) error {
	return s.markNotificationsRead(ctx, actor, "", true)
}
func (s *Store) MarkNotificationRead(ctx context.Context, actor Identity, id string) error {
	return s.markNotificationsRead(ctx, actor, id, false)
}
func (s *Store) markNotificationsRead(ctx context.Context, actor Identity, id string, all bool) error {
	seq, err := strconv.ParseInt(id, 10, 64)
	if !all && (err != nil || seq <= 0) {
		return ErrInvalid
	}
	// 已读操作天然幂等；权限检查和修改仍在同一事务中进行。
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE instance SET id=id"); err != nil {
		return err
	}
	u, err := currentIdentity(ctx, tx, actor)
	if err != nil {
		return err
	}
	if u.ID == "" {
		return ErrUnauthenticated
	}
	result, err := tx.ExecContext(ctx, "UPDATE review_notifications SET is_read=1 WHERE recipient=? AND (?=1 OR sequence=?)", u.ID, all, seq)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if !all && n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}
