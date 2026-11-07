package hubhttp

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	governancev1 "github.com/skillhubhq/skillhub/gen/api/governance/v1"
	"github.com/skillhubhq/skillhub/gen/api/governance/v1/governancev1connect"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
)

func TestDiscoveryReviewTasksAndCuration(t *testing.T) {
	for _, jsonCodec := range []bool{false, true} {
		t.Run(map[bool]string{false: "binary", true: "json"}[jsonCodec], func(t *testing.T) {
			f := registry(t, true)
			ctx := context.Background()
			opts := []connect.ClientOption{}
			if jsonCodec {
				opts = append(opts, connect.WithProtoJSON())
			}
			g := governancev1connect.NewGovernanceServiceClient(f.server.Client(), f.server.URL, opts...)
			pid, publisher := f.user(t, "publisher")
			rid, reviewer := f.user(t, "reviewer")
			_, reader := f.user(t, "reader")
			f.grant(t, pid, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, true)
			f.grant(t, rid, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, true)
			submit := func(name, version string, visibility taxonomy.Visibility) string {
				r, e := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: version, Archive: skillArchive(t, name, "Test content."), Visibility: visibility}, publisher))
				if e != nil {
					t.Fatal(e)
				}
				return r.Msg.Release.ReleaseId
			}
			first := submit("alpha", "1.0.0", taxonomy.Visibility_VISIBILITY_MEMBERS)
			second := submit("beta", "1.0.0", taxonomy.Visibility_VISIBILITY_MEMBERS)
			task, e := g.ListReviewTasks(ctx, authenticated(&governancev1.ListReviewTasksRequest{Page: &commonv1.PageRequest{PageSize: 1}}, reviewer))
			if e != nil || task.Msg.Total != 2 || len(task.Msg.Tasks) != 1 || task.Msg.Tasks[0].ReleaseId != first || task.Msg.OldestAt == nil || task.Msg.Page.NextPageToken == "" {
				t.Fatal(task, e)
			}
			cursor := task.Msg.Page.NextPageToken
			f.grant(t, pid, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, true)
			for _, token := range []string{publisher, reader} {
				r, e := g.ListReviewTasks(ctx, authenticated(&governancev1.ListReviewTasksRequest{}, token))
				if e != nil || r.Msg.Total != 0 || len(r.Msg.Tasks) != 0 {
					t.Fatal("nonreviewer tasks", r, e)
				}
			}
			_, e = g.ListReviewTasks(ctx, authenticated(&governancev1.ListReviewTasksRequest{Page: &commonv1.PageRequest{PageToken: cursor}}, publisher))
			if connect.CodeOf(e) != connect.CodeInvalidArgument {
				t.Fatal("cross actor cursor", e)
			}
			for _, id := range []string{first, second} {
				_, e = f.publishing.ReviewRelease(ctx, authenticated(&publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: id, Approve: true}, reviewer))
				if e != nil {
					t.Fatal(e)
				}
			}
			task, e = g.ListReviewTasks(ctx, authenticated(&governancev1.ListReviewTasksRequest{}, reviewer))
			if e != nil || task.Msg.Total != 0 {
				t.Fatal(task, e)
			}
			_, e = g.SetFeaturedSkill(ctx, authenticated(&governancev1.SetFeaturedSkillRequest{Mutation: mutation(), Namespace: "team", Name: "alpha", Featured: true}, reviewer))
			if connect.CodeOf(e) != connect.CodePermissionDenied {
				t.Fatal("reviewer curated", e)
			}
			key := mutation()
			set := &governancev1.SetFeaturedSkillRequest{Mutation: key, Namespace: "team", Name: "alpha", Featured: true}
			for i := 0; i < 2; i++ {
				_, e = g.SetFeaturedSkill(ctx, authenticated(set, f.owner))
				if e != nil {
					t.Fatal(e)
				}
			}
			search := func(token string, order taxonomy.CatalogOrder, featured bool, cursor string) *catalogv1.SearchSkillsResponse {
				r, e := f.catalog.SearchSkills(ctx, rpcRequest(&catalogv1.SearchSkillsRequest{Order: order, FeaturedOnly: featured, Page: &commonv1.PageRequest{PageSize: 1, PageToken: cursor}}, token))
				if e != nil {
					t.Fatal(e)
				}
				return r.Msg
			}
			recent := search(reader, taxonomy.CatalogOrder_CATALOG_ORDER_RECENT, false, "")
			if len(recent.Skills) != 1 || recent.Skills[0].Name != "beta" {
				t.Fatal(recent)
			}
			next := search(reader, taxonomy.CatalogOrder_CATALOG_ORDER_RECENT, false, recent.Page.NextPageToken)
			if len(next.Skills) != 1 || next.Skills[0].Name != "alpha" {
				t.Fatal(next)
			}
			_, e = f.catalog.SearchSkills(ctx, authenticated(&catalogv1.SearchSkillsRequest{Page: &commonv1.PageRequest{PageToken: recent.Page.NextPageToken}}, reader))
			if connect.CodeOf(e) != connect.CodeInvalidArgument {
				t.Fatal("cursor order changed", e)
			}
			pick := search(reader, 0, true, "")
			if len(pick.Skills) != 1 || pick.Skills[0].Name != "alpha" || !pick.Skills[0].Featured {
				t.Fatal(pick)
			}
			if hidden := search("", 0, true, ""); len(hidden.Skills) != 0 {
				t.Fatal("featured leaks private content", hidden)
			}
			_, e = f.catalog.SearchSkills(ctx, authenticated(&catalogv1.SearchSkillsRequest{Order: taxonomy.CatalogOrder(99)}, reader))
			if connect.CodeOf(e) != connect.CodeInvalidArgument {
				t.Fatal("unknown order", e)
			}
			// 撤下、受众变化和新版本都不得借精选扩大权限。
			_, e = f.publishing.YankRelease(ctx, authenticated(&publishingv1.YankReleaseRequest{Mutation: mutation(), ReleaseId: first, Reason: "Fixture withdrawal"}, publisher))
			if e != nil {
				t.Fatal(e)
			}
			if pick = search(reader, 0, true, ""); len(pick.Skills) != 0 {
				t.Fatal("withdrawn pick", pick)
			}
			third := submit("gamma", "1.0.0", taxonomy.Visibility_VISIBILITY_MEMBERS)
			f.grant(t, rid, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, false)
			task, e = g.ListReviewTasks(ctx, authenticated(&governancev1.ListReviewTasksRequest{}, reviewer))
			if e != nil || task.Msg.Total != 0 {
				t.Fatal("revoked reviewer tasks", task, e)
			}
			_, e = f.publishing.ReviewRelease(ctx, authenticated(&publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: third, Approve: true}, reviewer))
			if connect.CodeOf(e) != connect.CodePermissionDenied {
				t.Fatal("revoked reviewer approved", e)
			}
		})
	}
}
