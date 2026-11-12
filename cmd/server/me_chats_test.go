package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"my-api/internal/database"
)

type meChatsRepoStub struct {
	lookupSession func([32]byte) (int, error)
	getChats      func(int) ([]database.Chat, error)
}

func (s meChatsRepoStub) GetActiveSessionUserID(_ context.Context, hash [32]byte) (int, error) {
	return s.lookupSession(hash)
}

func (s meChatsRepoStub) GetChatsByUserID(_ context.Context, userID int) ([]database.Chat, error) {
	return s.getChats(userID)
}

func TestMeChats(t *testing.T) {
	const token = "test-session-token"
	const sessionUserID = 42
	tests := []struct {
		name       string
		cookie     *http.Cookie
		sessionErr error
		wantLookup bool
		wantStatus int
	}{
		{name: "missing cookie", wantStatus: http.StatusUnauthorized},
		{name: "empty cookie", cookie: &http.Cookie{Name: "session_token"}, wantStatus: http.StatusUnauthorized},
		{name: "inactive session", cookie: &http.Cookie{Name: "session_token", Value: token}, sessionErr: pgx.ErrNoRows, wantLookup: true, wantStatus: http.StatusUnauthorized},
		{name: "session lookup fails", cookie: &http.Cookie{Name: "session_token", Value: token}, sessionErr: errors.New("session storage unavailable"), wantLookup: true, wantStatus: http.StatusInternalServerError},
		{name: "active session", cookie: &http.Cookie{Name: "session_token", Value: token}, wantLookup: true, wantStatus: http.StatusOK},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookups, chatQueries := 0, 0
			repo := meChatsRepoStub{
				lookupSession: func(hash [32]byte) (int, error) {
					lookups++
					if hash != sha256.Sum256([]byte(token)) {
						t.Error("session lookup received the wrong token hash")
					}
					return sessionUserID, tc.sessionErr
				},
				getChats: func(userID int) ([]database.Chat, error) {
					chatQueries++
					if userID != sessionUserID {
						t.Errorf("chat lookup user ID = %d, want %d from session", userID, sessionUserID)
					}
					return []database.Chat{{ID: 7}}, nil
				},
			}
			mux := http.NewServeMux()
			mux.Handle("GET /me/chats", requireSession(repo, logger, getChatsHandler(repo, logger)))
			req := httptest.NewRequest(http.MethodGet, "/me/chats?userID=999", nil)
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			wantLookups := 0
			if tc.wantLookup {
				wantLookups = 1
			}
			if lookups != wantLookups {
				t.Errorf("session lookups = %d, want %d", lookups, wantLookups)
			}
			if tc.wantStatus != http.StatusOK {
				if chatQueries != 0 {
					t.Errorf("chat queries = %d, want 0 without an active session", chatQueries)
				}
				return
			}
			if chatQueries != 1 {
				t.Errorf("chat queries = %d, want 1", chatQueries)
			}
			var chats []database.Chat
			if err := json.Unmarshal(rec.Body.Bytes(), &chats); err != nil {
				t.Fatalf("decode chats response: %v", err)
			}
			if len(chats) != 1 || chats[0].ID != 7 {
				t.Errorf("chats = %+v, want one chat with ID 7", chats)
			}
		})
	}
}
