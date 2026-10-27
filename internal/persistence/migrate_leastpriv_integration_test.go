//go:build integration

package persistence

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KlangHaus/resonans/migrations"
)

// TestMigrateAsLeastPrivilegeMigrator measures that migrate runs as the role a
// least-privilege deploy guide actually creates: LOGIN NOSUPERUSER NOCREATEDB
// NOCREATEROLE, on a database it does not own, with resonans_app pre-created
// for it by a superuser. That role has no CREATE on the database, and postgres
// checks that privilege on `create schema if not exists` BEFORE the existence
// check — so the naive form is refused with 42501 while the schema is already
// there. This cell exists because that refusal is what a self-hoster meets on
// their first deploy, and nothing in the unit suite can see it.
//
// The second arm is load-bearing, not decoration. Arm one (schema present →
// migrate succeeds) would pass just as well if the role secretly held CREATE
// on the database, which would make the whole cell vacuous. Arm two drops the
// schema and requires the SAME role on the SAME database to be refused, which
// is the positive control on the role being least-privilege at all.
func TestMigrateAsLeastPrivilegeMigrator(t *testing.T) {
	admin := testPool(t) // t.Fatalf, not t.Skip, when the DSN is unwired
	ctx := context.Background()

	scope := testScope(t)
	role := "resonans_lp_probe_" + scope
	db := "resonans_lp_probe_db_" + scope
	const pw = "probe-not-a-secret"

	// A dropped role/database from an earlier aborted run must not make this
	// pass or fail for the wrong reason.
	_, _ = admin.Exec(ctx, "drop database if exists "+db)
	_, _ = admin.Exec(ctx, "drop role if exists "+role)
	if _, err := admin.Exec(ctx, fmt.Sprintf(
		"create role %s login password '%s' nosuperuser nocreatedb nocreaterole", role, pw)); err != nil {
		t.Fatalf("create probe role: %v", err)
	}
	// Owned by the CONNECTING superuser, deliberately not by the probe role:
	// that ownership is the whole difference between a guide that works and one
	// that does not.
	if _, err := admin.Exec(ctx, "create database "+db); err != nil {
		t.Fatalf("create probe database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "drop database if exists "+db)
		_, _ = admin.Exec(ctx, "drop role if exists "+role)
	})

	adminOnProbe := connectAs(t, probeDSN(t, "", ""), db)
	defer func() {
		if err := adminOnProbe.Close(ctx); err != nil {
			t.Logf("close the superuser connection to the probe database: %v", err)
		}
	}()
	if _, err := adminOnProbe.Exec(ctx,
		"create schema resonans_app authorization "+role); err != nil {
		t.Fatalf("pre-create schema as superuser: %v", err)
	}
	for _, g := range []string{
		"grant usage on schema resonans_app to " + role,
		fmt.Sprintf("alter default privileges for role %s in schema resonans_app "+
			"grant select, insert, update, delete on tables to %s", role, role),
	} {
		if _, err := adminOnProbe.Exec(ctx, g); err != nil {
			t.Fatalf("grant %q: %v", g, err)
		}
	}

	probe, err := pgxpool.New(ctx, probeDSN(t, role, pw)+"/"+db)
	if err != nil {
		t.Fatalf("probe pool: %v", err)
	}
	defer probe.Close()

	// Arm 1 — the shape a deploy guide produces: schema present, no database
	// CREATE. This must run.
	applied, err := migrateFS(ctx, probe, migrations.FS)
	if err != nil {
		t.Fatalf("migrate as least-privilege migrator with the schema present: %v\n"+
			"this is the first thing a self-hoster runs; it must not need CREATE on the database", err)
	}
	if len(applied) == 0 {
		t.Fatalf("migrate applied nothing on a fresh database — the cell measured an already-migrated object")
	}
	var tables int
	if err := adminOnProbe.QueryRow(ctx,
		"select count(*) from information_schema.tables where table_schema = 'resonans_app'").
		Scan(&tables); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tables == 0 {
		t.Fatalf("migrate reported %d applied migrations and left 0 tables — it answered about another database", len(applied))
	}

	// Arm 2 — the positive control on the role being least-privilege. Same role,
	// same database, schema gone: creating it must be REFUSED. If this arm
	// succeeds, the role held CREATE all along and arm 1 proved nothing.
	if _, err := adminOnProbe.Exec(ctx, "drop schema resonans_app cascade"); err != nil {
		t.Fatalf("drop schema for the control arm: %v", err)
	}
	probe.Reset()
	_, err = migrateFS(ctx, probe, migrations.FS)
	if err == nil {
		t.Fatalf("control arm: role %s created schema resonans_app on a database it does not own — "+
			"it is not least-privilege, so arm 1 above measured nothing", role)
	}
	if !strings.Contains(err.Error(), "42501") {
		t.Fatalf("control arm: expected SQLSTATE 42501 (insufficient_privilege), got %v", err)
	}
	// And the refusal has to tell the operator what to do about it, because this
	// error is the entire diagnostic they get from a failed deploy.
	for _, want := range []string{"authorization", "CREATE on the database"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal a self-hoster reads does not mention %q: %v", want, err)
		}
	}
}

// probeDSN rebuilds the test DSN with a different user (empty = keep the
// configured superuser) and without the database segment.
func probeDSN(t *testing.T, user, pass string) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(testDSNEnv))
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", testDSNEnv, err)
	}
	u, p := cfg.User, cfg.Password
	if user != "" {
		u, p = user, pass
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d", u, p, cfg.Host, cfg.Port)
}

func connectAs(t *testing.T, base, db string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(context.Background(), base+"/"+db)
	if err != nil {
		t.Fatalf("connect to %s: %v", db, err)
	}
	return c
}
