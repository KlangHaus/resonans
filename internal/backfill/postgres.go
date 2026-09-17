package backfill

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore reads across tenants, so it must run as the migrator role
// (BYPASSRLS), like `resonans migrate`. Writes still set app.tenant_id per row,
// so an update can only ever touch the row's own tenant.
type PostgresStore struct{ Pool *pgxpool.Pool }

var scanSQL = map[string]string{
	TableDocuments: `select id, tenant_id, type, body from resonans_app.documents order by id`,
	// LEFT join: a version whose document is gone must be COUNTED (as unknown
	// type ""), not silently dropped from the scan by an inner join.
	TableVersions: `select v.id, v.tenant_id, coalesce(d.type, ''), v.body
	                  from resonans_app.document_versions v
	                  left join resonans_app.documents d on d.id = v.document_id and d.tenant_id = v.tenant_id
	                 order by v.id`,
}

var updateSQL = map[string]string{
	TableDocuments: `update resonans_app.documents set body = $1 where id = $2 and tenant_id = $3`,
	TableVersions:  `update resonans_app.document_versions set body = $1 where id = $2 and tenant_id = $3`,
}

func (s PostgresStore) Scan(ctx context.Context, table string, fn func(Row) error) error {
	q, ok := scanSQL[table]
	if !ok {
		return fmt.Errorf("backfill: unknown table %q", table)
	}
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return fmt.Errorf("backfill: scan %s: %w", table, err)
	}
	// Collect first: fn may write, and a pgx connection cannot run another
	// statement while rows are still being read on it.
	var all []Row
	for rows.Next() {
		var r Row
		var body []byte
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Type, &body); err != nil {
			rows.Close()
			return fmt.Errorf("backfill: read %s: %w", table, err)
		}
		r.Body = json.RawMessage(body)
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backfill: rows %s: %w", table, err)
	}
	for _, r := range all {
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}

func (s PostgresStore) Update(ctx context.Context, table string, row Row, body json.RawMessage) error {
	q, ok := updateSQL[table]
	if !ok {
		return fmt.Errorf("backfill: unknown table %q", table)
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "select set_config('app.tenant_id', $1, true)", row.TenantID.String()); err != nil {
			return fmt.Errorf("set tenant guc: %w", err)
		}
		tag, err := tx.Exec(ctx, q, []byte(body), row.ID, row.TenantID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("expected 1 row updated, got %d", tag.RowsAffected())
		}
		return nil
	})
}
