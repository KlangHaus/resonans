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
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

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
var mountPrefixes = map[string]bool{"/api/": true, "/delivery/": true}

// resolvable maps the identifiers a route pattern may be built from to their
// values. Taken from the real constants, so renaming or re-pointing one moves
// this cell with it instead of leaving it asserting a stale literal.
var resolvable = map[string]string{
	"media.AssetPath": media.AssetPath,
	"bridge.Path":     bridge.Path,
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
