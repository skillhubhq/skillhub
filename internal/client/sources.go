package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/gen/api/admin/v1/adminv1connect"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/skillhubhq/skillhub/gen/api/auth/v1/authv1connect"
	"github.com/skillhubhq/skillhub/gen/api/catalog/v1/catalogv1connect"
	"github.com/skillhubhq/skillhub/gen/api/community/v1/communityv1connect"
	"github.com/skillhubhq/skillhub/gen/api/evaluation/v1/evaluationv1connect"
	"github.com/skillhubhq/skillhub/gen/api/governance/v1/governancev1connect"
	"github.com/skillhubhq/skillhub/gen/api/publishing/v1/publishingv1connect"
	registryv1 "github.com/skillhubhq/skillhub/gen/api/registry/v1"
	"github.com/skillhubhq/skillhub/gen/api/registry/v1/registryv1connect"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/zalando/go-keyring"
)

var (
	ErrSourceNotFound    = errors.New("source not found")
	ErrRegistryChanged   = errors.New("source registry identity changed; reconnect explicitly")
	ErrCredentialMissing = errors.New("no saved credential")
	ErrCredentialStore   = errors.New("operating system credential store unavailable")
)

type Source struct {
	AccountID     string `json:"account_id"`
	Label         string `json:"label"`
	Note          string `json:"note"`
	Official      bool   `json:"official"`
	Alias         string `json:"alias"`
	URL           string `json:"url"`
	RegistryID    string `json:"registry_id"`
	DisplayName   string `json:"display_name"`
	Private       bool   `json:"private"`
	CredentialRef string `json:"credential_ref"`
	HasCredential bool   `json:"has_credential"`
}

// Credentials 允许测试注入隔离实现；生产默认使用操作系统凭据库。
type Credentials interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}
type SystemCredentials struct{}

func (SystemCredentials) Get(ref string) (string, error) {
	value, err := keyring.Get("io.github.skillhubhq.skillhub", ref)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrCredentialMissing
	}
	if err != nil {
		return "", ErrCredentialStore
	}
	return value, nil
}
func (SystemCredentials) Set(ref, token string) error {
	if err := keyring.Set("io.github.skillhubhq.skillhub", ref, token); err != nil {
		return ErrCredentialStore
	}
	return nil
}
func (SystemCredentials) Delete(ref string) error {
	if err := keyring.Delete("io.github.skillhubhq.skillhub", ref); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return ErrCredentialStore
	}
	return nil
}

type ClientOptions struct {
	Credentials Credentials
	HTTPClient  *http.Client
	Home        string
}
type Client struct {
	store       *Store
	credentials Credentials
	http        *http.Client
	home        string
}

func NewClient(store *Store, options ClientOptions) *Client {
	httpClient := http.Client{Timeout: 30 * time.Second}
	if options.HTTPClient != nil {
		httpClient = *options.HTTPClient
		if httpClient.Timeout == 0 {
			httpClient.Timeout = 30 * time.Second
		}
	}
	// 所有 RPC 均禁止自动跳转，也不使用浏览器 Cookie；源凭据不跟随重定向。
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	httpClient.Jar = nil
	credentials := options.Credentials
	if credentials == nil {
		credentials = SystemCredentials{}
	}
	return &Client{store: store, credentials: credentials, http: &httpClient, home: options.Home}
}

func canonicalURL(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("source URL must be an absolute HTTP(S) origin")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return "", errors.New("sources require HTTPS except on loopback")
		}
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = ""
	return u.String(), nil
}

func (c *Client) probe(ctx context.Context, address string) (*registryv1.GetRegistryResponse, error) {
	api := registryv1connect.NewRegistryServiceClient(c.http, address, connect.WithReadMaxBytes(64<<10), connect.WithInterceptors(identityRetryInterceptor()))
	response, err := api.GetRegistry(ctx, connect.NewRequest(&registryv1.GetRegistryRequest{}))
	if err != nil {
		return nil, err
	}
	if response.Msg.ProtocolMajor != 2 || len(response.Msg.RegistryId) < 1 || len(response.Msg.RegistryId) > 128 {
		return nil, errors.New("unsupported registry protocol or identity")
	}
	return response.Msg, nil
}

