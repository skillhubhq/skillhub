package hubhttp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/identityprovider"
	"golang.org/x/oauth2"
	"html"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type loginAttempt struct {
	nonce, verifier, browser, destination string
	expires                               time.Time
}
type externalLogin struct {
	id, name, callbackPath string
	adapter                identityprovider.Adapter
	mu                     sync.Mutex
	attempts               map[string]loginAttempt
}
type loginOption struct{ Name, URL string }

func (s *Server) configureProviders() error {
	s.providers = map[string]*externalLogin{}
	callbacks := map[string]bool{}
	if len(s.config.Providers) > 8 {
		return errors.New("too many identity providers")
	}
	for _, c := range s.config.Providers {
		if _, ok := s.providers[c.ID]; ok {
			return errors.New("duplicate identity provider id")
		}
		callback := "/auth/providers/" + c.ID + "/callback"

		if callbacks[callback] {
			return errors.New("duplicate identity callback")
		}
		callbacks[callback] = true
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		adapter, err := identityprovider.New(ctx, c, s.config.PublicURL+callback, s.config.providerHTTPClient)
		cancel()
		if err != nil {
			return err
		}
		s.providers[c.ID] = &externalLogin{id: c.ID, name: c.Name, callbackPath: callback, adapter: adapter, attempts: map[string]loginAttempt{}}
		s.providerOrder = append(s.providerOrder, c.ID)
	}
	return nil
}
func (s *Server) registerProviderRoutes(mux *http.ServeMux) {
	for _, id := range s.providerOrder {
		p := s.providers[id]
		mux.HandleFunc("/auth/providers/"+id+"/start", func(w http.ResponseWriter, r *http.Request) { s.providerStart(p, w, r) })
		mux.HandleFunc(p.callbackPath, func(w http.ResponseWriter, r *http.Request) { s.providerCallback(p, w, r) })

	}
}
func (s *Server) loginOptions(next string) []loginOption {
	result := make([]loginOption, 0, len(s.providerOrder))
	for _, id := range s.providerOrder {
		result = append(result, loginOption{s.providers[id].name, "/auth/providers/" + id + "/start?next=" + url.QueryEscape(returnPath(next))})
	}
	return result
}
func loginSecret() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic("random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func (s *Server) providerCookieName(id string) string {
	prefix := ""
	if strings.HasPrefix(s.config.PublicURL, "https:") {
		prefix = "__Host-"
	}
	return prefix + "skillhub-provider-" + id
}
func (s *Server) providerStart(p *externalLogin, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	state, browser := loginSecret(), loginSecret()
	p.mu.Lock()
	for k, v := range p.attempts {
		if time.Now().After(v.expires) {
			delete(p.attempts, k)
		}
	}
	if len(p.attempts) >= 1000 {
		p.mu.Unlock()
		http.Error(w, "Sign-in busy. Try again later.", 429)
		return
	}
	a := loginAttempt{nonce: loginSecret(), verifier: oauth2.GenerateVerifier(), browser: browser, destination: returnPath(r.URL.Query().Get("next")), expires: time.Now().Add(5 * time.Minute)}
	p.attempts[state] = a
	p.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: s.providerCookieName(p.id), Value: browser, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.config.PublicURL, "https:"), SameSite: http.SameSiteLaxMode, MaxAge: 300})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, p.adapter.AuthorizationURL(state, a.nonce, a.verifier), http.StatusFound)
}
func (s *Server) providerCallback(p *externalLogin, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	fail := func() {
		providerMessage(w, r, http.StatusUnauthorized,
			[2]string{"Sign-in could not be completed", "未能完成登录"},
			[2]string{"Return to sign-in and try again. Your Hub access has not changed.", "请返回登录页重试，当前 Hub 权限未发生变化。"}, "/login")
	}
	p.mu.Lock()
	a, ok := p.attempts[r.URL.Query().Get("state")]
	delete(p.attempts, r.URL.Query().Get("state"))
	p.mu.Unlock()
	cookie, err := r.Cookie(s.providerCookieName(p.id))
	http.SetCookie(w, &http.Cookie{Name: s.providerCookieName(p.id), Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.config.PublicURL, "https:"), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if !ok || time.Now().After(a.expires) || err != nil || subtle.ConstantTimeCompare([]byte(a.browser), []byte(cookie.Value)) != 1 || r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		fail()
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	verified, err := p.adapter.Exchange(ctx, r.URL.Query().Get("code"), a.nonce, a.verifier)
	if err != nil {
		fail()
		return
	}
	session, err := s.store.FederatedLogin(ctx, verified.Authority, verified.Subject, verified.DisplayName, s.config.Policy.IsCommunity())
	if errors.Is(err, hub.ErrPendingApproval) {
		providerMessage(w, r, http.StatusForbidden,
			[2]string{"Waiting for approval", "等待管理员批准"},
			[2]string{"Your identity is verified. A Hub administrator must approve your access before you can sign in.", "身份已验证，Hub 管理员批准后即可登录。"}, "/")
		return
	}
	if errors.Is(err, hub.ErrQuota) {
		providerMessage(w, r, http.StatusTooManyRequests,
			[2]string{"Account limit reached", "账号数量已达上限"},
			[2]string{"This community cannot create another account. Contact the Hub operator.", "当前社区暂时无法创建新账号，请联系 Hub 维护者。"}, "/")
		return
	}
	if err != nil {
		fail()
		return
	}
	s.startAvatarSync(p.adapter, verified, session.Identity)
	s.completeFederatedLogin(w, r, session, a.destination)
}
func (s *Server) completeFederatedLogin(w http.ResponseWriter, r *http.Request, session hub.Session, destination string) {
	if old, e := r.Cookie(s.sessionCookieName()); e == nil {
		_ = s.store.Logout(r.Context(), old.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: s.sessionCookieName(), Value: session.Token, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.config.PublicURL, "https:"), SameSite: http.SameSiteStrictMode, Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds())})
	providerMessage(w, r, http.StatusOK, [2]string{"You're signed in", "登录成功"},
		[2]string{"Your identity is connected to this Hub. Continue to finish signing in.", "已登录当前 Hub，点击继续完成登录。"}, destination)
}

// 统一第三方回调反馈；只输出选定语言，跳转仍受同源路径约束。
func providerMessage(w http.ResponseWriter, r *http.Request, status int, title, message [2]string, destination string) {
	index, language := 0, "en"
	if cookie, err := r.Cookie("skillhub-language"); err == nil && cookie.Value == "zh" {
		index, language = 1, "zh"
	}
	action := [2]string{"Continue", "继续"}
	if status != http.StatusOK {
		action = [2]string{"Back to the community", "返回社区"}
	}
	if destination == "/login" {
		action = [2]string{"Return to sign-in", "返回登录页"}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`<!doctype html><html lang="` + language + `"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/assets/auth.css"><title>` + html.EscapeString(title[index]) + ` · SkillHub</title><main><h1>` + html.EscapeString(title[index]) + `</h1><p>` + html.EscapeString(message[index]) + `</p><a class="sso-button" href="` + html.EscapeString(returnPath(destination)) + `">` + action[index] + `</a></main></html>`))
}
