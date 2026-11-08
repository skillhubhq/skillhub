package client_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
)

func TestPublicationDraftAccountAndHubIsolation(t *testing.T) {
	ctx := context.Background()
	core, _ := newProjectClient(t)
	first, second := httptest.NewServer(handler(t)), httptest.NewServer(handler(t))
	defer first.Close()
	defer second.Close()
	for name, address := range map[string]string{"first": first.URL, "second": second.URL} {
		if _, e := core.AddSource(ctx, name, address); e != nil {
			t.Fatal(e)
		}
	}
	owner, e := core.LoginPassword(ctx, "first", "admin", "a-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	original, e := core.Source(ctx, "first")
	if e != nil {
		t.Fatal(e)
	}
	payload := `{"namespace":"team","version":"1.1.0","archive":"fixture"}`
	if e = core.SavePublicationDraft(ctx, "first", owner.UserId, payload); e != nil {
		t.Fatal(e)
	}
	remote, e := core.Remote(ctx, "first")
	if e != nil {
		t.Fatal(e)
	}
	_, e = remote.Admin.CreateUser(ctx, connect.NewRequest(&adminv1.CreateUserRequest{Mutation: projectMutation(), Username: "another", Password: "another-strong-test-password"}))
	if e != nil {
		t.Fatal(e)
	}
	other, e := core.LoginPassword(ctx, "first", "another", "another-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	switched, e := core.Source(ctx, "first")
	if e != nil {
		t.Fatal(e)
	}
	if original.CredentialRef == switched.CredentialRef {
		t.Fatal("account switch reused installation identity")
	}
	if _, e = core.LoadPublicationDraft(ctx, "first", owner.UserId); e == nil {
		t.Fatal("other account read draft")
	}
	if v, e := core.LoadPublicationDraft(ctx, "first", other.UserId); e != nil || v != "" {
		t.Fatal("draft crossed accounts", e)
	}
	secondOwner, e := core.LoginPassword(ctx, "second", "admin", "a-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	if v, e := core.LoadPublicationDraft(ctx, "second", secondOwner.UserId); e != nil || v != "" {
		t.Fatal("draft crossed hubs", e)
	}
	if _, e = core.LoginPassword(ctx, "first", "admin", "a-strong-test-password"); e != nil {
		t.Fatal(e)
	}
	if v, e := core.LoadPublicationDraft(ctx, "first", owner.UserId); e != nil || v != payload {
		t.Fatal("draft not restored", e)
	}
	before, e := core.Source(ctx, "first")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = core.LoginPassword(ctx, "first", "admin", "a-strong-test-password"); e != nil {
		t.Fatal(e)
	}
	after, e := core.Source(ctx, "first")
	if e != nil {
		t.Fatal(e)
	}
	if before.CredentialRef != after.CredentialRef {
		t.Fatal("same account sign-in broke installations")
	}
	for _, bad := range []string{"not-json", strings.Repeat(" ", 32<<20+1)} {
		if e = core.SavePublicationDraft(ctx, "first", owner.UserId, bad); e == nil {
			t.Fatal("accepted invalid draft")
		}
	}
	if e = core.SavePublicationDraft(ctx, "first", owner.UserId, ""); e != nil {
		t.Fatal(e)
	}
	if v, e := core.LoadPublicationDraft(ctx, "first", owner.UserId); e != nil || v != "" {
		t.Fatal("draft clear failed", e)
	}
}
