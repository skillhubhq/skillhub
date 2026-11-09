package hubhttp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	communityv1 "github.com/skillhubhq/skillhub/gen/api/community/v1"
	"github.com/skillhubhq/skillhub/gen/api/community/v1/communityv1connect"
	governancev1 "github.com/skillhubhq/skillhub/gen/api/governance/v1"
	"github.com/skillhubhq/skillhub/gen/api/governance/v1/governancev1connect"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
)

func TestDeliverySubscriptionsDiffAndAudienceRevocation(t *testing.T) {
	for _, jsonCodec := range []bool{false, true} {
		t.Run(map[bool]string{false: "protobuf", true: "json"}[jsonCodec], func(t *testing.T) {
			f := registry(t, false)
			ctx := context.Background()
			opts := []connect.ClientOption{}
			if jsonCodec {
				opts = append(opts, connect.WithProtoJSON())
			}
			c := communityv1connect.NewCommunityServiceClient(f.server.Client(), f.server.URL, opts...)
			g := governancev1connect.NewGovernanceServiceClient(f.server.Client(), f.server.URL, opts...)
			id, user := f.user(t, "subscriber")
			group := f.group(t, "members")
			f.member(t, group, id, true)
			publish := func(version, body string) *catalogv1.Release {
				t.Helper()
				r, e := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: version, Archive: skillArchive(t, "delivery", body), Visibility: taxonomy.Visibility_VISIBILITY_GROUPS, GroupIds: []string{group}}, f.owner))
				if e != nil {
					t.Fatal(e)
				}
				return r.Msg.Release
			}
			first := publish("1.0.0", "Original instructions")
			if _, e := c.SetSubscription(ctx, authenticated(&communityv1.SetSubscriptionRequest{Mutation: mutation(), Namespace: "team", Name: "delivery", Subscribed: true}, user)); e != nil {
				t.Fatal(e)
			}
			second := publish("1.1.0", "Updated instructions")
			notes, e := g.ListNotifications(ctx, authenticated(&governancev1.ListNotificationsRequest{}, user))
			if e != nil || len(notes.Msg.Notifications) != 1 || notes.Msg.Notifications[0].Kind != "update" {
				t.Fatal("missing update", notes, e)
			}
			diff, e := f.catalog.CompareReleases(ctx, authenticated(&catalogv1.CompareReleasesRequest{Namespace: "team", Name: "delivery", BeforeId: first.ReleaseId, AfterId: second.ReleaseId}, user))
			if e != nil || len(diff.Msg.Files) != 1 || !diff.Msg.Files[0].TextAvailable || diff.Msg.Files[0].BeforeText == diff.Msg.Files[0].AfterText {
				t.Fatal("diff", diff, e)
			}
			f.member(t, group, id, false)
			if _, e = f.catalog.CompareReleases(ctx, authenticated(&catalogv1.CompareReleasesRequest{Namespace: "team", Name: "delivery", BeforeId: first.ReleaseId, AfterId: second.ReleaseId}, user)); connect.CodeOf(e) != connect.CodeNotFound {
				t.Fatal("diff leaked", e)
			}
			notes, e = g.ListNotifications(ctx, authenticated(&governancev1.ListNotificationsRequest{}, user))
			if e != nil || !notes.Msg.Notifications[0].Unavailable || notes.Msg.Notifications[0].Coordinate != "" {
				t.Fatal("notification leaked", e)
			}
			publish("1.2.0", "New inaccessible instructions")
			notes, e = g.ListNotifications(ctx, authenticated(&governancev1.ListNotificationsRequest{}, user))
			if e != nil || len(notes.Msg.Notifications) != 1 {
				t.Fatal("unauthorized notification", e)
			}
			list, e := c.ListSubscriptions(ctx, authenticated(&communityv1.ListSubscriptionsRequest{}, user))
			if e != nil || len(list.Msg.Subscriptions) != 1 || list.Msg.Subscriptions[0].Available {
				t.Fatal("subscription availability", e)
			}
			if _, e = c.SetSubscription(ctx, authenticated(&communityv1.SetSubscriptionRequest{Mutation: mutation(), Namespace: "team", Name: "delivery", Subscribed: false}, user)); e != nil {
				t.Fatal("cannot unfollow unavailable skill", e)
			}
		})
	}
}
func TestAutomationTokensCannotEscalateOrOutliveGrants(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	id, user := f.user(t, "author")
	f.grant(t, id, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, true)
	create := func(scope, namespace string) *authv1.CreateAccessTokenResponse {
		t.Helper()
		v, e := f.auth.CreateAccessToken(ctx, authenticated(&authv1.CreateAccessTokenRequest{Name: "CI", Scope: scope, Namespace: namespace, Days: 1}, user))
		if e != nil {
			t.Fatal(e)
		}
		if v.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("secret response cached")
		}
		return v.Msg
	}
	read := create("read", "")
	write := create("publish", "team")
	submission := func(token, namespace, version string) error {
		_, e := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: namespace, Version: version, Archive: skillArchive(t, "token-test", "Instructions"), Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}, token))
		return e
	}
	if e := submission(read.Secret, "team", "1.0.0"); connect.CodeOf(e) != connect.CodePermissionDenied {
		t.Fatal("read token published", e)
	}
	if e := submission(write.Secret, "team", "1.0.0"); e != nil {
		t.Fatal("publish denied", e)
	}
	if e := submission(write.Secret, "other", "1.0.0"); connect.CodeOf(e) != connect.CodePermissionDenied {
		t.Fatal("namespace escape", e)
	}
	if _, e := f.auth.CreateAccessToken(ctx, authenticated(&authv1.CreateAccessTokenRequest{Name: "escape", Scope: "read", Days: 1}, write.Secret)); connect.CodeOf(e) != connect.CodePermissionDenied {
		t.Fatal("token minted token", e)
	}
	if _, e := f.admin.ListUsers(ctx, authenticated(&adminv1.ListUsersRequest{}, write.Secret)); connect.CodeOf(e) != connect.CodePermissionDenied {
		t.Fatal("token read users", e)
	}
	if _, e := f.catalog.DownloadRelease(ctx, authenticated(&catalogv1.DownloadReleaseRequest{Namespace: "team", Name: "token-test", Version: "1.0.0"}, read.Secret)); e != nil {
		t.Fatal("read token", e)
	}
	if _, e := f.auth.RevokeAccessToken(ctx, authenticated(&authv1.RevokeAccessTokenRequest{Id: read.Token.Id}, user)); e != nil {
		t.Fatal(e)
	}
	if _, e := f.auth.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, read.Secret)); connect.CodeOf(e) != connect.CodeUnauthenticated {
		t.Fatal("revoked token", e)
	}
	f.grant(t, id, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, false)
	if e := submission(write.Secret, "team", "1.1.0"); connect.CodeOf(e) != connect.CodePermissionDenied {
		t.Fatal("revoked role still publishes", e)
	}
}

