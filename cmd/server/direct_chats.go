package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"my-api/internal/database"
)

type directChatRepository interface {
	GetUserIDByUsername(ctx context.Context, username string) (int, error)
	GetOrCreateDirectChat(ctx context.Context, userID, peerID int) (database.Chat, error)
}

func openDirectChatHandler(repo directChatRepository, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(sessionUserIDKey).(int)
		if !ok || userID <= 0 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req struct {
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		username := strings.TrimPrefix(strings.TrimSpace(req.Username), "@")
		if username == "" {
			http.Error(w, "username is required", http.StatusBadRequest)
			return
		}

		peerID, err := repo.GetUserIDByUsername(r.Context(), username)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err != nil {
			logger.Error("look up direct chat peer", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if peerID == userID {
			http.Error(w, "cannot open a direct chat with yourself", http.StatusBadRequest)
			return
		}

		chat, err := repo.GetOrCreateDirectChat(r.Context(), userID, peerID)
		if err != nil {
			logger.Error("open direct chat", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(chat); err != nil {
			logger.Error("encode direct chat response", "error", err)
		}
	}
}
