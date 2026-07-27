package schema

import (
	"testing"
	"testing/fstest"
)

func TestDefaultRegistryLoads(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	for _, typ := range []string{"course", "module", "lesson", "page", "post", "resource"} {
		if _, ok := r.DocType(typ); !ok {
			t.Errorf("missing doc-type %q", typ)
		}
	}
	for _, blk := range []string{"rich_text", "video_embed", "quiz", "hero", "cta", "columns"} {
		if _, ok := r.Block(blk); !ok {
			t.Errorf("missing block %q", blk)
		}
	}
	lesson, _ := r.DocType("lesson")
	var hasBody bool
	for _, f := range lesson.Fields {
		if f.Name == "body" && f.Type == TypeBlocks {
			hasBody = true
		}
	}
	if !hasBody {
		t.Error("lesson should have a body blocks field")
	}
	if len(r.DocTypes()) != 7 {
		t.Errorf("DocTypes = %d, want 7", len(r.DocTypes()))
	}
}

func TestLoadRejectsUnknownBlockInAllow(t *testing.T) {
	fsys := fstest.MapFS{
		"blocks.yaml": &fstest.MapFile{Data: []byte("blocks:\n  - name: text\n    fields: []\n")},
		"types/bad.yaml": &fstest.MapFile{Data: []byte(
			"type: bad\nfields:\n  - name: body\n    type: blocks\n    allow: [nonexistent]\n",
		)},
	}
	if _, err := Load(fsys); err == nil {
		t.Fatal("expected error for allowlist referencing unknown block")
	}
}

func TestLoadRejectsUnknownReferenceTarget(t *testing.T) {
	fsys := fstest.MapFS{
		"blocks.yaml": &fstest.MapFile{Data: []byte("blocks: []\n")},
		"types/bad.yaml": &fstest.MapFile{Data: []byte(
			"type: bad\nfields:\n  - { name: rel, type: reference, ref: ghost }\n",
		)},
	}
	if _, err := Load(fsys); err == nil {
		t.Fatal("expected error for reference to unknown doc-type")
	}
}
