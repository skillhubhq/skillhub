package hub

import (
	"bytes"
	"context"
	"database/sql"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

type FileDifference struct {
	Path             string `json:"path"`
	Change           string `json:"change"`
	BeforeText       string `json:"before_text"`
	AfterText        string `json:"after_text"`
	TextAvailable    bool   `json:"text_available"`
	BeforeExecutable bool   `json:"before_executable"`
	AfterExecutable  bool   `json:"after_executable"`
}
type ReleaseDifference struct {
	Files        []FileDifference `json:"files"`
	BeforeDigest string           `json:"before_digest"`
	AfterDigest  string           `json:"after_digest"`
}

func (s *Store) CompareReleases(ctx context.Context, actor Identity, namespace, name, beforeID, afterID string) (ReleaseDifference, error) {
	result := ReleaseDifference{Files: []FileDifference{}}
	if !packageformat.ValidName(namespace) || !packageformat.ValidName(name) || beforeID == "" || afterID == "" {
		return result, ErrInvalid
	}
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	bundles := []*packageformat.Bundle{}
	for _, id := range []string{beforeID, afterID} {
		args := append(principalArgs(u), sql.Named("id", id), sql.Named("ns", namespace), sql.Named("name", name))
		r, e := scanRelease(tx.QueryRowContext(ctx, "SELECT "+releaseColumns+" FROM releases r WHERE r.id=@id AND r.namespace=@ns AND r.name=@name AND r.cleaned=0 AND ((r.status=2 AND "+readableSQL+") OR "+manageableSQL+")", args...))
		if e != nil {
			return result, assertExists(e)
		}
		raw, e := s.readBlob(r.Digest)
		if e != nil {
			return result, e
		}
		b, e := packageformat.Inspect(raw)
		if e != nil {
			return result, e
		}
		if e = s.validateSuite(ctx, tx, u, b); e != nil {
			return result, e
		}
		bundles = append(bundles, b)
	}
	result.BeforeDigest, result.AfterDigest = bundles[0].Digest, bundles[1].Digest
	before, after := map[string]packageformat.File{}, map[string]packageformat.File{}
	paths := map[string]bool{}
	for _, f := range bundles[0].Files {
		before[f.Path] = f
		paths[f.Path] = true
	}
	for _, f := range bundles[1].Files {
		after[f.Path] = f
		paths[f.Path] = true
	}
	ordered := []string{}
	for p := range paths {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)
	budget := 512 << 10
	for _, path := range ordered {
		a, aok := before[path]
		b, bok := after[path]
		if aok && bok && bytes.Equal(a.Data, b.Data) && a.Executable == b.Executable {
			continue
		}
		kind := "modified"
		if !aok {
			kind = "added"
		}
		if !bok {
			kind = "removed"
		}
		d := FileDifference{Path: path, Change: kind, BeforeExecutable: a.Executable, AfterExecutable: b.Executable}
		size := len(a.Data) + len(b.Data)
		if size <= 128<<10 && bytes.Count(a.Data, []byte("\n"))+bytes.Count(b.Data, []byte("\n")) <= 4000 && size <= budget && utf8.Valid(a.Data) && utf8.Valid(b.Data) && !bytes.ContainsRune(a.Data, 0) && !bytes.ContainsRune(b.Data, 0) {
			d.TextAvailable = true
			d.BeforeText = string(a.Data)
			d.AfterText = string(b.Data)
			budget -= size
		}
		result.Files = append(result.Files, d)
	}
	return result, nil
}

type Subscription struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

func subscriptionVisible(ctx context.Context, tx *hubdb.Tx, u Identity, namespace, name string) error {
	args := append(principalArgs(u), sql.Named("ns", namespace), sql.Named("name", name))
	var exists bool
	err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM releases r WHERE r.namespace=@ns AND r.name=@name AND r.status=2 AND r.cleaned=0 AND "+activeReleaseSQL+" AND "+readableSQL+")", args...).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}
func (s *Store) GetSubscription(ctx context.Context, actor Identity, namespace, name string) (bool, error) {
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err = authenticatedOnly(tx, u); err != nil {
		return false, err
	}
	var active bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM subscriptions WHERE user_id=? AND namespace=? AND name=?)", u.ID, namespace, name).Scan(&active)
	return active, err
}
func (s *Store) SetSubscription(ctx context.Context, actor Identity, key, namespace, name string, active bool) error {
	if !packageformat.ValidName(namespace) || !packageformat.ValidName(name) {
		return ErrInvalid
	}
	cmd := struct {
		Namespace, Name string
		Active          bool
	}{namespace, name, active}
	_, err := mutate(s, ctx, actor, "skill.subscribe", key, cmd, authenticatedOnly, func(tx *hubdb.Tx, u Identity) (struct{}, error) {
		if !active {
			_, e := tx.ExecContext(ctx, "DELETE FROM subscriptions WHERE user_id=? AND namespace=? AND name=?", u.ID, namespace, name)
			return struct{}{}, e
		}
		if e := subscriptionVisible(ctx, tx, u, namespace, name); e != nil {
			return struct{}{}, e
		}
		var count int
		if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM subscriptions WHERE user_id=?", u.ID).Scan(&count); e != nil {
			return struct{}{}, e
		}
		var exists bool
		if e := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM subscriptions WHERE user_id=? AND namespace=? AND name=?)", u.ID, namespace, name).Scan(&exists); e != nil {
			return struct{}{}, e
		}
		if count >= 500 && !exists {
			return struct{}{}, ErrRateLimited
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO subscriptions(user_id,namespace,name) VALUES(?,?,?) ON CONFLICT DO NOTHING", u.ID, namespace, name)
		return struct{}{}, e
	})
	return err
}
func (s *Store) ListSubscriptions(ctx context.Context, actor Identity, page Page) (PageResult[Subscription], error) {
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[Subscription]{}, err
	}
	defer tx.Rollback()
	if err = authenticatedOnly(tx, u); err != nil {
		return PageResult[Subscription]{}, err
	}
	return pageQuery(s, ctx, tx, page, scope(u.ID, "subscriptions", nil), `SELECT sub.namespace,sub.name,EXISTS(SELECT 1 FROM releases r WHERE r.namespace=sub.namespace AND r.name=sub.name AND r.status=2 AND r.cleaned=0 AND `+activeReleaseSQL+` AND `+readableSQL+`) FROM subscriptions sub WHERE sub.user_id=@actor AND sub.namespace||'/'||sub.name>@after ORDER BY sub.namespace||'/'||sub.name LIMIT @limit`, principalArgs(u), func(rows *sql.Rows) (Subscription, string, error) {
		var v Subscription
		e := rows.Scan(&v.Namespace, &v.Name, &v.Available)
		return v, v.Namespace + "/" + v.Name, e
	})
}

// 发布与通知共用事务；订阅不改变受众，读取通知时仍重验当前访问权。
func (s *Store) notifySubscribers(ctx context.Context, tx *hubdb.Tx, id string) error {
	visible := strings.NewReplacer("@actor", "sub.user_id", "@admin", "u.administrator").Replace(readableSQL)
	_, err := tx.ExecContext(ctx, `INSERT INTO review_notifications(recipient,release_id,kind,comment_id,created_at) SELECT sub.user_id,r.id,'update','update',? FROM subscriptions sub JOIN users u ON u.id=sub.user_id JOIN releases r ON r.namespace=sub.namespace AND r.name=sub.name WHERE r.id=? AND r.status=2 AND u.disabled=0 AND sub.user_id<>r.submitted_by AND `+visible+` ON CONFLICT DO NOTHING`, s.now().Unix(), id)
	return err
}
