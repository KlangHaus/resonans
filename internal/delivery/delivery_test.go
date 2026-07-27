package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/preview"
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

func TestDeliveryExpandCourseTree(t *testing.T) {
	s := content.NewMemoryStore()
	ctx := context.Background()
	mk := func(typ, slug, title, body string) content.Document {
		d, err := s.Create(ctx, tenantA, content.CreateInput{Type: typ, Slug: slug, Title: title, Body: json.RawMessage(body)})
		if err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
		if _, err := s.Publish(ctx, tenantA, d.ID); err != nil {
			t.Fatalf("publish %s: %v", slug, err)
		}
		return d
	}
	lesson := mk("lesson", "l1", "Lektion 1", `{"estimatedMinutes":15}`)
	module := mk("module", "m1", "Modul 1", `{"lessons":[{"lesson":"`+lesson.ID.String()+`"}]}`)
	mk("course", "c1", "Kursus 1", `{"modules":[{"module":"`+module.ID.String()+`"}]}`)

	h := NewRouter(Deps{Store: s, Registry: reg(t)})
	rec := get(h, "/delivery/v1/course/c1?expand=modules.lessons", "", tenantA.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("expand = %d", rec.Code)
	}
	var env Envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	mods, _ := env.Data["modules"].([]any)
	if len(mods) != 1 {
		t.Fatalf("modules = %d, want 1 (tree): %v", len(mods), env.Data["modules"])
	}
	m0, _ := mods[0].(map[string]any)
	if m0["title"] != "Modul 1" || m0["slug"] != "m1" {
		t.Errorf("module node = %v", m0)
	}
	lessons, _ := m0["lessons"].([]any)
	if len(lessons) != 1 {
		t.Fatalf("lessons = %d, want 1", len(lessons))
	}
	l0, _ := lessons[0].(map[string]any)
	if l0["title"] != "Lektion 1" || l0["durationEstimate"] != float64(15) {
		t.Errorf("lesson node = %v", l0)
	}
}

func TestDeliveryPreviewDraft(t *testing.T) {
	s := content.NewMemoryStore()
	ctx := context.Background()
	if _, err := s.Create(ctx, tenantA, content.CreateInput{Type: "page", Slug: "draft-page", Title: "Udkast"}); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	secret := "preview-secret"
	h := NewRouter(Deps{Store: s, Registry: reg(t), PreviewSecret: secret})

	// Draft is NOT on the published path.
	if rec := get(h, "/delivery/v1/page/draft-page", "", tenantA.String()); rec.Code != http.StatusNotFound {
		t.Fatalf("published draft = %d, want 404", rec.Code)
	}
	// A valid preview token renders the draft (tenant comes from the token).
	tok := preview.Sign([]byte(secret), preview.Claims{
		Tenant: tenantA.String(), Type: "page", Slug: "draft-page", Exp: time.Now().Add(time.Minute).Unix(),
	})
	rec := get(h, "/delivery/v1/preview/page/draft-page?token="+tok, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d, body=%s", rec.Code, rec.Body)
	}
	var env Envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Slug != "draft-page" || env.Data["title"] != "Udkast" {
		t.Errorf("preview envelope = %+v", env)
	}
	// No token → 401.
	if rec := get(h, "/delivery/v1/preview/page/draft-page", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token = %d, want 401", rec.Code)
	}
	// Token for a different slug → 403 scope mismatch.
	bad := preview.Sign([]byte(secret), preview.Claims{Tenant: tenantA.String(), Type: "page", Slug: "other", Exp: time.Now().Add(time.Minute).Unix()})
	if rec := get(h, "/delivery/v1/preview/page/draft-page?token="+bad, "", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("scope mismatch = %d, want 403", rec.Code)
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
