package content

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueViolation is the Postgres SQLSTATE for a unique-constraint breach.
const uniqueViolation = "23505"

const documentColumns = `id, tenant_id, dataset, type, slug, title, body, status, version, published_at, created_at, updated_at, deleted_at`

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
	// The bin is its own axis, not a status: a deleted document keeps the
	// status it had, so a status filter alone would list it again.
	switch f.Deleted {
	case DeletedExclude:
		conds = append(conds, "deleted_at is null")
	case DeletedOnly:
		conds = append(conds, "deleted_at is not null")
	case DeletedInclude:
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
				"where dataset = $1 and type = $2 and slug = $3 and status = 'published' and deleted_at is null",
			DefaultDataset, typ, slug)
		var scanErr error
		d, scanErr = scanDocRow(row)
		return scanErr
	})
	return d, err
}

func (s *PostgresStore) GetBySlug(ctx context.Context, tenantID uuid.UUID, typ, slug string) (Document, error) {
	var d Document
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"select "+documentColumns+" from resonans_app.documents "+
				"where dataset = $1 and type = $2 and slug = $3 and deleted_at is null", DefaultDataset, typ, slug)
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
			  version    = version + 1,
			  updated_at = now()
			where id = $1 and deleted_at is null
			returning `+documentColumns,
			id, in.Type, in.Title, body)
		var scanErr error
		d, scanErr = scanDocRow(row)
		if errors.Is(scanErr, ErrNotFound) {
			return classifyMiss(ctx, tx, id)
		}
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
			  version      = version + 1,
			  updated_at   = now()
			where id = $1 and deleted_at is null
			returning `+documentColumns, id)
		var scanErr error
		d, scanErr = scanDocRow(row)
		if errors.Is(scanErr, ErrNotFound) {
			return classifyMiss(ctx, tx, id)
		}
		if scanErr != nil {
			return scanErr
		}
		// Append a version snapshot (history + rollback).
		_, err := tx.Exec(ctx, `
			insert into resonans_app.document_versions (tenant_id, document_id, version, title, body, status)
			values ($1, $2, $3, $4, $5, $6)`,
			tenantID, d.ID, d.Version, d.Title, []byte(d.Body), d.Status)
		return err
	})
	return d, err
}

func (s *PostgresStore) ListVersions(ctx context.Context, tenantID, docID uuid.UUID) ([]Version, error) {
	out := make([]Version, 0)
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			select id, document_id, version, title, body, status, created_at
			from resonans_app.document_versions
			where document_id = $1 order by version desc`, docID)
		if err != nil {
			return fmt.Errorf("content: list versions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var v Version
			if err := rows.Scan(&v.ID, &v.DocumentID, &v.Version, &v.Title, &v.Body, &v.Status, &v.CreatedAt); err != nil {
				return fmt.Errorf("content: scan version: %w", err)
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *PostgresStore) Rollback(ctx context.Context, tenantID, docID uuid.UUID, version int) (Document, error) {
	var d Document
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		var title string
		var body []byte
		row := tx.QueryRow(ctx,
			`select title, body from resonans_app.document_versions where document_id = $1 and version = $2`,
			docID, version)
		if err := row.Scan(&title, &body); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("content: read version: %w", err)
		}
		// Restore the snapshot as a new draft.
		row2 := tx.QueryRow(ctx, `
			update resonans_app.documents set
			  title = $2, body = $3, status = 'draft', version = version + 1, updated_at = now()
			where id = $1
			returning `+documentColumns, docID, title, body)
		var scanErr error
		d, scanErr = scanDocRow(row2)
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
		&d.Body, &d.Status, &d.Version, &d.PublishedAt, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt,
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

// classifyMiss turns "the update matched no row" into the RIGHT error. Without
// it, an operation on a deleted document would report ErrNotFound — truthful
// about the query, misleading about the world: the editor would recreate
// content that is one click from coming back. The lookup runs in the SAME
// transaction, so RLS scopes it exactly like the statement that missed.
func classifyMiss(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var deleted *time.Time
	err := tx.QueryRow(ctx, "select deleted_at from resonans_app.documents where id = $1", id).Scan(&deleted)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("content: classify miss: %w", err)
	case deleted != nil:
		return ErrDeleted
	default:
		// The row exists, is live, and still did not match: that is not a
		// state this code knows, so it must not be reported as one it does.
		return fmt.Errorf("content: update matched no row for a live document %s", id)
	}
}

// SoftDelete puts a document in the bin. `version` is deliberately NOT bumped:
// deleting is not an edit of the content, and the version numbers the history
// snapshots. The row, its versions and its history stay.
func (s *PostgresStore) SoftDelete(ctx context.Context, tenantID, id uuid.UUID) (Document, error) {
	var d Document
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			update resonans_app.documents set
			  deleted_at = now(),
			  updated_at = now()
			where id = $1 and deleted_at is null
			returning `+documentColumns, id)
		var scanErr error
		d, scanErr = scanDocRow(row)
		if errors.Is(scanErr, ErrNotFound) {
			return classifyMiss(ctx, tx, id)
		}
		return scanErr
	})
	return d, err
}

// Restore takes a document back out of the bin. The partial unique index
// (migrations/0008) covers live rows only, so a slug taken in the meantime
// makes this fail with ErrSlugConflict — the LIVE document keeps the slug,
// because renaming either one would break a URL its readers already use.
func (s *PostgresStore) Restore(ctx context.Context, tenantID, id uuid.UUID) (Document, error) {
	var d Document
	err := s.inTx(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			update resonans_app.documents set
			  deleted_at = null,
			  updated_at = now()
			where id = $1 and deleted_at is not null
			returning `+documentColumns, id)
		var scanErr error
		d, scanErr = scanDocRow(row)
		if errors.Is(scanErr, ErrNotFound) {
			// Either the id is unknown, or the document was never deleted.
			var deleted *time.Time
			e := tx.QueryRow(ctx, "select deleted_at from resonans_app.documents where id = $1", id).Scan(&deleted)
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if e != nil {
				return fmt.Errorf("content: restore classify: %w", e)
			}
			return ErrNotDeleted
		}
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
