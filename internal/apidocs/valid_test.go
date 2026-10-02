package apidocs_test

import (
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// TestSpecIsValidOpenAPI validates docs/api/openapi.yaml as an OpenAPI 3 document.
//
// 🔴 WHY THIS EXISTS, and it is the drift cell's declared blind spot cashing in.
// internal/apidocs proves the spec lists the same ROUTES the router registers, in
// both directions. It says nothing about whether the document is VALID — and it
// was not. The README tells a reader to open the spec with an OpenAPI viewer, and
// the tool it names reported 20 errors and exited 1: every templated path
// (`{key}`, `{id}`, …) was declared in the path and never defined as a path
// parameter. So the documented way to read this API failed, and the guard that
// covers the spec was green the whole time, because route lists and parameter
// definitions are different questions.
//
// Measured before it was fixed, which is why this cell exists in this order:
// running it against the spec as it stood named
// `operation GET /_assets/{key} must define exactly all path parameters
// (missing: [key])`. Red before green, so the cell is known to fire rather than
// assumed to.
//
// A Go library rather than shelling out to the JS viewer the README names: that
// would be an unpinned network fetch at test time, so a validator outage or a new
// release would read as our spec breaking. This runs in `go test ./...`, offline,
// pinned by go.sum.
//
// What it does NOT measure, stated so the green does not overclaim: validity is
// not accuracy. A response schema can be valid OpenAPI and still describe a shape
// the handler never returns. That is the drift cell's half for routes, and nothing
// covers it for schemas yet.
func TestSpecIsValidOpenAPI(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "api", "openapi.yaml")

	loader := &openapi3.Loader{IsExternalRefsAllowed: false}
	doc, err := loader.LoadFromFile(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}

	// The denominator, so a pass cannot come from validating an empty document.
	// A loader that silently returned nothing would otherwise validate fine.
	if got := doc.Paths.Len(); got == 0 {
		t.Fatalf("%s parsed to ZERO paths — validating an empty document is not a pass", path)
	}

	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("%s is not a valid OpenAPI 3 document: %v\n\n"+
			"the README tells readers to open this file in an OpenAPI viewer, so an invalid "+
			"document breaks the only documented way to read this API", path, err)
	}
	t.Logf("%s is valid OpenAPI %s, %d paths", path, doc.OpenAPI, doc.Paths.Len())
}
