package config

import (
	"errors"
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PUBLIC_URL", "")
	t.Setenv("PORT", "")
	t.Setenv("NODE_ENV", "")
	t.Setenv("RESONANS_JWT_ISSUER", "")
	t.Setenv("RESONANS_JWKS_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("default Port = %d, want 3000 (deploy contract)", cfg.Port)
	}
	if cfg.IsProduction() {
		t.Error("default NodeEnv should not be production")
	}
	if cfg.AuthEnabled() {
		t.Error("auth should be disabled when issuer/jwks unset")
	}
	if cfg.ListenAddr() != "0.0.0.0:3000" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr())
	}
}

func TestAuthEnabled(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("RESONANS_JWT_ISSUER", "https://auth.klanghaus.dk")
	t.Setenv("RESONANS_JWKS_URL", "https://auth.klanghaus.dk/.well-known/jwks.json")
	t.Setenv("RESONANS_JWT_AUDIENCE", "resonans")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AuthEnabled() {
		t.Error("AuthEnabled should be true when both issuer and jwks set")
	}
}

func TestLoadInvalidNodeEnv(t *testing.T) {
	t.Setenv("NODE_ENV", "staging")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid NODE_ENV")
	}
}

func TestLoadInvalidPublicURL(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("PUBLIC_URL", "not-a-url")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid PUBLIC_URL")
	}
}

func TestLoadInvalidIssuerURL(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("RESONANS_JWT_ISSUER", "not-a-url")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid RESONANS_JWT_ISSUER")
	}
}

func TestLoadProductionFlag(t *testing.T) {
	t.Setenv("NODE_ENV", "production")
	t.Setenv("PUBLIC_URL", "https://indhold.klanghaus.dk")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.IsProduction() {
		t.Error("IsProduction should be true")
	}
}

// The dev auth gate is opened ONLY by RESONANS_DEV_AUTH=1, never by the absence
// of NODE_ENV=production (riff KH-1012). An empty NODE_ENV loads as
// "development", which is exactly the deploy mistake this guards against.
func TestDevAuthRequiresPositiveOptIn(t *testing.T) {
	cases := []struct {
		name                     string
		nodeEnv, devAuth         string
		jwt, deliveryToken       bool
		wantEditor, wantDelivery bool
		wantErr                  error
	}{
		{name: "(a) empty NODE_ENV, no JWT, no opt-in", nodeEnv: "", wantErr: ErrNoEditorAuth},
		{name: "(a) development, no JWT, no opt-in", nodeEnv: "development", wantErr: ErrNoEditorAuth},
		{name: "(a) opt-in 'yes' is not an opt-in", nodeEnv: "", devAuth: "yes", wantErr: ErrNoEditorAuth},
		{name: "(a) opt-in 'true' is not an opt-in", nodeEnv: "", devAuth: "true", wantErr: ErrNoEditorAuth},
		{name: "(a) opt-in ' 1 ' (whitespace) opens", nodeEnv: "", devAuth: " 1 ", wantEditor: true, wantDelivery: true},
		{name: "(b) opt-in, no JWT", nodeEnv: "", devAuth: "1", wantEditor: true, wantDelivery: true},
		{name: "(c) JWT set: header ignored even with opt-in", nodeEnv: "development", devAuth: "1", jwt: true, deliveryToken: true},
		{name: "JWT set, no delivery token, no opt-in", nodeEnv: "development", jwt: true, wantErr: ErrNoDeliveryAuth},
		{name: "opt-in in production", nodeEnv: "production", devAuth: "1", jwt: true, deliveryToken: true, wantErr: ErrDevAuthInProd},
		{name: "production, no JWT", nodeEnv: "production", wantErr: ErrNoEditorAuth},
		{name: "production, fully configured", nodeEnv: "production", jwt: true, deliveryToken: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("NODE_ENV", c.nodeEnv)
			t.Setenv("RESONANS_DEV_AUTH", c.devAuth)
			t.Setenv("PUBLIC_URL", "")
			t.Setenv("RESONANS_JWT_ISSUER", "")
			t.Setenv("RESONANS_JWKS_URL", "")
			t.Setenv("RESONANS_DELIVERY_TOKEN", "")
			if c.jwt {
				t.Setenv("RESONANS_JWT_ISSUER", "https://auth.example")
				t.Setenv("RESONANS_JWKS_URL", "https://auth.example/jwks.json")
				t.Setenv("RESONANS_JWT_AUDIENCE", "resonans")
			}
			if c.deliveryToken {
				t.Setenv("RESONANS_DELIVERY_TOKEN", "svc")
			}
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			editor, eErr := cfg.EditorDevAuth()
			delivery, dErr := cfg.DeliveryDevAuth()
			gotErr := eErr
			if gotErr == nil {
				gotErr = dErr
			}
			if !errors.Is(gotErr, c.wantErr) {
				t.Fatalf("err = %v, want %v", gotErr, c.wantErr)
			}
			if c.wantErr == nil && (editor != c.wantEditor || delivery != c.wantDelivery) {
				t.Errorf("editor=%v delivery=%v, want %v/%v", editor, delivery, c.wantEditor, c.wantDelivery)
			}
		})
	}
}

