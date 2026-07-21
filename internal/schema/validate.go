package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalid is the base error for a body that does not satisfy its schema.
// ErrUnknownType is returned when the doc-type is not in the registry. httpapi
// maps both to 400.
var (
	ErrInvalid     = errors.New("schema: document invalid")
	ErrUnknownType = errors.New("schema: unknown doc-type")
)

// ValidateDocument checks that a document's JSONB body satisfies its doc-type
// schema: required fields present, no unknown fields, per-field type + i18n
// shape, and — for blocks fields — every block's `_type` is permitted by the
// allowlist and its fields validate (recursing into nested blocks/arrays).
//
// Keys beginning with "_" (e.g. `_type`, `_key`) are editor/meta keys and are
// ignored by the unknown-field check.
func ValidateDocument(r *Registry, docType string, body json.RawMessage) error {
	dt, ok := r.DocType(docType)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownType, docType)
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return fmt.Errorf("%w: body is not a JSON object: %w", ErrInvalid, err)
	}
	return validateFields(r, dt.Fields, obj, docType)
}

func validateFields(r *Registry, fields []Field, obj map[string]any, path string) error {
	allowed := make(map[string]bool, len(fields))
	for _, f := range fields {
		allowed[f.Name] = true
	}
	for k := range obj {
		if strings.HasPrefix(k, "_") {
			continue // editor/meta key (_type, _key, ...)
		}
		if !allowed[k] {
			return fmt.Errorf("%w: unknown field %q at %s", ErrInvalid, k, path)
		}
	}
	for _, f := range fields {
		v, present := obj[f.Name]
		if !present || v == nil {
			if f.Required {
				return fmt.Errorf("%w: missing required field %q at %s", ErrInvalid, f.Name, path)
			}
			continue
		}
		if err := validateValue(r, f, v, path+"."+f.Name); err != nil {
			return err
		}
	}
	return nil
}

func validateValue(r *Registry, f Field, v any, path string) error {
	// Field-level i18n: translated scalars carry a locale-keyed object.
	if f.I18n && isI18nScalar(f.Type) {
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf("%w: field %s must be an i18n object (locale-keyed)", ErrInvalid, path)
		}
		return nil
	}

	switch f.Type {
	case TypeString, TypeText:
		if _, ok := v.(string); !ok {
			return fmt.Errorf("%w: field %s must be a string", ErrInvalid, path)
		}
	case TypeRichText:
		// Lenient: rich text may be a string or structured (portable-text-like).
	case TypeNumber:
		if _, ok := v.(float64); !ok {
			return fmt.Errorf("%w: field %s must be a number", ErrInvalid, path)
		}
	case TypeBoolean:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%w: field %s must be a boolean", ErrInvalid, path)
		}
	case TypeAsset, TypeReference:
		if _, ok := v.(string); !ok {
			return fmt.Errorf("%w: field %s must be a %s reference (string id)", ErrInvalid, path, f.Type)
		}
	case TypeArray:
		return validateArray(r, f, v, path)
	case TypeBlocks:
		return validateBlocks(r, f, v, path)
	default:
		return fmt.Errorf("%w: field %s has unsupported type %q", ErrInvalid, path, f.Type)
	}
	return nil
}

func validateArray(r *Registry, f Field, v any, path string) error {
	arr, ok := v.([]any)
	if !ok {
		return fmt.Errorf("%w: field %s must be an array", ErrInvalid, path)
	}
	for i, el := range arr {
		obj, ok := el.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %s[%d] must be an object", ErrInvalid, path, i)
		}
		if err := validateFields(r, f.Of, obj, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateBlocks(r *Registry, f Field, v any, path string) error {
	arr, ok := v.([]any)
	if !ok {
		return fmt.Errorf("%w: field %s must be an array of blocks", ErrInvalid, path)
	}
	allow := make(map[string]bool, len(f.Allow))
	for _, name := range f.Allow {
		allow[name] = true
	}
	for i, el := range arr {
		blk, ok := el.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %s[%d] must be a block object", ErrInvalid, path, i)
		}
		bt, _ := blk["_type"].(string)
		if bt == "" {
			return fmt.Errorf("%w: %s[%d] missing _type", ErrInvalid, path, i)
		}
		if !allow[bt] {
			return fmt.Errorf("%w: %s[%d] block %q not allowed here", ErrInvalid, path, i, bt)
		}
		def, ok := r.Block(bt)
		if !ok {
			return fmt.Errorf("%w: %s[%d] unknown block %q", ErrInvalid, path, i, bt)
		}
		if err := validateFields(r, def.Fields, blk, fmt.Sprintf("%s[%d]:%s", path, i, bt)); err != nil {
			return err
		}
	}
	return nil
}

// isI18nScalar reports whether i18n translation applies to a field type.
func isI18nScalar(t FieldType) bool {
	return t == TypeString || t == TypeText || t == TypeRichText
}
