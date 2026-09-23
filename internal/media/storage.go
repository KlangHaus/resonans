package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StorageBackend stores asset bytes under a key. The metadata lives in the
// asset Store; the bytes live here (Bunny Storage in prod, in-memory in dev/test).
type StorageBackend interface {
	Put(ctx context.Context, key, mime string, data []byte) error
	Name() string
}

// MemoryBackend keeps bytes in memory (dev/test). NOT persistent.
type MemoryBackend struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

// NewMemoryBackend constructs an empty in-memory backend.
func NewMemoryBackend() *MemoryBackend { return &MemoryBackend{objects: map[string][]byte{}} }

// Put stores a copy of data under key.
func (b *MemoryBackend) Put(_ context.Context, key, _ string, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objects[key] = append([]byte(nil), data...)
	return nil
}

// Name identifies the backend.
func (b *MemoryBackend) Name() string { return "memory" }

// nopSeekCloser adds a no-op Close to a bytes.Reader so the in-memory backend
// satisfies ByteServer and the same asset route serves dev and disk deploys.
type nopSeekCloser struct{ *bytes.Reader }

func (nopSeekCloser) Close() error { return nil }

// Open returns the stored bytes for key, or fs.ErrNotExist.
func (b *MemoryBackend) Open(key string) (io.ReadSeekCloser, int64, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.objects[key]
	if !ok {
		return nil, 0, fs.ErrNotExist
	}
	return nopSeekCloser{bytes.NewReader(v)}, int64(len(v)), nil
}

// Get returns stored bytes (test introspection).
func (b *MemoryBackend) Get(key string) ([]byte, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.objects[key]
	return v, ok
}

// BunnyBackend uploads to Bunny Storage via the Storage API (HTTP PUT with an
// AccessKey header). Endpoint + zone + access key come from trusted config
// ([infra]/SOPS), never from request input.
type BunnyBackend struct {
	endpoint  string // e.g. "storage.bunnycdn.com" or a regional host
	zone      string
	accessKey string
	client    *http.Client
}

// NewBunnyBackend constructs a Bunny Storage backend.
func NewBunnyBackend(endpoint, zone, accessKey string) *BunnyBackend {
	return &BunnyBackend{
		endpoint:  endpoint,
		zone:      zone,
		accessKey: accessKey,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Name identifies the backend.
func (b *BunnyBackend) Name() string { return "bunny" }

// Put uploads data to https://{endpoint}/{zone}/{key}.
func (b *BunnyBackend) Put(ctx context.Context, key, mime string, data []byte) error {
	// endpoint+zone come from trusted infra config (SOPS), not user input; the
	// key is sanitized by the media service before reaching here.
	url := fmt.Sprintf("https://%s/%s/%s", b.endpoint, b.zone, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("media: bunny put request: %w", err)
	}
	req.Header.Set("AccessKey", b.accessKey)
	req.Header.Set("Content-Type", mime)
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("media: bunny put: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("media: bunny put status %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ErrBadKey names a storage key that must never reach the filesystem. The disk
// backend turns a key into a path, so a key that climbs out of the root is the
// whole hazard. The media service builds keys itself (tenant/uuid-name.ext), so
// a rejected key means a bug in this repo rather than something a client sent —
// but the check lives at the boundary that does the damage, because whoever
// copies this backend into another project will not have read the service.
var ErrBadKey = errors.New("media: unsafe storage key")

// safeKey refuses any key that is not already a clean relative slash-path.
// path.Clean would happily ABSORB a climb ("a/../../b" becomes "b"), which
// turns an attack into a silent read of a different object; comparing against
// the input rejects it instead.
func safeKey(key string) (string, error) {
	if key == "" || strings.ContainsAny(key, `\\`+"\x00") {
		return "", ErrBadKey
	}
	clean := strings.TrimPrefix(path.Clean("/"+key), "/")
	if clean == "" || clean != key {
		return "", ErrBadKey
	}
	return clean, nil
}

// ByteServer is a StorageBackend that can also read its bytes back, so Resonans
// can serve /_assets itself. BunnyBackend deliberately does not implement it:
// its bytes are served by the CDN, and pulling them back through the origin
// would defeat the point of having a CDN at all.
type ByteServer interface {
	StorageBackend
	Open(key string) (io.ReadSeekCloser, int64, error)
}

// DiskBackend stores asset bytes on the local filesystem under a root
// directory. It is what makes Resonans self-hostable without a CDN account: a
// mounted volume is the only dependency, and the bytes survive a restart —
// which the in-memory backend does not.
type DiskBackend struct{ root string }

// NewDiskBackend constructs a disk backend rooted at dir, creating it if needed.
func NewDiskBackend(dir string) (*DiskBackend, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("media: disk root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("media: disk root: %w", err)
	}
	return &DiskBackend{root: abs}, nil
}

// Name identifies the backend.
func (b *DiskBackend) Name() string { return "disk" }

func (b *DiskBackend) pathFor(key string) (string, error) {
	clean, err := safeKey(key)
	if err != nil {
		return "", err
	}
	return filepath.Join(b.root, filepath.FromSlash(clean)), nil
}

// Put writes data under key. The write is atomic — a temp file in the target
// directory, then a rename — so a reader never sees a half-written asset and a
// failed upload leaves no truncated file behind under the real name.
func (b *DiskBackend) Put(_ context.Context, key, _ string, data []byte) error {
	name, err := b.pathFor(key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("media: disk put: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return fmt.Errorf("media: disk put: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // a no-op once the rename succeeded
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("media: disk put: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("media: disk put: %w", err)
	}
	if err := os.Chmod(tmpName, 0o640); err != nil {
		return fmt.Errorf("media: disk put: %w", err)
	}
	if err := os.Rename(tmpName, name); err != nil {
		return fmt.Errorf("media: disk put: %w", err)
	}
	return nil
}

// Open returns the stored bytes for key. A missing object reports fs.ErrNotExist
// so the caller can answer 404 without inspecting the message.
func (b *DiskBackend) Open(key string) (io.ReadSeekCloser, int64, error) {
	name, err := b.pathFor(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(name) //nolint:gosec // key is validated by safeKey above
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	if st.IsDir() {
		_ = f.Close()
		return nil, 0, fs.ErrNotExist
	}
	return f, st.Size(), nil
}
