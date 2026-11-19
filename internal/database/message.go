package database

import (
	"context"
)

type Message struct {
	ID         int    `json:"id"`
	Text       string `json:"text"`
	UserID     int    `json:"user_id"`
	ChatID     int    `json:"chat_id"`
	AuthorName string `json:"author_name,omitempty"`
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

func (r *Repository) GetMessagesByChatID(
	ctx context.Context,
	chatID int,
) ([]Message, error) {

	var messages []Message

	rows, err := r.pool.Query(
		ctx,
		`
			SELECT m.id, m.text, m.user_id, m.chat_id,
			       COALESCE(NULLIF(BTRIM(u.display_name), ''), '@' || u.username)
			FROM messages AS m
			JOIN users AS u ON u.id = m.user_id
			WHERE m.chat_id = $1
			ORDER BY m.id ASC
		`,
		chatID,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()
	for rows.Next() {
		var message Message

		err := rows.Scan(
			&message.ID,
			&message.Text,
			&message.UserID,
			&message.ChatID,
			&message.AuthorName,
		)
		if err != nil {
			return nil, err
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}
