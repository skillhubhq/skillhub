package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/skillhubhq/skillhub/gen/api/auth/v1/authv1connect"
	"github.com/skillhubhq/skillhub/internal/client"
)

type refreshTransport struct {
	base    http.RoundTripper
	drop    atomic.Bool
	offline atomic.Bool
	calls   atomic.Int32
}

func (r *refreshTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasSuffix(req.URL.Path, "/RefreshSession") {
		r.calls.Add(1)
		if r.offline.Load() {
			return nil, errors.New("test offline")
		}
		result, err := r.base.RoundTrip(req)
		if err == nil && r.drop.Swap(false) {
			io.Copy(io.Discard, result.Body)
			result.Body.Close()
			return nil, io.ErrUnexpectedEOF
		}
		return result, err
	}
	return r.base.RoundTrip(req)
}
func expireSavedAccess(t *testing.T, c *credentialStore, ref string) map[string]any {
	t.Helper()
	raw, err := c.Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if !strings.HasPrefix(raw, "skillhub-session-v1:") || json.Unmarshal([]byte(strings.TrimPrefix(raw, "skillhub-session-v1:")), &v) != nil {
		t.Fatal("renewable credential not saved")
	}
	v["expires"] = time.Now().Add(-time.Minute).Unix()
	b, _ := json.Marshal(v)
	if err = c.Set(ref, "skillhub-session-v1:"+string(b)); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestRenewalSharedClientsOfflineAndLostResponse(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(handler(t))
	defer server.Close()
	root := t.TempDir()
	store, err := client.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	creds := &credentialStore{values: map[string]string{}}
	transport := &refreshTransport{base: http.DefaultTransport}
	core := client.NewClient(store, client.ClientOptions{Credentials: creds, HTTPClient: &http.Client{Transport: transport}})
	if _, err = core.AddSource(ctx, "one", server.URL); err != nil {
		t.Fatal(err)
	}
	identity, err := core.LoginPassword(ctx, "one", "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	source, err := core.Source(ctx, "one")
	if err != nil {
		t.Fatal(err)
	}
	initial := expireSavedAccess(t, creds, source.CredentialRef)
	transport.offline.Store(true)
	remote, err := core.Remote(ctx, "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = remote.Auth.GetIdentity(ctx, connect.NewRequest(&authv1.GetIdentityRequest{})); err == nil {
		t.Fatal("offline refresh succeeded")
	}
	raw, err := creds.Get(source.CredentialRef)
	if err != nil || !strings.Contains(raw, `"pending":`) {
		t.Fatal("offline refresh lost recovery marker", err)
	}
	transport.offline.Store(false)
	transport.drop.Store(true)
	transport.calls.Store(0)
	otherStore, err := client.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer otherStore.Close()
	second := client.NewClient(otherStore, client.ClientOptions{Credentials: creds, HTTPClient: &http.Client{Transport: transport}})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := core
			if i%2 == 0 {
				c = second
			}
			r, e := c.Remote(ctx, "one")
			if e != nil {
				t.Error(e)
				return
			}
			v, e := r.Auth.GetIdentity(ctx, connect.NewRequest(&authv1.GetIdentityRequest{}))
			if e != nil || v.Msg.GetIdentity().GetUserId() != identity.UserId {
				t.Error("concurrent recovery failed", e)
			}
		}(i)
	}
	wg.Wait()
	if transport.calls.Load() != 2 {
		t.Fatalf("refresh calls %d, wanted rotation plus recovery", transport.calls.Load())
	}
	raw, err = creds.Get(source.CredentialRef)
	if err != nil || strings.Contains(raw, `"pending":`) {
		t.Fatal("pending refresh not completed", err)
	}
	for _, file := range []string{"client.db", "client.db-wal"} {
		b, _ := os.ReadFile(filepath.Join(root, file))
		if strings.Contains(string(b), initial["refresh"].(string)) {
			t.Fatal("refresh secret leaked to local metadata")
		}
	}
	if _, err = core.AddSource(ctx, "two", server.URL); err != nil {
		t.Fatal(err)
	}
	if _, err = core.LoginPassword(ctx, "two", "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	otherSource, _ := core.Source(ctx, "two")
	otherRaw, _ := creds.Get(otherSource.CredentialRef)
	saved := expireSavedAccess(t, creds, source.CredentialRef)
	if err = core.Logout(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	otherAfter, _ := creds.Get(otherSource.CredentialRef)
	if otherAfter != otherRaw {
		t.Fatal("logout changed other connection")
	}
	auth := authv1connect.NewAuthServiceClient(server.Client(), server.URL)
	if _, err = auth.RefreshSession(ctx, connect.NewRequest(&authv1.RefreshSessionRequest{RefreshToken: saved["refresh"].(string), RequestId: uuid.NewString()})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("logout left refresh active", err)
	}
}

type interruptedCredentials struct {
	*credentialStore
	fail atomic.Bool
}

func (c *interruptedCredentials) Set(ref, value string) error {
	if strings.HasPrefix(value, "skillhub-session-v1:") && !strings.Contains(value, `"pending":`) && c.fail.Swap(false) {
		return client.ErrCredentialStore
	}
	return c.credentialStore.Set(ref, value)
}
func TestRenewalRecoversCredentialWriteFailureAfterRestart(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(handler(t))
	defer server.Close()
	root := t.TempDir()
	store, err := client.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	creds := &interruptedCredentials{credentialStore: &credentialStore{values: map[string]string{}}}
	core := client.NewClient(store, client.ClientOptions{Credentials: creds})
	if _, err = core.AddSource(ctx, "hub", server.URL); err != nil {
		t.Fatal(err)
	}
	user, err := core.LoginPassword(ctx, "hub", "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	source, _ := core.Source(ctx, "hub")
	expireSavedAccess(t, creds.credentialStore, source.CredentialRef)
	creds.fail.Store(true)
	remote, err := core.Remote(ctx, "hub")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = remote.Auth.GetIdentity(ctx, connect.NewRequest(&authv1.GetIdentityRequest{})); !errors.Is(err, client.ErrCredentialStore) {
		t.Fatal("write failure not reported", err)
	}
	raw, _ := creds.Get(source.CredentialRef)
	if !strings.Contains(raw, `"pending":`) {
		t.Fatal("refresh recovery marker lost")
	}
	store.Close()
	store, err = client.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restarted := client.NewClient(store, client.ClientOptions{Credentials: creds})
	remote, err = restarted.Remote(ctx, "hub")
	if err != nil {
		t.Fatal(err)
	}
	result, err := remote.Auth.GetIdentity(ctx, connect.NewRequest(&authv1.GetIdentityRequest{}))
	if err != nil || result.Msg.GetIdentity().GetUserId() != user.UserId {
		t.Fatal("session recovery after restart failed", err)
	}
}
