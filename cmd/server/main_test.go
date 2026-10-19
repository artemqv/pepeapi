package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"my-api/internal/database"

	"go.uber.org/mock/gomock"
)

func TestGetItem(t *testing.T) {
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name       string
		path       string
		mock       func(*MockItemRepository)
		wantStatus int
		wantItem   *database.Item
		wantBody   string
	}{
		{
			name: "item found",
			path: "/items/1",

			mock: func(m *MockItemRepository) {
				m.EXPECT().
					GetItem(gomock.Any(), 1).
					Return(
						database.Item{
							ID:     1,
							Name:   "Test Item",
							Vremya: "5",
						},
						nil,
					)
			},

			wantStatus: http.StatusOK,

			wantItem: &database.Item{
				ID:     1,
				Name:   "Test Item",
				Vremya: "5",
			},
		},
		{
			name: "item not found",
			path: "/items/2",

			mock: func(m *MockItemRepository) {
				m.EXPECT().
					GetItem(gomock.Any(), 2).
					Return(
						database.Item{},
						database.ErrItemNotFound,
					)
			},

			wantStatus: http.StatusNotFound,
			wantBody:   "item not found\n",
		},
		{
			name: "repository error",
			path: "/items/3",

			mock: func(m *MockItemRepository) {
				m.EXPECT().
					GetItem(gomock.Any(), 3).
					Return(
						database.Item{},
						databaseErr,
					)
			},

			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal server error\n",
		},
		{
			name:       "id is not number",
			path:       "/items/abc",
			mock:       nil,
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid id\n",
		},
		{
			name:       "id is zero",
			path:       "/items/0",
			mock:       nil,
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid id\n",
		},
		{
			name:       "id is negative",
			path:       "/items/-1",
			mock:       nil,
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid id\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			repo := NewMockItemRepository(ctrl)

			if tt.mock != nil {
				tt.mock(repo)
			}

			mux := http.NewServeMux()

			mux.HandleFunc(
				"GET /items/{id}",
				getItem(repo),
			)

			req := httptest.NewRequest(
				http.MethodGet,
				tt.path,
				nil,
			)

			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf(
					"status = %d, want %d",
					rec.Code,
					tt.wantStatus,
				)
			}

			if tt.wantItem != nil {
				var got database.Item

				err := json.NewDecoder(rec.Body).Decode(&got)
				if err != nil {
					t.Fatalf(
						"cannot decode response: %v",
						err,
					)
				}

				if got != *tt.wantItem {
					t.Errorf(
						"item = %+v, want %+v",
						got,
						*tt.wantItem,
					)
				}

				return
			}

			if rec.Body.String() != tt.wantBody {
				t.Errorf(
					"body = %q, want %q",
					rec.Body.String(),
					tt.wantBody,
				)
			}
		})
	}
}
