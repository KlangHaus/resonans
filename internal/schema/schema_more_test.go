package schema

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// A small schema with every field type, so these cells do not depend on the
// shipped content model and say exactly what they exercise.
const testBlocks = `blocks:
  - name: note
    fields:
      - { name: text, type: string, required: true }
  - name: rich_text
    fields:
      - { name: value, type: richtext, i18n: true, required: true }
      - { name: format, type: string }
  - name: banner
    fields:
      - { name: text, type: string }
`

const testType = `type: sample
fields:
  - { name: title, type: string }
  - { name: summary, type: text, i18n: true }
  - { name: count, type: number }
  - { name: featured, type: boolean }
  - { name: image, type: asset }
  - { name: author, type: reference, ref: sample }
  - name: links
    type: array
    of:
      - { name: label, type: string, required: true }
  - { name: body, type: blocks, allow: [note, rich_text] }
`

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := Load(fstest.MapFS{
		"blocks.yaml":       {Data: []byte(testBlocks)},
		"types/sample.yaml": {Data: []byte(testType)},
		"types/README.md":   {Data: []byte("not a schema: skipped")},
		"types/sub/x.yaml":  {Data: []byte("type: nested\n")},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return r
}

// Load refuses a schema that is wrong, naming what is wrong, before anything
// is served from it.
func TestLoadRefusesABrokenSchema(t *testing.T) {
	for _, c := range []struct {
		name   string
		blocks string
		types  map[string]string
		want   string
	}{
		{"no blocks.yaml", "", nil, "read blocks.yaml"},
		{"blocks.yaml not YAML", "blocks: [", nil, "parse blocks.yaml"},
		{"block without a name", "blocks:\n  - fields: []\n", nil, "block with empty name"},
		{"duplicate block", "blocks:\n  - name: a\n  - name: a\n", nil, `duplicate block "a"`},
		{"type file not YAML", testBlocks, map[string]string{"x.yaml": "type: ["}, "parse x.yaml"},
		{"type without a name", testBlocks, map[string]string{"x.yaml": "fields: []\n"}, "x.yaml has empty type"},
		{"duplicate type", testBlocks, map[string]string{"a.yaml": "type: d\n", "b.yaml": "type: d\n"}, `duplicate doc-type "d"`},
		{"blocks field without allow", testBlocks, map[string]string{"x.yaml": "type: x\nfields:\n  - { name: body, type: blocks }\n"}, "has empty allow list"},
		{"reference without target", testBlocks, map[string]string{"x.yaml": "type: x\nfields:\n  - { name: r, type: reference }\n"}, "has no ref target"},
		{"bad reference inside an array", testBlocks, map[string]string{"x.yaml": "type: x\nfields:\n  - name: items\n    type: array\n    of:\n      - { name: r, type: reference, ref: nope }\n"}, `x.items[].r references unknown doc-type "nope"`},
		{"a block's own field is checked", "blocks:\n  - name: b\n    fields:\n      - { name: inner, type: blocks }\n", map[string]string{}, "block:b.inner (blocks) has empty allow list"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fsys := fstest.MapFS{}
			if c.blocks != "" {
				fsys["blocks.yaml"] = &fstest.MapFile{Data: []byte(c.blocks)}
			}
			for name, body := range c.types {
				fsys["types/"+name] = &fstest.MapFile{Data: []byte(body)}
			}
			if c.types != nil && len(c.types) == 0 {
				fsys["types/.keep"] = &fstest.MapFile{}
			}
			if c.blocks != "" && c.types == nil {
				// A schema can't load without the types directory either.
				fsys["types/.keep"] = &fstest.MapFile{}
			}
			_, err := Load(fsys)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Load = %v; want an error mentioning %q", err, c.want)
			}
		})
	}
	// No types directory at all.
	if _, err := Load(fstest.MapFS{"blocks.yaml": {Data: []byte(testBlocks)}}); err == nil || !strings.Contains(err.Error(), "read types dir") {
		t.Fatalf("Load without types/ = %v", err)
	}
}

func TestLoadSkipsNonSchemaFiles(t *testing.T) {
	r := testRegistry(t)
	if _, ok := r.DocType("nested"); ok {
		t.Error("a YAML file in a subdirectory of types/ was loaded")
	}
	if got := len(r.DocTypes()); got != 1 {
		t.Errorf("%d doc-types loaded, want 1 (README.md and the subdirectory skipped)", got)
	}
}

