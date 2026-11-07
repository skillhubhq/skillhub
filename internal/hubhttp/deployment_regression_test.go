package hubhttp

import (
	"connectrpc.com/connect"
	"context"
	"fmt"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hub"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

func TestExactDetailAndSubmissionFiltersBeyondFirstPage(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	actor, err := f.store.Authenticate(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	archive := skillArchive(t, "review-check", "Pagination regression")
	for i := 0; i < 31; i++ {
		ns := fmt.Sprintf("ns-%02d", i)
		if _, err = f.store.PutNamespace(ctx, actor, "ns-"+ns, hub.PutNamespace{Name: ns}); err != nil {
			t.Fatal(err)
		}
		if _, err = f.store.SubmitRelease(ctx, actor, "rel-"+ns, hub.SubmitRelease{Namespace: ns, Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.catalog.SearchSkills(ctx, connect.NewRequest(&catalogv1.SearchSkillsRequest{Query: "review-check"}))
	if err != nil || len(first.Msg.Skills) != 30 || first.Msg.Page.NextPageToken == "" {
		t.Fatalf("search page: %v %v", first, err)
	}
	exact, err := f.catalog.GetSkill(ctx, connect.NewRequest(&catalogv1.GetSkillRequest{Namespace: "ns-30", Name: "review-check"}))
	if err != nil || exact.Msg.Skill.Namespace != "ns-30" {
		t.Fatalf("exact lookup: %v", err)
	}
	id, token := f.user(t, "publisher")
	f.grant(t, id, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, true)
	for i := 0; i < 32; i++ {
		_, err = f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: fmt.Sprintf("2.0.%d", i), Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC}, token))
		if err != nil {
			t.Fatal(err)
		}
	}
	filtered, err := f.publishing.ListSubmissions(ctx, authenticated(&publishingv1.ListSubmissionsRequest{Mine: true, Query: "2.0.31", Status: taxonomy.ReleaseStatus_RELEASE_STATUS_PUBLISHED}, token))
	if err != nil || len(filtered.Msg.Releases) != 1 || filtered.Msg.Releases[0].Version != "2.0.31" {
		t.Fatalf("server filter %v %v", filtered, err)
	}
	page, err := f.publishing.ListSubmissions(ctx, authenticated(&publishingv1.ListSubmissionsRequest{Mine: true}, token))
	if err != nil || len(page.Msg.Releases) != 30 || page.Msg.Page.NextPageToken == "" {
		t.Fatalf("mine page %v %v", page, err)
	}
	next, err := f.publishing.ListSubmissions(ctx, authenticated(&publishingv1.ListSubmissionsRequest{Mine: true, Page: &commonv1.PageRequest{PageToken: page.Msg.Page.NextPageToken}}, token))
	if err != nil || len(next.Msg.Releases) != 2 {
		t.Fatalf("mine next %v %v", next, err)
	}
	_, err = f.publishing.ListSubmissions(ctx, authenticated(&publishingv1.ListSubmissionsRequest{Mine: true, Query: "2.0.31", Page: &commonv1.PageRequest{PageToken: page.Msg.Page.NextPageToken}}, token))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("cursor crossed filters: %v", err)
	}
	history, err := f.catalog.ListReleases(ctx, connect.NewRequest(&catalogv1.ListReleasesRequest{Namespace: "team", Name: "review-check"}))
	if err != nil || len(history.Msg.Releases) != 30 || history.Msg.Page.NextPageToken == "" {
		t.Fatalf("history %v %v", history, err)
	}
	history, err = f.catalog.ListReleases(ctx, connect.NewRequest(&catalogv1.ListReleasesRequest{Namespace: "team", Name: "review-check", Page: &commonv1.PageRequest{PageToken: history.Msg.Page.NextPageToken}}))
	if err != nil || len(history.Msg.Releases) != 2 {
		t.Fatalf("history next %v %v", history, err)
	}
}

func TestNamespaceLookupAndCapabilities(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	id, token := f.user(t, "namespace-admin")
	target, targetToken := f.user(t, "ordinary-user")
	f.grant(t, id, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN, true)
	_, err := f.admin.ListUsers(ctx, authenticated(&adminv1.ListUsersRequest{}, token))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatal(err)
	}
	found, err := f.admin.LookupNamespaceUsers(ctx, authenticated(&adminv1.LookupNamespaceUsersRequest{Namespace: "team", Query: "ordinary"}, token))
	if err != nil || len(found.Msg.Users) != 1 || found.Msg.Users[0].UserId != target {
		t.Fatalf("lookup %v %v", found, err)
	}
	_, err = f.admin.LookupNamespaceUsers(ctx, authenticated(&adminv1.LookupNamespaceUsersRequest{Namespace: "team", Query: "ordinary"}, targetToken))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("nonadmin lookup %v", err)
	}
	_, err = f.admin.SetNamespaceGrant(ctx, authenticated(&adminv1.SetNamespaceGrantRequest{Mutation: mutation(), Namespace: "team", SubjectKind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectId: target, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, Granted: true}, token))
	if err != nil {
		t.Fatal(err)
	}
	access, err := f.admin.GetNamespaceAccess(ctx, authenticated(&adminv1.GetNamespaceAccessRequest{Namespace: "team"}, targetToken))
	if err != nil || !access.Msg.CanPublish || access.Msg.CanReview || access.Msg.CanManage {
		t.Fatalf("maintainer capabilities %v %v", access, err)
	}
	actor, _ := f.store.Authenticate(ctx, f.owner)
	release, err := f.store.SubmitRelease(ctx, actor, "other-publisher", hub.SubmitRelease{Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "review-check", "Other publisher"), Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.publishing.YankRelease(ctx, authenticated(&publishingv1.YankReleaseRequest{Mutation: mutation(), ReleaseId: release.ID, Reason: "Remove invalid release"}, targetToken))
	if err != nil {
		t.Fatalf("maintainer cannot withdraw other publisher: %v", err)
	}
}

