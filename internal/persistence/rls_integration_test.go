//go:build integration

// Package persistence's integration cell: the half of tenant isolation that a
// unit test cannot reach.
//
// Isolation has two halves. The ROLE half — the server connects as a role that
// cannot bypass RLS — holds once and stays true. The TABLE half is re-decided by
// every migration, and a migration that forgets `enable`/`force` is INVISIBLE to
// the role half: the role stays correct while rows read across tenants. This file
// is the half that can regress, and it exists because the guard that catches it
// otherwise runs only when somebody remembers to run it.
//
// Build tag on purpose: `go test ./...` must not need a database. The only way
// to compile this is to ask for it, which is why a missing DSN below is a
// FAILURE and not a skip — a skipped test is indistinguishable from a passing
// one on a check-run, and that is exactly how a green means nothing.
package persistence

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testDSNEnv = "RESONANS_TEST_DB_URL"

// scopeEnv carries a per-build suffix for CLUSTER-WIDE object names.
const scopeEnv = "RESONANS_TEST_SCOPE"

// testScope makes the roles and databases these cells create unique per build.
//
// 🔴 WHY: roles and databases are CLUSTER objects, not database objects. The CI
// postgres is shared by every repo's agents, so two concurrent builds using a fixed
// name would create, alter and drop each other's roles — and the failure would look
// like a flaky test rather than a collision. infra's contract for KH_TEST_PG_DSN
// states it outright: name your objects after BUILD_TAG and drop them afterwards.
//
// Deterministic per build rather than random: a RETRY of the same build reuses the
// same names, so the `drop ... if exists` in each cell cleans up after a killed run.
// A random suffix would instead leave a fresh orphan behind on every crash.
//
// Empty is a FAILURE, not a default. A cell that silently falls back to a shared
// name is the collision this function exists to prevent.
func testScope(t *testing.T) string {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(scopeEnv))
	if raw == "" {
		raw = "local"
	}
	var b strings.Builder
	for _, r := range strings.ToLower(raw) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	// Collapse runs of '_' so a BUILD_TAG full of separators does not eat the budget.
	for strings.Contains(b.String(), "__") {
		s := strings.ReplaceAll(b.String(), "__", "_")
		b.Reset()
		b.WriteString(s)
	}
	scope := strings.Trim(b.String(), "_")
	// Postgres identifiers are capped at 63 bytes and the callers prefix ~25, so
	// leave room rather than discovering the truncation as a name collision.
	if len(scope) > 24 {
		scope = scope[:24]
	}
	if scope == "" {
		t.Fatalf("%s=%q sanitises to an empty identifier — refusing to fall back to a shared "+
			"name, because a collision between concurrent builds reads as a flaky test", scopeEnv, raw)
	}
	return scope
}

// testPool returns a pool to this build's own database. Use testPoolDSN when the
// test also needs to CONNECT AS ANOTHER ROLE, because the DSN in the environment
// points at the ADMIN database and not at the one the migrations ran in.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := testPoolDSN(t)
	return pool
}

