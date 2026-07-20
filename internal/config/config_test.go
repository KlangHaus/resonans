package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PUBLIC_URL", "")
	t.Setenv("PORT", "")
	t.Setenv("NODE_ENV", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("default Port = %d, want 3000 (deploy contract)", cfg.Port)
	}
	if cfg.DefaultSite != "klanghaus.dk" {
		t.Errorf("DefaultSite = %q, want klanghaus.dk", cfg.DefaultSite)
	}
	if cfg.IsProduction() {
		t.Error("default NodeEnv should not be production")
	}
	if cfg.ListenAddr() != "0.0.0.0:3000" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr())
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

func TestLoadInvalidPort(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("PORT", "-1")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-positive PORT")
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
