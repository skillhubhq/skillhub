package hubhttp

import (
	"bytes"
	"connectrpc.com/connect"
	"context"
	api "github.com/skillhubhq/skillhub/gen/api/community/v1"
	"github.com/skillhubhq/skillhub/gen/api/community/v1/communityv1connect"
	"image"
	"image/png"
	"testing"
)

func TestAvatarRPCIdentityRevisionAndFormats(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	id, token := f.user(t, "photo-owner")
	_, other := f.user(t, "other-person")
	var raw bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 320, 320))
	// 难以压缩的图片同时覆盖超过旧 64 KiB 请求上限的真实上传。
	var state uint32 = 23
	for i := range img.Pix {
		state = 1664525*state + 1013904223
		img.Pix[i] = byte(state >> 24)
	}
	if err := png.Encode(&raw, img); err != nil {
		t.Fatal(err)
	}
	if raw.Len() <= 65536 || raw.Len() > 524288 {
		t.Fatal("fixture size", raw.Len())
	}
	revision := uint64(0)
	for _, options := range [][]connect.ClientOption{nil, {connect.WithProtoJSON()}} {
		client := communityv1connect.NewCommunityServiceClient(f.server.Client(), f.server.URL, options...)
		if _, err := client.UpdateAvatar(ctx, connect.NewRequest(&api.UpdateAvatarRequest{Mutation: mutation(), Image: raw.Bytes()})); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatal("anonymous photo accepted", err)
		}
		req := &api.UpdateAvatarRequest{Mutation: mutation(), Image: raw.Bytes(), ExpectedRevision: revision}
		updated, err := client.UpdateAvatar(ctx, authenticated(req, token))
		if err != nil || updated.Msg.Profile.UserId != id || updated.Msg.Profile.Avatar == "" {
			t.Fatal("photo upload", err)
		}
		replay, err := client.UpdateAvatar(ctx, authenticated(req, token))
		if err != nil || replay.Msg.Profile.Revision != updated.Msg.Profile.Revision {
			t.Fatal("replay", err)
		}
		revision = updated.Msg.Profile.Revision
		untouched, err := client.GetProfile(ctx, authenticated(&api.GetProfileRequest{}, other))
		if err != nil || untouched.Msg.Profile.Avatar != "" {
			t.Fatal("cross-identity photo write", err)
		}
		if _, err = client.UpdateAvatar(ctx, authenticated(&api.UpdateAvatarRequest{Mutation: mutation(), Image: []byte("<svg/>")}, token)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatal("unsafe format", err)
		}
	}
}
