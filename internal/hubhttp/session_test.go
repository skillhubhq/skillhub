package hubhttp

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"connectrpc.com/connect"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	governancev1 "github.com/skillhubhq/skillhub/gen/api/governance/v1"
	"github.com/skillhubhq/skillhub/gen/api/governance/v1/governancev1connect"
)

func TestWebSessionOriginIsolationAndLogout(t *testing.T) {
	server, client := testServer(t)
	browser := *server.Client()
	browser.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	get, err := browser.Get(server.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	if get.Header.Get("Referrer-Policy") != "same-origin" {
		t.Fatal("login form must preserve same-origin POST Origin")
	}
	csrf := csrfCookie(t, get)
	_ = get.Body.Close()
	if !csrf.HttpOnly || csrf.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe csrf cookie")
	}
	post := func(path, origin, token string, cookies ...*http.Cookie) *http.Response {
		form := url.Values{"csrf": {token}, "username": {"admin"}, "password": {"a-strong-test-password"}}
		req, _ := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", origin)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		response, e := browser.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		return response
	}
	bad := post("/login", "https://other.example", csrf.Value, csrf)
	_ = bad.Body.Close()
	if bad.StatusCode != 403 {
		t.Fatal("cross origin login accepted")
	}
	bad = post("/login", "http://127.0.0.1", "wrong", csrf)
	_ = bad.Body.Close()
	if bad.StatusCode != 403 {
		t.Fatal("invalid csrf accepted")
	}
	response := post("/login", "http://127.0.0.1", csrf.Value, csrf)
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 303 {
		t.Fatalf("login status %d", response.StatusCode)
	}
	var session *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == "skillhub-session" {
			session = cookie
		}
	}
	if session == nil || !session.HttpOnly || session.SameSite != http.SameSiteStrictMode || session.Path != "/" || strings.Contains(string(body), session.Value) {
		t.Fatal("unsafe browser session")
	}
	identity := func(origin, marker string) error {
		req := connect.NewRequest(&authv1.GetIdentityRequest{})
		req.Header().Set("Cookie", session.String())
		req.Header().Set("Origin", origin)
		req.Header().Set("X-SkillHub-Web", marker)
		_, e := client.GetIdentity(context.Background(), req)
		return e
	}
	if err = identity("http://127.0.0.1", "1"); err != nil {
		t.Fatal(err)
	}
	for _, test := range [][2]string{{"https://other.example", "1"}, {"http://127.0.0.1", ""}, {"", "1"}} {
		if e := identity(test[0], test[1]); connect.CodeOf(e) != connect.CodePermissionDenied {
			t.Fatalf("cross site cookie request: %v", e)
		}
	}
	// 新治理 RPC 也必须经过同源 Cookie 防护，不能因为服务新增而绕开。
	governance := governancev1connect.NewGovernanceServiceClient(server.Client(), server.URL)
	for _, origin := range []string{"https://other.example", "http://127.0.0.1"} {
		request := connect.NewRequest(&governancev1.GetPlatformAccessRequest{})
		request.Header().Set("Cookie", session.String())
		request.Header().Set("Origin", origin)
		request.Header().Set("X-SkillHub-Web", "1")
		_, e := governance.GetPlatformAccess(context.Background(), request)
		if origin == "https://other.example" && connect.CodeOf(e) != connect.CodePermissionDenied {
			t.Fatal("governance accepted cross-origin cookie", e)
		}
		if origin == "http://127.0.0.1" && e != nil {
			t.Fatal("governance rejected valid web session", e)
		}
	}
	// 独立 CLI 令牌不依赖 Cookie/Origin；浏览器退出只吊销浏览器会话。
	cliToken := login(t, client)
	get, err = browser.Get(server.URL + "/logout")
	if err != nil {
		t.Fatal(err)
	}
	csrf = csrfCookie(t, get)
	_ = get.Body.Close()
	response = post("/logout", "http://127.0.0.1", csrf.Value, csrf, session)
	_ = response.Body.Close()
	if response.StatusCode != 303 {
		t.Fatal("logout failed")
	}
	if e := identity("http://127.0.0.1", "1"); connect.CodeOf(e) != connect.CodeUnauthenticated {
		t.Fatalf("revoked browser session: %v", e)
	}
	if _, e := client.GetIdentity(context.Background(), authenticated(&authv1.GetIdentityRequest{}, cliToken)); e != nil {
		t.Fatal("browser logout revoked CLI", e)
	}
}

func csrfCookie(t *testing.T, response *http.Response) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if cookie.Name == "skillhub-web-csrf" {
			return cookie
		}
	}
	t.Fatal("missing CSRF cookie")
	return nil
}
func TestWebSessionLanguages(t *testing.T) {
	server, _ := testServer(t)
	for _, test := range []struct{ path, language, text string }{{"/login", "en", "Sign in"}, {"/login?lang=zh", "zh", "登录"}} {
		response, err := server.Client().Get(server.URL + test.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if !strings.Contains(string(body), `lang="`+test.language+`"`) || !strings.Contains(string(body), test.text) {
			t.Fatalf("incorrect language for %s", test.path)
		}
	}
}
