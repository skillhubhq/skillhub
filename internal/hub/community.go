package hub

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	c "github.com/skillhubhq/skillhub/internal/community"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/google/uuid"
)

const profileColumns = `u.id,u.username,COALESCE(NULLIF(p.display_name,''),u.username),COALESCE(p.bio,''),COALESCE(p.revision,0),COALESCE(p.avatar,'')`

func profileScan(row scanner) (c.Profile, error) {
	var p c.Profile
	err := row.Scan(&p.UserID, &p.Username, &p.DisplayName, &p.Bio, &p.Revision, &p.Avatar)
	return p, assertExists(err)
}
func profileAt(ctx context.Context, tx *hubdb.Tx, id string) (c.Profile, error) {
	return profileScan(tx.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM users u LEFT JOIN profiles p ON p.user_id=u.id WHERE u.id=?`, id))
}
func validProfile(name, bio string) bool {
	return utf8.ValidString(name) && utf8.ValidString(bio) && utf8.RuneCountInString(name) <= 80 && utf8.RuneCountInString(bio) <= 600 && strings.TrimSpace(name) != "" && !strings.ContainsAny(name, "\r\n\x00") && !strings.ContainsRune(bio, 0)
}

// 登录账号名只向本人或 Hub 管理员返回，不作为对外展示名的兜底。
func profileForViewer(p c.Profile, user Identity) c.Profile {
	if user.ID != p.UserID && !user.Administrator {
		if p.DisplayName == p.Username {
			p.DisplayName = "Member"
		}
		p.Username = ""
	}
	return p
}

// 资料发现沿用资源可见性；同团队成员可以互相查看，但登录不等于取得全站目录。
const sharedTeamProfileSQL = `(@actor<>'' AND EXISTS(SELECT 1 FROM namespace_grants target WHERE target.role IN (1,2,3,4) AND ((target.kind=1 AND target.subject_id=@subject) OR (target.kind=2 AND EXISTS(SELECT 1 FROM group_members tm WHERE tm.group_id=target.subject_id AND tm.user_id=@subject))) AND EXISTS(SELECT 1 FROM namespace_grants viewer WHERE viewer.namespace=target.namespace AND viewer.role IN (1,2,3,4) AND ((viewer.kind=1 AND viewer.subject_id=@actor) OR (viewer.kind=2 AND EXISTS(SELECT 1 FROM group_members vm WHERE vm.group_id=viewer.subject_id AND vm.user_id=@actor))))))`

func (s *Store) GetProfile(ctx context.Context, actor Identity, id string) (c.Profile, error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return c.Profile{}, err
	}
	defer tx.Rollback()
	if id == "" {
		id = user.ID
	}
	if id == "" {
		return c.Profile{}, ErrUnauthenticated
	}
	if user.ID != id && !user.Administrator {
		var visible bool
		args := append(principalArgs(user), sql.Named("subject", id))
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM releases r WHERE r.status=2 AND `+readableSQL+` AND (r.submitted_by=@subject OR EXISTS(SELECT 1 FROM comments c WHERE c.release_id=r.id AND c.author_id=@subject AND c.deleted=0))) OR `+sharedTeamProfileSQL, args...).Scan(&visible)
		if err != nil {
			return c.Profile{}, err
		}
		if !visible {
			return c.Profile{}, ErrNotFound
		}
	}
	p, err := profileAt(ctx, tx, id)
	p.CanEdit = id == user.ID
	return profileForViewer(p, user), err
}
func (s *Store) UpdateProfile(ctx context.Context, actor Identity, key string, cmd c.ProfileChange) (c.Profile, error) {
	cmd.DisplayName = strings.TrimSpace(cmd.DisplayName)
	cmd.Bio = strings.TrimSpace(cmd.Bio)
	if !validProfile(cmd.DisplayName, cmd.Bio) {
		return c.Profile{}, ErrInvalid
	}
	return mutate(s, ctx, actor, "community.profile", key, cmd, authenticatedOnly, func(tx *hubdb.Tx, user Identity) (c.Profile, error) {
		p, err := profileAt(ctx, tx, user.ID)
		if err != nil {
			return p, err
		}
		if p.Revision != cmd.ExpectedRevision {
			return p, ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO profiles(user_id,display_name,bio,revision) VALUES(?,?,?,1) ON CONFLICT(user_id) DO UPDATE SET display_name=excluded.display_name,bio=excluded.bio,revision=profiles.revision+1`, user.ID, cmd.DisplayName, cmd.Bio)
		if err != nil {
			return p, err
		}
		p, err = profileAt(ctx, tx, user.ID)
		p.CanEdit = true
		return p, err
	})
}

// 团队公开页只由可见的已发布资源或当前成员资格发现，权限群组不成为公开团队。
const teamVisibleSQL = `(@admin=1 OR EXISTS(SELECT 1 FROM releases r WHERE r.namespace=n.name AND r.status=2 AND ` + readableSQL + `) OR (@actor<>'' AND EXISTS(SELECT 1 FROM namespace_grants g WHERE g.namespace=n.name AND ((g.kind=1 AND g.subject_id=@actor) OR (g.kind=2 AND EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=g.subject_id AND gm.user_id=@actor))))))`
const teamColumns = `n.name,COALESCE(NULLIF(t.display_name,''),n.name),COALESCE(t.bio,''),COALESCE(t.revision,0)`

func teamScan(row scanner) (c.Team, error) {
	var v c.Team
	err := row.Scan(&v.Namespace, &v.DisplayName, &v.Bio, &v.Revision)
	return v, assertExists(err)
}
func teamCapabilities(ctx context.Context, tx *hubdb.Tx, user Identity, v *c.Team) {
	v.CanEdit = requireRole(ctx, tx, user, v.Namespace) == nil
	v.CanViewMembers = requireRole(ctx, tx, user, v.Namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_READER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER) == nil
}
func (s *Store) GetTeam(ctx context.Context, actor Identity, namespace string) (c.Team, error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return c.Team{}, err
	}
	defer tx.Rollback()
	args := append(principalArgs(user), sql.Named("namespace", namespace))
	v, err := teamScan(tx.QueryRowContext(ctx, `SELECT `+teamColumns+` FROM namespaces n LEFT JOIN team_profiles t ON t.namespace=n.name WHERE n.name=@namespace AND `+teamVisibleSQL, args...))
	if err == nil {
		teamCapabilities(ctx, tx, user, &v)
	}
	return v, err
}
func (s *Store) ListTeams(ctx context.Context, actor Identity, page Page) (PageResult[c.Team], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[c.Team]{}, err
	}
	defer tx.Rollback()
	result, err := pageQuery(s, ctx, tx, page, scope(user.ID, "community.teams", nil), `SELECT `+teamColumns+` FROM namespaces n LEFT JOIN team_profiles t ON t.namespace=n.name WHERE n.name>@after AND `+teamVisibleSQL+` ORDER BY n.name LIMIT @limit`, principalArgs(user), func(rows *sql.Rows) (c.Team, string, error) { v, e := teamScan(rows); return v, v.Namespace, e })
	if err == nil {
		for i := range result.Items {
			teamCapabilities(ctx, tx, user, &result.Items[i])
		}
	}
	return result, err
}
func (s *Store) UpdateTeam(ctx context.Context, actor Identity, key string, cmd c.TeamChange) (c.Team, error) {
	cmd.DisplayName = strings.TrimSpace(cmd.DisplayName)
	cmd.Bio = strings.TrimSpace(cmd.Bio)
	if !validProfile(cmd.DisplayName, cmd.Bio) {
		return c.Team{}, ErrInvalid
	}
	return mutate(s, ctx, actor, "community.team", key, cmd, func(tx *hubdb.Tx, user Identity) error { return requireRole(ctx, tx, user, cmd.Namespace) }, func(tx *hubdb.Tx, user Identity) (c.Team, error) {
		v, err := teamScan(tx.QueryRowContext(ctx, `SELECT `+teamColumns+` FROM namespaces n LEFT JOIN team_profiles t ON t.namespace=n.name WHERE n.name=?`, cmd.Namespace))
		if err != nil {
			return v, err
		}
		if v.Revision != cmd.ExpectedRevision {
			return v, ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO team_profiles(namespace,display_name,bio,revision) VALUES(?,?,?,1) ON CONFLICT(namespace) DO UPDATE SET display_name=excluded.display_name,bio=excluded.bio,revision=team_profiles.revision+1`, cmd.Namespace, cmd.DisplayName, cmd.Bio)
		return c.Team{Namespace: cmd.Namespace, DisplayName: cmd.DisplayName, Bio: cmd.Bio, Revision: v.Revision + 1, CanEdit: true, CanViewMembers: true}, err
	})
}
func (s *Store) ListMembers(ctx context.Context, actor Identity, namespace string, page Page) (PageResult[c.Member], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[c.Member]{}, err
	}
	defer tx.Rollback()
	if err = requireRole(ctx, tx, user, namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_READER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER); err != nil {
		return PageResult[c.Member]{}, err
	}
	query := `SELECT ` + profileColumns + `,(SELECT json_group_array(DISTINCT g.role) FROM namespace_grants g WHERE g.namespace=@namespace AND ((g.kind=1 AND g.subject_id=u.id) OR (g.kind=2 AND EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=g.subject_id AND gm.user_id=u.id)))) FROM users u LEFT JOIN profiles p ON p.user_id=u.id WHERE u.disabled=0 AND u.id>@after AND EXISTS(SELECT 1 FROM namespace_grants g WHERE g.namespace=@namespace AND ((g.kind=1 AND g.subject_id=u.id) OR (g.kind=2 AND EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=g.subject_id AND gm.user_id=u.id)))) ORDER BY u.id LIMIT @limit`
	return pageQuery(s, ctx, tx, page, scope(user.ID, "community.members", namespace), query, []any{sql.Named("namespace", namespace)}, func(rows *sql.Rows) (c.Member, string, error) {
		var v c.Member
		var roles string
		e := rows.Scan(&v.Profile.UserID, &v.Profile.Username, &v.Profile.DisplayName, &v.Profile.Bio, &v.Profile.Revision, &v.Profile.Avatar, &roles)
		if e == nil {
			e = json.Unmarshal([]byte(roles), &v.Roles)
		}
		v.Profile = profileForViewer(v.Profile, user)
		return v, v.Profile.UserID, e
	})
}
func (s *Store) ListContributions(ctx context.Context, actor Identity, id, namespace string, page Page, bookmarks bool) (PageResult[c.Contribution], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[c.Contribution]{}, err
	}
	defer tx.Rollback()
	if bookmarks && user.ID == "" {
		return PageResult[c.Contribution]{}, ErrUnauthenticated
	}
	query := `SELECT r.id,r.namespace,r.name,r.version,r.description,r.display_name_en,r.display_name_zh,r.icon,` + profileColumns + ` FROM releases r JOIN users u ON u.id=r.submitted_by LEFT JOIN profiles p ON p.user_id=u.id WHERE r.status=2 AND ` + readableSQL + ` AND r.id>@after AND (@subject='' OR r.submitted_by=@subject) AND (@namespace='' OR r.namespace=@namespace)`
	if bookmarks {
		query += ` AND EXISTS(SELECT 1 FROM reactions x WHERE x.release_id=r.id AND x.user_id=@actor AND x.bookmark=1)`
	}
	query += ` ORDER BY r.id LIMIT @limit`
	return pageQuery(s, ctx, tx, page, scope(user.ID, "community.contributions", []any{id, namespace, bookmarks}), query, append(principalArgs(user), sql.Named("subject", id), sql.Named("namespace", namespace)), func(rows *sql.Rows) (c.Contribution, string, error) {
		var v c.Contribution
		e := rows.Scan(&v.ReleaseID, &v.Namespace, &v.Name, &v.Version, &v.Description, &v.DisplayNameEN, &v.DisplayNameZH, &v.Icon, &v.Author.UserID, &v.Author.Username, &v.Author.DisplayName, &v.Author.Bio, &v.Author.Revision, &v.Author.Avatar)
		v.Author = profileForViewer(v.Author, user)
		return v, v.ReleaseID, e
	})
}

