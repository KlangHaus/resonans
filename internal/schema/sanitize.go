package schema

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// richTextPolicy is the server-side twin of the Studio's sanitizeRichText
// (studio/app/utils/sanitize.ts): the same formatting tags, `href` on links
// only, and only absolute http(s)/mailto URLs. Keep the two lists identical.
//
// Why both: the Studio sanitizes what it renders, but a body can be written
// straight to the API. Rich text that reached an editor's Studio unsanitized
// ran <img onerror> in the Studio origin, and from there read another tenant's
// documents via the dev header (measured, riff hq713scnn7ciernhu8v63b0f).
var richTextPolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "div", "br", "strong", "b", "em", "i", "u", "s",
		"h2", "h3", "h4", "ul", "ol", "li", "blockquote", "a", "code", "pre")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.AllowRelativeURLs(false)
	p.RequireParseableURLs(true)
	return p
}()

var safeHref = regexp.MustCompile(`(?i)^(?:https?:|mailto:)`)

// SanitizeRichText returns html reduced to the allowed formatting markup.
func SanitizeRichText(html string) string { return richTextPolicy.Sanitize(html) }

// SanitizeDocument returns the body with rich-text HTML sanitized. It runs after
// ValidateDocument, so the body's shape is known to match the schema.
//
// 🔴 Only strings that ARE HTML go through the HTML sanitizer. bluemonday
// escapes text (`'` → `&#39;`, `&` → `&amp;`), and consumers that render plain
// text escape again, so running it on plain text corrupts every save,
// permanently (idempotent). Measured by [sikkerhed] on the first version of
// this function. A string is HTML when:
//   - it is a `rich_text` block's value and the block declares format "html", or
//   - it is a richtext field OUTSIDE blocks (the Studio emits HTML there).
//
// Everything else stays byte-identical, and that is safe because nothing renders
// it as HTML unsanitized: the Studio sanitizes at render whatever the format,
// Etude renders non-html rich text as text, and blog escapes all but "html".
// Structured (portable-text-like) rich text keeps its text; only a `href` that
// is not http(s)/mailto is removed.
func SanitizeDocument(r *Registry, docType string, body json.RawMessage) (json.RawMessage, error) {
	dt, ok := r.DocType(docType)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownType, docType)
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("%w: body is not a JSON object: %w", ErrInvalid, err)
	}
	if err := sanitizeFields(&walk{}, r, dt.Fields, obj, false, false, docType); err != nil {
		return nil, err
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("schema: marshal sanitized body: %w", err)
	}
	return out, nil
}

// walk carries how a non-string in HTML position is handled: rejected (the API,
// on save) or blanked and recorded (the backfill, for rows already stored).
type walk struct {
	blank   bool
	blanked []string
}

// inBlock: the fields belong to a block (or something nested in one).
// htmlFormat: that block is a rich_text block declaring format "html".
func sanitizeFields(w *walk, r *Registry, fields []Field, obj map[string]any, inBlock, htmlFormat bool, path string) error {
	for _, f := range fields {
		v, ok := obj[f.Name]
		if !ok || v == nil {
			continue
		}
		clean, err := sanitizeValue(w, r, f, v, inBlock, htmlFormat, path+"."+f.Name)
		if err != nil {
			return err
		}
		obj[f.Name] = clean
	}
	return nil
}

