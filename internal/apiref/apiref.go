// Package apiref serves the OpenAPI document and a browser page that renders
// it. It is PUBLIC: it carries no tenant data, and an API description that
// needs a login is not a description anyone outside can read.
//
// Why it is not mounted under /api/: measured, `auth.Middleware` has no
// exemptions, so every /api/ route needs a JWT. /delivery/ is worse -- the
// cell in internal/apidocs requires the service token on every delivery
// route, so a public route there would break it. The house pattern for a
// public surface is the outer mux in internal/app, which is what
// internal/bridge does, and the version lives in the path for the same
// reason it does there.
//
// 🔴 THE RENDERER IS NOT EMBEDDED, AND THAT IS DELIBERATE. The Scalar bundle
// is 4.4 MB raw (1.20 MB gzipped, measured 2026-10-02 for 1.72.4). Vendoring
// it would put 1.20 MB into a git history that is about to become public, for
// a repository whose .git is 4.2 MB and whose largest tracked file is 244 KB.
// So it is fetched at IMAGE build time against a pinned sha256 and served
// from disk. That keeps `go build` offline and reproducible -- the binary does
// not need the asset -- and puts the download where the pin can be verified.
//
// The consequence, stated rather than hidden: `go run ./cmd/resonans` has no
// image, so the renderer is ABSENT in development. The page then says so and
// links the specification, which is a degradation a reader can act on. It
// must never be a blank page: a renderer that fails silently looks like a
// specification with no routes in it.
package apiref

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	apispec "github.com/KlangHaus/resonans/docs/api"
)

// Mount paths. The version is in the path so a breaking change to the page
// gets a new path instead of silently changing what a bookmark renders.
const (
	PagePath     = "/docs/v1/"
	SpecPath     = "/docs/v1/openapi.yaml"
	RendererPath = "/docs/v1/renderer.js"
)

// The renderer is looked for in these places, in order, each as a FIXED
// directory plus a FIXED file name.
//
// 🔴 Why a filesystem and a name rather than a path string: a path built at
// runtime makes os.ReadFile a file-inclusion sink (gosec G304), and this
// repository does not suppress lint findings. Removing the environment
// variable was not enough -- a loop variable is still a variable. Reading
// through an fs.FS with a constant name removes the sink AND keeps the
// development case, because a test can hand the same function an fstest.MapFS
// instead of a second code path that only looks like the real one.
const rendererName = "renderer.js.gz"

// candidate is one place to look: where the Dockerfile COPYs the bundle, and a
// repository-relative directory for `go run ./cmd/resonans` (gitignored, so a
// developer drops a copy there and the page renders locally too).
type candidate struct {
	fsys  fs.FS
	label string
}

func imageCandidates() []candidate {
	return []candidate{
		{os.DirFS("/app/apidocs"), "/app/apidocs/" + rendererName},
		{os.DirFS(".apidocs"), ".apidocs/" + rendererName},
	}
}

// The three handlers are exported and registered by internal/app rather than
// behind a sub-router here, and that is the drift cell's doing: measured, its
// resolvePattern has no *ast.Ident case, so a registration written inside this
// package as `"GET "+PagePath` is unresolvable and the cell cannot see it.
// Registered in app.go as `apiref.PagePath` it is a selector the cell resolves
// from the real constant. The encapsulation would have cost the coverage.

// spec serves the embedded OpenAPI document -- the same bytes the drift cell
// in internal/apidocs compares against the registered routes, so the page
// cannot render a description that the cell is not keeping honest.
func Spec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(apispec.Document)
}

// Asset holds the renderer bundle, read ONCE at construction.
//
// Why not opened per request: a variable path in os.Open is a file-inclusion
// sink (gosec G304), and the honest fix is to remove the sink rather than
// silence the warning. Reading once also moves the answer to "is the renderer
// installed" from the first reader who happens to load the page to the boot
// log, where an operator sees it.
type Asset struct {
	gz   []byte
	path string
	at   time.Time
	err  error
}

// Load reads the renderer from the configured path. A missing file is not an
// error to fail boot on -- the binary serves the specification either way --
// so the failure is carried and reported by the handler and by Describe.
func Load() *Asset { return load(imageCandidates()) }

