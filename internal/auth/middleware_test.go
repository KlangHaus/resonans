package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// capture is a terminal handler that records the actor it saw.
func capture(seen *Actor, ok *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, present := ActorFromContext(r.Context())
		*seen, *ok = a, present
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestMiddlewareValidJWT(t *testing.T) {
	priv, set := testKeys(t)
	v := newTestVerifier(t, set)
	token := signToken(t, priv, testIssuer, "user-1", "org-9")

	var seen Actor
	var ok bool
	h := Middleware(v, nil)(capture(&seen, &ok))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if !ok || seen.Sub != "user-1" || seen.Tenant != "org-9" {
		t.Fatalf("actor = %+v ok=%v", seen, ok)
	}
}

func TestMiddlewareMissingAndInvalid(t *testing.T) {
	_, set := testKeys(t)
	v := newTestVerifier(t, set)
	h := Middleware(v, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := map[string]string{
		"missing":  "",
		"invalid":  "Bearer not-a-jwt",
		"noprefix": "abc",
	}
	for name, hdr := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if hdr != "" {
				req.Header.Set("Authorization", hdr)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestMiddlewareDevTenantHeader(t *testing.T) {
	var seen Actor
	var ok bool
	h := Middleware(nil, nil)(capture(&seen, &ok)) // nil verifier = dev mode

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(devTenantHeader, "dev-org")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !ok || seen.Tenant != "dev-org" {
		t.Fatalf("dev actor = %+v ok=%v", seen, ok)
	}
}

func TestMiddlewareDevNoHeader(t *testing.T) {
	var seen Actor
	var ok bool
	h := Middleware(nil, nil)(capture(&seen, &ok))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if ok {
		t.Fatalf("expected no actor without header, got %+v", seen)
	}
}
