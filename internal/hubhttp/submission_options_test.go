package hubhttp

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
)

func TestSubmissionNamespacesFollowCurrentPermissions(t *testing.T) {
	f := registry(t, true)
	ctx := context.Background()
	userID, token := f.user(t, "publisher")
	list := func(token string, filtered bool, want int) {
		t.Helper()
		got, err := f.admin.ListNamespaces(ctx, rpcRequest(&adminv1.ListNamespacesRequest{ForSubmission: filtered}, token))
		if err != nil || len(got.Msg.Namespaces) != want {
			t.Fatalf("namespaces filtered=%v want=%d: %v %v", filtered, want, got, err)
		}
	}
	if _, err := f.admin.ListNamespaces(ctx, connect.NewRequest(&adminv1.ListNamespacesRequest{ForSubmission: true})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("anonymous options: %v", err)
	}
	list(token, true, 0)
	list(f.owner, true, 1)
	for _, role := range []taxonomy.NamespaceRole{taxonomy.NamespaceRole_NAMESPACE_ROLE_READER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN} {
		f.grant(t, userID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, role, true)
		list(token, false, 1)
		want := 0
		if role == taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER || role == taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN {
			want = 1
		}
		list(token, true, want)
		f.grant(t, userID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, role, false)
		list(token, true, 0)
	}
	group := f.group(t, "publishers")
	f.grant(t, group, taxonomy.PrincipalKind_PRINCIPAL_KIND_GROUP, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, true)
	f.member(t, group, userID, true)
	list(token, true, 1)
	f.member(t, group, userID, false)
	list(token, true, 0)
	// 列表不是授权凭证，撤权后仍需拒绝之前已选中的提交。
	_, err := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "options-check", "No stale authority."), Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}, token))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("stale options bypassed authorization: %v", err)
	}
}

func TestSubmissionNamespacePaginationAndInvalidPublishChoices(t *testing.T) {
	f := registry(t, true)
	ctx := context.Background()
	for _, name := range []string{"alpha", "beta"} {
		if _, err := f.admin.PutNamespace(ctx, authenticated(&adminv1.PutNamespaceRequest{Mutation: mutation(), Name: name, ReviewRequired: true}, f.owner)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.admin.ListNamespaces(ctx, authenticated(&adminv1.ListNamespacesRequest{ForSubmission: true, Page: &commonv1.PageRequest{PageSize: 1}}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Msg.Namespaces) != 1 || first.Msg.Namespaces[0].Name != "alpha" || first.Msg.Page.NextPageToken == "" {
		t.Fatalf("first page: %v", first.Msg)
	}
	next := &adminv1.ListNamespacesRequest{ForSubmission: true, Page: &commonv1.PageRequest{PageSize: 1, PageToken: first.Msg.Page.NextPageToken}}
	second, err := f.admin.ListNamespaces(ctx, authenticated(next, f.owner))
	if err != nil || len(second.Msg.Namespaces) != 1 || second.Msg.Namespaces[0].Name != "beta" {
		t.Fatalf("second page: %v %v", second, err)
	}
	next.ForSubmission = false
	if _, err := f.admin.ListNamespaces(ctx, authenticated(next, f.owner)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("cursor reused across filters: %v", err)
	}
	archive := skillArchive(t, "options-check", "Validate selections on submit.")
	for _, tc := range []struct {
		namespace, category string
		code                connect.Code
	}{
		{"missing", "", connect.CodeNotFound},
		{"team", "missing", connect.CodeInvalidArgument},
	} {
		_, err := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: tc.namespace, Category: tc.category, Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}, f.owner))
		if connect.CodeOf(err) != tc.code {
			t.Fatalf("invalid choice namespace=%s category=%s: %v", tc.namespace, tc.category, err)
		}
	}
}
