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
		CREATE TABLE IF NOT EXISTS users (
    		id       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    		username TEXT NOT NULL UNIQUE
		);

		CREATE TABLE IF NOT EXISTS chats (
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY
		);

		CREATE TABLE IF NOT EXISTS chat_members (
			user_id BIGINT NOT NULL,
			chat_id BIGINT NOT NULL,

			CONSTRAINT pk_chat_members
				PRIMARY KEY (user_id, chat_id),

			CONSTRAINT fk_chat_members_user
				FOREIGN KEY (user_id)
				REFERENCES users(id)
				ON DELETE CASCADE,

			CONSTRAINT fk_chat_members_chat
				FOREIGN KEY (chat_id)
				REFERENCES chats(id)
				ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS messages (
			id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			text    TEXT NOT NULL,
			chat_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,

			CONSTRAINT fk_messages_chat
				FOREIGN KEY (chat_id)
				REFERENCES chats(id),

			CONSTRAINT fk_messages_user
				FOREIGN KEY (user_id)
				REFERENCES users(id)
		);
	`
	_, err := pool.Exec(ctx, query)

	return err
}
