package hubhttp

import (
	"connectrpc.com/connect"
	"context"
	admin "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	catalog "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	publishing "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"testing"
)

func TestNamespaceLifecycleTransportGuards(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	_, ordinary := f.user(t, "ordinary")
	release, e := f.publishing.SubmitRelease(ctx, authenticated(&publishing.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "lifecycle-check", "# Content"), Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC}, f.owner))
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.admin.GetNamespaceLifecycle(ctx, authenticated(&admin.GetNamespaceLifecycleRequest{Name: "team"}, ordinary))
	if connect.CodeOf(e) != connect.CodePermissionDenied {
		t.Fatal(e)
	}
	archived, e := f.admin.SetNamespaceArchived(ctx, authenticated(&admin.SetNamespaceArchivedRequest{Mutation: mutation(), Name: "team", ExpectedRevision: 1, Archived: true}, f.owner))
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.catalog.DownloadRelease(ctx, authenticated(&catalog.DownloadReleaseRequest{Namespace: "team", Name: release.Msg.Release.Name, Version: "1.0.0"}, f.owner))
	if e != nil {
		t.Fatal("archive blocked authorized download", e)
	}
	_, e = f.admin.PurgeNamespace(ctx, authenticated(&admin.PurgeNamespaceRequest{Mutation: mutation(), Name: "team", ExpectedRevision: archived.Msg.Lifecycle.Revision, ConfirmName: "other"}, f.owner))
	if connect.CodeOf(e) != connect.CodeInvalidArgument {
		t.Fatal("wrong confirmation", e)
	}
	request := &admin.PurgeNamespaceRequest{Mutation: mutation(), Name: "team", ExpectedRevision: archived.Msg.Lifecycle.Revision, ConfirmName: "team"}
	for i := 0; i < 2; i++ {
		result, e := f.admin.PurgeNamespace(ctx, authenticated(request, f.owner))
		if e != nil || result.Msg.Lifecycle.Releases != 1 {
			t.Fatal("purge/replay", e)
		}
	}
	_, e = f.admin.GetNamespaceLifecycle(ctx, authenticated(&admin.GetNamespaceLifecycleRequest{Name: "team"}, f.owner))
	if connect.CodeOf(e) != connect.CodeNotFound {
		t.Fatal(e)
	}
}
