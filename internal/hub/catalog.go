package hub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/provenance"
	"github.com/skillhubhq/skillhub/internal/skillscan"
	"golang.org/x/mod/semver"
)

type Release struct {
	Provenance    *provenance.Record `json:"provenance,omitempty"`
	DisplayNameEN string             `json:"display_name_en"`
	DisplayNameZH string             `json:"display_name_zh"`
	Icon          taxonomy.SkillIcon `json:"icon"`
	ID            string
	Namespace     string
	Name          string
	Version       string
	Digest        string
	Status        taxonomy.ReleaseStatus
	Visibility    taxonomy.Visibility
	GroupIDs      []string
	SubmittedBy   string
	CreatedAt     time.Time
	ReviewReason  string
	Markdown      string
	Description   string
	Category      string
	Tags          []string
	Sequence      int64
}

type Skill struct {
	Revision      uint64             `json:"revision"`
	CanEdit       bool               `json:"can_edit"`
	Provenance    *provenance.Record `json:"provenance,omitempty"`
	Featured      bool               `json:"featured"`
	DisplayNameEN string             `json:"display_name_en"`
	DisplayNameZH string             `json:"display_name_zh"`
	Icon          taxonomy.SkillIcon `json:"icon"`
	Namespace     string
	Name          string
	Description   string
	Category      string
	Tags          []string
	LatestVersion string
}
type Search struct {
	IncludeArchived bool
	Order           taxonomy.CatalogOrder
	FeaturedOnly    bool
	Query           string
	Namespace       string
	Name            string
	Category        string
	Tags            []string
}
type SubmitRelease struct {
	Provenance    *provenance.Record `json:"provenance,omitempty"`
	DisplayNameEN string             `json:"display_name_en"`
	DisplayNameZH string             `json:"display_name_zh"`
	Icon          taxonomy.SkillIcon `json:"icon"`
	Namespace     string
	Version       string
	Archive       []byte `json:"-"`
	ArchiveDigest string
	Visibility    taxonomy.Visibility
	GroupIDs      []string
	Category      string
	Tags          []string
}
type ReviewRelease struct {
	ReleaseID string
	Approve   bool
	Reason    string
}
type YankRelease struct {
	ReleaseID string
	Reason    string
}

// 权限表达式在目录、精确版本和下载中共用；所有角色值都限定在已知集合内。
const activeReleaseSQL = `EXISTS(SELECT 1 FROM namespaces active_namespace WHERE active_namespace.name=r.namespace AND active_namespace.archived=0)`
const readableSQL = `(@admin=1 OR EXISTS(SELECT 1 FROM skills skill WHERE skill.namespace=r.namespace AND skill.name=r.name AND (skill.visibility=1 OR (@actor<>'' AND (skill.visibility=2 OR
 EXISTS(SELECT 1 FROM json_each(skill.audience_groups) a JOIN group_members m ON a.value=m.group_id WHERE m.user_id=@actor) OR
 EXISTS(SELECT 1 FROM namespace_grants g WHERE g.namespace=r.namespace AND g.role IN (2,3,4) AND ((g.kind=1 AND g.subject_id=@actor) OR (g.kind=2 AND EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.subject_id AND m.user_id=@actor)))))))))`
const manageableSQL = activeReleaseSQL + ` AND (@admin=1 OR (@actor<>'' AND EXISTS(SELECT 1 FROM namespace_grants g WHERE g.namespace=r.namespace AND g.role IN (2,3,4) AND ((g.kind=1 AND g.subject_id=@actor) OR (g.kind=2 AND EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.subject_id AND m.user_id=@actor))))))`
const releaseColumns = `r.id,r.namespace,r.name,r.version,r.digest,r.status,CASE WHEN r.ever_published=1 THEN (SELECT visibility FROM skills WHERE namespace=r.namespace AND name=r.name) ELSE r.visibility END,r.submitted_by,r.created_at,r.review_reason,r.markdown,r.description,r.category,r.sequence,r.display_name_en,r.display_name_zh,r.icon,r.provenance,
 CASE WHEN r.ever_published=1 THEN (SELECT audience_groups FROM skills WHERE namespace=r.namespace AND name=r.name) ELSE COALESCE((SELECT json_group_array(group_id) FROM (SELECT group_id FROM release_groups WHERE release_id=r.id ORDER BY group_id) AS ordered_groups),'[]') END,
 COALESCE((SELECT json_group_array(tag) FROM (SELECT tag FROM release_tags WHERE release_id=r.id ORDER BY tag) AS ordered_tags),'[]')`

