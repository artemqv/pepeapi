package database

import (
	"context"
	"time"
)

type AuthUser struct {
	ID           int
	Username     string
	PasswordHash *string
}
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

func (r *Repository) GetAuthUserByUsername(
	ctx context.Context,
	username string,
) (AuthUser, error) {
	var user AuthUser
	err := r.pool.QueryRow(
		ctx,
		`
			SELECT id, username, password_hash
			FROM users
			WHERE username = $1
		`,
		username,
	).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
	)
	if err != nil {
		return AuthUser{}, err
	}
	return user, nil
}
