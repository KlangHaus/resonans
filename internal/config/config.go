// Package config loads the Resonans CMS configuration from the process
// environment. It mirrors the hand-rolled pattern used across the KlangHaus Go
// services (permissions/notifications): same "empty string means unset"
// coercion, same return-error-not-os.Exit policy so tests decide behavior.
//
// The env-var names honor the VM2 deploy contract in
// infra/compose/prod/vm2-ops/compose.yml: DATABASE_URL + PUBLIC_URL are set by
// Compose, and Caddy reverse-proxies to resonans:3000 — so the server MUST
// default to port 3000 when PORT is unset.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration.
type Config struct {
	NodeEnv  string // development | test | production
	Host     string
	Port     int
	LogLevel string

	// PublicURL is the canonical externally-reachable base URL of this CMS
	// (https://indhold.klanghaus.dk in prod). Surfaced in API responses so
	// consumers can build absolute links. Deploy contract: PUBLIC_URL.
	PublicURL string

	// DefaultSite scopes content when a request omits an explicit site. Beslutning
	// B (this session): single-site now — klanghaus.dk — but the content model
	// carries a `site` column so multi-site + RLS can land later without a
	// migration rewrite.
	DefaultSite string

	// DatabaseURL is the Postgres DSN for content storage (Beslutning K,
	// self-host). Empty => in-memory fallback (dev/test only). Migrations run at
	// boot. Deploy contract: DATABASE_URL.
	DatabaseURL string

	// EditorToken is a shared-secret bearer guarding write endpoints until the
	// BetterAuth-JWT + PDP editor-login lands (follow-up issue). Empty in dev =>
	// writes allowed with a boot-warning; empty in prod => boot fails
	// (fail-closed, mirrors the permissions JWT invariant).
	EditorToken string

	// OTel — emitted from day one (product brief). tenant.id/site is set
	// per-request on spans, never on the resource (high cardinality).
	OTelServiceName      string
	OTelServiceNamespace string
	OTelEnabled          bool
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getenvOptional(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func getint(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getbool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v == "true" || v == "1"
}

// Load reads and validates configuration from the environment.
func Load() (*Config, error) {
	cfg := &Config{
		NodeEnv:  getenv("NODE_ENV", "development"),
		Host:     getenv("HOST", "0.0.0.0"),
		Port:     getint("PORT", 3000),
		LogLevel: getenv("LOG_LEVEL", "info"),

		PublicURL:   getenv("PUBLIC_URL", "http://localhost:3000"),
		DefaultSite: getenv("RESONANS_DEFAULT_SITE", "klanghaus.dk"),
		DatabaseURL: getenvOptional("DATABASE_URL"),
		EditorToken: getenvOptional("EDITOR_API_TOKEN"),

		OTelServiceName:      getenv("OTEL_SERVICE_NAME", "klanghaus-resonans"),
		OTelServiceNamespace: getenv("OTEL_SERVICE_NAMESPACE", "klanghaus"),
		OTelEnabled:          getbool("OTEL_ENABLED", false),
	}

	switch cfg.NodeEnv {
	case "development", "test", "production":
	default:
		return nil, fmt.Errorf("NODE_ENV: must be one of development|test|production, got %q", cfg.NodeEnv)
	}

	if cfg.Port <= 0 {
		return nil, fmt.Errorf("PORT: must be a positive integer, got %d", cfg.Port)
	}

	if _, err := url.ParseRequestURI(cfg.PublicURL); err != nil {
		return nil, fmt.Errorf("PUBLIC_URL: must be a valid URL: %w", err)
	}

	if cfg.DefaultSite == "" {
		return nil, fmt.Errorf("RESONANS_DEFAULT_SITE: must not be empty")
	}

	return cfg, nil
}

// IsProduction reports whether NODE_ENV == "production".
func (c *Config) IsProduction() bool { return c.NodeEnv == "production" }

// ListenAddr returns the host:port the HTTP server binds to.
func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
