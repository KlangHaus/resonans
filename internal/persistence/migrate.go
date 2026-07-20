package persistence

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KlangHaus/resonans/migrations"
)

// Migrate runs all embedded *.sql migrations that have not yet been applied, in
// lexical filename order, each in its own transaction. Applied state is tracked
// in resonans_app.schema_migrations. Idempotent — safe on every boot. Returns
// the names of newly-applied migrations. Copied from the permissions service.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	return migrateFS(ctx, pool, migrations.FS)
}

func migrateFS(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) ([]string, error) {
	if _, err := pool.Exec(ctx, "create schema if not exists resonans_app"); err != nil {
		return nil, fmt.Errorf("migrate: create schema: %w", err)
	}
	if _, err := pool.Exec(ctx, `
		create table if not exists resonans_app.schema_migrations (
		  name        text        primary key,
		  applied_at  timestamptz not null default now()
		)`); err != nil {
		return nil, fmt.Errorf("migrate: create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var applied []string
	for _, name := range names {
		var seen string
		err := pool.QueryRow(
			ctx,
			"select name from resonans_app.schema_migrations where name = $1", name,
		).Scan(&seen)
		if err == nil {
			continue // already applied
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("migrate: check %s: %w", name, err)
		}

		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", name, err)
		}

		if err := applyOne(ctx, pool, name, string(content)); err != nil {
			return nil, err
		}
		applied = append(applied, name)
	}
	return applied, nil
}

func applyOne(ctx context.Context, pool *pgxpool.Pool, name, content string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrate: begin %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, content); err != nil {
		return fmt.Errorf("migrate: exec %s: %w", name, err)
	}
	if _, err := tx.Exec(
		ctx,
		"insert into resonans_app.schema_migrations (name) values ($1)", name,
	); err != nil {
		return fmt.Errorf("migrate: record %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: commit %s: %w", name, err)
	}
	return nil
}
