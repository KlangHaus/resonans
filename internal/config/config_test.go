package config

import "testing"

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