// testPoolDSN returns the pool AND the DSN of this build's own database.
//
// 🔴 The DSN matters, and getting it wrong is how this change first failed: the RLS
// cell rebuilt the app role's DSN from the ENVIRONMENT variable, which points at the
// admin database. The migrations had run in the per-build database, so the app role
// connected to a database with no `resonans_app.documents` and the cell died on a
// missing relation — a correct answer about a NEIGHBOURING object. Every role-swap
// has to start from the DSN below, not from the environment.
func testPoolDSN(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(testDSNEnv))
	if dsn == "" {
		// Not t.Skip. See the package comment: reaching this line means someone
		// asked for the integration build and did not wire the database, and a
		// quiet skip would report that as a pass.
		t.Fatalf("%s is empty — this suite only compiles under -tags=integration, so a missing database is a wiring error, not a reason to pass", testDSNEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 🔴 EACH BUILD GETS ITS OWN DATABASE, and this is infra's contract for the
	// shared CI postgres rather than our preference: "create your own database named
	// after BUILD_TAG, and drop it afterwards", because two concurrent builds of the
	// same repo would otherwise share a schema and overwrite each other's rows.
	//
	// Measured: two concurrent runs with their own database and scope both pass; two
	// sharing both go one RED and one green. So the isolation is what carries the
	// passing case, not an accident of timing.
	//
	// Done HERE rather than in CI config because CI runners may have no docker
	// (the former Jenkins agents had none, by [sikkerhed]'s veto) and psql is not
	// something this repo may assume is on the image. Doing it in Go means the same code path runs locally, so the
	// contract is measurable on a laptop instead of only on an agent.
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to the admin DSN from %s: %v", testDSNEnv, err)
	}
	defer admin.Close()
	if err := admin.Ping(ctx); err != nil {
		t.Fatalf("ping the admin DSN: %v", err)
	}

	dbName := "resonans_ci_" + testScope(t)
	// Drop first: a killed run leaves the database behind, and because the name is
	// deterministic per build a retry must be able to reclaim it rather than fail on
	// "already exists".
	if _, err := admin.Exec(ctx, "drop database if exists "+dbName+" with (force)"); err != nil {
		t.Fatalf("reclaim %s: %v", dbName, err)
	}
	if _, err := admin.Exec(ctx, "create database "+dbName); err != nil {
		t.Fatalf("create %s: %v — the DSN's role must be able to CREATE DATABASE, which is "+
			"what infra's contract for this facility provides", dbName, err)
	}

	scopedDSN := withDatabase(t, dsn, dbName)
	pool, err := pgxpool.New(ctx, scopedDSN)
	if err != nil {
		t.Fatalf("connect to %s: %v", dbName, err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping %s: %v", dbName, err)
	}
	// LIFO: the pool closes before the drop runs, because `drop database` fails while
	// a connection is open — and `with (force)` on the reclaim above is the belt for
	// the case where something else is still attached.
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		a, err := pgxpool.New(c, dsn)
		if err != nil {
			t.Logf("could not reopen the admin DSN to drop %s: %v — it will be reclaimed by the "+
				"next run with this scope", dbName, err)
			return
		}
		defer a.Close()
		if _, err := a.Exec(c, "drop database if exists "+dbName+" with (force)"); err != nil {
			t.Logf("could not drop %s: %v — it will be reclaimed by the next run with this scope",
				dbName, err)
		}
	})
	t.Cleanup(pool.Close)
	return pool, scopedDSN
}

// withDatabase rewrites a DSN to point at another database on the same server,
// keeping every other parameter (sslmode in particular) exactly as configured.
func withDatabase(t *testing.T, dsn, db string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse %s as a URL: %v", testDSNEnv, err)
	}
	u.Path = "/" + db
	return u.String()
}

// coverageSQL runs the file we actually ship, rather than a copy of its query.
// Two instruments with the same description are one instrument — and if the test
// paraphrased the file, the shipped file could rot while the test stayed green.
func coverageSQL(t *testing.T) string {
	t.Helper()
	// From internal/persistence to the repository root.
	path := filepath.Join("..", "..", "deploy", "assert-rls-coverage.sql")
	b, err := os.ReadFile(path) //nolint:gosec // fixed path inside the repository
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sql := string(b)
	// Strip the psql-only meta-command; pgx speaks SQL, not psql.
	sql = strings.ReplaceAll(sql, `\set ON_ERROR_STOP on`, "")
	if !strings.Contains(sql, "relforcerowsecurity") {
		t.Fatalf("%s does not look like the coverage assertion — refusing to report on a file whose content I cannot recognise", path)
	}
	return sql
}

func runCoverage(ctx context.Context, pool *pgxpool.Pool, sql string) error {
	_, err := pool.Exec(ctx, sql)
	return err
}

