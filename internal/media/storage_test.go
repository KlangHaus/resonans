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

// The traversal table above measures safeKey: path.Clean has already absorbed
// every climb by the time os.Root would see it, so that test never reaches the
// kernel boundary. A SYMLINK is the case safeKey cannot see — the key is an
// ordinary name, and only os.Root refuses to follow it out of the volume. This
// is the cell for that second mechanism; without it, os.Root would be armed
// against nothing measured.
func TestASymlinkOutOfTheVolumeIsNotFollowed(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.png")
	original := []byte("do not touch")
	if err := os.WriteFile(secret, original, 0o600); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tenant-a"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "tenant-a", "escape.png")); err != nil {
		t.Skipf("symlinks unavailable on this filesystem: %v", err)
	}

	b, err := NewDiskBackend(dir)
	if err != nil {
		t.Fatalf("new disk backend: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	// Positive control in the same run: an ordinary key in the same directory
	// must still work, or "everything fails" would pass the assertions below.
	const ok = "tenant-a/ordinary.png"
	if err := b.Put(context.Background(), ok, "image/png", pngBytes(t, 2, 2)); err != nil {
		t.Fatalf("an ordinary key in the same directory was refused: %v", err)
	}
	if _, _, err := b.Open(ok); err != nil {
		t.Fatalf("an ordinary key in the same directory could not be read: %v", err)
	}

	if _, _, err := b.Open("tenant-a/escape.png"); err == nil {
		t.Fatal("Open read through a symlink pointing out of the volume")
	}

	// Put is allowed to SUCCEED here: the rename replaces the link entry itself
	// rather than writing through it, which is a safe outcome and the one Go's
	// os.Root actually produces. The property under test is not "Put errors" —
	// asserting that would pin an implementation detail instead of the security
	// boundary. It is that nothing outside the volume changed, and that what we
	// wrote is readable back from INSIDE it.
	payload := pngBytes(t, 3, 3)
	if err := b.Put(context.Background(), "tenant-a/escape.png", "image/png", payload); err != nil {
		t.Logf("Put refused the symlinked name: %v", err)
	}
	got, err := os.ReadFile(secret)
	if err != nil {
		t.Fatalf("read outside file: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("the file outside the volume was modified through the symlink: %q", got)
	}
	inside, err := os.Lstat(filepath.Join(dir, "tenant-a", "escape.png"))
	if err != nil {
		t.Fatalf("stat inside the volume: %v", err)
	}
	if inside.Mode()&os.ModeSymlink != 0 {
		t.Fatal("the entry inside the volume is still a symlink out of it")
	}
}
