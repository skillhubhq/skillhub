package client_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/skillsuite"
)

func TestSuiteInstallUpdateOwnershipAndSharedRemoval(t *testing.T) {
	ctx := context.Background()
	core, store := newProjectClient(t)
	server := httptest.NewServer(handler(t))
	defer server.Close()
	if _, e := core.AddSource(ctx, "test", server.URL); e != nil {
		t.Fatal(e)
	}
	if _, e := core.LoginPassword(ctx, "test", "admin", "a-strong-test-password"); e != nil {
		t.Fatal(e)
	}
	remote, e := core.Remote(ctx, "test")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = remote.Admin.PutNamespace(ctx, connect.NewRequest(&adminv1.PutNamespaceRequest{Mutation: projectMutation(), Name: "team"})); e != nil {
		t.Fatal(e)
	}
	member := func(name, version string) skillsuite.Member {
		t.Helper()
		dir := t.TempDir()
		if e := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: Test a delivery workflow\n---\n"+version), 0600); e != nil {
			t.Fatal(e)
		}
		raw, _, e := packageformat.Pack(dir)
		if e != nil {
			t.Fatal(e)
		}
		r, e := remote.Publishing.SubmitRelease(ctx, connect.NewRequest(&publishingv1.SubmitReleaseRequest{Mutation: projectMutation(), Namespace: "team", Version: version, Archive: raw, Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}))
		if e != nil {
			t.Fatal(e)
		}
		return skillsuite.Member{Namespace: "team", Name: name, Version: version, Digest: r.Msg.Release.Digest}
	}
	one := member("one", "1.0.0")
	two := member("two", "1.0.0")
	publish := func(name, version string, members ...skillsuite.Member) client.Coordinate {
		t.Helper()
		_, e := core.PublishSuite(ctx, client.PublishSuite{Source: "test", Namespace: "team", Name: name, Version: version, Description: "A test suite", Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS, Members: members, Entry: members[0].Namespace + "/" + members[0].Name, RequestID: projectMutation().IdempotencyKey})
		if e != nil {
			t.Fatal(e)
		}
		return client.Coordinate{Source: "test", Namespace: "team", Name: name, Version: version}
	}
	suite := publish("starter", "1.0.0", one, two)
	project := t.TempDir()
	install := func(c client.Coordinate) *client.SuiteInstallPlan {
		t.Helper()
		p, e := core.PlanSuiteInstall(ctx, c, "cursor", "project", project)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = core.ApplySuite(ctx, p); e != nil {
			t.Fatal(e)
		}
		return p
	}
	p := install(suite)
	if len(p.Plans) != 3 {
		t.Fatal("incomplete suite", p)
	}
	items, e := store.ListInstallations(ctx)
	if e != nil || len(items) != 3 {
		t.Fatal("install inventory", e)
	}
	// 套装更新固定成员版本，并移除已不再声明的自有成员。
	oneUpdated := member("one", "1.1.0")
	updated := publish("starter", "1.1.0", oneUpdated)
	install(updated)
	items, e = store.ListInstallations(ctx)
	if e != nil || len(items) != 2 {
		t.Fatal("suite update did not remove obsolete member", items, e)
	}
	for _, item := range items {
		if item.Name == "one" && item.Version != "1.1.0" {
			t.Fatal("suite member version did not update", item)
		}
	}
	// 恢复原来的组合后继续验证共享成员和本地修改保留。
	install(suite)
	second := publish("other", "1.0.0", one)
	install(second)
	var firstTarget, secondTarget, oneTarget, twoTarget string
	items, _ = store.ListInstallations(ctx)
	for _, i := range items {
		switch i.Name {
		case "starter":
			firstTarget = i.Target
		case "other":
			secondTarget = i.Target
		case "one":
			oneTarget = i.Target
		case "two":
			twoTarget = i.Target
		}
	}
	// 用户修改文件，移除套装时保留它；共享成员由最后一个套装移除。
	if e = os.WriteFile(filepath.Join(twoTarget, "notes.txt"), []byte("personal notes"), 0600); e != nil {
		t.Fatal(e)
	}
	removal, e := core.PlanSuiteRemove(ctx, firstTarget)
	if e != nil {
		t.Fatal(e)
	}
	if len(removal.Retained) != 2 {
		t.Fatal("lost retained members", removal)
	}
	if _, e = core.ApplySuite(ctx, removal); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(oneTarget); e != nil {
		t.Fatal("removed shared member")
	}
	if _, e = os.Stat(twoTarget); e != nil {
		t.Fatal("removed local edits")
	}
	removal, e = core.PlanSuiteRemove(ctx, secondTarget)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = core.ApplySuite(ctx, removal); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(oneTarget); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("last owner did not remove member", e)
	}
	// 独立安装同版本的成员在套装移除后继续保留。
	p = install(second)
	single, raw, e := core.PlanRemoteInstall(ctx, client.Coordinate{Source: "test", Namespace: "team", Name: "one", Version: "1.0.0"}, "cursor", "project", project)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Apply(ctx, *single, raw); e != nil {
		t.Fatal(e)
	}
	removal, e = core.PlanSuiteRemove(ctx, secondTarget)
	if e != nil {
		t.Fatal(e)
	}
	if len(removal.Retained) != 1 {
		t.Fatal("independent install not retained")
	}
	if _, e = core.ApplySuite(ctx, removal); e != nil {
		t.Fatal(e)
	}
	if _, e = core.ApplySuite(ctx, p); !errors.Is(e, client.ErrStalePlan) {
		t.Fatal("stale plan applied", e)
	}
	// 独立版本不能被套装悄悄覆盖。
	one2 := member("one", "2.0.0")
	next := publish("other", "2.0.0", one2)
	if _, e = core.PlanSuiteInstall(ctx, next, "cursor", "project", project); e == nil {
		t.Fatal("overwrote independent version")
	}
}
func TestSuiteRejectsUnboundedMemberCount(t *testing.T) {
	ctx := context.Background()
	core, _ := newProjectClient(t)
	server := httptest.NewServer(handler(t))
	defer server.Close()
	_, e := core.AddSource(ctx, "test", server.URL)
	if e != nil {
		t.Fatal(e)
	}
	_, e = core.LoginPassword(ctx, "test", "admin", "a-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = core.PublishSuite(ctx, client.PublishSuite{Source: "test", Members: make([]skillsuite.Member, 51)}); e == nil {
		t.Fatal("unbounded suite")
	}
}
