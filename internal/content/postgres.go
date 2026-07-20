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

// PostgresStore is the persistent Store backed by resonans_app.entries.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore wraps a pgx pool as a content Store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

var _ Store = (*PostgresStore)(nil)

const entryColumns = `id, site, type, slug, title, body, status, published_at, created_at, updated_at`

func (s *PostgresStore) List(ctx context.Context, f ListFilter) ([]Entry, error) {
	var (
		conds []string
		args  []any
	)
	add := func(col, val string) {
		if val != "" {
			args = append(args, val)
			conds = append(conds, fmt.Sprintf("%s = $%d", col, len(args)))
		}
	}
	add("site", f.Site)
	add("type", f.Type)
	add("status", f.Status)

	q := "select " + entryColumns + " from resonans_app.entries"
	if len(conds) > 0 {
		q += " where " + strings.Join(conds, " and ")
	}
	q += " order by created_at desc, id asc"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("content: list: %w", err)
	}
	defer rows.Close()

	out := make([]Entry, 0)
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("content: list scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("content: list rows: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) Get(ctx context.Context, id uuid.UUID) (Entry, error) {
	row := s.pool.QueryRow(ctx,
		"select "+entryColumns+" from resonans_app.entries where id = $1", id)
	return scanEntryRow(row)
}

func (s *PostgresStore) GetPublished(ctx context.Context, site, slug string) (Entry, error) {
	row := s.pool.QueryRow(ctx,
		"select "+entryColumns+" from resonans_app.entries "+
			"where site = $1 and slug = $2 and status = 'published'", site, slug)
	return scanEntryRow(row)
}

func (s *PostgresStore) Create(ctx context.Context, in CreateInput) (Entry, error) {
	if err := in.Validate(); err != nil {
		return Entry{}, err
	}
	row := s.pool.QueryRow(ctx, `
		insert into resonans_app.entries (site, type, slug, title, body, status)
		values ($1, $2, $3, $4, $5, 'draft')
		returning `+entryColumns,
		in.Site, in.Type, in.Slug, in.Title, []byte(in.Body))
	e, err := scanEntryRow(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Entry{}, ErrSlugConflict
		}
		return Entry{}, err
	}
	return e, nil
}

func (s *PostgresStore) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (Entry, error) {
	if err := in.Validate(); err != nil {
		return Entry{}, err
	}
	// coalesce($n, col) leaves a column unchanged when the argument is nil.
	var body any
	if in.Body != nil {
		body = []byte(*in.Body)
	}
	row := s.pool.QueryRow(ctx, `
		update resonans_app.entries set
		  type       = coalesce($2, type),
		  title      = coalesce($3, title),
		  body       = coalesce($4, body),
		  updated_at = now()
		where id = $1
		returning `+entryColumns,
		id, in.Type, in.Title, body)
	return scanEntryRow(row)
}

func (s *PostgresStore) Publish(ctx context.Context, id uuid.UUID) (Entry, error) {
	row := s.pool.QueryRow(ctx, `
		update resonans_app.entries set
		  status       = 'published',
		  published_at = now(),
		  updated_at   = now()
		where id = $1
		returning `+entryColumns, id)
	return scanEntryRow(row)
}

// scanner is satisfied by both pgx.Row and pgx.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanEntry(sc scanner) (Entry, error) {
	var e Entry
	if err := sc.Scan(
		&e.ID, &e.Site, &e.Type, &e.Slug, &e.Title,
		&e.Body, &e.Status, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt,
	); err != nil {
		return Entry{}, err
	}
	return e, nil
}

func scanEntryRow(row pgx.Row) (Entry, error) {
	e, err := scanEntry(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Entry{}, ErrNotFound
		}
		return Entry{}, fmt.Errorf("content: scan: %w", err)
	}
	return e, nil
}
