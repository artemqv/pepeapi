package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"my-api/internal/auth"
	"my-api/internal/database"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type sessionDeleter interface {
	DeleteSession(
		context.Context,
		[32]byte,
	) error
}
type loginRepository interface {
	GetAuthUserByUsername(
		context.Context,
		string,
	) (database.AuthUser, error)
	CreateSession(
		context.Context,
		[32]byte,
		int,
		time.Time,
	) error
}

func logoutHandler(repo sessionDeleter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		switch {
		case errors.Is(err, http.ErrNoCookie):
			http.SetCookie(
				w,
				&http.Cookie{
					Name:   "session_token",
					Value:  "",
					MaxAge: -1,
				},
			)
			w.WriteHeader(http.StatusNoContent)
		case err != nil:
			logger.Error("reading error", "error", err)
			http.Error(w, "reading error", http.StatusInternalServerError)
			return

		case cookie.Value == "":
			http.SetCookie(
				w,
				&http.Cookie{
					Name:   "session_token",
					Value:  "",
					MaxAge: -1,
				},
			)
			w.WriteHeader(http.StatusNoContent)

		default:
			tokenHash := auth.HashSessionToken(cookie.Value)
			err := repo.DeleteSession(r.Context(), tokenHash)
			if err != nil {
				logger.Error("deletion error", "error", err)
				http.Error(w, "database error", http.StatusInternalServerError)
				return

			}
			logger.Info("logged out successfully")
			http.SetCookie(
				w,
				&http.Cookie{
					Name:   "session_token",
					Value:  "",
					MaxAge: -1,
				},
			)
			w.WriteHeader(http.StatusNoContent)

		}

	}
}
func loginHandler(repo loginRepository, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			logger.Error("JSON error", "error", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.Username == "" || req.Password == "" {
			logger.Warn("Login validation failed", "reason", "missing username or password")
			http.Error(w, "Username and password are required", http.StatusBadRequest)
			return
		}

		authUser, err := repo.GetAuthUserByUsername(
			r.Context(),
			req.Username,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Warn("Login failed", "reason", "user not found")
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}
		if err != nil {
			logger.Error("GetAuthUserByUsername error", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if authUser.PasswordHash == nil {
			logger.Warn("Login failed", "reason", "user not found or password hash is nil")
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}
		valid, err := auth.VerifyPassword(
			req.Password,
			*authUser.PasswordHash,
		)
		if err != nil {
			logger.Error("Password verification error", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !valid {
			logger.Warn("Login failed", "reason", "invalid password")
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}
		authSessionToken, err := auth.GenerateSessionToken()

		if err != nil {
			logger.Error("Failed to generate session token", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		authSessionTokenHash := auth.HashSessionToken(authSessionToken)

		const sessionTTL = 24 * time.Hour
		err =
			repo.CreateSession(
				r.Context(),
				authSessionTokenHash,
				authUser.ID,
				time.Now().Add(sessionTTL),
			)
		if err != nil {
			logger.Error("Failed to create session", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		http.SetCookie(
			w,
			&http.Cookie{
				Name:     "session_token",
				Value:    authSessionToken,
				MaxAge:   int(sessionTTL.Seconds()),
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteLaxMode,
			},
		)
		w.WriteHeader(http.StatusOK)
	}
}