func (c *Client) AddSource(ctx context.Context, alias, address string) (Source, error) {
	if !packageformat.ValidName(alias) {
		return Source{}, errors.New("invalid source alias")
	}
	address, err := canonicalURL(address)
	if err != nil {
		return Source{}, err
	}
	info, err := c.probe(ctx, address)
	if err != nil {
		return Source{}, err
	}
	source := Source{Alias: alias, URL: address, RegistryID: info.RegistryId, DisplayName: info.DisplayName, Private: info.PrivateMode, CredentialRef: uuid.NewString()}
	raw, err := json.Marshal(source)
	if err != nil {
		return Source{}, err
	}
	result, err := c.store.db.ExecContext(ctx, "INSERT INTO sources(alias,registry_id,record) VALUES(?,?,?) ON CONFLICT(alias) DO NOTHING", alias, source.RegistryID, string(raw))
	if err != nil {
		return Source{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Source{}, err
	}
	if n == 0 {
		return Source{}, errors.New("source alias already exists")
	}
	source.Official = IsOfficialHub(source.URL)
	return source, nil
}

func (c *Client) Source(ctx context.Context, alias string) (Source, error) {
	var raw string
	err := c.store.db.QueryRowContext(ctx, "SELECT record FROM sources WHERE alias=?", alias).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Source{}, ErrSourceNotFound
	}
	if err != nil {
		return Source{}, err
	}
	var source Source
	if err = json.Unmarshal([]byte(raw), &source); err != nil {
		return Source{}, err
	}
	if source.Alias != alias {
		return Source{}, errors.New("invalid source record")
	}
	source.Official = IsOfficialHub(source.URL)
	return source, nil
}

func (c *Client) ListSources(ctx context.Context) ([]Source, error) {
	rows, err := c.store.db.QueryContext(ctx, "SELECT record FROM sources ORDER BY alias")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Source{}
	for rows.Next() {
		var raw string
		var source Source
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &source); err != nil {
			return nil, err
		}
		source.Official = IsOfficialHub(source.URL)
		result = append(result, source)
	}
	return result, rows.Err()
}

func (c *Client) RemoveSource(ctx context.Context, alias string) error {
	return c.store.withTarget(ctx, "source:"+alias, func() error { return c.removeSource(ctx, alias) })
}

func (c *Client) removeSource(ctx context.Context, alias string) error {
	source, err := c.Source(ctx, alias)
	if err != nil {
		return err
	}
	if source.HasCredential {
		if err = c.credentials.Delete(source.CredentialRef); err != nil {
			return err
		}
	}
	_, err = c.store.db.ExecContext(ctx, "DELETE FROM sources WHERE alias=? AND registry_id=?", alias, source.RegistryID)
	return err
}

type Remote struct {
	Evaluation   evaluationv1connect.EvaluationServiceClient
	Governance   governancev1connect.GovernanceServiceClient
	Capabilities []string
	Community    communityv1connect.CommunityServiceClient
	Source       Source
	Auth         authv1connect.AuthServiceClient
	Admin        adminv1connect.AdminServiceClient
	Catalog      catalogv1connect.CatalogServiceClient
	Publishing   publishingv1connect.PublishingServiceClient
}

func tokenEnvironment(alias string) string {
	return "SKILLHUB_TOKEN_" + strings.ReplaceAll(strings.ToUpper(alias), "-", "_")
}

