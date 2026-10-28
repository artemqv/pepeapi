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
