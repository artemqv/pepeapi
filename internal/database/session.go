package database

import (
	"context"
	"time"
)

func (r *Repository) CreateSession(
	ctx context.Context,
	tokenHash [32]byte,
	userID int,
	expiresAt time.Time,
) error {

	_, err := r.pool.Exec(
		ctx,
		`
			INSERT INTO sessions (token_hash, user_id, expires_at)
			VALUES ($1, $2, $3)
		`,
		tokenHash[:],
		userID,
		expiresAt,
	)

	return err
}
func (r *Repository) GetActiveSessionUserID(
	ctx context.Context,
	tokenHash [32]byte,
) (int, error) {
	var userID int
	err := r.pool.QueryRow(
		ctx,
		`
			SELECT user_id
			FROM sessions
			WHERE token_hash = $1 AND expires_at > NOW()
		`,
		tokenHash[:],
	).Scan(&userID)

	if err != nil {
		return 0, err
	}

	return userID, nil
}
func (r *Repository) DeleteSession(
	ctx context.Context,
	tokenHash [32]byte,
) error {
	_, err := r.pool.Exec(
		ctx,
		`
			DELETE FROM sessions
			WHERE token_hash = $1
		`,
		tokenHash[:],
	)

	return err
}