func TestRLSCoverageAssertionFiresOnATableThatForgot(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if _, err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sql := coverageSQL(t)

	// 1. The real schema passes. Without this, everything below could be a
	//    database that simply refuses every query.
	if err := runCoverage(ctx, pool, sql); err != nil {
		t.Fatalf("the migrated schema does not satisfy its own coverage assertion: %v", err)
	}

	// 2. THE CANARY. Plant a table that carries a tenant_id and forgot RLS, and
	//    require the assertion to name it. A guard that has never been seen to
	//    fire is armed against nothing.
	const canary = "rls_canary_forgot"
	if _, err := pool.Exec(ctx, `create table resonans_app.`+canary+` (tenant_id uuid not null, body text)`); err != nil {
		t.Fatalf("plant canary: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `drop table if exists resonans_app.`+canary)
	})

	err := runCoverage(ctx, pool, sql)
	if err == nil {
		t.Fatal("the coverage assertion PASSED with a tenant table that has no RLS — the guard cannot fire, so its green says nothing")
	}
	if !strings.Contains(err.Error(), canary) {
		t.Fatalf("the assertion failed but did not name the offending table: %v", err)
	}

	// 3. Removing the canary makes it pass again, so the failure was caused by
	//    the canary and not by something the canary happened to coincide with.
	if _, err := pool.Exec(ctx, `drop table resonans_app.`+canary); err != nil {
		t.Fatalf("drop canary: %v", err)
	}
	if err := runCoverage(ctx, pool, sql); err != nil {
		t.Fatalf("assertion still fails after the canary was removed: %v", err)
	}
}

// TestRLSIsEnforcedForANonBypassingRole measures the half that holds once: a role
// without BYPASSRLS sees only its own tenant, and a superuser sees everything.
// The second half is the negative control — without it, "the app role saw one
// row" could equally mean the table had one row.
func TestRLSIsEnforcedForANonBypassingRole(t *testing.T) {
	ctx := context.Background()
	pool, scopedDSN := testPoolDSN(t)
	if _, err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	role := "rls_probe_app_" + testScope(t)
	const pw = "rls-probe-not-a-secret"
	for _, stmt := range []string{
		`drop role if exists ` + role,
		`create role ` + role + ` with login password '` + pw + `' nosuperuser nobypassrls noinherit`,
		`grant usage on schema resonans_app to ` + role,
		`grant select, insert, update, delete on all tables in schema resonans_app to ` + role,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `revoke all on all tables in schema resonans_app from `+role)
		_, _ = pool.Exec(c, `revoke all on schema resonans_app from `+role)
		_, _ = pool.Exec(c, `drop role if exists `+role)
	})

	tenantA, tenantB := uuid.New(), uuid.New()
	for _, tid := range []uuid.UUID{tenantA, tenantB} {
		if _, err := pool.Exec(ctx,
			`insert into resonans_app.documents (tenant_id, type, slug, title, status, body)
			 values ($1, 'page', $2, 'T', 'draft', '{}')`, tid, "s-"+tid.String()[:8]); err != nil {
			t.Fatalf("seed %s: %v", tid, err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`delete from resonans_app.documents where tenant_id = any($1)`, []uuid.UUID{tenantA, tenantB})
	})

	// NEGATIVE CONTROL, on the SAME rows: the migrator is a superuser and
	// therefore bypasses RLS. If this did not see both, the "app sees one" result
	// below would be about the data rather than about the policy.
	var asMigrator int
	if err := pool.QueryRow(ctx,
		`select count(*) from resonans_app.documents where tenant_id = any($1)`,
		[]uuid.UUID{tenantA, tenantB}).Scan(&asMigrator); err != nil {
		t.Fatalf("count as migrator: %v", err)
	}
	if asMigrator != 2 {
		t.Fatalf("migrator sees %d of the 2 seeded rows — the control is broken, so nothing below can be trusted", asMigrator)
	}

	// The build's own database, NOT os.Getenv(testDSNEnv) — see testPoolDSN.
	appDSN := rewriteUser(t, scopedDSN, role, pw)
	appPool, err := pgxpool.New(ctx, appDSN)
	if err != nil {
		t.Fatalf("connect as %s: %v", role, err)
	}
	defer appPool.Close()

	// With the tenant GUC set, exactly one of the two rows is visible.
	var seen int
	if err := withTenant(ctx, appPool, tenantA, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`select count(*) from resonans_app.documents where tenant_id = any($1)`,
			[]uuid.UUID{tenantA, tenantB}).Scan(&seen)
	}); err != nil {
		t.Fatalf("count as app role: %v", err)
	}
	if seen != 1 {
		t.Fatalf("the app role sees %d of 2 rows across two tenants, want 1", seen)
	}

	// With NO GUC the policy RAISES rather than returning an empty set, because
	// the migrations read it as `current_setting('app.tenant_id', false)` — the
	// non-optional form. An empty result is a plausible answer a caller could act
	// on; an error is not.
	//
	// 🔴 But the MESSAGE depends on the connection's history, which is worth
	// pinning because it would otherwise be discovered by whoever matches on it:
	//
	//   a connection that never set the GUC   -> unrecognized configuration parameter
	//   one that set it in an earlier tx      -> invalid input syntax for type uuid: ""
	//
	// `set_config(..., is_local=true)` DEFINES the placeholder for the whole
	// session; ending the transaction resets the value to the empty string rather
	// than to undefined. With a pool, both cases occur in normal operation. The
	// property that holds in both is "it errors and returns nothing", so that is
	// what is asserted — and both branches are exercised so neither can rot.
	for _, c := range []struct {
		name  string
		prime bool
		want  string
	}{
		{"a connection that never set the tenant", false, "unrecognized configuration parameter"},
		{"a connection that set it in an earlier transaction", true, `invalid input syntax for type uuid: ""`},
	} {
		t.Run(c.name, func(t *testing.T) {
			// A DEDICATED connection, not one from the pool. The pool's connection
			// has already run a tenant transaction by this point, so asking it for
			// an untouched session would be asking for something that does not
			// exist — and the subtest would fail for a reason unrelated to RLS.
			conn, err := pgx.Connect(ctx, appDSN)
			if err != nil {
				t.Fatalf("connect: %v", err)
			}
			defer func() { _ = conn.Close(context.Background()) }()
			if c.prime {
				if err := primeTenant(ctx, conn, tenantA); err != nil {
					t.Fatalf("prime the GUC: %v", err)
				}
			}
			var n int
			err = conn.QueryRow(ctx, `select count(*) from resonans_app.documents`).Scan(&n)
			if err == nil {
				t.Fatalf("returned %d rows instead of failing", n)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want it to contain %q", err, c.want)
			}
		})
	}

	// Escalation is refused.
	for _, stmt := range []string{
		`set role postgres`,
		`alter table resonans_app.documents disable row level security`,
	} {
		if _, err := appPool.Exec(ctx, stmt); err == nil {
			t.Fatalf("the app role was allowed to run %q", stmt)
		}
	}
}