// load reads the first candidate that has the file. It takes its candidates so
// a test exercises THIS function with the same shape of input the binary uses,
// rather than a parallel path that resembles it.
func load(candidates []candidate) *Asset {
	labels := make([]string, 0, len(candidates))
	for _, c := range candidates {
		labels = append(labels, c.label)
		gz, err := fs.ReadFile(c.fsys, rendererName)
		if err != nil {
			continue
		}
		at := time.Now()
		if st, err := fs.Stat(c.fsys, rendererName); err == nil {
			at = st.ModTime()
		}
		return &Asset{gz: gz, at: at, path: c.label}
	}
	return &Asset{err: fs.ErrNotExist, path: strings.Join(labels, " or "), at: time.Now()}
}

// Describe is what the boot log reports: present and how large, or absent and
// where it was looked for. Never silent -- an absent renderer that logs
// nothing is a page that degrades with no record of why.
func (a *Asset) Describe() (ok bool, detail string) {
	if a.err != nil {
		return false, fmt.Sprintf("absent at %s (%v)", a.path, a.err)
	}
	return true, fmt.Sprintf("%d gzipped bytes from %s", len(a.gz), a.path)
}

// Renderer serves the pre-gzipped bundle.
//
// When it is absent the answer is 503 naming the path. NOT 404: 404 says the
// route does not exist, which would send a reader looking for a typo in their
// URL instead of at an uninstalled asset. The failure has to be
// distinguishable from both a working page and a wrong path.
func (a *Asset) Renderer(w http.ResponseWriter, r *http.Request) {
	if a.err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, "api-reference renderer not installed (looked in %s)\n"+
			"The container image fetches it at build time. Running outside the\n"+
			"image, drop a gzipped copy at .apidocs/%s, or read the\n"+
			"specification directly at %s.\n", a.path, rendererName, SpecPath)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeContent(w, r, "renderer.js", a.at, bytes.NewReader(a.gz))
}

// page serves the HTML that loads the renderer and points it at the spec.
//
// The Content-Security-Policy is restrictive on purpose, and one line of it is
// load-bearing: `font-src 'self' data:`. Vendoring or pinning a bundle stops
// it being FETCHED from a third party; it does not stop it REACHING OUT.
// [backstage] measured fourteen requests to fonts.scalar.com from this very
// bundle, all blocked by exactly this directive. Self-hosted is not
// self-contained.
//
// style-src keeps 'unsafe-inline' because the bundle injects a <style> element
// at load (visible in its first 120 bytes). That is stated rather than left
// for a reader to discover, and it is the one directive here that is weaker
// than it looks.
func Page(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'none'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"font-src 'self' data:",
		"img-src 'self' data:",
		"connect-src 'self'",
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors 'none'",
	}, "; "))
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(pageHTML)
}

// pageHTML is written out rather than templated: there is no input to
// interpolate, and a template with no variables is a place for one to be added
// later without anyone noticing it is now reflected into a page.
var pageHTML = []byte(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Resonans API</title>
<style>
  body { margin: 0; font: 16px/1.5 ui-sans-serif, system-ui, sans-serif; }
  noscript, .fallback { display: block; padding: 2rem; max-width: 46rem; }
  code { background: #f4f4f5; padding: .1em .35em; border-radius: 3px; }
</style>
</head>
<body>
<noscript class="fallback">
  <h1>Resonans API</h1>
  <p>This page renders the specification with JavaScript. Without it, read the
  document directly: <a href="` + SpecPath + `"><code>` + SpecPath + `</code></a></p>
</noscript>
<script id="api-reference" data-url="` + SpecPath + `"></script>
<script src="` + RendererPath + `" onerror="document.body.insertAdjacentHTML('beforeend', document.getElementById('absent').innerHTML)"></script>
<template id="absent"><div class="fallback">
  <h1>Resonans API</h1>
  <p><strong>The renderer is not installed here.</strong> It is fetched when the
  container image is built, so it is absent when the binary runs outside the
  image &mdash; for example under <code>go run ./cmd/resonans</code>.</p>
  <p>The specification itself is served and complete:
  <a href="` + SpecPath + `"><code>` + SpecPath + `</code></a></p>
</div></template>
</body>
</html>
`)
