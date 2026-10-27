package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	tracer := otel.Tracer("pepeapi/database")

	ctx, span := tracer.Start(
		ctx,
		"database.GetItem",
	)
	defer span.End()

	itemIDAttr := attribute.Int("item.id", id)
	span.SetAttributes(itemIDAttr)

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
		outcomeAttr := attribute.String("outcome", "not_found")
		span.SetAttributes(outcomeAttr)
		return Item{}, ErrItemNotFound
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			"database operation failed",
		)
		outcomeAttr := attribute.String("outcome", "error")
		span.SetAttributes(outcomeAttr)
		return Item{}, fmt.Errorf(
			"get item %d: %w",
			id,
			err,
		)

	}
	outcomeAttr := attribute.String("outcome", "success")
	span.SetAttributes(outcomeAttr)
	return item, nil
}