type scanner interface{ Scan(...any) error }

func scanRelease(row scanner) (Release, error) {
	var r Release
	var timestamp int64
	var groups, tags, origin string
	err := row.Scan(&r.ID, &r.Namespace, &r.Name, &r.Version, &r.Digest, &r.Status, &r.Visibility, &r.SubmittedBy, &timestamp, &r.ReviewReason, &r.Markdown, &r.Description, &r.Category, &r.Sequence, &r.DisplayNameEN, &r.DisplayNameZH, &r.Icon, &origin, &groups, &tags)
	if err != nil {
		return r, err
	}
	r.Provenance, err = provenance.Decode(origin)
	if err != nil {
		return r, err
	}
	r.CreatedAt = time.Unix(timestamp, 0)
	if err = json.Unmarshal([]byte(groups), &r.GroupIDs); err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(tags), &r.Tags); err != nil {
		return r, err
	}
	return r, nil
}
func releaseByID(ctx context.Context, tx *hubdb.Tx, id string) (Release, error) {
	r, err := scanRelease(tx.QueryRowContext(ctx, "SELECT "+releaseColumns+" FROM releases r WHERE r.id=? AND r.cleaned=0", id))
	return r, assertExists(err)
}
func principalArgs(user Identity) []any {
	return []any{sql.Named("actor", user.ID), sql.Named("admin", user.Administrator)}
}

func normalized(values []string, max int, names bool) ([]string, error) {
	if len(values) > max {
		return nil, ErrInvalid
	}
	result := append([]string{}, values...)
	sort.Strings(result)
	for i, value := range result {
		if len(value) == 0 || len(value) > 128 || (names && !packageformat.ValidName(value)) || (i > 0 && value == result[i-1]) {
			return nil, ErrInvalid
		}
	}
	return result, nil
}

func validDisplayName(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= 80 && strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069
	}) == -1
}

