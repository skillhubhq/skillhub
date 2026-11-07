package serverconfig

import (
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
		{"secret", "oidc:\n  client_secret: do-not-print-this", false},
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
	c.OIDC.Issuer = "https://identity.example.com/tenant"
	c.OIDC.ClientID = "app-id"
	t.Setenv("SKILLHUB_OIDC_CLIENT_SECRET", "")
	if _, err := c.Secret(); err == nil {
		t.Fatal("missing secret accepted")
	}
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("secret-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c.OIDC.ClientSecretFile = path
	if s, err := c.Secret(); err != nil || s != "secret-value" {
		t.Fatal("file secret not resolved")
	}
	t.Setenv("SKILLHUB_OIDC_CLIENT_SECRET", "environment-value")
	if _, err := c.Secret(); err == nil {
		t.Fatal("ambiguous secret accepted")
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.Listen = "localhost:0" },
		func(c *Config) { c.PublicURL = "http://example.com" },
		func(c *Config) { c.PublicURL = "https://example.com/subpath" },
		func(c *Config) { c.TrustedProxies = []string{"127.0.0.1"} },
		func(c *Config) { c.OIDC.ClientID = "" },
	} {
		invalid := c
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
