package delivery

import (
	"testing"
	"time"

	"github.com/KlangHaus/resonans/internal/content"
)

// A body rewritten without a version bump (the sanitize-backfill) must still
// change the ETag, or a revalidating cache keeps the old copy on 304.
func TestETagChangesWithUpdatedAtAtSameVersion(t *testing.T) {
	t0 := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	before := content.Document{Version: 3, UpdatedAt: t0}
	after := content.Document{Version: 3, UpdatedAt: t0.Add(time.Second)}
	if etagFor("da", before) == etagFor("da", after) {
		t.Fatalf("ETag unchanged across a write at the same version: %s", etagFor("da", before))
	}
	if etagFor("da", before) != etagFor("da", before) {
		t.Fatal("ETag not stable for an unchanged document")
	}
	if etagFor("da", before) == etagFor("en", before) {
		t.Fatal("ETag must differ per locale")
	}
}
