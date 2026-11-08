package hub

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/skillhubhq/skillhub/internal/hubdb"
)

// 待办仅包含可以独立审核的提交，人员管理员和普通维护者不会获得审核权。
const reviewableSQL = activeReleaseSQL + ` AND r.status=1 AND r.submitted_by<>@actor AND (@admin=1 OR EXISTS(SELECT 1 FROM namespace_grants g WHERE g.namespace=r.namespace AND g.role IN (3,4) AND ((g.kind=1 AND g.subject_id=@actor) OR (g.kind=2 AND EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.subject_id AND m.user_id=@actor)))))`

type ReviewTask struct {
	ReleaseID  string    `json:"release_id"`
	Coordinate string    `json:"coordinate"`
	CreatedAt  time.Time `json:"created_at"`
}
type ReviewTasks struct {
	Tasks    []ReviewTask
	Total    uint32
	OldestAt time.Time
	Next     string
}

func (s *Store) ListReviewTasks(ctx context.Context, actor Identity, page Page) (ReviewTasks, error) {
	out := ReviewTasks{Tasks: []ReviewTask{}}
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = authenticatedOnly(tx, u); err != nil {
		return out, err
	}
	var oldest sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),MIN(r.created_at) FROM releases r WHERE `+reviewableSQL, principalArgs(u)...).Scan(&out.Total, &oldest); err != nil {
		return out, err
	}
	if oldest.Valid {
		out.OldestAt = time.Unix(oldest.Int64, 0)
	}
	p, err := pageQuery(s, ctx, tx, page, scope(u.ID, "review-tasks", nil), `SELECT r.id,r.namespace||'/'||r.name||'@'||r.version,r.created_at,r.sequence FROM releases r WHERE `+reviewableSQL+` AND r.sequence>CAST(COALESCE(NULLIF(@after,''),'0') AS BIGINT) ORDER BY r.sequence LIMIT @limit`, principalArgs(u), func(rows *sql.Rows) (ReviewTask, string, error) {
		var task ReviewTask
		var at, seq int64
		err := rows.Scan(&task.ReleaseID, &task.Coordinate, &at, &seq)
		task.CreatedAt = time.Unix(at, 0)
		return task, strconv.FormatInt(seq, 10), err
	})
	out.Tasks, out.Next = p.Items, p.Next
	return out, err
}
func (s *Store) SetFeaturedSkill(ctx context.Context, actor Identity, key, namespace, name string, featured bool) error {
	if namespace == "" || name == "" || len(namespace) > 128 || len(name) > 128 {
		return ErrInvalid
	}
	cmd := struct {
		Namespace, Name string
		Featured        bool
	}{namespace, name, featured}
	_, err := mutate(s, ctx, actor, "skill.featured", key, cmd, adminOnly, func(tx *hubdb.Tx, u Identity) (struct{}, error) {
		if err := requireRole(ctx, tx, u, namespace); err != nil {
			return struct{}{}, err
		}
		if featured {
			var exists bool
			if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM releases WHERE namespace=? AND name=? AND status=2)", namespace, name).Scan(&exists); err != nil {
				return struct{}{}, err
			}
			if !exists {
				return struct{}{}, ErrNotFound
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO featured_skills(namespace,name) VALUES(?,?) ON CONFLICT DO NOTHING", namespace, name); err != nil {
				return struct{}{}, err
			}
		} else {
			if _, err := tx.ExecContext(ctx, "DELETE FROM featured_skills WHERE namespace=? AND name=?", namespace, name); err != nil {
				return struct{}{}, err
			}
		}
		return struct{}{}, s.auditChange(ctx, tx, u.ID, "skill.featured", namespace+"/"+name, cmd)
	})
	return err
}
