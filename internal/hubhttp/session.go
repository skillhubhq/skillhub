package hubhttp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/skillhubhq/skillhub/internal/hub"
)

func (s *Server) sessionCookieName() string {
	if strings.HasPrefix(s.config.PublicURL, "https:") {
		return "__Host-skillhub-session"
	}
	return "skillhub-session"
}

// 浏览器只从 HttpOnly 会话读取身份；自定义请求头和严格同源检查阻止跨站借用 Cookie。
func (s *Server) browserSession(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(s.sessionCookieName())
	if err != nil || !strings.HasPrefix(r.URL.Path, "/api.") {
		return true
	}
	if r.Header.Get("Authorization") != "" {
		return true
	}
	if r.Method != http.MethodPost || r.Header.Get("Origin") != s.config.PublicURL || r.Header.Get("X-SkillHub-Web") != "1" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		_ = connect.NewErrorWriter().Write(w, r, connect.NewError(connect.CodePermissionDenied, nil))
		return false
	}
	if len(cookie.Value) != 43 {
		_ = connect.NewErrorWriter().Write(w, r, publicError(hub.ErrUnauthenticated))
		return false
	}
	r.Header.Set("Authorization", "Bearer "+cookie.Value)
	return true
}

var sessionPage = template.Must(template.New("session").Parse(`<!doctype html><html lang="{{.Language}}"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" type="image/svg+xml" href="/assets/brand/favicon.svg"><script src="/assets/preferences-init.js"></script><title>{{.Labels.SignIn}} · {{.Name}}</title><style>body{font:16px/1.6 system-ui,sans-serif;background:#f8f9fb;color:#24272e;padding:8vh 20px;margin:0}main{max-width:400px;margin:auto;background:white;border:1px solid #e5e7eb;border-radius:8px;padding:40px}h1{font-size:24px}label{display:block;margin:20px 0 6px}input,button{box-sizing:border-box;width:100%;font:inherit;padding:12px;border:1px solid #c4cddc;border-radius:6px}button{background:#252a34;color:white;margin-top:24px;cursor:pointer}a{color:#252a34}.error{color:#992d37}</style><link rel="stylesheet" href="/assets/auth.css"><main><img class="brand-light" src="/assets/brand/logo-color.svg" alt="SkillHub" width="145"><img class="brand-dark" src="/assets/brand/logo-white.svg" alt="SkillHub" width="145"><h1>{{.Name}}</h1><p>{{if .Logout}}{{.Labels.Leave}}{{else}}{{.Labels.Intro}}{{end}}</p>{{if .Error}}<p class="error" role="alert">{{.Error}}</p>{{end}}{{if and .Providers (not .Logout)}}{{range .Providers}}<p><a class="sso-button" href="{{.URL}}">{{$.Labels.Continue}} {{.Name}}</a></p>{{end}}<details class="local-login"><summary>{{.Labels.Local}}</summary>{{end}}<form method="post"><input type="hidden" name="next" value="{{.Next}}"><input type="hidden" name="csrf" value="{{.CSRF}}">{{if not .Logout}}<label for="username">{{.Labels.Username}}</label><input id="username" name="username" autocomplete="username" required maxlength="128"><label for="password">{{.Labels.Password}}</label><input id="password" name="password" type="password" autocomplete="current-password" required maxlength="1024">{{end}}<button>{{if .Logout}}{{.Labels.SignOut}}{{else}}{{.Labels.SignIn}}{{end}}</button></form>{{if and .Providers (not .Logout)}}</details>{{end}}<p><a href="/">{{.Labels.Back}}</a></p><p class="languages"><a href="{{.EnglishURL}}">English</a> · <a href="{{.ChineseURL}}">简体中文</a></p></main></html>`))

