-- +goose Up
CREATE TABLE direct_chats (
    chat_id BIGINT PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
    user_low_id BIGINT NOT NULL REFERENCES users(id),
    user_high_id BIGINT NOT NULL REFERENCES users(id),

    CONSTRAINT uq_direct_chats_pair UNIQUE (user_low_id, user_high_id),
    CONSTRAINT chk_direct_chats_user_order CHECK (user_low_id < user_high_id)
);

-- +goose Down
DROP TABLE direct_chats;