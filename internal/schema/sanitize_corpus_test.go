package schema

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// corpusCase is one entry of testdata/sanitize-corpus.json — the corpus the
// editor's own sanitizer (studio/app/utils/sanitize.ts) runs too, so the two
// policies cannot drift apart unnoticed.
type corpusCase struct {
	Name           string   `json:"name"`
	Input          string   `json:"input"`
	MustContain    []string `json:"mustContain"`
	MustNotContain []string `json:"mustNotContain"`
}

func TestSanitizeRichTextCorpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/sanitize-corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []corpusCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	// A corpus that loaded nothing would pass every assertion below.
	if len(corpus.Cases) < 10 {
		t.Fatalf("corpus has %d cases; refusing to report green on a near-empty table", len(corpus.Cases))
	}
	for _, c := range corpus.Cases {
		t.Run(c.Name, func(t *testing.T) {
			out := SanitizeRichText(c.Input)
			for _, want := range c.MustContain {
				if !strings.Contains(out, want) {
					t.Errorf("lost %q\n  in:  %s\n  out: %s", want, c.Input, out)
				}
			}
			for _, bad := range c.MustNotContain {
				if strings.Contains(strings.ToLower(out), strings.ToLower(bad)) {
					t.Errorf("kept %q\n  in:  %s\n  out: %s", bad, c.Input, out)
				}
			}
			if again := SanitizeRichText(out); again != out {
				t.Errorf("not idempotent:\n  once:  %s\n  twice: %s", out, again)
			}
		})
	}
}
