package client_test

import (
	"connectrpc.com/connect"
	"context"
	"errors"
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

func TestWithdrawnInstalledVersionIsNotReportedCurrent(t *testing.T) {
	ctx := context.Background()
	core, inventory := newProjectClient(t)
	server := httptest.NewServer(handler(t))
	defer server.Close()
	if _, e := core.AddSource(ctx, "company", server.URL); e != nil {
		t.Fatal(e)
	}
	if _, e := core.LoginPassword(ctx, "company", "admin", "a-strong-test-password"); e != nil {
		t.Fatal(e)
	}
	remote, e := core.Remote(ctx, "company")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = remote.Admin.PutNamespace(ctx, connect.NewRequest(&adminv1.PutNamespaceRequest{Mutation: projectMutation(), Name: "team"})); e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	if e = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: availability\ndescription: Check installed version availability.\n---\nRead carefully.\n"), 0600); e != nil {
		t.Fatal(e)
	}
	archive, _, e := packageformat.Pack(dir)
	if e != nil {
		t.Fatal(e)
	}
	submit := func(version string) string {
		t.Helper()
		r, e := remote.Publishing.SubmitRelease(ctx, connect.NewRequest(&publishingv1.SubmitReleaseRequest{Mutation: projectMutation(), Namespace: "team", Version: version, Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC, DisplayNameEn: "Availability " + version, DisplayNameZh: "版本可用性", Icon: taxonomy.SkillIcon_SKILL_ICON_DOCUMENT}))
		if e != nil {
			t.Fatal(e)
		}
		return r.Msg.Release.ReleaseId
	}
	id := submit("1.0.0")
	plan, bytes, e := core.PlanRemoteInstall(ctx, client.Coordinate{Source: "company", Namespace: "team", Name: "availability", Version: "1.0.0"}, "codex", "project", t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = inventory.Apply(ctx, *plan, bytes); e != nil {
		t.Fatal(e)
	}
	items, e := inventory.ListInstallations(ctx)
	if e != nil || len(items) != 1 || items[0].DisplayNameEN != "Availability 1.0.0" || items[0].DisplayNameZH != "版本可用性" || items[0].Icon != taxonomy.SkillIcon_SKILL_ICON_DOCUMENT {
		t.Fatal("installation snapshot missing", e)
	}
	check := func(unavailable bool, version string) {
		t.Helper()
		checks, e := core.CheckUpdates(ctx, plan.Target)
		if e != nil || len(checks) != 1 {
			t.Fatalf("check %v %v", checks, e)
		}
		if checks[0].Error != "" || checks[0].VersionUnavailable != unavailable || checks[0].AvailableVersion != version {
			t.Fatalf("incorrect availability %+v", checks[0])
		}
	}
	check(false, "")
	if _, e = remote.Publishing.YankRelease(ctx, connect.NewRequest(&publishingv1.YankReleaseRequest{Mutation: projectMutation(), ReleaseId: id, Reason: "Withdraw test"})); e != nil {
		t.Fatal(e)
	}
	check(true, "")
	if _, _, e = core.PlanRemoteUpdate(ctx, plan.Target, ""); !errors.Is(e, client.ErrInstalledVersionUnavailable) {
		t.Fatalf("withdrawn considered current: %v", e)
	}
	if _, e = os.Stat(filepath.Join(plan.Target, "SKILL.md")); e != nil {
		t.Fatal("local content removed", e)
	}
	submit("1.0.1")
	check(true, "1.0.1")
	update, bytes, e := core.PlanRemoteUpdate(ctx, plan.Target, "")
	if e != nil {
		t.Fatal(e)
	}
	if e = inventory.Apply(ctx, *update, bytes); e != nil {
		t.Fatal(e)
	}
	check(false, "")
	before, e := os.ReadFile(filepath.Join(plan.Target, "SKILL.md"))
	if e != nil {
		t.Fatal(e)
	}
	server.Close()
	offline, e := core.CheckUpdates(ctx, plan.Target)
	if e != nil || len(offline) != 1 || offline[0].Error == "" || offline[0].VersionUnavailable || offline[0].AvailableVersion != "" {
		t.Fatal("network failure misreported as withdrawn/current", offline, e)
	}
	after, e := os.ReadFile(filepath.Join(plan.Target, "SKILL.md"))
	if e != nil || string(before) != string(after) {
		t.Fatal("offline check modified content", e)
	}
}
