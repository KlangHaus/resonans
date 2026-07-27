package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/schema"
)

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

func mustRegistry(t *testing.T) *schema.Registry {
	t.Helper()
	r, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	return r
}

// testRouter builds a router in dev mode (nil verifier → X-Dev-Tenant header).
func testRouter(t *testing.T) http.Handler {
	mediaSvc := media.NewService(media.NewMemoryStore(), media.NewMemoryBackend(), media.NewURLBuilder("https://cdn.test"))
	return NewRouter(Deps{
		Store:    content.NewMemoryStore(),
		Registry: mustRegistry(t),
		Media:    mediaSvc,
		Video:    media.VimeoProvider{},
	})
}

func do(t *testing.T, h http.Handler, method, path, tenant string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if tenant != "" {
		req.Header.Set("X-Dev-Tenant", tenant)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreatePublishReadFlow(t *testing.T) {
	h := testRouter(t)

	rec := do(t, h, http.MethodPost, "/api/v1/documents", tenantA, map[string]any{
		"type": "lesson", "slug": "home", "title": "Home",
		"body": map[string]any{
			"body": []any{map[string]any{"_type": "rich_text", "content": map[string]any{"da": "Hej"}}},
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body)
	}
	var created content.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.TenantID.String() != tenantA {
		t.Fatalf("created.TenantID = %s, want %s", created.TenantID, tenantA)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/content/lesson/home", tenantA, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("draft published-read = %d, want 404", rec.Code)
	}
	rec = do(t, h, http.MethodPost, "/api/v1/documents/"+created.ID.String()+"/publish", tenantA, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("publish status = %d", rec.Code)
	}
	rec = do(t, h, http.MethodGet, "/api/v1/content/lesson/home", tenantA, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("published read = %d", rec.Code)
	}
}

func TestCreateUnknownType(t *testing.T) {
	h := testRouter(t)
	rec := do(t, h, http.MethodPost, "/api/v1/documents", tenantA, map[string]any{
		"type": "gizmo", "slug": "x", "title": "X",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown type = %d, want 400", rec.Code)
	}
}

func TestCreateInvalidBody(t *testing.T) {
	h := testRouter(t)
	// "blocks" is not a field of lesson (the field is "body") → schema rejects.
	rec := do(t, h, http.MethodPost, "/api/v1/documents", tenantA, map[string]any{
		"type": "lesson", "slug": "x", "title": "X",
		"body": map[string]any{"blocks": []any{}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid body = %d, want 400", rec.Code)
	}
}

func TestSchemaEndpoints(t *testing.T) {
	h := testRouter(t)
	// Schema is authenticated but not tenant-scoped — a dev tenant still works.
	rec := do(t, h, http.MethodGet, "/api/v1/schema", tenantA, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("schema all = %d", rec.Code)
	}
	var all struct {
		DocTypes []schema.DocType   `json:"docTypes"`
		Blocks   []schema.BlockType `json:"blocks"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &all)
	if len(all.DocTypes) != 6 || len(all.Blocks) == 0 {
		t.Errorf("schema payload: %d types, %d blocks", len(all.DocTypes), len(all.Blocks))
	}

	if rec := do(t, h, http.MethodGet, "/api/v1/schema/lesson", tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("schema/lesson = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/schema/gizmo", tenantA, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("schema/gizmo = %d, want 404", rec.Code)
	}
}

func TestNoTenantForbidden(t *testing.T) {
	h := testRouter(t)
	rec := do(t, h, http.MethodGet, "/api/v1/documents", "", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestTenantIsolation(t *testing.T) {
	h := testRouter(t)

	rec := do(t, h, http.MethodPost, "/api/v1/documents", tenantA, map[string]any{
		"type": "lesson", "slug": "secret", "title": "A",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, body=%s", rec.Code, rec.Body)
	}
	var a content.Document
	_ = json.Unmarshal(rec.Body.Bytes(), &a)

	rec = do(t, h, http.MethodGet, "/api/v1/documents", tenantB, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d", rec.Code)
	}
	var listResp struct {
		Documents []content.Document `json:"documents"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Documents) != 0 {
		t.Errorf("tenant B sees %d docs, want 0", len(listResp.Documents))
	}

	rec = do(t, h, http.MethodGet, "/api/v1/documents/"+a.ID.String(), tenantB, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("cross-tenant get = %d, want 404", rec.Code)
	}
}

func TestInvalidTenant(t *testing.T) {
	h := testRouter(t)
	rec := do(t, h, http.MethodGet, "/api/v1/documents", "not-a-uuid", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (invalid tenant)", rec.Code)
	}
}

func TestGetInvalidID(t *testing.T) {
	h := testRouter(t)
	rec := do(t, h, http.MethodGet, "/api/v1/documents/not-a-uuid", tenantA, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSlugConflict(t *testing.T) {
	h := testRouter(t)
	payload := map[string]any{"type": "post", "slug": "dup", "title": "Dup"}
	if rec := do(t, h, http.MethodPost, "/api/v1/documents", tenantA, payload); rec.Code != http.StatusCreated {
		t.Fatalf("first create = %d, body=%s", rec.Code, rec.Body)
	}
	rec := do(t, h, http.MethodPost, "/api/v1/documents", tenantA, payload)
	if rec.Code != http.StatusConflict {
		t.Fatalf("dup create = %d, want 409", rec.Code)
	}
}

func TestCreateInvalidJSON(t *testing.T) {
	h := testRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", bytes.NewBufferString("{bad"))
	req.Header.Set("X-Dev-Tenant", tenantA)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
