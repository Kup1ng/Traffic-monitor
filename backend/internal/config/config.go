// Package config loads runtime configuration from environment variables and
// command-line flags, with sensible defaults. There is no config-file parser:
// the systemd unit supplies values via an EnvironmentFile, and flags override
// the environment for ad-hoc/dev runs. Secrets (admin password hash, session
// secret) are NOT stored here — they live in the SQLite database.
package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all process configuration for the `serve` command.
type Config struct {
	Listen        string         // host:port to listen on
	Interface     string         // interface to monitor; "" means auto-detect the default route
	DBPath        string         // path to the SQLite database file
	TZ            string         // timezone name for day/month aggregation; "" means system local
	Location      *time.Location // resolved from TZ
	PollInterval  time.Duration  // live sampling interval
	FlushInterval time.Duration  // DB flush interval
	SessionTTL    time.Duration  // session cookie lifetime
	CookieSecure  string         // "auto" | "true" | "false"
	Demo          bool           // use synthetic counters (UI development on any OS)
	BasePath      string         // secret base path the whole app is served under; "/" = root
	TrustProxy    bool           // trust X-Forwarded-For (enable only behind a trusted reverse proxy)
}

// Default returns the built-in defaults before env/flag overrides are applied.
func Default() *Config {
	return &Config{
		Listen:        "0.0.0.0:8088",
		Interface:     "",
		DBPath:        "traffic.db",
		TZ:            "",
		PollInterval:  2 * time.Second,
		FlushInterval: 60 * time.Second,
		SessionTTL:    7 * 24 * time.Hour,
		CookieSecure:  "auto",
		Demo:          false,
		BasePath:      "/",
		TrustProxy:    false,
	}
}

// Load builds a Config from defaults, then environment variables (TM_*), then
// the provided flag arguments (which take precedence).
func Load(args []string) (*Config, error) {
	c := Default()
	c.applyEnv()

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.StringVar(&c.Listen, "listen", c.Listen, "listen address as host:port")
	fs.StringVar(&c.Interface, "iface", c.Interface, "network interface to monitor (default: auto-detect default route)")
	fs.StringVar(&c.DBPath, "db", c.DBPath, "path to the SQLite database file")
	fs.StringVar(&c.TZ, "tz", c.TZ, "timezone for day/month aggregation (default: system local)")
	fs.DurationVar(&c.PollInterval, "poll", c.PollInterval, "live sampling interval")
	fs.DurationVar(&c.FlushInterval, "flush", c.FlushInterval, "database flush interval")
	fs.DurationVar(&c.SessionTTL, "session-ttl", c.SessionTTL, "session cookie lifetime")
	fs.StringVar(&c.CookieSecure, "cookie-secure", c.CookieSecure, `Secure cookie flag: "auto", "true" or "false"`)
	fs.BoolVar(&c.Demo, "demo", c.Demo, "use synthetic counters instead of real interface statistics")
	fs.StringVar(&c.BasePath, "base-path", c.BasePath, "secret base path to serve the app under (default: root)")
	fs.BoolVar(&c.TrustProxy, "trust-proxy", c.TrustProxy, "trust the X-Forwarded-For header (only behind a trusted reverse proxy)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	c.BasePath = NormalizeBasePath(c.BasePath)

	if err := c.resolveLocation(); err != nil {
		return nil, err
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) applyEnv() {
	if v := os.Getenv("TM_LISTEN"); v != "" {
		c.Listen = v
	}
	if v := os.Getenv("TM_INTERFACE"); v != "" {
		c.Interface = v
	}
	if v := os.Getenv("TM_DB"); v != "" {
		c.DBPath = v
	}
	if v := os.Getenv("TM_TZ"); v != "" {
		c.TZ = v
	}
	if v := os.Getenv("TM_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.PollInterval = d
		}
	}
	if v := os.Getenv("TM_FLUSH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.FlushInterval = d
		}
	}
	if v := os.Getenv("TM_SESSION_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.SessionTTL = d
		}
	}
	if v := os.Getenv("TM_COOKIE_SECURE"); v != "" {
		c.CookieSecure = v
	}
	if v := os.Getenv("TM_DEMO"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Demo = b
		}
	}
	if v := os.Getenv("TM_BASE_PATH"); v != "" {
		c.BasePath = v
	}
	if v := os.Getenv("TM_TRUST_PROXY"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.TrustProxy = b
		}
	}
}

// NormalizeBasePath returns a base path with exactly one leading and trailing
// slash (e.g. "abc" or "/abc/" -> "/abc/"). Empty or "/" returns "/".
func NormalizeBasePath(s string) string {
	s = strings.Trim(strings.TrimSpace(s), "/")
	if s == "" {
		return "/"
	}
	return "/" + s + "/"
}

func (c *Config) resolveLocation() error {
	if c.TZ == "" {
		c.Location = time.Local
		return nil
	}
	loc, err := time.LoadLocation(c.TZ)
	if err != nil {
		return fmt.Errorf("invalid timezone %q: %w", c.TZ, err)
	}
	c.Location = loc
	return nil
}

func (c *Config) validate() error {
	if !strings.Contains(c.Listen, ":") {
		return fmt.Errorf("listen %q must be in host:port form", c.Listen)
	}
	if c.PollInterval <= 0 {
		return fmt.Errorf("poll interval must be positive")
	}
	if c.FlushInterval < c.PollInterval {
		return fmt.Errorf("flush interval (%s) must be >= poll interval (%s)", c.FlushInterval, c.PollInterval)
	}
	switch c.CookieSecure {
	case "auto", "true", "false":
	default:
		return fmt.Errorf("cookie-secure must be auto, true or false (got %q)", c.CookieSecure)
	}
	if err := validateBasePath(c.BasePath); err != nil {
		return err
	}
	return nil
}

// validateBasePath rejects base paths that would make http.ServeMux panic when
// registering route patterns (unclean ".."/"." segments, spaces, "{"/"}", etc.).
// The value is already normalized to "/" or "/seg.../".
func validateBasePath(base string) error {
	if base == "/" {
		return nil
	}
	for _, seg := range strings.Split(strings.Trim(base, "/"), "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("base path %q must not contain empty, %q or %q segments", base, ".", "..")
		}
		for _, r := range seg {
			ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
				r == '.' || r == '_' || r == '~' || r == '-'
			if !ok {
				return fmt.Errorf("base path %q contains invalid character %q (allowed: letters, digits, . _ ~ -)", base, string(r))
			}
		}
	}
	return nil
}
