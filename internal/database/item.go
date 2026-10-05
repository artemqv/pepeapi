package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrItemNotFound = errors.New("item not found")

type Item struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Vremya string `json:"vremya"`
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
		&item.Vremya,
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
