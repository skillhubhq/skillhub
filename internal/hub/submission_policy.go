package hub

import (
	"context"
	"database/sql"
	"errors"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
)

// 全员投稿仅开放共享团队；个人空间仍由各自的授权控制。
func (s *Store) requireSubmissionNamespace(ctx context.Context, tx *hubdb.Tx, u Identity, namespace string) error {
	err := requireRole(ctx, tx, u, namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER)
	if !errors.Is(err, ErrForbidden) || !s.policy.MemberSubmissions() {
		return err
	}
	var personal bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM personal_namespaces WHERE namespace=?)", namespace).Scan(&personal); err != nil {
		return err
	}
	if personal {
		return ErrForbidden
	}
	return nil
}

// 投稿者可以创建技能并更新自己最初提交的技能，不接管他人的坐标。
func (s *Store) requireSkillSubmission(ctx context.Context, tx *hubdb.Tx, u Identity, namespace, name string) error {
	err := requireRole(ctx, tx, u, namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER)
	if err == nil {
		return nil
	}
	if err = s.requireSubmissionNamespace(ctx, tx, u, namespace); err != nil {
		return err
	}
	var owner string
	err = tx.QueryRowContext(ctx, "SELECT submitted_by FROM releases WHERE namespace=? AND name=? ORDER BY sequence LIMIT 1", namespace, name).Scan(&owner)
	if err == nil {
		if owner == u.ID {
			return nil
		}
		return ErrForbidden
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM skills WHERE namespace=? AND name=?)", namespace, name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrForbidden
	}
	return nil
}

// 投稿权限只允许操作本人提交，不能扩大到团队内其他作者的版本。
func (s *Store) requireReleaseAuthor(ctx context.Context, tx *hubdb.Tx, u Identity, r Release, roles ...taxonomy.NamespaceRole) error {
	err := requireRole(ctx, tx, u, r.Namespace, roles...)
	if !errors.Is(err, ErrForbidden) || !s.policy.MemberSubmissions() || r.SubmittedBy != u.ID {
		return err
	}
	return s.requireSubmissionNamespace(ctx, tx, u, r.Namespace)
}

// 仅用于提交详情、扫描、评测和反馈读取；不用于审核或团队管理授权。
func (s *Store) submissionReadableSQL() string {
	if !s.policy.MemberSubmissions() {
		return manageableSQL
	}
	return "(" + manageableSQL + " OR (" + activeReleaseSQL + " AND @actor<>'' AND r.submitted_by=@actor AND NOT EXISTS(SELECT 1 FROM personal_namespaces p WHERE p.namespace=r.namespace)))"
}
