// Package persistence holds the Postgres connection pool and the migration
// runner for the Resonans CMS. Postgres backing is Beslutning K (self-host);
// the in-memory content store (internal/content) is the dev/test fallback.
package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool constructs a pgx connection pool and verifies connectivity.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("persistence: parse dsn: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("persistence: connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("persistence: ping postgres: %w", err)
	}
	return pool, nil
}
