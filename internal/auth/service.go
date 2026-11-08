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
//   - serviceToken == "" and trustHeaderWithoutToken (dev/test only): the
//     request passes through; if an X-Scope-OrgID header is present a dev service
//     Actor is set from it. app.Run sets the flag only from DeliveryDevAuth(),
//     which requires RESONANS_DEV_AUTH=1 and is never true in production.
//   - serviceToken == "" without the flag: every request is refused with 401.
//     The middleware does not rely on the boot gate to stay closed: a caller
//     that passes an empty token by mistake gets a closed surface, not an open one.
//
// With a token configured the flag changes nothing.
//
// The delivery path serves ONLY published content (enforced by the handlers).
func ServiceMiddleware(serviceToken string, trustHeaderWithoutToken bool, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if serviceToken == "" && trustHeaderWithoutToken {
				if org := r.Header.Get(ScopeOrgHeader); org != "" {
					serveAs(next, w, r, Actor{Sub: "service-dev", Tenant: org})
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if !validServiceToken(r.Header.Get("Authorization"), serviceToken) {
				if logger != nil {
					logger.Warn("delivery_service_auth_failed")
				}
				unauthorized(w, "invalid service credential")
				return
			}
			serveAs(next, w, r, Actor{Sub: "service", Tenant: r.Header.Get(ScopeOrgHeader)})
		})
	}
}

// validServiceToken reports whether authorization is "Bearer <want>". An empty
// want is never valid, whatever the header holds, so an unset token cannot be
// matched by an empty bearer.
func validServiceToken(authorization, want string) bool {
	if want == "" {
		return false
	}
	m := bearerRe.FindStringSubmatch(authorization)
	return m != nil && subtle.ConstantTimeCompare([]byte(m[1]), []byte(want)) == 1
}

// serveAs runs next with actor on the request context and the span.
func serveAs(next http.Handler, w http.ResponseWriter, r *http.Request, actor Actor) {
	ctx := WithActor(r.Context(), actor)
	setSpanActor(ctx, actor)
	next.ServeHTTP(w, r.WithContext(ctx))
}
