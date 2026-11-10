package apidocs

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/delivery"
	"github.com/KlangHaus/resonans/internal/preview"
	"github.com/KlangHaus/resonans/internal/schema"
)

// The delivery surface is what other teams build against, so its responses are
// documented with schemas, and three cells keep the documentation true:
//
//   - every status a delivery handler can write is in the spec, read from the
//     handlers' SOURCE so a new `writeJSON(w, http.StatusX, …)` cannot ship
//     undocumented;
//   - the Envelope schema lists exactly the fields of delivery.Envelope;
//   - real requests through the real router produce responses the spec accepts.

func loadedSpec(t *testing.T) *openapi3.T {
	t.Helper()
	loader := &openapi3.Loader{IsExternalRefsAllowed: false}
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		t.Fatalf("load %s: %v", specPath, err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("%s: %v", specPath, err)
	}
	return doc
}

// statusCodes maps net/http's constant names to their codes, read from the
// standard library's own declarations so no second table can drift from it.
// (A name derived from http.StatusText would not do: StatusTeapot's text is
// "I'm a teapot", StatusProxyAuthRequired's is "Proxy Authentication Required".)
func statusCodes(t *testing.T) map[string]int {
	t.Helper()
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	path := filepath.Join(strings.TrimSpace(string(goroot)), "src", "net", "http", "status.go")
	af, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := map[string]int{}
	for _, d := range af.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.CONST {
			addIntConsts(gd, out)
		}
	}
	// Control: the declarations were read, not an empty file.
	if out["StatusOK"] != http.StatusOK || out["StatusTeapot"] != http.StatusTeapot || len(out) < 60 {
		t.Fatalf("read %d status constants from %s, and StatusOK/StatusTeapot did not match net/http", len(out), path)
	}
	return out
}

// addIntConsts records every `Name = <integer literal>` in a const block.
func addIntConsts(gd *ast.GenDecl, out map[string]int) {
	for _, spec := range gd.Specs {
		vs := spec.(*ast.ValueSpec)
		for i, name := range vs.Names {
			if i >= len(vs.Values) {
				continue
			}
			if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.INT {
				c, _ := strconv.Atoi(lit.Value)
				out[name.Name] = c
			}
		}
	}
}

// deliverySource is the delivery package as parsed source: its functions by
// name and its routes, pattern to handler name.
type deliverySource struct {
	fset   *token.FileSet
	funcs  map[string]*ast.FuncDecl
	routes map[string]string
}

func parseDelivery(t *testing.T) deliverySource {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "delivery", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no delivery sources found (%v)", err)
	}
	src := deliverySource{fset: token.NewFileSet(), funcs: map[string]*ast.FuncDecl{}, routes: map[string]string{}}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(src.fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				src.funcs[fd.Name.Name] = fd
			}
		}
		ast.Inspect(af, func(n ast.Node) bool {
			src.addRoute(t, n)
			return true
		})
	}
	return src
}

