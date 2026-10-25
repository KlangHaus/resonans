//go:build integration

// The Postgres settings store against a real Postgres. Until these cells
// existed, no test called settings.NewPostgresStore — and its queries once
// lacked the resonans_app schema prefix, a 500 that only a database finds.
// Run as a role WITHOUT bypassrls (testpg.AppPool), like the other stores.
package settings

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/persistence"
	"github.com/KlangHaus/resonans/internal/testpg"
)

func settingsStores(t *testing.T) (app, migrator *PostgresStore) {
	t.Helper()
	pool, scopedDSN := testpg.PoolDSN(t)
	if _, err := persistence.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewPostgresStore(testpg.AppPool(t, pool, scopedDSN, "settings_it_app")), NewPostgresStore(pool)
}

func TestPostgresSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	app, _ := settingsStores(t)
	tenant := uuid.New()

	// No row yet: every type, no preview.
	if got, err := app.EnabledTypes(ctx, tenant); err != nil || len(got) != 0 {
		t.Fatalf("EnabledTypes(no row) = %v, %v; want empty", got, err)
	}
	if got, err := app.PreviewURL(ctx, tenant); err != nil || got != "" {
		t.Fatalf("PreviewURL(no row) = %q, %v; want empty", got, err)
	}

	// Each upsert writes only its own column. The row is created by the first
	// one either way, so each order must leave the other column intact.
	if err := app.SetEnabledTypes(ctx, tenant, []string{"post", "page"}); err != nil {
		t.Fatalf("SetEnabledTypes: %v", err)
	}
	if got, err := app.PreviewURL(ctx, tenant); err != nil || got != "" {
		t.Fatalf("PreviewURL after only the profile was set = %q, %v; want empty, no error", got, err)
	}
	const tpl = "https://site.example/preview/{type}/{slug}?token={token}"
	if err := app.SetPreviewURL(ctx, tenant, tpl); err != nil {
		t.Fatalf("SetPreviewURL: %v", err)
	}
	if got, _ := app.EnabledTypes(ctx, tenant); len(got) != 2 || got[0] != "post" || got[1] != "page" {
		t.Fatalf("EnabledTypes after setting the preview = %v; the profile was overwritten", got)
	}
	if got, _ := app.PreviewURL(ctx, tenant); got != tpl {
		t.Fatalf("PreviewURL = %q, want %q", got, tpl)
	}

	// Reset paths: nil re-enables everything; "" turns visual editing off.
	if err := app.SetEnabledTypes(ctx, tenant, nil); err != nil {
		t.Fatalf("SetEnabledTypes(nil): %v", err)
	}
	if got, _ := app.EnabledTypes(ctx, tenant); len(got) != 0 {
		t.Fatalf("EnabledTypes after reset = %v, want empty", got)
	}
	if err := app.SetPreviewURL(ctx, tenant, ""); err != nil {
		t.Fatalf("SetPreviewURL(\"\"): %v", err)
	}
	if got, _ := app.PreviewURL(ctx, tenant); got != "" {
		t.Fatalf("PreviewURL after reset = %q", got)
	}

	// A tenant whose row was created by the PREVIEW first keeps an empty profile.
	other := uuid.New()
	if err := app.SetPreviewURL(ctx, other, tpl); err != nil {
		t.Fatalf("SetPreviewURL(other): %v", err)
	}
	if got, err := app.EnabledTypes(ctx, other); err != nil || len(got) != 0 {
		t.Fatalf("EnabledTypes for a row created by the preview = %v, %v; want empty (all types)", got, err)
	}
}

func TestPostgresSettingsAreTenantScoped(t *testing.T) {
	ctx := context.Background()
	app, migrator := settingsStores(t)
	a, b := uuid.New(), uuid.New()
	if err := app.SetEnabledTypes(ctx, a, []string{"post"}); err != nil {
		t.Fatal(err)
	}
	if err := app.SetPreviewURL(ctx, a, "https://a.example/p?token={token}"); err != nil {
		t.Fatal(err)
	}
	if got, err := app.EnabledTypes(ctx, b); err != nil || len(got) != 0 {
		t.Errorf("tenant b's profile = %v, %v; want empty", got, err)
	}
	if got, err := app.PreviewURL(ctx, b); err != nil || got != "" {
		t.Errorf("tenant b's preview = %q, %v; want empty", got, err)
	}
	// B writing its own row must not touch A's.
	if err := app.SetEnabledTypes(ctx, b, []string{"page"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := app.EnabledTypes(ctx, a); len(got) != 1 || got[0] != "post" {
		t.Errorf("tenant a's profile after b wrote = %v", got)
	}
	// Control: the migrator reads the same rows, so the store is reading real data.
	if got, err := migrator.EnabledTypes(ctx, a); err != nil || len(got) != 1 {
		t.Errorf("control: the migrator reads a's profile as %v, %v", got, err)
	}
}

// 🔴 An unreadable table is an ERROR, never the "no row" default. For the
// profile, empty means every type is enabled, so a read failure reported as
// empty would open the whole registry: fail-open by a swallowed error. Each
// method is run against a role that has lost its grant on the table.
//
// Two mechanisms hold this, and the cell guards the PROPERTY, not one line:
// the read branch returns its error, and a failed statement aborts the
// transaction, so inTx's Commit fails too. Measured: swallowing the read error
// alone still reaches the caller as "commit unexpectedly resulted in
// rollback"; only with the commit error ignored as well does EnabledTypes
// return ([], nil) — and then this cell fails.
func TestPostgresSettingsFailClosedWhenTheTableIsUnreadable(t *testing.T) {
	ctx := context.Background()
	pool, scopedDSN := testpg.PoolDSN(t)
	if _, err := persistence.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := NewPostgresStore(testpg.AppPool(t, pool, scopedDSN, "settings_it_noread"))
	tenant := uuid.New()
	if err := app.SetEnabledTypes(ctx, tenant, []string{"post"}); err != nil {
		t.Fatalf("control: SetEnabledTypes with the grant: %v", err)
	}
	if _, err := pool.Exec(ctx, "revoke all on resonans_app.tenant_settings from settings_it_noread_"+testpg.Scope(t)); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if got, err := app.EnabledTypes(ctx, tenant); err == nil {
		t.Errorf("EnabledTypes on an unreadable table = %v, nil; want an error, not the all-types default", got)
	}
	if got, err := app.PreviewURL(ctx, tenant); err == nil {
		t.Errorf("PreviewURL on an unreadable table = %q, nil; want an error", got)
	}
	if err := app.SetEnabledTypes(ctx, tenant, []string{"page"}); err == nil {
		t.Error("SetEnabledTypes without the grant succeeded")
	}
	if err := app.SetPreviewURL(ctx, tenant, "https://x.example/p?token={token}"); err == nil {
		t.Error("SetPreviewURL without the grant succeeded")
	}

	// A canceled context fails before any query, and says so.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := NewPostgresStore(pool).EnabledTypes(canceled, tenant); err == nil {
		t.Error("EnabledTypes with a canceled context succeeded")
	}
}
