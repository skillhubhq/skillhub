package hubhttp

import (
	"context"
	"github.com/skillhubhq/skillhub/internal/hubpolicy"
	"github.com/skillhubhq/skillhub/internal/identityprovider"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type verifiedAdapter struct {
	calls    int
	identity identityprovider.Identity
}

func (a *verifiedAdapter) AuthorizationURL(state, nonce, verifier string) string {
	return "https://identity.example/authorize?state=" + url.QueryEscape(state)
}
func (a *verifiedAdapter) Exchange(context.Context, string, string, string) (identityprovider.Identity, error) {
	a.calls++
	return a.identity, nil
}
func (a *verifiedAdapter) Avatar(context.Context, identityprovider.Identity) ([]byte, error) {
	return nil, nil
}
func TestExternalLoginUsesProviderConfigurationAndMode(t *testing.T) {
	for _, mode := range []string{hubpolicy.Enterprise, hubpolicy.Community} {
		t.Run(mode, func(t *testing.T) {
			store, e := testDatabase(t, t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			defer store.Close()
			p := hubpolicy.Policy{Mode: mode, Limits: hubpolicy.Defaults()}
			server, e := New(store, Config{PublicURL: "http://localhost", Policy: p, Providers: []identityprovider.Config{{ID: "social", Type: "github", Name: "Community account", ClientID: "client", Secret: "secret"}, {ID: "second", Type: "github", Name: "Another account", ClientID: "client2", Secret: "secret2"}}})
			if e != nil {
				t.Fatal(e)
			}
			a := &verifiedAdapter{identity: identityprovider.Identity{Authority: "https://identity.example", Subject: "one", DisplayName: "One"}}
			server.providers["social"].adapter = a
			server.providers["second"].adapter = &verifiedAdapter{identity: identityprovider.Identity{Authority: "https://another.example", Subject: "one", DisplayName: "One"}}
			page := httptest.NewRecorder()
			server.ServeHTTP(page, httptest.NewRequest("GET", "http://localhost/login", nil))
			body := page.Body.String()
			if !strings.Contains(body, "Community account") || !strings.Contains(body, "Another account") || strings.Contains(body, "Continue with GitHub") || strings.Contains(body, "/auth/github/") {
				t.Fatal("hardcoded provider UI")
			}
			start := httptest.NewRecorder()
			server.ServeHTTP(start, httptest.NewRequest("GET", "http://localhost/auth/providers/social/start?next=https://outside.example", nil))
			target, _ := url.Parse(start.Header().Get("Location"))
			state := target.Query().Get("state")
			cookie := start.Result().Cookies()[0]
			callback := func(provider, state string) *httptest.ResponseRecorder {
				r := httptest.NewRequest("GET", "http://localhost/auth/providers/"+provider+"/callback?state="+url.QueryEscape(state)+"&code=verified", nil)
				r.AddCookie(cookie)
				w := httptest.NewRecorder()
				server.ServeHTTP(w, r)
				return w
			}
			if callback("second", state).Code != 401 || a.calls != 0 {
				t.Fatal("cross-provider state accepted")
			}
			result := callback("social", state)
			want := 403
			if mode == hubpolicy.Community {
				want = 200
			}
			if result.Code != want || a.calls != 1 {
				t.Fatal("admission differs from mode", result.Code, result.Body.String())
			}
			if strings.Contains(result.Body.String(), "outside.example") {
				t.Fatal("open redirect")
			}
			if mode == hubpolicy.Community {
				token := ""
				for _, v := range result.Result().Cookies() {
					if v.Name == server.sessionCookieName() {
						token = v.Value
					}
				}
				u, e := store.Authenticate(context.Background(), token)
				if e != nil || u.Administrator {
					t.Fatal("invalid ordinary session", e)
				}
				v, e := store.GetParticipation(context.Background(), u)
				if e != nil || v.PersonalNamespace == "" {
					t.Fatal("missing personal space", e)
				}
			}
			if callback("social", state).Code != 401 || a.calls != 1 {
				t.Fatal("callback replay")
			}
		})
	}
}

func TestCommunityRequestLimitSharesRPCBudget(t *testing.T) {
	store, e := testDatabase(t, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	policy := hubpolicy.Policy{Mode: hubpolicy.Community, Limits: hubpolicy.Defaults()}
	policy.Limits.RequestsPerMinute = 1
	server, e := New(store, Config{PublicURL: "http://localhost", Policy: policy})
	if e != nil {
		t.Fatal(e)
	}
	request := func(path, address string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost"+path, strings.NewReader("{}"))
		r.RemoteAddr = address
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	if w := request("/api.registry.v1.RegistryService/GetRegistry", "192.0.2.1:1000"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request("/api.catalog.v1.CatalogService/SearchSkills", "192.0.2.1:2000"); w.Code != 429 || w.Header().Get("Retry-After") != "60" {
		t.Fatal("RPC bypassed shared budget", w.Code)
	}
	if w := request("/api.registry.v1.RegistryService/GetRegistry", "192.0.2.2:1000"); w.Code != 200 {
		t.Fatal("independent client blocked", w.Code)
	}
}

func TestProviderMessageLanguageAndSafeReturn(t *testing.T) {
	for _, language := range []string{"en", "zh"} {
		for _, status := range []int{http.StatusOK, http.StatusForbidden, http.StatusUnauthorized, http.StatusTooManyRequests} {
			r := httptest.NewRequest("GET", "/auth/providers/social/callback", nil)
			r.AddCookie(&http.Cookie{Name: "skillhub-language", Value: language})
			w := httptest.NewRecorder()
			providerMessage(w, r, status, [2]string{"Test title", "测试标题"}, [2]string{"Test message", "测试说明"}, "https://untrusted.example/")
			body := w.Body.String()
			if w.Code != status || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(body, "untrusted.example") || !strings.Contains(body, `href="/"`) {
				t.Fatal("invalid result page or unsafe return", status, language)
			}
			if language == "zh" && (strings.Contains(body, "Test title") || !strings.Contains(body, "测试标题")) {
				t.Fatal("mixed Chinese result page")
			}
			if language == "en" && (strings.Contains(body, "测试标题") || !strings.Contains(body, "Test title")) {
				t.Fatal("mixed English result page")
			}
		}
	}
}