// addRoute records n when it is a `mux.HandleFunc("<pattern>", h.<handler>)`.
func (src deliverySource) addRoute(t *testing.T, n ast.Node) {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "HandleFunc" {
		return
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	h, ok2 := call.Args[1].(*ast.SelectorExpr)
	if !ok || !ok2 {
		t.Fatalf("%s: a HandleFunc this cell cannot read; it would be a route without a check", src.fset.Position(call.Pos()))
	}
	pattern, _ := strconv.Unquote(lit.Value)
	src.routes[pattern] = h.Sel.Name
}

// statusWalker collects the http.Status constants reachable from a function,
// following calls to other functions of the same package.
type statusWalker struct {
	t     *testing.T
	src   deliverySource
	codes map[string]int
	seen  map[string]bool
	out   map[int]bool
}

func (w *statusWalker) walk(name string) {
	if w.seen[name] {
		return
	}
	w.seen[name] = true
	if fd := w.src.funcs[name]; fd != nil && fd.Body != nil {
		ast.Inspect(fd.Body, w.visit)
	}
}

func (w *statusWalker) visit(n ast.Node) bool {
	switch v := n.(type) {
	case *ast.SelectorExpr:
		if id, ok := v.X.(*ast.Ident); ok && id.Name == "http" && strings.HasPrefix(v.Sel.Name, "Status") {
			w.record(v)
			return true
		}
		w.walk(v.Sel.Name)
	case *ast.Ident:
		w.walk(v.Name)
	}
	return true
}

func (w *statusWalker) record(v *ast.SelectorExpr) {
	c, known := w.codes[v.Sel.Name]
	if !known {
		w.t.Fatalf("%s: http.%s is not a status this cell knows", w.src.fset.Position(v.Pos()), v.Sel.Name)
	}
	w.out[c] = true
}

// deliveryHandlerStatuses returns, per registered delivery route, the status
// codes its handler can write: every http.Status constant in the handler and in
// the package functions it calls, transitively. It over-counts a status in a
// branch the route never takes, which errs on the side of documenting.
func deliveryHandlerStatuses(t *testing.T) map[string]map[int]bool {
	t.Helper()
	codes := statusCodes(t)
	src := parseDelivery(t)
	out := map[string]map[int]bool{}
	for pattern, handler := range src.routes {
		w := &statusWalker{t: t, src: src, codes: codes, seen: map[string]bool{}, out: map[int]bool{}}
		w.walk(handler)
		// The service-token middleware wraps every route: 401.
		w.out[http.StatusUnauthorized] = true
		out[pattern] = w.out
	}
	return out
}

func TestEveryDeliveryStatusIsDocumented(t *testing.T) {
	doc := loadedSpec(t)
	statuses := deliveryHandlerStatuses(t)
	if len(statuses) < 4 {
		t.Fatalf("found %d delivery routes, expected at least 4 — the enumerator failed", len(statuses))
	}
	// Positive control: the walker must see a status written by a HELPER, not
	// only in the handler itself. listPublished writes 403 only through h.tenant.
	if !statuses["GET /delivery/v1/{type}"][http.StatusForbidden] {
		t.Fatal("the walker did not find 403 behind h.tenant, so it does not see into helpers and every result below is incomplete")
	}
	patterns := make([]string, 0, len(statuses))
	for p := range statuses {
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)
	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) { checkDocumented(t, doc, pattern, statuses[pattern]) })
	}
}

// checkDocumented fails for each status in codes that the spec's operation for
// pattern does not document, and when its 200 has no JSON schema.
func checkDocumented(t *testing.T, doc *openapi3.T, pattern string, codes map[int]bool) {
	t.Helper()
	method, path, _ := strings.Cut(pattern, " ")
	item := doc.Paths.Find(path)
	if item == nil {
		t.Fatalf("%s is not in the spec", path)
	}
	op := item.GetOperation(method)
	if op == nil {
		t.Fatalf("%s %s is not in the spec", method, path)
	}
	sorted := make([]int, 0, len(codes))
	for c := range codes {
		sorted = append(sorted, c)
	}
	sort.Ints(sorted)
	for _, c := range sorted {
		if op.Responses.Status(c) == nil {
			t.Errorf("the handler can answer %d, and the spec does not document it", c)
		}
	}
	if !hasJSONSchema(op.Responses.Status(http.StatusOK)) {
		t.Errorf("200 has no application/json schema")
	}
}

func hasJSONSchema(r *openapi3.ResponseRef) bool {
	if r == nil || r.Value == nil {
		return false
	}
	mt := r.Value.Content.Get("application/json")
	return mt != nil && mt.Schema != nil
}

func TestEnvelopeSchemaMatchesTheType(t *testing.T) {
	doc := loadedSpec(t)
	ref := doc.Components.Schemas["Envelope"]
	if ref == nil || ref.Value == nil {
		t.Fatal("components.schemas.Envelope is missing")
	}
	var fields, required []string
	typ := reflect.TypeOf(delivery.Envelope{})
	for i := 0; i < typ.NumField(); i++ {
		name, opts, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		fields = append(fields, name)
		if !strings.Contains(opts, "omitempty") {
			required = append(required, name)
		}
	}
	var props []string
	for p := range ref.Value.Properties {
		props = append(props, p)
	}
	spRequired := append([]string(nil), ref.Value.Required...)
	sort.Strings(fields)
	sort.Strings(required)
	sort.Strings(props)
	sort.Strings(spRequired)
	if !reflect.DeepEqual(fields, props) {
		t.Errorf("Envelope properties = %v, delivery.Envelope fields = %v", props, fields)
	}
	if !reflect.DeepEqual(required, spRequired) {
		t.Errorf("Envelope required = %v, delivery.Envelope fields without omitempty = %v", spRequired, required)
	}
}

