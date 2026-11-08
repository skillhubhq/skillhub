package hubhttp

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	publishing "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
)

func TestReleaseCleanupTransportGuards(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	_, ordinary := f.user(t, "ordinary")
	submitted, e := f.publishing.SubmitRelease(ctx, authenticated(&publishing.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "cleanup-check", "# Content"), Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC}, f.owner))
	if e != nil {
		t.Fatal(e)
	}
	id := submitted.Msg.Release.ReleaseId
	request := &publishing.DeleteReleaseRequest{Mutation: mutation(), ReleaseId: id, ConfirmCoordinate: "team/cleanup-check@1.0.0"}
	if _, e = f.publishing.DeleteRelease(ctx, authenticated(request, ordinary)); connect.CodeOf(e) != connect.CodePermissionDenied {
		t.Fatal("unauthorized delete", e)
	}
	if _, e = f.publishing.DeleteRelease(ctx, authenticated(request, f.owner)); connect.CodeOf(e) != connect.CodeAlreadyExists {
		t.Fatal("published delete", e)
	}
	_, e = f.publishing.YankRelease(ctx, authenticated(&publishing.YankReleaseRequest{Mutation: mutation(), ReleaseId: id, Reason: "Obsolete"}, f.owner))
	if e != nil {
		t.Fatal(e)
	}
	wrong := &publishing.DeleteReleaseRequest{Mutation: mutation(), ReleaseId: id, ConfirmCoordinate: "team/cleanup-check@2.0.0"}
	if _, e = f.publishing.DeleteRelease(ctx, authenticated(wrong, f.owner)); connect.CodeOf(e) != connect.CodeInvalidArgument {
		t.Fatal("wrong confirmation", e)
	}
	for i := 0; i < 2; i++ {
		result, e := f.publishing.DeleteRelease(ctx, authenticated(request, f.owner))
		if e != nil || result.Msg.ReleaseId != id {
			t.Fatal("delete or replay", e)
		}
	}
	if _, e = f.publishing.GetSubmission(ctx, authenticated(&publishing.GetSubmissionRequest{ReleaseId: id}, f.owner)); connect.CodeOf(e) != connect.CodeNotFound {
		t.Fatal("deleted content still readable", e)
	}
}
