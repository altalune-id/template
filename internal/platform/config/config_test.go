package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"altalune.id/template/internal/platform/db"
)

func TestLoad_DefaultsPopulate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	cfg, err := Load("", withCwdOverride(t, dir), withGenesisFallback(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != ModeSelfhosted {
		t.Fatalf("mode: want %q, got %q", ModeSelfhosted, cfg.Mode)
	}
	if cfg.HTTP.Addr != ":5150" {
		t.Fatalf("http.addr: want :5150, got %q", cfg.HTTP.Addr)
	}
	if cfg.API.Enabled != true {
		t.Fatalf("api.enabled: want true, got %v", cfg.API.Enabled)
	}
	if cfg.Log.Level != "info" || cfg.Log.Format != "json" {
		t.Fatalf("log defaults wrong: %+v", cfg.Log)
	}
	if !cfg.DB.AutoMigrate {
		t.Fatalf("db.autoMigrate: want true, got false")
	}
}

func TestLoad_YAMLThenEnvOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	yaml := `
mode: selfhosted
http:
  addr: ":6000"
  baseURL: "http://localhost:6000"
genesis:
  email: "root@example.com"
  password: "correct-horse"
`
	path := writeTempYAML(t, "cfg.yaml", yaml)

	t.Setenv("ALT_HTTP_ADDR", ":8080")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Fatalf("env should win: got %q", cfg.HTTP.Addr)
	}
	if cfg.HTTP.BaseURL != "http://localhost:6000" {
		t.Fatalf("yaml value lost: %q", cfg.HTTP.BaseURL)
	}
	if cfg.Genesis.Email != "root@example.com" {
		t.Fatalf("genesis.email: %q", cfg.Genesis.Email)
	}
}

func TestLoad_ValidateCascades(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	yaml := `
mode: selfhosted
genesis:
  email: "root@example.com"
  password: "x"
log:
  level: "shout"
`
	path := writeTempYAML(t, "cfg.yaml", yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "logger:") {
		t.Fatalf("expected logger validation error, got %v", err)
	}
}

func TestValidate_ModeInvariants(t *testing.T) {
	base := func() *Config {
		c := &Config{
			Mode:    ModeSelfhosted,
			DB:      validDB(),
			Genesis: GenesisConfig{Email: "root@example.com", Password: "x"},
		}
		c.Tenant.SingletonOrg.Slug = "default"
		c.Tenant.SingletonOrg.Name = "Default Organization"
		return c
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{
			name:    "unset mode fails",
			mutate:  func(c *Config) { c.Mode = "" },
			wantSub: "Config.Mode",
		},
		{
			name:    "unknown mode fails",
			mutate:  func(c *Config) { c.Mode = "hybrid" },
			wantSub: "oneof",
		},
		{
			name: "selfhosted without genesis or oidc is allowed (onboarding-first)",
			mutate: func(c *Config) {
				c.Genesis = GenesisConfig{}
				c.OIDC = OIDCConfig{}
			},
			wantSub: "",
		},
		{
			name: "cloud without oidc fails",
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.Genesis = GenesisConfig{}
				c.OIDC = OIDCConfig{}
			},
			wantSub: "cloud requires oidc.issuer",
		},
		{
			name: "cloud without oidc clientID fails",
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.Genesis = GenesisConfig{}
				c.OIDC = OIDCConfig{Issuer: "https://iss.example.com"}
			},
			wantSub: "cloud requires oidc.clientID",
		},
		{
			name: "cloud with sqlite driver fails",
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.Genesis = GenesisConfig{}
				c.OIDC = OIDCConfig{Issuer: "https://iss.example.com", ClientID: "c", ClientSecret: "s"}
				c.DB.Driver = "sqlite"
			},
			wantSub: "cloud requires db.driver=postgres",
		},
		{
			name: "cloud with full oidc + postgres + genesis + singleton org (no break-glass) is allowed",
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.OIDC = OIDCConfig{Issuer: "https://iss.example.com", ClientID: "c", ClientSecret: "s"}
				c.DB.Driver = "postgres"
				c.Genesis = GenesisConfig{Email: "root@example.com", Password: "s3cret", BreakGlass: true}
			},
			wantSub: "",
		},
		{
			name: "cloud with break-glass and genesis password is allowed",
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.OIDC = OIDCConfig{Issuer: "https://iss.example.com", ClientID: "c", ClientSecret: "s"}
				c.DB.Driver = "postgres"
				c.Genesis = GenesisConfig{Email: "root@example.com", Password: "s3cret", BreakGlass: true}
			},
			wantSub: "",
		},
		{
			name: "cloud with break-glass but no password fails",
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.OIDC = OIDCConfig{Issuer: "https://iss.example.com", ClientID: "c", ClientSecret: "s"}
				c.DB.Driver = "postgres"
				c.Genesis = GenesisConfig{Email: "root@example.com", BreakGlass: true}
			},
			wantSub: "'required_with'",
		},
		{
			name: "cloud without genesis email fails",
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.OIDC = OIDCConfig{Issuer: "https://iss.example.com", ClientID: "c", ClientSecret: "s"}
				c.DB.Driver = "postgres"
				c.Genesis = GenesisConfig{}
			},
			wantSub: "requires genesis.email",
		},
		{
			name: "cloud without singleton org slug fails",
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.OIDC = OIDCConfig{Issuer: "https://iss.example.com", ClientID: "c", ClientSecret: "s"}
				c.DB.Driver = "postgres"
				c.Genesis = GenesisConfig{Email: "root@example.com", Password: "s3cret", BreakGlass: true}
				c.Tenant.SingletonOrg.Slug = ""
			},
			wantSub: "singletonOrg.slug",
		},
		{
			name: "cloud without singleton org name fails",
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.OIDC = OIDCConfig{Issuer: "https://iss.example.com", ClientID: "c", ClientSecret: "s"}
				c.DB.Driver = "postgres"
				c.Genesis = GenesisConfig{Email: "root@example.com", Password: "s3cret", BreakGlass: true}
				c.Tenant.SingletonOrg.Name = ""
			},
			wantSub: "singletonOrg.name",
		},
		{
			name: "selfhosted with genesis email without password fails",
			mutate: func(c *Config) {
				c.Genesis = GenesisConfig{Email: "root@example.com"}
			},
			wantSub: "'required_with'",
		},
		{
			name: "selfhosted with genesis password without email fails",
			mutate: func(c *Config) {
				c.Genesis = GenesisConfig{Password: "x"}
			},
			wantSub: "'required_with'",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mutate(c)
			err := c.Validate()
			if tc.wantSub == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("want error containing %q, got %v", tc.wantSub, err)
			}
		})
	}
}

func TestMode_IsProduction(t *testing.T) {
	if !ModeCloud.IsProduction() {
		t.Fatal("cloud must be production")
	}
	if ModeSelfhosted.IsProduction() {
		t.Fatal("selfhosted must not be production")
	}
}

func TestLoad_RequireFile_Missing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	_, err := Load(missing, WithRequireFile())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "required") && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected required-file error, got %v", err)
	}
}

func writeTempYAML(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	return p
}

func validDB() db.DBConfig {
	return db.DBConfig{Driver: db.DriverSQLite, DSN: ":memory:"}
}

func withCwdOverride(t *testing.T, dir string) Option {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return func(*loadOptions) {}
}

func withGenesisFallback(t *testing.T) Option {
	t.Helper()
	t.Setenv("ALT_GENESIS_EMAIL", "root@example.com")
	t.Setenv("ALT_GENESIS_PASSWORD", "x")
	return func(*loadOptions) {}
}
