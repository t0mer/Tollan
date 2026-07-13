// Package config defines Tollan's runtime configuration and its loader.
//
// Precedence (highest first): command-line flags > environment variables
// (TOLLAN_ prefix) > YAML config file > built-in defaults. Loading is driven
// by Viper with a pflag.FlagSet bound in from the CLI layer.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/t0mer/tollan/internal/input"
)

// EnvPrefix is the environment-variable prefix for all settings.
const EnvPrefix = "TOLLAN"

// Config is the fully-resolved runtime configuration.
type Config struct {
	// DataDir is the root directory for the journal, log partitions and
	// metadata database.
	DataDir string `mapstructure:"data_dir"`

	Log       LogConfig       `mapstructure:"log"`
	HTTP      HTTPConfig      `mapstructure:"http"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Journal   JournalConfig   `mapstructure:"journal"`
	Inputs    []input.Config  `mapstructure:"inputs"`
	GeoIP     GeoIPConfig     `mapstructure:"geoip"`
	Retention RetentionConfig `mapstructure:"retention"`
}

// GeoIPConfig points at an optional MaxMind/IPinfo .mmdb database.
type GeoIPConfig struct {
	DBPath string `mapstructure:"db_path"`
}

// RetentionConfig controls how long log partitions are kept.
type RetentionConfig struct {
	// Days is the global default retention; 0 disables partition-level pruning.
	Days int `mapstructure:"days"`
}

// JournalConfig bounds the disk-backed ingest journal. Zero values fall back to
// the journal package defaults.
type JournalConfig struct {
	MaxSegmentBytes int64 `mapstructure:"max_segment_bytes"`
	MaxTotalBytes   int64 `mapstructure:"max_total_bytes"`
}

// LogConfig controls structured logging.
type LogConfig struct {
	// Level is one of debug, info, warning/warn, error.
	Level string `mapstructure:"level"`
	// Format is one of text or json.
	Format string `mapstructure:"format"`
}

// HTTPConfig controls the web UI + REST API listener.
type HTTPConfig struct {
	// Addr is the bind address, e.g. ":8080" or "127.0.0.1:8080".
	Addr string `mapstructure:"addr"`
	// ReadTimeout / WriteTimeout / IdleTimeout bound the HTTP server.
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

// AuthConfig controls access control.
type AuthConfig struct {
	// Mode is "enabled" (local users + tokens) or "disabled" (open lab mode).
	Mode string `mapstructure:"mode"`
}

// Defaults returns a Config populated with the built-in defaults.
func Defaults() Config {
	return Config{
		DataDir: "./data",
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		HTTP: HTTPConfig{
			Addr:         ":8080",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Auth: AuthConfig{
			Mode: "enabled",
		},
		Retention: RetentionConfig{
			Days: 90,
		},
	}
}

// DefaultInputs returns the built-in inputs used when none are configured, so a
// fresh install can receive syslog and GELF out of the box (§4). Ports are the
// unprivileged defaults; map 514→1514 in Docker for standard syslog.
func DefaultInputs() []input.Config {
	return []input.Config{
		{ID: "syslog-udp", Type: "syslog", Bind: ":1514", Protocol: input.UDP},
		{ID: "syslog-tcp", Type: "syslog", Bind: ":1514", Protocol: input.TCP},
		{ID: "gelf-udp", Type: "gelf", Bind: ":12201", Protocol: input.UDP},
	}
}

// RegisterFlags registers the common configuration flags onto fs. The CLI layer
// binds these into Viper via Load.
func RegisterFlags(fs *pflag.FlagSet) {
	d := Defaults()
	fs.String("config", "", "path to YAML config file")
	fs.String("data-dir", d.DataDir, "root data directory (journal, logs, metadata)")
	fs.String("log-level", d.Log.Level, "log level: debug, info, warning, error")
	fs.String("log-format", d.Log.Format, "log format: text or json")
	fs.String("http-addr", d.HTTP.Addr, "HTTP UI/API bind address")
	fs.String("auth", d.Auth.Mode, "auth mode: enabled or disabled")
}

// Load resolves configuration from flags, environment and an optional YAML
// file, applying the documented precedence.
func Load(fs *pflag.FlagSet) (Config, error) {
	v := viper.New()

	// Defaults.
	d := Defaults()
	v.SetDefault("data_dir", d.DataDir)
	v.SetDefault("log.level", d.Log.Level)
	v.SetDefault("log.format", d.Log.Format)
	v.SetDefault("http.addr", d.HTTP.Addr)
	v.SetDefault("http.read_timeout", d.HTTP.ReadTimeout)
	v.SetDefault("http.write_timeout", d.HTTP.WriteTimeout)
	v.SetDefault("http.idle_timeout", d.HTTP.IdleTimeout)
	v.SetDefault("auth.mode", d.Auth.Mode)
	v.SetDefault("retention.days", d.Retention.Days)

	// Environment: TOLLAN_HTTP_ADDR, TOLLAN_LOG_LEVEL, TOLLAN_DATA_DIR, ...
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	// Bind flags with dotted config keys so precedence works.
	bindings := map[string]string{
		"data_dir":   "data-dir",
		"log.level":  "log-level",
		"log.format": "log-format",
		"http.addr":  "http-addr",
		"auth.mode":  "auth",
	}
	for key, flag := range bindings {
		if f := fs.Lookup(flag); f != nil {
			if err := v.BindPFlag(key, f); err != nil {
				return Config{}, fmt.Errorf("binding flag %q: %w", flag, err)
			}
		}
	}

	// Optional YAML config file.
	if cf, _ := fs.GetString("config"); cf != "" {
		v.SetConfigFile(cf)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("reading config file %q: %w", cf, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshalling config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks that the resolved config is internally consistent.
func (c Config) Validate() error {
	switch strings.ToLower(c.Log.Format) {
	case "text", "json":
	default:
		return fmt.Errorf("invalid log format %q (want text or json)", c.Log.Format)
	}
	switch strings.ToLower(c.Auth.Mode) {
	case "enabled", "disabled":
	default:
		return fmt.Errorf("invalid auth mode %q (want enabled or disabled)", c.Auth.Mode)
	}
	if c.DataDir == "" {
		return fmt.Errorf("data_dir must not be empty")
	}
	if c.HTTP.Addr == "" {
		return fmt.Errorf("http.addr must not be empty")
	}
	return nil
}
