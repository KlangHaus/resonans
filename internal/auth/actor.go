// Package auth verifies BetterAuth-issued JWTs locally against a cached JWKS
// and derives the request Actor (subject + tenant). Beslutning D (auth is a
// shared dependency, verified locally, must not degrade on auth downtime) and
// Beslutning K (tenant = the JWT `org` claim). Ported from the permissions PDP.
package auth

import "context"

// Actor is the verified sender of a request.
//
// Derived from the JWT (Beslutning D + K): Sub = BetterAuth user-id,
// Tenant = org-claim id (= tenant_id per Beslutning K). Handlers use Tenant to
// scope every content query — a consumer cannot spoof another tenant by setting
// a tenant in the request body/header, because it is only ever taken from the
// verified token. Tenant is "" when the org-claim is missing/empty.
type Actor struct {
	Sub    string
	Tenant string
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
