package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(
	ctx context.Context,
	dsn string,
) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func CreateTables(
	ctx context.Context,
	pool *pgxpool.Pool,
) error {
	query := `
		CREATE TABLE IF NOT EXISTS items (
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			name TEXT NOT NULL,
			vremya TEXT NOT NULL
		);
	`

	_, err := pool.Exec(ctx, query)

	return err
}