func (c *Client) connectSource(ctx context.Context, alias string, withCredential bool) (*Remote, error) {
	source, err := c.Source(ctx, alias)
	if err != nil {
		return nil, err
	}
	address, err := canonicalURL(source.URL)
	if err != nil {
		return nil, err
	}
	info, err := c.probe(ctx, address)
	if err != nil {
		return nil, err
	}
	if info.RegistryId != source.RegistryID {
		return nil, ErrRegistryChanged
	}
	token := ""
	if withCredential {
		var injected bool
		token, injected = os.LookupEnv(tokenEnvironment(alias))
		if !injected && source.HasCredential {
			token, err = c.credentials.Get(source.CredentialRef)
			if err != nil {
				return nil, err
			}
		}
		if (injected || (source.HasCredential && err == nil)) && len(token) != 43 && !(len(token) == 47 && strings.HasPrefix(token, "sht_")) {
			return nil, errors.New("invalid source credential")
		}
	}
	interceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if token != "" {
				req.Header().Set("Authorization", "Bearer "+token)
			}
			return next(ctx, req)
		}
	})
	opts := []connect.ClientOption{connect.WithInterceptors(identityRetryInterceptor(), interceptor), connect.WithReadMaxBytes(32 << 20)}
	return &Remote{Evaluation: evaluationv1connect.NewEvaluationServiceClient(c.http, address, opts...), Governance: governancev1connect.NewGovernanceServiceClient(c.http, address, opts...), Capabilities: info.Capabilities, Source: source, Community: communityv1connect.NewCommunityServiceClient(c.http, address, opts...), Auth: authv1connect.NewAuthServiceClient(c.http, address, opts...), Admin: adminv1connect.NewAdminServiceClient(c.http, address, opts...), Catalog: catalogv1connect.NewCatalogServiceClient(c.http, address, opts...), Publishing: publishingv1connect.NewPublishingServiceClient(c.http, address, opts...)}, nil
}

// Remote 先匿名确认实例身份，再按此来源添加凭据。
func (c *Client) Remote(ctx context.Context, alias string) (*Remote, error) {
	return c.connectSource(ctx, alias, true)
}

func (c *Client) saveSession(ctx context.Context, remote *Remote, response *authv1.LoginResponse) (*authv1.Identity, error) {
	if len(response.GetAccessToken()) != 43 || response.GetIdentity() == nil {
		return nil, errors.New("invalid login response")
	}
	err := c.store.withTarget(ctx, "source:"+remote.Source.Alias, func() error {
		current, err := c.Source(ctx, remote.Source.Alias)
		if err != nil || current != remote.Source {
			return errors.New("source changed during login")
		}
		// 账号变化时分离凭据引用，安装和配置不会自动切换到另一身份。
		previousRef := current.CredentialRef
		if current.AccountID != response.Identity.UserId {
			current.CredentialRef = uuid.NewString()
		}
		current.AccountID = response.Identity.UserId
		if err = c.credentials.Set(current.CredentialRef, response.AccessToken); err != nil {
			return err
		}
		current.HasCredential = true
		raw, err := json.Marshal(current)
		if err != nil {
			return err
		}
		_, err = c.store.db.ExecContext(ctx, "UPDATE sources SET record=? WHERE alias=?", string(raw), current.Alias)
		if err == nil && previousRef != current.CredentialRef {
			_ = c.credentials.Delete(previousRef)
		}
		return err
	})
	if err != nil {
		// 本地保存失败时尽力撤销刚签发的远端令牌，且不将令牌放入错误。
		req := connect.NewRequest(&authv1.LogoutRequest{})
		req.Header().Set("Authorization", "Bearer "+response.AccessToken)
		_, _ = remote.Auth.Logout(ctx, req)
		return nil, err
	}
	return response.Identity, nil
}

func (c *Client) LoginPassword(ctx context.Context, alias, username, password string) (*authv1.Identity, error) {
	if _, ok := os.LookupEnv(tokenEnvironment(alias)); ok {
		return nil, errors.New("unset the source token environment variable before saving a login")
	}
	remote, err := c.connectSource(ctx, alias, false)
	if err != nil {
		return nil, err
	}
	response, err := remote.Auth.Login(ctx, connect.NewRequest(&authv1.LoginRequest{Username: username, Password: password}))
	if err != nil {
		return nil, err
	}
	return c.saveSession(ctx, remote, response.Msg)
}

type DeviceLogin struct {
	Source          Source        `json:"source"`
	DeviceCode      string        `json:"-"`
	UserCode        string        `json:"user_code"`
	VerificationURI string        `json:"verification_uri"`
	ExpiresAt       time.Time     `json:"expires_at"`
	Interval        time.Duration `json:"-"`
}

