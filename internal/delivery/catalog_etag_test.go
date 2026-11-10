package delivery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
)

// publishFour publishes courses a, b, c and d in that order, so the catalog
// (newest publish first) lists d, c, b, a.
func publishFour(t *testing.T) (*content.MemoryStore, map[string]content.Document) {
	t.Helper()
	s := content.NewMemoryStore()
	ctx := context.Background()
	docs := map[string]content.Document{}
	for _, slug := range []string{"a", "b", "c", "d"} {
		d, err := s.Create(ctx, tenantA, content.CreateInput{Type: "course", Slug: slug, Title: slug})
		if err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
		if d, err = s.Publish(ctx, tenantA, d.ID); err != nil {
			t.Fatalf("publish %s: %v", slug, err)
		}
		docs[slug] = d
	}
	return s, docs
}

func getWith(h http.Handler, path string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Scope-OrgID", tenantA.String())
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The green case first: a catalog page carries a weak ETag, the same one on
// every request, and that ETag answers 304 with no body.
func TestCatalogETagAnswers304(t *testing.T) {
	s, _ := publishFour(t)
	h := NewRouter(Deps{Store: s, Registry: reg(t), TrustHeaderWithoutToken: true})

	first := get(h, pageOfTwo, "", tenantA.String())
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || len(etag) < 4 || etag[:3] != `W/"` {
		t.Fatalf("catalog = %d ETag %q, want 200 and a weak ETag", first.Code, etag)
	}
	if again := get(h, pageOfTwo, "", tenantA.String()); again.Header().Get("ETag") != etag {
		t.Fatalf("ETag changed between two identical requests: %q, %q", etag, again.Header().Get("ETag"))
	}
	if cc := first.Header().Get("Cache-Control"); cc != "public, max-age=0, must-revalidate" {
		t.Errorf("Cache-Control = %q", cc)
	}

	for name, inm := range map[string]string{
		"the tag":            etag,
		"without W/":         etag[2:],
		"in a list":          `"other", ` + etag,
		"any representation": "*",
	} {
		rec := getWith(h, pageOfTwo, map[string]string{"If-None-Match": inm})
		if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != etag {
			t.Errorf("%s: If-None-Match %q = %d, %d bytes, ETag %q; want 304, 0, %q",
				name, inm, rec.Code, rec.Body.Len(), rec.Header().Get("ETag"), etag)
		}
	}
	if rec := getWith(h, pageOfTwo, map[string]string{"If-None-Match": `W/"other"`}); rec.Code != http.StatusOK {
		t.Errorf("a stale tag = %d, want 200", rec.Code)
	}
}

const (
	pageOfTwo   = "/delivery/v1/course?limit=2"
	pageOfTwoEN = pageOfTwo + "&locale=en"
	nextCursor  = `"nextCursor"`
)

// catalogCase is four published courses (d, c, b, a in catalog order) behind a
// router, and the ETag of their first page of two.
type catalogCase struct {
	t    *testing.T
	s    *content.MemoryStore
	docs map[string]content.Document
	h    http.Handler
	base string
}

func newCatalogCase(t *testing.T) *catalogCase {
	t.Helper()
	s, docs := publishFour(t)
	c := &catalogCase{t: t, s: s, docs: docs, h: NewRouter(Deps{Store: s, Registry: reg(t), TrustHeaderWithoutToken: true})}
	c.base = c.etagOf(c.h, pageOfTwo)
	return c
}

func (c *catalogCase) etagOf(h http.Handler, path string) string {
	c.t.Helper()
	rec := get(h, path, "", tenantA.String())
	if rec.Code != http.StatusOK {
		c.t.Fatalf("%s = %d: %s", path, rec.Code, rec.Body)
	}
	return rec.Header().Get("ETag")
}

func (c *catalogCase) bin(slugs ...string) {
	c.t.Helper()
	for _, slug := range slugs {
		if _, err := c.s.SoftDelete(context.Background(), tenantA, c.docs[slug].ID); err != nil {
			c.t.Fatal(err)
		}
	}
}

func TestCatalogETagMovesWithTheRequest(t *testing.T) {
	c := newCatalogCase(t)
	if c.etagOf(c.h, pageOfTwoEN) == c.base {
		t.Error("same ETag for another locale")
	}
	if c.etagOf(c.h, "/delivery/v1/course?limit=4") == c.base {
		t.Error("same ETag for a larger page")
	}
	// With ?locale named, so the default only acts as the fallback: without it
	// the default is also the requested locale, and this would pass on the
	// locale alone.
	other := NewRouter(Deps{Store: c.s, Registry: reg(t), TrustHeaderWithoutToken: true, DefaultLocale: "sv"})
	if c.etagOf(other, pageOfTwoEN) == c.etagOf(c.h, pageOfTwoEN) {
		t.Error("same ETag under another default locale")
	}
}

// The case a newest-write-and-row-count key gets wrong: the page d, c loses c
// and gains the older b. Its newest write is still d's, it still has two rows
// and another page still follows, but it is a different page.
func TestCatalogETagMovesWhenAnOlderDocumentEntersThePage(t *testing.T) {
	c := newCatalogCase(t)
	naiveBefore := naiveKey(t, c.s, c.docs["d"].ID, c.docs["c"].ID)
	c.bin("c")
	if naiveAfter := naiveKey(t, c.s, c.docs["d"].ID, c.docs["b"].ID); naiveAfter != naiveBefore {
		t.Fatalf("fixture: the naive key moved (%s → %s), so this case does not show what it is for", naiveBefore, naiveAfter)
	}
	rec := get(c.h, pageOfTwo, "", tenantA.String())
	if !strings.Contains(rec.Body.String(), nextCursor) {
		t.Fatalf("fixture: no next page after the change, so the ETag could move on that alone: %s", rec.Body)
	}
	if rec.Header().Get("ETag") == c.base {
		t.Errorf("same ETag after the page changed from d,c to d,b: %s", rec.Body)
	}
}

func TestCatalogETagMovesWithAnEditRepublishedInPlace(t *testing.T) {
	c := newCatalogCase(t)
	ctx := context.Background()
	title := "d, again"
	if _, err := c.s.Update(ctx, tenantA, c.docs["d"].ID, content.UpdateInput{Title: &title}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.s.Publish(ctx, tenantA, c.docs["d"].ID); err != nil {
		t.Fatal(err)
	}
	if c.etagOf(c.h, pageOfTwo) == c.base {
		t.Error("same ETag after d was edited and republished")
	}
}

// The page's own documents stay d, c while everything after them is binned: the
// body loses its nextCursor, so the ETag must move with it.
func TestCatalogETagMovesWhenTheNextPageGoes(t *testing.T) {
	c := newCatalogCase(t)
	before := get(c.h, pageOfTwo, "", tenantA.String())
	c.bin("a", "b")
	after := get(c.h, pageOfTwo, "", tenantA.String())
	if !strings.Contains(before.Body.String(), nextCursor) || strings.Contains(after.Body.String(), nextCursor) {
		t.Fatalf("fixture: want a next page before and none after:\n%s\n%s", before.Body, after.Body)
	}
	if before.Header().Get("ETag") == after.Header().Get("ETag") {
		t.Errorf("same ETag with and without a next page: %s", after.Header().Get("ETag"))
	}
}

// naiveKey is the key this ETag deliberately is not: the newest write on the
// page and the page's row count, read from the store.
func naiveKey(t *testing.T, s *content.MemoryStore, ids ...uuid.UUID) string {
	t.Helper()
	var newest int64
	for _, id := range ids {
		d, err := s.Get(context.Background(), tenantA, id)
		if err != nil {
			t.Fatal(err)
		}
		newest = max(newest, d.UpdatedAt.UnixMicro())
	}
	return fmt.Sprintf("%d/%d", newest, len(ids))
}
