package database

import (
	"context"
)

type Message struct {
	ID     int    `json:"id"`
	Text   string `json:"text"`
	UserID int    `json:"user_id"`
	ChatID int    `json:"chat_id"`
}

func (r *Repository) CreateMessage(
	ctx context.Context,
	chatID int,
	userID int,
	text string,
) (Message, error) {

	var message Message

	err := r.pool.QueryRow(
		ctx,
		`
			INSERT INTO messages (text, user_id, chat_id)
			VALUES ($1, $2, $3)
			RETURNING id, text, user_id, chat_id
		`,
		text,
		userID,
		chatID,
	).Scan(
		&message.ID,
		&message.Text,
		&message.UserID,
		&message.ChatID,
	)

	if err != nil {
		return Message{}, err
	}

	return message, nil
}
