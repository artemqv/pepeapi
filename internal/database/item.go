package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Item struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Vremya string `json:"tecno"`
}

func GetItem(
	ctx context.Context,
	pool *pgxpool.Pool,
	id int,
) (Item, error) {
	var item Item

	err := pool.QueryRow(
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

	return item, err
}
