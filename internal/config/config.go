// Package config loads the Resonans CMS configuration from the process
// environment. It mirrors the hand-rolled pattern used across the KlangHaus Go
// services (permissions/notifications): "empty string means unset" coercion,
// return-error-not-os.Exit so tests decide behavior.
//
// Deploy contract (infra/compose/prod/vm2-ops): DATABASE_URL + PUBLIC_URL are
// set by Compose, Caddy reverse-proxies to resonans:3000 — so PORT defaults to
// 3000. Resonans uses its OWN content-DB (separate from Etude's money-tier-DB).
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
	// (https://indhold.klanghaus.dk in prod). Deploy contract: PUBLIC_URL.
	PublicURL string

	// DatabaseURL is the Postgres DSN for the content-DB (Beslutning K,
	// self-host). Empty => in-memory fallback (dev/test only). Deploy contract:
	// DATABASE_URL. The app connects as a non-BYPASSRLS role; migrations run as
	// a BYPASSRLS role.
	DatabaseURL string

	// JWT / JWKS (Beslutning D + K). Redaktør-login = BetterAuth-JWT verified
	// locally against the cached JWKS; tenant = the `org` claim. Both required
	// to enable auth; in production both MUST be set or boot fails.
	JWTIssuer string
	JWKSURL   string

	// OTel — from day one (product brief). tenant.id is set per-request on
	// spans, never on the resource (Beslutning J2c).
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
		DatabaseURL: getenvOptional("DATABASE_URL"),

		JWTIssuer: getenvOptional("RESONANS_JWT_ISSUER"),
		JWKSURL:   getenvOptional("RESONANS_JWKS_URL"),

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

	if cfg.JWTIssuer != "" {
		if _, err := url.ParseRequestURI(cfg.JWTIssuer); err != nil {
			return nil, fmt.Errorf("RESONANS_JWT_ISSUER: must be a valid URL: %w", err)
		}
	}
	if cfg.JWKSURL != "" {
		if _, err := url.ParseRequestURI(cfg.JWKSURL); err != nil {
			return nil, fmt.Errorf("RESONANS_JWKS_URL: must be a valid URL: %w", err)
		}
	}

	return cfg, nil
}

// IsProduction reports whether NODE_ENV == "production".
func (c *Config) IsProduction() bool { return c.NodeEnv == "production" }

// AuthEnabled reports whether the JWT middleware should be wired (both issuer
// and JWKS URL present).
func (c *Config) AuthEnabled() bool { return c.JWTIssuer != "" && c.JWKSURL != "" }

// ListenAddr returns the host:port the HTTP server binds to.
func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
