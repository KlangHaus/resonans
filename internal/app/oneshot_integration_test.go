//go:build integration

// The one-shot commands and Run against a real Postgres. The one-shots only
// log what they did, so each cell measures the EFFECT in the database: rows
// that exist, a schema that is there, a dry run that changed no byte.
package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/KlangHaus/resonans/internal/persistence"
	"github.com/KlangHaus/resonans/internal/testpg"
)

// oneShotDB points DATABASE_URL at this build's own database, as the migrator
// (the role the one-shots run as in production), and returns a pool on it.
func oneShotDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, scopedDSN := testpg.PoolDSN(t)
	t.Setenv("NODE_ENV", "development")
	t.Setenv("DATABASE_URL", scopedDSN)
	t.Setenv("LOG_LEVEL", "error")
	return pool
}

func countDocs(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`select count(*) from resonans_app.documents where tenant_id = $1`, tenant).Scan(&n); err != nil {
		t.Fatalf("count documents: %v", err)
	}
	return n
}

// bodiesDigest is one hash over every stored body, so "the dry run changed
// nothing" is a comparison of two values rather than a belief.
func bodiesDigest(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var d string
	if err := pool.QueryRow(context.Background(), `
		select md5(coalesce(string_agg(id::text || body::text, '|' order by id), ''))
		  || md5(coalesce((select string_agg(id::text || body::text, '|' order by id) from resonans_app.document_versions), ''))
		from resonans_app.documents`).Scan(&d); err != nil {
		t.Fatalf("digest: %v", err)
	}
	return d
}

func TestOneShotsAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := oneShotDB(t)

	if err := Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := Migrate(ctx); err != nil {
		t.Fatalf("Migrate again (must be idempotent): %v", err)
	}
	var tables int
	if err := pool.QueryRow(ctx, `select count(*) from information_schema.tables
		where table_schema = 'resonans_app' and table_name in ('documents','assets','tenant_settings')`).Scan(&tables); err != nil || tables != 3 {
		t.Fatalf("after Migrate, %d of the 3 core tables exist (%v)", tables, err)
	}

	tenant := uuid.New()
	t.Setenv("RESONANS_SEED_TENANT", tenant.String())
	if n := countDocs(t, pool, tenant); n != 0 {
		t.Fatalf("control: the tenant has %d documents before the seed", n)
	}
	if err := Seed(ctx); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	seeded := countDocs(t, pool, tenant)
	if seeded == 0 {
		t.Fatal("Seed returned nil and wrote no document for the tenant")
	}
	if err := Seed(ctx); err != nil {
		t.Fatalf("re-Seed: %v", err)
	}
	if n := countDocs(t, pool, tenant); n != seeded {
		t.Fatalf("re-Seed changed the document count %d -> %d", seeded, n)
	}

	if err := SlugAudit(ctx); err != nil {
		t.Fatalf("SlugAudit: %v", err)
	}
	// A row the backfill MUST change, planted here rather than borrowed from the
	// seed: the seed's own dirt (<cite>, ClickUp 1amk32fkxbq) is due to be fixed,
	// and a dry-run cell that leaned on it would then stop telling a dry run from
	// an apply without failing.
	if _, err := pool.Exec(ctx, `insert into resonans_app.documents (tenant_id, type, slug, title, status, body)
		values ($1, 'page', 'planted', 'Planted', 'draft', $2)`, tenant,
		`{"body":[{"_type":"rich_text","value":{"da":"<p>ok</p><script>alert(1)</script>"}}]}`); err != nil {
		t.Fatalf("plant a dirty row: %v", err)
	}
	before := bodiesDigest(t, pool)
	if err := SanitizeBackfill(ctx, false); err != nil {
		t.Fatalf("SanitizeBackfill dry run: %v", err)
	}
	if after := bodiesDigest(t, pool); after != before {
		t.Fatal("a DRY-RUN backfill changed stored bodies")
	}
	if err := SanitizeBackfill(ctx, true); err != nil {
		t.Fatalf("SanitizeBackfill apply: %v", err)
	}
	applied := bodiesDigest(t, pool)
	if applied == before {
		t.Fatal("applying the backfill changed nothing, though a <script> row was planted")
	}
	var planted string
	if err := pool.QueryRow(ctx, `select body::text from resonans_app.documents where slug = 'planted'`).Scan(&planted); err != nil ||
		strings.Contains(planted, "<script") || !strings.Contains(planted, "<p>ok</p>") {
		t.Fatalf("planted row after apply = %s (%v); want the script gone and the paragraph kept", planted, err)
	}
	if err := SanitizeBackfill(ctx, true); err != nil {
		t.Fatalf("SanitizeBackfill second apply: %v", err)
	}
	if again := bodiesDigest(t, pool); again != applied {
		t.Fatal("a second apply changed bodies again: the backfill does not converge")
	}
}

