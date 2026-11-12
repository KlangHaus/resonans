package seed

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/schema"
)

type seeded struct {
	ctx    context.Context
	cs     *content.MemoryStore
	ms     *media.MemoryStore
	reg    *schema.Registry
	tenant uuid.UUID
}

func newSeeded(t *testing.T, runs int) seeded {
	t.Helper()
	reg, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	s := seeded{ctx: context.Background(), cs: content.NewMemoryStore(), ms: media.NewMemoryStore(), reg: reg, tenant: uuid.New()}
	for i := range runs {
		if err := Run(s.ctx, s.cs, s.ms, s.reg, s.tenant); err != nil {
			t.Fatalf("seed run %d: %v", i+1, err)
		}
	}
	return s
}

func (s seeded) docs(t *testing.T) []content.Document {
	t.Helper()
	docs, err := s.cs.List(s.ctx, s.tenant, content.ListFilter{})
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	return docs
}

// A re-run reuses the documents of the first run, so it must reuse their
// assets too; a new asset per run is a row no document points at.
func TestSeedRerunCreatesNoOrphanAssets(t *testing.T) {
	once := newSeeded(t, 1)
	first, err := once.ms.List(once.ctx, once.tenant, media.ListFilter{})
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("fixture: one run made %d assets, want 2", len(first))
	}

	twice := newSeeded(t, 2)
	assets, err := twice.ms.List(twice.ctx, twice.tenant, media.ListFilter{Deleted: media.DeletedInclude})
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	var bodies []byte
	for _, d := range twice.docs(t) {
		bodies = append(bodies, d.Body...)
	}
	orphans := 0
	for _, a := range assets {
		if !bytes.Contains(bodies, []byte(a.ID.String())) {
			orphans++
		}
	}
	if len(assets) != 2 || orphans != 0 {
		t.Errorf("after two runs: %d assets, %d referenced by no document; want 2 and 0", len(assets), orphans)
	}
}

// The seed must store what a save through the editor API would store, so the
// demo data holds no markup the editor could never write.
func TestSeedStoresSanitizedBodies(t *testing.T) {
	s := newSeeded(t, 1)
	docs := s.docs(t)
	if len(docs) == 0 {
		t.Fatal("fixture: the seed created no documents")
	}

	// Positive control: the sanitizer must change a body this test would flag,
	// or an unchanged body below proves nothing.
	raw := []byte(`{"body":[{"_type":"rich_text","format":"html","value":{"da":"<blockquote><p>q</p><cite>c</cite></blockquote>"}}]}`)
	ctl, err := schema.SanitizeDocument(s.reg, "post", raw)
	if err != nil {
		t.Fatalf("control: %v", err)
	}
	if bytes.Equal(ctl, raw) {
		t.Fatal("control: sanitizing a body with <cite> left it unchanged")
	}

	var changed []string
	for _, d := range docs {
		clean, err := schema.SanitizeDocument(s.reg, d.Type, d.Body)
		if err != nil {
			t.Fatalf("sanitize %s/%s: %v", d.Type, d.Slug, err)
		}
		if !bytes.Equal(clean, d.Body) {
			changed = append(changed, d.Type+"/"+d.Slug)
		}
	}
	if len(changed) > 0 {
		t.Errorf("%d of %d seeded bodies change when sanitized: %s", len(changed), len(docs), strings.Join(changed, ", "))
	}
}
