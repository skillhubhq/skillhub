package client_test

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

func TestCuratedInstallRetainsAttributionOffline(t *testing.T) {
	ctx := context.Background()
	core, store := newProjectClient(t)
	server := httptest.NewServer(handler(t))
	defer server.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	_, e := core.AddSource(ctx, "hub", server.URL)
	must(e)
	_, e = core.LoginPassword(ctx, "hub", "admin", "a-strong-test-password")
	must(e)
	admin, e := core.Remote(ctx, "hub")
	must(e)
	must(admin.RequireProvenance())
	_, e = admin.Admin.PutNamespace(ctx, connect.NewRequest(&adminv1.PutNamespaceRequest{Mutation: projectMutation(), Name: "curators", ReviewRequired: true}))
	must(e)
	user, e := admin.Admin.CreateUser(ctx, connect.NewRequest(&adminv1.CreateUserRequest{Mutation: projectMutation(), Username: "curator", Password: "curator-test-password"}))
	must(e)
	_, e = admin.Admin.SetNamespaceGrant(ctx, connect.NewRequest(&adminv1.SetNamespaceGrantRequest{Mutation: projectMutation(), Namespace: "curators", SubjectKind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectId: user.Msg.User.UserId, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, Granted: true}))
	must(e)
	curator, _ := newProjectClient(t)
	_, e = curator.AddSource(ctx, "hub", server.URL)
	must(e)
	_, e = curator.LoginPassword(ctx, "hub", "curator", "curator-test-password")
	must(e)
	remote, e := curator.Remote(ctx, "hub")
	must(e)
	dir := t.TempDir()
	body := []byte("---\nname: curated-example\ndescription: Curated integration fixture\n---\nOriginal content\n")
	must(os.WriteFile(filepath.Join(dir, "SKILL.md"), body, 0600))
	must(os.WriteFile(filepath.Join(dir, "LICENSE.txt"), []byte("Example license notice"), 0600))
	archive, _, e := packageformat.Pack(dir)
	must(e)
	p := &catalogv1.Provenance{AuthorName: "Example Authors", AuthorUrl: "https://example.org/authors", SourceUrl: "https://example.org/source/" + strings.Repeat("b", 40), Revision: strings.Repeat("b", 40), License: "MIT", LicenseFile: "LICENSE.txt"}
	r, e := remote.Publishing.SubmitRelease(ctx, connect.NewRequest(&publishingv1.SubmitReleaseRequest{Mutation: projectMutation(), Namespace: "curators", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC, Provenance: p}))
	must(e)
	if r.Msg.Release.Provenance.AuthorName != p.AuthorName || r.Msg.Release.SubmittedBy != user.Msg.User.UserId {
		t.Fatal("wire lost author/operator separation")
	}
	_, e = admin.Publishing.ReviewRelease(ctx, connect.NewRequest(&publishingv1.ReviewReleaseRequest{Mutation: projectMutation(), ReleaseId: r.Msg.Release.ReleaseId, Approve: true, Reason: "Reviewed original source and license"}))
	must(e)
	coord := client.Coordinate{Source: "hub", Namespace: "curators", Name: "curated-example", Version: "1.0.0"}
	preview, e := core.PreviewRelease(ctx, coord)
	must(e)
	if preview.Release.Provenance.AuthorName != p.AuthorName {
		t.Fatal("preview lost attribution")
	}
	plan, data, e := core.PlanRemoteInstall(ctx, coord, "codex", "project", t.TempDir())
	must(e)
	must(store.Apply(ctx, *plan, data))
	server.Close()
	items, e := store.ListInstallations(ctx)
	must(e)
	if len(items) != 1 || items[0].Provenance == nil || items[0].Provenance.Revision != p.Revision || items[0].Provenance.License != "MIT" {
		t.Fatal("offline attribution missing")
	}
	installed, e := os.ReadFile(filepath.Join(items[0].Target, "SKILL.md"))
	must(e)
	if string(installed) != string(body) {
		t.Fatal("installation rewrote upstream file")
	}
	if (&client.Remote{}).RequireProvenance() == nil {
		t.Fatal("old hub silently accepted attribution")
	}
}
