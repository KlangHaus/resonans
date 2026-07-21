package media

import (
	"net/url"
	"strconv"
	"strings"
)

// URLBuilder turns an asset into a public CDN URL, optionally with Bunny
// Optimizer transform parameters (resizing happens at the edge — Resonans just
// stores the original + emits transform URLs).
type URLBuilder struct {
	cdnBase string // Bunny pull-zone base, e.g. https://cdn.klanghaus.dk
}

// NewURLBuilder constructs a URL builder for the given CDN base.
func NewURLBuilder(cdnBase string) *URLBuilder {
	return &URLBuilder{cdnBase: strings.TrimRight(cdnBase, "/")}
}

// Transform requests an on-the-fly rendition (Bunny Optimizer). Zero fields are
// omitted (original).
type Transform struct {
	Width  int
	Height int
}

// AssetURL builds the CDN URL for an asset, applying any transform.
func (u *URLBuilder) AssetURL(a Asset, t Transform) string {
	base := u.cdnBase + "/" + strings.TrimLeft(a.StorageKey, "/")
	q := url.Values{}
	if t.Width > 0 {
		q.Set("width", strconv.Itoa(t.Width))
	}
	if t.Height > 0 {
		q.Set("height", strconv.Itoa(t.Height))
	}
	if len(q) == 0 {
		return base
	}
	return base + "?" + q.Encode()
}
