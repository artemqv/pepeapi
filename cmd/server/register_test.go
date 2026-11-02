package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"my-api/internal/auth"
	"my-api/internal/database"

	"github.com/jackc/pgx/v5/pgconn"
)

// registerRepoFunc is an in-memory implementation of registrationCreator.
// It lets us test the HTTP handler without starting PostgreSQL.
type registerRepoFunc func(context.Context, string, *string, string) (database.User, error)

func (f registerRepoFunc) CreateUser(
	ctx context.Context,
	username string,
	displayName *string,
	passwordHash string,
) (database.User, error) {
	return f(ctx, username, displayName, passwordHash)
}

func runRegisterRequest(repo registrationCreator, body string) *httptest.ResponseRecorder {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", registerHandler(repo, logger))

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	return response
}

func TestRegisterHandlerSuccess(t *testing.T) {
	const password = "Pepe12345"
	const username = "pepe"
	const displayName = "Pepe Frog"
	createdAt := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)
	calls := 0

	repo := registerRepoFunc(func(ctx context.Context, gotUsername string, gotDisplayName *string, passwordHash string) (database.User, error) {
		calls++
		if ctx == nil {
			t.Fatal("context must not be nil")
		}
		if gotUsername != username {
			t.Fatalf("username = %q, want %q", gotUsername, username)
		}
		if gotDisplayName == nil || *gotDisplayName != displayName {
			t.Fatalf("display name = %v, want %q", gotDisplayName, displayName)
		}
		if passwordHash == password || !strings.HasPrefix(passwordHash, "$argon2id$") {
			t.Fatalf("repository received an invalid password hash")
		}
		valid, err := auth.VerifyPassword(password, passwordHash)
		if err != nil || !valid {
			t.Fatalf("stored hash does not verify: valid=%v, err=%v", valid, err)
		}
		return database.User{
			ID:          42,
			Username:    gotUsername,
			DisplayName: gotDisplayName,
			CreatedAt:   createdAt,
		}, nil
	})

	response := runRegisterRequest(repo, `{"username":"pepe","display_name":"Pepe Frog","password":"Pepe12345"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %q", response.Code, response.Body.String())
	}
	if calls != 1 {
		t.Fatalf("CreateUser calls = %d, want 1", calls)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var user database.User
	if err := json.Unmarshal(response.Body.Bytes(), &user); err != nil {
		t.Fatalf("response is not valid user JSON: %v", err)
	}
	if user.ID != 42 || user.Username != username || user.DisplayName == nil || *user.DisplayName != displayName {
		t.Errorf("unexpected user in response: %+v", user)
	}
	if !user.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at = %s, want %s", user.CreatedAt, createdAt)
	}

	// Neither plaintext password nor password_hash must leak in the HTTP response.
	if strings.Contains(response.Body.String(), password) || strings.Contains(response.Body.String(), "password_hash") {
		t.Fatal("HTTP response contains password information")
	}
}

func TestRegisterHandlerInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"invalid JSON", `{"username":`},
		{"empty body", ``},
		{"missing username", `{"display_name":"Pepe Frog","password":"Pepe12345"}`},
		{"missing password", `{"username":"pepe","display_name":"Pepe Frog"}`},
		{"missing display name", `{"username":"pepe","password":"Pepe12345"}`},
		{"null display name", `{"username":"pepe","display_name":null,"password":"Pepe12345"}`},
		{"blank display name", `{"username":"pepe","display_name":"   ","password":"Pepe12345"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			repo := registerRepoFunc(func(context.Context, string, *string, string) (database.User, error) {
				calls++
				return database.User{}, errors.New("CreateUser must not be called")
			})
			response := runRegisterRequest(repo, tc.body)
			if response.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body = %q", response.Code, response.Body.String())
			}
			if calls != 0 {
				t.Errorf("CreateUser called %d times for invalid input", calls)
			}
		})
	}
}

func TestRegisterHandlerDuplicateUsername(t *testing.T) {
	repo := registerRepoFunc(func(context.Context, string, *string, string) (database.User, error) {
		return database.User{}, &pgconn.PgError{Code: "23505", ConstraintName: "users_username_key"}
	})

	response := runRegisterRequest(repo, `{"username":"pepe","display_name":"Pepe Frog","password":"Pepe12345"}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %q", response.Code, response.Body.String())
	}
}

func TestRegisterHandlerDatabaseFailure(t *testing.T) {
	repo := registerRepoFunc(func(context.Context, string, *string, string) (database.User, error) {
		return database.User{}, errors.New("secret database connection details")
	})

	response := runRegisterRequest(repo, `{"username":"pepe","display_name":"Pepe Frog","password":"Pepe12345"}`)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %q", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret database connection details") {
		t.Fatal("internal database error details leaked to client")
	}
}