// 每个互动请求重新确认精确版本已发布且受众可见，撤权与撤下对重放同样有效。
func communityRelease(ctx context.Context, tx *hubdb.Tx, user Identity, id string) (string, error) {
	var namespace string
	args := append(principalArgs(user), sql.Named("release", id))
	err := tx.QueryRowContext(ctx, `SELECT r.namespace FROM releases r WHERE r.id=@release AND r.status=2 AND `+readableSQL, args...).Scan(&namespace)
	return namespace, assertExists(err)
}
func discussionAt(ctx context.Context, tx *hubdb.Tx, user Identity, id string) (c.Discussion, error) {
	var d c.Discussion
	d.CanInteract = user.ID != ""
	err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM reactions WHERE release_id=? AND bookmark=0),(SELECT count(*) FROM comments WHERE release_id=? AND deleted=0),EXISTS(SELECT 1 FROM reactions WHERE release_id=? AND user_id=? AND bookmark=0),EXISTS(SELECT 1 FROM reactions WHERE release_id=? AND user_id=? AND bookmark=1)`, id, id, id, user.ID, id, user.ID).Scan(&d.LikeCount, &d.CommentCount, &d.Liked, &d.Bookmarked)
	return d, err
}
func (s *Store) GetDiscussion(ctx context.Context, actor Identity, id string) (c.Discussion, error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return c.Discussion{}, err
	}
	defer tx.Rollback()
	if _, err = communityRelease(ctx, tx, user, id); err != nil {
		return c.Discussion{}, err
	}
	return discussionAt(ctx, tx, user, id)
}
func (s *Store) SetReaction(ctx context.Context, actor Identity, key string, cmd c.ReactionChange) (c.Discussion, error) {
	return mutate(s, ctx, actor, "community.reaction", key, cmd, func(tx *hubdb.Tx, user Identity) error {
		_, err := communityRelease(ctx, tx, user, cmd.ReleaseID)
		return err
	}, func(tx *hubdb.Tx, user Identity) (c.Discussion, error) {
		var err error
		if cmd.Active {
			_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO reactions(user_id,release_id,bookmark) VALUES(?,?,?)`, user.ID, cmd.ReleaseID, cmd.Bookmark)
		} else {
			_, err = tx.ExecContext(ctx, `DELETE FROM reactions WHERE user_id=? AND release_id=? AND bookmark=?`, user.ID, cmd.ReleaseID, cmd.Bookmark)
		}
		if err != nil {
			return c.Discussion{}, err
		}
		return discussionAt(ctx, tx, user, cmd.ReleaseID)
	})
}

