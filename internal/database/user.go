package database

import (
	"context"
	"time"
)

type User struct {
	ID          int       `json:"id"`
	Username    string    `json:"username"`
	DisplayName *string   `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

func (r *Repository) CreateUser(
	ctx context.Context,
	username string,
	displayName *string,
	passwordHash string,
) (User, error) {

	var user User
	err := r.pool.QueryRow(
		ctx,
		`
			INSERT INTO users (username, display_name, password_hash)
			VALUES ($1, $2, $3)
			RETURNING id, username, display_name, created_at
		`,
		username,
		displayName,
		passwordHash,
	).Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.CreatedAt,
	)
	if err != nil {
		return User{}, err
	}
	return user, nil
}
