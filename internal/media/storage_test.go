package media

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The disk backend exists for exactly one property the memory backend lacks, so
// the memory backend is the negative control in the same run: same key, same
// bytes, and only one of them still has the object after the process is gone.
func TestDiskSurvivesARestartAndMemoryDoesNot(t *testing.T) {
	dir := t.TempDir()
	key := "tenant-a/asset-1.png"
	want := pngBytes(t, 2, 2)

	first, err := NewDiskBackend(dir)
	if err != nil {
		t.Fatalf("new disk backend: %v", err)
	}
	mem := NewMemoryBackend()
	for _, b := range []StorageBackend{first, mem} {
		if err := b.Put(context.Background(), key, "image/png", want); err != nil {
			t.Fatalf("%s put: %v", b.Name(), err)
		}
	}

	// A second backend over the same directory is what a restarted container is.
	restarted, err := NewDiskBackend(dir)
	if err != nil {
		t.Fatalf("reopen disk backend: %v", err)
	}
	f, size, err := restarted.Open(key)
	if err != nil {
		t.Fatalf("disk lost the object across a restart: %v", err)
	}
	defer func() { _ = f.Close() }()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("disk returned %d bytes, want the %d written", len(got), len(want))
	}
	if size != int64(len(want)) {
		t.Fatalf("disk reported size %d, want %d", size, len(want))
	}

	// Negative control: a fresh memory backend is what a restart leaves.
	if _, _, err := NewMemoryBackend().Open(key); err == nil {
		t.Fatal("the memory backend kept an object across a restart — then this test cannot tell the two backends apart")
	}
}

func TestStorageKeysThatClimbOutOfTheRootAreRefused(t *testing.T) {
	dir := t.TempDir()
	b, err := NewDiskBackend(dir)
	if err != nil {
		t.Fatalf("new disk backend: %v", err)
	}

	// Positive control first: without it, a Put that refuses EVERY key would
	// pass every case below.
	const good = "tenant-a/f7a1-logo.png"
	if err := b.Put(context.Background(), good, "image/png", pngBytes(t, 2, 2)); err != nil {
		t.Fatalf("a well-formed key was refused: %v", err)
	}

	bad := []string{
		"../escape.png",
		"tenant-a/../../escape.png", // path.Clean absorbs this into "escape.png"
		"/etc/passwd",
		"tenant-a//double.png",
		"./tenant-a/dot.png",
		`tenant-a\windows.png`,
		"",
		"..",
	}
	for _, key := range bad {
		t.Run(key, func(t *testing.T) {
			if err := b.Put(context.Background(), key, "image/png", []byte("x")); err == nil {
				t.Fatalf("Put accepted %q", key)
			}
			if _, _, err := b.Open(key); err == nil {
				t.Fatalf("Open accepted %q", key)
			}
		})
	}

	// Nothing may have landed outside the root. The denominator is stated: one
	// tenant directory holding one file is the whole expected tree.
	var files []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if len(files) != 1 || files[0] != good {
		t.Fatalf("root holds %v, want exactly [%s]", files, good)
	}
}

func serveAsset(t *testing.T, b ByteServer, key string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET "+AssetPath+"{key...}", AssetHandler(b))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, AssetPath+key, nil))
	return rec
}

func TestAssetRouteServesStoredBytesAndRefusesTheRest(t *testing.T) {
	dir := t.TempDir()
	b, err := NewDiskBackend(dir)
	if err != nil {
		t.Fatalf("new disk backend: %v", err)
	}
	want := pngBytes(t, 2, 2)
	const key = "tenant-a/f7a1-logo.png"
	if err := b.Put(context.Background(), key, "image/png", want); err != nil {
		t.Fatalf("put: %v", err)
	}

	rec := serveAsset(t, b, key)
	if rec.Code != http.StatusOK {
		t.Fatalf("stored asset: status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type %q, want image/png", ct)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options %q, want nosniff", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatalf("body is %d bytes, want the %d stored", rec.Body.Len(), len(want))
	}

	if rec := serveAsset(t, b, "tenant-a/missing.png"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset: status %d, want 404", rec.Code)
	}

	// A byte sequence the upload policy forbids, planted on disk by something
	// other than an upload, must not become servable just because it is there.
	planted := filepath.Join(dir, "tenant-a", "script.png")
	if err := os.WriteFile(planted, []byte("<html><script>alert(1)</script></html>"), 0o600); err != nil {
		t.Fatalf("plant: %v", err)
	}
	if rec := serveAsset(t, b, "tenant-a/script.png"); rec.Code != http.StatusNotFound {
		t.Fatalf("planted markup: status %d, want 404", rec.Code)
	}
}
