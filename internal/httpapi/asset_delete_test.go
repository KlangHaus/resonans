package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/settings"
)

// pdfFixture is the same trivial payload the other asset tests use: the upload
// path types a file from its CONTENT, so these bytes have to actually sniff as a
// permitted type.
func pdfFixture() []byte { return []byte("%PDF-1.4 hello") }

// testRouterWithBackend is testRouter, but it hands back the storage backend so a
// test can assert what happened to the BYTES rather than only to the metadata.
func testRouterWithBackend(t *testing.T) (http.Handler, *media.MemoryBackend) {
	t.Helper()
	backend := media.NewMemoryBackend()
	svc := media.NewService(media.NewMemoryStore(), backend, media.NewURLBuilder("https://cdn.test"))
	h := NewRouter(Deps{
		Store:    content.NewMemoryStore(),
		Registry: mustRegistry(t),
		Media:    svc,
		Video:    media.NewVimeoProvider(),
		Settings: settings.NewMemoryStore(),
	})
	return h, backend
}

// storageKeyOf reads the key the service assigned, so the byte assertion looks up
// the object the server actually wrote rather than a key the test invented.
func storageKeyOf(t *testing.T, h http.Handler, tenant, id string) string {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/api/v1/assets/"+id, tenant, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get asset: status = %d", rec.Code)
	}
	var a struct {
		StorageKey string `json:"storageKey"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if a.StorageKey == "" {
		t.Fatal("the response carries no storageKey, so the byte assertion has nothing to look up")
	}
	return a.StorageKey
}

// The asset bin. KH-1158 asked for a delete surface for "documents and assets";
// PR #67 closed only the first half, so an editor who uploaded the wrong file
// could not remove it at all. These cells pin the half that was missing, and the
// property that makes it an undo rather than a delete with extra steps: the
// stored BYTES survive.

func newAsset(t *testing.T, h http.Handler, tenant, filename string) string {
	t.Helper()
	rec := uploadAsset(t, h, tenant, filename, "application/pdf", pdfFixture())
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload %s: status = %d body=%s", filename, rec.Code, rec.Body)
	}
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func assetFilenames(t *testing.T, h http.Handler, tenant, query string) []string {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/api/v1/assets"+query, tenant, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list%s: status = %d", query, rec.Code)
	}
	var body struct {
		Assets []struct {
			Filename  string  `json:"filename"`
			DeletedAt *string `json:"deletedAt"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(body.Assets))
	for _, a := range body.Assets {
		out = append(out, a.Filename)
	}
	return out
}

func TestAssetSoftDeleteHidesItFromEveryReadPath(t *testing.T) {
	h := testRouter(t)
	keep := newAsset(t, h, tenantA, "behold.pdf")
	gone := newAsset(t, h, tenantA, "forkert.pdf")
	_ = keep

	// Both visible first: without this the assertions below would also pass on a
	// listing that returns nothing at all.
	if got := assetFilenames(t, h, tenantA, ""); len(got) != 2 {
		t.Fatalf("before delete the listing holds %v, want both files", got)
	}

	if rec := do(t, h, http.MethodDelete, "/api/v1/assets/"+gone, tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete: status = %d body=%s", rec.Code, rec.Body)
	}

	if got := assetFilenames(t, h, tenantA, ""); len(got) != 1 || got[0] != "behold.pdf" {
		t.Fatalf("live listing holds %v, want only behold.pdf", got)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/assets/"+gone, tenantA, nil); rec.Code != http.StatusConflict {
		t.Fatalf("get deleted asset: status = %d, want 409 — a 404 would send the editor looking for something one click from returning", rec.Code)
	}

	// The bin is reachable on purpose, and only when asked for.
	if got := assetFilenames(t, h, tenantA, "?deleted=only"); len(got) != 1 || got[0] != "forkert.pdf" {
		t.Fatalf("bin holds %v, want only forkert.pdf", got)
	}
	if got := assetFilenames(t, h, tenantA, "?deleted=include"); len(got) != 2 {
		t.Fatalf("include holds %v, want both", got)
	}
	// A typo must not open the bin.
	if got := assetFilenames(t, h, tenantA, "?deleted=yes"); len(got) != 1 {
		t.Fatalf("an unknown ?deleted value returned %v — it must fall back to live only", got)
	}

	// Deleting twice is refused rather than silently rewriting the timestamp.
	if rec := do(t, h, http.MethodDelete, "/api/v1/assets/"+gone, tenantA, nil); rec.Code != http.StatusConflict {
		t.Fatalf("second delete: status = %d, want 409", rec.Code)
	}
}

func TestAssetRestoreBringsItBackAndTheBytesWereNeverGone(t *testing.T) {
	h, backend := testRouterWithBackend(t)
	id := newAsset(t, h, tenantA, "fortrudt.pdf")

	key := storageKeyOf(t, h, tenantA, id)
	if _, ok := backend.Get(key); !ok {
		t.Fatalf("the fixture was never stored under %q, so this test cannot say anything about deletion", key)
	}

	if rec := do(t, h, http.MethodDelete, "/api/v1/assets/"+id, tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete: status = %d", rec.Code)
	}

	// THE property that makes this an undo: the object is still in storage while
	// the metadata sits in the bin. A bin that had already discarded the bytes
	// would be a delete with extra steps.
	if _, ok := backend.Get(key); !ok {
		t.Fatal("the stored bytes were discarded on delete — then restore cannot return the file")
	}

	if rec := do(t, h, http.MethodPost, "/api/v1/assets/"+id+"/restore", tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("restore: status = %d body=%s", rec.Code, rec.Body)
	}
	if got := assetFilenames(t, h, tenantA, ""); len(got) != 1 || got[0] != "fortrudt.pdf" {
		t.Fatalf("after restore the live listing holds %v", got)
	}
	// Restoring a live asset is a conflict, not a no-op.
	if rec := do(t, h, http.MethodPost, "/api/v1/assets/"+id+"/restore", tenantA, nil); rec.Code != http.StatusConflict {
		t.Fatalf("restore of a live asset: status = %d, want 409", rec.Code)
	}
}

func TestAssetDeleteIsTenantScoped(t *testing.T) {
	h := testRouter(t)
	id := newAsset(t, h, tenantA, "andres.pdf")

	// Positive control first: the owner CAN delete it. Without this, a handler
	// that rejected every delete would pass the cross-tenant assertions below.
	if rec := do(t, h, http.MethodDelete, "/api/v1/assets/"+id, tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("owner delete: status = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/assets/"+id+"/restore", tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("owner restore: status = %d", rec.Code)
	}

	// Another tenant gets 404, not 409: telling them the id exists but is
	// deleted would confirm that it exists.
	if rec := do(t, h, http.MethodDelete, "/api/v1/assets/"+id, tenantB, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant delete: status = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/assets/"+id+"/restore", tenantB, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant restore: status = %d, want 404", rec.Code)
	}
	if got := assetFilenames(t, h, tenantA, ""); len(got) != 1 {
		t.Fatalf("owner's asset was affected by the other tenant's calls: %v", got)
	}
}
