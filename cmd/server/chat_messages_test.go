package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"my-api/internal/database"
)

type chatMessagesRepoStub struct {
	checkedChatID int
	checkedUserID int
	messageReads  int
	isMember      bool
	createMessage func(int, int, string) (database.Message, error)
}

func (*chatMessagesRepoStub) GetActiveSessionUserID(context.Context, [32]byte) (int, error) {
	return 42, nil
}

func (r *chatMessagesRepoStub) IsChatMember(_ context.Context, chatID, userID int) (bool, error) {
	r.checkedChatID = chatID
	r.checkedUserID = userID
	return r.isMember, nil
}

func (r *chatMessagesRepoStub) GetMessagesByChatID(context.Context, int) ([]database.Message, error) {
	r.messageReads++
	return nil, nil
}

func TestGetMessagesForbiddenForNonMember(t *testing.T) {
	repo := &chatMessagesRepoStub{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	mux.Handle("GET /chats/{chatID}/messages", requireSession(repo, logger, getMessagesHandler(repo, logger)))

	req := httptest.NewRequest(http.MethodGet, "/chats/7/messages", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "test-session-token"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if repo.checkedChatID != 7 || repo.checkedUserID != 42 {
		t.Errorf("membership checked for chat %d and user %d, want chat 7 and user 42", repo.checkedChatID, repo.checkedUserID)
	}
	if repo.messageReads != 0 {
		t.Errorf("GetMessagesByChatID called %d times, want 0", repo.messageReads)
	}
}

func (r *chatMessagesRepoStub) CreateMessage(_ context.Context, chatID, userID int, text string) (database.Message, error) {
	return r.createMessage(chatID, userID, text)
}

func TestPostChatMessages(t *testing.T) {
	tests := []struct {
		name       string
		isMember   bool
		wantStatus int
		wantWrites int
	}{
		{name: "non-member cannot post", wantStatus: http.StatusForbidden},
		{name: "member posts as session user", isMember: true, wantStatus: http.StatusCreated, wantWrites: 1},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			repo := &chatMessagesRepoStub{isMember: tc.isMember}
			repo.createMessage = func(chatID, userID int, text string) (database.Message, error) {
				writes++
				if repo.checkedChatID != 7 || repo.checkedUserID != 42 {
					t.Error("message write attempted before checking membership for chat 7 and session user 42")
				}
				if chatID != 7 || userID != 42 || text != "hello" {
					t.Errorf("CreateMessage got chat %d, user %d, text %q; want chat 7, session user 42, text hello", chatID, userID, text)
				}
				return database.Message{ID: 1, ChatID: chatID, UserID: userID, Text: text}, nil
			}
			mux := http.NewServeMux()
			mux.Handle("POST /chats/{chatID}/messages", requireSession(repo, logger, createMessageHandler(repo, logger)))

			req := httptest.NewRequest(http.MethodPost, "/chats/7/messages", strings.NewReader(`{"text":"hello","user_id":999}`))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: "session_token", Value: "test-session-token"})
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if repo.checkedChatID != 7 || repo.checkedUserID != 42 {
				t.Errorf("membership checked for chat %d and user %d, want chat 7 and user 42", repo.checkedChatID, repo.checkedUserID)
			}
			if writes != tc.wantWrites {
				t.Errorf("CreateMessage called %d times, want %d", writes, tc.wantWrites)
			}
		})
	}
}
