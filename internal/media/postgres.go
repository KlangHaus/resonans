package media

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const assetColumns = `id, tenant_id, kind, storage_key, filename, mime, size_bytes, width, height, hotspot, created_at, deleted_at`

// PostgresStore is the RLS-backed asset store. Every operation runs in a
// tenant-scoped transaction (set_config('app.tenant_id', …, is_local)) so the
// assets_tenant_isolation RLS policy scopes all rows. Same pattern as
// content.PostgresStore.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore wraps a pgx pool as an asset Store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

var _ Store = (*PostgresStore)(nil)

func (s *PostgresStore) inTx(ctx context.Context, tenantID uuid.UUID, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("media: begin tx: %w", err)
	}
	if _, err := tx.Exec(ctx, "select set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("media: set tenant guc: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("media: commit tx: %w", err)
	}
	return nil
}

func (s *PostgresStore) Create(ctx context.Context, tenantID uuid.UUID, a Asset) (Asset, error) {
	var out Asset
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		hotspot := a.Hotspot
		if len(hotspot) == 0 {
			hotspot = []byte(`{}`)
		}
		row := tx.QueryRow(ctx, `
			insert into resonans_app.assets
			  (tenant_id, kind, storage_key, filename, mime, size_bytes, width, height, hotspot)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			returning `+assetColumns,
			tenantID, a.Kind, a.StorageKey, a.Filename, a.Mime, a.SizeBytes, a.Width, a.Height, []byte(hotspot))
		var scanErr error
		out, scanErr = scanAsset(row)
		return scanErr
	})
	return out, err
}

func (s *PostgresStore) Get(ctx context.Context, tenantID, id uuid.UUID) (Asset, error) {
	var out Asset
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, "select "+assetColumns+" from resonans_app.assets where id = $1 and deleted_at is null", id)
		var scanErr error
		out, scanErr = scanAsset(row)
		if errors.Is(scanErr, ErrNotFound) {
			// Distinguish "in the bin" from "never existed", in the SAME
			// transaction so RLS still scopes the lookup to this tenant.
			return classifyAssetMiss(ctx, tx, id)
		}
		return scanErr
	})
	return out, err
}

func (s *PostgresStore) List(ctx context.Context, tenantID uuid.UUID, f ListFilter) ([]Asset, error) {
	out := make([]Asset, 0)
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		where := " where deleted_at is null"
		switch f.Deleted {
		case DeletedOnly:
			where = " where deleted_at is not null"
		case DeletedInclude:
			where = ""
		case DeletedExclude:
		}
		rows, err := tx.Query(ctx, "select "+assetColumns+" from resonans_app.assets"+where+" order by created_at desc, id asc")
		if err != nil {
			return fmt.Errorf("media: list: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAsset(rows)
			if err != nil {
				return fmt.Errorf("media: list scan: %w", err)
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// classifyAssetMiss answers why a lookup found nothing: the row is in the bin,
// or it does not exist for this tenant. It MUST run inside the caller's
// transaction, because that is where the tenant GUC is set — outside it, this
// query would either fail or answer about another tenant's row.
func classifyAssetMiss(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var exists bool
	if err := tx.QueryRow(ctx,
		"select true from resonans_app.assets where id = $1 and deleted_at is not null", id,
	).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("media: classify miss: %w", err)
	}
	return ErrDeleted
}

// SoftDelete moves the asset to the bin. The `deleted_at is null` in the WHERE
// is what makes a second delete report ErrDeleted instead of silently rewriting
// the original timestamp.
func (s *PostgresStore) SoftDelete(ctx context.Context, tenantID, id uuid.UUID) (Asset, error) {
	var out Asset
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			update resonans_app.assets set deleted_at = now()
			where id = $1 and deleted_at is null
			returning `+assetColumns, id)
		var scanErr error
		out, scanErr = scanAsset(row)
		if errors.Is(scanErr, ErrNotFound) {
			return classifyAssetMiss(ctx, tx, id)
		}
		return scanErr
	})
	return out, err
}

// Restore brings the asset back out of the bin.
func (s *PostgresStore) Restore(ctx context.Context, tenantID, id uuid.UUID) (Asset, error) {
	var out Asset
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			update resonans_app.assets set deleted_at = null
			where id = $1 and deleted_at is not null
			returning `+assetColumns, id)
		var scanErr error
		out, scanErr = scanAsset(row)
		if errors.Is(scanErr, ErrNotFound) {
			// Nothing to restore: either it is already live, or it is not ours.
			var live bool
			if err := tx.QueryRow(ctx,
				"select true from resonans_app.assets where id = $1 and deleted_at is null", id,
			).Scan(&live); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrNotFound
				}
				return fmt.Errorf("media: classify restore: %w", err)
			}
			return ErrNotDeleted
		}
		return scanErr
	})
	return out, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAsset(sc scanner) (Asset, error) {
	var a Asset
	if err := sc.Scan(
		&a.ID, &a.TenantID, &a.Kind, &a.StorageKey, &a.Filename, &a.Mime,
		&a.SizeBytes, &a.Width, &a.Height, &a.Hotspot, &a.CreatedAt, &a.DeletedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Asset{}, ErrNotFound
		}
		return Asset{}, fmt.Errorf("media: scan: %w", err)
	}
	return a, nil
}
