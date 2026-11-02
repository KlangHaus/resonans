package delivery

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

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/preview"
)

// errDB stands for a failure the store cannot classify. Its text must reach
// the log and must NOT reach the caller.
var errDB = errors.New("pq: connection to 10.0.0.7 refused")

// storeStub is the memory store with its read paths replaceable, for the cases
// a real store does not produce on demand: a failure, or a list in an order the
// handler has to correct.
type storeStub struct {
	*content.MemoryStore
	list    func() ([]content.Document, error)
	failGet bool
}

func (s storeStub) List(ctx context.Context, tid uuid.UUID, f content.ListFilter) ([]content.Document, error) {
	if s.list != nil {
		return s.list()
	}
	return s.MemoryStore.List(ctx, tid, f)
}

func (s storeStub) GetPublished(ctx context.Context, tid uuid.UUID, typ, slug string) (content.Document, error) {
	if s.failGet {
		return content.Document{}, errDB
	}
	return s.MemoryStore.GetPublished(ctx, tid, typ, slug)
}

func (s storeStub) GetBySlug(ctx context.Context, tid uuid.UUID, typ, slug string) (content.Document, error) {
	if s.failGet {
		return content.Document{}, errDB
	}
	return s.MemoryStore.GetBySlug(ctx, tid, typ, slug)
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not JSON (%v): %s", err, rec.Body)
	}
	return m
}

func TestDeliverySchemaFlagsTheQuizAsServerSideOnly(t *testing.T) {
	h := NewRouter(Deps{Store: content.NewMemoryStore(), Registry: reg(t)})
	rec := get(h, "/delivery/v1/schema", "", tenantA.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("schema = %d, body=%s", rec.Code, rec.Body)
	}
	body := decode(t, rec)
	docTypes, _ := body["docTypes"].(map[string]any)
	quiz, _ := docTypes["quiz"].(map[string]any)
	if quiz["serverSideOnly"] != true {
		t.Errorf("quiz.serverSideOnly = %v, want true: the answer key must not reach a browser", quiz["serverSideOnly"])
	}
	page, _ := docTypes["page"].(map[string]any)
	if _, ok := page["serverSideOnly"]; ok {
		t.Errorf("page carries serverSideOnly; only the quiz holds an answer key")
	}
	if allowed, _ := page["allowedBlocks"].([]any); len(allowed) == 0 {
		t.Errorf("page.allowedBlocks is empty; a page has a block field")
	}
	if quiz["allowedBlocks"] != nil {
		t.Errorf("quiz.allowedBlocks = %v; a quiz has no block field", quiz["allowedBlocks"])
	}
	blocks, _ := body["blocks"].(map[string]any)
	if _, ok := blocks["rich_text"]; !ok {
		t.Errorf("blocks lacks rich_text: %v", blocks)
	}
}

func TestDeliveryETagRevalidation(t *testing.T) {
	h := seeded(t, "")
	first := get(h, "/delivery/v1/course/intro", "", tenantA.String())
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" {
		t.Fatalf("first get = %d, ETag %q", first.Code, etag)
	}

	req := httptest.NewRequest(http.MethodGet, "/delivery/v1/course/intro", nil)
	req.Header.Set("X-Scope-OrgID", tenantA.String())
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != etag {
		t.Fatalf("revalidation = %d, %d body bytes, ETag %q; want 304, empty, %q", rec.Code, rec.Body.Len(), rec.Header().Get("ETag"), etag)
	}

	// Another locale is another representation: its ETag differs, so the
	// cached Danish copy is not served for English.
	req = httptest.NewRequest(http.MethodGet, "/delivery/v1/course/intro?locale=en", nil)
	req.Header.Set("X-Scope-OrgID", tenantA.String())
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") == etag {
		t.Fatalf("other locale = %d with ETag %q; want 200 and a different ETag", rec.Code, rec.Header().Get("ETag"))
	}
	if env := decode(t, rec); env["locale"] != "en" {
		t.Errorf("envelope locale = %v, want en", env["locale"])
	}
}

func TestDeliveryStoreFailureIsLoggedNotLeaked(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	stub := storeStub{
		MemoryStore: content.NewMemoryStore(),
		list:        func() ([]content.Document, error) { return nil, errDB },
		failGet:     true,
	}
	secret := "preview-secret"
	h := NewRouter(Deps{Store: stub, Registry: reg(t), PreviewSecret: secret, Logger: logger})
	tok := preview.Sign([]byte(secret), preview.Claims{
		Tenant: tenantA.String(), Type: "page", Slug: "x", Exp: time.Now().Add(time.Minute).Unix(),
	})

	for _, path := range []string{
		"/delivery/v1/course",
		"/delivery/v1/course/intro",
		"/delivery/v1/preview/page/x?token=" + tok,
	} {
		rec := get(h, path, "", tenantA.String())
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s = %d, want 500", path, rec.Code)
		}
		if body := rec.Body.String(); strings.Contains(body, "10.0.0.7") || !strings.Contains(body, "internal error") {
			t.Errorf("%s body = %s; want a generic error with no store detail", path, body)
		}
	}
	if n := strings.Count(logBuf.String(), "10.0.0.7"); n != 3 {
		t.Errorf("the store error was logged %d times, want 3 (one per request)", n)
	}
}

