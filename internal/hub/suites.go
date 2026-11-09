package hub

import (
	"context"
	"database/sql"

	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/skillsuite"
)

// 套装不授予成员权限；在投稿、审核和下载三个边界检查所有固定成员。
func (s *Store) validateSuite(ctx context.Context, tx *hubdb.Tx, u Identity, b *packageformat.Bundle) error {
	m, err := skillsuite.FromBundle(b)
	if err != nil {
		return ErrInvalid
	}
	if m == nil {
		return nil
	}
	for _, v := range m.Members {
		if v.Name == b.Metadata.Name {
			return ErrInvalid
		}
		args := append(principalArgs(u), sql.Named("ns", v.Namespace), sql.Named("name", v.Name), sql.Named("version", v.Version))
		r, e := scanRelease(tx.QueryRowContext(ctx, "SELECT "+releaseColumns+" FROM releases r WHERE r.namespace=@ns AND r.name=@name AND r.version=@version AND r.status=2 AND r.cleaned=0 AND "+readableSQL, args...))
		if e != nil {
			return assertExists(e)
		}
		if r.Digest != v.Digest {
			return ErrConflict
		}
		raw, e := s.readBlob(r.Digest)
		if e != nil {
			return e
		}
		member, e := packageformat.Inspect(raw)
		if e != nil {
			return e
		}
		nested, e := skillsuite.FromBundle(member)
		if e != nil || nested != nil {
			return ErrInvalid
		}
	}
	return nil
}
