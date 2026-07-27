package schema

import (
	"encoding/json"
	"errors"
	"testing"
)

func reg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	return r
}

func validate(t *testing.T, typ, body string) error {
	t.Helper()
	return ValidateDocument(reg(t), typ, json.RawMessage(body))
}

func TestValidateValidLesson(t *testing.T) {
	body := `{
		"summary": {"da": "Om alkaner", "en": "About alkanes"},
		"estimatedMinutes": 20,
		"body": [
			{"_type": "rich_text", "_key": "a", "value": {"da": "Hej"}},
			{"_type": "quiz", "quizRef": "11111111-1111-1111-1111-111111111111"}
		]
	}`
	if err := validate(t, "lesson", body); err != nil {
		t.Fatalf("valid lesson rejected: %v", err)
	}
}

func TestValidateQuizDocType(t *testing.T) {
	body := `{
		"question": {"da": "Hvor mange bindinger?"},
		"type": "single",
		"options": [{"id": "a", "label": {"da": "4"}, "correct": true},
		            {"id": "b", "label": {"da": "2"}, "correct": false}],
		"explanation": {"da": "Kulstof har 4."}
	}`
	if err := validate(t, "quiz", body); err != nil {
		t.Fatalf("valid quiz rejected: %v", err)
	}
}

func TestValidateUnknownType(t *testing.T) {
	if err := validate(t, "nope", `{}`); !errors.Is(err, ErrUnknownType) {
		t.Fatalf("err = %v, want ErrUnknownType", err)
	}
}

func TestValidateUnknownField(t *testing.T) {
	if err := validate(t, "lesson", `{"foo": "bar"}`); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestValidateMissingRequiredInBlock(t *testing.T) {
	// quiz block requires quizRef.
	body := `{"body": [{"_type": "quiz"}]}`
	if err := validate(t, "lesson", body); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid (missing quiz.quizRef)", err)
	}
}

func TestValidateBlockNotAllowed(t *testing.T) {
	// hero is a marketing block, not allowed in a lesson body.
	body := `{"body": [{"_type": "hero", "heading": {"da": "Hej"}}]}`
	if err := validate(t, "lesson", body); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid (hero not allowed)", err)
	}
}

func TestValidateI18nShape(t *testing.T) {
	// summary is an i18n field → must be a locale-keyed object, not a bare string.
	if err := validate(t, "lesson", `{"summary": "plain string"}`); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid (i18n must be object)", err)
	}
}

func TestValidateNestedColumns(t *testing.T) {
	body := `{"body": [
		{"_type": "columns", "columns": [
			{"content": [{"_type": "rich_text", "value": {"da": "A"}}]},
			{"content": [{"_type": "cta", "label": {"da": "Køb"}, "href": "/x"}]}
		]}
	]}`
	if err := validate(t, "page", body); err != nil {
		t.Fatalf("valid nested columns rejected: %v", err)
	}
}

func TestValidatePageMarketingBlocks(t *testing.T) {
	body := `{"body": [
		{"_type": "hero", "heading": {"da": "Velkommen"}},
		{"_type": "feature_grid", "features": [{"title": {"da": "Hurtig"}}]}
	]}`
	if err := validate(t, "page", body); err != nil {
		t.Fatalf("valid page rejected: %v", err)
	}
}
