package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/preview"
	"github.com/KlangHaus/resonans/internal/settings"
)

const previewSecret = "preview-secret-of-sufficient-length-0123456789"

func routerWith(t *testing.T, d Deps) http.Handler {
	t.Helper()
	if d.Store == nil {
		d.Store = content.NewMemoryStore()
	}
	if d.Registry == nil {
		d.Registry = mustRegistry(t)
	}
	if d.Media == nil {
		d.Media = media.NewService(media.NewMemoryStore(), media.NewMemoryBackend(), media.NewURLBuilder("https://cdn.test"))
	}
	if d.Settings == nil {
		d.Settings = settings.NewMemoryStore()
	}
	return NewRouter(d)
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not JSON (%v): %s", err, rec.Body)
	}
	return m
}

// 🔴 A preview token grants draft-read for one (type, slug) in ONE tenant: the
// caller's, from the verified actor. The body cannot name another tenant —
// an unknown field is refused, not ignored.
func TestPreviewTokenCarriesTheCallersTenant(t *testing.T) {
	h := routerWith(t, Deps{PreviewSecret: previewSecret})
	rec := do(t, h, http.MethodPost, "/api/v1/preview-token", tenantA, map[string]any{"type": "page", "slug": "home"})
	if rec.Code != http.StatusOK {
		t.Fatalf("preview-token = %d %s", rec.Code, rec.Body)
	}
	body := decodeMap(t, rec)
	claims, err := preview.Verify([]byte(previewSecret), fmt.Sprint(body["token"]), time.Now())
	if err != nil {
		t.Fatalf("issued token does not verify: %v", err)
	}
	if claims.Tenant != tenantA || claims.Type != "page" || claims.Slug != "home" {
		t.Errorf("claims = %+v, want tenant A, page/home", claims)
	}
	if left := time.Until(time.Unix(claims.Exp, 0)); left < 14*time.Minute || left > 15*time.Minute+time.Second {
		t.Errorf("token lives %v, want 15 minutes", left)
	}
	if body["expiresInSeconds"] != float64(900) {
		t.Errorf("expiresInSeconds = %v", body["expiresInSeconds"])
	}
	// The verified tenant is the only tenant: a "tenant" field is refused.
	rec = do(t, h, http.MethodPost, "/api/v1/preview-token", tenantA, map[string]any{"type": "page", "slug": "home", "tenant": tenantB})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a body naming another tenant = %d, want 400", rec.Code)
	}
	if _, err := preview.Verify([]byte("another-secret"), fmt.Sprint(body["token"]), time.Now()); err == nil {
		t.Error("the token verifies under another secret")
	}
}

