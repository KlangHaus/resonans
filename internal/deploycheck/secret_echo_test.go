// Package deploycheck guards the SQL we ship in deploy/ against regressions a
// unit test can otherwise not see, without needing a database.
package deploycheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestShippedSQLDoesNotEchoSecrets asserts that every statement which RETURNS a
// secret is wrapped in psql's `\o /dev/null` … `\o` window.
//
// Why this exists: `set_config` returns the value it sets, and psql prints
// returned rows. `deploy/initdb/10-app-role.sql` runs inside the postgres image's
// init, so the unwrapped form wrote the app-role password in CLEARTEXT to the
// container's log. Measured 2026-09-28 with a canary password: 1 occurrence in
// `docker logs`, 0 with the redirect, and init completed in BOTH arms — so the
// difference was the redirect and not a broken instrument.
//
// Not measured by this test, stated so the green says what it did not cover: this
// reads the files, it does not run them. The end-to-end canary against a real
// postgres is by hand until CI has a database (KH-1220).
//
// A static check like this can go green by seeing nothing, which is the worst
// guard state, so it asserts its denominator PER FILE.
//
// Both refinements below came from [sikkerhed] firing the guard with a control in
// the same run, and both were false negatives — the class that looks like work:
//
//  1. The `perform` exemption was tested with Contains on the whole line, and a
//     trailing `-- perform this later` on a LEAKING statement turned the guard
//     green. So comments are stripped before anything is tested, and the
//     exemption now requires `perform` to be the statement's leading keyword,
//     which is the only position where it actually discards a result.
//
//  2. The denominator was global. Renaming set_config in 10-app-role.sql alone
//     left manual-setup.sql's three matches carrying the count, so the guard
//     stayed green while the file whose stdout IS a container log had stopped
//     being checked. A global count proves the pattern still matches SOMETHING
//     we ship, not every file — and the asymmetry pointed the wrong way. Every
//     file that interpolates a password variable must now contribute at least
//     one checked statement.
//
// passwordVar matches a psql variable interpolation whose name looks like a
// secret. Deliberately broad: a file this matches must have at least one checked
// statement, so a false positive here costs a floor rather than a hole.
var passwordVar = regexp.MustCompile(`:'[a-zA-Z_]*(password|passwd|secret|token)[a-zA-Z_]*'`)

// stripComment removes a trailing `--` comment and surrounding whitespace, taking
// care not to cut inside a single-quoted literal. A trailing comment that reached
// the exemption test is exactly how this guard was disarmed once.
func stripComment(line string) string {
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '\'':
			inQuote = !inQuote
		case !inQuote && line[i] == '-' && i+1 < len(line) && line[i+1] == '-':
			return strings.TrimSpace(line[:i])
		}
	}
	return strings.TrimSpace(line)
}

func TestShippedSQLDoesNotEchoSecrets(t *testing.T) {
	// Functions that return what they were given. Adding a secret to a statement
	// built from any of these without the redirect is the bug this catches.
	echoing := []string{"set_config("}

	files, err := filepath.Glob(filepath.Join("..", "..", "deploy", "*.sql"))
	if err != nil {
		t.Fatalf("glob deploy/*.sql: %v", err)
	}
	nested, err := filepath.Glob(filepath.Join("..", "..", "deploy", "*", "*.sql"))
	if err != nil {
		t.Fatalf("glob deploy/*/*.sql: %v", err)
	}
	files = append(files, nested...)
	if len(files) == 0 {
		t.Fatalf("found no deploy SQL files — this guard is looking at the wrong path, " +
			"which is not the same as passing")
	}

	checked := 0
	perFile := map[string]int{}
	handlesPassword := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		silenced := false
		for i, raw := range strings.Split(string(b), "\n") {
			line := stripComment(raw)
			if line == "" {
				continue
			}
			// A file that interpolates a password variable is one this guard MUST
			// have something to say about, whatever the statement is called.
			if passwordVar.MatchString(line) {
				handlesPassword[f] = true
			}
			switch {
			case strings.HasPrefix(line, `\o /dev/null`):
				silenced = true
				continue
			case line == `\o`:
				silenced = false
				continue
			}
			for _, fn := range echoing {
				if !strings.Contains(line, fn) {
					continue
				}
				// Only a LEADING `perform` discards the result. Anywhere else the
				// word is prose, and testing for its presence is what let a
				// trailing comment disarm this check.
				if strings.HasPrefix(strings.ToLower(line), "perform ") {
					continue
				}
				checked++
				perFile[f]++
				if !silenced {
					t.Errorf("%s:%d returns a value that psql will print, outside any "+
						"`\\o /dev/null` window:\n\t%s\n"+
						"if the argument is a secret this writes it to stdout — for "+
						"deploy/initdb/*.sql that is the container log",
						f, i+1, line)
				}
			}
		}
	}

	// The denominator, per file. Zero matches in a file that handles a password
	// means the pattern stopped matching THAT file, not that the file became safe.
	for f := range handlesPassword {
		if perFile[f] == 0 {
			t.Errorf("%s interpolates a password variable but this guard found no %v "+
				"statement in it at all — the pattern no longer matches this file, so "+
				"its green measured nothing", f, echoing)
		}
	}
	if len(handlesPassword) == 0 {
		t.Fatalf("scanned %d deploy SQL files and none of them interpolates a password "+
			"variable — this guard is looking at the wrong files, which is not the same "+
			"as passing", len(files))
	}

	t.Logf("%d secret-returning statements across %d deploy SQL files (%d of them handle a "+
		"password, each with at least one checked statement), all inside an `\\o` window",
		checked, len(files), len(handlesPassword))
}
