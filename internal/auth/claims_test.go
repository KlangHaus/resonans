package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const testAudience = "resonans"

// mint signs a token with arbitrary claims. iss, sub, iat and exp are filled in
// unless the claims set them.
func mint(t *testing.T, priv jwk.Key, claims map[string]any) string {
	t.Helper()
	b := jwt.NewBuilder().
		Issuer(testIssuer).
		Subject("user-1").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour))
	for k, v := range claims {
		b = b.Claim(k, v)
	}
	tok, err := b.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, priv))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return string(signed)
}

func verifierWith(t *testing.T, set jwk.Set, cfg VerifierConfig) Verifier {
	t.Helper()
	cfg.Issuer = testIssuer
	cfg.KeySet = set
	v, err := NewVerifier(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return v
}

// The audience gap this change closes: with an audience configured, a token
// minted for another service must be refused, and the right one accepted —
// both directions in the same test, so a verifier that refuses everything
// cannot pass.
func TestAudienceIsEnforced(t *testing.T) {
	priv, set := testKeys(t)
	v := verifierWith(t, set, VerifierConfig{Audience: testAudience})

	if _, err := v.Verify(context.Background(), mint(t, priv, map[string]any{"aud": testAudience})); err != nil {
		t.Fatalf("token for this audience refused: %v", err)
	}
	if _, err := v.Verify(context.Background(), mint(t, priv, map[string]any{"aud": "permissions"})); err == nil {
		t.Fatal("token for another service accepted")
	}
	if _, err := v.Verify(context.Background(), mint(t, priv, nil)); err == nil {
		t.Fatal("token with no audience accepted")
	}
}

// An HMAC token must be refused on the header, before any key is tried. The
// control is the same claims signed RS256, which passes.
func TestOnlyAsymmetricAlgorithmsAreAccepted(t *testing.T) {
	priv, set := testKeys(t)
	v := verifierWith(t, set, VerifierConfig{})

	tok, _ := jwt.NewBuilder().Issuer(testIssuer).Subject("u").Expiration(time.Now().Add(time.Hour)).Build()
	hs, err := jwt.Sign(tok, jwt.WithKey(jwa.HS256, []byte("a-shared-secret-of-sufficient-length")))
	if err != nil {
		t.Fatalf("sign hs256: %v", err)
	}
	_, err = v.Verify(context.Background(), string(hs))
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("HS256 token: err = %v, want refusal naming the algorithm", err)
	}
	if _, err := v.Verify(context.Background(), mint(t, priv, nil)); err != nil {
		t.Fatalf("control RS256 token refused: %v", err)
	}
}

func TestTenantClaimIsConfigurable(t *testing.T) {
	priv, set := testKeys(t)
	v := verifierWith(t, set, VerifierConfig{TenantClaim: "tenant_id"})

	a, err := v.Verify(context.Background(), mint(t, priv, map[string]any{"tenant_id": "t-1", "org": "o-1"}))
	if err != nil {
		t.Fatal(err)
	}
	if a.Tenant != "t-1" {
		t.Fatalf("tenant = %q, want the configured claim t-1 (not org)", a.Tenant)
	}
}

// Single-tenant mode ignores the claim entirely: a token naming a different
// tenant still lands in the configured one.
func TestFixedTenantIgnoresTheClaim(t *testing.T) {
	priv, set := testKeys(t)
	const fixed = "11111111-1111-1111-1111-111111111111"
	v := verifierWith(t, set, VerifierConfig{FixedTenant: fixed})

	a, err := v.Verify(context.Background(), mint(t, priv, map[string]any{"org": "22222222-2222-2222-2222-222222222222"}))
	if err != nil {
		t.Fatal(err)
	}
	if a.Tenant != fixed {
		t.Fatalf("tenant = %q, want fixed %q", a.Tenant, fixed)
	}
}

func TestRoleFromClaim(t *testing.T) {
	priv, set := testKeys(t)
	roleMap, err := ParseRoleMap("cms-admins=admin,member=editor")
	if err != nil {
		t.Fatal(err)
	}
	v := verifierWith(t, set, VerifierConfig{RoleClaim: "roles", RoleMap: roleMap})

	cases := []struct {
		name  string
		claim any
		want  Role
	}{
		{"role name as string", "publisher", RolePublisher},
		{"mapped provider value", "cms-admins", RoleAdmin},
		{"array takes the highest", []string{"viewer", "member"}, RoleEditor},
		{"unknown value is none", "superuser", RoleNone},
		{"array of unknowns is none", []string{"x", "y"}, RoleNone},
		{"non-string is none", 7, RoleNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := v.Verify(context.Background(), mint(t, priv, map[string]any{"roles": tc.claim}))
			if err != nil {
				t.Fatal(err)
			}
			if a.Role != tc.want {
				t.Fatalf("role = %v, want %v", a.Role, tc.want)
			}
		})
	}

	a, err := v.Verify(context.Background(), mint(t, priv, nil))
	if err != nil {
		t.Fatal(err)
	}
	if a.Role != RoleNone {
		t.Fatalf("missing role claim gave %v, want none", a.Role)
	}
}

func TestParseRoleMapRefusesTypos(t *testing.T) {
	for _, bad := range []string{"a=superuser", "a", "=admin", "a=", "a=admin,a=editor"} {
		if _, err := ParseRoleMap(bad); err == nil {
			t.Errorf("ParseRoleMap(%q) accepted", bad)
		}
	}
	m, err := ParseRoleMap(" a = admin , b=viewer ")
	if err != nil || m["a"] != RoleAdmin || m["b"] != RoleViewer || len(m) != 2 {
		t.Fatalf("valid map: %v %v", m, err)
	}
}

func TestRequireRole(t *testing.T) {
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	h := RequireRole(RoleEditor, ok)

	cases := []struct {
		name   string
		actor  *Actor
		status int
	}{
		{"no actor", nil, http.StatusForbidden},
		{"none", &Actor{Sub: "u", Role: RoleNone}, http.StatusForbidden},
		{"viewer below editor", &Actor{Sub: "u", Role: RoleViewer}, http.StatusForbidden},
		{"editor", &Actor{Sub: "u", Role: RoleEditor}, http.StatusNoContent},
		{"admin includes editor", &Actor{Sub: "u", Role: RoleAdmin}, http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.actor != nil {
				r = r.WithContext(WithActor(r.Context(), *tc.actor))
			}
			w := httptest.NewRecorder()
			h(w, r)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
		})
	}

	// A route that asks for RoleNone is unreachable, even for admin.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	RequireRole(RoleNone, ok)(w, r.WithContext(WithActor(r.Context(), Actor{Sub: "u", Role: RoleAdmin})))
	if w.Code != http.StatusForbidden {
		t.Fatalf("RoleNone route reachable: %d", w.Code)
	}
}