func (s *Store) SubmitRelease(ctx context.Context, actor Identity, key string, cmd SubmitRelease) (Release, error) {
	if !packageformat.ValidName(cmd.Namespace) || !semver.IsValid("v"+cmd.Version) || strings.TrimPrefix(semver.Canonical("v"+cmd.Version), "v") != strings.SplitN(cmd.Version, "+", 2)[0] || len(cmd.Version) > 128 || len(cmd.Archive) == 0 || len(cmd.Archive) > packageformat.MaxArchiveBytes {
		return Release{}, ErrInvalid
	}
	cmd.DisplayNameEN = strings.TrimSpace(cmd.DisplayNameEN)
	cmd.DisplayNameZH = strings.TrimSpace(cmd.DisplayNameZH)
	if !validDisplayName(cmd.DisplayNameEN) || !validDisplayName(cmd.DisplayNameZH) {
		return Release{}, ErrInvalid
	}
	if _, ok := taxonomy.SkillIcon_name[int32(cmd.Icon)]; !ok {
		return Release{}, ErrInvalid
	}
	if cmd.Visibility < taxonomy.Visibility_VISIBILITY_PUBLIC || cmd.Visibility > taxonomy.Visibility_VISIBILITY_GROUPS {
		return Release{}, ErrInvalid
	}
	if (cmd.Visibility == taxonomy.Visibility_VISIBILITY_GROUPS) != (len(cmd.GroupIDs) > 0) {
		return Release{}, ErrInvalid
	}
	var err error
	cmd.GroupIDs, err = normalized(cmd.GroupIDs, 100, false)
	if err != nil {
		return Release{}, err
	}
	cmd.Tags, err = normalized(cmd.Tags, 20, true)
	if err != nil {
		return Release{}, err
	}
	cmd.ArchiveDigest = packageformat.Digest(cmd.Archive)
	return mutate(s, ctx, actor, "release.submit", key, cmd, func(tx *hubdb.Tx, user Identity) error {
		return requireRole(ctx, tx, user, cmd.Namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER)
	}, func(tx *hubdb.Tx, user Identity) (Release, error) {
		if err := s.checkSubmissionQuota(ctx, tx, user, cmd); err != nil {
			return Release{}, err
		}
		bundle, err := packageformat.Inspect(cmd.Archive)
		if err != nil || len(bundleMarkdown(bundle)) > 256<<10 {
			return Release{}, ErrInvalid
		}
		if err = s.validateSuite(ctx, tx, user, bundle); err != nil {
			return Release{}, err
		}
		if err = provenance.Validate(cmd.Provenance, bundle); err != nil {
			return Release{}, ErrInvalid
		}
		var review bool
		if err = tx.QueryRowContext(ctx, "SELECT review_required FROM namespaces WHERE name=?", cmd.Namespace).Scan(&review); err != nil {
			return Release{}, err
		}
		if cmd.Category != "" {
			var available bool
			if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM categories WHERE name=? AND archived=0)", cmd.Category).Scan(&available); err != nil {
				return Release{}, err
			}
			if !available {
				return Release{}, ErrInvalid
			}
		}
		for _, group := range cmd.GroupIDs {
			var exists bool
			if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM groups WHERE id=?)", group).Scan(&exists); err != nil {
				return Release{}, err
			}
			if !exists {
				return Release{}, ErrInvalid
			}
		}
		var retired bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM release_tombstones WHERE namespace=? AND name=? AND version=?)", cmd.Namespace, bundle.Metadata.Name, cmd.Version).Scan(&retired); err != nil {
			return Release{}, err
		}
		if retired {
			return Release{}, ErrConflict
		}
		report, err := skillscan.Scan(ctx, bundle)
		if err != nil {
			return Release{}, err
		}
		review = cmd.Provenance != nil || review || s.policy.IsCommunity() || report.Blocking || !report.Complete || len(report.Findings) > 0
		status := taxonomy.ReleaseStatus_RELEASE_STATUS_PUBLISHED
		if review {
			status = taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING
		}
		// 同一未发布版本的新提交使用新的身份；旧审核不能作用于新内容。
		var published bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM releases WHERE namespace=? AND name=? AND version=? AND ever_published=1)", cmd.Namespace, bundle.Metadata.Name, cmd.Version).Scan(&published); err != nil {
			return Release{}, err
		}
		if published {
			return Release{}, ErrConflict
		}
		if _, err = tx.ExecContext(ctx, "UPDATE releases SET superseded=1,status=CASE WHEN status=1 THEN 3 ELSE status END WHERE namespace=? AND name=? AND version=?", cmd.Namespace, bundle.Metadata.Name, cmd.Version); err != nil {
			return Release{}, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO skills(namespace,name,visibility,display_name_en,display_name_zh,icon,description,category,tags) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING", cmd.Namespace, bundle.Metadata.Name, cmd.Visibility, cmd.DisplayNameEN, cmd.DisplayNameZH, cmd.Icon, bundle.Metadata.Description, cmd.Category, jsonText(cmd.Tags)); err != nil {
			return Release{}, err
		}
		id := uuid.NewString()
		_, err = tx.ExecContext(ctx, `INSERT INTO releases(id,namespace,name,version,digest,description,markdown,status,visibility,submitted_by,created_at,category,display_name_en,display_name_zh,icon,provenance) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, cmd.Namespace, bundle.Metadata.Name, cmd.Version, bundle.Digest, bundle.Metadata.Description, bundle.Markdown, status, cmd.Visibility, user.ID, s.now().Unix(), cmd.Category, cmd.DisplayNameEN, cmd.DisplayNameZH, cmd.Icon, provenance.Encode(cmd.Provenance))
		if isUnique(err) {
			return Release{}, ErrConflict
		}
		if err != nil {
			return Release{}, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE releases SET version_order=?,ever_published=? WHERE id=?", versionOrder(cmd.Version), status == taxonomy.ReleaseStatus_RELEASE_STATUS_PUBLISHED, id); err != nil {
			return Release{}, err
		}
		for _, group := range cmd.GroupIDs {
			if _, err = tx.ExecContext(ctx, "INSERT INTO release_groups(release_id,group_id) VALUES(?,?)", id, group); err != nil {
				return Release{}, err
			}
		}
		for _, tag := range cmd.Tags {
			if _, err = tx.ExecContext(ctx, "INSERT INTO release_tags(release_id,tag) VALUES(?,?)", id, tag); err != nil {
				return Release{}, err
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE releases SET archive_bytes=? WHERE id=?", len(cmd.Archive), id); err != nil {
			return Release{}, err
		}
		if err = s.saveScan(ctx, tx, id, report); err != nil {
			return Release{}, err
		}
		if err = s.writeBlob(bundle.Digest, cmd.Archive); err != nil {
			return Release{}, err
		}
		if err = audit(ctx, tx, user.ID, "release.submit", id, s.now()); err != nil {
			return Release{}, err
		}
		if !review {
			if err = s.notifySubscribers(ctx, tx, id); err != nil {
				return Release{}, err
			}
			if err = publishSkillDetails(ctx, tx, id); err != nil {
				return Release{}, err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE releases SET published_order=(SELECT max(sequence) FROM audit) WHERE id=?", id); err != nil {
				return Release{}, err
			}
		}
		return releaseByID(ctx, tx, id)
	})
}

func bundleMarkdown(bundle *packageformat.Bundle) string {
	if bundle == nil {
		return ""
	}
	return bundle.Markdown
}

func (s *Store) ReviewRelease(ctx context.Context, actor Identity, key string, cmd ReviewRelease) (Release, error) {
	if len(cmd.Reason) > 4096 || (!cmd.Approve && strings.TrimSpace(cmd.Reason) == "") {
		return Release{}, ErrInvalid
	}
	return mutate(s, ctx, actor, "release.review", key, cmd, func(tx *hubdb.Tx, user Identity) error {
		r, err := releaseByID(ctx, tx, cmd.ReleaseID)
		if err != nil {
			return err
		}
		return requireRole(ctx, tx, user, r.Namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER)
	}, func(tx *hubdb.Tx, user Identity) (Release, error) {
		r, err := releaseByID(ctx, tx, cmd.ReleaseID)
		if err != nil {
			return Release{}, err
		}
		if r.SubmittedBy == user.ID {
			return Release{}, ErrForbidden
		}
		if r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING {
			return Release{}, ErrConflict
		}
		status := taxonomy.ReleaseStatus_RELEASE_STATUS_REJECTED
		if cmd.Approve {
			raw, e := s.readBlob(r.Digest)
			if e != nil {
				return Release{}, e
			}
			bundle, e := packageformat.Inspect(raw)
			if e != nil {
				return Release{}, e
			}
			if e = s.validateSuite(ctx, tx, user, bundle); e != nil {
				return Release{}, e
			}
			report, err := latestScan(ctx, tx, r.ID)
			if err != nil {
				return Release{}, err
			}
			if report.Engine != skillscan.Engine || report.Digest != r.Digest || report.Blocking || ((!report.Complete || len(report.Findings) > 0) && strings.TrimSpace(cmd.Reason) == "") {
				return Release{}, ErrScanRequired
			}
			status = taxonomy.ReleaseStatus_RELEASE_STATUS_PUBLISHED
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO audit(actor_id,action,resource,created_at,detail) VALUES(?,?,?,?,?)", user.ID, "release.review", r.ID, s.now().Unix(), cmd.Reason); err != nil {
			return Release{}, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE releases SET status=?,review_reason=?,ever_published=CASE WHEN ?=2 THEN 1 ELSE ever_published END,published_order=CASE WHEN ?=2 THEN (SELECT max(sequence) FROM audit) ELSE 0 END WHERE id=?", status, cmd.Reason, status, status, r.ID); err != nil {
			return Release{}, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO review_notifications(recipient,release_id,approved,created_at) VALUES(?,?,?,?) ON CONFLICT DO NOTHING", r.SubmittedBy, r.ID, cmd.Approve, s.now().Unix()); err != nil {
			return Release{}, err
		}
		if status == taxonomy.ReleaseStatus_RELEASE_STATUS_PUBLISHED {
			if err = s.notifySubscribers(ctx, tx, r.ID); err != nil {
				return Release{}, err
			}
			if err = publishSkillDetails(ctx, tx, r.ID); err != nil {
				return Release{}, err
			}
		}
		return releaseByID(ctx, tx, r.ID)
	})
}

func (s *Store) YankRelease(ctx context.Context, actor Identity, key string, cmd YankRelease) (Release, error) {
	if len(cmd.Reason) > 4096 || strings.TrimSpace(cmd.Reason) == "" {
		return Release{}, ErrInvalid
	}
	return mutate(s, ctx, actor, "release.yank", key, cmd, func(tx *hubdb.Tx, user Identity) error {
		r, err := releaseByID(ctx, tx, cmd.ReleaseID)
		if err != nil {
			return err
		}
		return requireRole(ctx, tx, user, r.Namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER)
	}, func(tx *hubdb.Tx, user Identity) (Release, error) {
		r, err := releaseByID(ctx, tx, cmd.ReleaseID)
		if err != nil {
			return Release{}, err
		}
		if r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PUBLISHED && r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING {
			return Release{}, ErrConflict
		}
		if _, err = tx.ExecContext(ctx, "UPDATE releases SET status=4,review_reason=? WHERE id=?", cmd.Reason, r.ID); err != nil {
			return Release{}, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO audit(actor_id,action,resource,created_at,detail) VALUES(?,?,?,?,?)", user.ID, "release.yank", r.ID, s.now().Unix(), cmd.Reason); err != nil {
			return Release{}, err
		}
		return releaseByID(ctx, tx, r.ID)
	})
}

func (s *Store) SearchSkills(ctx context.Context, actor Identity, filter Search, page Page) (PageResult[Skill], error) {
	if len(filter.Query) > 800 || filter.Order < 0 || filter.Order > taxonomy.CatalogOrder_CATALOG_ORDER_RECENT {
		return PageResult[Skill]{}, ErrInvalid
	}
	var err error
	filter.Tags, err = normalized(filter.Tags, 20, true)
	if err != nil {
		return PageResult[Skill]{}, err
	}
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[Skill]{}, err
	}
	defer tx.Rollback()
	tags, _ := json.Marshal(filter.Tags)
	args := append(principalArgs(user), sql.Named("query", strings.ToLower(filter.Query)), sql.Named("namespace", filter.Namespace), sql.Named("name", filter.Name), sql.Named("category", filter.Category), sql.Named("tags", string(tags)))
	after, order := "r.namespace||'/'||r.name>@after", "r.namespace,r.name"
	if filter.Order == taxonomy.CatalogOrder_CATALOG_ORDER_RECENT {
		after = "(@after='' OR r.activity_order<CAST(COALESCE(NULLIF(@after,''),'0') AS BIGINT))"
		order = "r.activity_order DESC"
	}
	args = append(args, sql.Named("featured", filter.FeaturedOnly), sql.Named("archived", filter.IncludeArchived))
	query := `WITH visible AS (SELECT r.*,ROW_NUMBER() OVER(PARTITION BY namespace,name ORDER BY version_order DESC,published_order DESC) AS rank,MAX(published_order) OVER(PARTITION BY namespace,name) AS activity_order FROM releases r WHERE status=2 AND (@archived=1 OR ` + activeReleaseSQL + `) AND ` + readableSQL + `)
SELECT r.namespace,r.name,m.description,m.category,r.version,m.display_name_en,m.display_name_zh,m.icon,r.provenance,r.activity_order,EXISTS(SELECT 1 FROM featured_skills f WHERE f.namespace=r.namespace AND f.name=r.name),m.tags,m.revision,(` + strings.Replace(manageableSQL, "(2,3,4)", "(2,4)", 1) + `) FROM visible r JOIN skills m ON m.namespace=r.namespace AND m.name=r.name
WHERE rank=1 AND ` + after + ` AND (@featured=0 OR EXISTS(SELECT 1 FROM featured_skills f WHERE f.namespace=r.namespace AND f.name=r.name)) AND (@namespace='' OR r.namespace=@namespace) AND (@name='' OR r.name=@name) AND (@category='' OR m.category=@category)
AND (@query='' OR instr(lower(r.name||' '||m.description||' '||m.display_name_en||' '||m.display_name_zh),@query)>0)
AND NOT EXISTS(SELECT 1 FROM json_each(@tags) wanted WHERE NOT EXISTS(SELECT 1 FROM json_each(m.tags) t WHERE t.value=wanted.value))
ORDER BY ` + order + ` LIMIT @limit`
	return pageQuery(s, ctx, tx, page, scope(user.ID, "search", filter), query, args, func(rows *sql.Rows) (Skill, string, error) {
		var v Skill
		var published int64
		var tags, origin string
		err := rows.Scan(&v.Namespace, &v.Name, &v.Description, &v.Category, &v.LatestVersion, &v.DisplayNameEN, &v.DisplayNameZH, &v.Icon, &origin, &published, &v.Featured, &tags, &v.Revision, &v.CanEdit)
		if err == nil {
			err = json.Unmarshal([]byte(tags), &v.Tags)
		}
		if err == nil {
			v.Provenance, err = provenance.Decode(origin)
		}
		if filter.Order == taxonomy.CatalogOrder_CATALOG_ORDER_RECENT {
			return v, strconv.FormatInt(published, 10), err
		}
		return v, v.Namespace + "/" + v.Name, err
	})
}

func (s *Store) GetSkill(ctx context.Context, actor Identity, namespace, name string) (Skill, error) {
	result, err := s.SearchSkills(ctx, actor, Search{Namespace: namespace, Name: name, IncludeArchived: true}, Page{Size: 1})
	if err != nil {
		return Skill{}, err
	}
	if len(result.Items) == 0 {
		return Skill{}, ErrNotFound
	}
	return result.Items[0], nil
}

type SubmissionFilter struct {
	ActiveOnly bool
	Mine       bool
	Status     taxonomy.ReleaseStatus
	Query      string
}

func (s *Store) ListReleases(ctx context.Context, actor Identity, namespace, name string, page Page, submissions bool) (PageResult[Release], error) {
	return s.listReleases(ctx, actor, namespace, name, page, submissions, SubmissionFilter{})
}
func (s *Store) ListSubmissions(ctx context.Context, actor Identity, namespace string, page Page, filter SubmissionFilter) (PageResult[Release], error) {
	if filter.Status < 0 || filter.Status > 4 || len(filter.Query) > 3200 {
		return PageResult[Release]{}, ErrInvalid
	}
	return s.listReleases(ctx, actor, namespace, "", page, true, filter)
}
func (s *Store) listReleases(ctx context.Context, actor Identity, namespace, name string, page Page, submissions bool, filter SubmissionFilter) (PageResult[Release], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[Release]{}, err
	}
	defer tx.Rollback()
	visibility := "r.status=2 AND " + readableSQL
	if submissions {
		if user.ID == "" {
			return PageResult[Release]{}, ErrUnauthenticated
		}
		visibility = manageableSQL
	}
	args := append(principalArgs(user), sql.Named("namespace", namespace), sql.Named("name", name), sql.Named("mine", filter.Mine), sql.Named("active", filter.ActiveOnly), sql.Named("status", filter.Status), sql.Named("query", strings.ToLower(filter.Query)))
	columns := strings.Replace(releaseColumns, "r.markdown", "''", 1)
	query := "SELECT " + columns + ` FROM releases r WHERE r.cleaned=0 AND r.superseded=0 AND (@after='' OR r.sequence<CAST(COALESCE(NULLIF(@after,''),'0') AS INTEGER)) AND (@namespace='' OR r.namespace=@namespace) AND (@name='' OR r.name=@name) AND (@mine=0 OR r.submitted_by=@actor) AND (@active=0 OR r.status=1 OR NOT EXISTS(SELECT 1 FROM releases newer WHERE newer.namespace=r.namespace AND newer.name=r.name AND newer.sequence>r.sequence AND newer.cleaned=0 AND newer.superseded=0 AND (@mine=0 OR newer.submitted_by=@actor))) AND (@status=0 OR r.status=@status) AND (@query='' OR instr(lower(r.namespace||'/'||r.name||' '||r.version||' '||r.display_name_en||' '||r.display_name_zh),@query)>0) AND ` + visibility + ` ORDER BY r.sequence DESC LIMIT @limit`
	return pageQuery(s, ctx, tx, page, scope(user.ID, "releases", []any{namespace, name, submissions, filter}), query, args, func(rows *sql.Rows) (Release, string, error) {
		v, err := scanRelease(rows)
		return v, strconv.FormatInt(v.Sequence, 10), err
	})
}

func (s *Store) DownloadRelease(ctx context.Context, actor Identity, namespace, name, version string) (Release, []byte, error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return Release{}, nil, err
	}
	defer tx.Rollback()
	args := append(principalArgs(user), sql.Named("namespace", namespace), sql.Named("name", name), sql.Named("version", version))
	r, err := scanRelease(tx.QueryRowContext(ctx, "SELECT "+releaseColumns+" FROM releases r WHERE namespace=@namespace AND name=@name AND version=@version AND status=2 AND "+readableSQL, args...))
	if err != nil {
		return Release{}, nil, assertExists(err)
	}
	data, err := s.readBlob(r.Digest)
	if err != nil {
		return Release{}, nil, err
	}
	bundle, err := packageformat.Inspect(data)
	if err != nil {
		return Release{}, nil, err
	}
	if err = s.validateSuite(ctx, tx, user, bundle); err != nil {
		return Release{}, nil, err
	}
	return r, data, nil
}

func (s *Store) GetSubmission(ctx context.Context, actor Identity, id string) (Release, []byte, error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return Release{}, nil, err
	}
	defer tx.Rollback()
	r, err := releaseByID(ctx, tx, id)
	if err != nil {
		return Release{}, nil, err
	}
	if err = requireRole(ctx, tx, user, r.Namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER); err != nil {
		return Release{}, nil, err
	}
	data, err := s.readBlob(r.Digest)
	return r, data, err
}

func (s *Store) readBlob(digest string) ([]byte, error) {
	if len(digest) != 71 || !strings.HasPrefix(digest, "sha256:") {
		return nil, errors.New("invalid stored package digest")
	}
	for _, r := range digest[7:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return nil, errors.New("invalid stored package digest")
		}
	}
	path := filepath.Join(s.root, "packages", digest[7:]+".zip")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > packageformat.MaxArchiveBytes {
		return nil, errors.New("invalid stored package")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, packageformat.MaxArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	if packageformat.Digest(data) != digest {
		return nil, errors.New("stored package integrity check failed")
	}
	return data, nil
}

func (s *Store) writeBlob(digest string, data []byte) error {
	dir := filepath.Join(s.root, "packages")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if _, err := s.readBlob(digest); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(dir, ".upload-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, digest[7:]+".zip")); err != nil {
		return err
	}
	return syncPackageDirectory(dir)
}

func jsonText(value any) string { raw, _ := json.Marshal(value); return string(raw) }

// 审核通过后原子更新技能访问规则，所有版本和互动共用该规则。
func publishSkillDetails(ctx context.Context, tx *hubdb.Tx, id string) error {
	var visibility int
	var ns, name, groups string
	err := tx.QueryRowContext(ctx, `SELECT r.namespace,r.name,r.visibility,COALESCE((SELECT json_group_array(group_id) FROM (SELECT group_id FROM release_groups WHERE release_id=r.id ORDER BY group_id) AS ordered_groups),'[]') FROM releases r WHERE r.id=?`, id).Scan(&ns, &name, &visibility, &groups)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE skills SET visibility=?,audience_groups=?,
        display_name_en=(SELECT display_name_en FROM releases WHERE id=?),
        display_name_zh=(SELECT display_name_zh FROM releases WHERE id=?),
        description=(SELECT description FROM releases WHERE id=?),
        icon=(SELECT icon FROM releases WHERE id=?),category=(SELECT category FROM releases WHERE id=?),
        tags=COALESCE((SELECT json_group_array(tag) FROM (SELECT tag FROM release_tags WHERE release_id=? ORDER BY tag) AS ordered_tags),'[]'),
        revision=revision+1 WHERE namespace=? AND name=?`, visibility, groups, id, id, id, id, id, id, ns, name)
	return err
}
