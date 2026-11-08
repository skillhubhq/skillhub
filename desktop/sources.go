package main

import (
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type IdentityView struct {
	Federated     bool   `json:"federated"`
	ID            string `json:"id"`
	Username      string `json:"username"`
	Administrator bool   `json:"administrator"`
}

func identityView(i *authv1.Identity) IdentityView {
	if i == nil {
		return IdentityView{}
	}
	return IdentityView{ID: i.UserId, Username: i.Username, Administrator: i.Administrator, Federated: i.Federated}
}

type LoginView struct {
	Handle          string `json:"handle"`
	Code            string `json:"code"`
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
	ExpiresAt       string `json:"expires_at"`
}
type LoginPoll struct {
	Pending  bool         `json:"pending"`
	Identity IdentityView `json:"identity"`
}
type pendingLogin struct {
	login   client.DeviceLogin
	next    time.Time
	polling bool
}

func (s *DesktopService) ListSources() ([]client.Source, error) {
	s.officialOnce.Do(func() { _ = s.core.EnsureOfficialSource(s.ctx) })
	return s.core.ListSources(s.ctx)
}
func (s *DesktopService) AddSource(alias, url string) (client.Source, error) {
	return s.core.AddSource(s.ctx, alias, url)
}
func (s *DesktopService) RemoveSource(alias string) error { return s.core.RemoveSource(s.ctx, alias) }
func (s *DesktopService) Logout(alias string) error       { return s.core.Logout(s.ctx, alias) }
func (s *DesktopService) Identity(alias string) (IdentityView, error) {
	r, err := s.core.Remote(s.ctx, alias)
	if err != nil {
		return IdentityView{}, err
	}
	response, err := r.Auth.GetIdentity(s.ctx, connect.NewRequest(&authv1.GetIdentityRequest{}))
	if err != nil {
		return IdentityView{}, err
	}
	return identityView(response.Msg.Identity), nil
}
func (s *DesktopService) LoginPassword(alias, username, password string) (IdentityView, error) {
	identity, err := s.core.LoginPassword(s.ctx, alias, username, password)
	if err != nil {
		return IdentityView{}, err
	}
	return identityView(identity), nil
}
func (s *DesktopService) BeginLogin(alias string) (LoginView, error) {
	login, err := s.core.StartLogin(s.ctx, alias)
	if err != nil {
		return LoginView{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, p := range s.logins {
		if time.Now().After(p.login.ExpiresAt) || p.login.Source.Alias == alias {
			delete(s.logins, id)
		}
	}
	if len(s.logins) >= 4 {
		return LoginView{}, errors.New("too many pending logins")
	}
	id := uuid.NewString()
	s.logins[id] = &pendingLogin{login: login, next: time.Now().Add(login.Interval)}
	return LoginView{Handle: id, Code: login.UserCode, URL: login.VerificationURI, IntervalSeconds: int(login.Interval / time.Second), ExpiresAt: login.ExpiresAt.UTC().Format(time.RFC3339)}, nil
}
func (s *DesktopService) OpenLoginBrowser(id string) error {
	s.mu.Lock()
	pending, ok := s.logins[id]
	s.mu.Unlock()
	if !ok || time.Now().After(pending.login.ExpiresAt) {
		return errors.New("login request expired")
	}
	app := application.Get()
	if app == nil {
		return errors.New("native application is unavailable")
	}
	return app.Browser.OpenURL(pending.login.VerificationURI)
}
func (s *DesktopService) CancelLogin(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.logins, id)
}
func (s *DesktopService) PollLogin(id string) (LoginPoll, error) {
	s.mu.Lock()
	pending, ok := s.logins[id]
	if !ok || time.Now().After(pending.login.ExpiresAt) {
		delete(s.logins, id)
		s.mu.Unlock()
		return LoginPoll{}, errors.New("login request expired")
	}
	if pending.polling || time.Now().Before(pending.next) {
		s.mu.Unlock()
		return LoginPoll{Pending: true}, nil
	}
	pending.polling = true
	pending.next = time.Now().Add(pending.login.Interval)
	s.mu.Unlock()
	identity, waiting, err := s.core.CompleteLogin(s.ctx, pending.login)
	s.mu.Lock()
	pending.polling = false
	if err != nil || !waiting {
		delete(s.logins, id)
	}
	s.mu.Unlock()
	if err != nil {
		return LoginPoll{}, err
	}
	return LoginPoll{Pending: waiting, Identity: identityView(identity)}, nil
}

func (s *DesktopService) ChangePassword(source, current, password string) error {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return err
	}
	_, err = remote.Auth.ChangePassword(s.ctx, connect.NewRequest(&authv1.ChangePasswordRequest{CurrentPassword: current, NewPassword: password}))
	return err
}
