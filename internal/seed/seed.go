// Package seed inserts a realistic explainers demo dataset (Danish gymnasium
// organic-chemistry course) so consumers (Etude M5) can render against real
// Resonans content instead of mocks. Domain data is realistic, not lorem
// (rockfaker doctrine). All documents are validated against the schema before
// insert and published.
package seed

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/schema"
)

// Run seeds the explainers demo course (+ assets) for a tenant. Idempotent-ish:
// re-running conflicts on slugs — intended for a fresh tenant/dataset.
func Run(ctx context.Context, store content.Store, assets media.Store, reg *schema.Registry, tenantID uuid.UUID) error {
	s := &seeder{ctx: ctx, store: store, assets: assets, reg: reg, tenant: tenantID}
	return s.run()
}

type seeder struct {
	ctx    context.Context
	store  content.Store
	assets media.Store
	reg    *schema.Registry
	tenant uuid.UUID
}

func i18n(da string) map[string]any { return map[string]any{"da": da} }

func (s *seeder) run() error {
	// --- Assets (image + downloadable PDF) so image/download blocks resolve ---
	w, h := 1200, 800
	imgID, err := s.asset(media.Asset{
		Kind: "image", StorageKey: "explainers/molekyle-diagram.png",
		Filename: "molekyle-diagram.png", Mime: "image/png", SizeBytes: 148_000,
		Width: &w, Height: &h,
	})
	if err != nil {
		return err
	}
	pdfID, err := s.asset(media.Asset{
		Kind: "file", StorageKey: "explainers/opgaver-alkaner.pdf",
		Filename: "opgaver-alkaner.pdf", Mime: "application/pdf", SizeBytes: 92_000,
	})
	if err != nil {
		return err
	}

	// --- Quiz doc (answer key; delivery serves it, Etude redacts) ---
	quizID, err := s.doc("quiz", "kulstof-bindinger", "Kulstof-bindinger", map[string]any{
		"question": i18n("Hvor mange kovalente bindinger danner et kulstofatom typisk?"),
		"type":     "single",
		"options": []any{
			map[string]any{"id": "a", "label": i18n("4"), "correct": true},
			map[string]any{"id": "b", "label": i18n("2"), "correct": false},
			map[string]any{"id": "c", "label": i18n("6"), "correct": false},
		},
		"explanation": i18n("Kulstof har 4 valenselektroner og danner derfor typisk 4 kovalente bindinger."),
	})
	if err != nil {
		return err
	}

	// --- Lessons (varied blocks) ---
	les1, err := s.doc("lesson", "hvad-er-organisk-kemi", "Hvad er organisk kemi?", map[string]any{
		"summary":          i18n("Introduktion til kulstofkemiens grundbegreber."),
		"estimatedMinutes": 12,
		"body": []any{
			block("rich_text", map[string]any{
				"value":  i18n("Organisk kemi er læren om kulstofforbindelser. Kulstof er unikt, fordi det kan danne lange kæder og ringe."),
				"format": "portable_text",
			}),
			block("video_embed", map[string]any{
				"provider": "vimeo", "videoId": "76979871",
				"title": i18n("Introduktion til organisk kemi"),
			}),
			block("callout", map[string]any{
				"variant": "tip",
				"value":   i18n("Husk: 'organisk' betyder ikke 'levende' — det handler om kulstof."),
			}),
			block("image", map[string]any{
				"asset": imgID.String(),
				"alt":   i18n("Diagram over et methan-molekyle"),
			}),
		},
	})
	if err != nil {
		return err
	}
	les2, err := s.doc("lesson", "alkaner-og-alkener", "Alkaner og alkener", map[string]any{
		"summary":          i18n("De simpleste kulbrinter og deres bindinger."),
		"estimatedMinutes": 18,
		"body": []any{
			block("rich_text", map[string]any{
				"value": i18n("Alkaner har kun enkeltbindinger; alkener har mindst én dobbeltbinding."),
			}),
			block("code", map[string]any{
				"language": "text", "value": "CH4  (methan)\nC2H6 (ethan)\nC2H4 (ethen)",
				"filename": "molekylformler.txt",
			}),
			block("download", map[string]any{
				"asset": pdfID.String(),
				"label": i18n("Øvelsesopgaver: alkaner (PDF)"),
			}),
			block("quiz", map[string]any{"quizRef": quizID.String()}),
		},
	})
	if err != nil {
		return err
	}
	les3, err := s.doc("lesson", "isomeri", "Isomeri", map[string]any{
		"summary":          i18n("Samme formel, forskellig struktur."),
		"estimatedMinutes": 15,
		"body": []any{
			block("rich_text", map[string]any{
				"value": i18n("Isomerer har samme molekylformel men forskellig strukturformel og dermed forskellige egenskaber."),
			}),
			block("callout", map[string]any{
				"variant": "info",
				"value":   i18n("Butan og isobutan har begge formlen C4H10, men koger ved forskellige temperaturer."),
			}),
		},
	})
	if err != nil {
		return err
	}

	// --- Modules (ref lessons) ---
	mod1, err := s.doc("module", "kulstofforbindelser", "Kulstofforbindelser", map[string]any{
		"description": i18n("Grundlaget: hvad kulstof kan, og hvordan vi navngiver forbindelser."),
		"lessons": []any{
			map[string]any{"lesson": les1.String()},
			map[string]any{"lesson": les2.String()},
		},
	})
	if err != nil {
		return err
	}
	mod2, err := s.doc("module", "struktur-og-isomeri", "Struktur og isomeri", map[string]any{
		"description": i18n("Hvordan strukturen bestemmer stoffets egenskaber."),
		"lessons":     []any{map[string]any{"lesson": les3.String()}},
	})
	if err != nil {
		return err
	}

	// --- Course (ref modules) ---
	if _, err := s.doc("course", "introduktion-til-organisk-kemi", "Introduktion til organisk kemi", map[string]any{
		"description": i18n("Et gymnasieforløb om kulstofkemiens grundbegreber, bindinger og isomeri."),
		"coverImage":  imgID.String(),
		"subject":     "Kemi",
		"modules": []any{
			map[string]any{"module": mod1.String()},
			map[string]any{"module": mod2.String()},
		},
	}); err != nil {
		return err
	}

	// --- Standalone marketing page ---
	if _, err := s.doc("page", "velkommen", "Velkommen til Explainers", map[string]any{
		"body": []any{
			block("hero", map[string]any{
				"heading":    i18n("Forstå organisk kemi — trin for trin"),
				"subheading": i18n("Video-baseret gymnasieforberedelse på dansk."),
				"ctaLabel":   i18n("Start forløbet"),
				"ctaHref":    "/kurser/introduktion-til-organisk-kemi",
			}),
			block("rich_text", map[string]any{
				"value": i18n("Explainers gør svære kemi-emner konkrete med korte videoer, quizzer og opgaver."),
			}),
		},
		"seoTitle":       i18n("Explainers — organisk kemi for gymnasiet"),
		"seoDescription": i18n("Lær organisk kemi med video, quiz og opgaver."),
	}); err != nil {
		return err
	}

	return nil
}

