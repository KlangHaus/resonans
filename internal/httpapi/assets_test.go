package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/KlangHaus/resonans/internal/media"
)

func uploadAsset(t *testing.T, h http.Handler, tenant, filename, mime string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	hdr.Set("Content-Type", mime)
	part, err := mw.CreatePart(hdr)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if tenant != "" {
		req.Header.Set("X-Dev-Tenant", tenant)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAssetUploadListGet(t *testing.T) {
	h := testRouter(t)

	rec := uploadAsset(t, h, tenantA, "doc.pdf", "application/pdf", []byte("%PDF-1.4 hello"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d, body=%s", rec.Code, rec.Body)
	}
	var a assetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if a.TenantID.String() != tenantA {
		t.Fatalf("asset tenant = %s, want %s", a.TenantID, tenantA)
	}
	if a.URL == "" || a.Kind != media.KindFile {
		t.Errorf("asset response = %+v", a)
	}

	// List includes it.
	rec = do(t, h, http.MethodGet, "/api/v1/assets", tenantA, nil)
	var list struct {
		Assets []assetResponse `json:"assets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Assets) != 1 {
		t.Fatalf("list = %d assets, want 1", len(list.Assets))
	}

	// Get by id.
	if rec := do(t, h, http.MethodGet, "/api/v1/assets/"+a.ID.String(), tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("get = %d", rec.Code)
	}
	// Cross-tenant get → 404.
	if rec := do(t, h, http.MethodGet, "/api/v1/assets/"+a.ID.String(), tenantB, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant get = %d, want 404", rec.Code)
	}
}

func TestAssetUploadNoTenant(t *testing.T) {
	h := testRouter(t)
	rec := uploadAsset(t, h, "", "x.txt", "text/plain", []byte("hi"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no tenant upload = %d, want 403", rec.Code)
	}
}

func TestVideoEmbed(t *testing.T) {
	h := testRouter(t)
	rec := do(t, h, http.MethodGet, "/api/v1/video/embed?id=98765", tenantA, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("video embed = %d", rec.Code)
	}
	var resp struct {
		Provider string `json:"provider"`
		EmbedURL string `json:"embedUrl"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Provider != "vimeo" || resp.EmbedURL != "https://player.vimeo.com/video/98765" {
		t.Errorf("video resp = %+v", resp)
	}
}

func TestAssetUploadRejectsSVGAndHTML(t *testing.T) {
	h := testRouter(t)
	for name, c := range map[string]struct{ filename, mime, body string }{
		"svg":           {"logo.svg", "image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`},
		"html as image": {"photo.png", "image/png", `<html><script>alert(1)</script></html>`},
	} {
		rec := uploadAsset(t, h, tenantA, c.filename, c.mime, []byte(c.body))
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("%s: status = %d, want 415 (%s)", name, rec.Code, rec.Body)
		}
	}
	rec := do(t, h, http.MethodGet, "/api/v1/assets", tenantA, nil)
	var list struct {
		Assets []assetResponse `json:"assets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Assets) != 0 {
		t.Errorf("rejected uploads were recorded: %d", len(list.Assets))
	}
}
