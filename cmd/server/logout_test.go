package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
)

type logoutRepoFunc func(context.Context, [32]byte) error

func (f logoutRepoFunc) DeleteSession(ctx context.Context, hash [32]byte) error {
	return f(ctx, hash)
}

func TestLogoutHandler(t *testing.T) {
	const token = "current-session-token"
	tests := []struct {
		name       string
		cookie     *http.Cookie
		deleteErr  error
		wantCalls  int
		wantStatus int
	}{
		{name: "missing cookie", wantStatus: http.StatusNoContent},
		{name: "empty cookie", cookie: &http.Cookie{Name: "session_token"}, wantStatus: http.StatusNoContent},
		{name: "current session", cookie: &http.Cookie{Name: "session_token", Value: token}, wantCalls: 1, wantStatus: http.StatusNoContent},
		{name: "database failure", cookie: &http.Cookie{Name: "session_token", Value: token}, deleteErr: errors.New("private database error"), wantCalls: 1, wantStatus: http.StatusInternalServerError},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := logoutRepoFunc(func(gotCtx context.Context, hash [32]byte) error {
				calls++
				if gotCtx != ctx {
					t.Error("DeleteSession did not receive request context")
				}
				if hash != sha256.Sum256([]byte(token)) {
					t.Error("DeleteSession received wrong token hash")
				}
				return tc.deleteErr
			})
			mux := http.NewServeMux()
			mux.HandleFunc("POST /logout", logoutHandler(repo, logger))
			req := httptest.NewRequest(http.MethodPost, "/logout", nil).WithContext(ctx)
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			resp := rec.Result()
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if calls != tc.wantCalls {
				t.Errorf("DeleteSession called %d times, want %d", calls, tc.wantCalls)
			}
			if tc.deleteErr != nil {
				if len(resp.Header.Values("Set-Cookie")) != 0 {
					t.Error("failed logout must preserve cookie for retry")
				}
				if rec.Body.String() != "database error\n" {
					t.Errorf("unexpected error response: %q", rec.Body.String())
				}
				return
			}
			if rec.Body.Len() != 0 {
				t.Errorf("204 response has body %q", rec.Body.String())
			}
			// Inspect committed response headers to detect SetCookie after WriteHeader.
			cookies := resp.Cookies()
			if len(cookies) != 1 {
				t.Fatalf("got %d response cookies, want 1", len(cookies))
			}
			cookie := cookies[0]
			if cookie.Name != "session_token" || cookie.Value != "" || cookie.MaxAge >= 0 {
				t.Errorf("expected session cookie deletion, got %+v", cookie)
			}
			if cookie.Path != "" && cookie.Path != "/" {
				t.Errorf("unexpected deletion path %q", cookie.Path)
			}
			if cookie.Domain != "" {
				t.Errorf("unexpected deletion domain %q", cookie.Domain)
			}
		})
	}
}

func TestLogoutRevokesOnlyCurrentSessionAndAllowsRepeatedLogout(t *testing.T) {
	const currentToken = "current-token"
	const otherToken = "other-token"
	sessions := map[[32]byte]int{
		sha256.Sum256([]byte(currentToken)): 42,
		sha256.Sum256([]byte(otherToken)):   42,
	}
	calls := 0
	repo := logoutRepoFunc(func(_ context.Context, hash [32]byte) error {
		calls++
		delete(sessions, hash)
		return nil
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /logout", logoutHandler(repo, logger))
	sessionRepo := meChatsRepoStub{lookupSession: func(hash [32]byte) (int, error) {
		userID, ok := sessions[hash]
		if !ok {
			return 0, pgx.ErrNoRows
		}
		return userID, nil
	}}
	mux.Handle("GET /protected", requireSession(sessionRepo, logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(sessionUserIDKey) != 42 {
			t.Error("protected handler received wrong user ID")
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	request := func(method, path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := request(http.MethodGet, "/protected", currentToken); rec.Code != http.StatusNoContent {
		t.Fatalf("session not active before logout: %d", rec.Code)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		rec := request(http.MethodPost, "/logout", currentToken)
		resp := rec.Result()
		cookies := resp.Cookies()
		resp.Body.Close()
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("logout attempt %d returned status %d, body %q", attempt, rec.Code, rec.Body.String())
		}
		if len(cookies) != 1 || cookies[0].Name != "session_token" || cookies[0].MaxAge >= 0 {
			t.Fatalf("logout attempt %d did not clear cookie", attempt)
		}
	}
	if calls != 2 {
		t.Errorf("DeleteSession called %d times, want 2", calls)
	}
	if rec := request(http.MethodGet, "/protected", currentToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("revoked token returned %d, want 401", rec.Code)
	}
	if rec := request(http.MethodGet, "/protected", otherToken); rec.Code != http.StatusNoContent {
		t.Errorf("other session returned %d, want 204", rec.Code)
	}
}