// 会话页面只处理浏览器认证，不新增第二套业务 API。
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// 表单使用同源 Referrer 策略，避免浏览器把 POST Origin 降为 null。
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	secure := strings.HasPrefix(s.config.PublicURL, "https:")
	csrfName := "skillhub-web-csrf"
	if secure {
		csrfName = "__Host-skillhub-web-csrf"
	}
	language := r.URL.Query().Get("lang")
	if language == "" {
		if cookie, err := r.Cookie("skillhub-language"); err == nil {
			language = cookie.Value
		}
	}
	labels := map[string]string{"Continue": "Continue with", "Local": "Use a local Hub account", "SignIn": "Sign in", "SignOut": "Sign out", "Username": "Username", "Password": "Password", "Intro": "Welcome back. Sign in to share, learn and work together.", "Leave": "Sign out of this hub?", "Back": "Back to the community", "Error": "Unable to sign in. Check your username and password."}
	if language == "zh" {
		labels = map[string]string{"Continue": "使用以下账号登录：", "Local": "使用本地 Hub 账号", "SignIn": "登录", "SignOut": "确认退出", "Username": "用户名", "Password": "密码", "Intro": "登录当前 Hub，继续浏览、发布与协作。", "Leave": "退出当前 Hub 账号？", "Back": "返回技能社区", "Error": "无法登录，请检查账号和密码。"}
	} else {
		language = "en"
	}
	// 语言不是认证凭据；Lax 允许第三方登录回调继续使用用户选择的语言。
	http.SetCookie(w, &http.Cookie{Name: "skillhub-language", Value: language, Path: "/", SameSite: http.SameSiteLaxMode, Secure: secure, MaxAge: 365 * 24 * 60 * 60})
	next := returnPath(r.URL.Query().Get("next"))
	logout := r.URL.Path == "/logout"
	message := ""
	status := http.StatusOK
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		next = returnPath(r.PostForm.Get("next"))
		cookie, err := r.Cookie(csrfName)
		if r.Header.Get("Origin") != s.config.PublicURL || r.Header.Get("Sec-Fetch-Site") == "cross-site" || err != nil || len(cookie.Value) != 43 || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(r.PostForm.Get("csrf"))) != 1 {
			s.config.Logger.Warn("browser session form rejected", "origin_matches", r.Header.Get("Origin") == s.config.PublicURL, "origin_missing", r.Header.Get("Origin") == "", "cookie_present", err == nil, "form_token_length", len(r.PostForm.Get("csrf")))
			http.Error(w, "invalid session form", http.StatusForbidden)
			return
		}
		if logout {
			if current, e := r.Cookie(s.sessionCookieName()); e == nil {
				_ = s.store.Logout(r.Context(), current.Value)
			}
			http.SetCookie(w, &http.Cookie{Name: s.sessionCookieName(), Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		value, e := s.store.Login(r.Context(), r.PostForm.Get("username"), r.PostForm.Get("password"))
		if e != nil {
			message = labels["Error"]
			status = http.StatusUnauthorized
		} else {
			if old, e := r.Cookie(s.sessionCookieName()); e == nil {
				_ = s.store.Logout(r.Context(), old.Value)
			}
			http.SetCookie(w, &http.Cookie{Name: s.sessionCookieName(), Value: value.Token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, Expires: value.ExpiresAt, MaxAge: int(time.Until(value.ExpiresAt).Seconds())})
			http.SetCookie(w, &http.Cookie{Name: csrfName, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
			http.Redirect(w, r, returnPath(r.PostForm.Get("next")), http.StatusSeeOther)
			return
		}
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		http.Error(w, "session unavailable", 500)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(random[:])
	http.SetCookie(w, &http.Cookie{Name: csrfName, Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: 600})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = sessionPage.Execute(w, struct {
		Name, CSRF, Error, Next, EnglishURL, ChineseURL, Language string
		Providers                                                 []loginOption
		Labels                                                    map[string]string
		Logout                                                    bool
	}{Name: s.config.DisplayName, CSRF: token, Error: message, Next: next,
		EnglishURL: "?lang=en&next=" + url.QueryEscape(next), ChineseURL: "?lang=zh&next=" + url.QueryEscape(next),
		Providers: s.loginOptions(next), Language: language, Labels: labels, Logout: logout})
}
