package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// The delivery path's gate. With a token configured, only that exact bearer
// passes; the tenant then comes from X-Scope-OrgID, and nowhere else.
func TestServiceMiddleware(t *testing.T) {
	const token = "s3cret-service-token"
	const org = "11111111-1111-1111-1111-111111111111"
	for _, c := range []struct {
		name       string
		token      string // "" = dev mode
		authz      string
		scope      string
		wantStatus int
		wantActor  *Actor
	}{
		{"no credential", token, "", org, http.StatusUnauthorized, nil},
		{"wrong token", token, "Bearer nope", org, http.StatusUnauthorized, nil},
		{"a prefix of the token", token, "Bearer s3cret", org, http.StatusUnauthorized, nil},
		{"the token plus a suffix", token, "Bearer " + token + "x", org, http.StatusUnauthorized, nil},
		{"not a bearer", token, "Basic " + token, org, http.StatusUnauthorized, nil},
		{"valid", token, "Bearer " + token, org, http.StatusOK, &Actor{Sub: "service", Tenant: org}},
		// A valid service with no scope header is let through WITHOUT a tenant;
		// the delivery handlers refuse a tenantless actor themselves (403).
		{"valid, no scope", token, "Bearer " + token, "", http.StatusOK, &Actor{Sub: "service"}},
		{"dev, scope", "", "", org, http.StatusOK, &Actor{Sub: "service-dev", Tenant: org}},
		{"dev, no scope", "", "", "", http.StatusOK, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			var seen Actor
			var reached, hasActor bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				seen, hasActor = ActorFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})
			h := ServiceMiddleware(c.token, slog.New(slog.NewTextHandler(&logBuf, nil)))(next)
			r := httptest.NewRequest(http.MethodGet, "/delivery/v1/page", nil)
			if c.authz != "" {
				r.Header.Set("Authorization", c.authz)
			}
			if c.scope != "" {
				r.Header.Set(ScopeOrgHeader, c.scope)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if w.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, c.wantStatus)
			}
			if c.wantStatus == http.StatusUnauthorized {
				if reached {
					t.Fatal("the handler ran behind a refused credential")
				}
				if w.Header().Get("WWW-Authenticate") != "Bearer" || !strings.Contains(logBuf.String(), "delivery_service_auth_failed") {
					t.Errorf("refusal: WWW-Authenticate %q, log %q", w.Header().Get("WWW-Authenticate"), logBuf.String())
				}
				return
			}
			switch {
			case c.wantActor == nil && hasActor:
				t.Errorf("an actor was set: %+v", seen)
			case c.wantActor != nil && (!hasActor || seen != *c.wantActor):
				t.Errorf("actor = %+v (set %v), want %+v", seen, hasActor, *c.wantActor)
			}
		})
	}
}

// Degrade-not-fail (Beslutning D), all the way: auth is down at boot, so the
// verifier is built anyway; a genuine token is refused BECAUSE the keys are
// missing; and once auth answers, the same token verifies — no restart. The
// older cell only checked the first step, and its token ("not.a.jwt") was
// refused by the algorithm check before any key was asked for.
func TestVerifierRecoversWhenTheJWKSComesBack(t *testing.T) {
	priv, set := testKeys(t)
	var up atomic.Bool
	var fetches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		if !up.Load() {
			http.Error(w, "auth is down", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(set)
	}))
	defer srv.Close()

	v, err := NewVerifier(context.Background(), VerifierConfig{Issuer: testIssuer, JWKSURL: srv.URL})
	if err != nil {
		t.Fatalf("NewVerifier with auth down: %v", err)
	}
	token := signToken(t, priv, testIssuer, "user-1", "11111111-1111-1111-1111-111111111111")

	if _, err := v.Verify(context.Background(), token); err == nil || !strings.Contains(err.Error(), "jwks unavailable") {
		t.Fatalf("Verify while auth is down = %v; want a refusal because the keys are missing", err)
	}
	up.Store(true)
	actor, err := v.Verify(context.Background(), token)
	if err != nil || actor.Sub != "user-1" {
		t.Fatalf("Verify after auth came back = %+v, %v; want the same token to verify without a restart", actor, err)
	}
	if fetches.Load() < 2 {
		t.Errorf("the JWKS was fetched %d times; recovery must have fetched again", fetches.Load())
	}
}

