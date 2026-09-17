package content

import (
	"errors"
	"strings"
	"testing"
)

func TestValidSlug(t *testing.T) {
	for _, s := range []string{"a", "organisk-kemi", "musikeren-er-kunden", "q1", "2026-plan", strings.Repeat("a", 200)} {
		if !ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = false, want true", s)
		}
	}
	for _, s := range []string{
		"", "/evil.example", `\evil.example`, "//evil", "evil.example", "UPPER", "a--b", "-a", "a-",
		"æble", "a b", "a/b", "a?b", "a#b", "a%2Fb", "a\tb", strings.Repeat("a", 201),
	} {
		if ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = true, want false", s)
		}
	}
}

func TestCreateInputRejectsInvalidSlug(t *testing.T) {
	in := CreateInput{Type: "page", Slug: "/evil.example", Title: "T"}
	if err := in.Validate(); !errors.Is(err, ErrInvalidSlug) || !errors.Is(err, ErrInvalid) {
		t.Fatalf("Validate() = %v, want ErrInvalidSlug (wrapping ErrInvalid)", err)
	}
}