func (c *Client) StartLogin(ctx context.Context, alias string) (DeviceLogin, error) {
	if _, ok := os.LookupEnv(tokenEnvironment(alias)); ok {
		return DeviceLogin{}, errors.New("unset the source token environment variable before saving a login")
	}
	remote, err := c.connectSource(ctx, alias, false)
	if err != nil {
		return DeviceLogin{}, err
	}
	response, err := remote.Auth.StartDeviceLogin(ctx, connect.NewRequest(&authv1.StartDeviceLoginRequest{}))
	if err != nil {
		return DeviceLogin{}, err
	}
	u, err := url.Parse(response.Msg.VerificationUri)
	if err != nil || u.Scheme+"://"+u.Host != remote.Source.URL || u.Path != "/authorize" || u.RawQuery != "" || u.User != nil || u.Fragment != "" {
		return DeviceLogin{}, errors.New("authorization URL does not belong to this source")
	}
	if len(response.Msg.DeviceCode) != 43 || response.Msg.ExpiresAt == nil || response.Msg.IntervalSeconds < 2 || response.Msg.IntervalSeconds > 60 {
		return DeviceLogin{}, errors.New("invalid device authorization response")
	}
	return DeviceLogin{Source: remote.Source, DeviceCode: response.Msg.DeviceCode, UserCode: response.Msg.UserCode, VerificationURI: u.String(), ExpiresAt: response.Msg.ExpiresAt.AsTime(), Interval: time.Duration(response.Msg.IntervalSeconds) * time.Second}, nil
}

func (c *Client) CompleteLogin(ctx context.Context, login DeviceLogin) (*authv1.Identity, bool, error) {
	remote, err := c.connectSource(ctx, login.Source.Alias, false)
	if err != nil {
		return nil, false, err
	}
	if remote.Source != login.Source {
		return nil, false, ErrRegistryChanged
	}
	response, err := remote.Auth.CompleteDeviceLogin(ctx, connect.NewRequest(&authv1.CompleteDeviceLoginRequest{DeviceCode: login.DeviceCode}))
	if err != nil {
		return nil, false, err
	}
	if response.Msg.Pending {
		return nil, true, nil
	}
	user, err := c.saveSession(ctx, remote, response.Msg.Login)
	return user, false, err
}

func (c *Client) Logout(ctx context.Context, alias string) error {
	return c.store.withTarget(ctx, "source:"+alias, func() error { return c.logout(ctx, alias) })
}

func (c *Client) logout(ctx context.Context, alias string) error {
	if _, ok := os.LookupEnv(tokenEnvironment(alias)); ok {
		return fmt.Errorf("credential supplied by %s; remove it from the environment to disconnect", tokenEnvironment(alias))
	}
	source, err := c.Source(ctx, alias)
	if err != nil {
		return err
	}
	remote, remoteErr := c.Remote(ctx, alias)
	if remoteErr == nil {
		_, remoteErr = remote.Auth.Logout(ctx, connect.NewRequest(&authv1.LogoutRequest{}))
	}
	if source.HasCredential {
		if err = c.credentials.Delete(source.CredentialRef); err != nil {
			return err
		}
	}
	source.HasCredential = false
	raw, _ := json.Marshal(source)
	if _, err = c.store.db.ExecContext(ctx, "UPDATE sources SET record=? WHERE alias=?", string(raw), alias); err != nil {
		return err
	}
	if remoteErr != nil && connect.CodeOf(remoteErr) != connect.CodeUnauthenticated {
		return errors.New("local credential removed; remote revocation could not be verified")
	}
	return nil
}

// RequireSkillPresentation 防止旧 Hub 静默忽略新版展示字段。
func (r *Remote) RequireSkillPresentation() error {
	for _, capability := range r.Capabilities {
		if capability == "catalog.presentation.v1" {
			return nil
		}
	}
	return errors.New("this Hub needs an upgrade to accept skill display names and icons")
}

// RequireProvenance 防止旧 Hub 丢弃署名和许可证声明。
func (r *Remote) RequireProvenance() error {
	for _, c := range r.Capabilities {
		if c == "catalog.provenance.v1" {
			return nil
		}
	}
	return errors.New("this Hub needs an upgrade to accept upstream attribution")
}
