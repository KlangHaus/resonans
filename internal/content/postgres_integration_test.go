//go:build integration

// The Postgres store against a real Postgres. Until these cells existed, no
// test called NewPostgresStore: the store production runs was covered by
// nothing, and the memory store every handler test uses stood in for it.
//
// 🔴 The store as a role WITHOUT bypassrls, never as the migrator. PostgresStore
// puts no tenant_id in its queries; isolation is the RLS policy alone. The CI
// database user is a superuser, which bypasses RLS, so a store on that pool
// would read across tenants and every isolation cell below would be measuring
// the superuser rather than the policy. TestPostgresIsolationIsTheRoles proves
// the difference on the same rows.
package content

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/persistence"
	"github.com/KlangHaus/resonans/internal/testpg"
)

// stores migrates this build's own database and returns the store as the app
// role (RLS applies) and as the migrator (RLS bypassed), over the same rows.
func stores(t *testing.T) (app, migrator *PostgresStore) {
	t.Helper()
	ctx := context.Background()
	pool, scopedDSN := testpg.PoolDSN(t)
	if _, err := persistence.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	appPool := testpg.AppPool(t, pool, scopedDSN, "content_it_app")
	return NewPostgresStore(appPool), NewPostgresStore(pool)
}

func pgDraft(t *testing.T, s *PostgresStore, tenant uuid.UUID, typ, slug string) Document {
	t.Helper()
	d, err := s.Create(context.Background(), tenant, CreateInput{
		Type: typ, Slug: slug, Title: "Title " + slug, Body: json.RawMessage(`{"blocks":[]}`),
	})
	if err != nil {
		t.Fatalf("Create(%s): %v", slug, err)
	}
	return d
}

func strPtr(s string) *string { return &s }

