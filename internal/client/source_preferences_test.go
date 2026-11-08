package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/skillhubhq/skillhub/internal/client"
)

func TestSourcePreferencesPreserveIdentityAndWorkOffline(t *testing.T) {
	ctx := context.Background()
	h := handler(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); h.ServeHTTP(w, r) }))
	defer server.Close()
	store, err := client.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	creds := &credentialStore{values: map[string]string{}}
	core := client.NewClient(store, client.ClientOptions{Credentials: creds})
	if _, err = core.AddSource(ctx, "work", server.URL); err != nil {
		t.Fatal(err)
	}
	if _, err = core.LoginPassword(ctx, "work", "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	before, _ := core.Source(ctx, "work")
	n := requests.Load()
	after, err := core.UpdateSourcePreferences(ctx, "work", "  Team knowledge  ", "Review account")
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != n || after.Label != "Team knowledge" || after.Note != "Review account" {
		t.Fatal("preferences must be local and normalized")
	}
	check := after
	check.Label, check.Note = before.Label, before.Note
	if check != before {
		t.Fatal("editing preferences changed connection identity or credentials")
	}
	remote, err := core.Remote(ctx, "work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = remote.Auth.GetIdentity(ctx, connect.NewRequest(&authv1.GetIdentityRequest{})); err != nil {
		t.Fatal("saved credential lost", err)
	}
	refreshed, err := core.RefreshSource(ctx, "work")
	if err != nil || refreshed.Label != after.Label || refreshed.Note != after.Note || refreshed.CredentialRef != before.CredentialRef {
		t.Fatal("refresh lost local preferences", err)
	}
	server.Close()
	if _, err = core.UpdateSourcePreferences(ctx, "work", "Offline name", ""); err != nil {
		t.Fatal("offline edit failed", err)
	}
	for _, bad := range []string{strings.Repeat("a", 81), "hidden\nline", "name\u202e"} {
		if _, err = core.UpdateSourcePreferences(ctx, "work", bad, ""); err == nil {
			t.Fatal("invalid label accepted")
		}
	}
	if _, err = core.UpdateSourcePreferences(ctx, "missing", "name", ""); !errors.Is(err, client.ErrSourceNotFound) {
		t.Fatal(err)
	}
	current, _ := core.Source(ctx, "work")
	if current.Label != "Offline name" {
		t.Fatal("failed edit changed saved preferences")
	}
}

func TestRefreshSourceRejectsReplacementWithoutSendingCredentials(t *testing.T) {
	ctx := context.Background()
	first, second := handler(t), handler(t)
	var replaced atomic.Bool
	var leaked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if replaced.Load() {
			if r.Header.Get("Authorization") != "" {
				leaked.Store(true)
			}
			second.ServeHTTP(w, r)
		} else {
			first.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	store, err := client.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	core := client.NewClient(store, client.ClientOptions{Credentials: &credentialStore{values: map[string]string{}}})
	if _, err = core.AddSource(ctx, "work", server.URL); err != nil {
		t.Fatal(err)
	}
	if _, err = core.LoginPassword(ctx, "work", "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	before, _ := core.Source(ctx, "work")
	replaced.Store(true)
	if _, err = core.RefreshSource(ctx, "work"); !errors.Is(err, client.ErrRegistryChanged) {
		t.Fatal(err)
	}
	after, _ := core.Source(ctx, "work")
	if after != before || leaked.Load() {
		t.Fatal("refresh trusted replacement or transmitted credentials")
	}
}