const responsesToken = "responses-service-token-not-a-secret"

// checkAgainstSpec validates one recorded response against the operation the
// spec gives for routePath. An undocumented status is an error too.
func checkAgainstSpec(t *testing.T, doc *openapi3.T, routePath string, req *http.Request, rec *httptest.ResponseRecorder) error {
	t.Helper()
	item := doc.Paths.Find(routePath)
	if item == nil {
		t.Fatalf("%s is not in the spec", routePath)
	}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: req,
			Route:   &routers.Route{Spec: doc, Path: routePath, PathItem: item, Method: req.Method, Operation: item.GetOperation(req.Method)},
		},
		Status:  rec.Code,
		Header:  rec.Header(),
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	in.SetBodyBytes(rec.Body.Bytes())
	return openapi3filter.ValidateResponse(context.Background(), in)
}

var responsesTenant = uuid.MustParse("11111111-1111-1111-1111-111111111111")

// responsesRouter is the real delivery router over a memory store holding one
// published and one draft course for responsesTenant, with preview enabled.
func responsesRouter(t *testing.T, secret []byte) http.Handler {
	t.Helper()
	reg, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	store := content.NewMemoryStore()
	ctx := context.Background()
	d, err := store.Create(ctx, responsesTenant, content.CreateInput{Type: "course", Slug: "intro", Title: "Intro"})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := store.Publish(ctx, responsesTenant, d.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err := store.Create(ctx, responsesTenant, content.CreateInput{Type: "course", Slug: "draft", Title: "Draft"}); err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	// A second published course, so a one-document page has a next page.
	more, err := store.Create(ctx, responsesTenant, content.CreateInput{Type: "course", Slug: "more", Title: "More"})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := store.Publish(ctx, responsesTenant, more.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return delivery.NewRouter(delivery.Deps{Store: store, Registry: reg, ServiceToken: responsesToken, PreviewSecret: string(secret)})
}

type deliveryCall struct {
	name, routePath, url string
	token, scope         bool
	header               map[string]string
	want                 int
}

// send makes c's request against h and returns the request and its response.
func send(h http.Handler, c deliveryCall) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, c.url, nil)
	if c.token {
		req.Header.Set("Authorization", "Bearer "+responsesToken)
	}
	if c.scope {
		req.Header.Set("X-Scope-OrgID", responsesTenant.String())
	}
	for k, v := range c.header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return req, rec
}

// expectDocumented sends c and fails unless it gets c.want and a response the
// spec accepts. It returns the response for callers that need its headers.
func expectDocumented(t *testing.T, doc *openapi3.T, h http.Handler, c deliveryCall) *httptest.ResponseRecorder {
	t.Helper()
	req, rec := send(h, c)
	if rec.Code != c.want {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, c.want, bytes.TrimSpace(rec.Body.Bytes()))
	}
	if err := checkAgainstSpec(t, doc, c.routePath, req, rec); err != nil {
		t.Errorf("the response does not match the spec: %v\nbody: %s", err, rec.Body.String())
	}
	return rec
}

const (
	documentRoute = "/delivery/v1/{type}/{slug}"
	documentURL   = "/delivery/v1/course/intro"
)

