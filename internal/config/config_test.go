package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

// newFlags returns a flag set with the config flags registered, as the CLI does.
func newFlags(t *testing.T) *pflag.FlagSet {
	t.Helper()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterFlags(fs)
	return fs
}

func TestLoadDefaults(t *testing.T) {
	fs := newFlags(t)
	cfg, err := Load(fs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("addr = %q, want :8080", cfg.HTTP.Addr)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("level = %q, want info", cfg.Log.Level)
	}
	if cfg.Auth.Mode != "enabled" {
		t.Errorf("auth = %q, want enabled", cfg.Auth.Mode)
	}
}

func TestEnvOverridesDefault(t *testing.T) {
	t.Setenv("TOLLAN_HTTP_ADDR", "127.0.0.1:9999")
	t.Setenv("TOLLAN_LOG_LEVEL", "debug")
	fs := newFlags(t)
	cfg, err := Load(fs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != "127.0.0.1:9999" {
		t.Errorf("addr = %q, want env value", cfg.HTTP.Addr)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("level = %q, want debug", cfg.Log.Level)
	}
}

func TestFlagOverridesEnv(t *testing.T) {
	t.Setenv("TOLLAN_HTTP_ADDR", "127.0.0.1:9999")
	fs := newFlags(t)
	if err := fs.Parse([]string{"--http-addr", "127.0.0.1:7000"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Load(fs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != "127.0.0.1:7000" {
		t.Errorf("addr = %q, want flag value (flags beat env)", cfg.HTTP.Addr)
	}
}

func TestYAMLBelowEnvAndFlag(t *testing.T) {
	dir := t.TempDir()
	cf := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cf, []byte("http:\n  addr: \":1111\"\nlog:\n  level: warning\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOLLAN_LOG_LEVEL", "error") // env beats yaml
	fs := newFlags(t)
	if err := fs.Parse([]string{"--config", cf}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := Load(fs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != ":1111" {
		t.Errorf("addr = %q, want yaml value", cfg.HTTP.Addr)
	}
	if cfg.Log.Level != "error" {
		t.Errorf("level = %q, want env to beat yaml", cfg.Log.Level)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	cases := map[string]Config{
		"bad log format": {DataDir: "d", HTTP: HTTPConfig{Addr: ":1"}, Log: LogConfig{Format: "xml"}, Auth: AuthConfig{Mode: "enabled"}},
		"bad auth mode":  {DataDir: "d", HTTP: HTTPConfig{Addr: ":1"}, Log: LogConfig{Format: "text"}, Auth: AuthConfig{Mode: "ldap"}},
		"empty data dir": {DataDir: "", HTTP: HTTPConfig{Addr: ":1"}, Log: LogConfig{Format: "text"}, Auth: AuthConfig{Mode: "enabled"}},
		"empty addr":     {DataDir: "d", HTTP: HTTPConfig{Addr: ""}, Log: LogConfig{Format: "text"}, Auth: AuthConfig{Mode: "enabled"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if err := c.Validate(); err == nil {
				t.Errorf("Validate() = nil, want error")
			}
		})
	}
}
