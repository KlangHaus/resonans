package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/schema"
)

func reg(t *testing.T) *schema.Registry {
	t.Helper()
	r, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	return r
}

var (
	tenantA = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tenantB = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

// seeded builds a store with: tenant A published "course/intro" + draft
// "course/wip"; tenant B published "course/intro". Returns a router (given
// service token) over that store.
func seeded(t *testing.T, serviceToken string) http.Handler {
	t.Helper()
	s := content.NewMemoryStore()
	ctx := context.Background()

	pub := func(tenant uuid.UUID, typ, slug string) {
		d, err := s.Create(ctx, tenant, content.CreateInput{Type: typ, Slug: slug, Title: slug})
		if err != nil {
			t.Fatalf("seed create: %v", err)
		}
		if _, err := s.Publish(ctx, tenant, d.ID); err != nil {
			t.Fatalf("seed publish: %v", err)
		}
	}
	pub(tenantA, "course", "intro")
	if _, err := s.Create(ctx, tenantA, content.CreateInput{Type: "course", Slug: "wip", Title: "wip"}); err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	pub(tenantB, "course", "intro")

	return NewRouter(Deps{Store: s, Registry: reg(t), ServiceToken: serviceToken})
}

func get(h http.Handler, path, token, scope string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if scope != "" {
		req.Header.Set("X-Scope-OrgID", scope)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDeliveryPublishedOnly(t *testing.T) {
	h := seeded(t, "") // dev mode: X-Scope-OrgID trusted directly

	// Catalog returns only published docs.
	rec := get(h, "/delivery/v1/course", "", tenantA.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d", rec.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 || resp.Items[0]["slug"] != "intro" {
		t.Fatalf("catalog = %+v, want only published intro summary", resp.Items)
	}
	if resp.Items[0]["title"] != "intro" || resp.Items[0]["type"] != "course" {
		t.Errorf("summary shape: %v", resp.Items[0])
	}

	// A draft is not deliverable.
	if rec := get(h, "/delivery/v1/course/wip", "", tenantA.String()); rec.Code != http.StatusNotFound {
		t.Fatalf("draft delivery = %d, want 404", rec.Code)
	}
	// A published doc is.
	if rec := get(h, "/delivery/v1/course/intro", "", tenantA.String()); rec.Code != http.StatusOK {
		t.Fatalf("published delivery = %d, want 200", rec.Code)
	}
}

func TestDeliveryTenantIsolation(t *testing.T) {
	h := seeded(t, "")
	// Tenant B must not see tenant A's documents by id/slug — each only sees
	// its own. Both have a "course/intro"; ensure scoping returns the caller's.
	recA := get(h, "/delivery/v1/course/intro", "", tenantA.String())
	recB := get(h, "/delivery/v1/course/intro", "", tenantB.String())
	if recA.Code != http.StatusOK || recB.Code != http.StatusOK {
		t.Fatalf("expected both tenants to see their own intro: A=%d B=%d", recA.Code, recB.Code)
	}
	var a, b Envelope
	_ = json.Unmarshal(recA.Body.Bytes(), &a)
	_ = json.Unmarshal(recB.Body.Bytes(), &b)
	if a.Slug != "intro" || b.Slug != "intro" {
		t.Fatalf("expected both intro: A=%s B=%s", a.Slug, b.Slug)
	}
	if a.ID == b.ID {
		t.Fatal("different tenants returned the same document (tenant scoping leaked)")
	}
}

func TestDeliveryNoScopeForbidden(t *testing.T) {
	h := seeded(t, "")
	if rec := get(h, "/delivery/v1/course", "", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("no scope = %d, want 403", rec.Code)
	}
}

func TestDeliveryServiceToken(t *testing.T) {
	h := seeded(t, "s3cret")

	// Missing token → 401.
	if rec := get(h, "/delivery/v1/course", "", tenantA.String()); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token = %d, want 401", rec.Code)
	}
	// Wrong token → 401.
	if rec := get(h, "/delivery/v1/course", "nope", tenantA.String()); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token = %d, want 401", rec.Code)
	}
	// Correct token + scope → 200.
	if rec := get(h, "/delivery/v1/course", "s3cret", tenantA.String()); rec.Code != http.StatusOK {
		t.Fatalf("valid token = %d, want 200", rec.Code)
	}
	// Correct token but no scope → 403.
	if rec := get(h, "/delivery/v1/course", "s3cret", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("token without scope = %d, want 403", rec.Code)
	}
}
