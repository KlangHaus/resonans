package schema

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Relabeling unlabeled rich text.
//
// The interim Studio editor wrote HTML into rich_text blocks WITHOUT setting
// format, and an absent format means portable_text (blocks.yaml:10). Those
// rows therefore never went through the HTML sanitizer: the structured path
// only strips unsafe hrefs. RepairDocument labels them so they do.
//
// 🔴 THE FAILURE DIRECTIONS ARE OPPOSITE, WHICH IS WHY THIS GUESSES AT
// NOTHING. A false negative leaves unsanitized HTML on the structured path --
// the security gap stays open. A false positive labels an author's plain text
// as HTML and then sanitizes it, which DESTROYS CONTENT. So a value is
// relabeled only when every non-empty translation is measurably HTML;
// anything in between is left untouched and reported for a human.

// richTextShape is what a stored rich-text value turns out to be.
type richTextShape int

const (
	shapePlain richTextShape = iota // no translation contains an HTML element
	shapeHTML                       // every non-empty translation does
	shapeMixed                      // some do, some do not: one format cannot describe both
)

func (s richTextShape) String() string {
	switch s {
	case shapeHTML:
		return "html"
	case shapeMixed:
		return "mixed"
	default:
		return "plain"
	}
}

// containsHTMLElement reports whether s contains at least one real HTML
// element, as the HTML parser itself sees it rather than as a regex guesses.
//
// It returns an ERROR instead of false when it cannot parse, because "false"
// has to mean "measured as plain text" and not "the instrument broke" -- the
// broken case must not be silently indistinguishable from the safe-looking
// one. Measured against both halves of a corpus (relabel_test.go): bare `<`,
// `<3`, `a < b`, entity-only text and the empty string are NOT elements;
// `<p>`, `<img onerror>`, `<script>` and unknown tag names ARE.
//
// Declared limit: `a<b and c>d` is reported as HTML, because `b` is a real
// element name and the parser is entitled to read it as a tag. Content that
// is only HTML ENTITIES with no tag at all (`2 &lt; 3`) is reported as plain,
// so it keeps portable_text and a consumer renders the entity literally. That
// is a rendering flaw, not a security one, and guessing would risk mangling
// plain text that legitimately contains `&amp;`.
func containsHTMLElement(s string) (bool, error) {
	if s == "" {
		return false, nil
	}
	ctx := &html.Node{Type: html.ElementNode, Data: atom.Body.String(), DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(s), ctx)
	if err != nil {
		return false, fmt.Errorf("schema: parse stored rich text: %w", err)
	}
	var found bool
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			found = true
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	for _, n := range nodes {
		visit(n)
	}
	return found, nil
}

// classifyRichText measures the shape of a block's rich-text value(s). The
// field list is the SCHEMA's, so the field name and its i18n flag are read
// rather than assumed: a rich_text block declares value/richtext/i18n today
// (blocks.yaml:7-9), and if that is renamed this follows it.
//
// Only a STRING can be mislabeled HTML; a structured portable-text array or
// object counts as non-HTML, so a block holding one is never relabeled.
func classifyRichText(fields []Field, blk map[string]any) (richTextShape, error) {
	var htmlCount, otherCount, richFields int
	count := func(x any) error {
		s, ok := x.(string)
		if !ok {
			if x != nil {
				otherCount++
			}
			return nil
		}
		if s == "" {
			return nil
		}
		isHTML, err := containsHTMLElement(s)
		if err != nil {
			return err
		}
		if isHTML {
			htmlCount++
		} else {
			otherCount++
		}
		return nil
	}
	for _, f := range fields {
		if f.Type != TypeRichText {
			continue
		}
		richFields++
		v, ok := blk[f.Name]
		if !ok || v == nil {
			continue
		}
		if f.I18n {
			if m, ok := v.(map[string]any); ok {
				for _, lv := range m {
					if err := count(lv); err != nil {
						return shapePlain, err
					}
				}
				continue
			}
		}
		if err := count(v); err != nil {
			return shapePlain, err
		}
	}
	if richFields == 0 {
		// The block type has no rich-text field, so there is nothing a format
		// could describe. Reached only if the schema changed under this code.
		return shapePlain, nil
	}
	switch {
	case htmlCount > 0 && otherCount == 0:
		return shapeHTML, nil
	case htmlCount > 0:
		return shapeMixed, nil
	default:
		return shapePlain, nil
	}
}

// relabel sets format on an unlabeled rich_text block whose value measures as
// HTML, so the caller then routes it through the HTML sanitizer. A mixed block
// and an unparseable one are left exactly as they are and recorded instead:
// one format cannot describe two shapes, and that is a decision for a person.
//
// Recorded entries are PATHS ONLY, never content -- the backfill's report is
// read in a terminal and copied into tickets.
func (w *walk) relabel(fields []Field, blk map[string]any, path string) {
	shape, err := classifyRichText(fields, blk)
	switch {
	case err != nil:
		w.ambiguous = append(w.ambiguous, fmt.Sprintf("%s (unparseable: %v)", path, err))
	case shape == shapeHTML:
		blk["format"] = formatHTML
		w.labeled = append(w.labeled, path)
	case shape == shapeMixed:
		w.ambiguous = append(w.ambiguous, fmt.Sprintf("%s (mixed: some translations are HTML, some are not)", path))
	}
}
