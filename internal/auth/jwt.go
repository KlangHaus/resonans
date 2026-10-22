package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// Default claim names (ADR 0001 §2). `org` is what KlangHaus' own provider
// issues (Beslutning K — tenant = BetterAuth organization); both are
// overridable so any provider's tokens can be read without changing it.
const (
	DefaultTenantClaim = "org"
	DefaultRoleClaim   = "resonans_role"
)

// allowedAlgs are the signature algorithms a token may use. Asymmetric only:
// `none` carries no signature, and an HMAC algorithm would let anyone holding
// the PUBLIC key (it is in the JWKS) mint tokens if a key were ever accepted
// for the wrong algorithm. Checked on the header before any key is tried.
var allowedAlgs = map[jwa.SignatureAlgorithm]bool{
	jwa.RS256: true,
	jwa.ES256: true,
	jwa.EdDSA: true,
}

// Verifier verifies a bearer token and returns the derived Actor.
type Verifier interface {
	Verify(ctx context.Context, token string) (Actor, error)
}

// VerifierConfig configures a JWKS-backed verifier.
type VerifierConfig struct {
	Issuer  string
	JWKSURL string
	// Audience is asserted when set. Config refuses to start an authenticated
	// editor API without one (RESONANS_JWT_AUDIENCE); empty is a test seam.
	Audience string
	// TenantClaim and RoleClaim name the claims to read; empty means the
	// defaults above.
	TenantClaim string
	RoleClaim   string
	// RoleMap maps provider values (groups, organization roles) to roles. A
	// value not in the map is tried as a role name itself.
	RoleMap map[string]Role
	// FixedTenant, when set, is single-tenant mode: every actor gets this
	// tenant and the tenant claim is not read at all.
	FixedTenant string
	// KeySet is a test seam. When set, the verifier uses it directly instead of
	// fetching/caching the remote JWKS.
	KeySet jwk.Set
}

// jwksVerifier verifies tokens against a (possibly cached) JWKS.
type jwksVerifier struct {
	issuer      string
	audience    string
	jwksURL     string
	tenantClaim string
	roleClaim   string
	roleMap     map[string]Role
	fixedTenant string
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
		issuer:      cfg.Issuer,
		audience:    cfg.Audience,
		jwksURL:     cfg.JWKSURL,
		tenantClaim: cfg.TenantClaim,
		roleClaim:   cfg.RoleClaim,
		roleMap:     cfg.RoleMap,
		fixedTenant: cfg.FixedTenant,
	}
	if v.tenantClaim == "" {
		v.tenantClaim = DefaultTenantClaim
	}
	if v.roleClaim == "" {
		v.roleClaim = DefaultRoleClaim
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

// Verify parses and validates the token (algorithm, signature, issuer,
// audience, exp/nbf), then extracts sub (required, non-empty), the tenant and
// the role. A missing tenant or role is NOT an error here: the token is
// genuine, and the handlers answer 403 for what it may not do.
func (v *jwksVerifier) Verify(ctx context.Context, token string) (Actor, error) {
	if err := checkAlg(token); err != nil {
		return Actor{}, err
	}
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

	tenant := v.fixedTenant
	if tenant == "" {
		tenant = stringClaim(tok, v.tenantClaim)
	}
	return Actor{Sub: sub, Tenant: tenant, Role: v.role(tok)}, nil
}

// checkAlg refuses a token whose header names an algorithm outside allowedAlgs,
// or which carries anything but exactly one signature.
func checkAlg(token string) error {
	msg, err := jws.Parse([]byte(token))
	if err != nil {
		return fmt.Errorf("auth: malformed token: %w", err)
	}
	sigs := msg.Signatures()
	if len(sigs) != 1 {
		return fmt.Errorf("auth: token has %d signatures, want exactly 1", len(sigs))
	}
	alg := sigs[0].ProtectedHeaders().Algorithm()
	if !allowedAlgs[alg] {
		return fmt.Errorf("auth: signature algorithm %q is not allowed", alg)
	}
	return nil
}

// stringClaim returns a claim as a non-empty string, or "" when it is absent,
// empty, or not a string.
func stringClaim(tok jwt.Token, name string) string {
	raw, ok := tok.Get(name)
	if !ok {
		return ""
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return ""
	}
	return s
}

// role reads the role claim — a string or an array of strings, since providers
// emit groups as arrays — and returns the HIGHEST role any value maps to. A
// value is looked up in the role map first, then as a role name. Values that
// map to nothing are ignored; if none map, the result is RoleNone.
func (v *jwksVerifier) role(tok jwt.Token) Role {
	raw, ok := tok.Get(v.roleClaim)
	if !ok {
		return RoleNone
	}
	var values []string
	switch t := raw.(type) {
	case string:
		values = []string{t}
	case []string:
		values = t
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				values = append(values, s)
			}
		}
	}
	best := RoleNone
	for _, val := range values {
		r, mapped := v.roleMap[val]
		if !mapped {
			r, _ = ParseRole(val)
		}
		if r > best {
			best = r
		}
	}
	return best
}
