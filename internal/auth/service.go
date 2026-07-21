package auth

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
)

// ScopeOrgHeader carries the tenant (org) id on a trusted service-to-service
// call. Per the [review]-endorsed invariant it is server-set by the calling
// service (e.g. Etude's Nitro, which resolved the tenant from Host→tenant /
// the end-user's JWT org-claim) and is ONLY honored on a call that is itself
// authenticated as a trusted service — it is never trusted from an end-user /
// browser request. ("X-Scope-OrgID SKAL server-sat, aldrig klient-kontrollerbart".)
const ScopeOrgHeader = "X-Scope-OrgID"

// ServiceMiddleware authenticates a trusted upstream service (the content
// delivery path) and resolves the tenant from the X-Scope-OrgID header.
//
//   - serviceToken != "": an Authorization: Bearer <serviceToken> is required
//     (constant-time compared). On success the tenant is taken from
//     X-Scope-OrgID and put on the context as the Actor. A missing/invalid
//     token → 401. This is the production shape.
//   - serviceToken == "" (dev/test only): the request passes through; if an
//     X-Scope-OrgID header is present a dev service Actor is set from it. app.Run
//     makes production boot-fail unless a token is configured, so the header is
//     never trusted unauthenticated in production (anti-spoof).
//
// The delivery path serves ONLY published content (enforced by the handlers).
func ServiceMiddleware(serviceToken string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if serviceToken == "" {
				if org := r.Header.Get(ScopeOrgHeader); org != "" {
					actor := Actor{Sub: "service-dev", Tenant: org}
					ctx := WithActor(r.Context(), actor)
					setSpanActor(ctx, actor)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			m := bearerRe.FindStringSubmatch(r.Header.Get("Authorization"))
			if m == nil || subtle.ConstantTimeCompare([]byte(m[1]), []byte(serviceToken)) != 1 {
				if logger != nil {
					logger.Warn("delivery_service_auth_failed")
				}
				unauthorized(w, "invalid service credential")
				return
			}
			actor := Actor{Sub: "service", Tenant: r.Header.Get(ScopeOrgHeader)}
			ctx := WithActor(r.Context(), actor)
			setSpanActor(ctx, actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