func TestBlocksAreListedSortedByName(t *testing.T) {
	var names []string
	for _, b := range testRegistry(t).Blocks() {
		names = append(names, b.Name)
	}
	if !slices.Equal(names, []string{"banner", "note", "rich_text"}) {
		t.Errorf("Blocks() = %v, want sorted", names)
	}
}

// Every way a document can have the wrong shape is refused as ErrInvalid,
// naming the path, so the editor can point at the field.
func TestValidateDocumentRefusesWrongShapes(t *testing.T) {
	r := testRegistry(t)
	valid := `{"title":"t","summary":{"da":"s"},"count":2,"featured":true,"image":"a1","author":"d1",
		"links":[{"label":"l"}],"body":[{"_type":"note","_key":"k","text":"x"},{"_type":"rich_text","value":{"da":"<p>x</p>"},"format":"html"}]}`
	if err := ValidateDocument(r, "sample", json.RawMessage(valid)); err != nil {
		t.Fatalf("control: a valid document was refused: %v", err)
	}
	for _, c := range []struct {
		name, body, want string
	}{
		{"not JSON", `{`, "not a JSON object"},
		{"not an object", `[1]`, "not a JSON object"},
		{"string given a number", `{"title":1}`, "sample.title must be a string"},
		{"i18n given a plain string", `{"summary":"s"}`, "sample.summary must be an i18n object"},
		{"number given a string", `{"count":"2"}`, "sample.count must be a number"},
		{"boolean given a string", `{"featured":"yes"}`, "sample.featured must be a boolean"},
		{"asset given an object", `{"image":{"id":"a1"}}`, "sample.image must be a asset reference"},
		{"reference given a number", `{"author":7}`, "sample.author must be a reference reference"},
		{"array given an object", `{"links":{"label":"l"}}`, "sample.links must be an array"},
		{"array element not an object", `{"links":["l"]}`, "sample.links[0] must be an object"},
		{"array element missing a required field", `{"links":[{}]}`, `missing required field "label" at sample.links[0]`},
		{"blocks given an object", `{"body":{"_type":"note"}}`, "sample.body must be an array of blocks"},
		{"block not an object", `{"body":["note"]}`, "sample.body[0] must be a block object"},
		{"block without _type", `{"body":[{"text":"x"}]}`, "sample.body[0] missing _type"},
		{"block not allowed here", `{"body":[{"_type":"banner"}]}`, `block "banner" not allowed here`},
		{"unknown field in a block", `{"body":[{"_type":"note","text":"x","color":"red"}]}`, `unknown field "color" at sample.body[0]:note`},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateDocument(r, "sample", json.RawMessage(c.body))
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ValidateDocument(%s) = %v; want ErrInvalid mentioning %q", c.body, err, c.want)
			}
		})
	}
	// A field whose declared type the validator does not know is refused, not
	// waved through: built directly, since Load would not produce one.
	odd := &Registry{docTypes: map[string]DocType{"odd": {Type: "odd", Fields: []Field{{Name: "x", Type: "rgb"}}}}, blocks: map[string]BlockType{}}
	if err := ValidateDocument(odd, "odd", json.RawMessage(`{"x":"red"}`)); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), `unsupported type "rgb"`) {
		t.Errorf("unsupported field type = %v", err)
	}
}

func TestSanitizeAndRepairRefuseWhatTheyCannotRead(t *testing.T) {
	r := testRegistry(t)
	for _, c := range []struct {
		name, docType, body string
		want                error
	}{
		{"unknown doc-type", "nope", `{}`, ErrUnknownType},
		{"not JSON", "sample", `{`, ErrInvalid},
	} {
		if _, err := SanitizeDocument(r, c.docType, json.RawMessage(c.body)); !errors.Is(err, c.want) {
			t.Errorf("SanitizeDocument %s = %v, want %v", c.name, err, c.want)
		}
		if _, err := RepairDocument(r, c.docType, json.RawMessage(c.body)); !errors.Is(err, c.want) {
			t.Errorf("RepairDocument %s = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestRichTextShapeNames(t *testing.T) {
	for s, want := range map[richTextShape]string{shapePlain: "plain", shapeHTML: "html", shapeMixed: "mixed"} {
		if s.String() != want {
			t.Errorf("%d.String() = %q, want %q", s, s.String(), want)
		}
	}
}
