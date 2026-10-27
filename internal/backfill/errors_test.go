package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/KlangHaus/resonans/internal/schema"
)

// A failure the backfill cannot classify stops the run and is returned,
// wrapped with what it was doing. Only a concurrent change is a skip; any
// other write error must not be counted as "skipped" and carry on.
func TestRunStopsOnStoreErrors(t *testing.T) {
	reg, err := schema.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	down := errors.New("pq: connection reset")

	scanFails := &failingScan{err: down}
	if _, err := Run(context.Background(), scanFails, reg, false); !errors.Is(err, down) {
		t.Errorf("a failing scan: Run = %v, want the scan error", err)
	}

	st := fixture(t)
	writeFails := &failingUpdate{memStore: st, err: down}
	reps, err := Run(context.Background(), writeFails, reg, true)
	if !errors.Is(err, down) || !strings.Contains(err.Error(), "backfill: update") {
		t.Fatalf("a failing write: Run = %v, want it wrapped as an update error", err)
	}
	for _, r := range reps {
		if r.SkippedConcurrent != 0 {
			t.Errorf("%s counted %d skips for a write that FAILED", r.Table, r.SkippedConcurrent)
		}
	}
}

type failingScan struct{ err error }

func (f *failingScan) Scan(context.Context, string, func(Row) error) error { return f.err }
func (f *failingScan) Update(context.Context, string, Row, json.RawMessage) error {
	return nil
}

type failingUpdate struct {
	*memStore
	err error
}

func (f *failingUpdate) Update(context.Context, string, Row, json.RawMessage) error { return f.err }
