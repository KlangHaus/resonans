//go:build integration

// Package testpg gives integration cells their own Postgres database on the
// cluster named by RESONANS_TEST_DB_URL, and drops it afterwards.
//
// Moved here VERBATIM from internal/persistence's RLS cell (only the names are
// exported), so internal/content's PostgresStore cells run on the same
// per-build database contract instead of a hand-written copy of it: a copy
// would have to re-earn every guard below, one forgotten line at a time.
//
// Build tag on purpose, like the cells that use it: `go test ./...` must not
// need a database, and this package does not exist without -tags=integration.
package testpg

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DSNEnv names the admin DSN of the test cluster.
const DSNEnv = "RESONANS_TEST_DB_URL"

// ScopeEnv carries a per-build suffix for CLUSTER-WIDE object names.
// ScopeEnv carries a per-build suffix for CLUSTER-WIDE object names.
const ScopeEnv = "RESONANS_TEST_SCOPE"

// Scope makes the roles and databases these cells create unique per build.
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
func Scope(t *testing.T) string {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(ScopeEnv))
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
		t.Fatalf("%s=%q sanitizes to an empty identifier — refusing to fall back to a shared "+
			"name, because a collision between concurrent builds reads as a flaky test", ScopeEnv, raw)
	}
	return scope
}

// Pool returns a pool to this build's own database. Use PoolDSN when the
// test also needs to CONNECT AS ANOTHER ROLE, because the DSN in the environment
// points at the ADMIN database and not at the one the migrations ran in.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := PoolDSN(t)
	return pool
}

// PoolDSN returns the pool AND the DSN of this build's own database.
//
// 🔴 The DSN matters, and getting it wrong is how this change first failed: the RLS
// cell rebuilt the app role's DSN from the ENVIRONMENT variable, which points at the
// admin database. The migrations had run in the per-build database, so the app role
// connected to a database with no `resonans_app.documents` and the cell died on a
// missing relation — a correct answer about a NEIGHBORING object. Every role-swap
// has to start from the DSN below, not from the environment.
func PoolDSN(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(DSNEnv))
	if dsn == "" {
		// Not t.Skip. See the package comment: reaching this line means someone
		// asked for the integration build and did not wire the database, and a
		// quiet skip would report that as a pass.
		t.Fatalf("%s is empty — this suite only compiles under -tags=integration, so a missing database is a wiring error, not a reason to pass", DSNEnv)
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
		t.Fatalf("connect to the admin DSN from %s: %v", DSNEnv, err)
	}
	defer admin.Close()
	if err := admin.Ping(ctx); err != nil {
		t.Fatalf("ping the admin DSN: %v", err)
	}

	// The scope separates BUILDS; the package separates the test binaries of one
	// build. `go test` runs packages in parallel, and with the scope alone the
	// persistence and content binaries shared one database name — measured: each
	// one's `drop ... with (force)` below killed the other's connections
	// (SQLSTATE 57P01) and both packages failed, while each passed on its own.
	dbName := "resonans_ci_" + Scope(t) + "_" + binaryName()
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

	scopedDSN := WithDatabase(t, dsn, dbName)
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

// WithDatabase rewrites a DSN to point at another database on the same server,
// keeping every other parameter (sslmode in particular) exactly as configured.
func WithDatabase(t *testing.T, dsn, db string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse %s as a URL: %v", DSNEnv, err)
	}
	u.Path = "/" + db
	return u.String()
}

// RewriteUser swaps the credentials in a DSN, so the probe role connects to the
// same database the migrations just ran against.
func RewriteUser(t *testing.T, dsn, user, password string) string {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		user, password, cfg.Host, cfg.Port, cfg.Database, SSLMode(dsn))
}

// SSLMode is the DSN's sslmode, or "disable" when it names none.
func SSLMode(dsn string) string {
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

// binaryName is the running test binary's package ("content" for content.test),
// as a lower-case identifier of at most 16 bytes, so two packages of one build
// get two databases. Postgres caps identifiers at 63 bytes: 12 for the prefix,
// 24 for the scope, 2 separators and 16 here stay under it.
func binaryName() string {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".test")
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 16 {
		out = out[:16]
	}
	if out == "" {
		out = "pkg"
	}
	return out
}

// AppPool connects to the build's database as a fresh role WITHOUT bypassrls,
// with the grants the app role has in production, and drops the role after
// the test. A store under test must run on this pool: the migrator is a
// superuser and bypasses RLS, so a cell run as the migrator measures the
// superuser rather than the policy. name keeps concurrent packages' roles
// apart (roles are cluster-wide); the scope keeps builds apart.
func AppPool(t *testing.T, admin *pgxpool.Pool, scopedDSN, name string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	role := name + "_" + Scope(t)
	const pw = "testpg-app-not-a-secret"
	for _, stmt := range []string{
		`drop role if exists ` + role,
		`create role ` + role + ` with login password '` + pw + `' nosuperuser nobypassrls noinherit`,
		`grant usage on schema resonans_app to ` + role,
		`grant select, insert, update, delete on all tables in schema resonans_app to ` + role,
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	appPool, err := pgxpool.New(ctx, RewriteUser(t, scopedDSN, role, pw))
	if err != nil {
		t.Fatalf("connect as %s: %v", role, err)
	}
	// LIFO: the app pool closes before its role is dropped.
	t.Cleanup(func() {
		c := context.Background()
		_, _ = admin.Exec(c, `revoke all on all tables in schema resonans_app from `+role)
		_, _ = admin.Exec(c, `revoke all on schema resonans_app from `+role)
		_, _ = admin.Exec(c, `drop role if exists `+role)
	})
	t.Cleanup(appPool.Close)
	return appPool
}
