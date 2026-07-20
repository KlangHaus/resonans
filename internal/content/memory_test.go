package content

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func newDraft(t *testing.T, s *MemoryStore, site, slug string) Entry {
	t.Helper()
	e, err := s.Create(context.Background(), CreateInput{
		Site:  site,
		Type:  "page",
		Slug:  slug,
		Title: "Title " + slug,
		Body:  json.RawMessage(`{"blocks":[]}`),
	})
	if err != nil {
		t.Fatalf("Create(%s): %v", slug, err)
	}
	return e
}

func TestMemoryCreateAndGet(t *testing.T) {
	s := NewMemoryStore()
	e := newDraft(t, s, "klanghaus.dk", "hello")

	if e.Status != StatusDraft {
		t.Errorf("new entry status = %q, want draft", e.Status)
	}
	got, err := s.Get(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Slug != "hello" {
		t.Errorf("Slug = %q", got.Slug)
	}
}

func TestMemoryCreateValidation(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Create(context.Background(), CreateInput{Site: "s", Type: "", Slug: "x", Title: "t"})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestMemorySlugConflict(t *testing.T) {
	s := NewMemoryStore()
	newDraft(t, s, "klanghaus.dk", "dup")
	_, err := s.Create(context.Background(), CreateInput{
		Site: "klanghaus.dk", Type: "page", Slug: "dup", Title: "again",
	})
	if !errors.Is(err, ErrSlugConflict) {
		t.Fatalf("err = %v, want ErrSlugConflict", err)
	}
	// Same slug, different site is allowed.
	if _, err := s.Create(context.Background(), CreateInput{
		Site: "other.dk", Type: "page", Slug: "dup", Title: "ok",
	}); err != nil {
		t.Fatalf("cross-site slug should be allowed: %v", err)
	}
}

func TestMemoryGetNotFound(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Get(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestMemoryPublishAndRead(t *testing.T) {
	s := NewMemoryStore()
	e := newDraft(t, s, "klanghaus.dk", "post")

	// Not visible on the published path while a draft.
	if _, err := s.GetPublished(context.Background(), "klanghaus.dk", "post"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("draft should not be published-visible, got %v", err)
	}

	pub, err := s.Publish(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if pub.Status != StatusPublished || pub.PublishedAt == nil {
		t.Errorf("publish did not set status/published_at: %+v", pub)
	}

	got, err := s.GetPublished(context.Background(), "klanghaus.dk", "post")
	if err != nil {
		t.Fatalf("GetPublished: %v", err)
	}
	if got.ID != e.ID {
		t.Errorf("GetPublished returned wrong entry")
	}
}

func TestMemoryUpdate(t *testing.T) {
	s := NewMemoryStore()
	e := newDraft(t, s, "klanghaus.dk", "edit")

	newTitle := "Updated"
	newBody := json.RawMessage(`{"blocks":[{"t":"p"}]}`)
	got, err := s.Update(context.Background(), e.ID, UpdateInput{Title: &newTitle, Body: &newBody})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Title != "Updated" {
		t.Errorf("Title = %q", got.Title)
	}

	// Invalid JSON body rejected.
	bad := json.RawMessage(`{not json`)
	if _, err := s.Update(context.Background(), e.ID, UpdateInput{Body: &bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestMemoryListFilters(t *testing.T) {
	s := NewMemoryStore()
	newDraft(t, s, "klanghaus.dk", "a")
	b := newDraft(t, s, "klanghaus.dk", "b")
	newDraft(t, s, "other.dk", "c")
	if _, err := s.Publish(context.Background(), b.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	all, err := s.List(context.Background(), ListFilter{Site: "klanghaus.dk"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("site filter: got %d, want 2", len(all))
	}

	published, err := s.List(context.Background(), ListFilter{Site: "klanghaus.dk", Status: StatusPublished})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(published) != 1 || published[0].Slug != "b" {
		t.Errorf("status filter: %+v", published)
	}
}

// Mutating a returned Entry's Body must not corrupt stored state.
func TestMemoryReturnsCopies(t *testing.T) {
	s := NewMemoryStore()
	e := newDraft(t, s, "klanghaus.dk", "iso")
	e.Body[0] = 'X' // mutate the returned slice

	got, _ := s.Get(context.Background(), e.ID)
	if string(got.Body) == string(e.Body) {
		t.Error("store returned a shared Body slice — mutation leaked")
	}
}
