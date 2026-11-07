package hubhttp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/skillhubhq/skillhub/internal/hub"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig 由部署者配置；不接受浏览器传入 issuer 或重定向地址。
type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	DisplayName  string
	AllowSignup  bool
}
type oidcAttempt struct {
	nonce, verifier, browser, destination string
	expires                               time.Time
}
type oidcLogin struct {
	provider *oidc.Provider
	oauth    oauth2.Config
	client   *http.Client
	mu       sync.Mutex
	attempts map[string]oidcAttempt
}

func (s *Server) configureOIDC() error {
	c := s.config.OIDC
	if c == nil {
		return nil
	}
	u, err := url.Parse(c.Issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || c.ClientID == "" || c.ClientSecret == "" {
		return errors.New("OIDC requires an HTTPS issuer, client ID and client secret")
	}
	if c.DisplayName == "" {
		c.DisplayName = "Single sign-on"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	if s.config.oidcHTTPClient != nil {
		client = s.config.oidcHTTPClient
	}
	ctx, cancel := context.WithTimeout(oidc.ClientContext(context.Background(), client), 15*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, c.Issuer)
	if err != nil {
		return errors.New("OIDC discovery failed; check issuer and connectivity")
	}
	endpoint := provider.Endpoint()
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL} {
		u, e := url.Parse(raw)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return errors.New("OIDC endpoints must use HTTPS")
		}
	}
	s.oidc = &oidcLogin{provider: provider, client: client, oauth: oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: endpoint, RedirectURL: s.config.PublicURL + "/auth/oidc/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}, attempts: map[string]oidcAttempt{}}
	return nil
}
func oidcSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func (s *Server) oidcCookieName() string {
	if strings.HasPrefix(s.config.PublicURL, "https:") {
		return "__Host-skillhub-oidc"
	}
	return "skillhub-oidc"
}
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.oidc == nil {
		http.NotFound(w, r)
		return
	}
	destination := returnPath(r.URL.Query().Get("next"))
	state, browser := oidcSecret(), oidcSecret()
	o := s.oidc
	o.mu.Lock()
	for k, v := range o.attempts {
		if time.Now().After(v.expires) {
			delete(o.attempts, k)
		}
	}
	if len(o.attempts) >= 1000 {
		o.mu.Unlock()
		http.Error(w, "Sign-in busy. Try again later.", http.StatusTooManyRequests)
		return
	}
	attempt := oidcAttempt{nonce: oidcSecret(), verifier: oauth2.GenerateVerifier(), browser: browser, destination: destination, expires: time.Now().Add(5 * time.Minute)}
	o.attempts[state] = attempt
	o.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: s.oidcCookieName(), Value: browser, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.config.PublicURL, "https:"), SameSite: http.SameSiteLaxMode, MaxAge: 300})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, o.oauth.AuthCodeURL(state, oidc.Nonce(attempt.nonce), oauth2.S256ChallengeOption(attempt.verifier)), http.StatusFound)
}
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.oidc == nil {
		http.NotFound(w, r)
		return
	}
	fail := func() {
		http.Error(w, "Sign-in could not be completed. Return to /login and try again, or contact your Hub administrator.", http.StatusUnauthorized)
	}
	o := s.oidc
	o.mu.Lock()
	attempt, ok := o.attempts[r.URL.Query().Get("state")]
	delete(o.attempts, r.URL.Query().Get("state"))
	o.mu.Unlock()
	cookie, err := r.Cookie(s.oidcCookieName())
	http.SetCookie(w, &http.Cookie{Name: s.oidcCookieName(), Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.config.PublicURL, "https:"), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if !ok || time.Now().After(attempt.expires) || err != nil || subtle.ConstantTimeCompare([]byte(attempt.browser), []byte(cookie.Value)) != 1 || r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		fail()
		return
	}
	ctx := oidc.ClientContext(r.Context(), o.client)
	token, err := o.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(attempt.verifier))
	if err != nil {
		fail()
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		fail()
		return
	}
	verified, err := o.provider.Verifier(&oidc.Config{ClientID: o.oauth.ClientID}).Verify(ctx, raw)
	if err != nil || verified.Subject == "" || subtle.ConstantTimeCompare([]byte(verified.Nonce), []byte(attempt.nonce)) != 1 {
		fail()
		return
	}
	var claims struct {
		Name string `json:"name"`
	}
	if verified.Claims(&claims) != nil {
		fail()
		return
	}
	session, err := s.store.FederatedLogin(ctx, verified.Issuer, verified.Subject, claims.Name, s.config.OIDC.AllowSignup)
	if errors.Is(err, hub.ErrPendingApproval) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/assets/auth.css"><title>Access requested · SkillHub</title><main><h1>Waiting for approval</h1><p>Your enterprise identity was verified. A Hub administrator must approve your access before you can sign in.</p><p>企业身份已验证，访问申请正在等待 Hub 管理员批准。批准后请重新登录。</p><a href="/login">Return to sign in / 返回登录</a></main></html>`))
		return
	}
	if err != nil {
		fail()
		return
	}
	if old, e := r.Cookie(s.sessionCookieName()); e == nil {
		_ = s.store.Logout(ctx, old.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: s.sessionCookieName(), Value: session.Token, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.config.PublicURL, "https:"), SameSite: http.SameSiteStrictMode, Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds())})
	// 单独的同源落地页使 Strict 会话在后续客户端授权导航中可用。
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/assets/auth.css"><title>Signed in · SkillHub</title><main><h1>You're signed in</h1><p>Your enterprise identity is connected to this Hub.</p><a href="` + html.EscapeString(attempt.destination) + `">Continue to SkillHub</a></main></html>`))
}
