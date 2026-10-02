package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

// Both halves of the detector's corpus. The accept set is the positive
// control, and it is not optional: an earlier version of this detector
// returned false for EVERY input, which made all six reject cases pass while
// the thing labeled nothing at all.
func TestContainsHTMLElement(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
		why  string
	}{
		{`Husk: 'organisk' betyder ikke "levende" & 2 < 3 > 1`, false, "plain text with bare < and >"},
		{`Hej`, false, "plain text"},
		{`jeg <3 dig`, false, "a heart is not a tag"},
		{`2 &lt; 3 &amp; 4`, false, "entities only, no tag"},
		{`a < b og c > d`, false, "space after <"},
		{``, false, "empty"},
		{`<p>ok</p>`, true, "real HTML"},
		{`<p>a</p><strong>b</strong>`, true, "interim editor output"},
		{`<img src=x onerror=alert(1)>`, true, "dangerous HTML must be labeled"},
		{`<script>x</script>`, true, "dangerous HTML"},
		{`<a href="javascript:alert(1)">x</a>`, true, "dangerous href"},
		{`a<b and c>d`, true, "DECLARED false positive: b is a real element name"},
		{`<ikkeetelement>x</ikkeetelement>`, true, "the parser makes unknown names elements"},
	} {
		got, err := containsHTMLElement(c.in)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.why, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: containsHTMLElement = %v, want %v", c.why, got, c.want)
		}
	}
}

func repair(t *testing.T, body map[string]any) RepairResult {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	res, err := RepairDocument(r, "page", raw)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	return res
}

func block(format any, value map[string]any) map[string]any {
	blk := map[string]any{"_type": "rich_text", "value": value}
	if format != nil {
		blk["format"] = format
	}
	return blk
}

func firstValue(t *testing.T, res RepairResult, loc string) (string, string) {
	t.Helper()
	var got struct {
		Body []map[string]any `json:"body"`
	}
	if err := json.Unmarshal(res.Body, &got); err != nil {
		t.Fatal(err)
	}
	format, _ := got.Body[0]["format"].(string)
	v, _ := got.Body[0]["value"].(map[string]any)[loc].(string)
	return format, v
}

// The row class this backfill exists for: the interim editor wrote HTML and no
// format, so the value never went through the HTML sanitizer. Labeling alone
// would leave it dirty while claiming it is HTML, so both must happen in one
// pass.
func TestRepairLabelsAndSanitizesUnlabeledHTML(t *testing.T) {
	res := repair(t, map[string]any{"body": []any{
		block(nil, map[string]any{"da": `<p>ok</p><img src=x onerror=alert(1)>`}),
	}})
	format, value := firstValue(t, res, "da")
	if format != formatHTML {
		t.Errorf("format = %q, want %q", format, formatHTML)
	}
	if strings.Contains(value, "onerror") {
		t.Errorf("labeled but NOT sanitized: %q", value)
	}
	if value != "<p>ok</p>" {
		t.Errorf("value = %q, want <p>ok</p>", value)
	}
	if !res.Changed {
		t.Error("Changed = false")
	}
	if len(res.Labeled) != 1 {
		t.Errorf("Labeled = %v, want 1 path", res.Labeled)
	}
	if len(res.Ambiguous) != 0 {
		t.Errorf("Ambiguous = %v, want none", res.Ambiguous)
	}
	// Idempotent: a second pass over the repaired body changes nothing.
	var again map[string]any
	if err := json.Unmarshal(res.Body, &again); err != nil {
		t.Fatal(err)
	}
	if second := repair(t, again); second.Changed || len(second.Labeled) != 0 {
		t.Errorf("second pass: Changed=%v Labeled=%v, want false/0", second.Changed, second.Labeled)
	}
}

// The accept set of the backfill itself: rows it must NOT touch. Without
// these, a detector that labeled everything would look correct.
func TestRepairLeavesTheseAlone(t *testing.T) {
	const plain = `Husk: 'organisk' betyder ikke "levende" & 2 < 3 > 1`
	for _, c := range []struct {
		name  string
		blk   map[string]any
		wantF string
	}{
		{"plain text, no format", block(nil, map[string]any{"da": plain}), ""},
		{"writer declared portable_text, HTML-looking value", block("portable_text", map[string]any{"da": `<p>x</p>`}), "portable_text"},
		{"structured value, no format", block(nil, map[string]any{"da": []any{map[string]any{"children": []any{map[string]any{"text": "x"}}}}}), ""},
	} {
		res := repair(t, map[string]any{"body": []any{c.blk}})
		format, _ := firstValue(t, res, "da")
		if format != c.wantF {
			t.Errorf("%s: format = %q, want %q", c.name, format, c.wantF)
		}
		if len(res.Labeled) != 0 {
			t.Errorf("%s: relabeled %v", c.name, res.Labeled)
		}
		if res.Changed {
			t.Errorf("%s: Changed = true, want byte-identical", c.name)
		}
	}
}

// One format cannot describe two shapes, so a mixed block is REFUSED and
// reported rather than guessed at. It stays byte-identical, which is why the
// backfill counts Ambiguous before its unchanged return.
func TestRepairRefusesMixedTranslations(t *testing.T) {
	res := repair(t, map[string]any{"body": []any{
		block(nil, map[string]any{"da": `<p>dansk</p>`, "en": `plain english & 2 < 3`}),
	}})
	format, _ := firstValue(t, res, "da")
	if format != "" {
		t.Errorf("format = %q, want it left unset", format)
	}
	if len(res.Labeled) != 0 {
		t.Errorf("Labeled = %v, want none", res.Labeled)
	}
	if len(res.Ambiguous) != 1 {
		t.Fatalf("Ambiguous = %v, want 1 path", res.Ambiguous)
	}
	if !strings.Contains(res.Ambiguous[0], "mixed") {
		t.Errorf("Ambiguous reason = %q, want it to say mixed", res.Ambiguous[0])
	}
	if res.Changed {
		t.Error("Changed = true: a refused row must stay byte-identical")
	}
}

// 🔴 NEGATIVE CONTROL ON THE SWITCH ITSELF. Relabeling is repair-only: a live
// save must never have its format rewritten under it, because the writer
// either declared one or chose not to. If SanitizeDocument ever relabels, a
// request's own declaration stops being authoritative.
func TestSanitizeDocumentNeverRelabels(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"body": []any{
		block(nil, map[string]any{"da": `<p>ok</p><img src=x onerror=alert(1)>`}),
	}})
	out, err := SanitizeDocument(r, "page", raw)
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	var got struct {
		Body []map[string]any `json:"body"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if _, present := got.Body[0]["format"]; present {
		t.Errorf("SanitizeDocument added a format: %v", got.Body[0]["format"])
	}
	// And the value keeps the structured contract: HTML stays TEXT here.
	if v, _ := got.Body[0]["value"].(map[string]any)["da"].(string); !strings.Contains(v, "onerror") {
		t.Errorf("structured path altered the value: %q", v)
	}
}
