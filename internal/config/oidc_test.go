package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/KlangHaus/resonans/internal/auth"
)

// clearAuthEnv makes a test independent of the developer's shell.
func clearAuthEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"NODE_ENV", "RESONANS_JWT_ISSUER", "RESONANS_JWKS_URL", "RESONANS_JWT_AUDIENCE",
		"RESONANS_TENANT_CLAIM", "RESONANS_ROLE_CLAIM", "RESONANS_ROLE_MAP",
		"RESONANS_TENANT_MODE", "RESONANS_TENANT_ID",
	} {
		t.Setenv(k, "")
	}
}

func withJWT(t *testing.T) {
	t.Helper()
	t.Setenv("RESONANS_JWT_ISSUER", "https://id.example")
	t.Setenv("RESONANS_JWKS_URL", "https://id.example/jwks")
}

// Both directions: JWT without an audience is refused, and the same
// configuration with one starts.
func TestAudienceIsRequiredWithJWT(t *testing.T) {
	clearAuthEnv(t)
	withJWT(t)
	if _, err := Load(); !errors.Is(err, ErrNoAudience) {
		t.Fatalf("Load without audience: err = %v, want ErrNoAudience", err)
	}
	t.Setenv("RESONANS_JWT_AUDIENCE", "resonans")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with audience: %v", err)
	}
	if cfg.JWTAudience != "resonans" {
		t.Fatalf("JWTAudience = %q", cfg.JWTAudience)
	}
}

// Dev setups without JWT need no audience: the requirement is tied to the
// surface it protects.
func TestAudienceNotRequiredWithoutJWT(t *testing.T) {
	clearAuthEnv(t)
	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestClaimDefaultsAndOverrides(t *testing.T) {
	clearAuthEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TenantClaim != "org" || cfg.RoleClaim != "resonans_role" {
		t.Fatalf("defaults: tenant=%q role=%q", cfg.TenantClaim, cfg.RoleClaim)
	}

	t.Setenv("RESONANS_TENANT_CLAIM", "tenant_id")
	t.Setenv("RESONANS_ROLE_CLAIM", "groups")
	t.Setenv("RESONANS_ROLE_MAP", "cms-admins=admin")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TenantClaim != "tenant_id" || cfg.RoleClaim != "groups" || cfg.RoleMap["cms-admins"] != auth.RoleAdmin {
		t.Fatalf("overrides not read: %+v", cfg)
	}

	t.Setenv("RESONANS_ROLE_MAP", "cms-admins=root")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "RESONANS_ROLE_MAP") {
		t.Fatalf("bad role map: err = %v", err)
	}
}

func TestTenantMode(t *testing.T) {
	const id = "11111111-1111-1111-1111-111111111111"
	cases := []struct {
		name, mode, id, wantErr, wantTenant string
	}{
		{"default is multi", "", "", "", ""},
		{"multi", "multi", "", "", ""},
		{"single with uuid", "single", id, "", id},
		{"single without id", "single", "", "needs RESONANS_TENANT_ID", ""},
		{"single with non-uuid", "single", "acme", "needs RESONANS_TENANT_ID", ""},
		{"id without single", "multi", id, "not single", ""},
		{"unknown mode", "many", "", "single or multi", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAuthEnv(t)
			t.Setenv("RESONANS_TENANT_MODE", tc.mode)
			t.Setenv("RESONANS_TENANT_ID", tc.id)
			cfg, err := Load()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.TenantID != tc.wantTenant {
				t.Fatalf("TenantID = %q, want %q", cfg.TenantID, tc.wantTenant)
			}
		})
	}
}
