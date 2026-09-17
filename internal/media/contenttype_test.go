package media

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recordingBackend struct {
	key, mime string
	puts      int
}

func (b *recordingBackend) Put(_ context.Context, key, mime string, _ []byte) error {
	b.key, b.mime, b.puts = key, mime, b.puts+1
	return nil
}
func (b *recordingBackend) Name() string { return "recording" }

// Rejected by CONTENT, whatever the client claims (KH-1011).
func TestUploadRejectsScriptCarryingTypes(t *testing.T) {
	cases := []struct {
		name, filename, claimed, body string
	}{
		{"svg with script", "logo.svg", "image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`},
		{"svg with xml head", "logo.svg", "image/svg+xml", `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`},
		{"html", "page.html", "text/html", `<html><script>alert(1)</script></html>`},
		{"html claiming png", "photo.png", "image/png", `<html><script>alert(1)</script></html>`},
		{"html with BOM (sniffs text/plain)", "photo.png", "image/png", "\xef\xbb\xbf<!DOCTYPE html><script>alert(1)</script>"},
		{"plain text", "notes.txt", "text/plain", "hello"},
		{"empty", "x.png", "image/png", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &recordingBackend{}
			svc := NewService(NewMemoryStore(), b, NewURLBuilder("https://cdn.test"))
			_, err := svc.Upload(context.Background(), tenantA, c.filename, c.claimed, []byte(c.body))
			if !errors.Is(err, ErrUnsupportedType) {
				t.Fatalf("err = %v, want ErrUnsupportedType", err)
			}
			if b.puts != 0 {
				t.Errorf("rejected upload reached the backend %d time(s)", b.puts)
			}
		})
	}
}

// The stored type and the key's extension follow the bytes, not the name.
func TestUploadTypeAndExtensionFollowContent(t *testing.T) {
	b := &recordingBackend{}
	svc := NewService(NewMemoryStore(), b, NewURLBuilder("https://cdn.test"))
	a, err := svc.Upload(context.Background(), tenantA, "evil.html", "text/html", pngBytes(t, 2, 3))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if a.Mime != "image/png" || b.mime != "image/png" {
		t.Errorf("stored mime = %q, backend mime = %q, want image/png", a.Mime, b.mime)
	}
	if a.Kind != KindImage {
		t.Errorf("kind = %q, want image", a.Kind)
	}
	if !strings.HasSuffix(b.key, "-evil_html.png") || strings.Contains(b.key, ".html") {
		t.Errorf("storage key = %q, want the claimed .html replaced by .png", b.key)
	}
	if a.Filename != "evil.html" {
		t.Errorf("display filename = %q, want the original", a.Filename)
	}
}

func TestDetectTypeAllowlist(t *testing.T) {
	ok := map[string]string{
		"%PDF-1.7\n":                   "application/pdf",
		"PK\x03\x04":                   "application/zip",
		"ID3\x03\x00":                  "audio/mpeg",
		"GIF89a":                       "image/gif",
		"RIFF\x00\x00\x00\x00WEBPVP8 ": "image/webp",
	}
	for body, want := range ok {
		if got, _, err := DetectType([]byte(body)); err != nil || got != want {
			t.Errorf("DetectType(%q) = %q, %v; want %q", body, got, err, want)
		}
	}
}
