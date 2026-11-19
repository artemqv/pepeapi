package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"my-api/internal/database"

	"github.com/gorilla/websocket"
)

func TestWebSocketHandshakeAuthorization(t *testing.T) {
	tests := []struct {
		name       string
		hasSession bool
		isMember   bool
		wantStatus int
	}{
		{name: "no session", wantStatus: http.StatusUnauthorized},
		{name: "non-member", hasSession: true, wantStatus: http.StatusForbidden},
		{name: "member", hasSession: true, isMember: true, wantStatus: http.StatusSwitchingProtocols},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &chatMessagesRepoStub{isMember: tc.isMember}
			hub := &Hub{register: make(chan *Client, 1), unregister: make(chan *Client, 1)}
			mux := http.NewServeMux()
			mux.Handle("GET /chats/{chatID}/ws", requireSession(repo, logger, websocketHandler(repo, hub, logger)))
			server := httptest.NewServer(mux)
			defer server.Close()

			headers := make(http.Header)
			if tc.hasSession {
				headers.Set("Cookie", "session_token=test-session-token")
			}
			dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
			url := "ws" + strings.TrimPrefix(server.URL, "http") + "/chats/7/ws"
			conn, resp, err := dialer.Dial(url, headers)
			if conn != nil {
				defer func() {
					conn.Close()
					select {
					case client := <-hub.unregister:
						close(client.send)
					case <-time.After(2 * time.Second):
						t.Error("WebSocket handler did not unregister the client")
					}
				}()
			}
			if resp == nil {
				t.Fatalf("handshake returned no HTTP response: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("handshake status = %d, want %d (error: %v)", resp.StatusCode, tc.wantStatus, err)
			}
			if tc.isMember {
				if err != nil || conn == nil {
					t.Fatalf("member could not connect: %v", err)
				}
			} else if !errors.Is(err, websocket.ErrBadHandshake) || conn != nil {
				t.Fatalf("expected rejected WebSocket handshake, got %v", err)
			}
		})
	}
}

func TestWebSocketMessagePersistence(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		databaseError error
		wantWrites    int
		wantBroadcast bool
	}{
		{name: "saved message is broadcast", text: "hello", wantWrites: 1, wantBroadcast: true},
		{name: "database failure prevents broadcast", text: "hello", databaseError: errors.New("database unavailable"), wantWrites: 1},
		{name: "whitespace prevents write and broadcast", text: "   "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			hub := &Hub{
				register:   make(chan *Client, 1),
				unregister: make(chan *Client, 1),
				broadcast:  make(chan BroadcastMessage, 2),
			}
			saved := database.Message{ID: 101, ChatID: 7, UserID: 42, Text: tc.text}
			writes := 0
			repo := &chatMessagesRepoStub{isMember: true}
			repo.createMessage = func(chatID, userID int, text string) (database.Message, error) {
				writes++
				if chatID != 7 || userID != 42 || text != tc.text {
					t.Errorf("CreateMessage got chat %d, user %d, text %q", chatID, userID, text)
				}
				if repo.checkedChatID != chatID || repo.checkedUserID != userID {
					t.Error("message saved without checking membership")
				}
				select {
				case <-hub.broadcast:
					t.Error("message broadcast before saving")
				default:
				}
				return saved, tc.databaseError
			}
			mux := http.NewServeMux()
			mux.Handle("GET /chats/{chatID}/ws", requireSession(repo, logger, websocketHandler(repo, hub, logger)))
			server := httptest.NewServer(mux)
			defer server.Close()
			headers := make(http.Header)
			headers.Set("Cookie", "session_token=test-session-token")
			dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
			conn, resp, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/chats/7/ws", headers)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				t.Fatalf("connect: %v", err)
			}
			// Waiting for unregistration also synchronizes access to the repository stub.
			defer func() {
				conn.Close()
				select {
				case client := <-hub.unregister:
					close(client.send)
					if writes != tc.wantWrites {
						t.Errorf("CreateMessage called %d times, want %d", writes, tc.wantWrites)
					}
					select {
					case message := <-hub.broadcast:
						t.Errorf("unexpected broadcast: %+v", message)
					default:
					}
				case <-time.After(2 * time.Second):
					t.Error("handler did not unregister client")
				}
			}()
			var client *Client
			select {
			case client = <-hub.register:
			case <-time.After(2 * time.Second):
				t.Fatal("handler did not register client")
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(tc.text)); err != nil {
				t.Fatalf("send: %v", err)
			}
			if !tc.wantBroadcast {
				conn.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _, err := conn.ReadMessage()
				var closeError *websocket.CloseError
				if !errors.As(err, &closeError) {
					t.Fatalf("expected handler to close connection after rejection, got %v", err)
				}
				return
			}
			var broadcast BroadcastMessage
			select {
			case broadcast = <-hub.broadcast:
			case <-time.After(2 * time.Second):
				t.Fatal("saved message was not broadcast")
			}
			if broadcast.Room != 7 {
				t.Errorf("broadcast room = %d, want 7", broadcast.Room)
			}
			var got database.Message
			if err := json.Unmarshal([]byte(broadcast.Message), &got); err != nil {
				t.Fatalf("broadcast is not message JSON: %v", err)
			}
			if got != saved {
				t.Errorf("broadcast message = %+v, want %+v", got, saved)
			}
			// Deliver the Hub payload through the existing WebSocket writer.
			client.send <- []byte(broadcast.Message)
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			var received database.Message
			if err := conn.ReadJSON(&received); err != nil {
				t.Fatalf("receive saved message: %v", err)
			}
			if received != saved {
				t.Errorf("received message = %+v, want %+v", received, saved)
			}
		})
	}
}
