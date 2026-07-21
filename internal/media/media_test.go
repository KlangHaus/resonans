package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"

	"github.com/google/uuid"
)

var (
	tenantA = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tenantB = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestServiceUploadImage(t *testing.T) {
	backend := NewMemoryBackend()
	svc := NewService(NewMemoryStore(), backend, NewURLBuilder("https://cdn.test"))

	data := pngBytes(t, 4, 6)
	a, err := svc.Upload(context.Background(), tenantA, "Photo Nr 1.PNG", "image/png", data)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if a.Kind != KindImage {
		t.Errorf("kind = %q, want image", a.Kind)
	}
	if a.Width == nil || *a.Width != 4 || a.Height == nil || *a.Height != 6 {
		t.Errorf("dimensions = %v x %v, want 4x6", a.Width, a.Height)
	}
	if a.SizeBytes != int64(len(data)) {
		t.Errorf("size = %d, want %d", a.SizeBytes, len(data))
	}
	// Bytes landed in the backend under the storage key.
	if _, ok := backend.Get(a.StorageKey); !ok {
		t.Error("backend has no object for storage key")
	}
	// Storage key is tenant-prefixed and sanitized.
	if want := tenantA.String() + "/"; a.StorageKey[:len(want)] != want {
		t.Errorf("storage key %q not tenant-prefixed", a.StorageKey)
	}
}

func TestServiceUploadFileKind(t *testing.T) {
	svc := NewService(NewMemoryStore(), NewMemoryBackend(), NewURLBuilder("https://cdn.test"))
	a, err := svc.Upload(context.Background(), tenantA, "notes.pdf", "application/pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if a.Kind != KindFile {
		t.Errorf("kind = %q, want file", a.Kind)
	}
}

func TestStoreTenantIsolation(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	a, err := s.Create(ctx, tenantA, Asset{Kind: KindFile, StorageKey: "k", Filename: "f", Mime: "text/plain"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.Get(ctx, tenantB, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant Get = %v, want ErrNotFound", err)
	}
	list, _ := s.List(ctx, tenantB)
	if len(list) != 0 {
		t.Errorf("tenant B sees %d assets, want 0", len(list))
	}
}

func TestURLBuilder(t *testing.T) {
	u := NewURLBuilder("https://cdn.klanghaus.dk/")
	a := Asset{StorageKey: "t/abc-photo.png"}
	if got := u.AssetURL(a, Transform{}); got != "https://cdn.klanghaus.dk/t/abc-photo.png" {
		t.Errorf("plain URL = %q", got)
	}
	got := u.AssetURL(a, Transform{Width: 800, Height: 600})
	if got != "https://cdn.klanghaus.dk/t/abc-photo.png?height=600&width=800" {
		t.Errorf("transform URL = %q", got)
	}
}

func TestVimeoProvider(t *testing.T) {
	if got := (VimeoProvider{}).EmbedURL("123456"); got != "https://player.vimeo.com/video/123456" {
		t.Errorf("embed = %q", got)
	}
	if _, ok := ProviderByName("vimeo"); !ok {
		t.Error("vimeo provider should resolve")
	}
	if _, ok := ProviderByName("nope"); ok {
		t.Error("unknown provider should not resolve")
	}
}
