package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
)

// byIDStore holds, for tenant A: a published course, a draft course, a published
// quiz, and a published course that was then binned; and for tenant B one
// published course.
type byIDStore struct {
	h                                  http.Handler
	s                                  *content.MemoryStore
	course, draft, quiz, binned, other content.Document
}

func newByIDStore(t *testing.T) byIDStore {
	t.Helper()
	s := content.NewMemoryStore()
	ctx := context.Background()
	create := func(tenant uuid.UUID, typ, slug string, publish bool) content.Document {
		d, err := s.Create(ctx, tenant, content.CreateInput{Type: typ, Slug: slug, Title: slug})
		if err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
		if publish {
			if d, err = s.Publish(ctx, tenant, d.ID); err != nil {
				t.Fatalf("publish %s: %v", slug, err)
			}
		}
		return d
	}
	b := byIDStore{s: s}
	b.course = create(tenantA, "course", "intro", true)
	b.draft = create(tenantA, "course", "wip", false)
	b.quiz = create(tenantA, "quiz", "q1", true)
	b.binned = create(tenantA, "course", "old", true)
	if _, err := s.SoftDelete(ctx, tenantA, b.binned.ID); err != nil {
		t.Fatalf("bin: %v", err)
	}
	b.other = create(tenantB, "course", "intro", true)
	b.h = NewRouter(Deps{Store: s, Registry: reg(t), TrustHeaderWithoutToken: true})
	return b
}

func byIDPath(typ string, d content.Document) string {
	return "/delivery/v1/by-id/" + typ + "/" + d.ID.String()
}

// The green case first: a published document comes back by id with the same
// body and ETag the slug route gives it, and its ETag answers 304.
func TestDeliveryByIDServesThePublishedDocument(t *testing.T) {
	b := newByIDStore(t)

	byID := get(b.h, byIDPath("course", b.course), "", tenantA.String())
	if byID.Code != http.StatusOK {
		t.Fatalf("by-id = %d, want 200: %s", byID.Code, byID.Body)
	}
	bySlug := get(b.h, "/delivery/v1/course/intro", "", tenantA.String())
	if byID.Body.String() != bySlug.Body.String() {
		t.Errorf("by-id body differs from by-slug body:\n%s\n%s", byID.Body, bySlug.Body)
	}
	etag := byID.Header().Get("ETag")
	if etag == "" || etag != bySlug.Header().Get("ETag") {
		t.Errorf("ETag by-id %q, by-slug %q: want one non-empty ETag", etag, bySlug.Header().Get("ETag"))
	}
	if cc := byID.Header().Get("Cache-Control"); cc != bySlug.Header().Get("Cache-Control") {
		t.Errorf("Cache-Control by-id %q, by-slug %q", cc, bySlug.Header().Get("Cache-Control"))
	}

	req := httptest.NewRequest(http.MethodGet, byIDPath("course", b.course), nil)
	req.Header.Set("X-Scope-OrgID", tenantA.String())
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	b.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Errorf("If-None-Match = %d with %d body bytes, want 304 and none", rec.Code, rec.Body.Len())
	}

	// The quiz is a document of its own and is served the same way.
	q := get(b.h, byIDPath("quiz", b.quiz), "", tenantA.String())
	var env Envelope
	if err := json.Unmarshal(q.Body.Bytes(), &env); err != nil || q.Code != http.StatusOK || env.ID != b.quiz.ID.String() {
		t.Errorf("quiz by-id = %d id %q, want 200 and %s", q.Code, env.ID, b.quiz.ID)
	}
}

