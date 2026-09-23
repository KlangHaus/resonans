package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The bin, end to end. Soft delete was Allan's decision (2026-09-20: "soft
// delete, så det kan fortrydes"), and these cells pin the part of that decision
// that is easy to get subtly wrong: a deleted document must vanish from every
// READ an editor or a site can perform, while the row itself stays.

func createDoc(t *testing.T, h http.Handler, tenant, slug string) string {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/api/v1/documents", tenant, map[string]any{
		"type": "page", "slug": slug, "title": "Side " + slug,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: status = %d body=%s", slug, rec.Code, rec.Body)
	}
	var d struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	return d.ID
}

func listSlugs(t *testing.T, h http.Handler, tenant, query string) []string {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/api/v1/documents?type=page"+query, tenant, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list%s: status = %d", query, rec.Code)
	}
	var body struct {
		Documents []struct {
			Slug      string  `json:"slug"`
			DeletedAt *string `json:"deletedAt"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(body.Documents))
	for _, d := range body.Documents {
		out = append(out, d.Slug)
	}
	return out
}

func TestSoftDeleteHidesFromEveryReadPath(t *testing.T) {
	h := testRouter(t)
	id := createDoc(t, h, tenantA, "midlertidig")
	if rec := do(t, h, http.MethodPost, "/api/v1/documents/"+id+"/publish", tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
	// Published and readable by the delivery path — the positive control, so a
	// later 404 means "deleted" and not "never worked".
	if rec := do(t, h, http.MethodGet, "/api/v1/content/page/midlertidig", tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("published read before delete: %d", rec.Code)
	}

	rec := do(t, h, http.MethodDelete, "/api/v1/documents/"+id, tenantA, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	var deleted struct {
		DeletedAt *string `json:"deletedAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &deleted); err != nil {
		t.Fatal(err)
	}
	if deleted.DeletedAt == nil {
		t.Error("delete response must carry deletedAt so the editor can show the undo")
	}

	for _, c := range []struct {
		name, method, path string
		want               int
	}{
		{"delivery read", http.MethodGet, "/api/v1/content/page/midlertidig", http.StatusNotFound},
		{"publish a deleted document", http.MethodPost, "/api/v1/documents/" + id + "/publish", http.StatusConflict},
		{"update a deleted document", http.MethodPut, "/api/v1/documents/" + id, http.StatusConflict},
		{"delete it twice", http.MethodDelete, "/api/v1/documents/" + id, http.StatusConflict},
	} {
		var body any
		if c.method == http.MethodPut {
			body = map[string]any{"title": "nyt navn"}
		}
		if rec := do(t, h, c.method, c.path, tenantA, body); rec.Code != c.want {
			t.Errorf("%s: status = %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body)
		}
	}

	if got := listSlugs(t, h, tenantA, ""); len(got) != 0 {
		t.Errorf("default listing = %v, want no deleted documents", got)
	}
	if got := listSlugs(t, h, tenantA, "&deleted=only"); len(got) != 1 || got[0] != "midlertidig" {
		t.Errorf("bin listing = %v, want [midlertidig]", got)
	}
	// A typo in the selector must not open the bin.
	if got := listSlugs(t, h, tenantA, "&deleted=ja-tak"); len(got) != 0 {
		t.Errorf("unknown ?deleted value = %v, want live-only", got)
	}
}

func TestRestoreBringsItBackAndSlugIsFreedMeanwhile(t *testing.T) {
	h := testRouter(t)
	id := createDoc(t, h, tenantA, "forside")

	if rec := do(t, h, http.MethodPost, "/api/v1/documents/"+id+"/restore", tenantA, nil); rec.Code != http.StatusConflict {
		t.Errorf("restore of a live document = %d, want 409 (a no-op dressed as an action hides a wrong id)", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/v1/documents/"+id, tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	// The bin must not keep the slug hostage.
	newID := createDoc(t, h, tenantA, "forside")
	if newID == id {
		t.Fatal("recreate returned the deleted document")
	}
	// …and the live one keeps it: restoring the old document now collides.
	if rec := do(t, h, http.MethodPost, "/api/v1/documents/"+id+"/restore", tenantA, nil); rec.Code != http.StatusConflict {
		t.Errorf("restore into a taken slug = %d, want 409", rec.Code)
	}
	// Free the slug again, and the restore succeeds — proving the conflict was
	// about the slug and not about the document being unrestorable.
	if rec := do(t, h, http.MethodDelete, "/api/v1/documents/"+newID, tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete the new one: %d", rec.Code)
	}
	rec := do(t, h, http.MethodPost, "/api/v1/documents/"+id+"/restore", tenantA, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore after the slug was freed = %d %s", rec.Code, rec.Body)
	}
	var back struct {
		DeletedAt *string `json:"deletedAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	if back.DeletedAt != nil {
		t.Errorf("restored document still carries deletedAt = %v", *back.DeletedAt)
	}
	if got := listSlugs(t, h, tenantA, ""); len(got) != 1 || got[0] != "forside" {
		t.Errorf("listing after restore = %v, want [forside]", got)
	}
}

func TestDeleteIsTenantScoped(t *testing.T) {
	h := testRouter(t)
	id := createDoc(t, h, tenantA, "andens-side")

	// 404 rather than 403: tenant B must not learn that this id exists.
	for _, c := range []struct {
		name, method, path string
	}{
		{"delete", http.MethodDelete, "/api/v1/documents/" + id},
		{"restore", http.MethodPost, "/api/v1/documents/" + id + "/restore"},
	} {
		if rec := do(t, h, c.method, c.path, tenantB, nil); rec.Code != http.StatusNotFound {
			t.Errorf("cross-tenant %s = %d, want 404 (%s)", c.name, rec.Code, rec.Body)
		}
	}
	// Positive control in the same run: tenant A still owns it.
	if rec := do(t, h, http.MethodDelete, "/api/v1/documents/"+id, tenantA, nil); rec.Code != http.StatusOK {
		t.Fatalf("owner delete = %d, want 200 — the 404s above would be meaningless if nobody could delete", rec.Code)
	}
	if got := listSlugs(t, h, tenantB, "&deleted=include"); len(got) != 0 {
		t.Errorf("tenant B sees %v, want nothing", got)
	}
}
