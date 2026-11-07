// Package serverconfig 加载云平台无关的 Hub 启动配置。
package serverconfig

import (
	"bytes"
	"errors"
	"github.com/skillhubhq/skillhub/internal/community"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/hubpolicy"
	"github.com/skillhubhq/skillhub/internal/identityprovider"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Database struct {
	Driver  string `yaml:"driver"`
	DSNFile string `yaml:"dsn_file"`
}

type Config struct {
	Mode           string           `yaml:"mode"`
	Limits         hubpolicy.Limits `yaml:"limits"`
	Auth           Auth             `yaml:"auth"`
	Site           Site             `yaml:"site"`
	Database       Database         `yaml:"database"`
	Data           string           `yaml:"data"`
	Listen         string           `yaml:"listen"`
	PublicURL      string           `yaml:"public_url"`
	Name           string           `yaml:"name"`
	Private        bool             `yaml:"private"`
	TrustedProxies []string         `yaml:"trusted_proxies"`
}

type Auth struct {
	Providers []identityprovider.Config `yaml:"providers"`
}

type Site = community.Site

func Defaults() Config {
	return Config{Mode: hubpolicy.Enterprise, Limits: hubpolicy.Defaults(), Database: Database{Driver: "sqlite"}, Data: ".local/hub", Listen: "127.0.0.1:8080", PublicURL: "http://127.0.0.1:8080", Name: "SkillHub", Private: true}
}

func Load(path string) (Config, error) {
	c := Defaults()
	if path == "" {
		return c, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return c, errors.New("cannot open configuration file")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 || len(bytes.TrimSpace(raw)) == 0 {
		return c, errors.New("configuration must be a non-empty document of at most 64 KiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return c, errors.New("configuration must be a mapping")
	}
	// 解析错误可能包含用户误填的密钥值，不透传原始 YAML 错误。
	if err := decoder.Decode(&c); err != nil {
		return c, errors.New("invalid configuration: check field names, types and duplicate keys")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return c, errors.New("configuration must contain exactly one document")
	}
	return c, nil
}

// Validate 不连接身份提供者、不打开数据库，也不输出配置内容。
func (c Config) Validate() error {
	if err := c.Policy().Validate(); err != nil {
		return err
	}
	if len(c.Auth.Providers) > 8 {
		return errors.New("at most eight auth providers are supported")
	}
	seen := map[string]bool{}
	for _, p := range c.Auth.Providers {
		if e := identityprovider.Validate(p); e != nil {
			return e
		}
		if seen[p.ID] {
			return errors.New("duplicate auth provider id")
		}
		seen[p.ID] = true
	}
	for _, raw := range []string{c.Site.PrivacyURL, c.Site.TermsURL, c.Site.ContactURL, c.Site.FilingURL} {
		if raw == "" {
			continue
		}
		u, e := url.Parse(raw)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return errors.New("site links must be HTTPS URLs")
		}
	}
	if len(c.Site.FilingText) > 200 {
		return errors.New("site.filing_text exceeds 200 bytes")
	}
	if c.Database.Driver != "sqlite" && c.Database.Driver != "postgres" {
		return errors.New("database.driver must be sqlite or postgres")
	}
	if c.Database.Driver == "sqlite" && c.Database.DSNFile != "" {
		return errors.New("SQLite uses data directory; remove database.dsn_file")
	}

	if strings.TrimSpace(c.Data) == "" || strings.TrimSpace(c.Name) == "" {
		return errors.New("data and name must not be empty")
	}
	_, port, err := net.SplitHostPort(c.Listen)
	n, numberErr := strconv.Atoi(port)
	if err != nil || numberErr != nil || n < 1 || n > 65535 {
		return errors.New("listen must be a host:port address with a port from 1 to 65535")
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("public_url must be an absolute HTTP(S) origin without a subpath")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return errors.New("public_url requires HTTPS except on loopback")
	}
	for _, cidr := range c.TrustedProxies {
		if strings.TrimSpace(cidr) == "" {
			continue
		}
		if _, err := netip.ParsePrefix(strings.TrimSpace(cidr)); err != nil {
			return errors.New("trusted_proxies must contain explicit IP CIDRs")
		}
	}

	return nil
}

func (c Config) DatabaseConfig() (hubdb.Config, error) {
	result := hubdb.Config{Driver: c.Database.Driver}
	dsn := os.Getenv("SKILLHUB_DATABASE_DSN")
	if result.Driver == "sqlite" {
		if dsn != "" {
			return result, errors.New("SKILLHUB_DATABASE_DSN requires database.driver postgres")
		}
		return result, nil
	}
	if c.Database.DSNFile != "" {
		if dsn != "" {
			return result, errors.New("choose database.dsn_file or SKILLHUB_DATABASE_DSN, not both")
		}
		f, err := os.Open(c.Database.DSNFile)
		if err != nil {
			return result, errors.New("cannot open database DSN file")
		}
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, 8193))
		if err != nil || len(raw) > 8192 {
			return result, errors.New("cannot read database DSN file or it exceeds 8 KiB")
		}
		dsn = strings.TrimSpace(string(raw))
	}
	if strings.TrimSpace(dsn) == "" || len(dsn) > 8192 {
		return result, errors.New("PostgreSQL requires a DSN of at most 8 KiB from SKILLHUB_DATABASE_DSN or database.dsn_file")
	}
	result.DSN = dsn
	return result, nil
}

func (c Config) Policy() hubpolicy.Policy { return hubpolicy.Policy{Mode: c.Mode, Limits: c.Limits} }

// Providers 在启动时解析密钥，不把密钥值写入配置或错误信息。
func (c Config) Providers() ([]identityprovider.Config, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	providers := append([]identityprovider.Config(nil), c.Auth.Providers...)
	for i := range providers {
		p := &providers[i]
		value := ""
		if p.ClientSecretEnv != "" {
			value = os.Getenv(p.ClientSecretEnv)
		}
		if p.ClientSecretFile != "" {
			f, e := os.Open(p.ClientSecretFile)
			if e != nil {
				return nil, errors.New("cannot open auth provider secret file")
			}
			raw, e := io.ReadAll(io.LimitReader(f, 8193))
			f.Close()
			if e != nil || len(raw) > 8192 {
				return nil, errors.New("cannot read auth provider secret file or it exceeds 8 KiB")
			}
			value = strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
		}
		if strings.TrimSpace(value) == "" || len(value) > 8192 {
			return nil, errors.New("auth provider requires a non-empty secret of at most 8 KiB")
		}
		p.Secret = value
	}
	return providers, nil
}
