package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"my-api/internal/auth"
	"my-api/internal/database"
)

// loginRepoFunc replaces PostgreSQL in handler tests.
type loginRepoFunc func(context.Context, string) (database.AuthUser, error)

func (f loginRepoFunc) GetAuthUserByUsername(ctx context.Context, username string) (database.AuthUser, error) {
	return f(ctx, username)
}

type loginRepoStub struct {
	loginRepoFunc
	createSession func(context.Context, [32]byte, int, time.Time) error
}

func (s loginRepoStub) CreateSession(ctx context.Context, hash [32]byte, userID int, expiresAt time.Time) error {
	return s.createSession(ctx, hash, userID, expiresAt)
}

func runLoginRequest(lookup loginRepoFunc, body string) *httptest.ResponseRecorder {
	return runLoginRequestWithRepo(loginRepoStub{loginRepoFunc: lookup}, body)
}

func runLoginRequestWithRepo(repo loginRepository, body string) *httptest.ResponseRecorder {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", loginHandler(repo, logger))

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestLoginHandlerCredentials(t *testing.T) {
	const password = "Pepe12345"
	encoded, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	tests := []struct {
		name       string
		password   string
		wantStatus int
		wantBody   string
		sessionErr error
		wantWrites int
	}{
		{name: "correct password", password: password, wantStatus: http.StatusOK, wantWrites: 1},
		{name: "incorrect password", password: "WrongPassword", wantStatus: http.StatusUnauthorized, wantBody: "Invalid username or password\n"},
		{name: "session write fails", password: password, wantStatus: http.StatusInternalServerError, wantBody: "Internal server error\n", sessionErr: errors.New("session storage unavailable"), wantWrites: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			lookup := func(ctx context.Context, username string) (database.AuthUser, error) {
				calls++
				if ctx == nil {
					t.Fatal("request context is nil")
				}
				if username != "pepe" {
					t.Fatalf("username = %q, want pepe", username)
				}
				return database.AuthUser{ID: 42, Username: "pepe", PasswordHash: &encoded}, nil
			}
			var writes int
			var savedHash [32]byte
			repo := loginRepoStub{
				loginRepoFunc: lookup,
				createSession: func(_ context.Context, hash [32]byte, userID int, expiresAt time.Time) error {
					writes++
					savedHash = hash
					if userID != 42 {
						t.Errorf("session user ID = %d, want 42", userID)
					}
					if !expiresAt.After(time.Now()) {
						t.Error("session expiration must be in the future")
					}
					return tc.sessionErr
				},
			}

			body := `{"username":"pepe","password":"` + tc.password + `"}`
			rec := runLoginRequestWithRepo(repo, body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body = %q", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if rec.Body.String() != tc.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tc.wantBody)
			}
			if calls != 1 {
				t.Errorf("GetAuthUserByUsername calls = %d, want 1", calls)
			}
			if writes != tc.wantWrites {
				t.Errorf("CreateSession calls = %d, want %d", writes, tc.wantWrites)
			}
			if tc.wantStatus == http.StatusOK {
				cookies := rec.Result().Cookies()
				if len(cookies) != 1 {
					t.Fatalf("response contains %d cookies, want 1", len(cookies))
				}
				cookie := cookies[0]
				if cookie.Name != "session_token" || cookie.Value == "" || !cookie.HttpOnly {
					t.Fatal("expected a nonempty HttpOnly session_token cookie")
				}
				if auth.HashSessionToken(cookie.Value) != savedHash {
					t.Error("cookie token does not match the saved hash")
				}
			} else if len(rec.Result().Header.Values("Set-Cookie")) != 0 {
				t.Error("failed login must not set a cookie")
			}
			if strings.Contains(rec.Body.String(), encoded) || strings.Contains(rec.Body.String(), password) {
				t.Fatal("password or hash leaked to client")
			}
		})
	}
}

func TestLoginHandlerInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `{"username":`},
		{name: "empty body", body: ""},
		{name: "missing username", body: `{"password":"Pepe12345"}`},
		{name: "missing password", body: `{"username":"pepe"}`},
		{name: "empty username", body: `{"username":"","password":"Pepe12345"}`},
		{name: "empty password", body: `{"username":"pepe","password":""}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			repo := loginRepoFunc(func(context.Context, string) (database.AuthUser, error) {
				calls++
				return database.AuthUser{}, errors.New("repository should not be called")
			})
			rec := runLoginRequest(repo, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body = %q", rec.Code, rec.Body.String())
			}
			if calls != 0 {
				t.Errorf("repository called %d times for invalid input", calls)
			}
		})
	}
}

func TestLoginHandlerUnknownUsername(t *testing.T) {
	repo := loginRepoFunc(func(context.Context, string) (database.AuthUser, error) {
		return database.AuthUser{}, pgx.ErrNoRows
	})
	rec := runLoginRequest(repo, `{"username":"nobody","password":"Pepe12345"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "Invalid username or password\n" {
		t.Errorf("response must not reveal whether username exists; body = %q", rec.Body.String())
	}
}

func TestLoginHandlerNullPasswordHash(t *testing.T) {
	repo := loginRepoFunc(func(context.Context, string) (database.AuthUser, error) {
		return database.AuthUser{ID: 10, Username: "legacy", PasswordHash: nil}, nil
	})
	rec := runLoginRequest(repo, `{"username":"legacy","password":"Pepe12345"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "Invalid username or password\n" {
		t.Errorf("unexpected error details: %q", rec.Body.String())
	}
}

func TestLoginHandlerInvalidStoredHash(t *testing.T) {
	malformed := "$argon2id$v=19$malformed"
	repo := loginRepoFunc(func(context.Context, string) (database.AuthUser, error) {
		return database.AuthUser{ID: 11, Username: "pepe", PasswordHash: &malformed}, nil
	})
	rec := runLoginRequest(repo, `{"username":"pepe","password":"Pepe12345"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %q", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), malformed) {
		t.Fatal("stored hash leaked to client")
	}
}

func TestLoginHandlerDatabaseFailure(t *testing.T) {
	const privateError = "postgresql connection password is secret"
	repo := loginRepoFunc(func(context.Context, string) (database.AuthUser, error) {
		return database.AuthUser{}, errors.New(privateError)
	})
	rec := runLoginRequest(repo, `{"username":"pepe","password":"Pepe12345"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %q", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), privateError) {
		t.Fatal("internal database error leaked to client")
	}
}
