// Package serverconfig 加载云平台无关的 Hub 启动配置。
package serverconfig

import (
	"bytes"
	"errors"
	"github.com/skillhubhq/skillhub/internal/hubdb"
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
	Database       Database `yaml:"database"`
	Data           string   `yaml:"data"`
	Listen         string   `yaml:"listen"`
	PublicURL      string   `yaml:"public_url"`
	Name           string   `yaml:"name"`
	Private        bool     `yaml:"private"`
	TrustedProxies []string `yaml:"trusted_proxies"`
	OIDC           OIDC     `yaml:"oidc"`
}

type OIDC struct {
	Issuer           string `yaml:"issuer"`
	ClientID         string `yaml:"client_id"`
	Name             string `yaml:"name"`
	AllowSignup      bool   `yaml:"allow_signup"`
	ClientSecretFile string `yaml:"client_secret_file"`
}

func Defaults() Config {
	return Config{Database: Database{Driver: "sqlite"}, Data: ".local/hub", Listen: "127.0.0.1:8080", PublicURL: "http://127.0.0.1:8080", Name: "SkillHub", Private: true, OIDC: OIDC{Name: "Microsoft"}}
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
	if c.OIDC.Issuer != "" || c.OIDC.ClientID != "" || c.OIDC.ClientSecretFile != "" || c.OIDC.AllowSignup {
		u, err := url.Parse(c.OIDC.Issuer)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || c.OIDC.ClientID == "" {
			return errors.New("OIDC requires an HTTPS issuer and client_id")
		}
	}
	return nil
}

func (c Config) Secret() (string, error) {
	if c.OIDC.Issuer == "" {
		return "", nil
	}
	env := os.Getenv("SKILLHUB_OIDC_CLIENT_SECRET")
	if c.OIDC.ClientSecretFile != "" {
		if env != "" {
			return "", errors.New("choose either OIDC client_secret_file or SKILLHUB_OIDC_CLIENT_SECRET, not both")
		}
		f, err := os.Open(c.OIDC.ClientSecretFile)
		if err != nil {
			return "", errors.New("cannot open OIDC secret file")
		}
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, 8193))
		if err != nil || len(raw) > 8192 {
			return "", errors.New("cannot read OIDC secret file or it exceeds 8 KiB")
		}
		env = strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	}
	if strings.TrimSpace(env) == "" || len(env) > 8192 {
		return "", errors.New("OIDC requires a non-empty client secret of at most 8 KiB")
	}
	return env, nil
}

// DatabaseConfig 从显式环境或密钥文件读取 DSN，不将连接串写入日志或示例配置。
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
