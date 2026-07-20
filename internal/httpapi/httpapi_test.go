package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KlangHaus/resonans/internal/content"
)

func testRouter(token string) http.Handler {
	return NewRouter(Deps{
		Store:       content.NewMemoryStore(),
		DefaultSite: "klanghaus.dk",
		EditorToken: token,
	})
}

func do(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreatePublishReadFlow(t *testing.T) {
	h := testRouter("s3cret")

	// Create (draft).
	rec := do(t, h, http.MethodPost, "/api/v1/entries", "s3cret", map[string]any{
		"site": "klanghaus.dk", "type": "page", "slug": "home", "title": "Home",
		"body": map[string]any{"blocks": []any{}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body)
	}
	var created content.Entry
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}

	// Published read path returns 404 while still a draft.
	rec = do(t, h, http.MethodGet, "/api/v1/content/klanghaus.dk/home", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("draft published-read status = %d, want 404", rec.Code)
	}

	// Publish.
	rec = do(t, h, http.MethodPost, "/api/v1/entries/"+created.ID.String()+"/publish", "s3cret", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("publish status = %d", rec.Code)
	}

	// Now visible on the public read path.
	rec = do(t, h, http.MethodGet, "/api/v1/content/klanghaus.dk/home", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("published read status = %d", rec.Code)
	}
}

func TestWriteRequiresToken(t *testing.T) {
	h := testRouter("s3cret")

	// No token → 401.
	rec := do(t, h, http.MethodPost, "/api/v1/entries", "", map[string]any{
		"type": "page", "slug": "x", "title": "X",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", rec.Code)
	}

	// Wrong token → 401.
	rec = do(t, h, http.MethodPost, "/api/v1/entries", "wrong", map[string]any{
		"type": "page", "slug": "x", "title": "X",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-token status = %d, want 401", rec.Code)
	}
}

func TestReadIsPublic(t *testing.T) {
	h := testRouter("s3cret")
	rec := do(t, h, http.MethodGet, "/api/v1/entries", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200 (public)", rec.Code)
	}
}

func TestGetInvalidID(t *testing.T) {
	h := testRouter("")
	rec := do(t, h, http.MethodGet, "/api/v1/entries/not-a-uuid", "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateSlugConflict(t *testing.T) {
	h := testRouter("") // empty token => unguarded (dev)

	payload := map[string]any{"type": "page", "slug": "dup", "title": "Dup"}
	if rec := do(t, h, http.MethodPost, "/api/v1/entries", "", payload); rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d", rec.Code)
	}
	rec := do(t, h, http.MethodPost, "/api/v1/entries", "", payload)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409", rec.Code)
	}
}

func TestCreateInvalidJSON(t *testing.T) {
	h := testRouter("")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries", bytes.NewBufferString("{bad"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
