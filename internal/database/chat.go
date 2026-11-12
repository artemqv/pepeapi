package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrChatMembersNotEditable = errors.New("chat members cannot be edited")

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
	result, err := r.pool.Exec(ctx, `
		INSERT INTO chat_members (user_id, chat_id)
		SELECT $1, c.id
		FROM chats AS c
		WHERE c.id = $2
		  AND NOT EXISTS (SELECT 1 FROM direct_chats AS d WHERE d.chat_id = c.id)
	`, userID, chatID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrChatMembersNotEditable
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
func (r *Repository) IsChatMember(
	ctx context.Context,
	chatID, userID int,
) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(
		ctx,
		`
			SELECT EXISTS (
				SELECT 1
				FROM chat_members
				WHERE chat_id = $1 AND user_id = $2
			)
		`,
		chatID,
		userID,
	).Scan(&exists)

	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) GetOrCreateDirectChat(
	ctx context.Context,
	userID, peerID int,
) (Chat, error) {
	var chat Chat
	if userID == peerID {
		return Chat{}, errors.New("userID and peerID are the same, cannot create a direct chat with oneself")
	}

	lowID, highID := userID, peerID
	if userID > peerID {
		lowID, highID = peerID, userID
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Chat{}, err
	}

	defer tx.Rollback(ctx)
	err = tx.QueryRow(
		ctx,
		`
			SELECT chat_id
			FROM direct_chats
			WHERE user_low_id = $1 AND user_high_id = $2
		`,
		lowID,
		highID,
	).Scan(&chat.ID)

	if err == nil {
		return chat, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Chat{}, err
	}
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO chats DEFAULT VALUES
			RETURNING id
		`,
	).Scan(&chat.ID)
	if err != nil {
		return Chat{}, err
	}
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO direct_chats (user_low_id, user_high_id, chat_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_low_id, user_high_id) DO NOTHING
			RETURNING chat_id
		`,
		lowID,
		highID,
		chat.ID,
	).Scan(&chat.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if err := tx.Rollback(ctx); err != nil {
				return Chat{}, err
			}
			if err := r.pool.QueryRow(
				ctx,
				`
					SELECT chat_id
					FROM direct_chats
					WHERE user_low_id = $1 AND user_high_id = $2
				`,
				lowID,
				highID,
			).Scan(&chat.ID); err != nil {
				return Chat{}, err
			}
			return chat, nil
		}
		return Chat{}, err
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO chat_members (user_id, chat_id)
		VALUES ($1, $3), ($2, $3)
		`,
		lowID,
		highID,
		chat.ID,
	)
	if err != nil {
		return Chat{}, err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return Chat{}, err
	}

	return chat, nil
}
