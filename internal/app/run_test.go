package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KlangHaus/resonans/internal/config"
)

// devEnv is the smallest environment Run boots in: dev auth on both surfaces,
// no database, a disk volume, and a port nobody else holds.
func devEnv(t *testing.T) (addr, storageDir string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick a port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	storageDir = t.TempDir()
	for k, v := range map[string]string{
		"NODE_ENV": "development", "HOST": "127.0.0.1", "PORT": fmt.Sprint(port),
		"RESONANS_DEV_AUTH": "1", "DATABASE_URL": "", "RESONANS_JWT_ISSUER": "", "RESONANS_JWKS_URL": "",
		"RESONANS_DELIVERY_TOKEN": "", "RESONANS_STORAGE_DIR": storageDir, "RESONANS_VIDEO_PROVIDER": "vimeo",
		"BUNNY_STORAGE_ZONE": "", "BUNNY_STORAGE_KEY": "", "BUNNY_STORAGE_ENDPOINT": "",
		"OTEL_ENABLED": "false", "LOG_LEVEL": "error",
	} {
		t.Setenv(k, v)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port), storageDir
}

// start runs Run in the background and waits until /healthz answers. The
// returned stop cancels it and reports what Run returned.
func start(t *testing.T, ctx context.Context, base string) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- Run(ctx) }()
	deadline := time.Now().Add(15 * time.Second)
	for {
		select {
		case err := <-done:
			cancel()
			t.Fatalf("Run returned before serving: %v", err)
		default:
		}
		if resp, err := http.Get(base + "/healthz"); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("Run never answered /healthz")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(15 * time.Second):
			return errors.New("Run did not return after cancel")
		}
	}
}

func fetch(t *testing.T, method, url string, header map[string]string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// Run wires every surface onto one listener and stops cleanly on cancel. Each
// route is asked for something only its own handler answers, so a mux that
// mounted the wrong thing (or nothing) cannot pass on a generic 200.
func TestRunServesEverySurfaceAndStopsOnCancel(t *testing.T) {
	base, storageDir := devEnv(t)
	// A real PNG on the volume, so the asset route has bytes to serve.
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")
	if err := os.MkdirAll(filepath.Join(storageDir, "t"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "t", "x.png"), png, 0o600); err != nil {
		t.Fatal(err)
	}
	stop := start(t, context.Background(), base)

	tenant := map[string]string{"X-Dev-Tenant": "11111111-1111-1111-1111-111111111111"}
	scope := map[string]string{"X-Scope-OrgID": "11111111-1111-1111-1111-111111111111"}
	for _, c := range []struct {
		path   string
		header map[string]string
		code   int
		want   string
	}{
		{"/healthz", nil, 200, `"service":"resonans"`},
		{"/readyz", nil, 200, `"ready"`},
		{"/api/v1/schema", tenant, 200, "docTypes"},
		{"/api/v1/documents", nil, 403, "requires role viewer"}, // no tenant, no role: refused
		{"/delivery/v1/schema", scope, 200, "serverSideOnly"},
		{"/bridge/v1/resonans-bridge.js", nil, 200, "resonans"},
		{"/docs/v1/openapi.yaml", nil, 200, "openapi:"},
		{"/_assets/t/x.png", nil, 200, "PNG"},
		{"/_assets/t/missing.png", nil, 404, ""},
	} {
		code, body := fetch(t, http.MethodGet, base+c.path, c.header)
		if code != c.code || !strings.Contains(body, c.want) {
			t.Errorf("GET %s = %d %.80q; want %d containing %q", c.path, code, body, c.code, c.want)
		}
	}

	if err := stop(); err != nil {
		t.Fatalf("Run after cancel = %v, want nil", err)
	}
	if resp, err := http.Get(base + "/healthz"); err == nil {
		_ = resp.Body.Close()
		t.Error("the listener still answers after Run returned")
	}
}

// Each refusal is checked by its OWN message: a test that only asked for "an
// error" would pass on whichever check happened to fire first.
func TestRunRefusesToBoot(t *testing.T) {
	for _, c := range []struct {
		name string
		env  map[string]string
		want error
		text string
	}{
		{
			name: "dev auth in production",
			env:  map[string]string{"NODE_ENV": "production"},
			want: config.ErrDevAuthInProd,
		},
		{
			name: "no editor auth at all",
			env:  map[string]string{"RESONANS_DEV_AUTH": ""},
			want: config.ErrNoEditorAuth,
		},
		{
			name: "unknown video provider",
			env:  map[string]string{"RESONANS_VIDEO_PROVIDER": "youtube"},
			text: `unknown video provider "youtube"`,
		},
		{
			name: "storage dir is a file",
			env:  map[string]string{"RESONANS_STORAGE_DIR": "FILE"},
			text: "media: disk root",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			devEnv(t)
			for k, v := range c.env {
				if v == "FILE" {
					f := filepath.Join(t.TempDir(), "not-a-dir")
					if err := os.WriteFile(f, nil, 0o600); err != nil {
						t.Fatal(err)
					}
					v = f
				}
				t.Setenv(k, v)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := Run(ctx)
			switch {
			case err == nil:
				t.Fatal("Run booted")
			case c.want != nil && !errors.Is(err, c.want):
				t.Fatalf("Run = %v, want %v", err, c.want)
			case c.text != "" && !strings.Contains(err.Error(), c.text):
				t.Fatalf("Run = %v, want it to mention %q", err, c.text)
			}
		})
	}
}

// 🔴 The in-memory store does not enforce RLS. In production, a missing
// DATABASE_URL must stop the boot, not degrade tenant isolation silently.
// The rest of a production configuration is present, so the refusal reached
// is the database one and not an earlier auth check.
func TestRunRefusesProductionWithoutADatabase(t *testing.T) {
	devEnv(t)
	for k, v := range map[string]string{
		"NODE_ENV": "production", "RESONANS_DEV_AUTH": "",
		"RESONANS_JWT_ISSUER": "https://auth.example", "RESONANS_JWKS_URL": "https://auth.example/jwks",
		"RESONANS_JWT_AUDIENCE": "https://cms.example", "RESONANS_DELIVERY_TOKEN": "delivery-token",
		"RESONANS_PREVIEW_SECRET": "preview-secret-of-sufficient-length-0123456789",
	} {
		t.Setenv(k, v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL not set in production") {
		t.Fatalf("Run = %v, want the refusal to run production on the in-memory store", err)
	}
}

// A port another process holds is a fatal error, reported, not a hang.
func TestRunReportsAPortInUse(t *testing.T) {
	base, _ := devEnv(t)
	addr := strings.TrimPrefix(base, "http://")
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("hold the port: %v", err)
	}
	defer func() { _ = l.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := Run(ctx); err == nil || !strings.Contains(err.Error(), "http server") {
		t.Fatalf("Run on a held port = %v, want an http server error", err)
	}
}
