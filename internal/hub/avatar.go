package hub

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"

	c "github.com/skillhubhq/skillhub/internal/community"
	"github.com/skillhubhq/skillhub/internal/hubdb"
)

// 头像只接收有界栅格图，裁切后重新编码以移除元数据和原始附加内容。
func normalizeAvatar(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	if len(raw) > 512*1024 {
		return "", ErrInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "png" && format != "jpeg") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 {
		return "", ErrInvalid
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", ErrInvalid
	}
	const size = 128
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	side := min(cfg.Width, cfg.Height)
	x0, y0 := src.Bounds().Min.X+(cfg.Width-side)/2, src.Bounds().Min.Y+(cfg.Height-side)/2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dst.SetNRGBA(x, y, color.NRGBAModel.Convert(src.At(x0+x*side/size, y0+y*side/size)).(color.NRGBA))
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(out.Bytes()), nil
}
func (s *Store) UpdateAvatar(ctx context.Context, actor Identity, key string, cmd c.AvatarChange) (c.Profile, error) {
	if actor.ID == "" {
		return c.Profile{}, ErrUnauthenticated
	}
	avatar, err := normalizeAvatar(cmd.Image)
	if err != nil {
		return c.Profile{}, err
	}
	// 幂等记录只保存规范化图片，避免重复持久化未经处理的大体积输入。
	change := struct {
		Avatar           string
		ExpectedRevision uint64
	}{avatar, cmd.ExpectedRevision}
	return mutate(s, ctx, actor, "community.avatar", key, change, authenticatedOnly, func(tx *hubdb.Tx, user Identity) (c.Profile, error) {
		p, err := profileAt(ctx, tx, user.ID)
		if err != nil {
			return p, err
		}
		if p.Revision != cmd.ExpectedRevision {
			return p, ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO profiles(user_id,avatar,revision) VALUES(?,?,1) ON CONFLICT(user_id) DO UPDATE SET avatar=excluded.avatar,revision=profiles.revision+1`, user.ID, avatar)
		if err != nil {
			return p, err
		}
		p, err = profileAt(ctx, tx, user.ID)
		p.CanEdit = true
		return p, err
	})
}
