// Package schema is the Resonans content schema-registry: the code-defined,
// declarative content model (Beslutning 4+5). Doc-types and the shared block
// library are authored as YAML in schema/ and loaded at boot. The registry
// drives (a) storage validation of a document's JSONB body (validate.go) and
// (b) editor-UI generation — the editor renders forms + the block palette FROM
// this registry, which is why the registry is the M5 content-enabler and the
// hard prerequisite for the Fase 4 editor.
//
// Resonans is broad and content-type-agnostic: Etude's LMS types (course,
// module, lesson) are one set of schemas; blog (post) + marketing (page) are
// others. The block library is shared across all doc-types; each doc-type's
// blocks field declares an `allow` list, so one library yields the right blocks
// per domain.
package schema

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FieldType enumerates the supported field kinds.
type FieldType string

const (
	TypeString    FieldType = "string"
	TypeText      FieldType = "text"
	TypeRichText  FieldType = "richtext"
	TypeNumber    FieldType = "number"
	TypeBoolean   FieldType = "boolean"
	TypeAsset     FieldType = "asset"     // reference to a media asset
	TypeReference FieldType = "reference" // reference to another document (Ref = target type)
	TypeBlocks    FieldType = "blocks"    // ordered block array (Allow = permitted block types)
	TypeArray     FieldType = "array"     // repeated object (Of = element sub-fields)
)

// Field describes one field of a doc-type or block.
type Field struct {
	Name     string    `yaml:"name"`
	Type     FieldType `yaml:"type"`
	I18n     bool      `yaml:"i18n"`     // field-level translation: value keyed by locale
	Required bool      `yaml:"required"` // must be present on a document
	Allow    []string  `yaml:"allow"`    // blocks: permitted block type names
	Ref      string    `yaml:"ref"`      // reference: target doc-type
	Of       []Field   `yaml:"of"`       // array: element sub-fields
}

// ListConfig declares which columns the editor's document list shows for a type.
type ListConfig struct {
	Columns []string `yaml:"columns"`
}

// DocType is a content type (course, lesson, page, post, ...).
type DocType struct {
	Type   string     `yaml:"type"`
	Fields []Field    `yaml:"fields"`
	List   ListConfig `yaml:"list"`
}

// BlockType is a reusable content block (rich-text, video-embed, hero, ...).
type BlockType struct {
	Name   string  `yaml:"name"`
	Fields []Field `yaml:"fields"`
}

// Registry holds the loaded, validated schema.
type Registry struct {
	docTypes map[string]DocType
	blocks   map[string]BlockType
}

// DocType looks up a doc-type by name.
func (r *Registry) DocType(name string) (DocType, bool) {
	dt, ok := r.docTypes[name]
	return dt, ok
}

// Block looks up a block type by name.
func (r *Registry) Block(name string) (BlockType, bool) {
	bt, ok := r.blocks[name]
	return bt, ok
}

// DocTypes returns all doc-types sorted by name (editor nav + schema API).
func (r *Registry) DocTypes() []DocType {
	out := make([]DocType, 0, len(r.docTypes))
	for _, dt := range r.docTypes {
		out = append(out, dt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// Blocks returns all block types sorted by name (editor block palette).
func (r *Registry) Blocks() []BlockType {
	out := make([]BlockType, 0, len(r.blocks))
	for _, b := range r.blocks {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// blockFile is the top-level shape of schema/blocks.yaml.
type blockFile struct {
	Blocks []BlockType `yaml:"blocks"`
}

// Load parses the block library (blocks.yaml) and every doc-type (types/*.yaml)
// from fsys, then checks referential consistency (block allowlists + reference
// targets resolve). It fails fast on any inconsistency.
func Load(fsys fs.FS) (*Registry, error) {
	r := &Registry{docTypes: map[string]DocType{}, blocks: map[string]BlockType{}}

	raw, err := fs.ReadFile(fsys, "blocks.yaml")
	if err != nil {
		return nil, fmt.Errorf("schema: read blocks.yaml: %w", err)
	}
	var bf blockFile
	if err := yaml.Unmarshal(raw, &bf); err != nil {
		return nil, fmt.Errorf("schema: parse blocks.yaml: %w", err)
	}
	for _, b := range bf.Blocks {
		if b.Name == "" {
			return nil, fmt.Errorf("schema: block with empty name")
		}
		if _, dup := r.blocks[b.Name]; dup {
			return nil, fmt.Errorf("schema: duplicate block %q", b.Name)
		}
		r.blocks[b.Name] = b
	}

	entries, err := fs.ReadDir(fsys, "types")
	if err != nil {
		return nil, fmt.Errorf("schema: read types dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		raw, err := fs.ReadFile(fsys, "types/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("schema: read %s: %w", e.Name(), err)
		}
		var dt DocType
		if err := yaml.Unmarshal(raw, &dt); err != nil {
			return nil, fmt.Errorf("schema: parse %s: %w", e.Name(), err)
		}
		if dt.Type == "" {
			return nil, fmt.Errorf("schema: %s has empty type", e.Name())
		}
		if _, dup := r.docTypes[dt.Type]; dup {
			return nil, fmt.Errorf("schema: duplicate doc-type %q", dt.Type)
		}
		r.docTypes[dt.Type] = dt
	}

	if err := r.checkConsistency(); err != nil {
		return nil, err
	}
	return r, nil
}

// checkConsistency verifies that every blocks-field allowlist names known blocks
// and every reference-field targets a known doc-type, recursing into array `of`
// sub-fields.
func (r *Registry) checkConsistency() error {
	for _, dt := range r.docTypes {
		if err := r.checkFields("type:"+dt.Type, dt.Fields); err != nil {
			return err
		}
	}
	for _, b := range r.blocks {
		if err := r.checkFields("block:"+b.Name, b.Fields); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) checkFields(owner string, fields []Field) error {
	for _, f := range fields {
		switch f.Type {
		case TypeBlocks:
			if len(f.Allow) == 0 {
				return fmt.Errorf("schema: %s.%s (blocks) has empty allow list", owner, f.Name)
			}
			for _, name := range f.Allow {
				if _, ok := r.blocks[name]; !ok {
					return fmt.Errorf("schema: %s.%s allows unknown block %q", owner, f.Name, name)
				}
			}
		case TypeReference:
			if f.Ref == "" {
				return fmt.Errorf("schema: %s.%s (reference) has no ref target", owner, f.Name)
			}
			if _, ok := r.docTypes[f.Ref]; !ok {
				return fmt.Errorf("schema: %s.%s references unknown doc-type %q", owner, f.Name, f.Ref)
			}
		case TypeArray:
			if err := r.checkFields(owner+"."+f.Name+"[]", f.Of); err != nil {
				return err
			}
		}
	}
	return nil
}
