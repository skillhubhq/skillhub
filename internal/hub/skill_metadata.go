package hub

import (
	"context"
	"strings"
	"unicode/utf8"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

type SkillMetadataChange struct {
	Namespace        string             `json:"namespace"`
	Name             string             `json:"name"`
	ExpectedRevision uint64             `json:"expected_revision"`
	DisplayNameEN    string             `json:"display_name_en"`
	DisplayNameZH    string             `json:"display_name_zh"`
	Description      string             `json:"description"`
	Icon             taxonomy.SkillIcon `json:"icon"`
	Category         string             `json:"category"`
	Tags             []string           `json:"tags"`
}

func (s *Store) UpdateSkillMetadata(ctx context.Context, actor Identity, key string, cmd SkillMetadataChange) (uint64, error) {
	if !packageformat.ValidName(cmd.Namespace) || !packageformat.ValidName(cmd.Name) || cmd.ExpectedRevision == 0 || !validDisplayName(cmd.DisplayNameEN) || !validDisplayName(cmd.DisplayNameZH) || !utf8.ValidString(cmd.Description) || utf8.RuneCountInString(cmd.Description) > 1024 || strings.TrimSpace(cmd.Description) == "" || strings.ContainsRune(cmd.Description, 0) {
		return 0, ErrInvalid
	}
	if _, ok := taxonomy.SkillIcon_name[int32(cmd.Icon)]; !ok {
		return 0, ErrInvalid
	}
	var err error
	cmd.Tags, err = normalized(cmd.Tags, 20, true)
	if err != nil {
		return 0, err
	}
	return mutate(s, ctx, actor, "skill.metadata", key, cmd, func(tx *hubdb.Tx, u Identity) error {
		return requireRole(ctx, tx, u, cmd.Namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER)
	}, func(tx *hubdb.Tx, u Identity) (uint64, error) {
		if cmd.Category != "" {
			var exists bool
			if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM categories WHERE name=? AND archived=0)", cmd.Category).Scan(&exists); err != nil {
				return 0, err
			}
			if !exists {
				return 0, ErrInvalid
			}
		}
		result, err := tx.ExecContext(ctx, "UPDATE skills SET display_name_en=?,display_name_zh=?,description=?,icon=?,category=?,tags=?,revision=revision+1 WHERE namespace=? AND name=? AND revision=?", strings.TrimSpace(cmd.DisplayNameEN), strings.TrimSpace(cmd.DisplayNameZH), cmd.Description, cmd.Icon, cmd.Category, jsonText(cmd.Tags), cmd.Namespace, cmd.Name, cmd.ExpectedRevision)
		if err != nil {
			return 0, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		if n != 1 {
			return 0, ErrConflict
		}
		return cmd.ExpectedRevision + 1, s.auditChange(ctx, tx, u.ID, "skill.metadata", cmd.Namespace+"/"+cmd.Name, cmd)
	})
}