// prodAuth makes Run boot in PRODUCTION mode with real JWT auth: a local JWKS
// (plain http, so the insecure-JWKS waiver is declared, as config demands) and
// a signer for tokens carrying the tenant and role claims.
func prodAuth(t *testing.T) (sign func(tenant, role string) string) {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := jwk.FromRaw(raw)
	_ = priv.Set(jwk.KeyIDKey, "app-it")
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256)
	pub, _ := jwk.FromRaw(raw.PublicKey)
	_ = pub.Set(jwk.KeyIDKey, "app-it")
	_ = pub.Set(jwk.AlgorithmKey, jwa.RS256)
	set := jwk.NewSet()
	_ = set.AddKey(pub)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(set)
	}))
	t.Cleanup(srv.Close)

	const issuer, audience = "https://auth.it.example", "https://cms.it.example"
	for k, v := range map[string]string{
		"NODE_ENV": "production", "RESONANS_DEV_AUTH": "",
		"RESONANS_JWT_ISSUER": issuer, "RESONANS_JWKS_URL": srv.URL, "RESONANS_ALLOW_INSECURE_JWKS": "1",
		"RESONANS_JWT_AUDIENCE": audience, "RESONANS_DELIVERY_TOKEN": "delivery-token",
		"RESONANS_PREVIEW_SECRET": "preview-secret-of-sufficient-length-0123456789",
		"RESONANS_VIDEO_PROVIDER": "",
	} {
		t.Setenv(k, v)
	}
	return func(tenant, role string) string {
		tok, err := jwt.NewBuilder().Issuer(issuer).Audience([]string{audience}).Subject("editor-1").
			IssuedAt(time.Now()).Expiration(time.Now().Add(time.Hour)).
			Claim("org", tenant).Claim("resonans_role", role).Build()
		if err != nil {
			t.Fatal(err)
		}
		signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, priv))
		if err != nil {
			t.Fatal(err)
		}
		return string(signed)
	}
}

