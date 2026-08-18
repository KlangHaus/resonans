package app

import (
	"context"
	"encoding/json"
	"testing"
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

	body, err := healthPayload()
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
	} {
		if got[field] != want {
			t.Errorf("%s = %q, want %q", field, got[field], want)
		}
	}
}
