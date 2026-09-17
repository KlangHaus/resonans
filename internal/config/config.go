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
	"errors"
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
	// (https://cms.klanghaus.dk in prod). Deploy contract: PUBLIC_URL.
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
	// DevAuth is the POSITIVE opt-in for the unauthenticated dev paths:
	// X-Dev-Tenant on /api and X-Scope-OrgID without a service token on
	// /delivery. It used to be implied by NODE_ENV != "production" — and an
	// empty NODE_ENV defaults to "development", so a prod deploy that forgot one
	// variable opened both paths. A missing value must never open a gate.
	// RESONANS_DEV_AUTH=1.
	DevAuth bool

	// PreviewSecret signs short-lived preview tokens (editor draft-preview). Empty
	// disables the preview endpoints. In production it should be set (SOPS).
	PreviewSecret string

	// DeliveryToken authenticates the trusted delivery path (server-side Nuxt
	// consumers). When set, the /delivery endpoints require it and read the
	// tenant from X-Scope-OrgID. In production it MUST be set or boot fails
	// (otherwise the delivery surface would trust an unauthenticated tenant
	// header — a cross-tenant read hole).
	DeliveryToken string

	// Media / object storage (Fase 3). Bunny Storage holds asset bytes; the CDN
	// base is the pull-zone URL used to build public asset URLs. Empty storage
	// config => in-memory backend (dev/test only). VideoProvider opt-in-enables a
	// video plugin (e.g. "vimeo"); empty = disabled.
	StorageEndpoint string
	StorageZone     string
	StorageKey      string
	CDNBase         string
	VideoProvider   string

	// OTel — from day one (product brief). tenant.id is set per-request on
	// spans, never on the resource (Beslutning J2c).
	OTelServiceName      string
	OTelServiceNamespace string
	OTelEnabled          bool
}

// StorageConfigured reports whether Bunny Storage is fully configured.
func (c *Config) StorageConfigured() bool {
	return c.StorageEndpoint != "" && c.StorageZone != "" && c.StorageKey != ""
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
		// Exactly "1" opens it. Not getbool: "true", "yes" and "" stay closed, so
		// only one positively declared value can open an auth gate.
		DevAuth:       strings.TrimSpace(os.Getenv("RESONANS_DEV_AUTH")) == "1",
		DeliveryToken: getenvOptional("RESONANS_DELIVERY_TOKEN"),
		PreviewSecret: getenvOptional("RESONANS_PREVIEW_SECRET"),

		StorageEndpoint: getenvOptional("BUNNY_STORAGE_ENDPOINT"),
		StorageZone:     getenvOptional("BUNNY_STORAGE_ZONE"),
		StorageKey:      getenvOptional("BUNNY_STORAGE_KEY"),
		CDNBase:         getenv("BUNNY_CDN_BASE", "http://localhost:3000/_assets"),
		VideoProvider:   getenvOptional("RESONANS_VIDEO_PROVIDER"),

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

// ErrNoEditorAuth and ErrNoDeliveryAuth name the refusal to start with an
// unauthenticated surface that nobody opted into.
var (
	ErrNoEditorAuth   = errors.New("RESONANS_JWT_ISSUER + RESONANS_JWKS_URL not set and RESONANS_DEV_AUTH is not 1 — refusing to start with unauthenticated editor endpoints (X-Dev-Tenant)")
	ErrNoDeliveryAuth = errors.New("RESONANS_DELIVERY_TOKEN not set and RESONANS_DEV_AUTH is not 1 — refusing to start with an unauthenticated content-delivery surface (X-Scope-OrgID)")
	ErrDevAuthInProd  = errors.New("RESONANS_DEV_AUTH=1 with NODE_ENV=production — refusing to start: dev auth is never allowed in production")
)

// EditorDevAuth decides whether the editor API may run WITHOUT JWT (tenant via
// X-Dev-Tenant). JWT configured → false: the header is never read, whatever
// DevAuth says. Otherwise only an explicit RESONANS_DEV_AUTH=1 outside
// production opens it; anything else is an error, so the server does not start.
func (c *Config) EditorDevAuth() (bool, error) {
	if c.DevAuth && c.IsProduction() {
		return false, ErrDevAuthInProd
	}
	if c.AuthEnabled() {
		return false, nil
	}
	if !c.DevAuth {
		return false, ErrNoEditorAuth
	}
	return true, nil
}

// DeliveryDevAuth is the same rule for /delivery: no service token requires
// the explicit opt-in (and never in production).
func (c *Config) DeliveryDevAuth() (bool, error) {
	if c.DevAuth && c.IsProduction() {
		return false, ErrDevAuthInProd
	}
	if c.DeliveryToken != "" {
		return false, nil
	}
	if !c.DevAuth {
		return false, ErrNoDeliveryAuth
	}
	return true, nil
}

// ListenAddr returns the host:port the HTTP server binds to.
func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
