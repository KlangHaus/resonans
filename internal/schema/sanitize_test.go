package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeRichTextCells(t *testing.T) {
	cases := []struct {
		name, in string
		mustNot  []string
		must     []string
	}{
		{"img onerror", `<p>x</p><img src=x onerror="alert(1)">`, []string{"onerror", "<img"}, []string{"<p>x</p>"}},
		{"svg onbegin", `<svg><animate onbegin="alert(1)"/></svg>`, []string{"onbegin", "<svg", "<animate"}, nil},
		{"script", `<p>a</p><script>alert(1)</script>`, []string{"<script", "alert"}, []string{"<p>a</p>"}},
		{"javascript href", `<a href="javascript:alert(1)">l</a>`, []string{"javascript:"}, nil},
		{"data href", `<a href="data:text/html,<script>x</script>">l</a>`, []string{"data:"}, nil},
		{"style and on* attrs", `<p style="x" onclick="y">t</p>`, []string{"style=", "onclick"}, []string{"<p>t</p>"}},
		{
			"formatting kept", `<h2>T</h2><p><strong>b</strong> <em>i</em> <a href="https://klanghaus.dk">l</a></p><ul><li>1</li></ul><blockquote>q</blockquote>`,
			nil,
			[]string{"<h2>T</h2>", "<strong>b</strong>", "<em>i</em>", `<a href="https://klanghaus.dk"`, "<li>1</li>", "<blockquote>q</blockquote>"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SanitizeRichText(c.in)
			for _, s := range c.mustNot {
				if strings.Contains(strings.ToLower(got), strings.ToLower(s)) {
					t.Errorf("output still contains %q: %s", s, got)
				}
			}
			for _, s := range c.must {
				if !strings.Contains(got, s) {
					t.Errorf("output lost %q: %s", s, got)
				}
			}
		})
	}
}

// The walk reaches rich text in i18n maps and inside blocks, and leaves plain
// string fields alone (they are text, escaped by consumers, not markup).
func TestSanitizeDocumentWalksSchema(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	body := `{
	  "seoTitle": {"da": "<b>not markup</b>"},
	  "body": [
	    {"_type": "rich_text", "_key": "k1", "value": {"da": "<p>ok</p><img src=x onerror=alert(1)>", "en": "<script>x</script><p>en</p>"}},
	    {"_type": "callout", "value": {"da": "<a href=\"javascript:alert(1)\">c</a>"}}
	  ]
	}`
	out, err := SanitizeDocument(r, "page", json.RawMessage(body))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		SeoTitle map[string]string `json:"seoTitle"`
		Body     []map[string]any  `json:"body"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	rich := got.Body[0]["value"].(map[string]any)
	if rich["da"] != "<p>ok</p>" || rich["en"] != "<p>en</p>" {
		t.Errorf("i18n rich text = %v, want da=<p>ok</p> en=<p>en</p>", rich)
	}
	if got.Body[0]["_key"] != "k1" {
		t.Errorf("_key dropped: %v", got.Body[0])
	}
	if c := got.Body[1]["value"].(map[string]any)["da"]; c != "c" {
		t.Errorf("callout (nested block) = %q, want the javascript: link removed", c)
	}
	if got.SeoTitle["da"] != "<b>not markup</b>" {
		t.Errorf("plain string field was changed: %q", got.SeoTitle["da"])
	}
}

func TestSanitizeStructuredRichTextHref(t *testing.T) {
	v := sanitizeRich([]any{map[string]any{
		"markDefs": []any{
			map[string]any{"_type": "link", "href": "javascript:alert(1)"},
			map[string]any{"_type": "link", "href": "https://ok.example"},
		},
		"children": []any{map[string]any{"text": "<img src=x onerror=1>"}},
	}})
	b, _ := json.Marshal(v)
	s := string(b)
	if strings.Contains(s, "javascript:") || strings.Contains(s, "onerror") {
		t.Errorf("structured rich text not sanitized: %s", s)
	}
	if !strings.Contains(s, "https://ok.example") {
		t.Errorf("safe href removed: %s", s)
	}
}
