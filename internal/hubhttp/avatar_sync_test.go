package hubhttp

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/google/uuid"
	community "github.com/skillhubhq/skillhub/internal/community"
	"github.com/skillhubhq/skillhub/internal/hubpolicy"
	"github.com/skillhubhq/skillhub/internal/identityprovider"
)

type delayedAvatar struct {
	verifiedAdapter
	entered chan struct{}
	release chan struct{}
	data    []byte
}

func (a *delayedAvatar) Avatar(ctx context.Context, _ identityprovider.Identity) ([]byte, error) {
	a.entered <- struct{}{}
	select {
	case <-a.release:
		return a.data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func TestAvatarSyncDoesNotBlockAndPreservesEdits(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial", true: "edited"}[edit], func(t *testing.T) {
			store, err := testDatabase(t, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			s, err := New(store, Config{PublicURL: "http://localhost", Policy: hubpolicy.Policy{Mode: hubpolicy.Community, Limits: hubpolicy.Defaults()}})
			if err != nil {
				t.Fatal(err)
			}
			defer s.CloseBackground()
			session, err := store.FederatedLogin(context.Background(), "https://identity.example", "person", "Person", true)
			if err != nil {
				t.Fatal(err)
			}
			var data bytes.Buffer
			if err = png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
				t.Fatal(err)
			}
			a := &delayedAvatar{entered: make(chan struct{}, 1), release: make(chan struct{}), data: data.Bytes()}
			returned := make(chan struct{})
			go func() { s.startAvatarSync(a, identityprovider.Identity{}, session.Identity); close(returned) }()
			select {
			case <-returned:
			case <-time.After(2 * time.Second):
				close(a.release)
				t.Fatal("avatar blocked sign-in")
			}
			select {
			case <-a.entered:
			case <-time.After(2 * time.Second):
				t.Fatal("avatar did not start")
			}
			if edit {
				_, err = store.UpdateProfile(context.Background(), session.Identity, uuid.NewString(), community.ProfileChange{DisplayName: "Chosen name", ExpectedRevision: 0})
				if err != nil {
					t.Fatal(err)
				}
			}
			s.startAvatarSync(a, identityprovider.Identity{}, session.Identity)
			close(a.release)
			s.avatarWG.Wait()
			p, err := store.GetProfile(context.Background(), session.Identity, session.Identity.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !edit && p.Avatar == "" {
				t.Fatal("initial avatar not saved")
			}
			if edit && (p.Avatar != "" || p.DisplayName != "Chosen name") {
				t.Fatal("background sync overwrote edited profile")
			}
			if len(a.entered) != 0 {
				t.Fatal("duplicate concurrent avatar fetch")
			}
		})
	}
}
func TestAvatarSyncBoundedAndCancelledOnShutdown(t *testing.T) {
	store, err := testDatabase(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := New(store, Config{PublicURL: "http://localhost", Policy: hubpolicy.Policy{Mode: hubpolicy.Community, Limits: hubpolicy.Defaults()}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.CloseBackground()
	a := &delayedAvatar{entered: make(chan struct{}, 8), release: make(chan struct{})}
	for i := 0; i < 6; i++ {
		session, err := store.FederatedLogin(context.Background(), "https://identity.example", uuid.NewString(), "Person", true)
		if err != nil {
			t.Fatal(err)
		}
		s.startAvatarSync(a, identityprovider.Identity{}, session.Identity)
	}
	for i := 0; i < 4; i++ {
		select {
		case <-a.entered:
		case <-time.After(2 * time.Second):
			t.Fatal("bounded worker did not start")
		}
	}
	s.avatarMu.Lock()
	count := len(s.avatarTasks)
	s.avatarMu.Unlock()
	if count != 4 {
		t.Fatal("unbounded avatar tasks", count)
	}
	done := make(chan struct{})
	go func() { s.CloseBackground(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not cancel avatar fetches")
	}
	if len(a.entered) != 0 {
		t.Fatal("exceeded avatar capacity")
	}
}
