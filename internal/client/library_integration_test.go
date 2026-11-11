package client_test

import (
	"connectrpc.com/connect"
	"context"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalAssociationMatchesContentRechecksAccessAndRetainsLocalChanges(t *testing.T) {
	ctx := context.Background()
	_, s := newProjectClient(t)
	core := client.NewClient(s, client.ClientOptions{Home: t.TempDir(), Credentials: &credentialStore{values: map[string]string{}}})
	server := httptest.NewServer(handler(t))
	defer server.Close()
	if _, e := core.AddSource(ctx, "test", server.URL); e != nil {
		t.Fatal(e)
	}
	if _, e := core.LoginPassword(ctx, "test", "admin", "a-strong-test-password"); e != nil {
		t.Fatal(e)
	}
	r, e := core.Remote(ctx, "test")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Admin.PutNamespace(ctx, connect.NewRequest(&adminv1.PutNamespaceRequest{Mutation: projectMutation(), Name: "team"})); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(t.TempDir(), "check")
	if e = os.Mkdir(target, 0700); e != nil {
		t.Fatal(e)
	}
	original := []byte("---\nname: check\ndescription: Local association test\n---\nOriginal\n")
	if e = os.WriteFile(filepath.Join(target, "SKILL.md"), original, 0600); e != nil {
		t.Fatal(e)
	}
	raw, _, e := packageformat.Pack(target)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Publishing.SubmitRelease(ctx, connect.NewRequest(&publishingv1.SubmitReleaseRequest{Mutation: projectMutation(), Namespace: "team", Version: "1.0.0", Archive: raw, Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS})); e != nil {
		t.Fatal(e)
	}
	coord := client.Coordinate{Source: "test", Namespace: "team", Name: "check", Version: "1.0.0"}
	p, e := core.PlanLocalSource(ctx, target, coord, false)
	if e != nil {
		t.Fatal(e)
	}
	if p.After == nil || len(p.Differences) != 0 {
		t.Fatal("exact match not registered", p)
	}
	if e = core.ApplyLocalPlan(ctx, p); e != nil {
		t.Fatal(e)
	}
	inv, e := core.LocalInventory(ctx)
	if e != nil || len(inv.Skills) != 1 || inv.Skills[0].State != "matched" {
		t.Fatal("associated folder absent", inv, e)
	}
	if e = os.WriteFile(filepath.Join(target, "SKILL.md"), append(original, []byte("Local edit\n")...), 0600); e != nil {
		t.Fatal(e)
	}
	p, e = core.PlanLocalSource(ctx, target, coord, false)
	if e != nil {
		t.Fatal(e)
	}
	if p.After != nil || len(p.Differences) != 1 {
		t.Fatal("local differences assigned false version", p)
	}
	if e = core.ApplyLocalPlan(ctx, p); e != nil {
		t.Fatal(e)
	}
	inv, e = core.LocalInventory(ctx)
	if e != nil || inv.Skills[0].State != "modified" || inv.Skills[0].Installation != nil {
		t.Fatal("unverified association promoted", inv, e)
	}
	p, e = core.PlanLocalSource(ctx, target, coord, true)
	if e != nil {
		t.Fatal(e)
	}
	if e = core.Logout(ctx, "test"); e != nil {
		t.Fatal(e)
	}
	if e = core.ApplyLocalPlan(ctx, p); e == nil {
		t.Fatal("revoked access permitted replacement")
	}
	current, _ := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if string(current) == string(original) {
		t.Fatal("failed replacement changed files")
	}
	if _, e = core.LoginPassword(ctx, "test", "admin", "a-strong-test-password"); e != nil {
		t.Fatal(e)
	}
	p, e = core.PlanLocalSource(ctx, target, coord, true)
	if e != nil {
		t.Fatal(e)
	}
	if e = core.ApplyLocalPlan(ctx, p); e != nil {
		t.Fatal(e)
	}
	backups, e := s.LocalBackups(ctx)
	if e != nil || len(backups) != 1 {
		t.Fatal("replacement lost backup", backups, e)
	}
	restore, e := core.PlanLocalRestore(ctx, backups[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = core.ApplyLocalPlan(ctx, restore); e != nil {
		t.Fatal(e)
	}
	restored, _ := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if string(restored) != string(current) {
		t.Fatal("restore lost local edits")
	}
}
