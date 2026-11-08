package hub

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Participation struct {
	CommunityMode                                             bool
	PersonalNamespace                                         string
	Versions, VersionLimit, SubmissionsToday, SubmissionLimit int
	StorageBytes, StorageLimit                                int64
}

func (s *Store) GetParticipation(ctx context.Context, actor Identity) (Participation, error) {
	tx, u, e := s.read(ctx, actor)
	if e != nil {
		return Participation{}, e
	}
	defer tx.Rollback()
	if e = authenticatedOnly(tx, u); e != nil {
		return Participation{}, e
	}
	v := Participation{CommunityMode: s.policy.IsCommunity()}
	if !v.CommunityMode {
		return v, nil
	}
	e = tx.QueryRowContext(ctx, "SELECT namespace FROM personal_namespaces WHERE user_id=?", u.ID).Scan(&v.PersonalNamespace)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return v, e
	}
	e = tx.QueryRowContext(ctx, "SELECT count(*),COALESCE(sum(archive_bytes),0),COALESCE(sum(CASE WHEN created_at>? THEN 1 ELSE 0 END),0) FROM releases WHERE submitted_by=?", s.now().Add(-24*time.Hour).Unix(), u.ID).Scan(&v.Versions, &v.StorageBytes, &v.SubmissionsToday)
	v.VersionLimit = s.policy.Limits.VersionsPerUser
	v.StorageLimit = s.policy.Limits.BytesPerUser
	v.SubmissionLimit = s.policy.Limits.SubmissionsPerDay
	return v, e
}

type ReportContent struct{ ReleaseID, CommentID, Reason string }
type ContentReport struct {
	ID, Reporter, ReleaseID, CommentID, Reason, Resolution, Coordinate, Content string
	Resolved                                                                    bool
	CreatedAt, ResolvedAt                                                       time.Time
}
type ResolveReport struct {
	ID, Note string
	Action   taxonomy.ReportResolution
}

