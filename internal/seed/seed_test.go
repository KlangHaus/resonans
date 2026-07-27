package seed

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/schema"
)

func TestSeedRun(t *testing.T) {
	ctx := context.Background()
	cs := content.NewMemoryStore()
	ms := media.NewMemoryStore()
	reg, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	tenant := uuid.New()

	// Run validates every document against the schema before insert; a schema
	// mismatch in the demo content fails here.
	if err := Run(ctx, cs, ms, reg, tenant); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	// Each type is present + published.
	published := map[string]string{
		"course": "introduktion-til-organisk-kemi",
		"module": "kulstofforbindelser",
		"lesson": "alkaner-og-alkener",
		"quiz":   "kulstof-bindinger",
		"page":   "velkommen",
	}
	for typ, slug := range published {
		if _, err := cs.GetPublished(ctx, tenant, typ, slug); err != nil {
			t.Errorf("expected published %s/%s: %v", typ, slug, err)
		}
	}

	// Content is tenant-scoped: another tenant sees nothing.
	if _, err := cs.GetPublished(ctx, uuid.New(), "course", "introduktion-til-organisk-kemi"); err == nil {
		t.Error("seed leaked across tenants")
	}
}
