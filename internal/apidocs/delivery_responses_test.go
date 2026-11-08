package apidocs

import (
	"bytes"
	"context"
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
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					continue
				}
				c, _ := strconv.Atoi(lit.Value)
				out[name.Name] = c
			}
		}
	}
	// Control: the declarations were read, not an empty file.
	if out["StatusOK"] != http.StatusOK || out["StatusTeapot"] != http.StatusTeapot || len(out) < 60 {
		t.Fatalf("read %d status constants from %s, and StatusOK/StatusTeapot did not match net/http", len(out), path)
	}
	return out
}

// deliveryHandlerStatuses returns, per registered delivery route, the status
// codes its handler can write: every http.Status constant in the handler and in
// the package functions it calls, transitively. It over-counts a status in a
// branch the route never takes, which errs on the side of documenting.
func deliveryHandlerStatuses(t *testing.T) map[string]map[int]bool {
	t.Helper()
	codes := statusCodes(t)
	fset := token.NewFileSet()
	files, err := filepath.Glob(filepath.Join("..", "delivery", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no delivery sources found (%v)", err)
	}
	funcs := map[string]*ast.FuncDecl{}
	routes := map[string]string{} // pattern -> handler name
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				funcs[fd.Name.Name] = fd
			}
		}
		ast.Inspect(af, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "HandleFunc" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			h, ok2 := call.Args[1].(*ast.SelectorExpr)
			if !ok || !ok2 {
				t.Fatalf("%s: a HandleFunc this cell cannot read; it would be a route without a check", fset.Position(call.Pos()))
			}
			pattern, _ := strconv.Unquote(lit.Value)
			routes[pattern] = h.Sel.Name
			return true
		})
	}
	reach := func(handler string) map[int]bool {
		out := map[int]bool{}
		seen := map[string]bool{}
		var walk func(string)
		walk = func(name string) {
			if seen[name] {
				return
			}
			seen[name] = true
			fd := funcs[name]
			if fd == nil || fd.Body == nil {
				return
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.SelectorExpr:
					if id, ok := v.X.(*ast.Ident); ok && id.Name == "http" && strings.HasPrefix(v.Sel.Name, "Status") {
						c, known := codes[v.Sel.Name]
						if !known {
							t.Fatalf("%s: http.%s is not a status this cell knows", fset.Position(v.Pos()), v.Sel.Name)
						}
						out[c] = true
						return true
					}
					walk(v.Sel.Name)
				case *ast.Ident:
					walk(v.Name)
				}
				return true
			})
		}
		walk(handler)
		return out
	}
	out := map[string]map[int]bool{}
	for pattern, handler := range routes {
		s := reach(handler)
		// The service-token middleware wraps every route: 401.
		s[http.StatusUnauthorized] = true
		out[pattern] = s
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
		method, path, _ := strings.Cut(pattern, " ")
		t.Run(pattern, func(t *testing.T) {
			item := doc.Paths.Find(path)
			if item == nil {
				t.Fatalf("%s is not in the spec", path)
			}
			op := item.GetOperation(method)
			if op == nil {
				t.Fatalf("%s %s is not in the spec", method, path)
			}
			var codes []int
			for c := range statuses[pattern] {
				codes = append(codes, c)
			}
			sort.Ints(codes)
			for _, c := range codes {
				if op.Responses.Status(c) == nil {
					t.Errorf("the handler can answer %d, and the spec does not document it", c)
				}
			}
			ok := op.Responses.Status(http.StatusOK)
			if ok == nil || ok.Value == nil || ok.Value.Content.Get("application/json") == nil ||
				ok.Value.Content.Get("application/json").Schema == nil {
				t.Errorf("200 has no application/json schema")
			}
		})
	}
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

