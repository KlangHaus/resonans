package media

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
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
