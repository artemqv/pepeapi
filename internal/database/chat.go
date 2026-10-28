package database

import (
	"context"
)

type Chat struct {
	ID int `json:"id"`
}

func (r *Repository) CreateChat(
	ctx context.Context,
) (Chat, error) {
	var chat Chat

	err := r.pool.QueryRow(
		ctx,
		`
            INSERT INTO chats DEFAULT VALUES
            RETURNING id
        `,
	).Scan(
		&chat.ID,
	)

	if err != nil {
		return Chat{}, err
	}

	return chat, nil
}
func (r *Repository) AddChatMember(
	ctx context.Context,
	chatID int,
	userID int,
) error {
	_, err := r.pool.Exec(
		ctx,
		`
		INSERT INTO chat_members (user_id, chat_id)
		VALUES ($1, $2)
	`,
		userID,
		chatID,
	)
	if err != nil {
		return err
	}
	return nil
}
