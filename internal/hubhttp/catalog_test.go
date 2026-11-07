package hubhttp

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	"github.com/skillhubhq/skillhub/gen/api/admin/v1/adminv1connect"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/skillhubhq/skillhub/gen/api/auth/v1/authv1connect"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	"github.com/skillhubhq/skillhub/gen/api/catalog/v1/catalogv1connect"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	"github.com/skillhubhq/skillhub/gen/api/publishing/v1/publishingv1connect"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/google/uuid"
)

type registryFixture struct {
	root       string
	store      *hub.Store
	server     *httptest.Server
	owner      string
	admin      adminv1connect.AdminServiceClient
	auth       authv1connect.AuthServiceClient
	catalog    catalogv1connect.CatalogServiceClient
	publishing publishingv1connect.PublishingServiceClient
}

func registry(t *testing.T, review bool) *registryFixture {
	t.Helper()
	root := t.TempDir()
	store, err := testDatabase(t, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err = store.Bootstrap(context.Background(), "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	server, err := New(store, Config{PublicURL: "http://127.0.0.1", Private: false})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	f := &registryFixture{root: root, store: store, server: httpServer, admin: adminv1connect.NewAdminServiceClient(httpServer.Client(), httpServer.URL), auth: authv1connect.NewAuthServiceClient(httpServer.Client(), httpServer.URL), catalog: catalogv1connect.NewCatalogServiceClient(httpServer.Client(), httpServer.URL), publishing: publishingv1connect.NewPublishingServiceClient(httpServer.Client(), httpServer.URL)}
	f.owner = login(t, f.auth)
	if _, err = f.admin.PutNamespace(context.Background(), authenticated(&adminv1.PutNamespaceRequest{Mutation: mutation(), Name: "team", ReviewRequired: review}, f.owner)); err != nil {
		t.Fatal(err)
	}
	return f
}

func mutation() *commonv1.Mutation { return &commonv1.Mutation{IdempotencyKey: uuid.NewString()} }

func (f *registryFixture) user(t *testing.T, name string) (string, string) {
	t.Helper()
	user, err := f.admin.CreateUser(context.Background(), authenticated(&adminv1.CreateUserRequest{Mutation: mutation(), Username: name, Password: "a-user-test-password"}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	session, err := f.auth.Login(context.Background(), connect.NewRequest(&authv1.LoginRequest{Username: name, Password: "a-user-test-password"}))
	if err != nil {
		t.Fatal(err)
	}
	return user.Msg.User.UserId, session.Msg.AccessToken
}

func (f *registryFixture) group(t *testing.T, name string) string {
	t.Helper()
	result, err := f.admin.CreateGroup(context.Background(), authenticated(&adminv1.CreateGroupRequest{Mutation: mutation(), Name: name}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	return result.Msg.Group.GroupId
}
func (f *registryFixture) member(t *testing.T, group, user string, member bool) {
	t.Helper()
	if _, err := f.admin.SetGroupMember(context.Background(), authenticated(&adminv1.SetGroupMemberRequest{Mutation: mutation(), GroupId: group, UserId: user, Member: member}, f.owner)); err != nil {
		t.Fatal(err)
	}
}
func (f *registryFixture) grant(t *testing.T, subject string, kind taxonomy.PrincipalKind, role taxonomy.NamespaceRole, granted bool) {
	t.Helper()
	if _, err := f.admin.SetNamespaceGrant(context.Background(), authenticated(&adminv1.SetNamespaceGrantRequest{Mutation: mutation(), Namespace: "team", SubjectKind: kind, SubjectId: subject, Role: role, Granted: granted}, f.owner)); err != nil {
		t.Fatal(err)
	}
}

func skillArchive(t *testing.T, name, body string) []byte {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: A test skill.\n---\n"+body), 0600); err != nil {
		t.Fatal(err)
	}
	data, _, err := packageformat.Pack(dir)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func rpcRequest[T any](msg *T, token string) *connect.Request[T] {
	if token == "" {
		return connect.NewRequest(msg)
	}
	return authenticated(msg, token)
}

func (f *registryFixture) assertVisible(t *testing.T, token string, want int) {
	t.Helper()
	ctx := context.Background()
	search, err := f.catalog.SearchSkills(ctx, rpcRequest(&catalogv1.SearchSkillsRequest{}, token))
	if err != nil || len(search.Msg.Skills) != want {
		t.Fatalf("search visibility want %d: %v %v", want, search, err)
	}
	list, err := f.catalog.ListReleases(ctx, rpcRequest(&catalogv1.ListReleasesRequest{Namespace: "team", Name: "review-check"}, token))
	if err != nil || len(list.Msg.Releases) != want {
		t.Fatalf("release visibility want %d: %v %v", want, list, err)
	}
	_, err = f.catalog.GetSkill(ctx, rpcRequest(&catalogv1.GetSkillRequest{Namespace: "team", Name: "review-check"}, token))
	if (want == 0 && connect.CodeOf(err) != connect.CodeNotFound) || (want == 1 && err != nil) {
		t.Fatalf("detail visibility: %v", err)
	}
	_, err = f.catalog.DownloadRelease(ctx, rpcRequest(&catalogv1.DownloadReleaseRequest{Namespace: "team", Name: "review-check", Version: "1.0.0"}, token))
	if (want == 0 && connect.CodeOf(err) != connect.CodeNotFound) || (want == 1 && err != nil) {
		t.Fatalf("download visibility: %v", err)
	}
}

func TestPublishingIndependentReviewAndAudienceRevocation(t *testing.T) {
	f := registry(t, true)
	ctx := context.Background()
	reviewerID, reviewer := f.user(t, "reviewer")
	memberID, member := f.user(t, "member")
	_, outsider := f.user(t, "outsider")
	f.grant(t, reviewerID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, true)
	group := f.group(t, "internal")
	f.member(t, group, memberID, true)
	archive := skillArchive(t, "review-check", "Review the exact immutable contents.")
	submitted, err := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_GROUPS, GroupIds: []string{group}}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	id := submitted.Msg.Release.ReleaseId
	if submitted.Msg.Release.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING {
		t.Fatal("review bypassed")
	}
	for _, token := range []string{"", member, outsider, f.owner} {
		f.assertVisible(t, token, 0)
	}
	_, err = f.publishing.ReviewRelease(ctx, authenticated(&publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: id, Approve: true}, f.owner))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("self review: %v", err)
	}
	preview, err := f.publishing.GetSubmission(ctx, authenticated(&publishingv1.GetSubmissionRequest{ReleaseId: id}, reviewer))
	if err != nil || !bytes.Equal(preview.Msg.Archive, archive) || preview.Msg.Release.SkillMarkdown == "" {
		t.Fatalf("review material unavailable: %v", err)
	}
	_, err = f.publishing.GetSubmission(ctx, authenticated(&publishingv1.GetSubmissionRequest{ReleaseId: id}, outsider))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("unauthorized preview: %v", err)
	}
	approved, err := f.publishing.ReviewRelease(ctx, authenticated(&publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: id, Approve: true, Reason: "Inspected fixed package"}, reviewer))
	if err != nil || approved.Msg.Release.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PUBLISHED {
		t.Fatalf("approval: %v", err)
	}
	f.assertVisible(t, member, 1)
	f.assertVisible(t, "", 0)
	f.assertVisible(t, outsider, 0)
	f.member(t, group, memberID, false)
	f.assertVisible(t, member, 0)
	f.member(t, group, memberID, true)
	f.assertVisible(t, member, 1)
	if _, err = f.publishing.YankRelease(ctx, authenticated(&publishingv1.YankReleaseRequest{Mutation: mutation(), ReleaseId: id, Reason: "Withdraw this version"}, f.owner)); err != nil {
		t.Fatal(err)
	}
	f.assertVisible(t, member, 0)
	f.assertVisible(t, f.owner, 0)
	list, err := f.publishing.ListSubmissions(ctx, authenticated(&publishingv1.ListSubmissionsRequest{}, reviewer))
	if err != nil || len(list.Msg.Releases) != 1 || list.Msg.Releases[0].Status != taxonomy.ReleaseStatus_RELEASE_STATUS_YANKED {
		t.Fatalf("lost submission history: %v", err)
	}
	events, err := f.admin.ListAudit(ctx, authenticated(&adminv1.ListAuditRequest{Page: &commonv1.PageRequest{PageSize: 100}}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	approvedSeen, yankedSeen := false, false
	for _, event := range events.Msg.Events {
		if event.Resource == id && event.Action == "release.review" && event.Detail == "Inspected fixed package" {
			approvedSeen = true
		}
		if event.Resource == id && event.Action == "release.yank" && event.Detail == "Withdraw this version" {
			yankedSeen = true
		}
	}
	if !approvedSeen || !yankedSeen {
		t.Fatal("audit did not retain independent review and withdrawal reasons")
	}
}

func TestPublishingRoleGrantsImmutableVersionsAndIdempotency(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	writerID, writer := f.user(t, "writer")
	group := f.group(t, "maintainers")
	f.member(t, group, writerID, true)
	f.grant(t, group, taxonomy.PrincipalKind_PRINCIPAL_KIND_GROUP, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, true)
	archive := skillArchive(t, "review-check", "Original content")
	request := &publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC}
	var wg sync.WaitGroup
	ids := make(chan string, 2)
	failures := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			response, err := f.publishing.SubmitRelease(ctx, authenticated(request, writer))
			if err != nil {
				failures <- err
				return
			}
			ids <- response.Msg.Release.ReleaseId
		})
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	first := <-ids
	second := <-ids
	if first == "" || first != second {
		t.Fatal("retry created two releases")
	}
	f.assertVisible(t, "", 1)
	changed := &publishingv1.SubmitReleaseRequest{Mutation: request.Mutation, Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "review-check", "Changed content"), Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC}
	if _, err := f.publishing.SubmitRelease(ctx, authenticated(changed, writer)); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("idempotency payload conflict: %v", err)
	}
	changed.Mutation = mutation()
	if _, err := f.publishing.SubmitRelease(ctx, authenticated(changed, writer)); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("version overwritten: %v", err)
	}
	changed.Version = "2.0.0"
	changed.Mutation = mutation()
	changed.Visibility = taxonomy.Visibility(999)
	if _, err := f.publishing.SubmitRelease(ctx, authenticated(changed, writer)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unknown audience accepted: %v", err)
	}
	f.grant(t, group, taxonomy.PrincipalKind_PRINCIPAL_KIND_GROUP, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, false)
	if _, err := f.publishing.SubmitRelease(ctx, authenticated(request, writer)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("idempotency bypassed revoked grant: %v", err)
	}
	if _, err := f.admin.PutNamespace(ctx, authenticated(&adminv1.PutNamespaceRequest{Mutation: mutation(), Name: "team", ExpectedRevision: 1, ReviewRequired: true}, f.owner)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.PutNamespace(ctx, authenticated(&adminv1.PutNamespaceRequest{Mutation: mutation(), Name: "team", ExpectedRevision: 1}, f.owner)); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("stale policy overwrote latest: %v", err)
	}
}

