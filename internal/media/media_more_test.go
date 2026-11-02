package media

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The bin, through the Service an editor uses: a deleted asset says it is
// deleted (not "not found"), leaves the default listing, and comes back.
func TestServiceAssetBin(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore(), NewMemoryBackend(), NewURLBuilder("https://cdn.example"))
	a, err := svc.Upload(ctx, tenantA, "photo.png", "", pngBytes(t, 4, 4))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if a.IsDeleted() {
		t.Fatal("a new asset reports IsDeleted")
	}
	if _, err := svc.Restore(ctx, tenantA, a.ID); !errors.Is(err, ErrNotDeleted) {
		t.Fatalf("Restore(live) err = %v, want ErrNotDeleted", err)
	}
	if _, err := svc.SoftDelete(ctx, tenantB, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant SoftDelete err = %v, want ErrNotFound", err)
	}

	del, err := svc.SoftDelete(ctx, tenantA, a.ID)
	if err != nil || !del.IsDeleted() {
		t.Fatalf("SoftDelete = deleted:%v, %v", del.IsDeleted(), err)
	}
	if _, err := svc.SoftDelete(ctx, tenantA, a.ID); !errors.Is(err, ErrDeleted) {
		t.Errorf("second SoftDelete err = %v, want ErrDeleted (the first deletion time must not be overwritten)", err)
	}
	if _, err := svc.Get(ctx, tenantA, a.ID); !errors.Is(err, ErrDeleted) {
		t.Errorf("Get(deleted) err = %v, want ErrDeleted, not ErrNotFound", err)
	}
	live, _ := svc.List(ctx, tenantA, ListFilter{})
	bin, _ := svc.List(ctx, tenantA, ListFilter{Deleted: DeletedOnly})
	all, _ := svc.List(ctx, tenantA, ListFilter{Deleted: DeletedInclude})
	if len(live) != 0 || len(bin) != 1 || len(all) != 1 {
		t.Errorf("List live/bin/all = %d/%d/%d, want 0/1/1", len(live), len(bin), len(all))
	}
	if _, err := svc.Restore(ctx, tenantB, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant Restore err = %v, want ErrNotFound", err)
	}

	back, err := svc.Restore(ctx, tenantA, a.ID)
	if err != nil || back.IsDeleted() {
		t.Fatalf("Restore = deleted:%v, %v", back.IsDeleted(), err)
	}
	if got, err := svc.Get(ctx, tenantA, a.ID); err != nil || got.ID != a.ID {
		t.Fatalf("Get after restore = %v, %v", got.ID, err)
	}
	if got := svc.URL(back, Transform{Width: 10}); got != "https://cdn.example/"+a.StorageKey+"?width=10" {
		t.Errorf("URL = %q", got)
	}
}

