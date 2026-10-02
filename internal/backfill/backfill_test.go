package backfill

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/schema"
)

type memStore struct {
	rows    map[string][]Row
	updates int
	// editBeforeUpdate simulates a save between scan and update for this row.
	editBeforeUpdate uuid.UUID
}

func (m *memStore) Scan(_ context.Context, table string, fn func(Row) error) error {
	for _, r := range m.rows[table] {
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}

func (m *memStore) Update(_ context.Context, table string, row Row, body json.RawMessage) error {
	for i := range m.rows[table] {
		r := &m.rows[table][i]
		if r.ID != row.ID {
			continue
		}
		if r.ID == m.editBeforeUpdate {
			r.Body = json.RawMessage(`{"body":[]}`) // the editor's newer save
		}
		if string(r.Body) != string(row.Body) {
			return ErrConcurrentChange
		}
		m.updates++
		r.Body = body
	}
	return nil
}

func body(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixture(t *testing.T) *memStore {
	tenant := uuid.New()
	dirtyHTML := body(t, map[string]any{"body": []any{map[string]any{
		"_type": "rich_text", "format": "html",
		"value": map[string]any{"da": `<p>ok</p><img src=x onerror=alert(1)>`},
	}}})
	arrayHTML := body(t, map[string]any{"body": []any{map[string]any{
		"_type": "rich_text", "format": "html",
		"value": map[string]any{"da": []any{"<img src=x onerror=alert(1)>", "<p>ok</p>"}},
	}}})
	plain := body(t, map[string]any{"body": []any{map[string]any{
		"_type": "callout",
		"value": map[string]any{"da": `Husk: 'organisk' & 2 < 3`},
	}}})
	return &memStore{rows: map[string][]Row{
		TableDocuments: {
			{ID: uuid.New(), TenantID: tenant, Type: "page", Body: dirtyHTML},
			{ID: uuid.New(), TenantID: tenant, Type: "page", Body: arrayHTML},
			{ID: uuid.New(), TenantID: tenant, Type: "page", Body: plain},
			{ID: uuid.New(), TenantID: tenant, Type: "retired-type", Body: plain},
		},
		TableVersions: {
			{ID: uuid.New(), TenantID: tenant, Type: "page", Body: dirtyHTML},
			{ID: uuid.New(), TenantID: tenant, Type: "page", Body: []byte(`"not an object"`)},
		},
	}}
}

func TestDryRunCountsAndWritesNothing(t *testing.T) {
	reg, _ := schema.DefaultRegistry()
	st := fixture(t)
	before := string(st.rows[TableDocuments][0].Body)
	reps, err := Run(context.Background(), st, reg, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.updates != 0 || string(st.rows[TableDocuments][0].Body) != before {
		t.Fatalf("dry-run wrote: %d updates", st.updates)
	}
	docs, vers := reps[0], reps[1]
	if docs.Scanned != 4 || docs.Changed != 2 || docs.Blanked != 1 || docs.Unchanged != 1 || docs.UnknownType != 1 {
		t.Errorf("documents report = %+v", docs)
	}
	if vers.Scanned != 2 || vers.Changed != 1 || vers.Invalid != 1 {
		t.Errorf("versions report = %+v", vers)
	}
	// The blanked finding names the path and type, never the markup.
	found := strings.Join(docs.Findings, "\n")
	if !strings.Contains(found, "rich_text.value.da (array)") || strings.Contains(found, "onerror") {
		t.Errorf("findings = %q", found)
	}
	for _, rep := range reps {
		if rep.Unchanged+rep.Changed+rep.SkippedConcurrent+rep.UnknownType+rep.Invalid != rep.Scanned {
			t.Errorf("%s: buckets do not add up: %+v", rep.Table, rep)
		}
	}
}

func TestApplyCleansAndIsIdempotent(t *testing.T) {
	reg, _ := schema.DefaultRegistry()
	st := fixture(t)
	plainBefore := string(st.rows[TableDocuments][2].Body)
	if _, err := Run(context.Background(), st, reg, true); err != nil {
		t.Fatal(err)
	}
	if st.updates != 3 {
		t.Errorf("updates = %d, want 3 (2 documents + 1 version)", st.updates)
	}
	all := string(st.rows[TableDocuments][0].Body) + string(st.rows[TableDocuments][1].Body) + string(st.rows[TableVersions][0].Body)
	if strings.Contains(all, "onerror") {
		t.Errorf("apply left onerror: %s", all)
	}
	if string(st.rows[TableDocuments][2].Body) != plainBefore {
		t.Errorf("plain row was rewritten")
	}
	// Second run: nothing to change.
	st.updates = 0
	reps, err := Run(context.Background(), st, reg, true)
	if err != nil {
		t.Fatal(err)
	}
	if st.updates != 0 || reps[0].Changed != 0 || reps[1].Changed != 0 {
		t.Errorf("second run changed rows: updates=%d reports=%+v", st.updates, reps)
	}
}

// A save between scan and update wins: the row is skipped and counted, and the
// editor's newer body is NOT overwritten with the old body cleaned.
func TestApplySkipsRowChangedSinceScan(t *testing.T) {
	reg, _ := schema.DefaultRegistry()
	st := fixture(t)
	edited := st.rows[TableDocuments][0].ID
	st.editBeforeUpdate = edited
	reps, err := Run(context.Background(), st, reg, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(st.rows[TableDocuments][0].Body) != `{"body":[]}` {
		t.Errorf("editor's save was overwritten: %s", st.rows[TableDocuments][0].Body)
	}
	docs := reps[0]
	if docs.SkippedConcurrent != 1 || docs.Changed != 1 {
		t.Errorf("documents report = %+v, want 1 skipped + 1 changed", docs)
	}
	if !strings.Contains(strings.Join(docs.Findings, "\n"), edited.String()+" skipped: changed since scan") {
		t.Errorf("findings = %v", docs.Findings)
	}
}

type slugStore []SlugRow

func (s slugStore) ScanSlugs(_ context.Context, fn func(SlugRow) error) error {
	for _, r := range s {
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}

func TestAuditSlugsCountsInvalidOnly(t *testing.T) {
	valid := func(s string) bool { return s == "ok-slug" }
	rep, err := AuditSlugs(context.Background(), slugStore{
		{ID: uuid.New(), Slug: "ok-slug"}, {ID: uuid.New(), Slug: "/evil.example"}, {ID: uuid.New(), Slug: `\\evil`},
	}, valid)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 3 || rep.Invalid != 2 || len(rep.Findings) != 2 {
		t.Errorf("report = %+v", rep)
	}
	if !strings.Contains(strings.Join(rep.Findings, "\n"), `"/evil.example"`) {
		t.Errorf("findings = %v", rep.Findings)
	}
}

// The rows this backfill was added for, and the overlay property that makes
// them visible: an unlabeled-HTML row is Changed AND Labeled, while a mixed
// row is left byte-identical -- so it is Unchanged AND Ambiguous at once. If
// Ambiguous were counted after the unchanged return, the rows that need a
// person would be the ones reported as clean.
func TestRunLabelsUnlabeledHTMLAndReportsTheRefusals(t *testing.T) {
	reg, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tenant := uuid.New()
	unlabeled := uuid.New()
	mixed := uuid.New()
	plain := uuid.New()
	st := &memStore{rows: map[string][]Row{
		TableDocuments: {
			{ID: unlabeled, TenantID: tenant, Type: "page", Body: body(t, map[string]any{"body": []any{
				map[string]any{"_type": "rich_text", "value": map[string]any{"da": `<p>ok</p><img src=x onerror=alert(1)>`}},
			}})},
			{ID: mixed, TenantID: tenant, Type: "page", Body: body(t, map[string]any{"body": []any{
				map[string]any{"_type": "rich_text", "value": map[string]any{"da": `<p>dansk</p>`, "en": `plain & 2 < 3`}},
			}})},
			{ID: plain, TenantID: tenant, Type: "page", Body: body(t, map[string]any{"body": []any{
				map[string]any{"_type": "rich_text", "value": map[string]any{"da": `Husk: 'organisk' & 2 < 3`}},
			}})},
		},
		TableVersions: {},
	}}

	reps, err := Run(context.Background(), st, reg, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	docs := reps[0]
	if docs.Scanned != 3 {
		t.Fatalf("Scanned = %d, want 3", docs.Scanned)
	}
	if docs.Changed != 1 || docs.Labeled != 1 {
		t.Errorf("Changed=%d Labeled=%d, want 1/1", docs.Changed, docs.Labeled)
	}
	// The overlay: the mixed row and the plain row are both Unchanged, and
	// exactly one of them is also Ambiguous.
	if docs.Unchanged != 2 || docs.Ambiguous != 1 {
		t.Errorf("Unchanged=%d Ambiguous=%d, want 2/1", docs.Unchanged, docs.Ambiguous)
	}
	// Buckets still sum to Scanned; overlays are not part of the sum.
	if docs.Unchanged+docs.Changed+docs.SkippedConcurrent+docs.UnknownType+docs.Invalid != docs.Scanned {
		t.Errorf("buckets do not sum to Scanned: %+v", docs)
	}

	// The written row is labeled AND sanitized, and findings name ids and
	// paths without carrying content.
	var stored map[string]any
	for _, r := range st.rows[TableDocuments] {
		if r.ID == unlabeled {
			if err := json.Unmarshal(r.Body, &stored); err != nil {
				t.Fatal(err)
			}
		}
	}
	blk := stored["body"].([]any)[0].(map[string]any)
	if blk["format"] != "html" {
		t.Errorf("stored format = %v, want html", blk["format"])
	}
	if v := blk["value"].(map[string]any)["da"].(string); strings.Contains(v, "onerror") {
		t.Errorf("stored value not sanitized: %q", v)
	}
	for _, f := range docs.Findings {
		if strings.Contains(f, "onerror") || strings.Contains(f, "organisk") {
			t.Errorf("finding carries content: %q", f)
		}
	}
	if len(docs.Findings) != 2 {
		t.Errorf("Findings = %v, want one labeled + one refused", docs.Findings)
	}
}