func TestDeliveryRefusesAMalformedTenant(t *testing.T) {
	h := seeded(t, "")
	rec := get(h, "/delivery/v1/course", "", "not-a-uuid")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("malformed scope = %d, want 403", rec.Code)
	}
}

func TestDeliveryPreviewRefusals(t *testing.T) {
	s := content.NewMemoryStore()
	if _, err := s.Create(context.Background(), tenantA, content.CreateInput{Type: "page", Slug: "d", Title: "D"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	secret := "preview-secret"
	valid := preview.Claims{Tenant: tenantA.String(), Type: "page", Slug: "d", Exp: time.Now().Add(time.Minute).Unix()}

	// No secret configured: preview does not exist, even for a token that
	// would verify under some secret.
	off := NewRouter(Deps{Store: s, Registry: reg(t)})
	if rec := get(off, "/delivery/v1/preview/page/d?token="+preview.Sign([]byte(secret), valid), "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("preview with no secret = %d, want 404", rec.Code)
	}

	on := NewRouter(Deps{Store: s, Registry: reg(t), PreviewSecret: secret})
	if rec := get(on, "/delivery/v1/preview/page/d?token="+preview.Sign([]byte(secret), valid), "", ""); rec.Code != http.StatusOK {
		t.Fatalf("control: a valid token = %d, want 200", rec.Code)
	}
	badTenant := valid
	badTenant.Tenant = "not-a-uuid"
	if rec := get(on, "/delivery/v1/preview/page/d?token="+preview.Sign([]byte(secret), badTenant), "", ""); rec.Code != http.StatusForbidden {
		t.Errorf("token with a malformed tenant = %d, want 403", rec.Code)
	}
	otherSecret := preview.Sign([]byte("another-secret"), valid)
	if rec := get(on, "/delivery/v1/preview/page/d?token="+otherSecret, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("token signed with another secret = %d, want 401", rec.Code)
	}
}

// The catalog orders by publish date, not by what the store returns: a
// re-published document rises to the top. Ties fall back to creation date and
// then id, so the order is total and stable across requests.
func TestDeliveryCatalogOrderAndTruncation(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := t0.Add(d); return &v }
	idA := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	idB := uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	doc := func(slug string, id uuid.UUID, pub *time.Time, created time.Duration) content.Document {
		return content.Document{
			ID: id, Type: "post", Slug: slug, Status: content.StatusPublished,
			PublishedAt: pub, CreatedAt: t0.Add(created), Body: json.RawMessage(`{}`),
		}
	}
	docs := []content.Document{
		doc("no-date", uuid.New(), nil, 9*time.Hour),
		doc("tie-b", idB, at(time.Hour), 0),
		doc("old", uuid.New(), at(0), 0),
		doc("tie-a", idA, at(time.Hour), 0),
		doc("same-pub-newer", uuid.New(), at(time.Hour), time.Minute),
		doc("newest", uuid.New(), at(2*time.Hour), 0),
	}
	h := NewRouter(Deps{Registry: reg(t), Store: storeStub{
		MemoryStore: content.NewMemoryStore(),
		list:        func() ([]content.Document, error) { return append([]content.Document(nil), docs...), nil },
	}})
	rec := get(h, "/delivery/v1/post", "", tenantA.String())
	var resp struct {
		Items     []map[string]any `json:"items"`
		Truncated bool             `json:"truncated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var got []string
	for _, it := range resp.Items {
		got = append(got, fmt.Sprint(it["slug"]))
	}
	want := "newest,same-pub-newer,tie-a,tie-b,old,no-date"
	if strings.Join(got, ",") != want || resp.Truncated {
		t.Errorf("order = %s truncated=%v; want %s, not truncated", strings.Join(got, ","), resp.Truncated, want)
	}

	for _, c := range []struct {
		n         int
		wantItems int
		truncated bool
	}{{maxList, maxList, false}, {maxList + 1, maxList, true}} {
		many := make([]content.Document, c.n)
		for i := range many {
			many[i] = doc(fmt.Sprintf("p%d", i), uuid.New(), at(time.Duration(i)*time.Second), 0)
		}
		h := NewRouter(Deps{Registry: reg(t), Store: storeStub{
			MemoryStore: content.NewMemoryStore(),
			list:        func() ([]content.Document, error) { return many, nil },
		}})
		rec := get(h, "/delivery/v1/post", "", tenantA.String())
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Items) != c.wantItems || resp.Truncated != c.truncated {
			t.Errorf("%d documents → %d items, truncated=%v; want %d, %v", c.n, len(resp.Items), resp.Truncated, c.wantItems, c.truncated)
		}
		// The cut keeps the most recently published, not the first stored.
		if c.truncated && resp.Items[0]["slug"] != fmt.Sprintf("p%d", c.n-1) {
			t.Errorf("first item after truncation = %v, want the newest p%d", resp.Items[0]["slug"], c.n-1)
		}
	}
}

func TestSummarizeChoosesTheTeaserAndThumbnail(t *testing.T) {
	d := content.Document{UpdatedAt: time.Now()}
	for _, c := range []struct {
		name string
		data map[string]any
		want any
	}{
		{"summary wins", map[string]any{"summary": "S", "description": "D", "excerpt": "E"}, "S"},
		{"then description", map[string]any{"description": "D", "excerpt": "E"}, "D"},
		{"then excerpt", map[string]any{"excerpt": "E"}, "E"},
		{"none", map[string]any{}, nil},
	} {
		if got := summarize(d, Envelope{Data: c.data})["summary"]; got != c.want {
			t.Errorf("%s: summary = %v, want %v", c.name, got, c.want)
		}
	}

	s := summarize(d, Envelope{Data: map[string]any{
		"author": "Allan", "category": "Studio",
		"coverImage": map[string]any{"url": "https://cdn/x.jpg", "alt": "Rummet", "width": 1200},
		"modules":    []any{"m1", "m2", "m3"},
	}})
	thumb, _ := s["thumbnail"].(map[string]any)
	if s["author"] != "Allan" || s["category"] != "Studio" || s["moduleCount"] != 3 {
		t.Errorf("metadata = author %v category %v moduleCount %v", s["author"], s["category"], s["moduleCount"])
	}
	if thumb["url"] != "https://cdn/x.jpg" || thumb["alt"] != "Rummet" || len(thumb) != 2 {
		t.Errorf("thumbnail = %v; want url and alt only", thumb)
	}
	noAlt, _ := summarize(d, Envelope{Data: map[string]any{"coverImage": map[string]any{"url": "u"}}})["thumbnail"].(map[string]any)
	if _, ok := noAlt["alt"]; ok {
		t.Errorf("thumbnail invents an alt: %v", noAlt)
	}
	if _, ok := summarize(d, Envelope{Data: map[string]any{"title": "x"}})["thumbnail"]; ok {
		t.Error("a document without a cover image got a thumbnail")
	}
}

func TestBuildEnvelopeFallsBackToAnyLocale(t *testing.T) {
	doc := content.Document{
		ID: uuid.New(), Type: "lesson", Slug: "s",
		Body: json.RawMessage(`{"summary": {"fr": "Résumé"}}`),
	}
	env := BuildEnvelope(context.Background(), uuid.New(), doc, reg(t), "en", "da", nil)
	if env.Data["summary"] != "Résumé" || !env.LocaleFallback {
		t.Errorf("summary = %v fallback=%v; want the only value, flagged", env.Data["summary"], env.LocaleFallback)
	}
	doc.Body = json.RawMessage(`{"summary": {}}`)
	if env := BuildEnvelope(context.Background(), uuid.New(), doc, reg(t), "en", "da", nil); env.Data["summary"] != nil {
		t.Errorf("empty translation map = %v, want nil", env.Data["summary"])
	}
	// A plain string in an i18n field (legacy content) is served as it is.
	doc.Body = json.RawMessage(`{"summary": "Plain"}`)
	if env := BuildEnvelope(context.Background(), uuid.New(), doc, reg(t), "en", "da", nil); env.Data["summary"] != "Plain" || env.LocaleFallback {
		t.Errorf("plain string = %v fallback=%v; want it unchanged, no fallback", env.Data["summary"], env.LocaleFallback)
	}
}

func TestBuildEnvelopePassthroughs(t *testing.T) {
	body := json.RawMessage(`{"anything": {"da": "x"}}`)
	unknown := content.Document{ID: uuid.New(), Type: "not-a-type", Body: body}
	if env := BuildEnvelope(context.Background(), uuid.New(), unknown, reg(t), "da", "da", nil); env.Data["anything"] == nil {
		t.Errorf("unknown doc-type data = %v, want the body passed through", env.Data)
	}
	lesson := content.Document{ID: uuid.New(), Type: "lesson", Body: body}
	if env := BuildEnvelope(context.Background(), uuid.New(), lesson, nil, "da", "da", nil); env.Data["anything"] == nil {
		t.Errorf("no registry: data = %v, want the body passed through", env.Data)
	}
	broken := content.Document{ID: uuid.New(), Type: "lesson", Slug: "b", Body: json.RawMessage(`{`)}
	env := BuildEnvelope(context.Background(), uuid.New(), broken, reg(t), "da", "da", nil)
	if len(env.Data) != 0 || env.Slug != "b" {
		t.Errorf("unparsable body: data = %v slug %q; want empty data and the metadata kept", env.Data, env.Slug)
	}
}