// 🔴 The DB-role guard (ClickUp 1amk32fkxc5), over the SAME seeded rows:
//
//   - CONTROL, dev mode as the superuser the CI database gives: another tenant
//     lists every document. The stores' queries carry no tenant_id, so this is
//     what a bypassing role does to isolation; without this control the
//     refusals below could be refusing a harmless thing.
//   - production as that superuser: refused, naming the role.
//   - production as a role with ONLY bypassrls: refused. The check is
//     `rolsuper or rolbypassrls`, and each half gets its own case.
//   - production as the app role (no bypassrls): serves, isolates, needs a token.
func TestRunRefusesARoleThatBypassesRLSInProduction(t *testing.T) {
	ctx := context.Background()
	base, _ := devEnv(t)
	pool, scopedDSN := testpg.PoolDSN(t)
	t.Setenv("LOG_LEVEL", "error")

	// Migrate and seed as the migrator, the way production deploys.
	t.Setenv("NODE_ENV", "development")
	t.Setenv("DATABASE_URL", scopedDSN)
	tenant := uuid.New()
	t.Setenv("RESONANS_SEED_TENANT", tenant.String())
	if err := Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := Seed(ctx); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	want := countDocs(t, pool, tenant)
	appDSN := testpg.AppPool(t, pool, scopedDSN, "app_it_app").Config().ConnString()
	bypassPool := testpg.AppPool(t, pool, scopedDSN, "app_it_bypass")
	if _, err := pool.Exec(ctx, "alter role app_it_bypass_"+testpg.Scope(t)+" bypassrls"); err != nil {
		t.Fatalf("give the probe role bypassrls: %v", err)
	}
	bypassDSN := bypassPool.Config().ConnString()
	// The CI user has BOTH attributes, so it cannot tell the two halves of the
	// check apart; this role is a superuser WITHOUT bypassrls.
	superPool := testpg.AppPool(t, pool, scopedDSN, "app_it_super")
	if _, err := pool.Exec(ctx, "alter role app_it_super_"+testpg.Scope(t)+" superuser nobypassrls"); err != nil {
		t.Fatalf("make the probe role a superuser: %v", err)
	}
	superDSN := superPool.Config().ConnString()
	var super, bypass bool
	if err := pool.QueryRow(ctx, `select rolsuper, rolbypassrls from pg_roles where rolname = $1`,
		"app_it_super_"+testpg.Scope(t)).Scan(&super, &bypass); err != nil || !super || bypass {
		t.Fatalf("control: the superuser-only role has rolsuper=%v rolbypassrls=%v (%v); want true/false", super, bypass, err)
	}

	countFor := func(header map[string]string) (int, int) {
		code, body := fetch(t, http.MethodGet, base+"/api/v1/documents", header)
		var resp struct {
			Documents []map[string]any `json:"documents"`
		}
		_ = json.Unmarshal([]byte(body), &resp)
		return code, len(resp.Documents)
	}

	// CONTROL: dev as the superuser boots (with a warning) and leaks.
	stop := start(t, ctx, base)
	if _, n := countFor(map[string]string{"X-Dev-Tenant": uuid.NewString()}); n != want {
		t.Fatalf("control: as the superuser another tenant listed %d documents, want all %d — "+
			"the refusals below would not be shown to matter", n, want)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run after cancel = %v", err)
	}

	sign := prodAuth(t)
	for _, c := range []struct{ name, dsn, role string }{
		{"the CI superuser (both attributes)", scopedDSN, "ci"},
		{"superuser only", superDSN, "app_it_super_" + testpg.Scope(t)},
		{"bypassrls only", bypassDSN, "app_it_bypass_" + testpg.Scope(t)},
	} {
		t.Setenv("DATABASE_URL", c.dsn)
		runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := Run(runCtx)
		cancel()
		if !errors.Is(err, persistence.ErrRoleBypassesRLS) || !strings.Contains(err.Error(), c.role) {
			t.Errorf("production as %s: Run = %v; want the refusal naming role %q", c.name, err, c.role)
		}
	}

	t.Setenv("DATABASE_URL", appDSN)
	stop = start(t, ctx, base)
	bearer := func(tid string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + sign(tid, "viewer")}
	}
	if code, n := countFor(bearer(tenant.String())); code != http.StatusOK || n != want {
		t.Errorf("app role, own tenant: %d with %d documents; want 200 and the %d seeded", code, n, want)
	}
	if _, n := countFor(bearer(uuid.NewString())); n != 0 {
		t.Errorf("app role, another tenant listed %d documents, want 0", n)
	}
	if code, _ := fetch(t, http.MethodGet, base+"/api/v1/documents", nil); code != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", code)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run after cancel = %v", err)
	}

	// A check that cannot measure is not a pass: with pg_roles unreadable in this
	// database, production refuses rather than serving as an unknown role. Last,
	// because it changes the catalog for every role here (the database is
	// dropped after the test).
	if _, err := pool.Exec(ctx, "revoke select on pg_catalog.pg_roles from public"); err != nil {
		t.Fatalf("hide pg_roles: %v", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := Run(runCtx); err == nil || !strings.Contains(err.Error(), "check the database role") {
		t.Errorf("production with the role unreadable: Run = %v; want a refusal, not a boot", err)
	}
}

// Dev with a database (the Tilt path) migrates at boot: a fresh database has
// the schema once Run serves, with no `resonans migrate` beforehand.
func TestRunInDevMigratesAtBoot(t *testing.T) {
	ctx := context.Background()
	base, _ := devEnv(t)
	pool, scopedDSN := testpg.PoolDSN(t)
	t.Setenv("DATABASE_URL", scopedDSN)
	var before int
	_ = pool.QueryRow(ctx, `select count(*) from information_schema.tables where table_schema = 'resonans_app'`).Scan(&before)
	if before != 0 {
		t.Fatalf("control: a fresh database already has %d resonans_app tables", before)
	}
	stop := start(t, ctx, base)
	defer func() { _ = stop() }()
	var after int
	if err := pool.QueryRow(ctx, `select count(*) from information_schema.tables where table_schema = 'resonans_app'`).Scan(&after); err != nil || after == 0 {
		t.Fatalf("after Run in dev, resonans_app has %d tables (%v); want the migrated schema", after, err)
	}
	if code, _ := fetch(t, http.MethodGet, base+"/api/v1/documents", map[string]string{"X-Dev-Tenant": uuid.NewString()}); code != http.StatusOK {
		t.Errorf("listing on the migrated database = %d, want 200", code)
	}
}

// 🔴 RESONANS_DEV_AUTH=1 is ignored for the editor once JWT is configured: the
// X-Dev-Tenant header must not let anyone in beside the token check.
func TestDevAuthIsIgnoredWhenJWTIsConfigured(t *testing.T) {
	ctx := context.Background()
	base, _ := devEnv(t)
	sign := prodAuth(t)
	t.Setenv("NODE_ENV", "development") // dev auth is refused outright in production
	t.Setenv("RESONANS_DEV_AUTH", "1")
	t.Setenv("DATABASE_URL", "")
	stop := start(t, ctx, base)
	defer func() { _ = stop() }()
	tenant := uuid.NewString()
	if code, _ := fetch(t, http.MethodGet, base+"/api/v1/documents", map[string]string{"X-Dev-Tenant": tenant}); code != http.StatusUnauthorized {
		t.Errorf("X-Dev-Tenant with JWT configured = %d, want 401", code)
	}
	if code, _ := fetch(t, http.MethodGet, base+"/api/v1/documents", map[string]string{"Authorization": "Bearer " + sign(tenant, "viewer")}); code != http.StatusOK {
		t.Errorf("control: a valid token = %d, want 200", code)
	}
}

// Every entry point fails against a database it cannot reach: none may report
// success over zero rows it never read.
func TestEntryPointsFailOnAnUnreachableDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t.Setenv("NODE_ENV", "development")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("DATABASE_URL", "postgres://nobody:x@127.0.0.1:1/none?sslmode=disable&connect_timeout=2")
	t.Setenv("RESONANS_SEED_TENANT", uuid.NewString())
	for name, run := range map[string]func() error{
		"migrate":           func() error { return Migrate(ctx) },
		"seed":              func() error { return Seed(ctx) },
		"sanitize-backfill": func() error { return SanitizeBackfill(ctx, false) },
		"slug-audit":        func() error { return SlugAudit(ctx) },
	} {
		if err := run(); err == nil {
			t.Errorf("%s against an unreachable database returned nil", name)
		}
	}
	devEnv(t)
	t.Setenv("DATABASE_URL", "postgres://nobody:x@127.0.0.1:1/none?sslmode=disable&connect_timeout=2")
	if err := Run(ctx); err == nil {
		t.Error("Run against an unreachable database returned nil")
	}
}

// A configuration that does not load stops every entry point at the config,
// before anything else runs.
func TestEntryPointsFailOnABadConfig(t *testing.T) {
	ctx := context.Background()
	t.Setenv("PORT", "-1")
	for name, run := range map[string]func() error{
		"run": func() error { return Run(ctx) }, "migrate": func() error { return Migrate(ctx) },
		"seed": func() error { return Seed(ctx) }, "sanitize-backfill": func() error { return SanitizeBackfill(ctx, false) },
		"slug-audit": func() error { return SlugAudit(ctx) },
	} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "load config") {
			t.Errorf("%s with PORT=-1 = %v, want a load config error", name, err)
		}
	}
}
