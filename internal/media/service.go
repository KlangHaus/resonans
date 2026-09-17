package media

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"  // register decoders so DecodeConfig reads dimensions (header only)
	_ "image/jpeg" //
	_ "image/png"  //
	"strings"

	"github.com/google/uuid"
)

// Service orchestrates asset upload: it writes bytes to the storage backend and
// records metadata in the store, and resolves assets to CDN URLs.
type Service struct {
	store   Store
	backend StorageBackend
	url     *URLBuilder
}

// NewService wires the asset service.
func NewService(store Store, backend StorageBackend, url *URLBuilder) *Service {
	return &Service{store: store, backend: backend, url: url}
}

// Upload stores the bytes and records the asset for the tenant. The stored MIME
// type and the storage key's extension come from the CONTENT (DetectType), not
// from the client: the claimed MIME type (the unnamed parameter) is ignored, so a
// PNG-looking name on an HTML file, or an HTML name on PNG bytes, cannot make a
// CDN serve markup. For images it detects width/height from the header.
func (s *Service) Upload(ctx context.Context, tenantID uuid.UUID, filename, _ string, data []byte) (Asset, error) {
	mime, ext, err := DetectType(data)
	if err != nil {
		return Asset{}, err
	}
	a := Asset{
		Kind:      KindFile,
		Filename:  filename,
		Mime:      mime,
		SizeBytes: int64(len(data)),
	}
	if strings.HasPrefix(mime, "image/") {
		a.Kind = KindImage
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			w, h := cfg.Width, cfg.Height
			a.Width, a.Height = &w, &h
		}
	}
	a.StorageKey = storageKey(tenantID, filename, ext)
	if err := s.backend.Put(ctx, a.StorageKey, mime, data); err != nil {
		return Asset{}, err
	}
	return s.store.Create(ctx, tenantID, a)
}

// Get returns an asset by id, tenant-scoped.
func (s *Service) Get(ctx context.Context, tenantID, id uuid.UUID) (Asset, error) {
	return s.store.Get(ctx, tenantID, id)
}

// List returns the tenant's assets.
func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]Asset, error) {
	return s.store.List(ctx, tenantID)
}

// URL builds the CDN URL for an asset with an optional transform.
func (s *Service) URL(a Asset, t Transform) string { return s.url.AssetURL(a, t) }

// storageKey produces a unique, tenant-prefixed object key whose extension is
// the one for the DETECTED type. CDNs commonly pick Content-Type from the key's
// extension, so the client's extension must not survive: "x.html" holding PNG
// bytes is stored as "…-x_html.png".
func storageKey(tenantID uuid.UUID, filename, ext string) string {
	base := sanitizeFilename(filename)
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		base = base[:i] + "_" + base[i+1:]
	}
	base = strings.ReplaceAll(base, ".", "_")
	if len(base)+len(ext) > 120 {
		base = base[:120-len(ext)]
	}
	return tenantID.String() + "/" + uuid.NewString() + "-" + base + ext
}

// sanitizeFilename keeps a safe subset of characters for an object key segment.
func sanitizeFilename(f string) string {
	f = strings.ToLower(strings.TrimSpace(f))
	var b strings.Builder
	for _, r := range f {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "file"
	}
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}