func TestStorageBackendNameMakesProductionChooseOnPurpose(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		want    string
		wantErr error
	}{
		{"production without any storage refuses to start", Config{NodeEnv: "production"}, "", ErrNoStorage},
		{"production with a disk root picks disk", Config{NodeEnv: "production", StorageDir: "/var/lib/resonans/assets"}, "disk", nil},
		{"production with bunny picks bunny", Config{NodeEnv: "production", StorageEndpoint: "storage.bunnycdn.com", StorageZone: "z", StorageKey: "k"}, "bunny", nil},
		{"bunny wins over a disk root", Config{NodeEnv: "production", StorageDir: "/var/lib/resonans/assets", StorageEndpoint: "storage.bunnycdn.com", StorageZone: "z", StorageKey: "k"}, "bunny", nil},
		{"development without storage still gets memory", Config{NodeEnv: "development"}, "memory", nil},
		// A half-set Bunny config is the shape a deploy actually lands in when
		// one variable is forgotten. It must not count as configured.
		{"production with a partial bunny config refuses to start", Config{NodeEnv: "production", StorageEndpoint: "storage.bunnycdn.com", StorageZone: "z"}, "", ErrNoStorage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.cfg.StorageBackendName()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("backend = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCDNBaseFollowsPublicURLWhenUnset(t *testing.T) {
	t.Setenv("PUBLIC_URL", "https://cms.example.org/")
	t.Setenv("BUNNY_CDN_BASE", "") // the shape `${VAR:-}` in a compose file produces
	t.Setenv("RESONANS_DEV_AUTH", "1")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if want := "https://cms.example.org/_assets"; cfg.CDNBase != want {
		t.Fatalf("CDNBase = %q, want %q — an empty pass-through must not fall back to localhost", cfg.CDNBase, want)
	}

	t.Setenv("BUNNY_CDN_BASE", "https://cdn.example.org")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.CDNBase != "https://cdn.example.org" {
		t.Fatalf("an explicit CDN base was overwritten: %q", cfg.CDNBase)
	}
}

// The source offer is published on an unauthenticated endpoint. Today it is read
// by people, so a `javascript:` value is merely useless — the point of rejecting
// it at load is that the day somebody renders the field as a link, the question
// cannot arise. A guard placed at the sink has to be remembered; this one does not.
func TestSourceOfferMustBeAnHTTPURL(t *testing.T) {
	t.Setenv("RESONANS_DEV_AUTH", "1")

	for _, bad := range []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"file:///etc/passwd",
		"ftp://example.org/src",
		"not a url",
		"example.org/src", // no scheme: url.Parse accepts it, so it needs the Host check
	} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("RESONANS_SOURCE_URL", bad)
			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted RESONANS_SOURCE_URL=%q", bad)
			}
		})
	}

	// Positive control: without it, a Load that rejected EVERY value would pass
	// all of the above.
	for _, good := range []string{
		"https://git.example.org/our-fork",
		"http://git.internal.example/resonans",
	} {
		t.Setenv("RESONANS_SOURCE_URL", good)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load rejected a valid source URL %q: %v", good, err)
		}
		if cfg.SourceURL != good {
			t.Fatalf("SourceURL = %q, want %q", cfg.SourceURL, good)
		}
	}
}