// asset creates + returns a media asset id.
func (s *seeder) asset(a media.Asset) (uuid.UUID, error) {
	created, err := s.assets.Create(s.ctx, s.tenant, a)
	if err != nil {
		return uuid.Nil, fmt.Errorf("seed asset %s: %w", a.Filename, err)
	}
	return created.ID, nil
}

// doc validates a body against its schema, creates + publishes it, returns its id.
func (s *seeder) doc(typ, slug, title string, body map[string]any) (uuid.UUID, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return uuid.Nil, err
	}
	if err := schema.ValidateDocument(s.reg, typ, raw); err != nil {
		return uuid.Nil, fmt.Errorf("seed %s/%s invalid: %w", typ, slug, err)
	}
	d, err := s.store.Create(s.ctx, s.tenant, content.CreateInput{Type: typ, Slug: slug, Title: title, Body: raw})
	if err != nil {
		return uuid.Nil, fmt.Errorf("seed create %s/%s: %w", typ, slug, err)
	}
	if _, err := s.store.Publish(s.ctx, s.tenant, d.ID); err != nil {
		return uuid.Nil, fmt.Errorf("seed publish %s/%s: %w", typ, slug, err)
	}
	return d.ID, nil
}

// block builds a content block with its _type.
func block(t string, fields map[string]any) map[string]any {
	m := map[string]any{"_type": t}
	for k, v := range fields {
		m[k] = v
	}
	return m
}
