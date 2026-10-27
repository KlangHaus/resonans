//go:build integration

// The backfill's Postgres store against a real Postgres, as the migrator
// (BYPASSRLS), which is the role it runs as in production. RLS does not fence
// it, so the two guards that matter live in its SQL and are tested here:
//
//   - the lost-write guard `and body = $4`: a save between scan and update
//     must not be overwritten with the old body cleaned ([sikkerhed]). The
//     existing unit cell tested this against a fake store that implements the
//     guard itself; the real guard is this SQL, which no test had run.
//   - the tenant in the WHERE: with RLS bypassed it is the only thing that
//     keeps an update on its own tenant's row.
package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KlangHaus/resonans/internal/persistence"
	"github.com/KlangHaus/resonans/internal/schema"
	"github.com/KlangHaus/resonans/internal/testpg"
)

const dirty = `{"body":[{"_type":"rich_text","format":"html","value":{"da":"<p>ok</p><img src=x onerror=alert(1)>"}}]}`

func migrated(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testpg.Pool(t)
	if _, err := persistence.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func insertDoc(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID, slug, body string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `insert into resonans_app.documents (tenant_id, type, slug, title, status, body)
		values ($1, 'page', $2, 'T', 'draft', $3) returning id`, tenant, slug, body).Scan(&id); err != nil {
		t.Fatalf("insert %s: %v", slug, err)
	}
	return id
}

func storedBody(t *testing.T, pool *pgxpool.Pool, table string, id uuid.UUID) string {
	t.Helper()
	var b string
	if err := pool.QueryRow(context.Background(), `select body::text from resonans_app.`+table+` where id = $1`, id).Scan(&b); err != nil {
		t.Fatalf("read %s %s: %v", table, id, err)
	}
	return b
}

// scanned returns the row exactly as Scan hands it to the backfill.
func scanned(t *testing.T, st PostgresStore, table string, id uuid.UUID) Row {
	t.Helper()
	var found *Row
	if err := st.Scan(context.Background(), table, func(r Row) error {
		if r.ID == id {
			rr := r
			found = &rr
		}
		return nil
	}); err != nil {
		t.Fatalf("scan %s: %v", table, err)
	}
	if found == nil {
		t.Fatalf("scan %s did not return %s", table, id)
	}
	return *found
}

func TestPostgresUpdateRefusesALostWrite(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	st := PostgresStore{Pool: pool}
	tenant := uuid.New()
	id := insertDoc(t, pool, tenant, "lost-write", dirty)
	row := scanned(t, st, TableDocuments, id)

	// An editor saves between the scan and the update.
	const saved = `{"body":[{"_type":"rich_text","format":"html","value":{"da":"<p>the editor's new text</p>"}}]}`
	if _, err := pool.Exec(ctx, `update resonans_app.documents set body = $1 where id = $2`, saved, id); err != nil {
		t.Fatal(err)
	}
	clean := json.RawMessage(`{"body":[{"_type":"rich_text","format":"html","value":{"da":"<p>ok</p>"}}]}`)
	if err := st.Update(ctx, TableDocuments, row, clean); !errors.Is(err, ErrConcurrentChange) {
		t.Fatalf("Update over a newer save = %v, want ErrConcurrentChange", err)
	}
	if got := storedBody(t, pool, TableDocuments, id); !strings.Contains(got, "the editor's new text") {
		t.Fatalf("the editor's save was overwritten: %s", got)
	}

	// Control: with the row as it now is, the same update goes through.
	fresh := scanned(t, st, TableDocuments, id)
	if err := st.Update(ctx, TableDocuments, fresh, clean); err != nil {
		t.Fatalf("control: Update on the current body = %v", err)
	}
	if got := storedBody(t, pool, TableDocuments, id); strings.Contains(got, "editor's new text") || !strings.Contains(got, "<p>ok</p>") {
		t.Errorf("after the control update: %s", got)
	}
}