func TestDeliveryResponsesMatchTheSpec(t *testing.T) {
	doc := loadedSpec(t)
	secret := []byte("preview-secret-not-a-secret-0123456789")
	h := responsesRouter(t, secret)
	exp := time.Now().Add(time.Minute).Unix()
	good := preview.Sign(secret, preview.Claims{Tenant: responsesTenant.String(), Type: "course", Slug: "draft", Exp: exp})
	other := preview.Sign(secret, preview.Claims{Tenant: responsesTenant.String(), Type: "course", Slug: "intro", Exp: exp})
	calls := []deliveryCall{
		{"schema", "/delivery/v1/schema", "/delivery/v1/schema", true, true, nil, 200},
		{"catalog", "/delivery/v1/{type}", "/delivery/v1/course", true, true, nil, 200},
		{"catalog, a page with a next page", "/delivery/v1/{type}", "/delivery/v1/course?limit=1", true, true, nil, 200},
		{"catalog, a limit out of range", "/delivery/v1/{type}", "/delivery/v1/course?limit=0", true, true, nil, 400},
		{"one document", documentRoute, documentURL, true, true, nil, 200},
		{"no service token", documentRoute, documentURL, false, true, nil, 401},
		{"no tenant", "/delivery/v1/{type}", "/delivery/v1/course", true, false, nil, 403},
		{"no tenant, one document", documentRoute, documentURL, true, false, nil, 403},
		{"a draft is not delivered", documentRoute, "/delivery/v1/course/draft", true, true, nil, 404},
		{"preview", "/delivery/v1/preview/{type}/{slug}", "/delivery/v1/preview/course/draft?token=" + good, true, true, nil, 200},
		{"preview, bad token", "/delivery/v1/preview/{type}/{slug}", "/delivery/v1/preview/course/draft?token=x.y", true, true, nil, 401},
		{"preview, token for another document", "/delivery/v1/preview/{type}/{slug}", "/delivery/v1/preview/course/draft?token=" + other, true, true, nil, 403},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) { expectDocumented(t, doc, h, c) })
	}
	t.Run("catalog, the next page", func(t *testing.T) {
		_, rec := send(h, calls[2])
		var page struct {
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || page.NextCursor == "" {
			t.Fatalf("no nextCursor on a one-document page of two (%v): %s", err, rec.Body.String())
		}
		expectDocumented(t, doc, h, deliveryCall{"next page", "/delivery/v1/{type}", "/delivery/v1/course?limit=1&cursor=" + page.NextCursor, true, true, nil, 200})
	})
	t.Run("not modified", func(t *testing.T) {
		_, rec := send(h, calls[4])
		etag := rec.Header().Get("ETag")
		if etag == "" {
			t.Fatal("no ETag from the document request, so the 304 cannot be requested")
		}
		expectDocumented(t, doc, h, deliveryCall{"not modified", documentRoute, documentURL, true, true, map[string]string{"If-None-Match": etag}, 304})
	})
	t.Run("catalog, not modified", func(t *testing.T) {
		_, rec := send(h, calls[1])
		etag := rec.Header().Get("ETag")
		if etag == "" {
			t.Fatal("no ETag from the catalog request, so the 304 cannot be requested")
		}
		expectDocumented(t, doc, h, deliveryCall{"catalog, not modified", "/delivery/v1/{type}", "/delivery/v1/course", true, true, map[string]string{"If-None-Match": etag}, 304})
	})
	t.Run("CONTROL: an envelope with an extra field is refused", func(t *testing.T) {
		validatorRefusesAnExtraField(t, doc, h, calls[4])
	})
}

// validatorRefusesAnExtraField shows the validator can fail, or the cells above
// prove nothing: a document response with a field the schema does not list must
// be refused.
func validatorRefusesAnExtraField(t *testing.T, doc *openapi3.T, h http.Handler, c deliveryCall) {
	t.Helper()
	req, rec := send(h, c)
	body := bytes.Replace(rec.Body.Bytes(), []byte(`{"id"`), []byte(`{"notInTheSchema":1,"id"`), 1)
	if bytes.Equal(body, rec.Body.Bytes()) {
		t.Fatal("the mutation changed nothing")
	}
	bad := httptest.NewRecorder()
	for k, v := range rec.Header() {
		bad.Header()[k] = v
	}
	bad.WriteHeader(rec.Code)
	_, _ = bad.Write(body)
	if err := checkAgainstSpec(t, doc, c.routePath, req, bad); err == nil {
		t.Error("the validator accepted a field the Envelope schema does not have")
	}
}
