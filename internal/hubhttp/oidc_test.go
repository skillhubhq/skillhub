package hubhttp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/skillhubhq/skillhub/internal/hub"

	"github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"
)

// 使用真实发现文档、JWKS 与签名令牌验证回调边界；不跳过 TLS 或 JWT 校验。
func TestOIDCCodeFlowBoundaries(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	wrongKey, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	var provider *httptest.Server
	var nonce, challenge, variant string
	provider = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": provider.URL, "authorization_endpoint": provider.URL + "/authorize", "token_endpoint": provider.URL + "/token", "jwks_uri": provider.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			if e := r.ParseForm(); e != nil {
				t.Error(e)
			}
			if r.Form.Get("code") != "test-code" || r.Form.Get("redirect_uri") != "http://localhost/auth/oidc/callback" || r.Form.Get("code_verifier") == "" || oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != challenge {
				http.Error(w, "invalid authorization", 400)
				return
			}
			claims := map[string]any{"iss": provider.URL, "aud": "test-client", "sub": "stable-subject", "nonce": nonce, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "name": "Enterprise Person"}
			signingKey := key
			switch variant {
			case "nonce":
				claims["nonce"] = "wrong"
			case "issuer":
				claims["iss"] = "https://other.example"
			case "audience":
				claims["aud"] = "other-client"
			case "expired":
				claims["exp"] = time.Now().Add(-time.Hour).Unix()
			case "signature":
				signingKey = wrongKey
			}
			signer, e := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: signingKey}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
			if e != nil {
				t.Error(e)
				return
			}
			body, _ := json.Marshal(claims)
			signed, e := signer.Sign(body)
			if e != nil {
				t.Error(e)
				return
			}
			raw, e := signed.CompactSerialize()
			if e != nil {
				t.Error(e)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "not-stored", "token_type": "Bearer", "id_token": raw})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	store, e := testDatabase(t, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	admin, e := store.Bootstrap(context.Background(), "admin", "a-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	server, e := New(store, Config{PublicURL: "http://localhost", OIDC: &OIDCConfig{Issuer: provider.URL, ClientID: "test-client", ClientSecret: "test-secret", DisplayName: "Microsoft", AllowSignup: true}, oidcHTTPClient: provider.Client()})
	if e != nil {
		t.Fatal(e)
	}
	destination := "authorize"
	start := func() (string, *http.Cookie) {
		req := httptest.NewRequest("GET", "http://localhost/auth/oidc/start?next="+url.QueryEscape(destination), nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		if w.Code != 302 {
			t.Fatalf("start: %d", w.Code)
		}
		redirect, _ := url.Parse(w.Header().Get("Location"))
		q := redirect.Query()
		nonce = q.Get("nonce")
		challenge = q.Get("code_challenge")
		if q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" || nonce == "" {
			t.Fatal("missing PKCE/nonce/code flow")
		}
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
			t.Fatal("unsafe callback cookie")
		}
		return q.Get("state"), cookies[0]
	}
	callback := func(state string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://localhost/auth/oidc/callback?code=test-code&state="+url.QueryEscape(state), nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		return w
	}
	for _, v := range []string{"nonce", "issuer", "audience", "expired", "signature", "cookie", "state", "timeout"} {
		t.Run(v, func(t *testing.T) {
			state, cookie := start()
			variant = v
			if v == "cookie" {
				cookie.Value = "other-browser"
			}
			if v == "state" {
				state = "invalid"
			}
			if v == "timeout" {
				a := server.oidc.attempts[state]
				a.expires = time.Now().Add(-time.Second)
				server.oidc.attempts[state] = a
			}
			if w := callback(state, cookie); w.Code != 401 {
				t.Fatalf("accepted invalid %s: %d", v, w.Code)
			}
		})
	}
	variant = ""
	server.config.OIDC.AllowSignup = false
	pendingState, pendingCookie := start()
	pending := callback(pendingState, pendingCookie)
	if pending.Code != http.StatusForbidden || !strings.Contains(pending.Body.String(), "Waiting for approval") {
		t.Fatal("pending sign-in feedback", pending.Code)
	}
	for _, cookie := range pending.Result().Cookies() {
		if cookie.Name == server.sessionCookieName() {
			t.Fatal("unapproved identity received a session")
		}
	}
	users, err := store.ListUsers(context.Background(), admin, "", hub.Page{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, user := range users.Items {
		if user.ApprovalPending {
			found = true
			if _, err = store.SetUserDisabled(context.Background(), admin, "approve-oidc-test", hub.SetUserDisabled{ID: user.ID}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !found {
		t.Fatal("missing enterprise access request")
	}
	state, cookie := start()
	w := callback(state, cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `href="/authorize"`) {
		t.Fatalf("callback failed: %d %s", w.Code, w.Body.String())
	}
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "skillhub-session" {
			session = c
		}
	}
	if session == nil || !session.HttpOnly || session.SameSite != http.SameSiteStrictMode {
		t.Fatal("missing safe session")
	}
	identity, e := store.Authenticate(context.Background(), session.Value)
	if e != nil || identity.Administrator {
		t.Fatal("invalid provisioned identity", e)
	}
	if callback(state, cookie).Code != 401 {
		t.Fatal("callback replay accepted")
	}
	destination = "/#discover?skill=team/review-check&tab=discussion"
	state, cookie = start()
	w = callback(state, cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `href="/#discover?skill=team/review-check&amp;tab=discussion"`) {
		t.Fatal("SSO lost safe return route")
	}
	// 浏览器会话仍需显式提交设备码，并通过 Origin/CSRF 校验。
	device, e := store.StartDevice(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("GET", "http://localhost/authorize", nil)
	req.AddCookie(session)
	page := httptest.NewRecorder()
	server.ServeHTTP(page, req)
	if !strings.Contains(page.Body.String(), "Signed in as") || strings.Contains(page.Body.String(), `name="password"`) {
		t.Fatal("device form ignored SSO identity")
	}
	var csrf *http.Cookie
	for _, c := range page.Result().Cookies() {
		if c.Name == "skillhub-csrf" {
			csrf = c
		}
	}
	if csrf == nil {
		t.Fatal("no CSRF")
	}
	form := url.Values{"csrf": {csrf.Value}, "code": {device.UserCode}}
	post := func(origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost/authorize", strings.NewReader(form.Encode()))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(session)
		r.AddCookie(csrf)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	if post("https://other.example").Code != 403 {
		t.Fatal("cross-site device authorization")
	}
	if w := post("http://localhost"); w.Code != 200 || !strings.Contains(w.Body.String(), "Device authorized") {
		t.Fatal("SSO device approval failed")
	}
	native, e := store.CompleteDevice(context.Background(), device.DeviceCode)
	if e != nil || native == nil || native.Identity.ID != identity.ID {
		t.Fatal("wrong device identity", e)
	}
	req = httptest.NewRequest("GET", "http://localhost/login", nil)
	page = httptest.NewRecorder()
	server.ServeHTTP(page, req)
	body, _ := io.ReadAll(page.Result().Body)
	if !strings.Contains(string(body), "Continue with") || !strings.Contains(string(body), "Microsoft") {
		t.Fatal("no SSO sign-in option")
	}
}
func TestOIDCRejectsIncompleteConfiguration(t *testing.T) {
	store, e := testDatabase(t, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	for _, config := range []OIDCConfig{{Issuer: "http://example.com", ClientID: "client", ClientSecret: "secret"}, {Issuer: "https://example.com", ClientID: "client"}} {
		if _, e := New(store, Config{PublicURL: "http://localhost", OIDC: &config}); e == nil {
			t.Fatal("invalid provider configuration accepted")
		}
	}
}
