// Package identityprovider 将第三方协议适配为已验证身份，不处理 Hub 建号和授权。
package identityprovider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

type Config struct {
	ID               string `yaml:"id"`
	Type             string `yaml:"type"`
	Name             string `yaml:"name"`
	Issuer           string `yaml:"issuer"`
	ClientID         string `yaml:"client_id"`
	ClientSecretFile string `yaml:"client_secret_file"`
	ClientSecretEnv  string `yaml:"client_secret_env"`
	Secret           string `yaml:"-"`
}

type Identity struct{ Authority, Subject, DisplayName string }

// Adapter 不签发 Hub 会话、不赋予角色；头像只允许由适配器从受信来源读取。
type Adapter interface {
	AuthorizationURL(state, nonce, verifier string) string
	Exchange(context.Context, string, string, string) (Identity, error)
	Avatar(context.Context, Identity) ([]byte, error)
}

type factory func(context.Context, Config, string, *http.Client) (Adapter, error)

var adapters = map[string]factory{"oidc": newOIDC, "github": newGitHub}
var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)

func Validate(c Config) error {
	if !identifier.MatchString(c.ID) || c.Name == "" || len(c.Name) > 80 || c.ClientID == "" || len(c.ClientID) > 1024 {
		return errors.New("auth provider requires a stable id, name and client_id")
	}
	if _, ok := adapters[c.Type]; !ok {
		return errors.New("unsupported auth provider adapter")
	}
	if c.ClientSecretFile != "" && c.ClientSecretEnv != "" {
		return errors.New("choose one auth provider secret source")
	}
	if c.ClientSecretEnv != "" && !envName.MatchString(c.ClientSecretEnv) {
		return errors.New("invalid auth provider secret environment name")
	}
	if c.Type == "oidc" {
		u, e := url.Parse(c.Issuer)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("OIDC adapter requires an HTTPS issuer")
		}
	} else if c.Issuer != "" {
		return errors.New("this adapter does not accept a custom issuer")
	}
	return nil
}
func New(ctx context.Context, c Config, callback string, client *http.Client) (Adapter, error) {
	if e := Validate(c); e != nil {
		return nil, e
	}
	if c.Secret == "" {
		return nil, errors.New("auth provider secret is required")
	}
	// 复制调用方的 Transport 方便隔离测试；超时和禁止跳转不能被配置绕过。
	bounded := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		bounded.Transport = client.Transport
	}
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return adapters[c.Type](ctx, c, callback, &bounded)
}
