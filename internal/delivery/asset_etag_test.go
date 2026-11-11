package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
)

const (
	coverID   = "cover-1"
	coverURL  = "https://cdn.test/cover.jpg"
	courseURL = "/delivery/v1/course/with-cover"
	courseCat = "/delivery/v1/course"
)

// binnableAssets resolves coverID until it is put in the bin, the way the media
// store stops resolving an asset whose row has deleted_at set.
type binnableAssets struct{ binned bool }

func (b *binnableAssets) ResolveAsset(_ context.Context, _ uuid.UUID, id string) (map[string]any, bool) {
	if id != coverID || b.binned {
		return nil, false
	}
	return map[string]any{"url": coverURL}, true
}

// assetCase is one published course whose cover is an asset, behind a router
// whose resolver can bin and restore that asset without touching the document.
type assetCase struct {
	t      *testing.T
	assets *binnableAssets
	h      http.Handler
}

func newAssetCase(t *testing.T) *assetCase {
	t.Helper()
	s := content.NewMemoryStore()
	ctx := context.Background()
	body := json.RawMessage(`{"coverImage":"` + coverID + `"}`)
	d, err := s.Create(ctx, tenantA, content.CreateInput{Type: "course", Slug: "with-cover", Title: "With cover", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, tenantA, d.ID); err != nil {
		t.Fatal(err)
	}
	a := &binnableAssets{}
	return &assetCase{t: t, assets: a, h: NewRouter(Deps{Store: s, Registry: reg(t), Assets: a, TrustHeaderWithoutToken: true})}
}

// fetch returns the status, ETag and body for path, revalidating with inm when
// it is not empty.
func (c *assetCase) fetch(path, inm string) (int, string, string) {
	c.t.Helper()
	header := map[string]string{}
	if inm != "" {
		header["If-None-Match"] = inm
	}
	rec := getWith(c.h, path, header)
	return rec.Code, rec.Header().Get("ETag"), rec.Body.String()
}

// The green case first, on both routes: with the cover live, the body carries
// its URL and the ETag answers 304. Then bin the cover: the body no longer
// carries it, so the old ETag must get 200 and the new body. Restore it: the
// body is the first one again.
func TestDeliveryETagMovesWhenAnAssetIsBinnedAndRestored(t *testing.T) {
	for _, path := range []string{courseURL, courseCat} {
		t.Run(strings.TrimPrefix(path, "/delivery/v1/"), func(t *testing.T) {
			newAssetCase(t).binAndRestore(path)
		})
	}
}

func (c *assetCase) binAndRestore(path string) {
	t := c.t
	t.Helper()
	code, live, body := c.fetch(path, "")
	if code != http.StatusOK || live == "" || !strings.Contains(body, coverURL) {
		t.Fatalf("live cover: %d ETag %q, cover in body %t; want 200, an ETag, true", code, live, strings.Contains(body, coverURL))
	}
	if code, _, _ := c.fetch(path, live); code != http.StatusNotModified {
		t.Fatalf("unchanged body revalidated = %d, want 304", code)
	}

	c.assets.binned = true
	code, binned, body := c.fetch(path, live)
	if code != http.StatusOK || strings.Contains(body, coverURL) {
		t.Fatalf("binned cover revalidated with the live ETag = %d, cover in body %t; want 200, false", code, strings.Contains(body, coverURL))
	}
	if binned == live {
		t.Fatalf("ETag %q did not move when the body lost the cover", live)
	}

	c.assets.binned = false
	if code, _, _ := c.fetch(path, binned); code != http.StatusOK {
		t.Fatalf("restored cover revalidated with the binned ETag = %d, want 200", code)
	}
	if code, _, _ := c.fetch(path, live); code != http.StatusNotModified {
		t.Fatalf("restored cover revalidated with the live ETag = %d, want 304", code)
	}
}