func TestPreviewTokenRefusals(t *testing.T) {
	off := routerWith(t, Deps{})
	if rec := do(t, off, http.MethodPost, "/api/v1/preview-token", tenantA, map[string]any{"type": "page", "slug": "x"}); rec.Code != http.StatusNotFound {
		t.Errorf("no preview secret = %d, want 404", rec.Code)
	}
	on := routerWith(t, Deps{PreviewSecret: previewSecret})
	for name, body := range map[string]any{
		"missing slug": map[string]any{"type": "page"},
		"missing type": map[string]any{"slug": "x"},
	} {
		if rec := do(t, on, http.MethodPost, "/api/v1/preview-token", tenantA, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/preview-token", strings.NewReader("{not json"))
	req.Header.Set("X-Dev-Tenant", tenantA)
	rec := httptest.NewRecorder()
	on.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("broken JSON = %d, want 400", rec.Code)
	}
}

// Version history and rollback through the API, tenant-scoped.
func TestVersionsAndRollback(t *testing.T) {
	h := testRouter(t)
	id := createDoc(t, h, tenantA, "history")
	publish := func() {
		t.Helper()
		if rec := do(t, h, http.MethodPost, "/api/v1/documents/"+id+"/publish", tenantA, nil); rec.Code != http.StatusOK {
			t.Fatalf("publish = %d %s", rec.Code, rec.Body)
		}
	}
	publish()
	if rec := do(t, h, http.MethodPut, "/api/v1/documents/"+id, tenantA, map[string]any{"title": "Second"}); rec.Code != http.StatusOK {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
	publish()

	rec := do(t, h, http.MethodGet, "/api/v1/documents/"+id+"/versions", tenantA, nil)
	var vs struct {
		Versions []content.Version `json:"versions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &vs); err != nil || rec.Code != http.StatusOK || len(vs.Versions) != 2 {
		t.Fatalf("versions = %d, %d entries (%v)", rec.Code, len(vs.Versions), err)
	}
	if vs.Versions[0].Version <= vs.Versions[1].Version {
		t.Errorf("versions not newest first: %d then %d", vs.Versions[0].Version, vs.Versions[1].Version)
	}
	first := vs.Versions[1]

	// Another tenant sees no history and cannot roll back.
	if rec := do(t, h, http.MethodGet, "/api/v1/documents/"+id+"/versions", tenantB, nil); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Second") {
		t.Errorf("tenant B versions = %d %s; want an empty history", rec.Code, rec.Body)
	}
	rb := fmt.Sprintf("/api/v1/documents/%s/rollback/%d", id, first.Version)
	if rec := do(t, h, http.MethodPost, rb, tenantB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("tenant B rollback = %d, want 404", rec.Code)
	}

	rec = do(t, h, http.MethodPost, rb, tenantA, nil)
	var d content.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("rollback = %d %s", rec.Code, rec.Body)
	}
	if d.Title != first.Title || d.Status != content.StatusDraft {
		t.Errorf("after rollback: title %q status %q; want %q as a draft", d.Title, d.Status, first.Title)
	}

	for path, want := range map[string]int{
		"/api/v1/documents/not-a-uuid/versions":     http.StatusBadRequest,
		"/api/v1/documents/not-a-uuid/rollback/1":   http.StatusBadRequest,
		"/api/v1/documents/" + id + "/rollback/one": http.StatusBadRequest,
		"/api/v1/documents/" + id + "/rollback/99":  http.StatusNotFound,
	} {
		method := http.MethodPost
		if strings.HasSuffix(path, "/versions") {
			method = http.MethodGet
		}
		if rec := do(t, h, method, path, tenantA, nil); rec.Code != want {
			t.Errorf("%s %s = %d, want %d", method, path, rec.Code, want)
		}
	}
}

// Every store error has its own answer; an unknown one is 500 with no detail,
// and logged. ErrInvalidSlug wraps ErrInvalid, so its case must come first.
func TestWriteErrMapping(t *testing.T) {
	for _, c := range []struct {
		err  error
		code int
		msg  string
	}{
		{content.ErrNotFound, 404, "not found"},
		{content.ErrSlugConflict, 409, "slug already exists"},
		{content.ErrDeleted, 409, "document is deleted; restore it first"},
		{content.ErrNotDeleted, 409, "document is not deleted"},
		{content.ErrInvalidSlug, 400, "invalid slug"},
		{content.ErrInvalid, 400, "invalid input"},
		{fmt.Errorf("wrapped: %w", content.ErrNotFound), 404, "not found"},
		{errors.New("pq: password authentication failed for user resonans_app"), 500, "internal error"},
	} {
		var logBuf bytes.Buffer
		h := &Handlers{log: slog.New(slog.NewTextHandler(&logBuf, nil))}
		rec := httptest.NewRecorder()
		h.writeErr(rec, c.err)
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.msg) {
			t.Errorf("%v → %d %s; want %d %q", c.err, rec.Code, rec.Body, c.code, c.msg)
		}
		if c.code == 500 {
			if strings.Contains(rec.Body.String(), "password") || !strings.Contains(logBuf.String(), "password authentication failed") {
				t.Errorf("500: body %q, log %q; want the detail in the log only", rec.Body, logBuf.String())
			}
		}
	}
}

func TestWriteMediaErrMapping(t *testing.T) {
	for _, c := range []struct {
		err  error
		code int
		msg  string
	}{
		{media.ErrNotFound, 404, "not found"},
		{media.ErrDeleted, 409, "asset is deleted; restore it first"},
		{media.ErrNotDeleted, 409, "asset is not deleted"},
		{errors.New("bunny: 401 zone key rejected"), 500, "internal error"},
	} {
		var logBuf bytes.Buffer
		h := &Handlers{log: slog.New(slog.NewTextHandler(&logBuf, nil))}
		rec := httptest.NewRecorder()
		h.writeMediaErr(rec, c.err)
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.msg) {
			t.Errorf("%v → %d %s; want %d %q", c.err, rec.Code, rec.Body, c.code, c.msg)
		}
		if c.code == 500 && (strings.Contains(rec.Body.String(), "zone key") || !strings.Contains(logBuf.String(), "media_store_error")) {
			t.Errorf("500: body %q, log %q", rec.Body, logBuf.String())
		}
	}
}

type fakeVideo struct{ metaErr error }

func (fakeVideo) Name() string              { return "fake" }
func (fakeVideo) EmbedURL(id string) string { return "https://player.example/" + id }
func (f fakeVideo) Metadata(context.Context, string) (media.VideoMeta, error) {
	if f.metaErr != nil {
		return media.VideoMeta{}, f.metaErr
	}
	return media.VideoMeta{Title: "Organisk kemi", DurationSeconds: 610}, nil
}

// The video plugin is opt-in; when the provider cannot give metadata the
// editor still gets a playable embed, told that the metadata is missing.
func TestVideoRoutes(t *testing.T) {
	off := routerWith(t, Deps{})
	for _, path := range []string{"/api/v1/video/embed?id=1", "/api/v1/video/resolve?id=1"} {
		if rec := do(t, off, http.MethodGet, path, tenantA, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s with no plugin = %d, want 404", path, rec.Code)
		}
	}
	on := routerWith(t, Deps{Video: fakeVideo{}})
	for _, path := range []string{"/api/v1/video/embed", "/api/v1/video/resolve"} {
		if rec := do(t, on, http.MethodGet, path, tenantA, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s without id = %d, want 400", path, rec.Code)
		}
	}
	if m := decodeMap(t, do(t, on, http.MethodGet, "/api/v1/video/embed?id=42", tenantA, nil)); m["embedUrl"] != "https://player.example/42" || m["provider"] != "fake" {
		t.Errorf("embed = %v", m)
	}
	m := decodeMap(t, do(t, on, http.MethodGet, "/api/v1/video/resolve?id=42", tenantA, nil))
	meta, _ := m["meta"].(map[string]any)
	if m["embedUrl"] != "https://player.example/42" || meta["title"] != "Organisk kemi" {
		t.Errorf("resolve = %v", m)
	}

	var logBuf bytes.Buffer
	down := routerWith(t, Deps{Video: fakeVideo{metaErr: errors.New("private video")}, Logger: slog.New(slog.NewTextHandler(&logBuf, nil))})
	rec := do(t, down, http.MethodGet, "/api/v1/video/resolve?id=42", tenantA, nil)
	m = decodeMap(t, rec)
	if rec.Code != http.StatusOK || m["embedUrl"] != "https://player.example/42" || m["metaAvailable"] != false || m["meta"] != nil {
		t.Errorf("resolve without metadata = %d %v; want 200, the embed, metaAvailable=false", rec.Code, m)
	}
	if !strings.Contains(logBuf.String(), "video_metadata_unavailable") {
		t.Errorf("log = %q", logBuf.String())
	}
}

type failingBackend struct{}

func (failingBackend) Put(context.Context, string, string, []byte) error {
	return errors.New("bunny: 401 zone key rejected")
}
func (failingBackend) Name() string { return "failing" }

func TestUploadRefusals(t *testing.T) {
	h := testRouter(t)
	// Not a multipart form at all.
	if rec := do(t, h, http.MethodPost, "/api/v1/assets", tenantA, map[string]any{"file": "x"}); rec.Code != http.StatusBadRequest {
		t.Errorf("JSON instead of multipart = %d, want 400", rec.Code)
	}
	// HTML bytes are refused by content, with the allowed list.
	rec := uploadAsset(t, h, tenantA, "photo.png", "image/png", []byte("<!doctype html><script>alert(1)</script>"))
	m := decodeMap(t, rec)
	if allowed, _ := m["allowed"].([]any); rec.Code != http.StatusUnsupportedMediaType || len(allowed) == 0 {
		t.Errorf("HTML upload = %d %v; want 415 with the allowed types", rec.Code, m)
	}
	// Bytes the backend could not store: 500, no backend detail in the body.
	var logBuf bytes.Buffer
	broken := routerWith(t, Deps{
		Media:  media.NewService(media.NewMemoryStore(), failingBackend{}, media.NewURLBuilder("https://cdn.test")),
		Logger: slog.New(slog.NewTextHandler(&logBuf, nil)),
	})
	rec = uploadAsset(t, broken, tenantA, "doc.pdf", "application/pdf", []byte("%PDF-1.4 hello"))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "zone key") || !strings.Contains(logBuf.String(), "asset_upload_error") {
		t.Errorf("backend failure = %d %s, log %q", rec.Code, rec.Body, logBuf.String())
	}
}

// The preview URL: an invalid template is refused before it is stored, a
// valid one comes back with its origin, "" turns visual editing off, and
// previewReady needs the server's preview secret too.
func TestPreviewSettings(t *testing.T) {
	h := routerWith(t, Deps{PreviewSecret: previewSecret})
	const tpl = "https://site.example/preview/{type}/{slug}?token={token}"
	for _, bad := range []string{"http://site.example/p?token={token}", "https://site.example/p", "javascript:alert(1)//{token}"} {
		if rec := do(t, h, http.MethodPut, "/api/v1/settings/preview", tenantA, map[string]any{"previewUrl": bad}); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %q = %d, want 400", bad, rec.Code)
		}
	}
	if m := decodeMap(t, do(t, h, http.MethodGet, "/api/v1/settings/preview", tenantA, nil)); m["previewUrl"] != "" {
		t.Fatalf("a refused template was stored: %v", m)
	}
	m := decodeMap(t, do(t, h, http.MethodPut, "/api/v1/settings/preview", tenantA, map[string]any{"previewUrl": tpl}))
	if m["previewUrl"] != tpl || m["previewOrigin"] != "https://site.example" || m["previewReady"] != true {
		t.Errorf("after PUT = %v", m)
	}
	if m := decodeMap(t, do(t, h, http.MethodGet, "/api/v1/settings/preview", tenantB, nil)); m["previewUrl"] != "" {
		t.Errorf("tenant B sees %v", m)
	}
	if m := decodeMap(t, do(t, h, http.MethodPut, "/api/v1/settings/preview", tenantA, map[string]any{"previewUrl": ""})); m["previewUrl"] != "" || m["previewReady"] != false {
		t.Errorf("after turning it off = %v", m)
	}
	// Without the server's preview secret a valid template is not "ready".
	noSecret := routerWith(t, Deps{})
	do(t, noSecret, http.MethodPut, "/api/v1/settings/preview", tenantA, map[string]any{"previewUrl": tpl})
	if m := decodeMap(t, do(t, noSecret, http.MethodGet, "/api/v1/settings/preview", tenantA, nil)); m["previewReady"] != false || m["previewOrigin"] != "https://site.example" {
		t.Errorf("no secret = %v; want an origin but not ready", m)
	}
}