// A genuine token without a subject names nobody, and is refused.
func TestVerifyRefusesATokenWithoutSubject(t *testing.T) {
	priv, set := testKeys(t)
	v := verifierWith(t, set, VerifierConfig{})
	tok, err := jwt.NewBuilder().Issuer(testIssuer).IssuedAt(time.Now()).Expiration(time.Now().Add(time.Hour)).Build()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, priv))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), string(signed)); err == nil || !strings.Contains(err.Error(), "sub") {
		t.Fatalf("token without sub: %v; want a refusal naming the missing subject", err)
	}
}

// A tenant claim of the wrong type is no tenant: 42 must not become "42", and
// a list must not become its first element.
func TestTenantClaimMustBeAString(t *testing.T) {
	priv, set := testKeys(t)
	v := verifierWith(t, set, VerifierConfig{})
	for _, org := range []any{42, []any{"11111111-1111-1111-1111-111111111111"}, map[string]any{"id": "x"}, ""} {
		actor, err := v.Verify(context.Background(), mint(t, priv, map[string]any{"org": org}))
		if err != nil {
			t.Fatalf("org=%v: %v", org, err)
		}
		if actor.HasTenant() {
			t.Errorf("org=%v (%T) became tenant %q", org, org, actor.Tenant)
		}
	}
}

// Non-string entries in an array role claim are skipped, not fatal: the
// highest role among the string entries wins.
func TestRoleArraySkipsNonStrings(t *testing.T) {
	priv, set := testKeys(t)
	v := verifierWith(t, set, VerifierConfig{RoleMap: map[string]Role{"member": RoleEditor}})
	actor, err := v.Verify(context.Background(), mint(t, priv, map[string]any{DefaultRoleClaim: []any{7, "member", nil}}))
	if err != nil || actor.Role != RoleEditor {
		t.Fatalf("role = %v (%v), want editor from the one string entry", actor.Role, err)
	}
}

// A refused JWT is logged by name, so an environment can be searched for it.
func TestMiddlewareLogsAFailedVerification(t *testing.T) {
	_, set := testKeys(t)
	var logBuf bytes.Buffer
	h := Middleware(verifierWith(t, set, VerifierConfig{}), slog.New(slog.NewTextHandler(&logBuf, nil)))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler ran") }))
	r := httptest.NewRequest(http.MethodGet, "/api/v1/documents", nil)
	r.Header.Set("Authorization", "Bearer not.a.jwt")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || !strings.Contains(logBuf.String(), "jwt_verify_failed") {
		t.Fatalf("status %d, log %q; want 401 and jwt_verify_failed", w.Code, logBuf.String())
	}
}

// The actor reaches the trace: the subject always, the tenant only when there
// is one (an empty tenant attribute would read as a tenant called "").
func TestSpanCarriesTheActor(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)).Tracer("test")
	for _, actor := range []Actor{{Sub: "u1", Tenant: "t1"}, {Sub: "u2"}} {
		ctx, span := tracer.Start(context.Background(), "req")
		setSpanActor(ctx, actor)
		span.End()
	}
	spans := rec.Ended()
	if len(spans) != 2 {
		t.Fatalf("%d spans recorded, want 2", len(spans))
	}
	attrs := func(i int) map[string]string {
		m := map[string]string{}
		for _, kv := range spans[i].Attributes() {
			m[string(kv.Key)] = kv.Value.AsString()
		}
		return m
	}
	if a := attrs(0); a[subjectOTelAttr] != "u1" || a[tenantOTelAttr] != "t1" {
		t.Errorf("span with tenant: %v", a)
	}
	if a := attrs(1); a[subjectOTelAttr] != "u2" {
		t.Errorf("span without tenant: %v", a)
	} else if _, ok := a[tenantOTelAttr]; ok {
		t.Errorf("span without tenant carries a tenant attribute: %v", a)
	}
}

func TestRoleEdges(t *testing.T) {
	if s := Role(99).String(); s == "" || s == "admin" {
		t.Errorf("unknown role String() = %q", s)
	}
	if m, err := ParseRoleMap("   "); err != nil || len(m) != 0 {
		t.Errorf("blank role map = %v, %v; want empty and no error", m, err)
	}
}
