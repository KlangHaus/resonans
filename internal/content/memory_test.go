package content

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

var (
	tenantA = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tenantB = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

func newDraft(t *testing.T, s *MemoryStore, tenant uuid.UUID, typ, slug string) Document {
	t.Helper()
	d, err := s.Create(context.Background(), tenant, CreateInput{
		Type:  typ,
		Slug:  slug,
		Title: "Title " + slug,
		Body:  json.RawMessage(`{"blocks":[]}`),
	})
	if err != nil {
		t.Fatalf("Create(%s): %v", slug, err)
	}
	return d
}

func TestMemoryCreateAndGet(t *testing.T) {
	s := NewMemoryStore()
	d := newDraft(t, s, tenantA, "lesson", "hello")

	if d.Status != StatusDraft {
		t.Errorf("new document status = %q, want draft", d.Status)
	}
	if d.TenantID != tenantA {
		t.Errorf("TenantID = %s, want %s", d.TenantID, tenantA)
	}
	got, err := s.Get(context.Background(), tenantA, d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Slug != "hello" {
		t.Errorf("Slug = %q", got.Slug)
	}
}

// The core tenant-isolation invariant: tenant B cannot see tenant A's document.
func TestMemoryTenantIsolation(t *testing.T) {
	s := NewMemoryStore()
	a := newDraft(t, s, tenantA, "lesson", "secret")

	// Get with the wrong tenant → not found.
	if _, err := s.Get(context.Background(), tenantB, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant Get err = %v, want ErrNotFound", err)
	}
	// List for tenant B → empty.
	list, err := s.List(context.Background(), tenantB, ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("tenant B sees %d documents, want 0", len(list))
	}
	// Same slug is allowed in a different tenant.
	if _, err := s.Create(context.Background(), tenantB, CreateInput{
		Type: "lesson", Slug: "secret", Title: "B's own",
	}); err != nil {
		t.Fatalf("cross-tenant same slug should be allowed: %v", err)
	}
}

func TestMemoryCreateValidation(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Create(context.Background(), tenantA, CreateInput{Type: "", Slug: "x", Title: "t"})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestMemorySlugConflictWithinTenant(t *testing.T) {
	s := NewMemoryStore()
	newDraft(t, s, tenantA, "lesson", "dup")
	_, err := s.Create(context.Background(), tenantA, CreateInput{Type: "lesson", Slug: "dup", Title: "again"})
	if !errors.Is(err, ErrSlugConflict) {
		t.Fatalf("err = %v, want ErrSlugConflict", err)
	}
}

func TestMemoryPublishAndRead(t *testing.T) {
	s := NewMemoryStore()
	d := newDraft(t, s, tenantA, "page", "home")

	if _, err := s.GetPublished(context.Background(), tenantA, "page", "home"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("draft should not be published-visible, got %v", err)
	}

	pub, err := s.Publish(context.Background(), tenantA, d.ID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if pub.Status != StatusPublished || pub.PublishedAt == nil {
		t.Errorf("publish did not set status/published_at: %+v", pub)
	}

	got, err := s.GetPublished(context.Background(), tenantA, "page", "home")
	if err != nil {
		t.Fatalf("GetPublished: %v", err)
	}
	if got.ID != d.ID {
		t.Errorf("GetPublished returned wrong document")
	}
	// Tenant B cannot read tenant A's published document.
	if _, err := s.GetPublished(context.Background(), tenantB, "page", "home"); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant published read should be not-found, got %v", err)
	}
}

func TestMemoryUpdate(t *testing.T) {
	s := NewMemoryStore()
	d := newDraft(t, s, tenantA, "post", "edit")

	newTitle := "Updated"
	newBody := json.RawMessage(`{"blocks":[{"t":"p"}]}`)
	got, err := s.Update(context.Background(), tenantA, d.ID, UpdateInput{Title: &newTitle, Body: &newBody})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Title != "Updated" {
		t.Errorf("Title = %q", got.Title)
	}

	// Cross-tenant update → not found.
	if _, err := s.Update(context.Background(), tenantB, d.ID, UpdateInput{Title: &newTitle}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant update err = %v, want ErrNotFound", err)
	}

	bad := json.RawMessage(`{not json`)
	if _, err := s.Update(context.Background(), tenantA, d.ID, UpdateInput{Body: &bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestMemoryListFilters(t *testing.T) {
	s := NewMemoryStore()
	newDraft(t, s, tenantA, "lesson", "a")
	b := newDraft(t, s, tenantA, "lesson", "b")
	if _, err := s.Publish(context.Background(), tenantA, b.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	all, err := s.List(context.Background(), tenantA, ListFilter{Type: "lesson"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("type filter: got %d, want 2", len(all))
	}

	published, err := s.List(context.Background(), tenantA, ListFilter{Status: StatusPublished})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(published) != 1 || published[0].Slug != "b" {
		t.Errorf("status filter: %+v", published)
	}
}

func TestMemoryVersionsAndRollback(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	d := newDraft(t, s, tenantA, "post", "v")

	if _, err := s.Publish(ctx, tenantA, d.ID); err != nil {
		t.Fatalf("publish 1: %v", err)
	}
	nt, nb := "Second", json.RawMessage(`{"blocks":[{"_type":"x"}]}`)
	if _, err := s.Update(ctx, tenantA, d.ID, UpdateInput{Title: &nt, Body: &nb}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := s.Publish(ctx, tenantA, d.ID); err != nil {
		t.Fatalf("publish 2: %v", err)
	}

	vs, err := s.ListVersions(ctx, tenantA, d.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(vs) != 2 {
		t.Fatalf("versions = %d, want 2", len(vs))
	}
	if vs[0].Title != "Second" { // newest first
		t.Errorf("newest version title = %q", vs[0].Title)
	}

	// Roll back to the older snapshot → new draft with that content.
	rb, err := s.Rollback(ctx, tenantA, d.ID, vs[1].Version)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if rb.Status != StatusDraft {
		t.Errorf("rollback status = %q, want draft", rb.Status)
	}
	if rb.Title != vs[1].Title {
		t.Errorf("rollback title = %q, want %q", rb.Title, vs[1].Title)
	}

	// Cross-tenant rollback is not found (isolation).
	if _, err := s.Rollback(ctx, tenantB, d.ID, vs[1].Version); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant rollback err = %v, want ErrNotFound", err)
	}
}

func TestMemoryReturnsCopies(t *testing.T) {
	s := NewMemoryStore()
	d := newDraft(t, s, tenantA, "lesson", "iso")
	d.Body[0] = 'X'

	got, _ := s.Get(context.Background(), tenantA, d.ID)
	if string(got.Body) == string(d.Body) {
		t.Error("store returned a shared Body slice — mutation leaked")
	}
}
