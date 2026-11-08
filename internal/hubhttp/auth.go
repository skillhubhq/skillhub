package hubhttp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"html/template"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/skillhubhq/skillhub/internal/hub"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func identityDTO(user hub.Identity) *authv1.Identity {
	return &authv1.Identity{UserId: user.ID, Username: user.Username, Administrator: user.Administrator, Federated: user.Federated}
}
func sessionDTO(value hub.Session) *authv1.LoginResponse {
	return &authv1.LoginResponse{AccessToken: value.Token, Identity: identityDTO(value.Identity), ExpiresAt: timestamppb.New(value.ExpiresAt)}
}

func (s *Server) Login(ctx context.Context, req *connect.Request[authv1.LoginRequest]) (*connect.Response[authv1.LoginResponse], error) {
	value, err := s.store.Login(ctx, req.Msg.Username, req.Msg.Password)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(sessionDTO(value)), nil
}
func (s *Server) GetIdentity(ctx context.Context, req *connect.Request[authv1.GetIdentityRequest]) (*connect.Response[authv1.GetIdentityResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&authv1.GetIdentityResponse{Identity: identityDTO(user)}), nil
}
func (s *Server) Logout(ctx context.Context, req *connect.Request[authv1.LogoutRequest]) (*connect.Response[authv1.LogoutResponse], error) {
	token, err := bearer(req.Header())
	if err != nil {
		return nil, err
	}
	if err = s.store.Logout(ctx, token); err != nil {
		return nil, err
	}
	return connect.NewResponse(&authv1.LogoutResponse{}), nil
}
func (s *Server) StartDeviceLogin(ctx context.Context, req *connect.Request[authv1.StartDeviceLoginRequest]) (*connect.Response[authv1.StartDeviceLoginResponse], error) {
	value, err := s.store.StartDevice(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&authv1.StartDeviceLoginResponse{DeviceCode: value.DeviceCode, UserCode: value.UserCode, VerificationUri: s.config.PublicURL + "/authorize", ExpiresAt: timestamppb.New(value.ExpiresAt), IntervalSeconds: 2}), nil
}
func (s *Server) CompleteDeviceLogin(ctx context.Context, req *connect.Request[authv1.CompleteDeviceLoginRequest]) (*connect.Response[authv1.CompleteDeviceLoginResponse], error) {
	value, err := s.store.CompleteDevice(ctx, req.Msg.DeviceCode)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return connect.NewResponse(&authv1.CompleteDeviceLoginResponse{Pending: true}), nil
	}
	return connect.NewResponse(&authv1.CompleteDeviceLoginResponse{Login: sessionDTO(*value)}), nil
}

var authorizationPage = template.Must(template.New("authorize").Parse(`<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Authorize · {{.Name}}</title>
<style>body{font:16px system-ui,sans-serif;background:#f5f7fa;color:#182235;margin:0;padding:10vh 24px}main{max-width:420px;margin:auto;padding:32px;background:white;border:1px solid #dfe4ec;border-radius:16px}h1{font-size:24px}label{display:block;margin:20px 0 6px}input,button{box-sizing:border-box;width:100%;font:inherit;border-radius:8px;padding:12px;border:1px solid #bac3d1}button{margin-top:24px;background:#244cba;color:white;border:0;cursor:pointer}p{line-height:1.5}.error{color:#a12020}</style>
<main><h1>{{.Name}}</h1>{{if .Done}}<p>Device authorized. Return to your SkillHub client.</p>{{else}}<p>Authorize your SkillHub client. Enter the code shown in the client you started.</p>{{if .Error}}<p class="error" role="alert">{{.Error}}</p>{{end}}<form method="post" action="/authorize">
<input type="hidden" name="csrf" value="{{.CSRF}}"><label for="code">Device code</label><input id="code" name="code" required maxlength="8" autocomplete="off">
{{if .SignedIn}}<p>Signed in as <strong>{{.Username}}</strong>. Only authorize a code shown in a client you started.</p>{{else}}{{range .Providers}}<p><a href="{{.URL}}">Continue with {{.Name}}</a></p>{{end}}{{if .Providers}}<p>Or use a local Hub account:</p>{{end}}<label for="username">Username</label><input id="username" name="username" required maxlength="128" autocomplete="username">
<label for="password">Password</label><input id="password" name="password" type="password" required maxlength="1024" autocomplete="current-password">{{end}}
<button type="submit">Authorize this device</button></form>{{end}}</main></html>`))

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	secure := strings.HasPrefix(s.config.PublicURL, "https:")
	cookieName := "skillhub-csrf"
	if secure {
		cookieName = "__Host-skillhub-csrf"
	}
	var sessionToken, username string
	if c, e := r.Cookie(s.sessionCookieName()); e == nil {
		if user, e := s.store.Authenticate(r.Context(), c.Value); e == nil {
			sessionToken = c.Value
			username = user.Username
		}
	}
	var token string
	var message string
	done := false
	status := http.StatusOK
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		cookie, err := r.Cookie(cookieName)
		origin := r.Header.Get("Origin")
		if (origin != "" && origin != s.config.PublicURL) || r.Header.Get("Sec-Fetch-Site") == "cross-site" || err != nil || len(cookie.Value) != 43 || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(r.PostForm.Get("csrf"))) != 1 {
			http.Error(w, "invalid authorization form", http.StatusForbidden)
			return
		}
		if sessionToken != "" {
			err = s.store.ApproveDeviceSession(r.Context(), strings.TrimSpace(r.PostForm.Get("code")), sessionToken)
		} else {
			err = s.store.ApproveDevice(r.Context(), strings.TrimSpace(r.PostForm.Get("code")), r.PostForm.Get("username"), r.PostForm.Get("password"))
		}
		if err != nil {
			message = "Unable to authorize. Check your credentials and the current device code."
			status = http.StatusBadRequest
		} else {
			done = true
		}
	}
	var random [32]byte
	_, _ = rand.Read(random[:])
	token = base64.RawURLEncoding.EncodeToString(random[:])
	maxAge := 600
	if done {
		maxAge = -1
		token = ""
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = authorizationPage.Execute(w, struct {
		Name, CSRF, Error, Username string
		Providers                   []loginOption
		SignedIn, Done              bool
	}{Name: s.config.DisplayName, CSRF: token, Error: message, Username: username,
		Providers: s.loginOptions("authorize"), SignedIn: sessionToken != "", Done: done})
}

func (s *Server) ChangePassword(ctx context.Context, req *connect.Request[authv1.ChangePasswordRequest]) (*connect.Response[authv1.ChangePasswordResponse], error) {
	user, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	if err = s.store.ChangePassword(ctx, user, req.Msg.CurrentPassword, req.Msg.NewPassword); err != nil {
		return nil, err
	}
	return connect.NewResponse(&authv1.ChangePasswordResponse{}), nil
}
