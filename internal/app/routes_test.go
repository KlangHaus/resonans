package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KlangHaus/resonans/internal/apiref"
)

// The MOUNTED patterns, which nothing measured before this file existed:
// measured 2026-10-02, internal/app had zero tests naming any route path. The
// handler cells in internal/apiref call the handlers directly, so a mistake in
// the pattern — a missing `{$}`, a wrong prefix, a method typo — passes every
// one of them and fails only in production.
//
// This mirrors the registrations in Run rather than calling it, because Run
// needs a database. The cost is stated: if someone changes the registration in
// Run and not here, this cell keeps passing. What it does catch is the thing it
// is about — that these three patterns, built from these three constants,
// resolve to the URLs the specification documents.
func TestMountedAPIReferencePatternsResolve(t *testing.T) {
	asset := apiref.Load()
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+apiref.PagePath+"{$}", apiref.Page)
	mux.HandleFunc("GET "+apiref.SpecPath, apiref.Spec)
	mux.HandleFunc("GET "+apiref.RendererPath, asset.Renderer)

	for _, c := range []struct {
		method, path string
		want         int
		why          string
	}{
		{http.MethodGet, apiref.PagePath, http.StatusOK, "the page itself"},
		{http.MethodGet, apiref.SpecPath, http.StatusOK, "the document: a more specific pattern must win over the page"},
		// `{$}` is what buys this: without the anchor, /docs/v1/ is a subtree
		// pattern and every unknown path below it would serve the page with a
		// 200, which reads as a working URL.
		{http.MethodGet, apiref.PagePath + "not-a-page", http.StatusNotFound, "the end anchor holds"},
		{http.MethodGet, "/docs/v2/", http.StatusNotFound, "the version is in the path, so v2 is not v1"},
		{http.MethodPost, apiref.PagePath, http.StatusMethodNotAllowed, "documentation is read, not written"},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		if w.Code != c.want {
			t.Errorf("%s %s -> %d, want %d (%s)", c.method, c.path, w.Code, c.want, c.why)
		}
	}

	// The renderer route answers either way, and which way depends on whether
	// the image build has run. Both are correct; a 404 is not, because that
	// would mean the route is not mounted at all.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, apiref.RendererPath, nil))
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Errorf("GET %s -> %d, want 200 (installed) or 503 (absent)", apiref.RendererPath, w.Code)
	}
	if present, detail := asset.Describe(); !present && w.Code != http.StatusServiceUnavailable {
		t.Errorf("the asset reports %q but the route answered %d", detail, w.Code)
	}
}
