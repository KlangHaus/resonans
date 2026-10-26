//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KlangHaus/resonans/internal/testpg"
)

// ConnectedRole against each kind of role. The CI user has BOTH attributes, so
// a superuser without bypassrls and a bypassrls role without superuser each get
// their own role here, and the app role is the one that must answer false.
func TestConnectedRole(t *testing.T) {
	ctx := context.Background()
	pool, scopedDSN := testpg.PoolDSN(t)
	if _, err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	scope := testpg.Scope(t)
	app := testpg.AppPool(t, pool, scopedDSN, "role_it_app")
	super := testpg.AppPool(t, pool, scopedDSN, "role_it_super")
	bypass := testpg.AppPool(t, pool, scopedDSN, "role_it_bypass")
	for _, stmt := range []string{
		"alter role role_it_super_" + scope + " superuser nobypassrls",
		"alter role role_it_bypass_" + scope + " nosuperuser bypassrls",
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	for _, c := range []struct {
		name     string
		pool     *pgxpool.Pool
		role     string
		bypasses bool
	}{
		{"app role", app, "role_it_app_" + scope, false},
		{"superuser only", super, "role_it_super_" + scope, true},
		{"bypassrls only", bypass, "role_it_bypass_" + scope, true},
	} {
		role, bypasses, err := ConnectedRole(ctx, c.pool)
		if err != nil || role != c.role || bypasses != c.bypasses {
			t.Errorf("%s: ConnectedRole = %q, %v, %v; want %q, %v", c.name, role, bypasses, err, c.role, c.bypasses)
		}
	}
	// The admin (the CI superuser) is a bypassing role too.
	if _, bypasses, err := ConnectedRole(ctx, pool); err != nil || !bypasses {
		t.Errorf("admin: bypasses = %v (%v), want true", bypasses, err)
	}
	// A pool that cannot answer is an error, never "does not bypass".
	app.Close()
	if role, bypasses, err := ConnectedRole(ctx, app); err == nil {
		t.Errorf("closed pool: ConnectedRole = %q, %v, nil; want an error", role, bypasses)
	}
}
