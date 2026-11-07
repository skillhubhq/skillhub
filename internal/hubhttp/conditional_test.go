package hubhttp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	"github.com/skillhubhq/skillhub/gen/api/catalog/v1/catalogv1connect"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
)

func TestConditionalDownloadsRecheckAudienceStatusAndBlobIntegrity(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	memberID, member := f.user(t, "member")
	group := f.group(t, "audience")
	f.member(t, group, memberID, true)
	archive := skillArchive(t, "review-check", "Cache must not grant access.")
	submitted, err := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_GROUPS, GroupIds: []string{group}}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	release := submitted.Msg.Release
	for _, jsonMode := range []bool{false, true} {
		options := []connect.ClientOption{}
		if jsonMode {
			options = append(options, connect.WithProtoJSON())
		}
		catalog := catalogv1connect.NewCatalogServiceClient(f.server.Client(), f.server.URL, options...)
		for _, known := range []string{"", release.Digest, "sha256:" + strings.Repeat("0", 64)} {
			response, err := catalog.DownloadRelease(ctx, authenticated(&catalogv1.DownloadReleaseRequest{Namespace: "team", Name: "review-check", Version: "1.0.0", KnownDigest: known}, member))
			if err != nil {
				t.Fatal(err)
			}
			matched := known == release.Digest
			if response.Msg.NotModified != matched || response.Msg.Digest != release.Digest || response.Msg.Release.ReleaseId != release.ReleaseId {
				t.Fatal("conditional download returned inconsistent metadata")
			}
			if (matched && len(response.Msg.Archive) != 0) || (!matched && !bytes.Equal(response.Msg.Archive, archive)) {
				t.Fatal("conditional download returned wrong package body")
			}
		}
	}
	request := &catalogv1.DownloadReleaseRequest{Namespace: "team", Name: "review-check", Version: "1.0.0", KnownDigest: release.Digest}
	f.member(t, group, memberID, false)
	if _, err = f.catalog.DownloadRelease(ctx, authenticated(request, member)); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("known digest bypassed revoked membership: %v", err)
	}
	f.member(t, group, memberID, true)
	blob := filepath.Join(f.root, "packages", release.Digest[7:]+".zip")
	if err = os.WriteFile(blob, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.catalog.DownloadRelease(ctx, authenticated(request, member)); err == nil {
		t.Fatal("conditional download concealed corrupted server package")
	}
	if err = os.WriteFile(blob, archive, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.publishing.YankRelease(ctx, authenticated(&publishingv1.YankReleaseRequest{Mutation: mutation(), ReleaseId: release.ReleaseId, Reason: "withdraw"}, f.owner)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.catalog.DownloadRelease(ctx, authenticated(request, f.owner)); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatal("known digest bypassed withdrawal")
	}
	request.KnownDigest = "invalid"
	if _, err = f.catalog.DownloadRelease(ctx, authenticated(request, member)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatal("invalid digest passed protocol validation")
	}
}
