package main

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
)

func websocketHandler(hub *Hub, logger *slog.Logger) http.HandlerFunc {
	upgrader := websocket.Upgrader{}

	return func(w http.ResponseWriter, r *http.Request) {
		chatID := r.PathValue("chatID")
		chatIDInt, err := strconv.Atoi(chatID)
		if err != nil || chatIDInt <= 0 {
			logger.Warn("invalid chatID", "error", err)
			http.Error(w, "Invalid chatID", http.StatusBadRequest)
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
		``
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
				logger.Info("websocket client disconnected", "error", err)
				return
			}

			if messageType == websocket.TextMessage {
				hub.broadcast <- BroadcastMessage{
					Room:    chatIDInt,
					Message: string(message),
				}
			}
		}
	}

}