func TestPostgresUpdateStaysOnItsTenant(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	st := PostgresStore{Pool: pool}
	tenantA, tenantB := uuid.New(), uuid.New()
	id := insertDoc(t, pool, tenantA, "owned-by-a", dirty)
	row := scanned(t, st, TableDocuments, id)

	// The same id and body, but claimed for another tenant: no row may change.
	forged := row
	forged.TenantID = tenantB
	if err := st.Update(ctx, TableDocuments, forged, json.RawMessage(`{"body":[]}`)); !errors.Is(err, ErrConcurrentChange) {
		t.Fatalf("Update with another tenant = %v, want ErrConcurrentChange (0 rows)", err)
	}
	if got := storedBody(t, pool, TableDocuments, id); !strings.Contains(got, "onerror") {
		t.Fatalf("tenant B's update changed tenant A's row: %s", got)
	}
}

// Cleaning a document bumps updated_at (the delivery ETag includes it, so a
// revalidating cache gets the clean body), and NOT the version counter, which
// numbers the history snapshots.
func TestPostgresUpdateBumpsUpdatedAtNotVersion(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	st := PostgresStore{Pool: pool}
	id := insertDoc(t, pool, uuid.New(), "etag", dirty)
	if _, err := pool.Exec(ctx, `update resonans_app.documents set updated_at = now() - interval '1 hour' where id = $1`, id); err != nil {
		t.Fatal(err)
	}
	var beforeAt time.Time
	var beforeV int
	_ = pool.QueryRow(ctx, `select updated_at, version from resonans_app.documents where id = $1`, id).Scan(&beforeAt, &beforeV)
	if err := st.Update(ctx, TableDocuments, scanned(t, st, TableDocuments, id), json.RawMessage(`{"body":[]}`)); err != nil {
		t.Fatal(err)
	}
	var afterAt time.Time
	var afterV int
	_ = pool.QueryRow(ctx, `select updated_at, version from resonans_app.documents where id = $1`, id).Scan(&afterAt, &afterV)
	if !afterAt.After(beforeAt) || afterV != beforeV {
		t.Errorf("updated_at %v -> %v, version %d -> %d; want updated_at later and version unchanged", beforeAt, afterAt, beforeV, afterV)
	}
}

