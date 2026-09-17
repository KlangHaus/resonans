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
	"github.com/KlangHaus/resonans/internal/settings"
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
		Video:    media.NewVimeoProvider(),
		Settings: settings.NewMemoryStore(),
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
			"body": []any{map[string]any{"_type": "rich_text", "value": map[string]any{"da": "Hej"}}},
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
	if len(all.DocTypes) != 7 || len(all.Blocks) == 0 {
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

func TestContentProfile(t *testing.T) {
	h := testRouter(t)

	// Default: all 7 types enabled.
	rec := do(t, h, http.MethodGet, "/api/v1/settings", tenantA, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings = %d", rec.Code)
	}
	var st struct {
		EnabledTypes []string `json:"enabledTypes"`
		AllTypes     []string `json:"allTypes"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if len(st.EnabledTypes) != 0 || len(st.AllTypes) != 7 {
		t.Fatalf("default profile: enabled=%v all=%d", st.EnabledTypes, len(st.AllTypes))
	}

	// Narrow tenant A to the web profile (post+page).
	rec = do(t, h, http.MethodPut, "/api/v1/settings", tenantA, map[string]any{"enabledTypes": []string{"post", "page"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings = %d: %s", rec.Code, rec.Body.String())
	}

	// Schema is filtered to the enabled set.
	rec = do(t, h, http.MethodGet, "/api/v1/schema", tenantA, nil)
	var sch struct {
		DocTypes []schema.DocType `json:"docTypes"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sch)
	if len(sch.DocTypes) != 2 {
		t.Fatalf("filtered schema: %d types, want 2", len(sch.DocTypes))
	}

	// Creating a disabled type is rejected; an enabled type still works.
	rec = do(t, h, http.MethodPost, "/api/v1/documents", tenantA, map[string]any{"type": "course", "slug": "x", "title": "X"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create disabled type = %d, want 403", rec.Code)
	}
	rec = do(t, h, http.MethodPost, "/api/v1/documents", tenantA, map[string]any{"type": "post", "slug": "x", "title": "X"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create enabled type = %d: %s", rec.Code, rec.Body.String())
	}

	// The profile is tenant-scoped: tenant B still sees everything.
	rec = do(t, h, http.MethodGet, "/api/v1/schema", tenantB, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &sch)
	if len(sch.DocTypes) != 7 {
		t.Fatalf("tenant B schema: %d types, want 7", len(sch.DocTypes))
	}

	// Unknown type names are rejected; empty list re-enables everything.
	if rec := do(t, h, http.MethodPut, "/api/v1/settings", tenantA, map[string]any{"enabledTypes": []string{"gizmo"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("put unknown type = %d, want 400", rec.Code)
	}
	if rec := do(t, h, http.MethodPut, "/api/v1/settings", tenantA, map[string]any{"enabledTypes": []string{}}); rec.Code != http.StatusOK {
		t.Fatalf("put empty = %d", rec.Code)
	}
	rec = do(t, h, http.MethodGet, "/api/v1/schema", tenantA, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &sch)
	if len(sch.DocTypes) != 7 {
		t.Fatalf("re-enabled schema: %d types, want 7", len(sch.DocTypes))
	}
}

// Rich text is sanitized before it is stored, on create AND update, so a body
// written straight to the API cannot carry script to an editor's Studio.
func TestRichTextSanitizedOnSave(t *testing.T) {
	h := testRouter(t)
	evil := `<p>ok</p><img src=x onerror="alert(1)"><a href="javascript:alert(1)">l</a>`
	rec := do(t, h, http.MethodPost, "/api/v1/documents", tenantA, map[string]any{
		"type": "page", "slug": "x", "title": "X",
		"body": map[string]any{"body": []any{map[string]any{"_type": "rich_text", "format": "html", "value": map[string]any{"da": evil}}}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var d struct {
		ID   string          `json:"id"`
		Body json.RawMessage `json:"body"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &d)
	check := func(stage string, raw string) {
		t.Helper()
		var doc struct {
			Body struct {
				Body []struct {
					Value map[string]string `json:"value"`
				} `json:"body"`
			} `json:"body"`
		}
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatalf("%s: decode: %v", stage, err)
		}
		if got := doc.Body.Body[0].Value["da"]; got != "<p>ok</p>l" {
			t.Errorf("%s: stored rich text = %q, want %q", stage, got, "<p>ok</p>l")
		}
	}
	got := do(t, h, http.MethodGet, "/api/v1/documents/"+d.ID, tenantA, nil)
	check("create", got.Body.String())

	rec = do(t, h, http.MethodPut, "/api/v1/documents/"+d.ID, tenantA, map[string]any{
		"body": map[string]any{"body": []any{map[string]any{"_type": "rich_text", "format": "html", "value": map[string]any{"da": evil}}}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
	got = do(t, h, http.MethodGet, "/api/v1/documents/"+d.ID, tenantA, nil)
	check("update", got.Body.String())
}

// Plain text survives a save byte-identical ([sikkerhed]'s correctness cell):
// no &#39; / &amp; / &lt; appears in a callout or in non-html rich text.
func TestPlainTextUnchangedOnSave(t *testing.T) {
	h := testRouter(t)
	const plain = `Husk: 'organisk' betyder ikke "levende" & 2 < 3`
	rec := do(t, h, http.MethodPost, "/api/v1/documents", tenantA, map[string]any{
		"type": "page", "slug": "p", "title": "P",
		"body": map[string]any{"body": []any{
			map[string]any{"_type": "callout", "value": map[string]any{"da": plain}},
			map[string]any{"_type": "rich_text", "format": "portable_text", "value": map[string]any{"da": plain}},
		}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var doc struct {
		Body struct {
			Body []struct {
				Value map[string]string `json:"value"`
			} `json:"body"`
		} `json:"body"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	for i, b := range doc.Body.Body {
		if b.Value["da"] != plain {
			t.Errorf("block %d stored %q, want byte-identical %q", i, b.Value["da"], plain)
		}
	}
}

// [sikkerhed]'s bypass: an ARRAY in HTML position was stored unsanitized and a
// consumer's String(value) turned it into markup. Now 400, and nothing stored.
func TestHTMLRichTextArrayRejectedOnSave(t *testing.T) {
	h := testRouter(t)
	arr := []any{"<img src=x onerror=alert(1)>", "<p>ok</p>"}
	bodies := map[string]map[string]any{
		"i18n": {"body": []any{map[string]any{"_type": "rich_text", "format": "html", "value": map[string]any{"da": arr}}}},
		"columns": {"body": []any{map[string]any{"_type": "columns", "columns": []any{map[string]any{"content": []any{
			map[string]any{"_type": "rich_text", "format": "html", "value": map[string]any{"da": arr}},
		}}}}}},
	}
	for name, body := range bodies {
		rec := do(t, h, http.MethodPost, "/api/v1/documents", tenantA, map[string]any{"type": "page", "slug": "a-" + name, "title": "A", "body": body})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: create = %d, want 400 (%s)", name, rec.Code, rec.Body)
		}
	}
	list := do(t, h, http.MethodGet, "/api/v1/documents?type=page", tenantA, nil)
	var docs struct {
		Documents []any `json:"documents"`
	}
	_ = json.Unmarshal(list.Body.Bytes(), &docs)
	if len(docs.Documents) != 0 {
		t.Errorf("rejected bodies were stored: %d", len(docs.Documents))
	}
}
