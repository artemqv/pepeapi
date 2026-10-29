package main

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

func websocketHandler(hub *Hub, logger *slog.Logger) http.HandlerFunc {
	upgrader := websocket.Upgrader{}

	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warn("websocket upgrade failed", "error", err)
			return
		}

		conn.SetReadLimit(4096)

		client := &Client{
			conn: conn,
			send: make(chan []byte, 16),
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
				logger.Info("websocket client disconnected")
				return
			}

			if messageType == websocket.TextMessage {
				hub.broadcast <- message
			}
		}
	}
}
