//go:build integration

// The Postgres asset store against a real Postgres. Until these cells existed,
// no test called media.NewPostgresStore, the store production runs.
//
// 🔴 As a role WITHOUT bypassrls (testpg.AppPool), never as the migrator. The
// store's queries carry no tenant_id — List has no WHERE on the tenant at all —
// so isolation is the RLS policy alone, and a superuser would read across
// tenants. TestPostgresAssetIsolationIsTheRoles shows that on the same row.
package media

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/persistence"
	"github.com/KlangHaus/resonans/internal/testpg"
)

func assetStores(t *testing.T) (app, migrator *PostgresStore) {
	t.Helper()
	pool, scopedDSN := testpg.PoolDSN(t)
	if _, err := persistence.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewPostgresStore(testpg.AppPool(t, pool, scopedDSN, "media_it_app")), NewPostgresStore(pool)
}

func pgAsset(t *testing.T, s *PostgresStore, tenant uuid.UUID, name string) Asset {
	t.Helper()
	w, h := 640, 480
	a, err := s.Create(context.Background(), tenant, Asset{
		Kind: KindImage, StorageKey: tenant.String() + "/" + name, Filename: name,
		Mime: "image/png", SizeBytes: 1234, Width: &w, Height: &h,
	})
	if err != nil {
		t.Fatalf("Create(%s): %v", name, err)
	}
	return a
}

func TestPostgresAssetIsolationIsTheRoles(t *testing.T) {
	ctx := context.Background()
	app, migrator := assetStores(t)
	tenantA, tenantB := uuid.New(), uuid.New()
	a := pgAsset(t, app, tenantA, "private.png")

	// NEGATIVE CONTROL, on the same row: the migrator bypasses RLS.
	if got, err := migrator.List(ctx, tenantB, ListFilter{}); err != nil || len(got) != 1 {
		t.Fatalf("control: the migrator listed %d rows as tenant B (%v), want 1 — "+
			"nothing below can show that RLS is what isolates", len(got), err)
	}

	if got, err := app.List(ctx, tenantB, ListFilter{Deleted: DeletedInclude}); err != nil || len(got) != 0 {
		t.Errorf("List as tenant B = %d rows, %v; want 0", len(got), err)
	}
	if _, err := app.Get(ctx, tenantB, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get across tenants err = %v, want ErrNotFound", err)
	}
	if _, err := app.SoftDelete(ctx, tenantB, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("SoftDelete across tenants err = %v, want ErrNotFound", err)
	}
	if got, err := app.Get(ctx, tenantA, a.ID); err != nil || got.IsDeleted() {
		t.Errorf("after B's attempt, A reads deleted=%v (%v); want it untouched", got.IsDeleted(), err)
	}
}

func TestPostgresAssetRoundTrip(t *testing.T) {
	ctx := context.Background()
	app, _ := assetStores(t)
	tenant := uuid.New()
	a := pgAsset(t, app, tenant, "photo.png")

	got, err := app.Get(ctx, tenant, a.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TenantID != tenant || got.Kind != KindImage || got.Mime != "image/png" || got.SizeBytes != 1234 ||
		got.Width == nil || *got.Width != 640 || got.Height == nil || *got.Height != 480 || got.CreatedAt.IsZero() {
		t.Errorf("round trip = %+v", got)
	}
	// An asset created without a hotspot stores `{}`, never NULL or garbage.
	var hotspot map[string]any
	if err := json.Unmarshal(got.Hotspot, &hotspot); err != nil || len(hotspot) != 0 {
		t.Errorf("default hotspot = %q (%v), want {}", got.Hotspot, err)
	}
	file, err := app.Create(ctx, tenant, Asset{
		Kind: KindFile, StorageKey: "k.pdf", Filename: "k.pdf", Mime: "application/pdf",
		Hotspot: json.RawMessage(`{"x":0.5,"y":0.25}`),
	})
	if err != nil || file.Width != nil || file.Height != nil {
		t.Fatalf("Create(file) = width %v height %v, %v; want no dimensions", file.Width, file.Height, err)
	}
	if !json.Valid(file.Hotspot) || string(file.Hotspot) == "{}" {
		t.Errorf("given hotspot was not kept: %q", file.Hotspot)
	}
	if _, err := app.Get(ctx, tenant, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(unknown) err = %v, want ErrNotFound", err)
	}

	list, err := app.List(ctx, tenant, ListFilter{})
	if err != nil || len(list) != 2 || list[0].ID != file.ID {
		t.Errorf("List = %d assets, first %v (%v); want 2, newest first", len(list), list[0].ID, err)
	}
}

func TestPostgresAssetBin(t *testing.T) {
	ctx := context.Background()
	app, _ := assetStores(t)
	tenant := uuid.New()
	a := pgAsset(t, app, tenant, "old.png")

	if _, err := app.Restore(ctx, tenant, a.ID); !errors.Is(err, ErrNotDeleted) {
		t.Fatalf("Restore(live) err = %v, want ErrNotDeleted", err)
	}
	del, err := app.SoftDelete(ctx, tenant, a.ID)
	if err != nil || !del.IsDeleted() {
		t.Fatalf("SoftDelete = deleted:%v, %v", del.IsDeleted(), err)
	}
	if _, err := app.SoftDelete(ctx, tenant, a.ID); !errors.Is(err, ErrDeleted) {
		t.Errorf("second SoftDelete err = %v, want ErrDeleted", err)
	}
	// classifyAssetMiss: the bin is told apart from "never existed".
	if _, err := app.Get(ctx, tenant, a.ID); !errors.Is(err, ErrDeleted) {
		t.Errorf("Get(deleted) err = %v, want ErrDeleted", err)
	}
	live, _ := app.List(ctx, tenant, ListFilter{})
	bin, _ := app.List(ctx, tenant, ListFilter{Deleted: DeletedOnly})
	all, _ := app.List(ctx, tenant, ListFilter{Deleted: DeletedInclude})
	if len(live) != 0 || len(bin) != 1 || len(all) != 1 {
		t.Errorf("List live/bin/all = %d/%d/%d, want 0/1/1", len(live), len(bin), len(all))
	}

	back, err := app.Restore(ctx, tenant, a.ID)
	if err != nil || back.IsDeleted() {
		t.Fatalf("Restore = deleted:%v, %v", back.IsDeleted(), err)
	}
	if _, err := app.Get(ctx, tenant, a.ID); err != nil {
		t.Errorf("Get after restore: %v", err)
	}
	if _, err := app.Restore(ctx, tenant, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("Restore(unknown) err = %v, want ErrNotFound", err)
	}
	if _, err := app.SoftDelete(ctx, tenant, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("SoftDelete(unknown) err = %v, want ErrNotFound", err)
	}
}