func TestMemoryAssetListIsNewestFirstAndTenantScoped(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	for i := 0; i < 3; i++ {
		if _, err := s.Create(ctx, tenantA, Asset{Kind: KindFile, StorageKey: "k", Filename: "f", Mime: "application/pdf"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if _, err := s.Create(ctx, tenantB, Asset{Kind: KindFile}); err != nil {
		t.Fatalf("Create B: %v", err)
	}
	list, _ := s.List(ctx, tenantA, ListFilter{})
	if len(list) != 3 {
		t.Fatalf("List(A) = %d assets, want 3 (B's must not appear)", len(list))
	}
	for i := 1; i < len(list); i++ {
		prev, cur := list[i-1], list[i]
		if prev.CreatedAt.Before(cur.CreatedAt) ||
			(prev.CreatedAt.Equal(cur.CreatedAt) && prev.ID.String() > cur.ID.String()) {
			t.Fatalf("List order broken at %d: %v then %v", i, prev.CreatedAt, cur.CreatedAt)
		}
	}
}

// No metadata row without the bytes: a backend that fails leaves no asset that
// would point at nothing.
func TestUploadRecordsNothingWhenTheBytesFail(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, failingBackend{}, NewURLBuilder("https://cdn.example"))
	if _, err := svc.Upload(context.Background(), tenantA, "photo.png", "", pngBytes(t, 2, 2)); err == nil {
		t.Fatal("Upload succeeded with a failing backend")
	}
	if all, _ := store.List(context.Background(), tenantA, ListFilter{Deleted: DeletedInclude}); len(all) != 0 {
		t.Fatalf("%d asset rows recorded for bytes that were never stored", len(all))
	}
}

type failingBackend struct{}

func (failingBackend) Put(context.Context, string, string, []byte) error {
	return errors.New("disk full")
}
func (failingBackend) Name() string { return "failing" }

func TestStorageKeyKeepsOnlyTheDetectedExtension(t *testing.T) {
	long := strings.Repeat("a", 300) + ".html"
	for _, c := range []struct{ in, wantSuffix string }{
		{"Photo Of Me.HTML", "-photo_of_me_html.png"},
		{"a.b.c.png", "-a_b_c_png.png"},
		{"   ", "-file.png"},
	} {
		key := storageKey(tenantA, c.in, ".png")
		if !strings.HasPrefix(key, tenantA.String()+"/") || !strings.HasSuffix(key, c.wantSuffix) {
			t.Errorf("storageKey(%q) = %q, want tenant prefix and suffix %q", c.in, key, c.wantSuffix)
		}
	}
	if key := storageKey(tenantA, long, ".png"); !strings.HasSuffix(key, ".png") || len(key) > len(tenantA.String())+1+36+1+120 {
		t.Errorf("long name gave a %d-byte key %q", len(key), key)
	}
}

func TestAllowedTypesExcludesEverythingScriptable(t *testing.T) {
	types := AllowedTypes()
	if !slices.IsSorted(types) || !slices.Contains(types, "image/png") || !slices.Contains(types, "application/pdf") {
		t.Fatalf("AllowedTypes = %v, want sorted and including png and pdf", types)
	}
	for _, bad := range []string{"text/html", "image/svg+xml", "text/plain", "application/javascript"} {
		if slices.Contains(types, bad) {
			t.Errorf("AllowedTypes includes %s", bad)
		}
	}
}

func TestBunnyBackendPut(t *testing.T) {
	var got struct{ method, path, key, ctype, body string }
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.method, got.path, got.key, got.ctype, got.body = r.Method, r.URL.Path, r.Header.Get("AccessKey"), r.Header.Get("Content-Type"), string(b)
		if strings.Contains(r.URL.Path, "refused") {
			http.Error(w, "zone quota exceeded", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	b := NewBunnyBackend(strings.TrimPrefix(srv.URL, "https://"), "zone1", "s3cret")
	b.client = srv.Client()
	if b.Name() != "bunny" {
		t.Errorf("Name = %q", b.Name())
	}
	if err := b.Put(context.Background(), "t/k.png", "image/png", []byte("bytes")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got.method != http.MethodPut || got.path != "/zone1/t/k.png" || got.key != "s3cret" || got.ctype != "image/png" || got.body != "bytes" {
		t.Errorf("request = %+v", got)
	}
	err := b.Put(context.Background(), "t/refused.png", "image/png", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "zone quota exceeded") {
		t.Errorf("rejected Put err = %v, want the status and the server's reason", err)
	}
	srv.Close()
	if err := b.Put(context.Background(), "t/k.png", "image/png", []byte("x")); err == nil {
		t.Error("Put to a closed server succeeded")
	}
}

func TestDiskBackendRefusals(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDiskBackend(file); err == nil {
		t.Error("NewDiskBackend on a regular file succeeded")
	}

	b, err := NewDiskBackend(filepath.Join(dir, "root"))
	if err != nil {
		t.Fatalf("NewDiskBackend: %v", err)
	}
	defer func() { _ = b.Close() }()
	if b.Name() != "disk" {
		t.Errorf("Name = %q", b.Name())
	}
	if err := b.Put(context.Background(), "t/a/obj.png", "", []byte("x")); err != nil {
		t.Fatalf("control Put: %v", err)
	}
	// "t/a" is a directory: opening it as an object is a miss, not its listing.
	if _, _, err := b.Open("t/a"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open(directory) err = %v, want fs.ErrNotExist", err)
	}
	// A key whose parent is an existing FILE cannot be created.
	if err := b.Put(context.Background(), "t/a/obj.png/child", "", []byte("x")); err == nil {
		t.Error("Put under a file succeeded")
	}
	if name := NewMemoryBackend().Name(); name != "memory" {
		t.Errorf("memory Name = %q", name)
	}
}

// byteServerStub fails where the real backends cannot be made to fail on demand.
type byteServerStub struct {
	openErr error
	r       io.ReadSeekCloser
}

func (s byteServerStub) Put(context.Context, string, string, []byte) error { return nil }
func (s byteServerStub) Name() string                                      { return "stub" }
func (s byteServerStub) Open(string) (io.ReadSeekCloser, int64, error) {
	if s.openErr != nil {
		return nil, 0, s.openErr
	}
	return s.r, 0, nil
}

type brokenReader struct{ readErr, seekErr error }

func (b brokenReader) Read([]byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return 0, io.EOF
}
func (b brokenReader) Seek(int64, int) (int64, error) { return 0, b.seekErr }
func (brokenReader) Close() error                     { return nil }

// A backend failure that is not "missing" is a 500 with a generic body, never
// a 404 that would hide an outage.
func TestAssetHandlerBackendFailures(t *testing.T) {
	serve := func(b ByteServer) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		mux.Handle("GET "+AssetPath+"{key...}", AssetHandler(b))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, AssetPath+"t/x.png", nil))
		return rec
	}
	for _, c := range []struct {
		name string
		b    ByteServer
	}{
		{"open fails", byteServerStub{openErr: errors.New("io error on volume")}},
		{"read fails", byteServerStub{r: brokenReader{readErr: errors.New("read error")}}},
	} {
		rec := serve(c.b)
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "error on volume") {
			t.Errorf("%s: %d %q, want 500 without the backend's message", c.name, rec.Code, rec.Body)
		}
	}
	// Bytes the type policy refuses are a 404, even when they are on the volume.
	if rec := serve(byteServerStub{r: brokenReader{}}); rec.Code != http.StatusNotFound {
		t.Errorf("unrecognizable bytes = %d, want 404", rec.Code)
	}
	if rec := serve(byteServerStub{openErr: fs.ErrNotExist}); rec.Code != http.StatusNotFound {
		t.Errorf("missing = %d, want 404", rec.Code)
	}
}

func TestAssetHandlerSeekFailure(t *testing.T) {
	png := pngBytes(t, 1, 1)
	r := &seekFailing{data: png}
	mux := http.NewServeMux()
	mux.Handle("GET "+AssetPath+"{key...}", AssetHandler(byteServerStub{r: r}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, AssetPath+"t/x.png", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("seek failure = %d, want 500", rec.Code)
	}
}

type seekFailing struct {
	data []byte
	off  int
}

func (s *seekFailing) Read(p []byte) (int, error) {
	if s.off >= len(s.data) {
		return 0, io.EOF
	}
	n := copy(p, s.data[s.off:])
	s.off += n
	return n, nil
}
func (*seekFailing) Seek(int64, int) (int64, error) { return 0, errors.New("seek unsupported") }
func (*seekFailing) Close() error                   { return nil }

func TestVimeoMetadataFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"title":`)
	}))
	p := &VimeoProvider{oembedBase: srv.URL, client: srv.Client()}
	if p.Name() != "vimeo" {
		t.Errorf("Name = %q", p.Name())
	}
	if _, err := p.Metadata(context.Background(), "1"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("truncated JSON err = %v, want a decode error", err)
	}
	srv.Close()
	if _, err := p.Metadata(context.Background(), "1"); err == nil {
		t.Error("Metadata against a closed server succeeded")
	}
}
