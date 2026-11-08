package identityprovider

import (
	"context"
	"errors"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestGitHubAdapterIdentityAndBoundaries(t *testing.T) {
	verifier := oauth2.GenerateVerifier()
	profile := `{"id":42,"login":"person","name":"Test Person","type":"User","avatar_url":"http://private.example/secret"}`
	phase := "success"
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.URL.Host + r.URL.Path {
		case "github.com/login/oauth/access_token":
			if r.Method != "POST" {
				t.Fatal("wrong token method")
			}
			if e := r.ParseForm(); e != nil {
				t.Fatal(e)
			}
			if r.Form.Get("client_secret") != "test-secret" || r.Form.Get("code_verifier") != verifier || r.Form.Get("code") != "code" || r.Form.Get("redirect_uri") != "https://skills.example.com/auth/providers/social/callback" {
				t.Fatal("invalid code exchange")
			}
			if phase == "token-error" {
				return response(401, "sensitive error"), nil
			}
			return response(200, `{"access_token":"temporary-token","token_type":"Bearer"}`), nil
		case "api.github.com/user":
			if r.Header.Get("Authorization") != "Bearer temporary-token" {
				t.Fatal("missing user authorization")
			}
			if phase == "profile-error" {
				return response(403, "sensitive error"), nil
			}
			if phase == "oversize" {
				return response(200, strings.Repeat("x", (128<<10)+1)), nil
			}
			return response(200, profile), nil
		case "avatars.githubusercontent.com/u/42":
			if r.Header.Get("Authorization") != "" || r.URL.RawQuery != "s=128" {
				t.Fatal("avatar leaked identity token")
			}
			if phase == "redirect" {
				v := response(302, "")
				v.Header.Set("Location", "https://untrusted.example")
				return v, nil
			}
			if phase == "avatar-large" {
				return response(200, strings.Repeat("x", (512<<10)+1)), nil
			}
			return response(200, "image-bytes"), nil
		default:
			t.Fatalf("untrusted endpoint %s", r.URL.Host)
			return nil, errors.New("unexpected endpoint")
		}
	})}
	adapter, e := New(context.Background(), Config{ID: "social", Type: "github", Name: "Community identity", ClientID: "test-client", Secret: "test-secret"}, "https://skills.example.com/auth/providers/social/callback", client)
	if e != nil {
		t.Fatal(e)
	}
	start, _ := url.Parse(adapter.AuthorizationURL("state", "nonce", verifier))
	q := start.Query()
	if start.Host != "github.com" || q.Get("state") != "state" || q.Get("code_challenge") != oauth2.S256ChallengeFromVerifier(verifier) || q.Get("code_challenge_method") != "S256" || q.Get("scope") != "" {
		t.Fatal("unsafe authorization URL")
	}
	p, e := adapter.Exchange(context.Background(), "code", "", verifier)
	if e != nil || p.Authority != "https://github.com" || p.Subject != "42" || p.DisplayName != "Test Person" {
		t.Fatal(p, e)
	}
	profile = `{"id":42,"login":"renamed","type":"User"}`
	again, e := adapter.Exchange(context.Background(), "code", "", verifier)
	if e != nil || again.Subject != p.Subject || again.DisplayName != "renamed" {
		t.Fatal("rename changed identity", e)
	}
	if _, e = adapter.Avatar(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	before := calls
	if _, e = adapter.Avatar(context.Background(), Identity{Authority: p.Authority, Subject: "../secret"}); e == nil || calls != before {
		t.Fatal("unsafe avatar path")
	}
	for _, phase = range []string{"token-error", "profile-error", "oversize"} {
		if _, e = adapter.Exchange(context.Background(), "code", "", verifier); e == nil || strings.Contains(e.Error(), "sensitive") {
			t.Fatal("unsafe exchange error", phase, e)
		}
	}
	phase = "success"
	for _, profile = range []string{`{"id":0,"login":"person","type":"User"}`, `{"id":42,"login":"org","type":"Organization"}`, `{"id":42,"type":"User"}`, `invalid`} {
		if _, e = adapter.Exchange(context.Background(), "code", "", verifier); e == nil {
			t.Fatal("invalid profile accepted")
		}
	}
	for _, phase = range []string{"redirect", "avatar-large"} {
		if _, e = adapter.Avatar(context.Background(), p); e == nil {
			t.Fatal("unsafe avatar response", phase)
		}
	}
}
func TestProviderConfigurationValidation(t *testing.T) {
	valid := Config{ID: "company", Type: "oidc", Name: "Company SSO", Issuer: "https://identity.example/tenant", ClientID: "client", Secret: "secret"}
	if e := Validate(valid); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.ID = "../../callback" }, func(c *Config) { c.Type = "unsupported" }, func(c *Config) { c.Issuer = "http://identity.example" }, func(c *Config) { c.ClientSecretEnv = "secret=value" }, func(c *Config) { c.ClientSecretEnv = "SECRET"; c.ClientSecretFile = "file" }, func(c *Config) { c.Name = "" }, func(c *Config) { c.Type = "github" }} {
		c := valid
		mutate(&c)
		if Validate(c) == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}