func TestPostgresIsolationIsTheRoles(t *testing.T) {
	ctx := context.Background()
	app, migrator := stores(t)
	tenantA, tenantB := uuid.New(), uuid.New()
	a := pgDraft(t, app, tenantA, "page", "private")

	// NEGATIVE CONTROL, on the same row: the migrator bypasses RLS, so tenant B
	// reads A's document through it. Were this refused, the refusals below would
	// be about the data or the query, not about the policy.
	if _, err := migrator.Get(ctx, tenantB, a.ID); err != nil {
		t.Fatalf("control: the migrator could not read the row as another tenant (%v) — "+
			"nothing below can show that RLS is what isolates", err)
	}

	if _, err := app.Get(ctx, tenantB, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get across tenants err = %v, want ErrNotFound", err)
	}
	if _, err := app.GetBySlug(ctx, tenantB, "page", "private"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetBySlug across tenants err = %v, want ErrNotFound", err)
	}
	if list, err := app.List(ctx, tenantB, ListFilter{Deleted: DeletedInclude}); err != nil || len(list) != 0 {
		t.Errorf("List for tenant B = %d rows, %v; want 0", len(list), err)
	}
	if _, err := app.Update(ctx, tenantB, a.ID, UpdateInput{Title: strPtr("hijack")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update across tenants err = %v, want ErrNotFound", err)
	}
	if _, err := app.Publish(ctx, tenantB, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Publish across tenants err = %v, want ErrNotFound", err)
	}
	if _, err := app.SoftDelete(ctx, tenantB, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("SoftDelete across tenants err = %v, want ErrNotFound", err)
	}
	if got, err := app.Get(ctx, tenantA, a.ID); err != nil || got.Title != a.Title || got.IsDeleted() {
		t.Errorf("after B's attempts, A reads title %q deleted=%v (%v); want it untouched", got.Title, got.IsDeleted(), err)
	}
	// A slug is per tenant: B may use the one A has.
	pgDraft(t, app, tenantB, "page", "private")
}

func TestPostgresLifecycle(t *testing.T) {
	ctx := context.Background()
	app, _ := stores(t)
	tenant := uuid.New()
	d := pgDraft(t, app, tenant, "page", "about")
	if d.Status != StatusDraft || d.Version != 1 || d.TenantID != tenant {
		t.Fatalf("Create = status %q version %d tenant %s", d.Status, d.Version, d.TenantID)
	}
	if _, err := app.Create(ctx, tenant, CreateInput{Type: "page", Slug: "about", Title: "x", Body: json.RawMessage(`{}`)}); !errors.Is(err, ErrSlugConflict) {
		t.Fatalf("Create on a taken slug err = %v, want ErrSlugConflict", err)
	}
	if _, err := app.Create(ctx, tenant, CreateInput{Type: "page", Slug: "Not A Slug", Title: "x", Body: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("Create with an invalid slug succeeded")
	}
	if _, err := app.GetPublished(ctx, tenant, "page", "about"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetPublished(draft) err = %v, want ErrNotFound", err)
	}

	body := json.RawMessage(`{"blocks":[{"type":"text"}]}`)
	u, err := app.Update(ctx, tenant, d.ID, UpdateInput{Title: strPtr("About us"), Body: &body})
	if err != nil || u.Title != "About us" || u.Version != 2 {
		t.Fatalf("Update = title %q version %d, %v; want \"About us\" v2", u.Title, u.Version, err)
	}
	bad := json.RawMessage(`{`)
	if _, err := app.Update(ctx, tenant, d.ID, UpdateInput{Body: &bad}); err == nil {
		t.Fatal("Update with an invalid body succeeded")
	}

	p, err := app.Publish(ctx, tenant, d.ID)
	if err != nil || p.Status != StatusPublished || p.PublishedAt == nil || p.Version != 3 {
		t.Fatalf("Publish = status %q version %d publishedAt %v, %v", p.Status, p.Version, p.PublishedAt, err)
	}
	if got, err := app.GetPublished(ctx, tenant, "page", "about"); err != nil || got.ID != d.ID {
		t.Fatalf("GetPublished = %v, %v; want %s", got.ID, err, d.ID)
	}
	if got, err := app.GetBySlug(ctx, tenant, "page", "about"); err != nil || got.ID != d.ID {
		t.Fatalf("GetBySlug = %v, %v; want %s", got.ID, err, d.ID)
	}

	// A second edit and publish, so there are two snapshots to choose from.
	if _, err := app.Update(ctx, tenant, d.ID, UpdateInput{Title: strPtr("About KlangHaus")}); err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if _, err := app.Publish(ctx, tenant, d.ID); err != nil {
		t.Fatalf("second Publish: %v", err)
	}
	versions, err := app.ListVersions(ctx, tenant, d.ID)
	if err != nil || len(versions) != 2 || versions[0].Version <= versions[1].Version {
		t.Fatalf("ListVersions = %d versions (%v); want 2, newest first", len(versions), err)
	}
	if versions[1].Title != "About us" || versions[0].Title != "About KlangHaus" {
		t.Fatalf("snapshot titles = %q, %q", versions[1].Title, versions[0].Title)
	}

	r, err := app.Rollback(ctx, tenant, d.ID, versions[1].Version)
	if err != nil || r.Title != "About us" || r.Status != StatusDraft || r.Version != 6 {
		t.Fatalf("Rollback = title %q status %q version %d, %v; want the first snapshot as a v6 draft", r.Title, r.Status, r.Version, err)
	}
	if _, err := app.Rollback(ctx, tenant, d.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Rollback to an unknown version err = %v, want ErrNotFound", err)
	}
	if _, err := app.Update(ctx, tenant, uuid.New(), UpdateInput{Title: strPtr("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update of an unknown id err = %v, want ErrNotFound", err)
	}
}

func TestPostgresListFilters(t *testing.T) {
	ctx := context.Background()
	app, _ := stores(t)
	tenant := uuid.New()
	pgDraft(t, app, tenant, "page", "p1")
	post := pgDraft(t, app, tenant, "post", "b1")
	if _, err := app.Publish(ctx, tenant, post.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	newest := pgDraft(t, app, tenant, "post", "b2")

	for _, c := range []struct {
		name string
		f    ListFilter
		want int
	}{
		{"all live", ListFilter{}, 3},
		{"type", ListFilter{Type: "post"}, 2},
		{"status", ListFilter{Status: StatusPublished}, 1},
		{"type and status", ListFilter{Type: "page", Status: StatusPublished}, 0},
	} {
		got, err := app.List(ctx, tenant, c.f)
		if err != nil || len(got) != c.want {
			t.Errorf("List(%s) = %d rows, %v; want %d", c.name, len(got), err, c.want)
		}
	}
	all, _ := app.List(ctx, tenant, ListFilter{})
	if len(all) == 0 || all[0].ID != newest.ID {
		t.Errorf("List order: first is not the newest document")
	}
}

func TestPostgresBin(t *testing.T) {
	ctx := context.Background()
	app, _ := stores(t)
	tenant := uuid.New()
	d := pgDraft(t, app, tenant, "page", "old")
	if _, err := app.Publish(ctx, tenant, d.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if _, err := app.Restore(ctx, tenant, d.ID); !errors.Is(err, ErrNotDeleted) {
		t.Fatalf("Restore(live) err = %v, want ErrNotDeleted", err)
	}
	del, err := app.SoftDelete(ctx, tenant, d.ID)
	if err != nil || !del.IsDeleted() {
		t.Fatalf("SoftDelete = deleted:%v, %v", del.IsDeleted(), err)
	}
	if del.Version != 2 {
		t.Errorf("SoftDelete changed the version to %d; deleting is not an edit", del.Version)
	}
	if _, err := app.SoftDelete(ctx, tenant, d.ID); !errors.Is(err, ErrDeleted) {
		t.Errorf("second SoftDelete err = %v, want ErrDeleted", err)
	}
	// classifyMiss: a write to a deleted document says so, not "not found".
	if _, err := app.Update(ctx, tenant, d.ID, UpdateInput{Title: strPtr("x")}); !errors.Is(err, ErrDeleted) {
		t.Errorf("Update(deleted) err = %v, want ErrDeleted", err)
	}
	if _, err := app.Publish(ctx, tenant, d.ID); !errors.Is(err, ErrDeleted) {
		t.Errorf("Publish(deleted) err = %v, want ErrDeleted", err)
	}
	if _, err := app.GetPublished(ctx, tenant, "page", "old"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetPublished(deleted) err = %v, want ErrNotFound", err)
	}
	if _, err := app.GetBySlug(ctx, tenant, "page", "old"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetBySlug(deleted) err = %v, want ErrNotFound", err)
	}
	live, _ := app.List(ctx, tenant, ListFilter{})
	bin, _ := app.List(ctx, tenant, ListFilter{Deleted: DeletedOnly})
	all, _ := app.List(ctx, tenant, ListFilter{Deleted: DeletedInclude})
	if len(live) != 0 || len(bin) != 1 || len(all) != 1 {
		t.Errorf("List live/bin/all = %d/%d/%d, want 0/1/1", len(live), len(bin), len(all))
	}

	// The bin does not reserve a slug; restoring onto a retaken one fails and the
	// live document keeps it.
	fresh := pgDraft(t, app, tenant, "page", "old")
	if _, err := app.Restore(ctx, tenant, d.ID); !errors.Is(err, ErrSlugConflict) {
		t.Fatalf("Restore onto a taken slug err = %v, want ErrSlugConflict", err)
	}
	if got, _ := app.Get(ctx, tenant, d.ID); !got.IsDeleted() {
		t.Fatal("a refused restore brought the document back anyway")
	}
	if _, err := app.SoftDelete(ctx, tenant, fresh.ID); err != nil {
		t.Fatalf("SoftDelete(fresh): %v", err)
	}
	r, err := app.Restore(ctx, tenant, d.ID)
	if err != nil || r.IsDeleted() {
		t.Fatalf("Restore once the slug is free = deleted:%v, %v", r.IsDeleted(), err)
	}
	if _, err := app.Restore(ctx, tenant, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("Restore(unknown id) err = %v, want ErrNotFound", err)
	}
	if _, err := app.SoftDelete(ctx, tenant, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("SoftDelete(unknown id) err = %v, want ErrNotFound", err)
	}
}

// The catalog order and its pages in SQL: the same documents, publish dates and
// ids as the memory store's cell, so both stores answer the same walk.
func TestPostgresListOrderPublishedPages(t *testing.T) {
	ctx := context.Background()
	app, migrator := stores(t)
	tenant := uuid.New()
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		slug string
		id   string
		pub  *time.Duration
	}{
		{"no-date-b", "00000000-0000-0000-0000-000000000009", nil},
		{"no-date-a", "00000000-0000-0000-0000-000000000008", nil},
		{"tie-b", "00000000-0000-0000-0000-000000000002", durPtr(time.Hour)},
		{"old", "00000000-0000-0000-0000-000000000005", durPtr(0)},
		{"tie-a", "00000000-0000-0000-0000-000000000001", durPtr(time.Hour)},
		{"newest", "00000000-0000-0000-0000-000000000003", durPtr(2 * time.Hour)},
	} {
		d := pgDraft(t, app, tenant, "post", c.slug)
		var pub *time.Time
		if c.pub != nil {
			v := t0.Add(*c.pub)
			pub = &v
		}
		// Ids and dates the API cannot set, so the order under test is known.
		if _, err := migrator.pool.Exec(ctx,
			`update resonans_app.documents set id = $2, status = 'published', published_at = $3 where id = $1`,
			d.ID, uuid.MustParse(c.id), pub); err != nil {
			t.Fatalf("set %s: %v", c.slug, err)
		}
	}
	f := ListFilter{Type: "post", Status: StatusPublished, Order: OrderPublished}
	want := "newest,tie-a,tie-b,old,no-date-a,no-date-b"
	all, err := app.List(ctx, tenant, f)
	if err != nil || slugs(all) != want {
		t.Fatalf("List = %s, %v; want %s", slugs(all), err, want)
	}
	for limit := 1; limit <= len(all)+1; limit++ {
		var walked []Document
		page := f
		page.Limit = limit
		for i := 0; i < len(all)+2; i++ {
			got, err := app.List(ctx, tenant, page)
			if err != nil {
				t.Fatalf("limit %d: %v", limit, err)
			}
			walked = append(walked, got...)
			if len(got) < limit {
				break
			}
			last := got[len(got)-1]
			page.After = &ListCursor{At: last.SortKey(OrderPublished), ID: last.ID}
		}
		if slugs(walked) != want {
			t.Errorf("limit %d: pages walk %s, want %s", limit, slugs(walked), want)
		}
	}
}

func durPtr(d time.Duration) *time.Duration { return &d }
