package delivery

import "testing"

// A body rewritten without a version bump (the sanitize-backfill) must still
// change the ETag, or a revalidating cache keeps the old copy on 304. The ETag
// is over the bytes, so any rewrite of them moves it.
func TestBodyETag(t *testing.T) {
	body := []byte(`{"v":3}`)
	base := bodyETag(body, "da", "da")
	if base != bodyETag(body, "da", "da") {
		t.Fatal("ETag not stable for an unchanged body")
	}
	if len(base) < 4 || base[:3] != `W/"` {
		t.Fatalf("ETag %q is not weak", base)
	}
	for name, other := range map[string]string{
		"another body":           bodyETag(append(body, ' '), "da", "da"),
		"another locale":         bodyETag(body, "en", "da"),
		"another default locale": bodyETag(body, "da", "sv"),
		"values moved across":    bodyETag(body, "dad", "a"),
	} {
		if other == base {
			t.Errorf("%s: same ETag %s", name, base)
		}
	}
}
