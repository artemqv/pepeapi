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

func (r *Repository) GetChatsByUserID(
	ctx context.Context,
	userID int,
) ([]Chat, error) {
	chats := make([]Chat, 0)

	rows, err := r.pool.Query(
		ctx,
		`
			SELECT chat_id
			FROM chat_members
			WHERE user_id = $1
			ORDER BY chat_id ASC
		`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var chat Chat

		if err := rows.Scan(&chat.ID); err != nil {
			return nil, err
		}

		chats = append(chats, chat)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return chats, nil
}
