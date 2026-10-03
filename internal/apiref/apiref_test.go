package apiref

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func get(t *testing.T, h http.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// The served document must BE the repository's specification, not a copy of
// it. A copy would let the drift cell in internal/apidocs keep one file honest
// while the page rendered the other.
func TestSpecServesTheRepositoryDocument(t *testing.T) {
	onDisk, err := os.ReadFile(filepath.Join("..", "..", "docs", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read the specification from disk: %v", err)
	}
	if len(onDisk) == 0 {
		t.Fatal("the specification on disk is empty, so this test proves nothing")
	}
	w := get(t, Spec, SpecPath)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !bytes.Equal(w.Body.Bytes(), onDisk) {
		t.Errorf("served %d bytes, the file on disk is %d: the embed is not the same document",
			w.Body.Len(), len(onDisk))
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("Content-Type = %q", ct)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("X-Content-Type-Options missing")
	}
}

// The page must carry the restrictive policy, and font-src is named explicitly
// because it is the directive that blocks the bundle's own outbound requests.
func TestPageCarriesItsPolicyAndItsFallback(t *testing.T) {
	w := get(t, Page, PagePath)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	csp := w.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'none'", "script-src 'self'", "font-src 'self' data:",
		"connect-src 'self'", "frame-ancestors 'none'",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP lacks %q: %s", want, csp)
		}
	}
	body := w.Body.String()
	// Both paths must be referenced, or the page cannot load what it renders.
	for _, want := range []string{SpecPath, RendererPath} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not reference %q", want)
		}
	}
	// The degradation is part of the page, not a thing to hope for: a reader
	// with no renderer must be told so and handed the document.
	if !strings.Contains(body, "not installed") {
		t.Error("page has no visible fallback for an absent renderer")
	}
	if strings.Count(body, SpecPath) < 2 {
		t.Errorf("the specification is linked %d times; the noscript and the "+
			"absent-renderer paths both need it", strings.Count(body, SpecPath))
	}
}

// Renderer present and absent, in the same run. The absent arm is the one that
// matters: it is what every developer sees, so it must be loud and specific.
func TestRendererPresentAndAbsent(t *testing.T) {
	raw := []byte("globalThis.__scalar_test__ = 1;\n")
	gz := gzipped(t, raw)

	w := get(t, load([]candidate{{fstest.MapFS{rendererName: &fstest.MapFile{Data: gz}}, "fixture"}}).Renderer, RendererPath)
	if w.Code != http.StatusOK {
		t.Fatalf("present: status = %d, want 200", w.Code)
	}
	if enc := w.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Errorf("present: Content-Encoding = %q, want gzip", enc)
	}
	// The bytes are served compressed and are never decompressed on our side.
	if !bytes.Equal(w.Body.Bytes(), gz) {
		t.Error("present: body is not the gzipped file verbatim")
	}
	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("present: served body is not valid gzip: %v", err)
	}
	got := new(bytes.Buffer)
	if _, err := got.ReadFrom(zr); err != nil {
		t.Fatalf("present: gunzip: %v", err)
	}
	if !bytes.Equal(got.Bytes(), raw) {
		t.Error("present: the round trip does not return the original bytes")
	}

	empty := load([]candidate{{fstest.MapFS{}, "/app/apidocs/" + rendererName}})
	w = get(t, empty.Renderer, RendererPath)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("absent: status = %d, want 503 — 404 would send a reader "+
			"looking for a typo in their URL", w.Code)
	}
	for _, want := range []string{rendererName, SpecPath, ".apidocs/"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("absent: the 503 body does not name %q: %s", want, w.Body.String())
		}
	}
}

// The candidate list is tried IN ORDER, and that order is the whole
// development story: the image directory first, the repository-relative one
// second. A test that only proved "a present file is served" would pass even
// if the order were reversed or the second candidate never consulted.
func TestLoadTriesEveryCandidateInOrder(t *testing.T) {
	one := fstest.MapFS{rendererName: &fstest.MapFile{Data: gzipped(t, []byte("one"))}}
	two := fstest.MapFS{rendererName: &fstest.MapFile{Data: gzipped(t, []byte("two"))}}
	none := fstest.MapFS{}

	if got := load([]candidate{{one, "first"}, {two, "second"}}); got.path != "first" {
		t.Errorf("both present: loaded %q, want the first candidate", got.path)
	}
	got := load([]candidate{{none, "first"}, {two, "second"}})
	if got.err != nil || got.path != "second" {
		t.Errorf("first absent: path=%q err=%v, want a fall-through to the second",
			got.path, got.err)
	}
	all := load([]candidate{{none, "first"}, {none, "second"}})
	if all.err == nil {
		t.Fatal("all absent: err is nil")
	}
	ok, detail := all.Describe()
	if ok {
		t.Error("all absent: Describe reports present")
	}
	// Every place it looked is named, so a reader is not left guessing which
	// path was meant.
	for _, want := range []string{"first", "second"} {
		if !strings.Contains(detail, want) {
			t.Errorf("Describe does not name %q: %s", want, detail)
		}
	}
}

func gzipped(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
