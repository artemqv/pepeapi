-- +goose Up
ALTER TABLE chat_members
    ADD COLUMN role TEXT NOT NULL DEFAULT 'member'
        CHECK (role IN ('member','owner'));
    CREATE UNIQUE INDEX uq_one_owner_per_chat
    ON chat_members(chat_id)
    WHERE role = 'owner';


-- +goose Down
DROP INDEX uq_one_owner_per_chat;
ALTER TABLE chat_members
    DROP COLUMN role;
