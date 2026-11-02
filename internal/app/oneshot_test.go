package app

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/media"
)

// Each one-shot refuses to run without a database, naming its own command, and
// says so before it tries to connect anywhere.
func TestOneShotsRequireADatabase(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("DATABASE_URL", "")
	ctx := context.Background()
	for name, run := range map[string]func() error{
		"seed":              func() error { return Seed(ctx) },
		"sanitize-backfill": func() error { return SanitizeBackfill(ctx, false) },
		"slug-audit":        func() error { return SlugAudit(ctx) },
		"migrate":           func() error { return Migrate(ctx) },
	} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required for "+name) {
			t.Errorf("%s without DATABASE_URL = %v, want the refusal naming %q", name, err, name)
		}
	}
}

// The seed's tenant is checked before any connection: a typo must not reach
// the database as some other tenant.
func TestSeedRequiresATenantUUID(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://nobody@127.0.0.1:1/none?sslmode=disable")
	for _, bad := range []string{"", "explainers", "1234"} {
		t.Setenv("RESONANS_SEED_TENANT", bad)
		if err := Seed(context.Background()); err == nil || !strings.Contains(err.Error(), "RESONANS_SEED_TENANT must be a tenant uuid") {
			t.Errorf("RESONANS_SEED_TENANT=%q: Seed = %v, want the tenant refusal", bad, err)
		}
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"fatal": slog.LevelError, "ERROR": slog.LevelError, "warn": slog.LevelWarn,
		"debug": slog.LevelDebug, "trace": slog.LevelDebug, "info": slog.LevelInfo, "": slog.LevelInfo, "loud": slog.LevelInfo,
	} {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestVersionAndRevisionAreTheStampedValues(t *testing.T) {
	oldV, oldR := version, revision
	t.Cleanup(func() { version, revision = oldV, oldR })
	version, revision = "1.2.3", "abc123"
	if Version() != "1.2.3" || Revision() != "abc123" {
		t.Errorf("Version/Revision = %q/%q", Version(), Revision())
	}
}

// The delivery envelope expands an asset id through this resolver. A bad or
// foreign id resolves to nothing (the id then passes through), and only the
// dimensions an asset actually has are reported.
func TestMediaAssetResolver(t *testing.T) {
	ctx := context.Background()
	store := media.NewMemoryStore()
	svc := media.NewService(store, media.NewMemoryBackend(), media.NewURLBuilder("https://cdn.example"))
	r := &mediaAssetResolver{media: svc}
	tenant, other := uuid.New(), uuid.New()
	w, h := 800, 600
	img, err := store.Create(ctx, tenant, media.Asset{Kind: media.KindImage, StorageKey: "t/i.png", Width: &w, Height: &h, SizeBytes: 42})
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.Create(ctx, tenant, media.Asset{Kind: media.KindFile, StorageKey: "t/f.pdf"})
	if err != nil {
		t.Fatal(err)
	}

	got, ok := r.ResolveAsset(ctx, tenant, img.ID.String())
	if !ok || got["url"] != "https://cdn.example/t/i.png" || got["width"] != 800 || got["height"] != 600 || got["sizeBytes"] != int64(42) {
		t.Errorf("image = %v, %v", got, ok)
	}
	got, ok = r.ResolveAsset(ctx, tenant, file.ID.String())
	_, hasW := got["width"]
	_, hasSize := got["sizeBytes"]
	if !ok || hasW || hasSize {
		t.Errorf("file = %v; want a url and no dimensions or size it does not have", got)
	}
	for name, id := range map[string]string{"not a uuid": "asset-1", "unknown": uuid.NewString()} {
		if got, ok := r.ResolveAsset(ctx, tenant, id); ok {
			t.Errorf("%s resolved to %v", name, got)
		}
	}
	if got, ok := r.ResolveAsset(ctx, other, img.ID.String()); ok {
		t.Errorf("another tenant resolved the asset: %v", got)
	}
}
