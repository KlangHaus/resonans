// Package apidocs holds no code. It holds the one cell that keeps
// docs/api/openapi.yaml honest.
//
// A hand-written API description next to a router is out of date within a month
// — this repository's own README proved it, listing roughly fourteen routes
// where the code had twenty-nine. So the specification is not the artifact to
// trust; the CELL that compares it to the code is. Add a route without touching
// the spec and this test turns red, naming the route and the direction.
//
// It lives in its own package on purpose: it reads the OTHER packages' source
// with go/parser rather than importing them, so it sees what actually registers
// a route instead of a table those packages and this test would share. A shared
// table would make the cell look like the production path exactly where it is
// not one — a route registered directly on the mux would bypass it and stay
// invisible.
package apidocs

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/KlangHaus/resonans/internal/apiref"
	"github.com/KlangHaus/resonans/internal/bridge"
	"github.com/KlangHaus/resonans/internal/media"
)

// specPath is the document under test, relative to this package.
const specPath = "../../docs/api/openapi.yaml"

// sourceDirs are every package that registers an HTTP route. A new router
// package that is not listed here would be invisible to this cell, so the list
// is asserted against the tree below rather than trusted.
var sourceDirs = []string{
	"../httpapi",
	"../delivery",
	"../app",
}

// mountPrefixes are the two ServeMux entries that mount a sub-router rather than
// answering a request. They are not endpoints and are not documented.
// mountPrefixes is this cell's DENOMINATOR, and it is the thing to update
// when a new surface is mounted -- not something to work around. Measured
// 2026-10-02: /healthz, /readyz and /bridge/v1/resonans-bridge.js are
// registered and fall outside it, so this cell's green says "every route
// under these prefixes", not "every route". /docs/ was added when the API
// reference was mounted, because the alternative was a public surface living
// in this cell's blind spot.
var mountPrefixes = map[string]bool{"/api/": true, "/delivery/": true, "/docs/": true}

// resolvable maps the identifiers a route pattern may be built from to their
// values. Taken from the real constants, so renaming or re-pointing one moves
// this cell with it instead of leaving it asserting a stale literal.
var resolvable = map[string]string{
	"media.AssetPath":     media.AssetPath,
	"bridge.Path":         bridge.Path,
	"apiref.PagePath":     apiref.PagePath,
	"apiref.SpecPath":     apiref.SpecPath,
	"apiref.RendererPath": apiref.RendererPath,
}

type route struct {
	method string
	path   string
	where  string
}

func (r route) key() string { return r.method + " " + r.path }

// resolvePattern turns the first argument of a mux registration into a pattern
// string. It returns an error rather than a guess: an unresolvable argument must
// make this cell RED, because a pattern it cannot read is a route it cannot see,
// and an empty string would silently pass as one more documented path.
func resolvePattern(e ast.Expr) (string, error) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", fmt.Errorf("non-string literal %s", v.Value)
		}
		return strconv.Unquote(v.Value)
	case *ast.SelectorExpr:
		if x, ok := v.X.(*ast.Ident); ok {
			name := x.Name + "." + v.Sel.Name
			if s, ok := resolvable[name]; ok {
				return s, nil
			}
			return "", fmt.Errorf("unresolvable identifier %s — add it to resolvable, do not skip it", name)
		}
		return "", fmt.Errorf("unresolvable selector")
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", fmt.Errorf("unsupported operator %s", v.Op)
		}
		l, err := resolvePattern(v.X)
		if err != nil {
			return "", err
		}
		r, err := resolvePattern(v.Y)
		if err != nil {
			return "", err
		}
		return l + r, nil
	default:
		return "", fmt.Errorf("unsupported expression %T", e)
	}
}

// routesFromSource parses every listed package and returns the routes it
// registers. Concatenation matters: `"GET "+media.AssetPath+"{key...}"` is a real
// registration in app.go, and a line-bound regex reading only the first literal
// reports the method with an EMPTY path — a phantom that looks like a documented
// endpoint. That is why this reads the syntax tree.
func routesFromSource(t *testing.T) []route {
	t.Helper()
	fset := token.NewFileSet()
	var out []route
	for _, dir := range sourceDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v — sourceDirs names a package that is not in the tree", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			full := filepath.Join(dir, name)
			f, err := parser.ParseFile(fset, full, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", full, err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "mux" {
					return true
				}
				pos := fset.Position(call.Pos())
				pattern, err := resolvePattern(call.Args[0])
				if err != nil {
					t.Fatalf("%s:%d: cannot read this route pattern: %v", pos.Filename, pos.Line, err)
				}
				method, path := "GET", pattern
				if i := strings.IndexByte(pattern, ' '); i >= 0 {
					method, path = pattern[:i], pattern[i+1:]
				}
				if mountPrefixes[path] {
					return true
				}
				// `{key...}` and `{key}` are the same endpoint to a reader.
				path = strings.ReplaceAll(path, "...}", "}")
				// `{$}` is net/http's end-of-path ANCHOR, not part of the URL: it
				// says "this exact path and no subtree". An OpenAPI path cannot
				// contain it, so leaving it in would force the specification to
				// carry a Go implementation detail -- or, worse, would push whoever
				// hit it into dropping the anchor and silently widening the route to
				// a subtree. Measured 2026-10-02: one registration uses it.
				path = strings.TrimSuffix(path, "{$}")
				out = append(out, route{method: method, path: path, where: fmt.Sprintf("%s:%d", pos.Filename, pos.Line)})
				return true
			})
		}
	}
	if len(out) == 0 {
		t.Fatal("found no routes at all — the parser, not the code, is what failed")
	}
	return out
}

