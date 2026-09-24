package apidocs

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/delivery"
	"github.com/KlangHaus/resonans/internal/schema"
)

// The delivery surface is trusted server-to-server: the caller authenticates with
// a service token and the tenant comes from X-Scope-OrgID, trusted ONLY because
// the token matched. Until today that surface was reachable only from inside a
// compose network. Infra's production wiring publishes it on the internet, with a
// named reason and a named expiry — so "is the token required on EVERY route"
// stopped being a design detail and became the thing standing between the open
// internet and a tenant header nobody verified.
//
// It IS required today, structurally: NewRouter wraps the whole mux in the
// middleware, so a route added to that mux inherits the guard. But that is a
// property you confirm by READING, and the existing test exercises exactly ONE of
// the four routes. Move a route out of that mux — into app.go, say — and the
// existing test still passes while the route answers without a token.
//
// So this cell enumerates the delivery routes from the SOURCE of every package
// that registers one, not from the router it is testing. A route registered
// somewhere else is therefore in the denominator.
const probeToken = "probe-service-token-not-a-secret"

// concrete fills a route pattern's wildcards so it can actually be requested.
// Unknown wildcards are a FAILURE rather than a guess: a pattern this cannot
// instantiate is a route this cell would silently skip, and a skipped route is
// exactly the one that would be unguarded.
func concrete(t *testing.T, pattern string) string {
	t.Helper()
	repl := map[string]string{"{type}": "page", "{slug}": "some-slug", "{version}": "1", "{id}": "00000000-0000-0000-0000-000000000000"}
	out := pattern
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	if strings.ContainsAny(out, "{}") {
		t.Fatalf("cannot instantiate %q — it still holds a wildcard this cell does not know, and skipping it would hide the one route that matters", pattern)
	}
	return out
}

func TestEveryDeliveryRouteRequiresTheServiceToken(t *testing.T) {
	reg, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	h := delivery.NewRouter(delivery.Deps{
		Store:        content.NewMemoryStore(),
		Registry:     reg,
		ServiceToken: probeToken,
	})

	var paths []string
	for _, r := range routesFromSource(t) {
		if strings.HasPrefix(r.path, "/delivery/") {
			paths = append(paths, r.path)
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatal("found no /delivery/ routes at all — the enumerator, not the guard, is what failed")
	}

	req := func(path, token string) int {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		// A tenant scope on purpose: without it a refusal could be the SCOPE
		// check rather than the token check, and the two answer differently (403
		// against 401). The cell has to fail for the reason it names.
		r.Header.Set("X-Scope-OrgID", "11111111-1111-1111-1111-111111111111")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	for _, p := range paths {
		url := concrete(t, p)
		t.Run(p, func(t *testing.T) {
			if got := req(url, ""); got != http.StatusUnauthorized {
				t.Errorf("no token: status = %d, want 401 — this route answers without the service token", got)
			}
			if got := req(url, "wrong-"+probeToken); got != http.StatusUnauthorized {
				t.Errorf("wrong token: status = %d, want 401", got)
			}
			// Positive control, per route. Without it a router that refused
			// everything — a broken mount, a panic recovered into 401 — would
			// satisfy both assertions above and read as a guarded surface.
			if got := req(url, probeToken); got == http.StatusUnauthorized {
				t.Errorf("correct token: status = 401, so this route refuses a VALID token and the two assertions above prove nothing")
			}
		})
	}
	t.Logf("%d delivery routes, each refused without the token and reached with it", len(paths))
}
