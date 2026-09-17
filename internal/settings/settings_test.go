package settings

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestValidatePreviewURL(t *testing.T) {
	ok := map[string]string{
		"https://etude.example/preview/{type}/{slug}?token={token}&locale={locale}": "https://etude.example",
		"https://etude.example:8443/p?t={token}":                                    "https://etude.example:8443",
		"http://localhost:3000/preview?token={token}":                               "http://localhost:3000",
		"http://127.0.0.1:4000/{slug}?token={token}":                                "http://127.0.0.1:4000",
	}
	for in, want := range ok {
		got, err := ValidatePreviewURL(in)
		if err != nil || got != want {
			t.Errorf("ValidatePreviewURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"",
		"https://etude.example/preview/{slug}",          // no {token}
		"http://etude.example/p?token={token}",          // plain http off localhost
		"javascript:alert(1)//{token}",                  // not http(s)
		"/preview?token={token}",                        // relative: no fixed origin
		"https://{slug}.example/p?token={token}",        // placeholder in host
		"https://user:pw@etude.example/p?token={token}", // credentials
		"https://etude.example/p?token={token}#frag",    // fragment
		"ftp://etude.example/{token}",                   // scheme
		"http://localhost.evil.example/p?token={token}", // localhost lookalike
	}
	for _, in := range bad {
		if got, err := ValidatePreviewURL(in); err == nil {
			t.Errorf("ValidatePreviewURL(%q) = %q, want error", in, got)
		}
	}
}

func TestMemoryStorePreviewURLIsTenantScoped(t *testing.T) {
	s := NewMemoryStore()
	a, b := uuid.New(), uuid.New()
	ctx := context.Background()
	if err := s.SetPreviewURL(ctx, a, "https://a.example/p?token={token}"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.PreviewURL(ctx, a); got != "https://a.example/p?token={token}" {
		t.Errorf("tenant a = %q", got)
	}
	if got, _ := s.PreviewURL(ctx, b); got != "" {
		t.Errorf("tenant b leaked %q", got)
	}
}