func (s *Store) ReportContent(ctx context.Context, actor Identity, key string, cmd ReportContent) (string, error) {
	cmd.Reason = strings.TrimSpace(cmd.Reason)
	if cmd.ReleaseID == "" || !utf8.ValidString(cmd.Reason) || utf8.RuneCountInString(cmd.Reason) > 1000 || cmd.Reason == "" || strings.ContainsRune(cmd.Reason, 0) {
		return "", ErrInvalid
	}
	return mutate(s, ctx, actor, "content.report", key, cmd, func(tx *hubdb.Tx, u Identity) error {
		if _, e := communityRelease(ctx, tx, u, cmd.ReleaseID); e != nil {
			return e
		}
		if cmd.CommentID != "" {
			var exists bool
			if e := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM comments WHERE id=? AND release_id=? AND deleted=0)", cmd.CommentID, cmd.ReleaseID).Scan(&exists); e != nil {
				return e
			}
			if !exists {
				return ErrNotFound
			}
		}
		return nil
	}, func(tx *hubdb.Tx, u Identity) (string, error) {
		var id string
		e := tx.QueryRowContext(ctx, "SELECT id FROM community_reports WHERE reporter=? AND release_id=? AND comment_id=?", u.ID, cmd.ReleaseID, cmd.CommentID).Scan(&id)
		if e == nil {
			return id, nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return "", e
		}
		limit := 10
		if s.policy.IsCommunity() {
			limit = s.policy.Limits.ReportsPerDay
		}
		var count int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM community_reports WHERE reporter=? AND created_at>?", u.ID, s.now().Add(-24*time.Hour).Unix()).Scan(&count); e != nil {
			return "", e
		}
		if count >= limit {
			return "", ErrQuota
		}
		id = uuid.NewString()
		if _, e = tx.ExecContext(ctx, "INSERT INTO community_reports(id,reporter,release_id,comment_id,reason,created_at) VALUES(?,?,?,?,?,?)", id, u.ID, cmd.ReleaseID, cmd.CommentID, cmd.Reason, s.now().Unix()); e != nil {
			return "", e
		}
		return id, audit(ctx, tx, u.ID, "content.report", id, s.now())
	})
}
func (s *Store) ListReports(ctx context.Context, actor Identity, resolved bool, page Page) (PageResult[ContentReport], error) {
	tx, u, e := s.read(ctx, actor)
	if e != nil {
		return PageResult[ContentReport]{}, e
	}
	defer tx.Rollback()
	if e = adminOnly(tx, u); e != nil {
		return PageResult[ContentReport]{}, e
	}
	return pageQuery(s, ctx, tx, page, scope(u.ID, "content.reports", resolved), `SELECT p.sequence,p.id,p.reporter,p.release_id,p.comment_id,p.reason,p.resolved,p.resolution,p.created_at,p.resolved_at,r.namespace||'/'||r.name||'@'||r.version,CASE WHEN p.comment_id='' THEN r.description ELSE COALESCE(c.body,'') END FROM community_reports p JOIN releases r ON r.id=p.release_id LEFT JOIN comments c ON c.id=p.comment_id WHERE `+activeReleaseSQL+` AND p.resolved=@resolved AND (@after='' OR p.sequence<CAST(COALESCE(NULLIF(@after,''),'0') AS BIGINT)) ORDER BY p.sequence DESC LIMIT @limit`, []any{sql.Named("resolved", resolved)}, func(rows *sql.Rows) (ContentReport, string, error) {
		var v ContentReport
		var created, done, sequence int64
		e := rows.Scan(&sequence, &v.ID, &v.Reporter, &v.ReleaseID, &v.CommentID, &v.Reason, &v.Resolved, &v.Resolution, &created, &done, &v.Coordinate, &v.Content)
		v.CreatedAt = time.Unix(created, 0)
		if done > 0 {
			v.ResolvedAt = time.Unix(done, 0)
		}
		return v, strconv.FormatInt(sequence, 10), e
	})
}
func (s *Store) ResolveReport(ctx context.Context, actor Identity, key string, cmd ResolveReport) error {
	cmd.Note = strings.TrimSpace(cmd.Note)
	if cmd.ID == "" || cmd.Note == "" || !utf8.ValidString(cmd.Note) || utf8.RuneCountInString(cmd.Note) > 1000 || strings.ContainsRune(cmd.Note, 0) || (cmd.Action != taxonomy.ReportResolution_REPORT_RESOLUTION_DISMISS && cmd.Action != taxonomy.ReportResolution_REPORT_RESOLUTION_REMOVE_CONTENT) {
		return ErrInvalid
	}
	_, e := mutate(s, ctx, actor, "content.resolve", key, cmd, func(tx *hubdb.Tx, u Identity) error {
		if err := adminOnly(tx, u); err != nil {
			return err
		}
		var namespace string
		if err := tx.QueryRowContext(ctx, "SELECT r.namespace FROM community_reports p JOIN releases r ON r.id=p.release_id WHERE p.id=?", cmd.ID).Scan(&namespace); err != nil {
			return assertExists(err)
		}
		return requireRole(ctx, tx, u, namespace)
	}, func(tx *hubdb.Tx, u Identity) (bool, error) {
		var release, comment string
		var resolved bool
		e := tx.QueryRowContext(ctx, "SELECT release_id,comment_id,resolved FROM community_reports WHERE id=?", cmd.ID).Scan(&release, &comment, &resolved)
		if e != nil {
			return false, assertExists(e)
		}
		if resolved {
			return false, ErrConflict
		}
		action := "dismiss"
		if cmd.Action == taxonomy.ReportResolution_REPORT_RESOLUTION_REMOVE_CONTENT {
			action = "remove_content"
			if comment != "" {
				_, e = tx.ExecContext(ctx, "UPDATE comments SET body='',deleted=1 WHERE id=? AND release_id=?", comment, release)
			} else {
				// 管理备注仅对管理员可见，不能通过发布者预览泄漏。
				_, e = tx.ExecContext(ctx, "UPDATE releases SET status=4,review_reason=? WHERE id=?", "Withdrawn by a Hub administrator following a content report.", release)
			}
			if e != nil {
				return false, e
			}
		}
		if _, e = tx.ExecContext(ctx, "UPDATE community_reports SET resolved=1,resolution=?,resolved_at=? WHERE id=?", action+": "+cmd.Note, s.now().Unix(), cmd.ID); e != nil {
			return false, e
		}
		return true, audit(ctx, tx, u.ID, "content."+action, cmd.ID, s.now())
	})
	return e
}
