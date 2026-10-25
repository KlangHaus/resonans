package seed

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/schema"
)

var errInjected = errors.New("injected")

// faultyContent fails the n-th call (1-based) of one method; 0 never fails.
type faultyContent struct {
	*content.MemoryStore
	failCreate, failPublish, failGetBySlug int
	creates, publishes                     int
}

func (f *faultyContent) Create(ctx context.Context, tid uuid.UUID, in content.CreateInput) (content.Document, error) {
	f.creates++
	if f.creates == f.failCreate {
		return content.Document{}, errInjected
	}
	return f.MemoryStore.Create(ctx, tid, in)
}

func (f *faultyContent) Publish(ctx context.Context, tid, id uuid.UUID) (content.Document, error) {
	f.publishes++
	if f.publishes == f.failPublish {
		return content.Document{}, errInjected
	}
	return f.MemoryStore.Publish(ctx, tid, id)
}

func (f *faultyContent) GetBySlug(ctx context.Context, tid uuid.UUID, typ, slug string) (content.Document, error) {
	if f.failGetBySlug > 0 {
		return content.Document{}, errInjected
	}
	return f.MemoryStore.GetBySlug(ctx, tid, typ, slug)
}

type faultyMedia struct {
	*media.MemoryStore
	failCreate, creates int
}

func (f *faultyMedia) Create(ctx context.Context, tid uuid.UUID, a media.Asset) (media.Asset, error) {
	f.creates++
	if f.creates == f.failCreate {
		return media.Asset{}, errInjected
	}
	return f.MemoryStore.Create(ctx, tid, a)
}

func registry(t *testing.T) *schema.Registry {
	t.Helper()
	reg, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg
}

func allDocs(t *testing.T, s content.Store, tenant uuid.UUID) []content.Document {
	t.Helper()
	docs, err := s.List(context.Background(), tenant, content.ListFilter{Deleted: content.DeletedInclude})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return docs
}

// Running the seed again (to add blog posts to an already-seeded tenant) reuses
// every existing document instead of failing or duplicating it.
func TestSeedRerunReusesDocuments(t *testing.T) {
	ctx := context.Background()
	reg := registry(t)
	cs, ms := content.NewMemoryStore(), media.NewMemoryStore()
	tenant := uuid.New()

	if err := Run(ctx, cs, ms, reg, tenant); err != nil {
		t.Fatalf("first run: %v", err)
	}
	before := allDocs(t, cs, tenant)
	ids := map[string]uuid.UUID{}
	for _, d := range before {
		ids[d.Type+"/"+d.Slug] = d.ID
	}
	if err := Run(ctx, cs, ms, reg, tenant); err != nil {
		t.Fatalf("second run: %v", err)
	}
	after := allDocs(t, cs, tenant)
	if len(after) != len(before) {
		t.Fatalf("documents after a re-run = %d, want the same %d", len(after), len(before))
	}
	for _, d := range after {
		if ids[d.Type+"/"+d.Slug] != d.ID {
			t.Errorf("%s/%s got a new id on re-run", d.Type, d.Slug)
		}
	}
}

// Every write the seed makes can fail, and each failure stops the run with an
// error that names what it was doing, rather than leaving a half-seeded tenant
// that reports success.
func TestSeedStopsOnEveryFailure(t *testing.T) {
	ctx := context.Background()
	reg := registry(t)

	// Count the writes of a clean run: that is the number of places to fail.
	clean := &faultyContent{MemoryStore: content.NewMemoryStore()}
	cleanMedia := &faultyMedia{MemoryStore: media.NewMemoryStore()}
	if err := Run(ctx, clean, cleanMedia, reg, uuid.New()); err != nil {
		t.Fatalf("clean run: %v", err)
	}
	if clean.creates == 0 || clean.publishes != clean.creates || cleanMedia.creates == 0 {
		t.Fatalf("clean run made %d creates, %d publishes, %d assets — the counter is broken",
			clean.creates, clean.publishes, cleanMedia.creates)
	}

	for n := 1; n <= clean.creates; n++ {
		cs := &faultyContent{MemoryStore: content.NewMemoryStore(), failCreate: n}
		err := Run(ctx, cs, media.NewMemoryStore(), reg, uuid.New())
		if !errors.Is(err, errInjected) || !strings.Contains(err.Error(), "seed create") {
			t.Errorf("create #%d failing: Run = %v, want a wrapped 'seed create' error", n, err)
		}
	}
	for n := 1; n <= clean.publishes; n++ {
		cs := &faultyContent{MemoryStore: content.NewMemoryStore(), failPublish: n}
		err := Run(ctx, cs, media.NewMemoryStore(), reg, uuid.New())
		if !errors.Is(err, errInjected) || !strings.Contains(err.Error(), "seed publish") {
			t.Errorf("publish #%d failing: Run = %v, want a wrapped 'seed publish' error", n, err)
		}
	}
	for n := 1; n <= cleanMedia.creates; n++ {
		ms := &faultyMedia{MemoryStore: media.NewMemoryStore(), failCreate: n}
		err := Run(ctx, content.NewMemoryStore(), ms, reg, uuid.New())
		if !errors.Is(err, errInjected) || !strings.Contains(err.Error(), "seed asset") {
			t.Errorf("asset #%d failing: Run = %v, want a wrapped 'seed asset' error", n, err)
		}
	}

	// A re-run whose lookup of the existing document fails must not carry on
	// with a nil id.
	tenant := uuid.New()
	cs := &faultyContent{MemoryStore: content.NewMemoryStore()}
	if err := Run(ctx, cs, media.NewMemoryStore(), reg, tenant); err != nil {
		t.Fatalf("first run: %v", err)
	}
	cs.failGetBySlug = 1
	err := Run(ctx, cs, media.NewMemoryStore(), reg, tenant)
	if !errors.Is(err, errInjected) || !strings.Contains(err.Error(), "seed lookup existing") {
		t.Errorf("re-run with a failing lookup: Run = %v, want a wrapped 'seed lookup existing' error", err)
	}
}

// The seed validates before it writes: a body the schema refuses never reaches
// the store.
func TestSeedRefusesAnInvalidDocument(t *testing.T) {
	cs := content.NewMemoryStore()
	s := &seeder{ctx: context.Background(), store: cs, assets: media.NewMemoryStore(), reg: registry(t), tenant: uuid.New()}
	_, err := s.doc("quiz", "broken", "Broken", map[string]any{"question": 42})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("doc(invalid quiz) err = %v, want a validation error", err)
	}
	if docs := allDocs(t, cs, s.tenant); len(docs) != 0 {
		t.Fatalf("an invalid document was written anyway: %d documents", len(docs))
	}
	if _, err := s.doc("quiz", "ok", "OK", map[string]any{
		"question": i18n("Q?"), "type": "single",
		"options": []any{map[string]any{"id": "a", "label": i18n("A"), "correct": true}},
	}); err != nil {
		t.Fatalf("control: a valid quiz was refused: %v", err)
	}
}
