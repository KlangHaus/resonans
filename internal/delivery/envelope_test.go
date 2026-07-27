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
			{"_type": "rich_text", "_key": "a", "value": {"da": "Hej", "en": "Hi"}},
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
	if env.Components[0]["_type"] != "rich_text" || env.Components[0]["value"] != "Hej" {
		t.Errorf("rich_text = %+v", env.Components[0])
	}
	asset, ok := env.Components[1]["asset"].(map[string]any)
	if !ok || asset["url"] != "https://cdn.test/x.jpg" {
		t.Errorf("image asset not expanded: %+v", env.Components[1])
	}
	// §F: image alt is moved INTO the asset object.
	if asset["alt"] != "Diagram" {
		t.Errorf("image asset.alt = %v (want da resolution)", asset["alt"])
	}
}

func TestBuildEnvelopeFallbackFlag(t *testing.T) {
	// Request "en": image.alt only has "da" → fallback flagged.
	env := BuildEnvelope(context.Background(), uuid.New(), lessonDoc(t), reg(t), "en", "da", fakeAssets{})

	if env.Data["summary"] != "About chemistry" {
		t.Errorf("summary = %v (want en)", env.Data["summary"])
	}
	if env.Components[0]["value"] != "Hi" {
		t.Errorf("rich_text = %v (want en)", env.Components[0]["value"])
	}
	asset, _ := env.Components[1]["asset"].(map[string]any)
	if asset["alt"] != "Diagram" {
		t.Errorf("asset.alt = %v (want da fallback)", asset["alt"])
	}
	if !env.LocaleFallback {
		t.Error("expected LocaleFallback=true (alt fell back to da)")
	}
}

func TestBuildEnvelopeQuizAnswerKey(t *testing.T) {
	doc := content.Document{
		ID: uuid.New(), Type: "quiz", Slug: "q1", Title: "Q1", Version: 1,
		Body: json.RawMessage(`{
			"question": {"da": "Hvor mange?"},
			"type": "single",
			"options": [{"id":"a","label":{"da":"4"},"correct":true},
			            {"id":"b","label":{"da":"2"},"correct":false}],
			"explanation": {"da": "Fire."}
		}`),
	}
	env := BuildEnvelope(context.Background(), uuid.New(), doc, reg(t), "da", "da", fakeAssets{})

	// correctOptionIds is derived + INCLUDED (Etude redacts server-side, §F).
	cids, _ := env.Data["correctOptionIds"].([]any)
	if len(cids) != 1 || cids[0] != "a" {
		t.Fatalf("correctOptionIds = %v, want [a]", env.Data["correctOptionIds"])
	}
	// per-option `correct` is folded away.
	opts, _ := env.Data["options"].([]any)
	first, _ := opts[0].(map[string]any)
	if _, has := first["correct"]; has {
		t.Error("option.correct should be stripped into correctOptionIds")
	}
	if first["label"] != "4" {
		t.Errorf("option label not i18n-resolved: %v", first)
	}
	if env.Data["explanation"] != "Fire." {
		t.Errorf("explanation = %v", env.Data["explanation"])
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
