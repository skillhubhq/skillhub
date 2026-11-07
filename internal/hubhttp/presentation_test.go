package hubhttp

import (
	"bytes"
	"connectrpc.com/connect"
	"context"
	catalog "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	publishing "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"strings"
	"testing"
)

func TestSkillPresentationIsReviewedSearchableAndVersioned(t *testing.T) {
	f := registry(t, true)
	ctx := context.Background()
	authorID, author := f.user(t, "author")
	f.grant(t, authorID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, true)
	archive := skillArchive(t, "experiment-readout", "# Read results")
	cmd := &publishing.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC, DisplayNameEn: "Experiment Readout", DisplayNameZh: "实验结果解读", Icon: taxonomy.SkillIcon_SKILL_ICON_CHART}
	submitted, err := f.publishing.SubmitRelease(ctx, authenticated(cmd, author))
	if err != nil {
		t.Fatal(err)
	}
	id := submitted.Msg.Release.ReleaseId
	preview, err := f.publishing.GetSubmission(ctx, authenticated(&publishing.GetSubmissionRequest{ReleaseId: id}, f.owner))
	if err != nil || preview.Msg.Release.DisplayNameZh != "实验结果解读" || !bytes.Equal(preview.Msg.Archive, archive) {
		t.Fatal("review lost presentation or changed package", err)
	}
	cmd.DisplayNameEn = "Different title"
	if _, err = f.publishing.SubmitRelease(ctx, authenticated(cmd, author)); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatal("idempotency did not bind presentation", err)
	}
	for _, q := range []string{"Experiment", "实验结果"} {
		r, e := f.catalog.SearchSkills(ctx, connect.NewRequest(&catalog.SearchSkillsRequest{Query: q}))
		if e != nil || len(r.Msg.Skills) != 0 {
			t.Fatal("pending title leaked", e)
		}
	}
	if _, err = f.publishing.ReviewRelease(ctx, authenticated(&publishing.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: id, Approve: true}, f.owner)); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"experiment-readout", "experiment readout", "实验结果"} {
		r, e := f.catalog.SearchSkills(ctx, connect.NewRequest(&catalog.SearchSkillsRequest{Query: q}))
		if e != nil || len(r.Msg.Skills) != 1 || r.Msg.Skills[0].DisplayNameEn != "Experiment Readout" || r.Msg.Skills[0].Icon != taxonomy.SkillIcon_SKILL_ICON_CHART {
			t.Fatal("localized search failed", q, e)
		}
	}
	downloaded, err := f.catalog.DownloadRelease(ctx, connect.NewRequest(&catalog.DownloadReleaseRequest{Namespace: "team", Name: "experiment-readout", Version: "1.0.0"}))
	if err != nil || downloaded.Msg.Release.DisplayNameZh != "实验结果解读" || !bytes.Equal(downloaded.Msg.Archive, archive) {
		t.Fatal("download lost approved presentation", err)
	}
	cmd.Mutation = mutation()
	cmd.Version = "1.1.0"
	cmd.DisplayNameZh = "新版实验解读"
	cmd.Icon = taxonomy.SkillIcon_SKILL_ICON_RESEARCH
	newer, err := f.publishing.SubmitRelease(ctx, authenticated(cmd, author))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.publishing.ReviewRelease(ctx, authenticated(&publishing.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: newer.Msg.Release.ReleaseId, Approve: true}, f.owner)); err != nil {
		t.Fatal(err)
	}
	old, err := f.catalog.DownloadRelease(ctx, connect.NewRequest(&catalog.DownloadReleaseRequest{Namespace: "team", Name: "experiment-readout", Version: "1.0.0", KnownDigest: downloaded.Msg.Digest}))
	if err != nil || !old.Msg.NotModified || old.Msg.Release.DisplayNameEn != "Experiment Readout" || old.Msg.Release.Icon != taxonomy.SkillIcon_SKILL_ICON_CHART {
		t.Fatal("new version mutated old presentation", err)
	}
	for _, name := range []string{strings.Repeat("中", 81), "bad\nname", "bad\u202ename"} {
		cmd.Mutation = mutation()
		cmd.Version = "2.0.0"
		cmd.DisplayNameEn = name
		if _, err = f.publishing.SubmitRelease(ctx, authenticated(cmd, author)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatal("invalid display name accepted", err)
		}
	}
	cmd.Mutation = mutation()
	cmd.DisplayNameEn = ""
	cmd.DisplayNameZh = ""
	cmd.Icon = 999
	if _, err = f.publishing.SubmitRelease(ctx, authenticated(cmd, author)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatal("unknown icon accepted", err)
	}
	cmd.Mutation = mutation()
	cmd.Icon = 0
	legacy, err := f.publishing.SubmitRelease(ctx, authenticated(cmd, author))
	if err != nil || legacy.Msg.Release.DisplayNameEn != "" || legacy.Msg.Release.Name != "experiment-readout" {
		t.Fatal("legacy client rejected", err)
	}
}
