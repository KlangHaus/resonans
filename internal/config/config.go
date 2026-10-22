// Package config loads the Resonans CMS configuration from the process
// environment. It mirrors the hand-rolled pattern used across the KlangHaus Go
// services (permissions/notifications): "empty string means unset" coercion,
// return-error-not-os.Exit so tests decide behavior.
//
// Deploy contract: DATABASE_URL + PUBLIC_URL come from the environment, a
// reverse proxy forwards to port 3000 — so PORT defaults to 3000. Resonans
// expects its own database, not a schema inside somebody else's.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/auth"
)

// Config holds all runtime configuration.
type Config struct {
	NodeEnv string // development | test | production

	// NodeEnvDefaulted reports that NODE_ENV was not set, so NodeEnv above is
	// the fallback rather than a declaration.
	//
	// 🔴 This field exists because the fallback is ALSO a legitimate value:
	// staging deliberately runs NODE_ENV=development. So a deploy that forgets
	// the variable is indistinguishable, in the config and in every log line,
	// from one that chose it on purpose — measured on platform 2026-10-06,
	// where the migrate container logged "env":"development" on a production
	// box because compose declares NODE_ENV on the api service alone and each
	// service's environment list is its own. A value that is wrong but
	// plausible does not announce itself; an empty string would have.
	//
	// This is observability, not a guard: nothing branches on it, because a
	// defaulted NODE_ENV is correct for local development and the code cannot
	// tell which machine it is on. IsProduction stays the only gate.
	NodeEnvDefaulted bool
	Host             string
	Port             int
	LogLevel         string

	// PublicURL is the canonical externally-reachable base URL of this CMS
	// (https://cms.klanghaus.dk in prod). Deploy contract: PUBLIC_URL.
	PublicURL string

	// DatabaseURL is the Postgres DSN for the content-DB (Beslutning K,
	// self-host). Empty => in-memory fallback (dev/test only). Deploy contract:
	// DATABASE_URL. The app connects as a non-BYPASSRLS role; migrations run as
	// a BYPASSRLS role.
	DatabaseURL string

	// JWT / JWKS (Beslutning D + K, ADR 0001). Editor requests carry a JWT
	// from any OpenID Connect provider, verified locally against the cached
	// JWKS. Issuer + JWKS URL enable auth; in production both MUST be set or
	// boot fails. With auth enabled the audience is required too.
	JWTIssuer   string
	JWKSURL     string
	JWTAudience string

	// TenantClaim and RoleClaim name the token claims that carry the tenant and
	// the role (RESONANS_TENANT_CLAIM, default "org"; RESONANS_ROLE_CLAIM,
	// default "resonans_role"). RoleMap maps provider values to roles
	// (RESONANS_ROLE_MAP, "group=admin,other=editor").
	TenantClaim string
	RoleClaim   string
	RoleMap     map[string]auth.Role

	// TenantID is single-tenant mode (RESONANS_TENANT_MODE=single +
	// RESONANS_TENANT_ID): every editor works in this tenant and the tenant
	// claim is not read. Empty = multi-tenant, tenant from the claim.
	TenantID string
	// DevAuth is the POSITIVE opt-in for the unauthenticated dev paths:
	// X-Dev-Tenant on /api and X-Scope-OrgID without a service token on
	// /delivery. It used to be implied by NODE_ENV != "production" — and an
	// empty NODE_ENV defaults to "development", so a prod deploy that forgot one
	// variable opened both paths. A missing value must never open a gate.
	// RESONANS_DEV_AUTH=1.
	DevAuth bool

	// AllowInsecureJWKS is the positive opt-in for fetching the JWKS over plain
	// http in production. It exists because a self-hoster may legitimately reach
	// their identity provider by service name on a private network — and it is an
	// explicit variable rather than an inferred exception, because the thing being
	// waived is the integrity of the TRUST ANCHOR. RESONANS_ALLOW_INSECURE_JWKS=1.
	AllowInsecureJWKS bool

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
	// base is the pull-zone URL used to build public asset URLs. No Bunny and no
	// StorageDir => in-memory backend, which is dev/test only and refused in
	// production (ErrNoStorage): it loses every upload on restart. VideoProvider opt-in-enables a
	// video plugin (e.g. "vimeo"); empty = disabled.
	StorageEndpoint string
	StorageZone     string
	StorageKey      string
	CDNBase         string
	VideoProvider   string

	// StorageDir is the root directory of the local-disk asset backend
	// (RESONANS_STORAGE_DIR). It is the self-host path: a mounted volume and
	// nothing else, with Resonans serving the bytes at /_assets itself. Bunny
	// wins when both are set — a CDN in front is strictly better than an
	// origin serving image bytes.
	StorageDir string

	// SourceURL is where this build's Corresponding Source can be obtained.
	// AGPL-3.0 §13 obliges whoever OFFERS Resonans to users over a network to
	// give those users its source — so a self-hoster who modified the code has
	// to be able to point at THEIR repository, not ours. Defaulting to upstream
	// and leaving it overridable is what makes the obligation satisfiable by
	// someone other than us; hardcoding our URL would quietly make every
	// modified deployment misdescribe itself.
	SourceURL string

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
		NodeEnv: getenv("NODE_ENV", "development"),
		// Measured against the same helper the value came from, so a whitespace-only
		// NODE_ENV counts as absent here exactly as getenv treats it as absent above.
		NodeEnvDefaulted: getenvOptional("NODE_ENV") == "",
		Host:             getenv("HOST", "0.0.0.0"),
		Port:             getint("PORT", 3000),
		LogLevel:         getenv("LOG_LEVEL", "info"),

		PublicURL:   getenv("PUBLIC_URL", "http://localhost:3000"),
		DatabaseURL: getenvOptional("DATABASE_URL"),

		JWTIssuer:   getenvOptional("RESONANS_JWT_ISSUER"),
		JWKSURL:     getenvOptional("RESONANS_JWKS_URL"),
		JWTAudience: getenvOptional("RESONANS_JWT_AUDIENCE"),
		TenantClaim: getenv("RESONANS_TENANT_CLAIM", auth.DefaultTenantClaim),
		RoleClaim:   getenv("RESONANS_ROLE_CLAIM", auth.DefaultRoleClaim),
		// Exactly "1" opens it. Not getbool: "true", "yes" and "" stay closed, so
		// only one positively declared value can open an auth gate.
		DevAuth:       strings.TrimSpace(os.Getenv("RESONANS_DEV_AUTH")) == "1",
		DeliveryToken: getenvOptional("RESONANS_DELIVERY_TOKEN"),
		// Exactly "1", like DevAuth: "true", "yes" and "" all stay closed, so only
		// one positively declared value can waive a security property.
		AllowInsecureJWKS: strings.TrimSpace(os.Getenv("RESONANS_ALLOW_INSECURE_JWKS")) == "1",
		PreviewSecret:     getenvOptional("RESONANS_PREVIEW_SECRET"),

		StorageEndpoint: getenvOptional("BUNNY_STORAGE_ENDPOINT"),
		StorageZone:     getenvOptional("BUNNY_STORAGE_ZONE"),
		StorageKey:      getenvOptional("BUNNY_STORAGE_KEY"),
		CDNBase:         getenvOptional("BUNNY_CDN_BASE"),
		StorageDir:      getenvOptional("RESONANS_STORAGE_DIR"),
		VideoProvider:   getenvOptional("RESONANS_VIDEO_PROVIDER"),

		SourceURL: getenv("RESONANS_SOURCE_URL", "https://github.com/KlangHaus/resonans"),

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

	roleMap, err := auth.ParseRoleMap(getenvOptional("RESONANS_ROLE_MAP"))
	if err != nil {
		return nil, fmt.Errorf("RESONANS_ROLE_MAP: %w", err)
	}
	cfg.RoleMap = roleMap

	if err := cfg.loadTenantMode(); err != nil {
		return nil, err
	}

	// The JWKS is the TRUST ANCHOR for every editor request. Fetched over plain
	// http, anyone on the path can substitute the signing keys and mint tokens
	// this server would accept — which is an authentication bypass, not a hygiene
	// warning. The issuer is checked with it because a token is only as trustworthy
	// as the pair.
	//
	// Production must therefore be https, or say out loud that it is not. Staging
	// runs NODE_ENV=development and keeps its named http accept untouched; this
	// only ever fires for production.
	if cfg.IsProduction() && !cfg.AllowInsecureJWKS {
		for _, f := range []struct{ name, raw string }{
			{"RESONANS_JWKS_URL", cfg.JWKSURL},
			{"RESONANS_JWT_ISSUER", cfg.JWTIssuer},
		} {
			if f.raw == "" {
				continue
			}
			u, err := url.Parse(f.raw)
			if err != nil || u.Scheme != "https" {
				return nil, fmt.Errorf(
					"%s: must be https in production, got %q — the JWKS is the trust anchor, so plain http lets anyone on the path substitute the signing keys. Set RESONANS_ALLOW_INSECURE_JWKS=1 to waive this deliberately",
					f.name, f.raw)
			}
		}
	}

	// Without an audience, any valid token from the issuer is accepted —
	// including one the provider minted for a different service. Measured on
	// f2114a9: Audience existed in the verifier and nothing ever set it.
	if cfg.AuthEnabled() && cfg.JWTAudience == "" {
		return nil, ErrNoAudience
	}

	// The source offer is published on /healthz. Today it is a JSON string read
	// by people, so a scheme other than http(s) is merely useless — but the day
	// anyone renders it as a link, `javascript:` or `data:` becomes a real XSS
	// path. Validating the scheme here makes that question impossible to ask
	// later, which is cheaper than remembering the constraint at the sink.
	if u, err := url.Parse(cfg.SourceURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("RESONANS_SOURCE_URL: must be an http(s) URL, got %q", cfg.SourceURL)
	}

	// An unset CDN base follows PUBLIC_URL to Resonans' own /_assets route
	// (media.AssetPath). It used to default to a hardcoded localhost:3000,
	// which a deploy inherits whenever it passes BUNNY_CDN_BASE through as an
	// empty string: every public asset URL then pointed at the VISITOR's own
	// machine, and the page just showed a broken image.
	if cfg.CDNBase == "" {
		cfg.CDNBase = strings.TrimRight(cfg.PublicURL, "/") + "/_assets"
	}

	return cfg, nil
}

