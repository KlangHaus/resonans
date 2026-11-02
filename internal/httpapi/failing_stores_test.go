package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/auth"
	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/media"
)

// errStoreDown stands for a store failure the API cannot classify. Its text
// must reach the log and never the caller.
var errStoreDown = errors.New("pq: connection to 10.0.0.7 refused")

type failingContent struct{}

func (failingContent) List(context.Context, uuid.UUID, content.ListFilter) ([]content.Document, error) {
	return nil, errStoreDown
}

func (failingContent) Get(context.Context, uuid.UUID, uuid.UUID) (content.Document, error) {
	return content.Document{}, errStoreDown
}

func (failingContent) GetPublished(context.Context, uuid.UUID, string, string) (content.Document, error) {
	return content.Document{}, errStoreDown
}

func (failingContent) GetBySlug(context.Context, uuid.UUID, string, string) (content.Document, error) {
	return content.Document{}, errStoreDown
}

func (failingContent) Create(context.Context, uuid.UUID, content.CreateInput) (content.Document, error) {
	return content.Document{}, errStoreDown
}

func (failingContent) Update(context.Context, uuid.UUID, uuid.UUID, content.UpdateInput) (content.Document, error) {
	return content.Document{}, errStoreDown
}

func (failingContent) Publish(context.Context, uuid.UUID, uuid.UUID) (content.Document, error) {
	return content.Document{}, errStoreDown
}

func (failingContent) ListVersions(context.Context, uuid.UUID, uuid.UUID) ([]content.Version, error) {
	return nil, errStoreDown
}

func (failingContent) Rollback(context.Context, uuid.UUID, uuid.UUID, int) (content.Document, error) {
	return content.Document{}, errStoreDown
}

func (failingContent) SoftDelete(context.Context, uuid.UUID, uuid.UUID) (content.Document, error) {
	return content.Document{}, errStoreDown
}

func (failingContent) Restore(context.Context, uuid.UUID, uuid.UUID) (content.Document, error) {
	return content.Document{}, errStoreDown
}

type failingMedia struct{}

func (failingMedia) Create(context.Context, uuid.UUID, media.Asset) (media.Asset, error) {
	return media.Asset{}, errStoreDown
}

func (failingMedia) Get(context.Context, uuid.UUID, uuid.UUID) (media.Asset, error) {
	return media.Asset{}, errStoreDown
}

func (failingMedia) List(context.Context, uuid.UUID, media.ListFilter) ([]media.Asset, error) {
	return nil, errStoreDown
}

func (failingMedia) SoftDelete(context.Context, uuid.UUID, uuid.UUID) (media.Asset, error) {
	return media.Asset{}, errStoreDown
}

func (failingMedia) Restore(context.Context, uuid.UUID, uuid.UUID) (media.Asset, error) {
	return media.Asset{}, errStoreDown
}

type failingSettings struct{}

func (failingSettings) EnabledTypes(context.Context, uuid.UUID) ([]string, error) {
	return nil, errStoreDown
}

func (failingSettings) SetEnabledTypes(context.Context, uuid.UUID, []string) error {
	return errStoreDown
}

func (failingSettings) PreviewURL(context.Context, uuid.UUID) (string, error) {
	return "", errStoreDown
}

func (failingSettings) SetPreviewURL(context.Context, uuid.UUID, string) error { return errStoreDown }

// Every route that reads or writes a store, with every store failing: each one
// answers 500 «internal error», the cause is in the log, and the body never
// carries it. Not 200 with an empty list, which an editor would read as "you
// have no documents". The routes that touch no store are named, with why, so
// a new route has to be placed on one side or the other.
func TestEveryStoreFailureIsA500WithoutDetail(t *testing.T) {
	storeFree := map[string]string{
		"GET /api/v1/schema/{type}":  "the code-defined registry, no tenant data",
		"GET /api/v1/video/embed":    "a provider URL, no store",
		"GET /api/v1/video/resolve":  "a provider call, no store",
		"POST /api/v1/preview-token": "signs a token, reads nothing",
	}
	var logBuf bytes.Buffer
	h := NewRouter(Deps{
		Store:         failingContent{},
		Registry:      mustRegistry(t),
		Media:         media.NewService(failingMedia{}, media.NewMemoryBackend(), media.NewURLBuilder("https://cdn.test")),
		Video:         fakeVideo{},
		Settings:      failingSettings{},
		PreviewSecret: previewSecret,
		Logger:        slog.New(slog.NewTextHandler(&logBuf, nil)),
		Verifier:      fixedVerifier{auth.Actor{Sub: "u", Tenant: tenantA, Role: auth.RoleAdmin}},
	})
	// The smallest body each write needs to get past validation to its store.
	bodies := map[string]string{
		"PUT /api/v1/settings":          `{"enabledTypes":[]}`,
		"PUT /api/v1/settings/preview":  `{"previewUrl":""}`,
		"POST /api/v1/documents":        `{"type":"post","slug":"x","title":"x","body":{}}`,
		"PUT /api/v1/documents/{id}":    `{"title":"x"}`,
		"POST /api/v1/preview/envelope": `{"type":"post","body":{}}`,
	}
	checked := 0
	for _, rt := range (&Handlers{}).Routes() {
		if _, free := storeFree[rt.Pattern]; free {
			continue
		}
		method, path, _ := strings.Cut(rt.Pattern, " ")
		path = strings.NewReplacer("{id}", "00000000-0000-0000-0000-000000000001", "{type}", "post",
			"{slug}", "hello", "{version}", "1").Replace(path)
		loggedBefore := strings.Count(logBuf.String(), "10.0.0.7")
		var w *httptest.ResponseRecorder
		if rt.Pattern == "POST /api/v1/assets" {
			// Upload is multipart; the bytes are stored, then the row fails.
			w = uploadAssetAs(t, h, "doc.pdf", []byte("%PDF-1.4 hello"))
		} else {
			body := bodies[rt.Pattern]
			if body == "" {
				body = "{}"
			}
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set("Authorization", "Bearer x")
			w = httptest.NewRecorder()
			h.ServeHTTP(w, r)
		}
		checked++
		want := `{"error":"internal error"}`
		if rt.Pattern == "POST /api/v1/assets" {
			want = `{"error":"upload failed"}`
		}
		if got := strings.TrimSpace(w.Body.String()); w.Code != 500 || got != want {
			t.Errorf("%s = %d %s; want 500 %s", rt.Pattern, w.Code, got, want)
		}
		// Per request, not in total: one route logging twice must not hide
		// another that logs nothing.
		if strings.Count(logBuf.String(), "10.0.0.7") == loggedBefore {
			t.Errorf("%s answered 500 without logging the cause", rt.Pattern)
		}
	}
	if checked != 21 {
		t.Fatalf("checked %d store routes, want 21 (25 routes, 4 store-free)", checked)
	}
}

func uploadAssetAs(t *testing.T, h interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}, filename string, data []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/assets", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Authorization", "Bearer x")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
