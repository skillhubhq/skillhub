package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"strconv"
)

type githubAdapter struct {
	oauth  oauth2.Config
	client *http.Client
}

func newGitHub(_ context.Context, c Config, callback string, client *http.Client) (Adapter, error) {
	return &githubAdapter{oauth2.Config{ClientID: c.ClientID, ClientSecret: c.Secret, RedirectURL: callback, Endpoint: oauth2.Endpoint{AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", AuthStyle: oauth2.AuthStyleInParams}}, client}, nil
}
func (g *githubAdapter) AuthorizationURL(state, _, verifier string) string {
	return g.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}
func (g *githubAdapter) Exchange(ctx context.Context, code, _, verifier string) (Identity, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, g.client)
	token, e := g.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if e != nil {
		return Identity{}, errors.New("identity exchange failed")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if e != nil {
		return Identity{}, e
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SkillHub")
	res, e := g.client.Do(req)
	if e != nil {
		return Identity{}, errors.New("identity lookup failed")
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, (128<<10)+1))
	if e != nil || res.StatusCode != 200 || len(raw) > 128<<10 {
		return Identity{}, errors.New("identity response invalid")
	}
	var p struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Type  string `json:"type"`
	}
	if json.Unmarshal(raw, &p) != nil || p.ID < 1 || p.Login == "" || p.Type != "User" {
		return Identity{}, errors.New("identity response invalid")
	}
	if p.Name == "" {
		p.Name = p.Login
	}
	return Identity{"https://github.com", strconv.FormatInt(p.ID, 10), p.Name}, nil
}
func (g *githubAdapter) Avatar(ctx context.Context, p Identity) ([]byte, error) {
	id, e := strconv.ParseInt(p.Subject, 10, 64)
	if e != nil || id < 1 || p.Authority != "https://github.com" {
		return nil, errors.New("invalid avatar identity")
	}
	// 固定主机和数值路径，不信任远程资料里的任意 URL；不携带 OAuth token。
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://avatars.githubusercontent.com/u/"+strconv.FormatInt(id, 10)+"?s=128", nil)
	if e != nil {
		return nil, e
	}
	res, e := g.client.Do(req)
	if e != nil {
		return nil, errors.New("avatar unavailable")
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, (512<<10)+1))
	if e != nil || res.StatusCode != 200 || len(raw) > 512<<10 {
		return nil, errors.New("avatar unavailable")
	}
	return raw, nil
}