// Scan reads every tenant (it runs as the migrator), and a version whose
// document is gone is COUNTED, with type "" — the LEFT join — not dropped.
func TestPostgresScanSeesEveryTenantAndOrphanVersions(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	st := PostgresStore{Pool: pool}
	a := insertDoc(t, pool, uuid.New(), "in-a", `{}`)
	b := insertDoc(t, pool, uuid.New(), "in-b", `{}`)
	var orphan uuid.UUID
	if err := pool.QueryRow(ctx, `insert into resonans_app.document_versions (tenant_id, document_id, version, title, body, status)
		values ($1, $2, 1, 'gone', $3, 'published') returning id`, uuid.New(), uuid.New(), dirty).Scan(&orphan); err != nil {
		t.Fatal(err)
	}
	seen := map[uuid.UUID]string{}
	if err := st.Scan(ctx, TableDocuments, func(r Row) error { seen[r.ID] = r.Type; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, okA := seen[a]; !okA || seen[b] != "page" {
		t.Errorf("documents scan saw %v; want both tenants' documents", seen)
	}
	var orphanType *string
	if err := st.Scan(ctx, TableVersions, func(r Row) error {
		if r.ID == orphan {
			ty := r.Type
			orphanType = &ty
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if orphanType == nil || *orphanType != "" {
		t.Errorf("orphan version: seen=%v type=%v; want it counted with type \"\"", orphanType != nil, orphanType)
	}

	slugs := map[uuid.UUID]string{}
	if err := st.ScanSlugs(ctx, func(r SlugRow) error { slugs[r.ID] = r.Slug; return nil }); err != nil {
		t.Fatal(err)
	}
	if slugs[a] != "in-a" || slugs[b] != "in-b" {
		t.Errorf("ScanSlugs = %v; want both tenants", slugs)
	}

	for _, err := range []error{
		st.Scan(ctx, "users", func(Row) error { return nil }),
		st.Update(ctx, "users", Row{}, nil),
	} {
		if err == nil || !strings.Contains(err.Error(), "unknown table") {
			t.Errorf("an unknown table = %v; want a refusal", err)
		}
	}
}

// End to end over the real store: a dirty document is cleaned, its version
// snapshot too, and a second run finds nothing left to do.
func TestPostgresRunCleansAndConverges(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	st := PostgresStore{Pool: pool}
	reg, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tenant := uuid.New()
	id := insertDoc(t, pool, tenant, "dirty", dirty)
	var ver uuid.UUID
	if err := pool.QueryRow(ctx, `insert into resonans_app.document_versions (tenant_id, document_id, version, title, body, status)
		values ($1, $2, 1, 'T', $3, 'published') returning id`, tenant, id, dirty).Scan(&ver); err != nil {
		t.Fatal(err)
	}

	reps, err := Run(ctx, st, reg, true)
	if err != nil {
		t.Fatalf("Run apply: %v", err)
	}
	changed := 0
	for _, r := range reps {
		changed += r.Changed
	}
	if changed != 2 {
		t.Errorf("first apply changed %d rows, want 2 (the document and its version): %+v", changed, reps)
	}
	for table, rid := range map[string]uuid.UUID{TableDocuments: id, TableVersions: ver} {
		if got := storedBody(t, pool, table, rid); strings.Contains(got, "onerror") || !strings.Contains(got, "<p>ok</p>") {
			t.Errorf("%s after apply: %s", table, got)
		}
	}
	reps, err = Run(ctx, st, reg, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reps {
		if r.Changed != 0 {
			t.Errorf("second apply changed %d rows in %s; the backfill must converge", r.Changed, r.Table)
		}
	}
}

// Failures are failures, each kind kept apart from the others: a callback that
// stops the scan stops it; a pool that cannot answer is an error, never an
// empty scan; and a body Postgres refuses is an error, NOT ErrConcurrentChange,
// which the backfill would count as "skipped" and carry on past.
func TestPostgresStoreFailuresAreNotSkips(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	st := PostgresStore{Pool: pool}
	id := insertDoc(t, pool, uuid.New(), "failures", dirty)
	stop := errors.New("stop here")

	if err := st.Scan(ctx, TableDocuments, func(Row) error { return stop }); !errors.Is(err, stop) {
		t.Errorf("Scan with a failing callback = %v, want that error", err)
	}
	if err := st.ScanSlugs(ctx, func(SlugRow) error { return stop }); !errors.Is(err, stop) {
		t.Errorf("ScanSlugs with a failing callback = %v, want that error", err)
	}

	row := scanned(t, st, TableDocuments, id)
	err := st.Update(ctx, TableDocuments, row, json.RawMessage(`{"not json`))
	if err == nil || errors.Is(err, ErrConcurrentChange) {
		t.Errorf("Update with a body Postgres refuses = %v; want an error that is not a skip", err)
	}
	if got := storedBody(t, pool, TableDocuments, id); !strings.Contains(got, "onerror") {
		t.Errorf("a refused update changed the row: %s", got)
	}

	closed, err := pgxpool.New(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	dead := PostgresStore{Pool: closed}
	if err := dead.Scan(ctx, TableDocuments, func(Row) error { return nil }); err == nil {
		t.Error("Scan on a closed pool returned nil")
	}
	if err := dead.ScanSlugs(ctx, func(SlugRow) error { return nil }); err == nil {
		t.Error("ScanSlugs on a closed pool returned nil")
	}
	if err := dead.Update(ctx, TableDocuments, row, json.RawMessage(`{}`)); err == nil || errors.Is(err, ErrConcurrentChange) {
		t.Errorf("Update on a closed pool = %v; want an error that is not a skip", err)
	}
}
