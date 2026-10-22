// Package auth verifies JWTs from any OpenID Connect provider locally against
// a cached JWKS and derives the request Actor (subject, tenant, role).
// Beslutning D (auth is a shared dependency, verified locally, must not degrade
// on auth downtime), Beslutning K (tenant = a token claim) and ADR 0001
// (provider-neutral claims and built-in roles). Ported from the permissions PDP.
package auth

import "context"

// Actor is the verified sender of a request.
//
// Derived from the JWT: Sub = the provider's user id, Tenant = the tenant claim
// (or the fixed tenant in single-tenant mode), Role = the role claim mapped to
// one of the four built-in roles. Handlers use Tenant to scope every content
// query — a consumer cannot spoof another tenant by setting a tenant in the
// request body/header, because it is only ever taken from the verified token or
// the server's own configuration. Tenant is "" when the claim is missing/empty;
// Role is RoleNone when the role claim is missing or unrecognized.
type Actor struct {
	Sub    string
	Tenant string
	Role   Role
}

// HasTenant reports whether a non-empty tenant was present on the JWT.
func (a Actor) HasTenant() bool { return a.Tenant != "" }

type actorContextKey struct{}

// WithActor returns a context carrying the verified Actor. Set by the JWT
// middleware, read by the HTTP handlers.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, a)
}

// ActorFromContext returns the Actor and whether one was present. When no
// middleware ran (dev without JWT configured), ok is false.
func ActorFromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorContextKey{}).(Actor)
	return a, ok
}