// The JWKS is the trust anchor for every editor request. Over plain http anyone
// on the path can substitute the signing keys and mint tokens this server would
// accept — an authentication bypass, not a hygiene warning. So production demands
// https, and the waiver has to be declared rather than inferred.
//
// The development cases are the blast-radius measurement, not filler: staging
// runs NODE_ENV=development with a named http accept, and this guard must leave
// it alone.
func TestProductionDemandsHTTPSForTheTrustAnchor(t *testing.T) {
	const (
		secureJWKS   = "https://auth.example.org/.well-known/jwks.json"
		secureIssuer = "https://auth.example.org"
		plainJWKS    = "http://auth.example.org/.well-known/jwks.json"
		plainIssuer  = "http://auth.example.org"
	)
	cases := []struct {
		name          string
		nodeEnv       string
		issuer, jwks  string
		waive         string
		wantErrSubstr string
	}{
		{"production with https is fine", "production", secureIssuer, secureJWKS, "", ""},
		{"production with a plain-http JWKS is refused", "production", secureIssuer, plainJWKS, "", "RESONANS_JWKS_URL: must be https in production"},
		{"production with a plain-http issuer is refused", "production", plainIssuer, secureJWKS, "", "RESONANS_JWT_ISSUER: must be https in production"},
		// Staging. If this ever starts failing, the guard has reached past
		// production and broken the named accept it was told not to touch.
		{"development keeps its named http accept", "development", plainIssuer, plainJWKS, "", ""},
		{"the waiver opens it, deliberately", "production", plainIssuer, plainJWKS, "1", ""},
		// Only one positively declared value waives a security property.
		{"true does not waive it", "production", secureIssuer, plainJWKS, "true", "must be https in production"},
		{"yes does not waive it", "production", secureIssuer, plainJWKS, "yes", "must be https in production"},
		{"empty does not waive it", "production", secureIssuer, plainJWKS, "", "must be https in production"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NODE_ENV", tc.nodeEnv)
			t.Setenv("RESONANS_JWT_ISSUER", tc.issuer)
			t.Setenv("RESONANS_JWKS_URL", tc.jwks)
			t.Setenv("RESONANS_JWT_AUDIENCE", "resonans")
			t.Setenv("RESONANS_ALLOW_INSECURE_JWKS", tc.waive)
			t.Setenv("RESONANS_DELIVERY_TOKEN", "t")
			t.Setenv("RESONANS_STORAGE_DIR", "/tmp/resonans-assets")
			t.Setenv("DATABASE_URL", "postgres://x@localhost:5432/x")

			cfg, err := Load()
			if tc.wantErrSubstr == "" {
				if err != nil {
					t.Fatalf("Load() = %v, want no error", err)
				}
				if cfg.JWKSURL != tc.jwks {
					t.Fatalf("JWKSURL = %q, want %q", cfg.JWKSURL, tc.jwks)
				}
				return
			}
			if err == nil {
				t.Fatalf("Load() accepted %s with NODE_ENV=%s and waiver %q", tc.jwks, tc.nodeEnv, tc.waive)
			}
			if !strings.Contains(err.Error(), tc.wantErrSubstr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErrSubstr)
			}
		})
	}
}

// TestNodeEnvDefaultedDistinguishesSourceFromValue pins the one thing the value
// alone cannot say.
//
// 🔴 The discriminating pair is "declared development" vs "defaulted
// development": identical NodeEnv, opposite NodeEnvDefaulted. Without that pair
// this test would pass against a field hard-wired to `NodeEnv != "production"`,
// which is a different claim and a wrong one — staging declares development on
// purpose. Measured on platform 2026-10-06: a migrate container logged
// "env":"development" on a production box, and nothing in the config or the log
// could tell a forgotten variable from a deliberate one.
func TestNodeEnvDefaultedDistinguishesSourceFromValue(t *testing.T) {
	cases := []struct {
		name, raw     string
		wantEnv       string
		wantDefaulted bool
	}{
		{name: "unset falls back and says so", raw: "", wantEnv: "development", wantDefaulted: true},
		{name: "declared development is NOT defaulted", raw: "development", wantEnv: "development", wantDefaulted: false},
		{name: "declared production", raw: "production", wantEnv: "production", wantDefaulted: false},
		{name: "declared test", raw: "test", wantEnv: "test", wantDefaulted: false},
		// getenv trims before it decides, so whitespace is absence for the value;
		// the source has to agree, or the two fields would describe different envs.
		{name: "whitespace only is absence, like getenv treats it", raw: "   ", wantEnv: "development", wantDefaulted: true},
		{name: "declared with padding is still declared", raw: "  production  ", wantEnv: "production", wantDefaulted: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("NODE_ENV", c.raw)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.NodeEnv != c.wantEnv {
				t.Errorf("NodeEnv = %q, want %q", cfg.NodeEnv, c.wantEnv)
			}
			if cfg.NodeEnvDefaulted != c.wantDefaulted {
				t.Errorf("NodeEnvDefaulted = %v, want %v (NODE_ENV=%q)", cfg.NodeEnvDefaulted, c.wantDefaulted, c.raw)
			}
		})
	}
}