func TestDeliveryResponsesMatchTheSpec(t *testing.T) {
	doc := loadedSpec(t)
	reg, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	tenant := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	store := content.NewMemoryStore()
	ctx := context.Background()
	d, err := store.Create(ctx, tenant, content.CreateInput{Type: "course", Slug: "intro", Title: "Intro"})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := store.Publish(ctx, tenant, d.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err := store.Create(ctx, tenant, content.CreateInput{Type: "course", Slug: "draft", Title: "Draft"}); err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	secret := []byte("preview-secret-not-a-secret-0123456789")
	h := delivery.NewRouter(delivery.Deps{Store: store, Registry: reg, ServiceToken: responsesToken, PreviewSecret: string(secret)})

	type call struct {
		name, routePath, url string
		token, scope         bool
		header               map[string]string
		want                 int
	}
	good := preview.Sign(secret, preview.Claims{Tenant: tenant.String(), Type: "course", Slug: "draft", Exp: time.Now().Add(time.Minute).Unix()})
	other := preview.Sign(secret, preview.Claims{Tenant: tenant.String(), Type: "course", Slug: "intro", Exp: time.Now().Add(time.Minute).Unix()})
	calls := []call{
		{"schema", "/delivery/v1/schema", "/delivery/v1/schema", true, true, nil, 200},
		{"catalog", "/delivery/v1/{type}", "/delivery/v1/course", true, true, nil, 200},
		{"one document", "/delivery/v1/{type}/{slug}", "/delivery/v1/course/intro", true, true, nil, 200},
		{"no service token", "/delivery/v1/{type}/{slug}", "/delivery/v1/course/intro", false, true, nil, 401},
		{"no tenant", "/delivery/v1/{type}", "/delivery/v1/course", true, false, nil, 403},
		{"no tenant, one document", "/delivery/v1/{type}/{slug}", "/delivery/v1/course/intro", true, false, nil, 403},
		{"a draft is not delivered", "/delivery/v1/{type}/{slug}", "/delivery/v1/course/draft", true, true, nil, 404},
		{"preview", "/delivery/v1/preview/{type}/{slug}", "/delivery/v1/preview/course/draft?token=" + good, true, true, nil, 200},
		{"preview, bad token", "/delivery/v1/preview/{type}/{slug}", "/delivery/v1/preview/course/draft?token=x.y", true, true, nil, 401},
		{"preview, token for another document", "/delivery/v1/preview/{type}/{slug}", "/delivery/v1/preview/course/draft?token=" + other, true, true, nil, 403},
	}
	do := func(c call) (*http.Request, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(http.MethodGet, c.url, nil)
		if c.token {
			req.Header.Set("Authorization", "Bearer "+responsesToken)
		}
		if c.scope {
			req.Header.Set("X-Scope-OrgID", tenant.String())
		}
		for k, v := range c.header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return req, rec
	}
	var etag string
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			req, rec := do(c)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, c.want, bytes.TrimSpace(rec.Body.Bytes()))
			}
			if err := checkAgainstSpec(t, doc, c.routePath, req, rec); err != nil {
				t.Errorf("the response does not match the spec: %v\nbody: %s", err, rec.Body.String())
			}
			if c.name == "one document" {
				etag = rec.Header().Get("ETag")
			}
		})
	}
	t.Run("not modified", func(t *testing.T) {
		if etag == "" {
			t.Fatal("no ETag from the document request, so the 304 cannot be requested")
		}
		c := call{"not modified", "/delivery/v1/{type}/{slug}", "/delivery/v1/course/intro", true, true, map[string]string{"If-None-Match": etag}, 304}
		req, rec := do(c)
		if rec.Code != http.StatusNotModified {
			t.Fatalf("status = %d, want 304", rec.Code)
		}
		if err := checkAgainstSpec(t, doc, c.routePath, req, rec); err != nil {
			t.Errorf("the response does not match the spec: %v", err)
		}
	})

	// The validator has to be able to fail, or the cells above prove nothing: a
	// document response with a field the schema does not list must be refused.
	t.Run("CONTROL: an envelope with an extra field is refused", func(t *testing.T) {
		req, rec := do(calls[2])
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
		if err := checkAgainstSpec(t, doc, "/delivery/v1/{type}/{slug}", req, bad); err == nil {
			t.Error("the validator accepted a field the Envelope schema does not have")
		}
	})
}
