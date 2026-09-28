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

// ensureSchema makes resonans_app exist without requiring CREATE on the
// database when it already does.
//
// Measured (postgres:16, 2026-09-28): `create schema if not exists` checks
// ACL_CREATE on the DATABASE *before* the existence check, so a least-privilege
// migrator (NOSUPERUSER NOCREATEDB, on a database owned by someone else) is
// refused with SQLSTATE 42501 even though the schema is already there and
// nothing would be created. Three databases, same role shape, one run:
// schema present + no database CREATE = denied; + grant create on database =
// ok; migrator owns the database = ok. So the older comment claiming
// "if not exists then finds it already present" was wrong, and every setup
// guide that pre-creates the schema as the superuser and hands the migrator
// only schema-level rights hit it. Looking first costs one cheap catalog
// read and lets those setups work at all.
func ensureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	var exists bool
	if err := pool.QueryRow(ctx,
		"select to_regnamespace('resonans_app') is not null").Scan(&exists); err != nil {
		return fmt.Errorf("migrate: look up schema: %w", err)
	}
	if exists {
		return nil
	}
	if _, err := pool.Exec(ctx, "create schema resonans_app"); err != nil {
		return fmt.Errorf("migrate: create schema resonans_app (absent, and this role "+
			"may not create it — either create it as a superuser with "+
			"`create schema resonans_app authorization <migrator>` or grant the "+
			"migrator CREATE on the database): %w", err)
	}
	return nil
}

func migrateFS(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) ([]string, error) {
	if err := ensureSchema(ctx, pool); err != nil {
		return nil, err
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
