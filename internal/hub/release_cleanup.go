package hub

import (
	"context"
	"encoding/json"
	"errors"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
)

type DeleteRelease struct{ ReleaseID, ConfirmCoordinate string }
type DeletedRelease struct{ ReleaseID string }

// 清理包正文；版本标识与技能互动保留，历史列表隐藏清理后的记录。
func (s *Store) DeleteRelease(ctx context.Context, actor Identity, key string, cmd DeleteRelease) (DeletedRelease, error) {
	if cmd.ReleaseID == "" || cmd.ConfirmCoordinate == "" {
		return DeletedRelease{}, ErrInvalid
	}
	result, err := mutate(s, ctx, actor, "release.delete", key, cmd, func(tx *hubdb.Tx, user Identity) error {
		var ns string
		err := tx.QueryRowContext(ctx, "SELECT namespace FROM releases WHERE id=? UNION ALL SELECT namespace FROM release_tombstones WHERE id=?", cmd.ReleaseID, cmd.ReleaseID).Scan(&ns)
		if err != nil {
			return assertExists(err)
		}
		return requireRole(ctx, tx, user, ns, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER)
	}, func(tx *hubdb.Tx, user Identity) (DeletedRelease, error) {
		r, err := releaseByID(ctx, tx, cmd.ReleaseID)
		if err != nil {
			return DeletedRelease{}, err
		}
		coordinate := r.Namespace + "/" + r.Name + "@" + r.Version
		if cmd.ConfirmCoordinate != coordinate {
			return DeletedRelease{}, ErrInvalid
		}
		if r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_REJECTED && r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_YANKED {
			return DeletedRelease{}, ErrConflict
		}
		if err = purgeReleaseResponses(ctx, tx, r.ID); err != nil {
			return DeletedRelease{}, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO package_cleanup(digest) VALUES(?) ON CONFLICT DO NOTHING", r.Digest); err != nil {
			return DeletedRelease{}, err
		}
		for _, table := range []string{"evaluation_evidence", "review_notifications", "release_scans"} {
			if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE release_id=?", r.ID); err != nil {
				return DeletedRelease{}, err
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO release_tombstones(id,namespace,name,version,digest,deleted_at) SELECT id,namespace,name,version,digest,? FROM releases WHERE id=? AND ever_published=1 ON CONFLICT DO NOTHING", s.now().Unix(), r.ID); err != nil {
			return DeletedRelease{}, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE releases SET cleaned=1,markdown='',archive_bytes=0 WHERE id=?", r.ID); err != nil {
			return DeletedRelease{}, err
		}
		if err = s.auditChange(ctx, tx, user.ID, "release.delete", r.ID, map[string]string{"coordinate": coordinate, "digest": r.Digest}); err != nil {
			return DeletedRelease{}, err
		}
		return DeletedRelease{ReleaseID: r.ID}, nil
	})
	if err == nil {
		err = s.CollectNamespacePackages(ctx)
	}
	return result, err
}

// 幂等响应中的正文、评论和评测副本也必须清除，避免重试读回已删除内容。
func purgeReleaseResponses(ctx context.Context, tx *hubdb.Tx, id string) error {
	ids := map[string]bool{id: true}
	for _, table := range []string{"comments", "evaluation_evidence", "community_reports"} {
		rows, err := tx.QueryContext(ctx, "SELECT id FROM "+table+" WHERE release_id=?", id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var value string
			if err = rows.Scan(&value); err != nil {
				rows.Close()
				return err
			}
			ids[value] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT actor,method,key,result FROM mutations WHERE method<>'release.delete'")
	if err != nil {
		return err
	}
	type cacheKey struct{ actor, method, key string }
	var remove []cacheKey
	for rows.Next() {
		var k cacheKey
		var raw []byte
		if err = rows.Scan(&k.actor, &k.method, &k.key, &raw); err != nil {
			rows.Close()
			return err
		}
		var result any
		if json.Unmarshal(raw, &result) != nil {
			rows.Close()
			return errors.New("invalid mutation cache")
		}
		if referencesDeletedID(result, ids) {
			remove = append(remove, k)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, k := range remove {
		if _, err = tx.ExecContext(ctx, "UPDATE mutations SET result=?,invalidated=1 WHERE actor=? AND method=? AND key=?", []byte("null"), k.actor, k.method, k.key); err != nil {
			return err
		}
	}
	return nil
}
func referencesDeletedID(value any, ids map[string]bool) bool {
	switch v := value.(type) {
	case string:
		return ids[v]
	case []any:
		for _, item := range v {
			if referencesDeletedID(item, ids) {
				return true
			}
		}
	case map[string]any:
		for _, item := range v {
			if referencesDeletedID(item, ids) {
				return true
			}
		}
	}
	return false
}
