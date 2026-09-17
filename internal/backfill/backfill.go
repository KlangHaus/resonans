// Package backfill cleans rich text that was stored BEFORE the API sanitized on
// save (resonans#52): every row of documents and document_versions goes through
// schema.RepairDocument. Without the versions table, a rollback would restore
// the dirty body again.
//
// Dry-run is the default and writes nothing; it reports what apply WOULD do.
// Apply writes only rows whose canonical JSON changes, so it is idempotent: a
// second run reports 0 changed.
package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/schema"
)

// Tables in scan order.
const (
	TableDocuments = "documents"
	TableVersions  = "document_versions"
)

// Row is one stored body.
type Row struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	Type     string // doc-type (versions: the owning document's type)
	Body     json.RawMessage
}

// Store is what the backfill needs from the database.
type Store interface {
	// Scan calls fn for every row of table, across all tenants.
	Scan(ctx context.Context, table string, fn func(Row) error) error
	// Update replaces one row's body, scoped to its tenant.
	Update(ctx context.Context, table string, row Row, body json.RawMessage) error
}

// Report counts per table. Every scanned row lands in exactly one of
// Unchanged, Changed, UnknownType or Invalid.
type Report struct {
	Table       string
	Scanned     int
	Unchanged   int
	Changed     int // would be (dry-run) / was (apply) rewritten
	Blanked     int // of Changed: rows with a non-string in HTML position set to ""
	UnknownType int // doc-type no longer in the registry: left untouched
	Invalid     int // body is not a JSON object: left untouched
	// Findings are "<row id> <path> (<json type>)" for blanked values, and
	// "<row id> unknown type <t>" / "<row id> invalid body": ids and paths only,
	// never content.
	Findings []string
}

// Run scans both tables. With apply=false nothing is written.
func Run(ctx context.Context, st Store, reg *schema.Registry, apply bool) ([]Report, error) {
	var out []Report
	for _, table := range []string{TableDocuments, TableVersions} {
		rep := Report{Table: table}
		err := st.Scan(ctx, table, func(row Row) error {
			rep.Scanned++
			res, err := schema.RepairDocument(reg, row.Type, row.Body)
			switch {
			case errors.Is(err, schema.ErrUnknownType):
				rep.UnknownType++
				rep.Findings = append(rep.Findings, fmt.Sprintf("%s unknown type %q", row.ID, row.Type))
				return nil
			case errors.Is(err, schema.ErrInvalid):
				rep.Invalid++
				rep.Findings = append(rep.Findings, fmt.Sprintf("%s invalid body", row.ID))
				return nil
			case err != nil:
				return fmt.Errorf("backfill: %s %s: %w", table, row.ID, err)
			}
			if !res.Changed {
				rep.Unchanged++
				return nil
			}
			rep.Changed++
			if len(res.Blanked) > 0 {
				rep.Blanked++
				for _, p := range res.Blanked {
					rep.Findings = append(rep.Findings, fmt.Sprintf("%s %s", row.ID, p))
				}
			}
			if apply {
				if err := st.Update(ctx, table, row, res.Body); err != nil {
					return fmt.Errorf("backfill: update %s %s: %w", table, row.ID, err)
				}
			}
			return nil
		})
		if err != nil {
			return out, err
		}
		sort.Strings(rep.Findings)
		out = append(out, rep)
	}
	return out, nil
}
