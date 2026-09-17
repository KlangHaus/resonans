package schema

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
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

// The walk sanitizes HTML only where the string IS HTML (rich_text with format
// "html"), reaches it through i18n maps and nested blocks, and leaves plain text
// byte-identical — the correctness bug [sikkerhed] measured on the first version.
func TestSanitizeDocumentOnlyDeclaredHTML(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	const plain = `Husk: 'organisk' betyder ikke "levende" & 2 < 3 > 1`
	body := map[string]any{
		"seoTitle": map[string]any{"da": plain},
		"body": []any{
			map[string]any{
				"_type": "rich_text", "_key": "k1", "format": "html",
				"value": map[string]any{"da": `<p>ok</p><img src=x onerror=alert(1)>`, "en": `<script>x</script><p>en & 'q'</p>`},
			},
			map[string]any{"_type": "rich_text", "format": "portable_text", "value": map[string]any{"da": plain + ` <img src=x onerror=alert(1)>`}},
			map[string]any{"_type": "rich_text", "value": map[string]any{"da": plain}},
			map[string]any{"_type": "callout", "value": map[string]any{"da": plain}},
			map[string]any{"_type": "columns", "columns": []any{map[string]any{"content": []any{
				map[string]any{"_type": "rich_text", "format": "html", "value": map[string]any{"da": `<img src=y onerror=alert(2)><p>nested</p>`}},
			}}}},
		},
	}
	raw, _ := json.Marshal(body)
	out, err := SanitizeDocument(r, "page", raw)
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
	val := func(i int, loc string) string {
		return got.Body[i]["value"].(map[string]any)[loc].(string)
	}
	// Cell 1: declared HTML is sanitized (also nested in columns).
	if val(0, "da") != "<p>ok</p>" {
		t.Errorf("html da = %q", val(0, "da"))
	}
	if strings.Contains(val(0, "en"), "script") || !strings.Contains(val(0, "en"), "<p>en") {
		t.Errorf("html en = %q", val(0, "en"))
	}
	nested := got.Body[4]["columns"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["value"].(map[string]any)["da"]
	if nested != "<p>nested</p>" {
		t.Errorf("nested html = %q", nested)
	}
	// Cell 2 + 3: non-HTML rich text and plain fields are byte-identical,
	// including ' & " < > — and the portable_text <img> stays TEXT, not markup.
	if val(1, "da") != plain+` <img src=x onerror=alert(1)>` {
		t.Errorf("portable_text changed: %q", val(1, "da"))
	}
	if val(2, "da") != plain || val(3, "da") != plain || got.SeoTitle["da"] != plain {
		t.Errorf("plain text changed: rich_text=%q callout=%q seo=%q", val(2, "da"), val(3, "da"), got.SeoTitle["da"])
	}
	if got.Body[0]["_key"] != "k1" {
		t.Errorf("_key dropped")
	}
}

// Structured rich text: unsafe hrefs removed, text untouched.
func TestSanitizeStructuredRichTextHref(t *testing.T) {
	v, _ := (&walk{}).sanitizeRich([]any{map[string]any{
		"markDefs": []any{
			map[string]any{"_type": "link", "href": "javascript:alert(1)"},
			map[string]any{"_type": "link", "href": " javascript:alert(1)"},
			map[string]any{"_type": "link", "href": "https://ok.example"},
		},
		"children": []any{map[string]any{"text": "Tom & Jerry <3 'x'"}},
	}}, false, "t")
	b, _ := json.Marshal(v)
	s := string(b)
	if strings.Contains(s, "javascript:") {
		t.Errorf("unsafe href kept: %s", s)
	}
	if !strings.Contains(s, "https://ok.example") {
		t.Errorf("safe href removed: %s", s)
	}
	if !strings.Contains(s, `Tom \u0026 Jerry \u003c3 'x'`) {
		t.Errorf("structured text changed: %s", s)
	}
}

// In HTML position every JSON type other than string is rejected, wherever the
// position is: i18n map, plain value, nested in columns, or rich text outside
// blocks. Outside HTML position the same values are accepted unchanged.
func TestHTMLPositionRejectsNonStrings(t *testing.T) {
	blocksReg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	topReg, err := Load(fstest.MapFS{
		"types/article.yaml": {Data: []byte("type: article\nfields:\n  - { name: intro, type: richtext, i18n: true }\n  - { name: note, type: richtext }\n")},
		"blocks.yaml":        {Data: []byte("blocks: []\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	nonStrings := map[string]any{
		"array":   []any{"<img src=x onerror=alert(1)>", "<p>ok</p>"},
		"object":  map[string]any{"html": "<img src=x onerror=alert(1)>"},
		"number":  float64(1),
		"boolean": true,
		"null":    nil,
	}
	for name, v := range nonStrings {
		cases := map[string]struct {
			reg     *Registry
			docType string
			body    map[string]any
		}{
			"i18n html block": {blocksReg, "page", map[string]any{"body": []any{
				map[string]any{"_type": "rich_text", "format": "html", "value": map[string]any{"da": v}},
			}}},
			"nested in columns": {blocksReg, "page", map[string]any{"body": []any{
				map[string]any{"_type": "columns", "columns": []any{map[string]any{"content": []any{
					map[string]any{"_type": "rich_text", "format": "html", "value": map[string]any{"da": v}},
				}}}},
			}}},
			"top-level i18n richtext": {topReg, "article", map[string]any{"intro": map[string]any{"da": v}}},
		}
		if name != "null" { // a field-level null is an absent value, not HTML
			cases["top-level plain richtext"] = struct {
				reg     *Registry
				docType string
				body    map[string]any
			}{topReg, "article", map[string]any{"note": v}}
		}
		for where, c := range cases {
			raw, _ := json.Marshal(c.body)
			if _, err := SanitizeDocument(c.reg, c.docType, raw); !errors.Is(err, ErrInvalid) {
				t.Errorf("%s / %s: err = %v, want ErrInvalid", name, where, err)
			}
		}
		// Same value outside HTML position (portable_text) is accepted.
		raw, _ := json.Marshal(map[string]any{"body": []any{
			map[string]any{"_type": "rich_text", "format": "portable_text", "value": map[string]any{"da": v}},
		}})
		if _, err := SanitizeDocument(blocksReg, "page", raw); err != nil {
			t.Errorf("%s / portable_text: err = %v, want accepted", name, err)
		}
	}
}
