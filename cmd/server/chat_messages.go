package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"my-api/internal/database"
)

type messageCreator interface {
	CreateMessage(
		context.Context,
		int,
		int,
		string,
	) (database.Message, error)
	IsChatMember(
		context.Context,
		int,
		int,
	) (bool, error)
}

type messageGetter interface {
	GetMessagesByChatID(
		context.Context,
		int,
	) ([]database.Message, error)
	IsChatMember(
		context.Context,
		int,
		int,
	) (bool, error)
}

type createMessageRequest struct {
	Text string `json:"text"`
}

func getMessagesHandler(repo messageGetter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID, err := strconv.Atoi(r.PathValue("chatID"))
		userID, ok := r.Context().Value(sessionUserIDKey).(int)

		if !ok || userID <= 0 {
			logger.Warn("invalid user id")
			http.Error(w, "invalid user id", http.StatusUnauthorized)
			return
		}
		if err != nil || chatID <= 0 {
			logger.Warn("invalid chat id", "error", err)
			http.Error(w, "invalid chat id", http.StatusBadRequest)
			return
		}
		isMember, err := repo.IsChatMember(
			r.Context(),
			chatID,
			userID,
		)
		if err != nil {
			logger.Warn("internal server error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !isMember {
			logger.Warn("user is not a member of the chat")
			http.Error(w, "user is not a member of the chat", http.StatusForbidden)
			return
		}
		messages, err := repo.GetMessagesByChatID(
			r.Context(),
			chatID,
		)
		if err != nil {
			logger.Error("get messages error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(messages); err != nil {
			logger.Error("json error", "error", err)
			return
		}

	}
}

func createMessageHandler(repo messageCreator, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID, err := strconv.Atoi(r.PathValue("chatID"))
		if err != nil || chatID <= 0 {
			http.Error(w, "invalid chat id", http.StatusBadRequest)
			return
		}
		userID, ok := r.Context().Value(sessionUserIDKey).(int)
		if !ok || userID <= 0 {
			logger.Warn("invalid user id")
			http.Error(w, "invalid user id", http.StatusUnauthorized)
			return
		}
		isMember, err := repo.IsChatMember(r.Context(), chatID, userID)
		if err != nil {
			logger.Error("check chat membership", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !isMember {
			logger.Warn("user is not a member of the chat")
			http.Error(w, "user is not a member of the chat", http.StatusForbidden)
			return
		}
		var req createMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("Json init failed", "error", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Text == "" {
			http.Error(w, "only string pls", http.StatusBadRequest)
			return
		}
		message, err := repo.CreateMessage(
			r.Context(),
			chatID,
			userID,
			req.Text,
		)
		if err != nil {
			logger.Error("create message error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(message); err != nil {
			logger.Error("json error", "error", err)
			return
		}
	}
}
