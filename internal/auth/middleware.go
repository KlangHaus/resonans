package auth

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	tenantOTelAttr  = "tenant.id"
	subjectOTelAttr = "enduser.id"
	// devTenantHeader lets local dev pass a tenant without a real JWT. It is
	// ONLY honored when no verifier is configured (dev/test) — app.Run makes
	// production boot-fail unless a verifier is wired, so this header is never
	// trusted in production (anti-spoof).
	devTenantHeader = "X-Dev-Tenant"
)

// bearerRe matches "Bearer <token>" case-insensitively with surrounding space.
var bearerRe = regexp.MustCompile(`(?i)^bearer\s+(\S+)\s*$`)

// Middleware returns an HTTP middleware that authenticates the request and puts
// the verified Actor on the context.
//
//   - verifier != nil: an Authorization: Bearer <jwt> is required; the token is
//     verified against the JWKS and the Actor (sub + org-claim tenant) is set.
//     Missing/invalid tokens → 401.
//   - verifier == nil (dev/test only): the request passes through; if an
//     X-Dev-Tenant header is present, a dev Actor is set from it so local work
//     can exercise tenant scoping without minting JWTs.
//
// OTel spans get enduser.id + tenant.id attributes (per-span, never on the
// resource — Beslutning J2c).
func Middleware(verifier Verifier, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if verifier == nil {
				// Dev/test: optional X-Dev-Tenant header.
				// The dev actor is admin: dev auth is already the explicit,
				// never-in-production opt-in to "no checks", and a dev
				// environment that could not reach settings would only teach
				// people to work around the role table.
				if tenant := r.Header.Get(devTenantHeader); tenant != "" {
					dev := Actor{Sub: "dev-user", Tenant: tenant, Role: RoleAdmin}
					ctx := WithActor(r.Context(), dev)
					setSpanActor(ctx, dev)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			m := bearerRe.FindStringSubmatch(r.Header.Get("Authorization"))
			if m == nil {
				unauthorized(w, "missing bearer token")
				return
			}
			actor, err := verifier.Verify(r.Context(), m[1])
			if err != nil {
				if logger != nil {
					logger.Warn("jwt_verify_failed", "err", err)
				}
				unauthorized(w, "invalid token")
				return
			}
			ctx := WithActor(r.Context(), actor)
			setSpanActor(ctx, actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func setSpanActor(ctx context.Context, actor Actor) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(attribute.String(subjectOTelAttr, actor.Sub))
	if actor.HasTenant() {
		span.SetAttributes(attribute.String(tenantOTelAttr, actor.Tenant))
	}
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}
