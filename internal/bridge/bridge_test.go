package bridge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesScript(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}
	if !strings.Contains(rec.Body.String(), "global.ResonansBridge") {
		t.Error("body is not the bridge script")
	}
}

func TestHandlerRejectsWrites(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, Path, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

// The security model rests on two lines; a refactor that drops either must fail.
func TestScriptKeepsOriginChecks(t *testing.T) {
	s := string(script)
	for _, must := range []string{
		"e.source !== global.parent || e.origin !== studioOrigin",
		"global.parent.postMessage(msg, studioOrigin)",
	} {
		if strings.Count(s, must) != 1 {
			t.Errorf("script must contain exactly one %q", must)
		}
	}
	// Exactly one postMessage call site, so no second one can post to '*'.
	if n := strings.Count(s, ".postMessage("); n != 1 {
		t.Errorf("postMessage call sites = %d, want 1", n)
	}
}