func sanitizeValue(w *walk, r *Registry, f Field, v any, inBlock, htmlFormat bool, path string) (any, error) {
	switch f.Type {
	case TypeRichText:
		isHTML := !inBlock || htmlFormat
		if f.I18n {
			if m, ok := v.(map[string]any); ok {
				for loc, lv := range m {
					clean, err := w.sanitizeRich(lv, isHTML, path+"."+loc)
					if err != nil {
						return nil, err
					}
					m[loc] = clean
				}
				return m, nil
			}
		}
		return w.sanitizeRich(v, isHTML, path)
	case TypeArray:
		if arr, ok := v.([]any); ok {
			for i, el := range arr {
				if o, ok := el.(map[string]any); ok {
					if err := sanitizeFields(w, r, f.Of, o, inBlock, htmlFormat, fmt.Sprintf("%s[%d]", path, i)); err != nil {
						return nil, err
					}
				}
			}
		}
	case TypeBlocks:
		if arr, ok := v.([]any); ok {
			for i, el := range arr {
				blk, ok := el.(map[string]any)
				if !ok {
					continue
				}
				bt, _ := blk["_type"].(string)
				def, ok := r.Block(bt)
				if !ok {
					continue
				}
				format, _ := blk["format"].(string)
				if err := sanitizeFields(w, r, def.Fields, blk, true, bt == "rich_text" && format == "html", fmt.Sprintf("%s[%d]:%s", path, i, bt)); err != nil {
					return nil, err
				}
			}
		}
	}
	return v, nil
}

// sanitizeRich handles both rich-text forms.
//
// 🔴 In HTML position ONLY A STRING is accepted. A consumer that renders HTML
// does `String(value)` — an array ["<img onerror>", "<p>ok</p>"] becomes markup
// that no string check ever looked at ([sikkerhed], bypass measured on the
// previous version). So any other JSON type there is rejected (400), not
// guessed at. Outside HTML position structured data keeps its text; only an
// unsafe href is removed.
func (w *walk) sanitizeRich(v any, isHTML bool, path string) (any, error) {
	if isHTML {
		s, ok := v.(string)
		if !ok && w.blank {
			w.blanked = append(w.blanked, path+" ("+jsonType(v)+")")
			return "", nil
		}
		if !ok {
			return nil, fmt.Errorf("%w: %s is HTML rich text and must be a string, got %s", ErrInvalid, path, jsonType(v))
		}
		return SanitizeRichText(s), nil
	}
	return stripUnsafeHrefs(v), nil
}

func stripUnsafeHrefs(v any) any {
	switch t := v.(type) {
	case []any:
		for i, el := range t {
			t[i] = stripUnsafeHrefs(el)
		}
	case map[string]any:
		for k, el := range t {
			if k == "href" {
				if s, ok := el.(string); !ok || !safeHref.MatchString(strings.TrimSpace(s)) {
					delete(t, k)
				}
				continue
			}
			t[k] = stripUnsafeHrefs(el)
		}
	}
	return v
}

func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case float64:
		return "number"
	case bool:
		return "boolean"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// RepairResult describes what RepairDocument did to a stored body.
type RepairResult struct {
	Body    json.RawMessage
	Changed bool
	// Blanked lists the paths (with the JSON type found) of non-string values
	// in HTML position that were replaced by "" — the named handling for data
	// the API now rejects on save. Never the values themselves.
	Blanked []string
}

// RepairDocument is SanitizeDocument for a body that is ALREADY stored: instead
// of rejecting a non-string in HTML position (which would leave the row as it
// is, dirty), it blanks the value and records where. Changed compares the
// canonical JSON of input and output, so a clean row is reported unchanged and
// a second run changes nothing (idempotent).
func RepairDocument(r *Registry, docType string, body json.RawMessage) (RepairResult, error) {
	dt, ok := r.DocType(docType)
	if !ok {
		return RepairResult{}, fmt.Errorf("%w: %q", ErrUnknownType, docType)
	}
	var obj, orig map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return RepairResult{}, fmt.Errorf("%w: body is not a JSON object: %w", ErrInvalid, err)
	}
	_ = json.Unmarshal(body, &orig)
	w := &walk{blank: true}
	if err := sanitizeFields(w, r, dt.Fields, obj, false, false, docType); err != nil {
		return RepairResult{}, err
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return RepairResult{}, fmt.Errorf("schema: marshal repaired body: %w", err)
	}
	before, _ := json.Marshal(orig)
	return RepairResult{Body: out, Changed: string(before) != string(out), Blanked: w.blanked}, nil
}
