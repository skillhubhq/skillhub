package serverconfig

import (
	"github.com/skillhubhq/skillhub/internal/identityprovider"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigStrictAndPreservesFalse(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		ok         bool
	}{
		{"valid", "name: Team Skills\nprivate: false\ntrusted_proxies: [127.0.0.1/32]\n", true},
		{"unknown", "privte: false", false},
		{"duplicate", "private: true\nprivate: false", false},
		{"multiple", "name: First\n---\nname: Second", false},
		{"null", "null", false},
		{"empty", "", false},
		{"secret", "auth:\n  providers:\n    - id: example\n      client_secret: do-not-print-this", false},
		{"oversize", strings.Repeat(" ", 65537), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hub.yaml")
			if err := os.WriteFile(path, []byte(tc.text), 0600); err != nil {
				t.Fatal(err)
			}
			c, err := Load(path)
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected result: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "do-not-print-this") {
				t.Fatal("secret leaked")
			}
			if tc.ok && (c.Private || c.Listen != "127.0.0.1:8080" || c.Validate() != nil) {
				t.Fatalf("defaults/false not preserved: %+v", c)
			}
		})
	}
}

func TestSecretsAndValidation(t *testing.T) {
	c := Defaults()
	c.Auth.Providers = []identityprovider.Config{{ID: "work", Type: "oidc", Name: "Work account", Issuer: "https://identity.example.com/tenant", ClientID: "app-id", ClientSecretEnv: "TEST_AUTH_SECRET"}}
	t.Setenv("TEST_AUTH_SECRET", "")
	if _, err := c.Providers(); err == nil {
		t.Fatal("missing secret accepted")
	}
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("secret-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Auth.Providers[0].ClientSecretEnv = ""
	c.Auth.Providers[0].ClientSecretFile = path
	if p, err := c.Providers(); err != nil || p[0].Secret != "secret-value" {
		t.Fatal("file secret not resolved", err)
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.Listen = "localhost:0" },
		func(c *Config) { c.PublicURL = "http://example.com" },
		func(c *Config) { c.PublicURL = "https://example.com/subpath" },
		func(c *Config) { c.TrustedProxies = []string{"127.0.0.1"} },
		func(c *Config) { c.Auth.Providers[0].ClientID = "" },
		func(c *Config) { c.Auth.Providers[0].Name = "" },
		func(c *Config) { c.Auth.Providers[0].ClientSecretEnv = "TEST_AUTH_SECRET" },
	} {
		invalid := c
		invalid.Auth.Providers = append([]identityprovider.Config(nil), c.Auth.Providers...)
		change(&invalid)
		if invalid.Validate() == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}

func TestDatabaseConfiguration(t *testing.T) {
	t.Setenv("SKILLHUB_DATABASE_DSN", "")
	c := Defaults()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	db, err := c.DatabaseConfig()
	if err != nil || db.Driver != "sqlite" {
		t.Fatal("default driver changed")
	}
	c.Database.Driver = "mysql"
	if c.Validate() == nil {
		t.Fatal("unknown driver accepted")
	}
	c.Database.Driver = "postgres"
	if _, err = c.DatabaseConfig(); err == nil {
		t.Fatal("missing DSN accepted")
	}
	t.Setenv("SKILLHUB_DATABASE_DSN", "postgres://test:secret@localhost/test")
	db, err = c.DatabaseConfig()
	if err != nil || db.DSN == "" {
		t.Fatal("environment DSN unavailable")
	}
	c.Database.DSNFile = filepath.Join(t.TempDir(), "dsn")
	if err = os.WriteFile(c.Database.DSNFile, []byte("postgres://test:secret@localhost/test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = c.DatabaseConfig(); err == nil {
		t.Fatal("ambiguous secret sources")
	}
	t.Setenv("SKILLHUB_DATABASE_DSN", "")
	db, err = c.DatabaseConfig()
	if err != nil || strings.HasSuffix(db.DSN, "\n") {
		t.Fatal("file DSN invalid")
	}
	c.Database.Driver = "sqlite"
	if c.Validate() == nil {
		t.Fatal("SQLite accepted DSN file")
	}
}

func TestAuthProviderConfiguration(t *testing.T) {
	c := Defaults()
	c.Mode = "community"
	c.Auth.Providers = []identityprovider.Config{{ID: "social", Type: "github", Name: "Community account", ClientID: "public-client", ClientSecretEnv: "TEST_PROVIDER_SECRET"}, {ID: "company", Type: "oidc", Name: "Company account", Issuer: "https://identity.example/tenant", ClientID: "company-client", ClientSecretEnv: "TEST_PROVIDER_SECRET"}}
	t.Setenv("TEST_PROVIDER_SECRET", "fixture-only-secret")
	providers, e := c.Providers()
	if e != nil || len(providers) != 2 || providers[0].Secret != "fixture-only-secret" {
		t.Fatal("provider normalization", e)
	}
	if c.Auth.Providers[0].Secret != "" {
		t.Fatal("secret mutated config")
	}
	c.Auth.Providers[1].ID = "social"
	if c.Validate() == nil {
		t.Fatal("duplicate provider")
	}
	c.Auth.Providers[1].ID = "company"
	t.Setenv("TEST_PROVIDER_SECRET", "")
	if _, e = c.Providers(); e == nil {
		t.Fatal("missing secret")
	}
	c.Auth.Providers = nil
	c.Mode = "unknown"
	if c.Validate() == nil {
		t.Fatal("unknown mode")
	}
	c = Defaults()
	c.Mode = "community"
	c.Limits.PackageBytes = 0
	if c.Validate() == nil {
		t.Fatal("unbounded community storage")
	}
}

func TestDocumentedDeploymentConfigs(t *testing.T) {
	for _, name := range []string{"hub.example.yaml", "community.example.yaml"} {
		t.Run(name, func(t *testing.T) {
			c, e := Load("../../deploy/" + name)
			if e != nil {
				t.Fatal(e)
			}
			if e = c.Validate(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
