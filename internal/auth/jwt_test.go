package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const testIssuer = "https://auth.klanghaus.dk"

// testKeys generates an RSA keypair and returns the signing key + a JWKS set
// holding the public key (for the verifier).
func testKeys(t *testing.T) (jwk.Key, jwk.Set) {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa: %v", err)
	}
	priv, err := jwk.FromRaw(raw)
	if err != nil {
		t.Fatalf("priv jwk: %v", err)
	}
	_ = priv.Set(jwk.KeyIDKey, "test-key")
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256)

	pub, err := jwk.FromRaw(raw.PublicKey)
	if err != nil {
		t.Fatalf("pub jwk: %v", err)
	}
	_ = pub.Set(jwk.KeyIDKey, "test-key")
	_ = pub.Set(jwk.AlgorithmKey, jwa.RS256)

	set := jwk.NewSet()
	if err := set.AddKey(pub); err != nil {
		t.Fatalf("add key: %v", err)
	}
	return priv, set
}

func signToken(t *testing.T, priv jwk.Key, issuer, sub, org string) string {
	t.Helper()
	b := jwt.NewBuilder().
		Issuer(issuer).
		Subject(sub).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour))
	if org != "" {
		b = b.Claim(orgClaim, org)
	}
	tok, err := b.Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, priv))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return string(signed)
}

func newTestVerifier(t *testing.T, set jwk.Set) Verifier {
	t.Helper()
	v, err := NewVerifier(context.Background(), VerifierConfig{Issuer: testIssuer, KeySet: set})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return v
}

func TestVerifyValid(t *testing.T) {
	priv, set := testKeys(t)
	v := newTestVerifier(t, set)

	token := signToken(t, priv, testIssuer, "user-123", "org-abc")
	actor, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if actor.Sub != "user-123" {
		t.Errorf("Sub = %q", actor.Sub)
	}
	if actor.Tenant != "org-abc" {
		t.Errorf("Tenant = %q, want org-abc", actor.Tenant)
	}
	if !actor.HasTenant() {
		t.Error("HasTenant should be true")
	}
}

func TestVerifyNoOrgClaim(t *testing.T) {
	priv, set := testKeys(t)
	v := newTestVerifier(t, set)

	token := signToken(t, priv, testIssuer, "user-123", "")
	actor, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if actor.HasTenant() {
		t.Errorf("Tenant should be empty, got %q", actor.Tenant)
	}
}

func TestVerifyWrongIssuer(t *testing.T) {
	priv, set := testKeys(t)
	v := newTestVerifier(t, set)

	token := signToken(t, priv, "https://evil.example.com", "user-123", "org-abc")
	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("expected verification failure for wrong issuer")
	}
}

func TestVerifyGarbageToken(t *testing.T) {
	_, set := testKeys(t)
	v := newTestVerifier(t, set)
	if _, err := v.Verify(context.Background(), "not-a-jwt"); err == nil {
		t.Fatal("expected error for garbage token")
	}
}

func TestNewVerifierRequiresConfig(t *testing.T) {
	if _, err := NewVerifier(context.Background(), VerifierConfig{}); err == nil {
		t.Fatal("expected error when issuer/jwks missing and no keyset")
	}
}
