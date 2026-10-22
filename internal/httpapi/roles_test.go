package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KlangHaus/resonans/internal/auth"
	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/settings"
)

// The role table is the only way onto the mux, so this is the whole surface.
// A new route without a deliberate role fails here.
func TestEveryRouteHasARole(t *testing.T) {
	routes := (&Handlers{}).Routes()
	if len(routes) != 25 {
		t.Fatalf("route table has %d routes; update this count when you add one, after choosing its role", len(routes))
	}
	seen := map[string]bool{}
	for _, rt := range routes {
		if rt.MinRole == auth.RoleNone {
			t.Errorf("%s has no role", rt.Pattern)
		}
		if seen[rt.Pattern] {
			t.Errorf("%s registered twice", rt.Pattern)
		}
		seen[rt.Pattern] = true
		// Reads are viewer. Anything that is not a GET needs more.
		isRead := strings.HasPrefix(rt.Pattern, "GET ")
		if isRead && rt.MinRole != auth.RoleViewer {
			t.Errorf("%s is a read but needs %v", rt.Pattern, rt.MinRole)
		}
		if !isRead && rt.MinRole == auth.RoleViewer {
			t.Errorf("%s writes but is open to viewer", rt.Pattern)
		}
	}
}

// fixedVerifier returns one actor for any bearer token.
type fixedVerifier struct{ actor auth.Actor }

func (f fixedVerifier) Verify(context.Context, string) (auth.Actor, error) {
	if f.actor.Sub == "" {
		return auth.Actor{}, errors.New("no")
	}
	return f.actor, nil
}

// Through the real router: the role gate sits in front of the handler, so a
// viewer is refused a settings write with 403 BEFORE the handler runs, and an
// admin reaches the handler (which then answers for itself — anything but our
// 403 body proves the gate let it through).
func TestRouterEnforcesRoles(t *testing.T) {
	const tenant = "11111111-1111-1111-1111-111111111111"
	cases := []struct {
		role       auth.Role
		method     string
		path       string
		wantGated  bool
		wantStatus int
	}{
		{auth.RoleViewer, http.MethodPut, "/api/v1/settings", true, http.StatusForbidden},
		{auth.RoleEditor, http.MethodPut, "/api/v1/settings/preview", true, http.StatusForbidden},
		{auth.RoleEditor, http.MethodPost, "/api/v1/documents/x/publish", true, http.StatusForbidden},
		{auth.RoleViewer, http.MethodPost, "/api/v1/documents", true, http.StatusForbidden},
		{auth.RoleNone, http.MethodGet, "/api/v1/schema", true, http.StatusForbidden},
		{auth.RoleAdmin, http.MethodPut, "/api/v1/settings", false, 0},
		{auth.RoleViewer, http.MethodGet, "/api/v1/schema", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.role.String()+" "+tc.method+" "+tc.path, func(t *testing.T) {
			h := NewRouter(Deps{Verifier: fixedVerifier{auth.Actor{Sub: "u", Tenant: tenant, Role: tc.role}}})
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
			r.Header.Set("Authorization", "Bearer x")
			w := httptest.NewRecorder()
			func() {
				// A handler reached with nil deps may panic; that still proves
				// the gate let the request through.
				defer func() { _ = recover() }()
				h.ServeHTTP(w, r)
			}()
			gated := w.Code == http.StatusForbidden && strings.Contains(w.Body.String(), "requires role")
			if gated != tc.wantGated {
				t.Fatalf("gated = %v (status %d, body %q), want %v", gated, w.Code, w.Body.String(), tc.wantGated)
			}
			if tc.wantGated && w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

// A role is not a tenant. An actor that passes the role gate but carries no
// tenant, or a tenant that is not a UUID, is still refused by the handler —
// the two checks are independent, and passing one must not imply the other.
func TestRoleWithoutValidTenantIsRefused(t *testing.T) {
	cases := []struct{ tenant, want string }{
		{"", "no tenant in context"},
		{"acme", "invalid tenant"},
	}
	for _, tc := range cases {
		t.Run("tenant="+tc.tenant, func(t *testing.T) {
			h := NewRouter(Deps{Verifier: fixedVerifier{auth.Actor{Sub: "u", Tenant: tc.tenant, Role: auth.RoleAdmin}}})
			r := httptest.NewRequest(http.MethodGet, "/api/v1/documents", nil)
			r.Header.Set("Authorization", "Bearer x")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), tc.want) {
				t.Fatalf("status %d body %q, want 403 %q", w.Code, w.Body.String(), tc.want)
			}
		})
	}
}

// Every route, through the real router, with an admin that has no tenant:
// none of them may answer with tenant data. The role gate passes (admin), so
// this exercises each handler's own tenant check — the line that keeps an
// org-less token out of every tenant's content.
func TestNoRouteServesATenantlessActor(t *testing.T) {
	mediaSvc := media.NewService(media.NewMemoryStore(), media.NewMemoryBackend(), media.NewURLBuilder("https://cdn.test"))
	h := NewRouter(Deps{
		Store:    content.NewMemoryStore(),
		Registry: mustRegistry(t),
		Media:    mediaSvc,
		Video:    media.NewVimeoProvider(),
		Settings: settings.NewMemoryStore(),
		Verifier: fixedVerifier{auth.Actor{Sub: "u", Role: auth.RoleAdmin}},
	})
	// Tenant-free by design: these answer about the CMS itself (a doc-type's
	// schema) or a third party (a video URL), never about a tenant's content.
	tenantFree := map[string]bool{
		"GET /api/v1/schema/{type}": true,
		"GET /api/v1/video/embed":   true,
		"GET /api/v1/video/resolve": true,
	}
	for _, rt := range (&Handlers{}).Routes() {
		if tenantFree[rt.Pattern] {
			continue
		}
		method, path, _ := strings.Cut(rt.Pattern, " ")
		path = strings.NewReplacer("{id}", "00000000-0000-0000-0000-000000000001", "{type}", "post",
			"{slug}", "hello", "{version}", "1").Replace(path)
		t.Run(rt.Pattern, func(t *testing.T) {
			r := httptest.NewRequest(method, path, strings.NewReader("{}"))
			r.Header.Set("Authorization", "Bearer x")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code < 400 {
				t.Fatalf("status %d for a tenantless actor, body %q", w.Code, w.Body.String())
			}
		})
	}
}
