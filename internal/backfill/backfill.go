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

// ErrConcurrentChange is returned by Store.Update when the row no longer holds
// the scanned body: someone saved in between. The row is skipped, not written.
var ErrConcurrentChange = errors.New("backfill: row changed since scan")

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
	// Update replaces one row's body, scoped to its tenant, ONLY if the row still
	// holds row.Body; otherwise it returns ErrConcurrentChange and writes nothing.
	Update(ctx context.Context, table string, row Row, body json.RawMessage) error
}

// Report counts per table.
//
// BUCKETS, which sum to Scanned: every scanned row lands in exactly one of
// Unchanged, Changed, SkippedConcurrent, UnknownType or Invalid.
//
// OVERLAYS, which do not: Blanked and Labeled are subsets of Changed, and
// Ambiguous crosses buckets -- an ambiguous row is left byte-identical, so it
// is counted Unchanged AND Ambiguous. Do not add an overlay to the sum.
type Report struct {
	Table     string
	Scanned   int
	Unchanged int
	Changed   int // would be (dry-run) / was (apply) rewritten
	// SkippedConcurrent (apply only): changed since the scan, so not written.
	// Run the backfill again to clean the row's new body.
	SkippedConcurrent int
	Blanked           int // of Changed: rows with a non-string in HTML position set to ""
	// Labeled (of Changed): rows carrying rich_text that the interim editor
	// wrote as HTML without a format. They are labeled AND sanitized.
	Labeled int
	// Ambiguous: rows holding a rich_text block this pass refused to label --
	// translations part HTML and part plain, or a value the parser could not
	// read. NOT written, and counted separately so a clean run cannot hide
	// them: a row can be Unchanged and still be Ambiguous.
	Ambiguous   int
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
			// 🔴 BEFORE the unchanged return, because an ambiguous row is
			// byte-identical by design: it is left alone. Counted after the
			// early return it would be reported as a clean row, and the rows
			// that need a person are exactly the ones that look clean.
			if len(res.Ambiguous) > 0 {
				rep.Ambiguous++
				for _, p := range res.Ambiguous {
					rep.Findings = append(rep.Findings, fmt.Sprintf("%s %s", row.ID, p))
				}
			}
			if !res.Changed {
				rep.Unchanged++
				return nil
			}
			if apply {
				err := st.Update(ctx, table, row, res.Body)
				if errors.Is(err, ErrConcurrentChange) {
					rep.SkippedConcurrent++
					rep.Findings = append(rep.Findings, fmt.Sprintf("%s skipped: changed since scan", row.ID))
					return nil
				}
				if err != nil {
					return fmt.Errorf("backfill: update %s %s: %w", table, row.ID, err)
				}
			}
			rep.Changed++
			if len(res.Labeled) > 0 {
				rep.Labeled++
				for _, p := range res.Labeled {
					rep.Findings = append(rep.Findings, fmt.Sprintf("%s %s labeled html", row.ID, p))
				}
			}
			if len(res.Blanked) > 0 {
				rep.Blanked++
				for _, p := range res.Blanked {
					rep.Findings = append(rep.Findings, fmt.Sprintf("%s %s", row.ID, p))
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

// SlugRow is one document's slug.
type SlugRow struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	Type     string
	Slug     string
}

// SlugScanner lists every document's slug across tenants.
type SlugScanner interface {
	ScanSlugs(ctx context.Context, fn func(SlugRow) error) error
}

// SlugReport counts slugs that the API now rejects at create
// (content.ValidSlug). Read-only: nothing is renamed, because a slug is part of
// a public URL and changing it is an editorial decision, not a cleanup.
type SlugReport struct {
	Scanned  int
	Invalid  int
	Findings []string // "<id> <tenant> <type> <quoted slug>"
}

// AuditSlugs reports invalid slugs.
func AuditSlugs(ctx context.Context, sc SlugScanner, valid func(string) bool) (SlugReport, error) {
	var rep SlugReport
	err := sc.ScanSlugs(ctx, func(r SlugRow) error {
		rep.Scanned++
		if !valid(r.Slug) {
			rep.Invalid++
			rep.Findings = append(rep.Findings, fmt.Sprintf("%s %s %s %q", r.ID, r.TenantID, r.Type, r.Slug))
		}
		return nil
	})
	sort.Strings(rep.Findings)
	return rep, err
}
