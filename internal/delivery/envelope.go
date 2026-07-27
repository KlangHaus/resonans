package delivery

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/schema"
)

// AssetResolver expands a stored asset id to the delivery asset shape
// (`{url, width?, height?}`) so Nitro renders ready URLs (Bunny CDN). Wired from
// the media service in app.go.
type AssetResolver interface {
	ResolveAsset(ctx context.Context, tenantID uuid.UUID, assetID string) (map[string]any, bool)
}

// Envelope is the [etude]-locked delivery shape (§D): metadata + resolved
// `data` (non-block fields) + `components` (the doc-type's block field). i18n
// fields are resolved to the requested locale (LocaleFallback flags any fallback
// to the default locale); asset ids are expanded to CDN URLs.
type Envelope struct {
	ID             string           `json:"id"`
	Type           string           `json:"type"`
	Slug           string           `json:"slug"`
	Locale         string           `json:"locale"`
	Version        int              `json:"version"`
	PublishedAt    *time.Time       `json:"publishedAt,omitempty"`
	Data           map[string]any   `json:"data"`
	Components     []map[string]any `json:"components,omitempty"`
	LocaleFallback bool             `json:"localeFallback,omitempty"`
}

// BuildEnvelope turns a published Document into the delivery envelope for a
// locale. Unknown doc-types (not in the registry) yield a passthrough data map.
func BuildEnvelope(ctx context.Context, tenantID uuid.UUID, doc content.Document, reg *schema.Registry, locale, defaultLocale string, assets AssetResolver) Envelope {
	env := Envelope{
		ID: doc.ID.String(), Type: doc.Type, Slug: doc.Slug,
		Locale: locale, Version: doc.Version, PublishedAt: doc.PublishedAt,
		Data: map[string]any{},
	}
	var body map[string]any
	if err := json.Unmarshal(doc.Body, &body); err != nil {
		return env
	}
	if reg == nil {
		env.Data = body // no schema wired (dev): passthrough
		return env
	}
	dt, ok := reg.DocType(doc.Type)
	if !ok {
		env.Data = body // best-effort passthrough for unknown types
		return env
	}
	rc := &resolveCtx{ctx: ctx, tenantID: tenantID, reg: reg, locale: locale, def: defaultLocale, assets: assets}
	for _, f := range dt.Fields {
		v, ok := body[f.Name]
		if !ok || v == nil {
			continue
		}
		if f.Type == schema.TypeBlocks {
			env.Components = rc.resolveBlocks(v)
			continue
		}
		env.Data[f.Name] = rc.resolveValue(f, v)
	}
	// The document title is a metadata column (not a body field); surface it in
	// data so consumers (Etude course/lesson/summary) have it.
	if _, exists := env.Data["title"]; !exists {
		env.Data["title"] = doc.Title
	}
	env.LocaleFallback = rc.fellBack
	return env
}

type resolveCtx struct {
	ctx      context.Context
	tenantID uuid.UUID
	reg      *schema.Registry
	locale   string
	def      string
	assets   AssetResolver
	fellBack bool
}

func (rc *resolveCtx) resolveValue(f schema.Field, v any) any {
	if f.I18n && isI18nScalar(f.Type) {
		return rc.resolveI18n(v)
	}
	switch f.Type {
	case schema.TypeAsset:
		if id, ok := v.(string); ok && rc.assets != nil {
			if a, ok := rc.assets.ResolveAsset(rc.ctx, rc.tenantID, id); ok {
				return a
			}
		}
		return v
	case schema.TypeReference:
		if id, ok := v.(string); ok {
			return map[string]any{"ref": map[string]any{"type": f.Ref, "id": id}}
		}
		return v
	case schema.TypeArray:
		return rc.resolveArray(f, v)
	default:
		return v
	}
}

// resolveI18n picks the requested locale, falling back to the default (and
// flagging it) then to any available value.
func (rc *resolveCtx) resolveI18n(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if val, ok := m[rc.locale]; ok {
		return val
	}
	if val, ok := m[rc.def]; ok {
		rc.fellBack = true
		return val
	}
	rc.fellBack = true
	for _, val := range m {
		return val
	}
	return nil
}

func (rc *resolveCtx) resolveArray(f schema.Field, v any) any {
	arr, ok := v.([]any)
	if !ok {
		return v
	}
	out := make([]any, 0, len(arr))
	for _, el := range arr {
		if obj, ok := el.(map[string]any); ok {
			out = append(out, rc.resolveFields(f.Of, obj))
		} else {
			out = append(out, el)
		}
	}
	return out
}

func (rc *resolveCtx) resolveBlocks(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, el := range arr {
		blk, ok := el.(map[string]any)
		if !ok {
			continue
		}
		bt, _ := blk["_type"].(string)
		def, ok := rc.reg.Block(bt)
		if !ok {
			out = append(out, blk) // unknown block → raw passthrough (Etude renders a graceful fallback)
			continue
		}
		out = append(out, rc.resolveFields(def.Fields, blk))
	}
	return out
}

// resolveFields resolves each declared field of an object, preserving editor
// meta keys (_type, _key).
func (rc *resolveCtx) resolveFields(fields []schema.Field, obj map[string]any) map[string]any {
	out := make(map[string]any, len(obj))
	for k, v := range obj {
		if strings.HasPrefix(k, "_") {
			out[k] = v
		}
	}
	for _, f := range fields {
		v, ok := obj[f.Name]
		if !ok || v == nil {
			continue
		}
		if f.Type == schema.TypeBlocks {
			out[f.Name] = rc.resolveBlocks(v)
			continue
		}
		out[f.Name] = rc.resolveValue(f, v)
	}
	return out
}

// isI18nScalar mirrors schema.isI18nScalar (unexported there): i18n applies to
// string/text/richtext.
func isI18nScalar(t schema.FieldType) bool {
	return t == schema.TypeString || t == schema.TypeText || t == schema.TypeRichText
}
