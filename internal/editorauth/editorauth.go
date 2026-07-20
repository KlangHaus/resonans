// Package editorauth guards the write (editor) endpoints with a shared-secret
// bearer token.
//
// This is a deliberate placeholder. The canonical editor-login is BetterAuth
// JWT + PDP-gating via [auth]/[permissions] (product brief), which is a
// follow-up issue. Until then a single EDITOR_API_TOKEN bearer keeps the write
// surface closed. Read endpoints are public (published content is served to our
// Nuxt sites).
//
// Fail-closed policy (mirrors the permissions JWT invariant): in production the
// server refuses to boot when no token is configured — app.Run enforces that;
// this middleware only checks the header when a token is set.
package editorauth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Middleware returns an http middleware that requires `Authorization: Bearer
// <token>` matching the configured token. When token is empty the middleware is
// a pass-through (dev/test only — app.Run guarantees this never happens in
// production).
func Middleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !validBearer(r.Header.Get("Authorization"), token) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// validBearer performs a constant-time comparison of the presented bearer token
// against the expected value.
func validBearer(header, expected string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}
