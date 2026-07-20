package editorauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }

func TestMiddlewarePassthroughWhenNoToken(t *testing.T) {
	h := Middleware("")(http.HandlerFunc(ok))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("passthrough status = %d, want 204", rec.Code)
	}
}

func TestMiddlewareGuards(t *testing.T) {
	h := Middleware("secret")(http.HandlerFunc(ok))

	cases := map[string]struct {
		header string
		want   int
	}{
		"missing":     {"", http.StatusUnauthorized},
		"wrong":       {"Bearer nope", http.StatusUnauthorized},
		"no-prefix":   {"secret", http.StatusUnauthorized},
		"correct":     {"Bearer secret", http.StatusNoContent},
		"with-spaces": {"Bearer  secret ", http.StatusNoContent},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
