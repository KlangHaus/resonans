package schema

import (
	"encoding/json"
	"fmt"
	"regexp"

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

// SanitizeDocument returns the body with every rich-text value sanitized. It
// runs after ValidateDocument, so the body's shape is known to match the
// schema. Only rich text is markup: string/text fields are plain text that
// consumers must escape, and are left untouched.
func SanitizeDocument(r *Registry, docType string, body json.RawMessage) (json.RawMessage, error) {
	dt, ok := r.DocType(docType)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownType, docType)
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("%w: body is not a JSON object: %w", ErrInvalid, err)
	}
	sanitizeFields(r, dt.Fields, obj)
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("schema: marshal sanitized body: %w", err)
	}
	return out, nil
}

func sanitizeFields(r *Registry, fields []Field, obj map[string]any) {
	for _, f := range fields {
		v, ok := obj[f.Name]
		if !ok || v == nil {
			continue
		}
		obj[f.Name] = sanitizeValue(r, f, v)
	}
}

func sanitizeValue(r *Registry, f Field, v any) any {
	switch f.Type {
	case TypeRichText:
		if f.I18n {
			if m, ok := v.(map[string]any); ok {
				for loc, lv := range m {
					m[loc] = sanitizeRich(lv)
				}
				return m
			}
		}
		return sanitizeRich(v)
	case TypeArray:
		if arr, ok := v.([]any); ok {
			for _, el := range arr {
				if o, ok := el.(map[string]any); ok {
					sanitizeFields(r, f.Of, o)
				}
			}
		}
	case TypeBlocks:
		if arr, ok := v.([]any); ok {
			for _, el := range arr {
				blk, ok := el.(map[string]any)
				if !ok {
					continue
				}
				bt, _ := blk["_type"].(string)
				if def, ok := r.Block(bt); ok {
					sanitizeFields(r, def.Fields, blk)
				}
			}
		}
	}
	return v
}

// sanitizeRich handles both rich-text forms: an HTML string, or structured
// (portable-text-like) data, where the only URL-carrying values are `href`s.
func sanitizeRich(v any) any {
	switch t := v.(type) {
	case string:
		return SanitizeRichText(t)
	case []any:
		for i, el := range t {
			t[i] = sanitizeRich(el)
		}
	case map[string]any:
		for k, el := range t {
			if k == "href" {
				if s, ok := el.(string); !ok || !safeHref.MatchString(s) {
					delete(t, k)
				}
				continue
			}
			t[k] = sanitizeRich(el)
		}
	}
	return v
}