const commentColumns = `c.id,c.release_id,COALESCE(c.parent_id,''),CASE WHEN c.deleted=1 THEN '' ELSE c.body END,c.created_at,c.deleted,` + profileColumns

func commentScan(row scanner) (c.Comment, error) {
	var v c.Comment
	var at int64
	err := row.Scan(&v.CommentID, &v.ReleaseID, &v.ParentID, &v.Body, &at, &v.Deleted, &v.Author.UserID, &v.Author.Username, &v.Author.DisplayName, &v.Author.Bio, &v.Author.Revision, &v.Author.Avatar)
	v.CreatedAt = time.Unix(at, 0)
	return v, assertExists(err)
}
func (s *Store) ListComments(ctx context.Context, actor Identity, id, parent string, page Page) (PageResult[c.Comment], error) {
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return PageResult[c.Comment]{}, err
	}
	defer tx.Rollback()
	namespace, err := communityRelease(ctx, tx, user, id)
	if err != nil {
		return PageResult[c.Comment]{}, err
	}
	if parent != "" {
		var valid bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM comments WHERE id=? AND release_id=? AND parent_id IS NULL)`, parent, id).Scan(&valid)
		if err != nil {
			return PageResult[c.Comment]{}, err
		}
		if !valid {
			return PageResult[c.Comment]{}, ErrNotFound
		}
	}
	moderate := requireRole(ctx, tx, user, namespace) == nil
	return pageQuery(s, ctx, tx, page, scope(user.ID, "community.comments", []string{id, parent}), `SELECT `+commentColumns+`,c.sequence FROM comments c JOIN users u ON u.id=c.author_id LEFT JOIN profiles p ON p.user_id=u.id WHERE c.release_id=@release AND COALESCE(c.parent_id,'')=@parent AND c.sequence>CAST(COALESCE(NULLIF(@after,''),'0') AS INTEGER) ORDER BY c.sequence LIMIT @limit`, []any{sql.Named("release", id), sql.Named("parent", parent)}, func(rows *sql.Rows) (c.Comment, string, error) {
		var v c.Comment
		var at, seq int64
		e := rows.Scan(&v.CommentID, &v.ReleaseID, &v.ParentID, &v.Body, &at, &v.Deleted, &v.Author.UserID, &v.Author.Username, &v.Author.DisplayName, &v.Author.Bio, &v.Author.Revision, &v.Author.Avatar, &seq)
		v.CreatedAt = time.Unix(at, 0)
		v.CanDelete = !v.Deleted && (moderate || user.ID == v.Author.UserID)
		v.Author = profileForViewer(v.Author, user)
		return v, fmt.Sprint(seq), e
	})
}
func (s *Store) CreateComment(ctx context.Context, actor Identity, key string, cmd c.CommentChange) (c.Comment, error) {
	cmd.Body = strings.TrimSpace(cmd.Body)
	if cmd.Body == "" || !utf8.ValidString(cmd.Body) || utf8.RuneCountInString(cmd.Body) > 4000 || strings.ContainsRune(cmd.Body, 0) {
		return c.Comment{}, ErrInvalid
	}
	result, err := mutate(s, ctx, actor, "community.comment", key, cmd, func(tx *hubdb.Tx, user Identity) error {
		_, err := communityRelease(ctx, tx, user, cmd.ReleaseID)
		return err
	}, func(tx *hubdb.Tx, user Identity) (c.Comment, error) {
		var recent int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM comments WHERE author_id=? AND created_at>?`, user.ID, s.now().Add(-time.Minute).Unix()).Scan(&recent); err != nil {
			return c.Comment{}, err
		}
		if recent >= 20 {
			return c.Comment{}, ErrRateLimited
		}
		var parent any
		if cmd.ParentID != "" {
			var valid bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM comments WHERE id=? AND release_id=? AND parent_id IS NULL AND deleted=0)`, cmd.ParentID, cmd.ReleaseID).Scan(&valid); err != nil {
				return c.Comment{}, err
			}
			if !valid {
				return c.Comment{}, ErrInvalid
			}
			parent = cmd.ParentID
		}
		id := uuid.NewString()
		at := s.now()
		_, err := tx.ExecContext(ctx, `INSERT INTO comments(id,release_id,parent_id,author_id,body,created_at) VALUES(?,?,?,?,?,?)`, id, cmd.ReleaseID, parent, user.ID, cmd.Body, at.Unix())
		if err != nil {
			return c.Comment{}, err
		}
		p, err := profileAt(ctx, tx, user.ID)
		return c.Comment{CommentID: id, ReleaseID: cmd.ReleaseID, ParentID: cmd.ParentID, Author: p, Body: cmd.Body, CreatedAt: at, CanDelete: true}, err
	})
	if err != nil {
		return c.Comment{}, err
	}
	// 幂等重放也读取当前记录，删除后的正文不能从旧响应缓存重新出现。
	tx, user, err := s.read(ctx, actor)
	if err != nil {
		return c.Comment{}, err
	}
	defer tx.Rollback()
	namespace, err := communityRelease(ctx, tx, user, cmd.ReleaseID)
	if err != nil {
		return c.Comment{}, err
	}
	current, err := commentScan(tx.QueryRowContext(ctx, `SELECT `+commentColumns+` FROM comments c JOIN users u ON u.id=c.author_id LEFT JOIN profiles p ON p.user_id=u.id WHERE c.id=?`, result.CommentID))
	current.CanDelete = !current.Deleted && (current.Author.UserID == user.ID || requireRole(ctx, tx, user, namespace) == nil)
	return current, err
}
func (s *Store) DeleteComment(ctx context.Context, actor Identity, key, id string) error {
	_, err := mutate(s, ctx, actor, "community.comment.delete", key, id, func(tx *hubdb.Tx, user Identity) error {
		var release, author string
		err := tx.QueryRowContext(ctx, `SELECT release_id,author_id FROM comments WHERE id=?`, id).Scan(&release, &author)
		if err != nil {
			return assertExists(err)
		}
		namespace, err := communityRelease(ctx, tx, user, release)
		if err != nil {
			return err
		}
		if author == user.ID {
			return nil
		}
		return requireRole(ctx, tx, user, namespace)
	}, func(tx *hubdb.Tx, user Identity) (struct{}, error) {
		_, err := tx.ExecContext(ctx, `UPDATE comments SET deleted=1,body='' WHERE id=?`, id)
		if err == nil {
			err = audit(ctx, tx, user.ID, "community.comment.delete", id, s.now())
		}
		return struct{}{}, err
	})
	return err
}