func TestDeliverySuiteMemberAccessDigestAndRevocation(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	uid, user := f.user(t, "consumer")
	group := f.group(t, "suite-readers")
	f.member(t, group, uid, true)
	child, e := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "member", "Member instructions"), Visibility: taxonomy.Visibility_VISIBILITY_GROUPS, GroupIds: []string{group}}, f.owner))
	if e != nil {
		t.Fatal(e)
	}
	bundle := func(name, member, digest string) []byte {
		t.Helper()
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		markdown, _ := w.Create("SKILL.md")
		_, _ = markdown.Write([]byte("---\nname: " + name + "\ndescription: A test suite\n---\nUse the member skill."))
		manifest, _ := w.Create("skillhub.suite.json")
		_ = json.NewEncoder(manifest).Encode(map[string]any{"schema": 1, "entry": "team/" + member, "members": []map[string]string{{"namespace": "team", "name": member, "version": "1.0.0", "digest": digest}}})
		if e := w.Close(); e != nil {
			t.Fatal(e)
		}
		return b.Bytes()
	}
	submit := func(name, member, digest string) (*connect.Response[publishingv1.SubmitReleaseResponse], error) {
		return f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: bundle(name, member, digest), Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC}, f.owner))
	}
	if _, e = submit("bad-suite", "member", "sha256:"+strings.Repeat("a", 64)); connect.CodeOf(e) != connect.CodeAlreadyExists && connect.CodeOf(e) != connect.CodeFailedPrecondition {
		t.Fatal("digest mismatch accepted", e)
	}
	suite, e := submit("starter", "member", child.Msg.Release.Digest)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = submit("nested", "starter", suite.Msg.Release.Digest); connect.CodeOf(e) != connect.CodeInvalidArgument {
		t.Fatal("nested suite accepted", e)
	}
	if _, e = f.catalog.DownloadRelease(ctx, authenticated(&catalogv1.DownloadReleaseRequest{Namespace: "team", Name: "starter", Version: "1.0.0"}, user)); e != nil {
		t.Fatal(e)
	}
	f.member(t, group, uid, false)
	if _, e = f.catalog.DownloadRelease(ctx, authenticated(&catalogv1.DownloadReleaseRequest{Namespace: "team", Name: "starter", Version: "1.0.0", KnownDigest: suite.Msg.Release.Digest}, user)); connect.CodeOf(e) != connect.CodeNotFound {
		t.Fatal("cache bypassed member permissions", e)
	}
}