// routesFromSpec reads the documented operations.
func routesFromSpec(t *testing.T) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read %s: %v", specPath, err)
	}
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse %s: %v", specPath, err)
	}
	out := map[string]bool{}
	for path, ops := range doc.Paths {
		for method := range ops {
			if strings.HasPrefix(method, "x-") || method == "parameters" {
				continue
			}
			out[strings.ToUpper(method)+" "+path] = true
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s documents no operations — an empty spec would otherwise satisfy nothing and look like a pass", specPath)
	}
	return out
}

func TestEveryRouteIsDocumentedAndNothingExtraIs(t *testing.T) {
	code := routesFromSource(t)
	spec := routesFromSpec(t)

	seen := map[string]string{}
	for _, r := range code {
		seen[r.key()] = r.where
	}

	var undocumented, phantom []string
	for key, where := range seen {
		if !spec[key] {
			undocumented = append(undocumented, fmt.Sprintf("%s  (registered at %s)", key, where))
		}
	}
	for key := range spec {
		if _, ok := seen[key]; !ok {
			phantom = append(phantom, key)
		}
	}
	sort.Strings(undocumented)
	sort.Strings(phantom)

	// The denominator goes in the output whether it passes or fails. A green that
	// does not say what it measured cannot be told from a green that measured
	// nothing.
	t.Logf("%d of %d registered routes documented; spec has %d operations",
		len(seen)-len(undocumented), len(seen), len(spec))

	for _, u := range undocumented {
		t.Errorf("route exists in code but not in docs/api/openapi.yaml: %s", u)
	}
	for _, p := range phantom {
		t.Errorf("docs/api/openapi.yaml documents a route that does not exist: %s", p)
	}
}

// sourceDirs is this cell's other denominator, and until now its comment
// claimed more than its assertion delivered: the existing check only verifies
// that each LISTED directory exists. It does not verify the list is COMPLETE,
// so a new package that registers a route would be invisible — which is
// precisely what the comment says must not happen.
//
// This walks the tree and asserts the two sets are equal, in BOTH directions:
// a package registering routes and missing from the list, and a listed
// directory that registers none. The second half matters too, because a stale
// entry is a reader being told the gate watches something it does not.
//
// 🔴 The discriminator is deliberately BROAD, and the direction is the reason.
// It matches `.Handle(`/`.HandleFunc(` in any non-test .go file that also
// imports net/http. Measured 2026-10-02 while writing this: without the
// net/http requirement, internal/observability/fanout.go matched on
// `h.Handle(ctx, r.Clone())` — a slog.Handler, not a route. With it, the set
// is exactly the three listed directories. A false positive here turns this
// cell red and somebody looks; a false negative leaves a served surface
// unwatched. Red-and-wrong is the cheaper mistake, so if this ever flags a
// package that registers nothing, widen the comment rather than narrowing the
// scan.
func TestSourceDirsListsEveryRouteRegisteringPackage(t *testing.T) {
	const repoRoot = "../.."
	skip := map[string]bool{
		".git": true, "node_modules": true, "studio": true,
		"testdata": true, ".output": true, ".nuxt": true, ".apidocs": true,
	}
	registers := regexp.MustCompile(`\b\w+\.(Handle|HandleFunc)\(`)

	found := map[string]bool{}
	scanned := 0
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		scanned++
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(b)
		if registers.MatchString(src) && strings.Contains(src, `"net/http"`) {
			rel, err := filepath.Rel(repoRoot, filepath.Dir(path))
			if err != nil {
				return err
			}
			found[rel] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", repoRoot, err)
	}
	// Positive control on the walk itself: a walk that saw nothing would make
	// every comparison below pass while measuring nothing at all.
	if scanned < 20 {
		t.Fatalf("the walk saw %d non-test .go files, which is too few to be "+
			"measuring this repository", scanned)
	}
	if len(found) == 0 {
		t.Fatalf("the scan found no route-registering package among %d files, "+
			"so this cell is asserting nothing", scanned)
	}

	// sourceDirs are relative to THIS package; the walk yields paths relative
	// to the repository root. Clean(Join(...)) expresses the first in the
	// second's terms -- filepath.Rel cannot, because it refuses to relate a
	// dot-dot prefix to a relative path (measured: the first version of this
	// test failed its own baseline on exactly that, which made two mutations
	// look like kills when they were my bug).
	listed := map[string]bool{}
	for _, d := range sourceDirs {
		listed[filepath.Clean(filepath.Join("internal", "apidocs", d))] = true
	}
	for dir := range found {
		if !listed[dir] {
			t.Errorf("%s registers routes and is NOT in sourceDirs: every route it "+
				"registers is invisible to this cell", dir)
		}
	}
	for dir := range listed {
		if !found[dir] {
			t.Errorf("sourceDirs lists %s, which registers no routes: a stale entry "+
				"tells a reader this cell watches something it does not", dir)
		}
	}
	t.Logf("scanned %d non-test .go files; %d packages register routes, %d listed",
		scanned, len(found), len(listed))
}