func TestPasswordAndAdministratorLifecycle(t *testing.T) {
	f := registry(t, false)
	ctx := context.Background()
	id, token := f.user(t, "member")
	_, err := f.auth.ChangePassword(ctx, authenticated(&authv1.ChangePasswordRequest{CurrentPassword: "wrong", NewPassword: "new-long-password"}, token))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal(err)
	}
	_, err = f.auth.ChangePassword(ctx, authenticated(&authv1.ChangePasswordRequest{CurrentPassword: "a-user-test-password", NewPassword: "new-long-password"}, token))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.auth.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, token)); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("old session survived %v", err)
	}
	loginUser := func(password string) string {
		t.Helper()
		v, e := f.auth.Login(ctx, connect.NewRequest(&authv1.LoginRequest{Username: "member", Password: password}))
		if e != nil {
			t.Fatal(e)
		}
		return v.Msg.AccessToken
	}
	token = loginUser("new-long-password")
	_, err = f.admin.ResetUserPassword(ctx, authenticated(&adminv1.ResetUserPasswordRequest{Mutation: mutation(), UserId: id, Password: "reset-long-password"}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.auth.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, token)); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("reset session survived %v", err)
	}
	token = loginUser("reset-long-password")
	owner, err := f.auth.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.admin.SetUserAdministrator(ctx, authenticated(&adminv1.SetUserAdministratorRequest{Mutation: mutation(), UserId: owner.Msg.Identity.UserId}, f.owner))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("last admin removed %v", err)
	}
	_, err = f.admin.SetUserAdministrator(ctx, authenticated(&adminv1.SetUserAdministratorRequest{Mutation: mutation(), UserId: id, Administrator: true}, f.owner))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.auth.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, token)); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("promotion session survived %v", err)
	}
	token = loginUser("reset-long-password")
	_, err = f.admin.SetUserAdministrator(ctx, authenticated(&adminv1.SetUserAdministratorRequest{Mutation: mutation(), UserId: owner.Msg.Identity.UserId}, token))
	if err != nil {
		t.Fatalf("handoff %v", err)
	}
	if _, err = f.auth.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, f.owner)); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("old admin session survived %v", err)
	}
}

func TestTrustedProxySeparatesAuthBucketsAndRejectsSpoofing(t *testing.T) {
	f := registry(t, false)
	for _, trusted := range []bool{false, true} {
		config := Config{PublicURL: "https://skills.example.com"}
		if trusted {
			config.TrustedProxies = []string{"127.0.0.1/32"}
		}
		s, err := New(f.store, config)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 121; i++ {
			req := httptest.NewRequest(http.MethodPost, "https://skills.example.com/api.auth.v1.AuthService/GetIdentity", strings.NewReader("{}"))
			req.RemoteAddr = "127.0.0.1:50000"
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i%5+1))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Connect-Protocol-Version", "1")
			req.Header.Set("Authorization", "Bearer "+f.owner)
			out := httptest.NewRecorder()
			s.ServeHTTP(out, req)
			want := 200
			if !trusted && i == 120 {
				want = 429
			}
			if out.Code != want {
				t.Fatalf("trusted=%v request=%d got=%d want=%d", trusted, i, out.Code, want)
			}
		}
	}
	s := &Server{proxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}
	for _, tc := range []struct{ peer, chain, want string }{{"127.0.0.1:12", "198.51.100.1, 10.1.1.1", "10.1.1.1"}, {"10.1.1.1:12", "198.51.100.1", "10.1.1.1"}, {"127.0.0.1:12", "bad-ip", "127.0.0.1"}} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Forwarded-For", tc.chain)
		if got := s.clientAddress(r); got != tc.want {
			t.Fatalf("address %q want %q", got, tc.want)
		}
	}
}

func TestSignInReturnsToExactSafeRoute(t *testing.T) {
	for _, raw := range []string{"https://evil.example/", "//evil.example/", "/logout", "/\\evil.example", "/%2fexample", "/\n"} {
		if returnPath(raw) != "/" {
			t.Fatalf("unsafe redirect %q", raw)
		}
	}
	route := "/#skill?namespace=team&name=review-check&tab=discussion"
	if returnPath(route) != route {
		t.Fatal("lost detail route")
	}
	server, _ := testServer(t)
	browser := *server.Client()
	browser.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := browser.Get(server.URL + "/login?next=" + url.QueryEscape(route))
	if err != nil {
		t.Fatal(err)
	}
	csrf := csrfCookie(t, response)
	response.Body.Close()
	form := url.Values{"csrf": {csrf.Value}, "username": {"admin"}, "password": {"a-strong-test-password"}, "next": {route}}
	r, _ := http.NewRequest("POST", server.URL+"/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "http://127.0.0.1")
	r.AddCookie(csrf)
	response, err = browser.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 303 || response.Header.Get("Location") != route {
		t.Fatalf("login redirect: %d %q", response.StatusCode, response.Header.Get("Location"))
	}
}