func TestDeliverySubscriptionPaginationAndApprovalDeduplication(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	_, reader := f.user(t, "follower")
	community := communityv1connect.NewCommunityServiceClient(f.server.Client(), f.server.URL)
	governance := governancev1connect.NewGovernanceServiceClient(f.server.Client(), f.server.URL)
	for _, ns := range []string{"a", "a-b", "b"} {
		if _, e := f.admin.PutNamespace(ctx, authenticated(&adminv1.PutNamespaceRequest{Mutation: mutation(), Name: ns}, f.owner)); e != nil {
			t.Fatal(e)
		}
		if _, e := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: ns, Version: "1.0.0", Archive: skillArchive(t, "example", "Example instructions"), Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}, f.owner)); e != nil {
			t.Fatal(e)
		}
		if _, e := community.SetSubscription(ctx, authenticated(&communityv1.SetSubscriptionRequest{Mutation: mutation(), Namespace: ns, Name: "example", Subscribed: true}, reader)); e != nil {
			t.Fatal(e)
		}
	}
	cursor := ""
	seen := map[string]bool{}
	for {
		page, e := community.ListSubscriptions(ctx, authenticated(&communityv1.ListSubscriptionsRequest{Page: &commonv1.PageRequest{PageSize: 1, PageToken: cursor}}, reader))
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range page.Msg.Subscriptions {
			if seen[item.Namespace] {
				t.Fatal("duplicate subscription", item.Namespace)
			}
			seen[item.Namespace] = true
		}
		cursor = page.Msg.GetPage().GetNextPageToken()
		if cursor == "" {
			break
		}
	}
	if len(seen) != 3 {
		t.Fatal("pagination skipped subscriptions", seen)
	}
	if _, e := f.admin.PutNamespace(ctx, authenticated(&adminv1.PutNamespaceRequest{Mutation: mutation(), Name: "a", ExpectedRevision: 1, ReviewRequired: true}, f.owner)); e != nil {
		t.Fatal(e)
	}
	next, e := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "a", Version: "1.1.0", Archive: skillArchive(t, "example", "Updated example"), Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}, f.owner))
	if e != nil {
		t.Fatal(e)
	}
	notes, e := governance.ListNotifications(ctx, authenticated(&governancev1.ListNotificationsRequest{}, reader))
	if e != nil || len(notes.Msg.Notifications) != 0 {
		t.Fatal("unapproved version notified", notes, e)
	}
	reviewerID, reviewer := f.user(t, "reviewer")
	// 通过管理员授权独立账号审核此命名空间。
	if _, e = f.admin.SetNamespaceGrant(ctx, authenticated(&adminv1.SetNamespaceGrantRequest{Mutation: mutation(), Namespace: "a", SubjectKind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectId: reviewerID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, Granted: true}, f.owner)); e != nil {
		t.Fatal(e)
	}
	review := &publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: next.Msg.Release.ReleaseId, Approve: true}
	for i := 0; i < 2; i++ {
		if _, e = f.publishing.ReviewRelease(ctx, authenticated(review, reviewer)); e != nil {
			t.Fatal(e)
		}
	}
	notes, e = governance.ListNotifications(ctx, authenticated(&governancev1.ListNotificationsRequest{}, reader))
	if e != nil || len(notes.Msg.Notifications) != 1 || notes.Msg.Notifications[0].Kind != "update" {
		t.Fatal("approval notification duplicated or absent", notes, e)
	}
}