func TestCatalogPaginationPrivateModeAndStoredIntegrity(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	_, member := f.user(t, "member")
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		if _, err := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, name, "Test"), Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC}, f.owner)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.catalog.SearchSkills(ctx, connect.NewRequest(&catalogv1.SearchSkillsRequest{Page: &commonv1.PageRequest{PageSize: 1}}))
	if err != nil || len(first.Msg.Skills) != 1 || first.Msg.Skills[0].Name != "alpha" || first.Msg.Page.NextPageToken == "" {
		t.Fatalf("first page: %v", err)
	}
	second, err := f.catalog.SearchSkills(ctx, connect.NewRequest(&catalogv1.SearchSkillsRequest{Page: &commonv1.PageRequest{PageSize: 1, PageToken: first.Msg.Page.NextPageToken}}))
	if err != nil || len(second.Msg.Skills) != 1 || second.Msg.Skills[0].Name != "bravo" {
		t.Fatalf("second page: %v", err)
	}
	for _, token := range []string{first.Msg.Page.NextPageToken + "x"} {
		if _, err = f.catalog.SearchSkills(ctx, connect.NewRequest(&catalogv1.SearchSkillsRequest{Page: &commonv1.PageRequest{PageToken: token}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("tampered cursor accepted: %v", err)
		}
	}
	if _, err = f.catalog.SearchSkills(ctx, authenticated(&catalogv1.SearchSkillsRequest{Page: &commonv1.PageRequest{PageToken: first.Msg.Page.NextPageToken}}, member)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("cursor crossed identity: %v", err)
	}
	if _, err = f.catalog.SearchSkills(ctx, connect.NewRequest(&catalogv1.SearchSkillsRequest{Query: "bravo", Page: &commonv1.PageRequest{PageToken: first.Msg.Page.NextPageToken}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("cursor crossed filter: %v", err)
	}
	if _, err = f.catalog.SearchSkills(ctx, authenticated(&catalogv1.SearchSkillsRequest{}, strings.Repeat("x", 43))); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("invalid credential fell back to public")
	}
	private, err := New(f.store, Config{PublicURL: "http://127.0.0.1", Private: true})
	if err != nil {
		t.Fatal(err)
	}
	privateServer := httptest.NewServer(private)
	defer privateServer.Close()
	privateCatalog := catalogv1connect.NewCatalogServiceClient(privateServer.Client(), privateServer.URL)
	if _, err = privateCatalog.SearchSkills(ctx, connect.NewRequest(&catalogv1.SearchSkillsRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("private registry exposed public release")
	}
	if _, err = privateCatalog.SearchSkills(ctx, authenticated(&catalogv1.SearchSkillsRequest{}, member)); err != nil {
		t.Fatal(err)
	}
	data, err := f.catalog.DownloadRelease(ctx, connect.NewRequest(&catalogv1.DownloadReleaseRequest{Namespace: "team", Name: "alpha", Version: "1.0.0"}))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(f.root, "packages", strings.TrimPrefix(data.Msg.Digest, "sha256:")+".zip"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.catalog.DownloadRelease(ctx, connect.NewRequest(&catalogv1.DownloadReleaseRequest{Namespace: "team", Name: "alpha", Version: "1.0.0"})); connect.CodeOf(err) != connect.CodeInternal || strings.Contains(err.Error(), f.root) {
		t.Fatalf("corrupt package delivered or diagnostic leaked: %v", err)
	}
}

func TestCategoryLifecycleRejectionAndAdminBoundaries(t *testing.T) {
	f := registry(t, true)
	ctx := context.Background()
	reviewerID, reviewer := f.user(t, "reviewer")
	f.grant(t, reviewerID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, true)
	category, err := f.admin.PutCategory(ctx, authenticated(&adminv1.PutCategoryRequest{Mutation: mutation(), Name: "operations", Title: "Operations"}, f.owner))
	if err != nil || category.Msg.Category.Revision != 1 {
		t.Fatalf("category create: %v", err)
	}
	archive := skillArchive(t, "review-check", "Rejected test")
	submit := &publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC, Category: "operations", Tags: []string{"release", "checks"}}
	submitted, err := f.publishing.SubmitRelease(ctx, authenticated(submit, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := f.publishing.ReviewRelease(ctx, authenticated(&publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: submitted.Msg.Release.ReleaseId, Reason: "Missing operational steps"}, reviewer))
	if err != nil || rejected.Msg.Release.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_REJECTED {
		t.Fatalf("rejection: %v", err)
	}
	f.assertVisible(t, "", 0)
	submit.Mutation = mutation()
	submit.Version = "1.1.0"
	second, err := f.publishing.SubmitRelease(ctx, authenticated(submit, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.publishing.ReviewRelease(ctx, authenticated(&publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: second.Msg.Release.ReleaseId, Approve: true}, reviewer)); err != nil {
		t.Fatal(err)
	}
	filtered, err := f.catalog.SearchSkills(ctx, connect.NewRequest(&catalogv1.SearchSkillsRequest{Category: "operations", Tags: []string{"checks", "release"}}))
	if err != nil || len(filtered.Msg.Skills) != 1 || filtered.Msg.Skills[0].LatestVersion != "1.1.0" {
		t.Fatalf("classification filter: %v", err)
	}
	filtered, err = f.catalog.SearchSkills(ctx, connect.NewRequest(&catalogv1.SearchSkillsRequest{Category: "operations", Tags: []string{"unrelated"}}))
	if err != nil || len(filtered.Msg.Skills) != 0 {
		t.Fatalf("tag filter widened result: %v", err)
	}
	if _, err = f.admin.PutCategory(ctx, authenticated(&adminv1.PutCategoryRequest{Mutation: mutation(), Name: "operations", Title: "Operations", Archived: true, ExpectedRevision: 1}, reviewer)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("reviewer edited Hub taxonomy: %v", err)
	}
	if _, err = f.admin.PutCategory(ctx, authenticated(&adminv1.PutCategoryRequest{Mutation: mutation(), Name: "operations", Title: "Operations", Archived: true, ExpectedRevision: 1}, f.owner)); err != nil {
		t.Fatal(err)
	}
	submit.Mutation = mutation()
	submit.Version = "1.2.0"
	if _, err = f.publishing.SubmitRelease(ctx, authenticated(submit, f.owner)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("archived category accepted: %v", err)
	}
	if _, err = f.admin.ListUsers(ctx, authenticated(&adminv1.ListUsersRequest{}, reviewer)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("reviewer read admin user directory: %v", err)
	}
	if _, err = f.admin.ListAudit(ctx, authenticated(&adminv1.ListAuditRequest{}, reviewer)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("reviewer read whole-Hub audit: %v", err)
	}
	namespaces, err := f.admin.ListNamespaces(ctx, authenticated(&adminv1.ListNamespacesRequest{}, reviewer))
	if err != nil || len(namespaces.Msg.Namespaces) != 1 {
		t.Fatalf("reviewer namespace list: %v", err)
	}
	grants, err := f.admin.ListNamespaceGrants(ctx, authenticated(&adminv1.ListNamespaceGrantsRequest{Namespace: "team"}, f.owner))
	if err != nil || len(grants.Msg.Grants) != 1 || grants.Msg.Grants[0].SubjectId != reviewerID {
		t.Fatalf("grant list: %v", err)
	}
	if _, err = f.admin.SetNamespaceGrant(ctx, authenticated(&adminv1.SetNamespaceGrantRequest{Mutation: mutation(), Namespace: "team", SubjectId: reviewerID, SubjectKind: taxonomy.PrincipalKind(999), Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN, Granted: true}, f.owner)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unknown subject kind accepted: %v", err)
	}
	if _, err = f.admin.SetUserDisabled(ctx, authenticated(&adminv1.SetUserDisabledRequest{Mutation: mutation(), UserId: reviewerID, Disabled: true}, f.owner)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.auth.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, reviewer)); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("disabled account retained access")
	}
}

func TestReaderMustBelongToGroupAudience(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	readerID, reader := f.user(t, "reader")
	f.grant(t, readerID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_READER, true)
	group := f.group(t, "restricted-audience")
	submitted, err := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "review-check", "Group protected"), Visibility: taxonomy.Visibility_VISIBILITY_GROUPS, GroupIds: []string{group}}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	f.assertVisible(t, reader, 0)
	f.member(t, group, readerID, true)
	f.assertVisible(t, reader, 1)
	f.member(t, group, readerID, false)
	f.assertVisible(t, reader, 0)
	_, err = f.catalog.DownloadRelease(ctx, authenticated(&catalogv1.DownloadReleaseRequest{Namespace: "team", Name: "review-check", Version: "1.0.0", KnownDigest: submitted.Msg.Release.Digest}, reader))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatal("conditional download bypassed group", err)
	}
	for _, role := range []taxonomy.NamespaceRole{taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN} {
		f.grant(t, readerID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, role, true)
		f.assertVisible(t, reader, 1)
		f.grant(t, readerID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, role, false)
		f.assertVisible(t, reader, 0)
	}
}
