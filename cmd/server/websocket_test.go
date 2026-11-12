package main

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
