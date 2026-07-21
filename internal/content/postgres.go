package content

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueViolation is the Postgres SQLSTATE for a unique-constraint breach.
const uniqueViolation = "23505"

const documentColumns = `id, tenant_id, dataset, type, slug, title, body, status, published_at, created_at, updated_at`

// PostgresStore is the persistent, RLS-backed Store. Every operation runs in a
// transaction that first sets the tenant GUC (set_config('app.tenant_id', …,
// is_local=true)), so the documents_tenant_isolation RLS policy scopes all rows
// to the caller's tenant. The GUC is transaction-local and never leaks to other
// pooled connections. Pattern ported from permissions/internal/persistence/audit.go.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore wraps a pgx pool as a content Store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

var _ Store = (*PostgresStore)(nil)

// inTx runs fn inside a transaction with app.tenant_id set to tenantID for the
// duration of the transaction (drives RLS).
func (s *PostgresStore) inTx(ctx context.Context, tenantID uuid.UUID, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("content: begin tx: %w", err)
	}
	if _, err := tx.Exec(ctx, "select set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("content: set tenant guc: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("content: commit tx: %w", err)
	}
	return nil
}

func (s *PostgresStore) List(ctx context.Context, tenantID uuid.UUID, f ListFilter) ([]Document, error) {
	var (
		conds = []string{"dataset = '" + DefaultDataset + "'"}
		args  []any
	)
	if f.Type != "" {
		args = append(args, f.Type)
		conds = append(conds, fmt.Sprintf("type = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	q := "select " + documentColumns + " from resonans_app.documents where " +
		strings.Join(conds, " and ") + " order by created_at desc, id asc"

	out := make([]Document, 0)
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("content: list: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			d, err := scanDoc(rows)
			if err != nil {
				return fmt.Errorf("content: list scan: %w", err)
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PostgresStore) Get(ctx context.Context, tenantID, id uuid.UUID) (Document, error) {
	var d Document
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"select "+documentColumns+" from resonans_app.documents where id = $1", id)
		var scanErr error
		d, scanErr = scanDocRow(row)
		return scanErr
	})
	return d, err
}

func (s *PostgresStore) GetPublished(ctx context.Context, tenantID uuid.UUID, typ, slug string) (Document, error) {
	var d Document
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"select "+documentColumns+" from resonans_app.documents "+
				"where dataset = $1 and type = $2 and slug = $3 and status = 'published'",
			DefaultDataset, typ, slug)
		var scanErr error
		d, scanErr = scanDocRow(row)
		return scanErr
	})
	return d, err
}

func (s *PostgresStore) Create(ctx context.Context, tenantID uuid.UUID, in CreateInput) (Document, error) {
	if err := in.Validate(); err != nil {
		return Document{}, err
	}
	var d Document
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			insert into resonans_app.documents (tenant_id, dataset, type, slug, title, body, status)
			values ($1, $2, $3, $4, $5, $6, 'draft')
			returning `+documentColumns,
			tenantID, DefaultDataset, in.Type, in.Slug, in.Title, []byte(in.Body))
		var scanErr error
		d, scanErr = scanDocRow(row)
		return scanErr
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Document{}, ErrSlugConflict
		}
		return Document{}, err
	}
	return d, nil
}

func (s *PostgresStore) Update(ctx context.Context, tenantID, id uuid.UUID, in UpdateInput) (Document, error) {
	if err := in.Validate(); err != nil {
		return Document{}, err
	}
	var body any
	if in.Body != nil {
		body = []byte(*in.Body)
	}
	var d Document
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			update resonans_app.documents set
			  type       = coalesce($2, type),
			  title      = coalesce($3, title),
			  body       = coalesce($4, body),
			  updated_at = now()
			where id = $1
			returning `+documentColumns,
			id, in.Type, in.Title, body)
		var scanErr error
		d, scanErr = scanDocRow(row)
		return scanErr
	})
	return d, err
}

func (s *PostgresStore) Publish(ctx context.Context, tenantID, id uuid.UUID) (Document, error) {
	var d Document
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			update resonans_app.documents set
			  status       = 'published',
			  published_at = now(),
			  updated_at   = now()
			where id = $1
			returning `+documentColumns, id)
		var scanErr error
		d, scanErr = scanDocRow(row)
		return scanErr
	})
	return d, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDoc(sc scanner) (Document, error) {
	var d Document
	if err := sc.Scan(
		&d.ID, &d.TenantID, &d.Dataset, &d.Type, &d.Slug, &d.Title,
		&d.Body, &d.Status, &d.PublishedAt, &d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		return Document{}, err
	}
	return d, nil
}

func scanDocRow(row pgx.Row) (Document, error) {
	d, err := scanDoc(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Document{}, ErrNotFound
		}
		return Document{}, fmt.Errorf("content: scan: %w", err)
	}
	return d, nil
}
