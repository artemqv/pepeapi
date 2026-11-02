-- +goose Up
ALTER TABLE users
    ADD COLUMN display_name  TEXT,
    ADD COLUMN password_hash TEXT,
    ADD COLUMN created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- +goose Down
ALTER TABLE users
    DROP COLUMN created_at,
    DROP COLUMN password_hash,
    DROP COLUMN display_name;