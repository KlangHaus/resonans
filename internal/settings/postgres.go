package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the persistent, RLS-backed Store. Same transaction+GUC
// pattern as content.PostgresStore: every operation sets app.tenant_id
// (is_local=true) so the tenant_settings_isolation policy scopes rows.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore wraps a pgx pool as a settings Store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

var _ Store = (*PostgresStore)(nil)

func (s *PostgresStore) inTx(ctx context.Context, tenantID uuid.UUID, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("settings: begin tx: %w", err)
	}
	if _, err := tx.Exec(ctx, "select set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("settings: set tenant guc: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("settings: commit tx: %w", err)
	}
	return nil
}

func (s *PostgresStore) EnabledTypes(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	var out []string
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		var raw []byte
		err := tx.QueryRow(
			ctx,
			`select enabled_types from tenant_settings where tenant_id = $1`, tenantID,
		).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // no row = all types enabled (empty slice)
		}
		if err != nil {
			return fmt.Errorf("settings: read: %w", err)
		}
		return json.Unmarshal(raw, &out)
	})
	return out, err
}

func (s *PostgresStore) SetEnabledTypes(ctx context.Context, tenantID uuid.UUID, types []string) error {
	if types == nil {
		types = []string{}
	}
	raw, err := json.Marshal(types)
	if err != nil {
		return fmt.Errorf("settings: marshal: %w", err)
	}
	return s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`insert into tenant_settings (tenant_id, enabled_types, updated_at)
			 values ($1, $2, now())
			 on conflict (tenant_id) do update set enabled_types = excluded.enabled_types, updated_at = now()`,
			tenantID, raw)
		if err != nil {
			return fmt.Errorf("settings: upsert: %w", err)
		}
		return nil
	})
}

func (s *PostgresStore) PreviewURL(ctx context.Context, tenantID uuid.UUID) (string, error) {
	var out string
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`select preview_url from tenant_settings where tenant_id = $1`, tenantID,
		).Scan(&out)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("settings: read preview url: %w", err)
		}
		return nil
	})
	return out, err
}

func (s *PostgresStore) SetPreviewURL(ctx context.Context, tenantID uuid.UUID, template string) error {
	return s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`insert into tenant_settings (tenant_id, preview_url, updated_at)
			 values ($1, $2, now())
			 on conflict (tenant_id) do update set preview_url = excluded.preview_url, updated_at = now()`,
			tenantID, template)
		if err != nil {
			return fmt.Errorf("settings: upsert preview url: %w", err)
		}
		return nil
	})
}
