package content

import (
	"context"
	"errors"
	"testing"
)

// The bin (soft delete) on the memory store, which every handler test runs
// against. Each rule is asserted from both sides, so a store that refuses
// everything, or allows everything, cannot pass.

func TestMemoryGetBySlug(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	d := newDraft(t, s, tenantA, "lesson", "intro")

	got, err := s.GetBySlug(ctx, tenantA, "lesson", "intro")
	if err != nil || got.ID != d.ID {
		t.Fatalf("GetBySlug(own draft) = %v, %v; want %s", got.ID, err, d.ID)
	}
	// Drafts are found by slug, unlike GetPublished.
	if _, err := s.GetPublished(ctx, tenantA, "lesson", "intro"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetPublished(draft) err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetBySlug(ctx, tenantB, "lesson", "intro"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant GetBySlug err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetBySlug(ctx, tenantA, "page", "intro"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBySlug(other type) err = %v, want ErrNotFound", err)
	}
}

func TestMemorySoftDeleteHidesFromReads(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	d := newDraft(t, s, tenantA, "lesson", "gone")
	if _, err := s.Publish(ctx, tenantA, d.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if d.IsDeleted() {
		t.Fatal("a new document reports IsDeleted")
	}

	del, err := s.SoftDelete(ctx, tenantA, d.ID)
	if err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if !del.IsDeleted() {
		t.Fatal("SoftDelete returned a document that is not deleted")
	}

	if _, err := s.GetBySlug(ctx, tenantA, "lesson", "gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetBySlug(deleted) err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetPublished(ctx, tenantA, "lesson", "gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetPublished(deleted) err = %v, want ErrNotFound", err)
	}
	live, _ := s.List(ctx, tenantA, ListFilter{})
	bin, _ := s.List(ctx, tenantA, ListFilter{Deleted: DeletedOnly})
	all, _ := s.List(ctx, tenantA, ListFilter{Deleted: DeletedInclude})
	if len(live) != 0 || len(bin) != 1 || len(all) != 1 {
		t.Errorf("List live/bin/all = %d/%d/%d, want 0/1/1", len(live), len(bin), len(all))
	}
	// Get by id still reaches it: that is how the bin is restored.
	if got, err := s.Get(ctx, tenantA, d.ID); err != nil || !got.IsDeleted() {
		t.Errorf("Get(deleted) = deleted:%v, %v; want the deleted document", got.IsDeleted(), err)
	}
}

func TestMemorySoftDeleteRefusals(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	d := newDraft(t, s, tenantA, "lesson", "x")

	if _, err := s.SoftDelete(ctx, tenantB, d.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant SoftDelete err = %v, want ErrNotFound", err)
	}
	if got, _ := s.Get(ctx, tenantA, d.ID); got.IsDeleted() {
		t.Fatal("a refused cross-tenant SoftDelete deleted the document anyway")
	}
	if _, err := s.SoftDelete(ctx, tenantA, d.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if _, err := s.SoftDelete(ctx, tenantA, d.ID); !errors.Is(err, ErrDeleted) {
		t.Fatalf("second SoftDelete err = %v, want ErrDeleted", err)
	}
}

func TestMemoryRestore(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	d := newDraft(t, s, tenantA, "lesson", "back")

	if _, err := s.Restore(ctx, tenantA, d.ID); !errors.Is(err, ErrNotDeleted) {
		t.Fatalf("Restore(live) err = %v, want ErrNotDeleted", err)
	}
	if _, err := s.SoftDelete(ctx, tenantA, d.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if _, err := s.Restore(ctx, tenantB, d.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant Restore err = %v, want ErrNotFound", err)
	}
	got, err := s.Restore(ctx, tenantA, d.ID)
	if err != nil || got.IsDeleted() {
		t.Fatalf("Restore = deleted:%v, %v; want a live document", got.IsDeleted(), err)
	}
	if found, err := s.GetBySlug(ctx, tenantA, "lesson", "back"); err != nil || found.ID != d.ID {
		t.Fatalf("GetBySlug after restore = %v, %v; want %s", found.ID, err, d.ID)
	}
}

// A deleted page does not reserve its slug, so it can be recreated; restoring
// the old one then fails rather than leaving two live documents on one URL.
func TestMemoryRestoreRefusesATakenSlug(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	old := newDraft(t, s, tenantA, "lesson", "taken")
	if _, err := s.SoftDelete(ctx, tenantA, old.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	fresh := newDraft(t, s, tenantA, "lesson", "taken") // allowed: the bin does not reserve

	if _, err := s.Restore(ctx, tenantA, old.ID); !errors.Is(err, ErrSlugConflict) {
		t.Fatalf("Restore onto a taken slug err = %v, want ErrSlugConflict", err)
	}
	if got, _ := s.Get(ctx, tenantA, old.ID); !got.IsDeleted() {
		t.Fatal("a refused restore brought the old document back anyway")
	}
	if got, err := s.GetBySlug(ctx, tenantA, "lesson", "taken"); err != nil || got.ID != fresh.ID {
		t.Fatalf("slug owner = %v, %v; want the live document %s", got.ID, err, fresh.ID)
	}
	// The same slug in another tenant is no conflict.
	other := newDraft(t, s, tenantB, "lesson", "free")
	if _, err := s.SoftDelete(ctx, tenantB, other.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	newDraft(t, s, tenantA, "lesson", "free")
	if _, err := s.Restore(ctx, tenantB, other.ID); err != nil {
		t.Fatalf("Restore with the slug live only in ANOTHER tenant: %v", err)
	}
}
