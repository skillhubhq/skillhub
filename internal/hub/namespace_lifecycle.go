package hub

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

// NamespaceLifecycle 是删除前的权限受控影响预览，不包含包正文或凭据。
type NamespaceLifecycle struct {
	Name     string `json:"name"`
	Revision uint64 `json:"revision"`
	Archived bool   `json:"archived"`
	Releases uint64 `json:"releases"`
	Grants   uint64 `json:"grants"`
	Comments uint64 `json:"comments"`
	Evidence uint64 `json:"evidence"`
}
type ChangeNamespaceLifecycle struct {
	Name             string
	ExpectedRevision uint64
	Archived         bool
	ConfirmName      string
}

func namespaceLifecycle(ctx context.Context, tx *hubdb.Tx, name string) (NamespaceLifecycle, error) {
	v := NamespaceLifecycle{Name: name}
	err := tx.QueryRowContext(ctx, `SELECT revision,archived,
 (SELECT count(*) FROM releases WHERE namespace=n.name),
 (SELECT count(*) FROM namespace_grants WHERE namespace=n.name),
 (SELECT count(*) FROM comments c JOIN releases r ON c.release_id=r.id WHERE r.namespace=n.name),
 (SELECT count(*) FROM evaluation_evidence e JOIN releases r ON e.release_id=r.id WHERE r.namespace=n.name)
 FROM namespaces n WHERE n.name=?`, name).Scan(&v.Revision, &v.Archived, &v.Releases, &v.Grants, &v.Comments, &v.Evidence)
	return v, assertExists(err)
}
func (s *Store) GetNamespaceLifecycle(ctx context.Context, actor Identity, name string) (NamespaceLifecycle, error) {
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return NamespaceLifecycle{}, err
	}
	defer tx.Rollback()
	if err = adminOnly(tx, u); err != nil {
		return NamespaceLifecycle{}, err
	}
	return namespaceLifecycle(ctx, tx, name)
}
func (s *Store) SetNamespaceArchived(ctx context.Context, actor Identity, key string, cmd ChangeNamespaceLifecycle) (NamespaceLifecycle, error) {
	if !packageformat.ValidName(cmd.Name) || cmd.ExpectedRevision == 0 {
		return NamespaceLifecycle{}, ErrInvalid
	}
	return mutate(s, ctx, actor, "namespace.archive", key, cmd, adminOnly, func(tx *hubdb.Tx, u Identity) (NamespaceLifecycle, error) {
		v, err := namespaceLifecycle(ctx, tx, cmd.Name)
		if err != nil {
			return v, err
		}
		if v.Revision != cmd.ExpectedRevision {
			return v, ErrConflict
		}
		if _, err = tx.ExecContext(ctx, "UPDATE namespaces SET archived=?,revision=revision+1 WHERE name=?", cmd.Archived, cmd.Name); err != nil {
			return v, err
		}
		v.Archived = cmd.Archived
		v.Revision++
		return v, s.auditChange(ctx, tx, u.ID, "namespace.archive", cmd.Name, v)
	})
}

// PurgeNamespace 先提交逻辑删除和垃圾回收队列，再删除未被其他版本引用的包；崩溃后可重试。
func (s *Store) PurgeNamespace(ctx context.Context, actor Identity, key string, cmd ChangeNamespaceLifecycle) (NamespaceLifecycle, error) {
	if !packageformat.ValidName(cmd.Name) || cmd.ExpectedRevision == 0 || cmd.ConfirmName != cmd.Name {
		return NamespaceLifecycle{}, ErrInvalid
	}
	result, err := mutate(s, ctx, actor, "namespace.purge", key, cmd, adminOnly, func(tx *hubdb.Tx, u Identity) (NamespaceLifecycle, error) {
		v, err := namespaceLifecycle(ctx, tx, cmd.Name)
		if err != nil {
			return v, err
		}
		if !v.Archived || v.Revision != cmd.ExpectedRevision {
			return v, ErrConflict
		}
		if err = purgeNamespaceResponses(ctx, tx, cmd.Name); err != nil {
			return v, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO package_cleanup(digest) SELECT DISTINCT digest FROM releases WHERE namespace=? ON CONFLICT DO NOTHING", cmd.Name); err != nil {
			return v, err
		}
		for _, table := range []string{"community_reports", "evaluation_evidence", "review_notifications", "release_scans", "reactions", "comments", "release_groups", "release_tags"} {
			if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE release_id IN (SELECT id FROM releases WHERE namespace=?)", cmd.Name); err != nil {
				return v, err
			}
		}
		for _, table := range []string{"releases", "featured_skills", "namespace_grants", "team_profiles", "personal_namespaces"} {
			if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE namespace=?", cmd.Name); err != nil {
				return v, err
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO namespace_tombstones(name,deleted_at) VALUES(?,?)", cmd.Name, s.now().Unix()); err != nil {
			return v, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM namespaces WHERE name=?", cmd.Name); err != nil {
			return v, err
		}
		return v, s.auditChange(ctx, tx, u.ID, "namespace.purge", cmd.Name, v)
	})
	if err == nil {
		err = s.CollectNamespacePackages(ctx)
	}
	return result, err
}

// CollectNamespacePackages 与所有提交共用写锁；删除失败保留队列，重启或幂等重试继续。
func (s *Store) CollectNamespacePackages(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE instance SET id=id"); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT digest FROM package_cleanup")
	if err != nil {
		return err
	}
	var digests []string
	for rows.Next() {
		var d string
		if err = rows.Scan(&d); err != nil {
			rows.Close()
			return err
		}
		digests = append(digests, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, d := range digests {
		if !validBackupDigest(d) {
			return errors.New("invalid cleanup digest")
		}
		var referenced bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM releases WHERE digest=?)", d).Scan(&referenced); err != nil {
			return err
		}
		if !referenced {
			if err = os.Remove(filepath.Join(s.root, "packages", d[7:]+".zip")); err != nil && !errors.Is(err, os.ErrNotExist) {
				return errors.New("package cleanup incomplete; retry the same request")
			}
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM package_cleanup WHERE digest=?", d); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// 删除幂等缓存中的团队资料、正文和评论副本；审计与其他团队缓存保留。
func purgeNamespaceResponses(ctx context.Context, tx *hubdb.Tx, name string) error {
	ids := map[string]bool{}
	rows, err := tx.QueryContext(ctx, "SELECT c.id FROM comments c JOIN releases r ON r.id=c.release_id WHERE r.namespace=?", name)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, "SELECT actor,method,key,result FROM mutations WHERE method IN ('release.submit','release.review','release.yank','community.team','community.comment')")
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
		var result map[string]json.RawMessage
		if err = json.Unmarshal(raw, &result); err != nil {
			rows.Close()
			return err
		}
		var ns, id string
		_ = json.Unmarshal(result["Namespace"], &ns)
		if ns == "" {
			_ = json.Unmarshal(result["namespace"], &ns)
		}
		_ = json.Unmarshal(result["comment_id"], &id)
		if ns == name || ids[id] {
			remove = append(remove, k)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, k := range remove {
		if _, err = tx.ExecContext(ctx, "DELETE FROM mutations WHERE actor=? AND method=? AND key=?", k.actor, k.method, k.key); err != nil {
			return err
		}
	}
	return nil
}
