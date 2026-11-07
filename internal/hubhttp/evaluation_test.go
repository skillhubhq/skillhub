package hubhttp

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"connectrpc.com/connect"
	evaluationv1 "github.com/skillhubhq/skillhub/gen/api/evaluation/v1"
	"github.com/skillhubhq/skillhub/gen/api/evaluation/v1/evaluationv1connect"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/evaluation"
)

func TestEvaluationRPCVersionAudienceAndRevocation(t *testing.T) {
	for _, jsonCodec := range []bool{false, true} {
		t.Run(map[bool]string{false: "protobuf", true: "json"}[jsonCodec], func(t *testing.T) {
			f := registry(t, true)
			ctx := context.Background()
			opts := []connect.ClientOption{}
			if jsonCodec {
				opts = append(opts, connect.WithProtoJSON())
			}
			api := evaluationv1connect.NewEvaluationServiceClient(f.server.Client(), f.server.URL, opts...)
			publisherID, publisher := f.user(t, "publisher")
			reviewerID, reviewer := f.user(t, "reviewer")
			readerID, reader := f.user(t, "reader")
			f.grant(t, publisherID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, true)
			f.grant(t, reviewerID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, true)
			f.grant(t, readerID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_READER, true)
			group := f.group(t, "limited")
			release, err := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "eval-check", "Summarize input."), Visibility: taxonomy.Visibility_VISIBILITY_GROUPS, GroupIds: []string{group}}, publisher))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile("../../examples/evaluations/report.json")
			if err != nil {
				t.Fatal(err)
			}
			var report evaluation.Report
			if err = json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			report.SkillDigest = release.Msg.Release.Digest
			request := &evaluationv1.ImportEvidenceRequest{Mutation: mutation(), ReleaseId: release.Msg.Release.ReleaseId, Document: report.Canonical()}
			imported, err := api.ImportEvidence(ctx, authenticated(request, publisher))
			if err != nil {
				t.Fatal(err)
			}
			replay, err := api.ImportEvidence(ctx, authenticated(request, publisher))
			if err != nil || replay.Msg.Id != imported.Msg.Id {
				t.Fatal("idempotency", err)
			}
			_, err = api.ImportEvidence(ctx, authenticated(&evaluationv1.ImportEvidenceRequest{Mutation: mutation(), ReleaseId: request.ReleaseId, Document: report.Canonical()}, reviewer))
			if connect.CodeOf(err) != connect.CodeNotFound {
				t.Fatal("reviewer mutation", err)
			}
			_, err = f.publishing.ReviewRelease(ctx, authenticated(&publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: request.ReleaseId, Approve: true}, reviewer))
			if err != nil {
				t.Fatal(err)
			}
			for _, token := range []string{"", reader} {
				_, err = api.GetEvidence(ctx, rpcRequest(&evaluationv1.GetEvidenceRequest{Id: imported.Msg.Id}, token))
				if connect.CodeOf(err) != connect.CodeNotFound {
					t.Fatal("group evidence leaked", err)
				}
			}
			f.member(t, group, readerID, true)
			evidence, err := api.GetEvidence(ctx, authenticated(&evaluationv1.GetEvidenceRequest{Id: imported.Msg.Id}, reader))
			if err != nil || evidence.Msg.Evidence.DocumentJson == "" {
				t.Fatal("member evidence", err)
			}
			list, err := api.ListEvidence(ctx, authenticated(&evaluationv1.ListEvidenceRequest{ReleaseId: request.ReleaseId}, reader))
			if err != nil || list.Msg.CanManage || len(list.Msg.Evidence) != 1 || list.Msg.Evidence[0].DocumentJson != "" {
				t.Fatal("bounded summaries", err)
			}
			// 同一归档的不同版本不能共享关联记录。
			next, err := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.1", Archive: skillArchive(t, "eval-check", "Summarize input."), Visibility: taxonomy.Visibility_VISIBILITY_GROUPS, GroupIds: []string{group}}, publisher))
			if err != nil {
				t.Fatal(err)
			}
			list, err = api.ListEvidence(ctx, authenticated(&evaluationv1.ListEvidenceRequest{ReleaseId: next.Msg.Release.ReleaseId}, reviewer))
			if err != nil || len(list.Msg.Evidence) != 0 {
				t.Fatal("cross-version evidence", err)
			}
			f.member(t, group, readerID, false)
			_, err = api.GetEvidence(ctx, authenticated(&evaluationv1.GetEvidenceRequest{Id: imported.Msg.Id}, reader))
			if connect.CodeOf(err) != connect.CodeNotFound {
				t.Fatal("revoked audience", err)
			}
			f.grant(t, publisherID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, false)
			_, err = api.ImportEvidence(ctx, authenticated(request, publisher))
			if connect.CodeOf(err) != connect.CodeNotFound {
				t.Fatal("replay after revoke", err)
			}
			_, err = api.RetractEvidence(ctx, authenticated(&evaluationv1.RetractEvidenceRequest{Mutation: mutation(), Id: imported.Msg.Id}, f.owner))
			if err != nil {
				t.Fatal(err)
			}
			evidence, err = api.GetEvidence(ctx, authenticated(&evaluationv1.GetEvidenceRequest{Id: imported.Msg.Id}, reviewer))
			if err != nil || !evidence.Msg.Evidence.Retracted || evidence.Msg.Evidence.DocumentJson != "" {
				t.Fatal("tombstone", err)
			}
		})
	}
}
