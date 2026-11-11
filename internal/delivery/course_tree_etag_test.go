package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/KlangHaus/resonans/internal/content"
)

// The expanded course tree carries its lessons' titles. A lesson renamed and
// republished changes that body while the course row stays the same, so the
// old ETag must get 200 and the new title.
func TestCourseTreeETagMovesWhenALessonChanges(t *testing.T) {
	s := content.NewMemoryStore()
	ctx := context.Background()
	mk := func(typ, slug, title, body string) content.Document {
		d, err := s.Create(ctx, tenantA, content.CreateInput{Type: typ, Slug: slug, Title: title, Body: json.RawMessage(body)})
		if err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
		if _, err := s.Publish(ctx, tenantA, d.ID); err != nil {
			t.Fatalf("publish %s: %v", slug, err)
		}
		return d
	}
	lesson := mk("lesson", "l1", "Lektion 1", `{"estimatedMinutes":15}`)
	module := mk("module", "m1", "Modul 1", `{"lessons":[{"lesson":"`+lesson.ID.String()+`"}]}`)
	mk("course", "c1", "Kursus 1", `{"modules":[{"module":"`+module.ID.String()+`"}]}`)
	h := NewRouter(Deps{TrustHeaderWithoutToken: true, Store: s, Registry: reg(t)})
	const tree = "/delivery/v1/course/c1?expand=modules.lessons"

	first := getWith(h, tree, nil)
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" || !strings.Contains(first.Body.String(), "Lektion 1") {
		t.Fatalf("tree = %d ETag %q: %s", first.Code, etag, first.Body)
	}
	if rec := getWith(h, tree, map[string]string{"If-None-Match": etag}); rec.Code != http.StatusNotModified {
		t.Fatalf("unchanged tree revalidated = %d, want 304", rec.Code)
	}

	title := "Lektion 1, omdøbt"
	if _, err := s.Update(ctx, tenantA, lesson.ID, content.UpdateInput{Title: &title}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, tenantA, lesson.ID); err != nil {
		t.Fatal(err)
	}
	if plain := getWith(h, tree, nil); !strings.Contains(plain.Body.String(), title) {
		t.Fatalf("fixture: the tree does not show the new title, so this case shows nothing: %s", plain.Body)
	}
	rec := getWith(h, tree, map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusOK {
		t.Fatalf("renamed lesson revalidated with the old ETag = %d, want 200", rec.Code)
	}
}
