package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// decodeLines reads the JSON records a slog JSON handler wrote.
func decodeLines(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

// TestLogNodeEnvDistinguishesByLevel pins the part that makes the line useful.
//
// 🔴 The two cases must differ in LEVEL, not only in a text field. The whole
// point is that a defaulted NODE_ENV stands out in a log nobody is reading
// closely; an Info line saying "defaulted" would be as invisible as the "env"
// field already is. Asserting only on the source attribute would pass against
// an implementation that logs both at Info.
func TestLogNodeEnvDistinguishesByLevel(t *testing.T) {
	cases := []struct {
		name       string
		nodeEnv    string
		defaulted  bool
		wantLevel  string
		wantSource string
		wantEffect bool
	}{
		{"declared production", "production", false, "INFO", "declared", false},
		{"declared development", "development", false, "INFO", "declared", false},
		{"defaulted", "development", true, "WARN", "defaulted", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			// The real handler, so this asserts on real output, not a fake.
			l := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			logNodeEnv(l, c.nodeEnv, c.defaulted)

			recs := decodeLines(t, buf.String())
			if len(recs) != 1 {
				t.Fatalf("got %d records, want exactly 1", len(recs))
			}
			r := recs[0]
			if r["msg"] != "node_env" {
				t.Errorf("msg = %v, want node_env", r["msg"])
			}
			if r["level"] != c.wantLevel {
				t.Errorf("level = %v, want %v", r["level"], c.wantLevel)
			}
			if r["source"] != c.wantSource {
				t.Errorf("source = %v, want %v", r["source"], c.wantSource)
			}
			if r["value"] != c.nodeEnv {
				t.Errorf("value = %v, want %v", r["value"], c.nodeEnv)
			}
			_, hasEffect := r["effect"]
			if hasEffect != c.wantEffect {
				t.Errorf("effect present = %v, want %v", hasEffect, c.wantEffect)
			}
		})
	}
}

// TestOneShotSaysNodeEnvBeforeTheDSNCheck binds the placement claim.
//
// The line is inserted BEFORE the DATABASE_URL check in each one-shot, so a run
// that dies on a missing DSN has still said which environment it thought it was
// in. Asserting that the function logs is not enough — a call placed after the
// check would satisfy that and emit nothing on the path this test takes.
//
// 🔴 This is the shape of the gap that shipped in #102: the line existed, but
// not on the surface where the question was asked. So the assertion is "Migrate
// failed AND the line is there", not "the line is there somewhere".
func TestOneShotSaysNodeEnvBeforeTheDSNCheck(t *testing.T) {
	t.Setenv("NODE_ENV", "")
	t.Setenv("DATABASE_URL", "")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	runErr := Migrate(context.Background())
	os.Stdout = orig
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if runErr == nil {
		t.Fatal("Migrate should fail with an empty DATABASE_URL — otherwise this test proves nothing about ordering")
	}
	var found map[string]any
	for _, rec := range decodeLines(t, string(out)) {
		if rec["msg"] == "node_env" {
			found = rec
		}
	}
	if found == nil {
		t.Fatalf("no node_env line before the DSN failure; got: %q", string(out))
	}
	if found["source"] != "defaulted" {
		t.Errorf("source = %v, want defaulted (NODE_ENV was set empty)", found["source"])
	}
	if found["level"] != "WARN" {
		t.Errorf("level = %v, want WARN", found["level"])
	}
}
