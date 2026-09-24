package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KlangHaus/resonans/internal/config"
)

func TestMigrateRequiresDatabaseURL(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("DATABASE_URL", "")
	if err := Migrate(context.Background()); err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}

// The probe must report which build is running, not just that something is.
// Against the previous constant `{"status":"ok"}` body this fails — that is the
// point: a liveness answer that cannot vary cannot identify anything.
func TestHealthPayloadReportsStampedIdentity(t *testing.T) {
	oldVersion, oldRevision := version, revision
	t.Cleanup(func() { version, revision = oldVersion, oldRevision })
	version, revision = "9.9.9", "0123456789abcdef0123456789abcdef01234567"

	body, err := healthPayload("https://example.org/fork")
	if err != nil {
		t.Fatalf("healthPayload: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal %s: %v", body, err)
	}
	for field, want := range map[string]string{
		"status":   "ok",
		"service":  "resonans",
		"version":  "9.9.9",
		"revision": "0123456789abcdef0123456789abcdef01234567",
		// AGPL-3.0 §13: the source offer must be the one THIS build was
		// configured with. A hardcoded upstream URL would make every modified
		// deployment point network users at code it is not running, which is
		// worse than no offer: it looks like compliance.
		"source": "https://example.org/fork",
	} {
		if got[field] != want {
			t.Errorf("%s = %q, want %q", field, got[field], want)
		}
	}
}

// The default exists so an unmodified deployment needs no configuration at all,
// and it must be a real URL rather than a placeholder — an offer nobody can
// follow is not an offer.
func TestSourceOfferDefaultsToUpstreamAndIsOverridable(t *testing.T) {
	t.Setenv("RESONANS_DEV_AUTH", "1")
	t.Setenv("RESONANS_SOURCE_URL", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	const upstream = "https://github.com/KlangHaus/resonans"
	if cfg.SourceURL != upstream {
		t.Fatalf("SourceURL = %q, want the upstream default %q", cfg.SourceURL, upstream)
	}

	t.Setenv("RESONANS_SOURCE_URL", "https://git.example.org/our-fork")
	cfg, err = config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.SourceURL != "https://git.example.org/our-fork" {
		t.Fatalf("a fork could not override the source offer: %q", cfg.SourceURL)
	}
}
