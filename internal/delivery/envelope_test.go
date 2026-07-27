package delivery

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
)

type fakeAssets struct{}

func (fakeAssets) ResolveAsset(_ context.Context, _ uuid.UUID, id string) (map[string]any, bool) {
	if id == "asset-1" {
		return map[string]any{"url": "https://cdn.test/x.jpg", "width": 800, "height": 600}, true
	}
	return nil, false
}

func lessonDoc(t *testing.T) content.Document {
	t.Helper()
	body := `{
		"summary": {"da": "Om kemi", "en": "About chemistry"},
		"estimatedMinutes": 20,
		"body": [
			{"_type": "rich_text", "_key": "a", "content": {"da": "Hej", "en": "Hi"}},
			{"_type": "image", "asset": "asset-1", "alt": {"da": "Diagram"}}
		]
	}`
	return content.Document{
		ID: uuid.New(), Type: "lesson", Slug: "alkaner", Title: "Alkaner",
		Version: 3, Body: json.RawMessage(body),
	}
}

func TestBuildEnvelopeLocaleAndAssets(t *testing.T) {
	env := BuildEnvelope(context.Background(), uuid.New(), lessonDoc(t), reg(t), "da", "da", fakeAssets{})

	if env.Type != "lesson" || env.Slug != "alkaner" || env.Version != 3 {
		t.Fatalf("envelope meta = %+v", env)
	}
	// data: metadata title + resolved i18n summary + passthrough number.
	if env.Data["title"] != "Alkaner" {
		t.Errorf("data.title = %v", env.Data["title"])
	}
	if env.Data["summary"] != "Om kemi" {
		t.Errorf("data.summary = %v (want da resolution)", env.Data["summary"])
	}
	if env.LocaleFallback {
		t.Error("no fallback expected when da present everywhere")
	}
	// components: rich_text i18n resolved + image asset expanded.
	if len(env.Components) != 2 {
		t.Fatalf("components = %d, want 2", len(env.Components))
	}
	if env.Components[0]["_type"] != "rich_text" || env.Components[0]["content"] != "Hej" {
		t.Errorf("rich_text component = %+v", env.Components[0])
	}
	asset, ok := env.Components[1]["asset"].(map[string]any)
	if !ok || asset["url"] != "https://cdn.test/x.jpg" {
		t.Errorf("image asset not expanded: %+v", env.Components[1])
	}
	if env.Components[1]["alt"] != "Diagram" {
		t.Errorf("image alt = %v (want da resolution)", env.Components[1]["alt"])
	}
}

func TestBuildEnvelopeFallbackFlag(t *testing.T) {
	// Request "en": image.alt only has "da" → fallback flagged.
	env := BuildEnvelope(context.Background(), uuid.New(), lessonDoc(t), reg(t), "en", "da", fakeAssets{})

	if env.Data["summary"] != "About chemistry" {
		t.Errorf("summary = %v (want en)", env.Data["summary"])
	}
	if env.Components[0]["content"] != "Hi" {
		t.Errorf("rich_text = %v (want en)", env.Components[0]["content"])
	}
	if env.Components[1]["alt"] != "Diagram" {
		t.Errorf("alt = %v (want da fallback)", env.Components[1]["alt"])
	}
	if !env.LocaleFallback {
		t.Error("expected LocaleFallback=true (alt fell back to da)")
	}
}

func TestBuildEnvelopeUnknownBlockPassthrough(t *testing.T) {
	doc := content.Document{
		ID: uuid.New(), Type: "lesson", Slug: "x", Title: "X", Version: 1,
		Body: json.RawMessage(`{"body":[{"_type":"future_block","x":1}]}`),
	}
	env := BuildEnvelope(context.Background(), uuid.New(), doc, reg(t), "da", "da", fakeAssets{})
	if len(env.Components) != 1 || env.Components[0]["_type"] != "future_block" {
		t.Fatalf("unknown block should pass through raw: %+v", env.Components)
	}
}
