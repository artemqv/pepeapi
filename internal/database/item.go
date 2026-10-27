package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

var ErrItemNotFound = errors.New("item not found")

type Item struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool: pool,
	}
}

func (r *Repository) GetItem(
	ctx context.Context,
	id int,
) (Item, error) {
	var item Item
	tracer := otel.Tracer("pepeapi/handlers")

	ctx, span := tracer.Start(
		ctx,
		"database.GetItem",
	)
	defer span.End()

	err := r.pool.QueryRow(
		ctx,
		`
			SELECT id, name, vremya
			FROM items
			WHERE id = $1
		`,
		id,
	).Scan(
		&item.ID,
		&item.Name,
		&item.Description,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrItemNotFound
	}

	if err != nil {
		return Item{}, fmt.Errorf(
			"get item %d: %w",
			id,
			err,
		)
	}

	return item, nil
}