// Every document the slug route would not serve is a 404 here too, with the
// same body, so a caller cannot tell a draft from a missing id.
func TestDeliveryByIDRefusesWhatIsNotPublishedHere(t *testing.T) {
	b := newByIDStore(t)
	missing := get(b.h, "/delivery/v1/course/no-such-slug", "", tenantA.String())

	cases := map[string]string{
		"draft":                byIDPath("course", b.draft),
		"binned":               byIDPath("course", b.binned),
		"other tenant":         byIDPath("course", b.other),
		"another type":         byIDPath("course", b.quiz),
		"unknown type":         byIDPath("nope", b.course),
		"not a uuid":           "/delivery/v1/by-id/course/intro",
		"uuid nobody has used": "/delivery/v1/by-id/course/00000000-0000-4000-8000-0000000000ff",
	}
	for name, path := range cases {
		rec := get(b.h, path, "", tenantA.String())
		if rec.Code != http.StatusNotFound || rec.Body.String() != missing.Body.String() {
			t.Errorf("%s: %s = %d %s, want 404 %s", name, path, rec.Code, rec.Body, missing.Body)
		}
	}

	// The binned course is still published in the store: the 404 above is the
	// handler's, not an artifact of the fixture.
	d, err := b.s.Get(context.Background(), tenantA, b.binned.ID)
	if err != nil || d.Status != content.StatusPublished || !d.IsDeleted() {
		t.Fatalf("fixture: binned course = %q deleted=%v err=%v, want published and binned", d.Status, d.IsDeleted(), err)
	}
}

// The service-auth gate is the one in front of every delivery route.
func TestDeliveryByIDNeedsTheServiceToken(t *testing.T) {
	s := content.NewMemoryStore()
	h := NewRouter(Deps{Store: s, Registry: reg(t), ServiceToken: "svc"})
	path := "/delivery/v1/by-id/course/00000000-0000-4000-8000-0000000000ff"
	if rec := get(h, path, "", tenantA.String()); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", rec.Code)
	}
	if rec := get(h, path, "svc", ""); rec.Code != http.StatusForbidden {
		t.Errorf("no scope = %d, want 403", rec.Code)
	}
	if rec := get(h, path, "svc", tenantA.String()); rec.Code != http.StatusNotFound {
		t.Errorf("token and scope = %d, want 404 (gate passed, no document)", rec.Code)
	}
}

// The course tree resolves its refs through the same lookup, so a binned module
// drops out of the tree as it drops out of the slug route.
func TestDeliveryExpandCourseTreeOmitsABinnedModule(t *testing.T) {
	s := content.NewMemoryStore()
	ctx := context.Background()
	mk := func(typ, slug, body string) content.Document {
		d, err := s.Create(ctx, tenantA, content.CreateInput{Type: typ, Slug: slug, Title: slug, Body: json.RawMessage(body)})
		if err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
		if _, err := s.Publish(ctx, tenantA, d.ID); err != nil {
			t.Fatalf("publish %s: %v", slug, err)
		}
		return d
	}
	live := mk("module", "m1", `{}`)
	binned := mk("module", "m2", `{}`)
	mk("course", "c1", `{"modules":[{"module":"`+live.ID.String()+`"},{"module":"`+binned.ID.String()+`"}]}`)
	if _, err := s.SoftDelete(ctx, tenantA, binned.ID); err != nil {
		t.Fatalf("bin: %v", err)
	}

	h := NewRouter(Deps{TrustHeaderWithoutToken: true, Store: s, Registry: reg(t)})
	rec := get(h, "/delivery/v1/course/c1?expand=modules.lessons", "", tenantA.String())
	var env Envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	mods, _ := env.Data["modules"].([]any)
	if rec.Code != http.StatusOK || len(mods) != 1 {
		t.Fatalf("expand = %d with %d modules, want 200 and 1 (m1 only): %v", rec.Code, len(mods), env.Data["modules"])
	}
	if m0, _ := mods[0].(map[string]any); m0["slug"] != "m1" {
		t.Errorf("module = %v, want m1", m0)
	}
}

// otherDataset answers Get with the document moved to another dataset. No store
// path creates a second dataset yet, so this stub is the only way to exercise
// the dataset condition GetPublished also holds.
type otherDataset struct{ *content.MemoryStore }

func (o otherDataset) Get(ctx context.Context, tenantID, id uuid.UUID) (content.Document, error) {
	d, err := o.MemoryStore.Get(ctx, tenantID, id)
	d.Dataset = "staging"
	return d, err
}

func TestDeliveryByIDServesOnlyTheDefaultDataset(t *testing.T) {
	b := newByIDStore(t)
	h := NewRouter(Deps{Store: otherDataset{b.s}, Registry: reg(t), TrustHeaderWithoutToken: true})
	if rec := get(h, byIDPath("course", b.course), "", tenantA.String()); rec.Code != http.StatusNotFound {
		t.Errorf("by-id in another dataset = %d, want 404", rec.Code)
	}
}
