package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
)

type chatMemberCheckers interface {
	IsChatMember(
		ctx context.Context,
		chatID,
		userID int,
	) (bool, error)
}

func websocketHandler(repo messageCreator, hub *Hub, logger *slog.Logger) http.HandlerFunc {
	upgrader := websocket.Upgrader{}
	return func(w http.ResponseWriter, r *http.Request) {

		chatID := r.PathValue("chatID")
		chatIDInt, err := strconv.Atoi(chatID)
		if err != nil || chatIDInt <= 0 {
			logger.Warn("invalid chatID", "error", err)
			http.Error(w, "Invalid chatID", http.StatusBadRequest)
			return
		}
		userID, ok := r.Context().Value(sessionUserIDKey).(int)
		if !ok || userID <= 0 {
			logger.Warn("invalid user id")
			http.Error(w, "invalid user id", http.StatusUnauthorized)
			return
		}
		if IsMember, err := repo.IsChatMember(r.Context(), chatIDInt, userID); err != nil {
			logger.Warn("error while checking chat membership", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		} else if !IsMember {
			http.Error(w, "user is not a member of this chat", http.StatusForbidden)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warn("websocket upgrade failed", "error", err)
			return
		}

		conn.SetReadLimit(64 * 1024)

		client := &Client{
			conn: conn,
			send: make(chan []byte, 16),
			room: chatIDInt,
		}

		hub.register <- client

		defer func() {
			hub.unregister <- client
			conn.Close()
		}()

		go func() {
			defer conn.Close()

			for message := range client.send {
				err := conn.WriteMessage(
					websocket.TextMessage,
					message,
				)
				if err != nil {
					logger.Warn("websocket write failed", "error", err)
					return
				}
			}
		}()

		logger.Info("websocket client connected")

		for {
			messageType, message, err := conn.ReadMessage()

			if err != nil {
				logger.Info("client has been disconnected", "error", err)
				return
			}

			if messageType == websocket.TextMessage {
				savedMessage, err := messageCheckup(
					r.Context(),
					repo,
					chatIDInt,
					userID,
					(string(message)),
				)
				if err != nil {
					logger.Error("message checkup error", "error", err)
					return
				}
				data, err := json.Marshal(savedMessage)
				if err != nil {
					logger.Error("marshal message failed", "error", err)
					return
				}

				hub.broadcast <- BroadcastMessage{
					Room:    chatIDInt,
					Message: string(data),
				}

			}
		}
	}

}
func chatMemberCheckHandler(repo chatMemberCheckers, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID := r.PathValue("chatID")
		chatIDInt, err := strconv.Atoi(chatID)
		if err != nil || chatIDInt <= 0 {
			logger.Warn("invalid chatID", "error", err)
			http.Error(w, "Invalid chatID", http.StatusBadRequest)
			return
		}
		userID, ok := r.Context().Value(sessionUserIDKey).(int)
		if !ok || userID <= 0 {
			logger.Warn("invalid user id")
			http.Error(w, "invalid user id", http.StatusUnauthorized)
			return
		}

		isMember, err := repo.IsChatMember(r.Context(), chatIDInt, userID)
		if err != nil {
			logger.Warn("error checking chat membership", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if !isMember {
			http.Error(w, "user is not a member of this chat", http.StatusForbidden)
			return
		}
	}
}
