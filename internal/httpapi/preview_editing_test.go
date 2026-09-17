package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestPreviewEnvelopeBuildsFromUnsavedBody(t *testing.T) {
	h := testRouter(t)
	rec := do(t, h, http.MethodPost, "/api/v1/preview/envelope", tenantA, map[string]any{
		"type":  "page",
		"slug":  "home",
		"title": "Forside (kladde)",
		"body": map[string]any{
			"seoTitle": map[string]any{"da": "Hej", "en": "Hi"},
			"body":     []any{map[string]any{"_type": "rich_text", "_key": "k1"}},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("preview envelope must not be cacheable")
	}
	var env struct {
		Type       string           `json:"type"`
		Slug       string           `json:"slug"`
		Data       map[string]any   `json:"data"`
		Components []map[string]any `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Type != "page" || env.Slug != "home" {
		t.Errorf("meta = %s/%s", env.Type, env.Slug)
	}
	if env.Data["title"] != "Forside (kladde)" || env.Data["seoTitle"] != "Hej" {
		t.Errorf("data = %v (want unsaved title + da-resolved seoTitle)", env.Data)
	}
	if len(env.Components) != 1 || env.Components[0]["_key"] != "k1" {
		t.Errorf("components = %v", env.Components)
	}
	// Nothing was persisted.
	list := do(t, h, http.MethodGet, "/api/v1/documents?type=page", tenantA, nil)
	var docs struct {
		Documents []any `json:"documents"`
	}
	_ = json.Unmarshal(list.Body.Bytes(), &docs)
	if len(docs.Documents) != 0 {
		t.Errorf("preview persisted %d documents", len(docs.Documents))
	}
}

func TestPreviewEnvelopeRejects(t *testing.T) {
	h := testRouter(t)
	cases := []struct {
		name   string
		tenant string
		body   any
		want   int
	}{
		{"no tenant", "", map[string]any{"type": "page"}, http.StatusForbidden},
		{"no type", tenantA, map[string]any{"slug": "x"}, http.StatusBadRequest},
		{"unknown type", tenantA, map[string]any{"type": "nope"}, http.StatusBadRequest},
		{"body not object", tenantA, map[string]any{"type": "page", "body": []any{1}}, http.StatusBadRequest},
		{"unknown field", tenantA, map[string]any{"type": "page", "extra": 1}, http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := do(t, h, http.MethodPost, "/api/v1/preview/envelope", c.tenant, c.body)
		if rec.Code != c.want {
			t.Errorf("%s: status = %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
		}
	}
}

func TestPreviewEnvelopeRespectsContentProfile(t *testing.T) {
	h := testRouter(t)
	if rec := do(t, h, http.MethodPut, "/api/v1/settings", tenantA, map[string]any{"enabledTypes": []string{"post"}}); rec.Code != http.StatusOK {
		t.Fatalf("put settings = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/preview/envelope", tenantA, map[string]any{"type": "page"}); rec.Code != http.StatusBadRequest {
		t.Errorf("disabled type: status = %d, want 400", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/preview/envelope", tenantA, map[string]any{"type": "post"}); rec.Code != http.StatusOK {
		t.Errorf("enabled type: status = %d, want 200", rec.Code)
	}
}

func TestPreviewSettingsRoundtripAndIsolation(t *testing.T) {
	h := testRouter(t)
	const tpl = "https://etude.example/preview/{type}/{slug}?token={token}"
	rec := do(t, h, http.MethodPut, "/api/v1/settings/preview", tenantA, map[string]any{"previewUrl": tpl})
	if rec.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rec.Code, rec.Body)
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["previewUrl"] != tpl || got["previewOrigin"] != "https://etude.example" {
		t.Errorf("put response = %v", got)
	}
	// testRouter has no preview secret: the URL is stored but preview is not ready.
	if got["previewReady"] != false {
		t.Errorf("previewReady = %v, want false without a preview secret", got["previewReady"])
	}
	rec = do(t, h, http.MethodGet, "/api/v1/settings/preview", tenantB, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["previewUrl"] != "" || got["previewOrigin"] != "" {
		t.Errorf("tenant B sees %v", got)
	}
	rec = do(t, h, http.MethodPut, "/api/v1/settings/preview", tenantA, map[string]any{"previewUrl": "http://etude.example/p?token={token}"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid url: status = %d, want 400", rec.Code)
	}
	rec = do(t, h, http.MethodPut, "/api/v1/settings/preview", tenantA, map[string]any{"previewUrl": ""})
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusOK || got["previewUrl"] != "" {
		t.Errorf("clear: %d %v", rec.Code, got)
	}
}
