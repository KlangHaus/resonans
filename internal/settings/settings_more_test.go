package settings

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestEnabled(t *testing.T) {
	for _, c := range []struct {
		set  []string
		typ  string
		want bool
	}{
		{nil, "page", true},        // no profile = every type
		{[]string{}, "page", true}, // explicit "everything"
		{[]string{"post", "page"}, "page", true},
		{[]string{"post"}, "page", false},
	} {
		if got := Enabled(c.set, c.typ); got != c.want {
			t.Errorf("Enabled(%v, %q) = %v, want %v", c.set, c.typ, got, c.want)
		}
	}
}

func TestMemoryStoreEnabledTypes(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	a, b := uuid.New(), uuid.New()
	if got, _ := s.EnabledTypes(ctx, a); len(got) != 0 {
		t.Fatalf("unset profile = %v, want empty (all types)", got)
	}
	in := []string{"post", "page"}
	if err := s.SetEnabledTypes(ctx, a, in); err != nil {
		t.Fatal(err)
	}
	in[0] = "mutated"
	got, _ := s.EnabledTypes(ctx, a)
	if len(got) != 2 || got[0] != "post" {
		t.Fatalf("EnabledTypes = %v; the store must keep its own copy of the input", got)
	}
	got[0] = "mutated"
	if again, _ := s.EnabledTypes(ctx, a); again[0] != "post" {
		t.Fatalf("EnabledTypes returned the store's own slice: %v", again)
	}
	if other, _ := s.EnabledTypes(ctx, b); len(other) != 0 {
		t.Fatalf("tenant b sees %v", other)
	}
	if err := s.SetEnabledTypes(ctx, a, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.EnabledTypes(ctx, a); len(got) != 0 {
		t.Fatalf("after resetting, profile = %v, want empty", got)
	}
}
