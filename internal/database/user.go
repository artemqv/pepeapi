package database

import (
	"context"
)

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

func (r *Repository) CreateUser(
	ctx context.Context,
	username string,
) (User, error) {
	var user User
	err := r.pool.QueryRow(
		ctx,
		`
			INSERT INTO users (username)
			VALUES ($1)
			RETURNING id, username
		`,
		username,
	).Scan(
		&user.ID,
		&user.Username,
	)
	if err != nil {
		return User{}, err
	}
	return user, nil
}
