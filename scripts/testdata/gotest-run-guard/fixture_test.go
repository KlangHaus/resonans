package guardfixture

import "testing"

// TestGuardFixtureTop is the accept case for a plain top-level pattern.
func TestGuardFixtureTop(t *testing.T) {
	if 1+1 != 2 {
		t.Fatal("arithmetic")
	}
}

// TestGuardFixtureWithSubtests is the accept case for a pattern that names a
// subtest, which `go test -list` cannot see -- the reason the guard lists only
// the first `/`-separated segment.
func TestGuardFixtureWithSubtests(t *testing.T) {
	for _, name := range []string{"alpha", "beta"} {
		t.Run(name, func(t *testing.T) {
			if name == "" {
				t.Fatal("empty")
			}
		})
	}
}

// TestGuardFixtureSkips is a reject case: the name matches, so -list is happy,
// but nothing is measured. A verification that selects only this test has
// measured nothing and must not read as a pass.
func TestGuardFixtureSkips(t *testing.T) {
	t.Skip("fixture: always skipped, so the guard has a matched-but-not-run case")
}
