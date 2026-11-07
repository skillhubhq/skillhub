package hub

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	c "github.com/skillhubhq/skillhub/internal/community"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
)

func avatarImage(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x % 256), G: uint8(y % 256), B: 200, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestAvatarNormalizationRevisionIsolationAndBackup(t *testing.T) {
	s, session, _, _ := backupFixture(t)
	ctx := context.Background()
	actor := session.Identity
	raw := append(avatarImage(t, 300, 170), []byte("private metadata")...)
	p, err := s.UpdateAvatar(ctx, actor, "avatar-set", c.AvatarChange{Image: raw})
	if err != nil {
		t.Fatal(err)
	}
	body, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(p.Avatar, "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(body))
	if err != nil || cfg.Width != 128 || cfg.Height != 128 || bytes.Contains(body, []byte("private metadata")) {
		t.Fatal("avatar not normalized", err)
	}
	same, err := s.UpdateAvatar(ctx, actor, "avatar-set", c.AvatarChange{Image: raw})
	if err != nil || same.Revision != p.Revision {
		t.Fatal("idempotency", err)
	}
	if _, err = s.UpdateAvatar(ctx, actor, "avatar-stale", c.AvatarChange{}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale update accepted", err)
	}
	if _, err = s.UpdateAvatar(ctx, Identity{}, "avatar-anonymous", c.AvatarChange{Image: raw}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("anonymous upload", err)
	}
	p, err = s.UpdateProfile(ctx, actor, "profile-after-avatar", c.ProfileChange{DisplayName: "Photo author", ExpectedRevision: p.Revision})
	if err != nil || p.Avatar == "" {
		t.Fatal("profile update dropped photo", err)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if _, err = s.Backup(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "restored")
	if _, err = RestoreBackup(ctx, snapshot, root); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	read, err := restored.GetProfile(ctx, actor, "")
	if err != nil || read.Avatar != p.Avatar {
		t.Fatal("avatar not restored", err)
	}
	cleared, err := s.UpdateAvatar(ctx, actor, "avatar-clear", c.AvatarChange{ExpectedRevision: p.Revision})
	if err != nil || cleared.Avatar != "" {
		t.Fatal("remove avatar", err)
	}
}
func TestAvatarRejectsUnsupportedOversizedAndCorruptImages(t *testing.T) {
	for name, raw := range map[string][]byte{"svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "oversized": make([]byte, 524289), "corrupt": []byte("not an image"), "dimensions": avatarImage(t, 2049, 1)} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeAvatar(raw); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid image accepted", err)
			}
		})
	}
}