// primeTenant runs one tenant-scoped transaction on a SPECIFIC connection, so a
// later query on that same session sees it after the GUC has been defined. Kept
// separate from withTenant because the distinction is the point: with a pool,
// "never set" and "set in an earlier transaction" are both normal states.
func primeTenant(ctx context.Context, conn *pgx.Conn, tenantID uuid.UUID) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `select set_config('app.tenant_id', $1, true)`, tenantID.String()); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	var n int
	if err := tx.QueryRow(ctx, `select count(*) from resonans_app.documents`).Scan(&n); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// withTenant mirrors what the stores do: set the GUC inside the transaction that
// reads, so the cell shares the production path's SHAPE and not only its code.
func withTenant(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select set_config('app.tenant_id', $1, true)`, tenantID.String()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// rewriteUser swaps the credentials in a DSN, so the probe role connects to the
// same database the migrations just ran against.
func rewriteUser(t *testing.T, dsn, user, password string) string {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		user, password, cfg.Host, cfg.Port, cfg.Database, sslMode(dsn))
}

func sslMode(dsn string) string {
	if strings.Contains(dsn, "sslmode=") {
		i := strings.Index(dsn, "sslmode=") + len("sslmode=")
		rest := dsn[i:]
		if j := strings.IndexAny(rest, "&"); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return "disable"
}