// loadTenantMode reads RESONANS_TENANT_MODE and RESONANS_TENANT_ID. The two
// must agree: single needs a tenant id, and an id without single would be a
// setting that looks like it does something and does not.
func (c *Config) loadTenantMode() error {
	mode := getenv("RESONANS_TENANT_MODE", "multi")
	id := getenvOptional("RESONANS_TENANT_ID")
	switch mode {
	case "multi":
		if id != "" {
			return errors.New("RESONANS_TENANT_ID is set but RESONANS_TENANT_MODE is not single — set RESONANS_TENANT_MODE=single, or remove the id")
		}
		return nil
	case "single":
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("RESONANS_TENANT_MODE=single needs RESONANS_TENANT_ID to be a UUID, got %q", id)
		}
		c.TenantID = id
		return nil
	default:
		return fmt.Errorf("RESONANS_TENANT_MODE: must be single or multi, got %q", mode)
	}
}

// IsProduction reports whether NODE_ENV == "production".
func (c *Config) IsProduction() bool { return c.NodeEnv == "production" }

// AuthEnabled reports whether the JWT middleware should be wired (both issuer
// and JWKS URL present).
func (c *Config) AuthEnabled() bool { return c.JWTIssuer != "" && c.JWKSURL != "" }

// ErrNoEditorAuth and ErrNoDeliveryAuth name the refusal to start with an
// unauthenticated surface that nobody opted into.
var (
	ErrNoAudience     = errors.New("RESONANS_JWT_AUDIENCE not set while JWT auth is configured — refusing to start: without an audience the editor API accepts any token the issuer minted, for any service")
	ErrNoEditorAuth   = errors.New("RESONANS_JWT_ISSUER + RESONANS_JWKS_URL not set and RESONANS_DEV_AUTH is not 1 — refusing to start with unauthenticated editor endpoints (X-Dev-Tenant)")
	ErrNoDeliveryAuth = errors.New("RESONANS_DELIVERY_TOKEN not set and RESONANS_DEV_AUTH is not 1 — refusing to start with an unauthenticated content-delivery surface (X-Scope-OrgID)")
	ErrDevAuthInProd  = errors.New("RESONANS_DEV_AUTH=1 with NODE_ENV=production — refusing to start: dev auth is never allowed in production")
	ErrNoStorage      = errors.New("neither BUNNY_STORAGE_* nor RESONANS_STORAGE_DIR set with NODE_ENV=production — refusing to start: the in-memory asset backend loses every upload on restart")
)

// StorageBackendName picks the asset backend and names it. Production must
// choose one positively: the old behavior logged a warning and ran on the
// in-memory backend, so a forgotten variable produced a CMS that accepted
// uploads, showed them to the editor, and dropped them at the next restart —
// a failure that only surfaces long after the deploy anyone could connect it to.
func (c *Config) StorageBackendName() (string, error) {
	switch {
	case c.StorageConfigured():
		return "bunny", nil
	case c.StorageDir != "":
		return "disk", nil
	case c.IsProduction():
		return "", ErrNoStorage
	default:
		return "memory", nil
	}
}

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
