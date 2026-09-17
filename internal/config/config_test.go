package config

import (
	"errors"
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
