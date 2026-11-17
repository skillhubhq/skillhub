package hubhttp

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubpolicy"
)

func TestMemberSubmissionAPI(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	if err := f.store.ConfigurePolicy(hubpolicy.Policy{Mode: hubpolicy.Enterprise, Submission: hubpolicy.SubmissionMembers}); err != nil {
		t.Fatal(err)
	}
	_, token := f.user(t, "contributor")
	access, err := f.admin.GetNamespaceAccess(ctx, authenticated(&adminv1.GetNamespaceAccessRequest{Namespace: "team"}, token))
	if err != nil {
		t.Fatal(err)
	}
	if !access.Msg.CanPublish || access.Msg.CanMaintain || access.Msg.CanManage || access.Msg.CanReview {
		t.Fatalf("incorrect contributor capabilities: %+v", access.Msg)
	}
	result, err := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "member-check", "Submission contract"), Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}, token))
	if err != nil {
		t.Fatal(err)
	}
	r := result.Msg.Release
	if r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING {
		t.Fatal("member bypassed approval")
	}
	detail, err := f.publishing.GetSubmission(ctx, authenticated(&publishingv1.GetSubmissionRequest{ReleaseId: r.ReleaseId}, token))
	if err != nil || len(detail.Msg.Archive) == 0 {
		t.Fatalf("own submission unavailable: %v", err)
	}
	if _, err = f.publishing.ReviewRelease(ctx, authenticated(&publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: r.ReleaseId, Approve: true}, token)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatal("self review allowed", err)
	}
	_, err = f.publishing.ReviewRelease(ctx, authenticated(&publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: r.ReleaseId, Approve: true}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	_, other := f.user(t, "another-member")
	download, err := f.catalog.DownloadRelease(ctx, authenticated(&catalogv1.DownloadReleaseRequest{Namespace: r.Namespace, Name: r.Name, Version: r.Version}, other))
	if err != nil || len(download.Msg.Archive) == 0 {
		t.Fatalf("approved contribution unavailable: %v", err)
	}
}
