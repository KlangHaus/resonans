package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// orgClaim is the agreed claim name with [auth] (Beslutning K — tenant =
// BetterAuth organization). [auth] sets it at login when the organization
// plugin is active.
const orgClaim = "org"

// Verifier verifies a bearer token and returns the derived Actor.
type Verifier interface {
	Verify(ctx context.Context, token string) (Actor, error)
}

// VerifierConfig configures a JWKS-backed verifier.
type VerifierConfig struct {
	Issuer  string
	JWKSURL string
	// Audience is optionally asserted; skipped when empty.
	Audience string
	// KeySet is a test seam. When set, the verifier uses it directly instead of
	// fetching/caching the remote JWKS.
	KeySet jwk.Set
}

// jwksVerifier verifies tokens against a (possibly cached) JWKS.
type jwksVerifier struct {
	issuer   string
	audience string
	jwksURL  string
	// cache is the jwx auto-refreshing JWKS cache; nil when a static KeySet is
	// supplied (test seam). Keys are served from local memory and refreshed in
	// the background, so a transient auth/JWKS outage does NOT take the service
	// down (degrade-not-fail, Beslutning D).
	cache  *jwk.Cache
	keySet jwk.Set
}

// NewVerifier builds a verifier. In production it registers the JWKS URL with a
// background-refreshing cache and primes it once (a priming failure is fatal at
// boot; later refresh failures keep serving cached keys).
func NewVerifier(ctx context.Context, cfg VerifierConfig) (Verifier, error) {
	v := &jwksVerifier{
		issuer:   cfg.Issuer,
		audience: cfg.Audience,
		jwksURL:  cfg.JWKSURL,
	}

	if cfg.KeySet != nil {
		v.keySet = cfg.KeySet
		return v, nil
	}

	if cfg.Issuer == "" || cfg.JWKSURL == "" {
		return nil, errors.New("auth: issuer and jwksUrl are required")
	}

	cache := jwk.NewCache(ctx)
	if err := cache.Register(cfg.JWKSURL); err != nil {
		return nil, fmt.Errorf("auth: register jwks url: %w", err)
	}
	// Degrade-not-fail (Beslutning D): priming is best-effort. If auth/JWKS is
	// unreachable at boot we STILL return a working verifier — the public
	// /delivery path uses no JWT, and the editor JWT path recovers as soon as
	// the keys are fetched (on demand in keys() + background refresh). This
	// stops a public prod surface from crash-looping when auth is briefly down.
	_, _ = cache.Refresh(ctx, cfg.JWKSURL)
	v.cache = cache
	return v, nil
}

func (v *jwksVerifier) keys(ctx context.Context) (jwk.Set, error) {
	if v.keySet != nil {
		return v.keySet, nil
	}
	set, err := v.cache.Get(ctx, v.jwksURL)
	if err != nil {
		// Boot priming may have failed (degrade-not-fail); fetch on demand so the
		// verifier recovers once auth is reachable — no server restart needed.
		if _, rerr := v.cache.Refresh(ctx, v.jwksURL); rerr != nil {
			return nil, rerr
		}
		return v.cache.Get(ctx, v.jwksURL)
	}
	return set, nil
}

// Verify parses and validates the token (signature, issuer, optional audience,
// exp/nbf), then extracts sub (required, non-empty) and the org-claim tenant
// (empty when missing/empty/non-string).
func (v *jwksVerifier) Verify(ctx context.Context, token string) (Actor, error) {
	set, err := v.keys(ctx)
	if err != nil {
		return Actor{}, fmt.Errorf("auth: jwks unavailable: %w", err)
	}

	opts := []jwt.ParseOption{
		jwt.WithKeySet(set),
		jwt.WithValidate(true),
		jwt.WithIssuer(v.issuer),
	}
	if v.audience != "" {
		opts = append(opts, jwt.WithAudience(v.audience))
	}

	tok, err := jwt.Parse([]byte(token), opts...)
	if err != nil {
		return Actor{}, fmt.Errorf("auth: token verification failed: %w", err)
	}

	sub := tok.Subject()
	if sub == "" {
		return Actor{}, errors.New("jwt missing sub-claim")
	}

	return Actor{Sub: sub, Tenant: extractTenant(tok)}, nil
}

// extractTenant returns the org-claim as a non-empty string, or "" when it is
// absent, empty, or not a string.
func extractTenant(tok jwt.Token) string {
	raw, ok := tok.Get(orgClaim)
	if !ok {
		return ""
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return ""
	}
	return s
}
