package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"my-api/internal/auth"
	"my-api/internal/database"
	telemetry "my-api/internal/telemetry"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

//go:generate mockgen -source=main.go -destination=item_repository_mock_test.go -package=main

type itemGetter interface {
	GetItem(
		context.Context,
		int,
	) (database.Item, error)
}

type registrationCreator interface {
	CreateUser(
		context.Context,
		string,
		*string,
		string,
	) (database.User, error)
}
type chatCreator interface {
	CreateChat(
		context.Context,
	) (database.Chat, error)
}

type chatMemberAdder interface {
	AddChatMember(
		context.Context,
		int,
		int,
	) error
}
type messageCreator interface {
	CreateMessage(
		context.Context,
		int,
		int,
		string,
	) (database.Message, error)
}
type messageGetter interface {
	GetMessagesByChatID(
		context.Context,
		int,
	) ([]database.Message, error)
}
type chatGetter interface {
	GetChatsByUserID(
		context.Context,
		int,
	) ([]database.Chat, error)
}
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type registerRequest struct {
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name"`
	Password    string  `json:"password"`
}
type authUserGetter interface {
	GetAuthUserByUsername(
		context.Context,
		string,
	) (database.AuthUser, error)
}

type addChatMemberRequest struct {
	UserID int `json:"user_id"`
}
type createMessageRequest struct {
	UserID int    `json:"user_id"`
	Text   string `json:"text"`
}

func loginHandler(repo authUserGetter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			logger.Error("Invalid JSON", "error", err)
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Username == "" || req.Password == "" {
			logger.Warn("Login validation failed", "reason", "missing username or password")
			http.Error(w, "Username and password are required", http.StatusBadRequest)
			return
		}

		authUser, err := repo.GetAuthUserByUsername(
			r.Context(),
			req.Username,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Warn("Login failed", "reason", "user not found")
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}
		if err != nil {
			logger.Error("GetAuthUserByUsername error", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if authUser.PasswordHash == nil {
			logger.Warn("Login failed", "reason", "user not found or password hash is nil")
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}
		valid, err := auth.VerifyPassword(
			req.Password,
			*authUser.PasswordHash,
		)
		if err != nil {
			logger.Error("Password verification error", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !valid {
			logger.Warn("Login failed", "reason", "invalid password")
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func getChatsHandler(repo chatGetter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.Atoi(r.PathValue("userID"))
		if err != nil || userID <= 0 {
			logger.Warn("invalid user id")
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}

		chats, err := repo.GetChatsByUserID(
			r.Context(),
			userID,
		)
		if err != nil {
			logger.Error("get chats error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(chats); err != nil {
			logger.Error("json error", "error", err)
			return
		}
	}
}

func getMessagesHandler(repo messageGetter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID, err := strconv.Atoi(r.PathValue("chatID"))
		if err != nil || chatID <= 0 {
			logger.Warn("invalid chat id", "error", err)
			http.Error(w, "invalid chat id", http.StatusBadRequest)
			return
		}
		messages, err := repo.GetMessagesByChatID(
			r.Context(),
			chatID,
		)
		if err != nil {
			logger.Warn("internal server error", "error", err)
			http.Error(w, "invalid chat id", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(messages); err != nil {
			logger.Error("json error", "error", err)
			return
		}

	}
}
func createMessageHandler(repo messageCreator, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatID, err := strconv.Atoi(r.PathValue("chatID"))
		if err != nil || chatID <= 0 {
			http.Error(w, "invalid chat id", http.StatusBadRequest)
			return
		}

		var req createMessageRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("Json init failed", "error", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.UserID <= 0 {
			logger.Warn("invalid UserID", "error", err)
			http.Error(w, "invalid UserID", http.StatusBadRequest)
			return

		}
		if req.Text == "" {
			logger.Warn("only string pls", "error", err)
			http.Error(w, "only string pls", http.StatusBadRequest)
			return

		}
		message, err := repo.CreateMessage(
			r.Context(),
			chatID,
			req.UserID,
			req.Text,
		)
		if err != nil {
			logger.Error("create message error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(message); err != nil {
			logger.Error("json error", "error", err)
			return
		}
	}
}

func addChatMemberHandler(repo chatMemberAdder, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addChatMemberRequest
		chatID, err := strconv.Atoi(r.PathValue("chatID"))
		if err != nil || chatID <= 0 {
			logger.Warn("invalid chatID", "error", err)
			http.Error(w, "invalid chatID", http.StatusBadRequest)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("Json init failed", "error", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return

		}
		if req.UserID <= 0 {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		err = repo.AddChatMember(
			r.Context(),
			chatID,
			req.UserID,
		)
		if err != nil {
			logger.Error("add chat member error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}
func createChatHandler(repo chatCreator, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chat, err := repo.CreateChat(r.Context())
		if err != nil {
			logger.Error("create chat error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(chat); err != nil {
			logger.Error("json error", "error", err)
			return
		}
	}
}
func registerHandler(repo registrationCreator, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			logger.Error("Invalid JSON", "error", err)
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Username == "" || req.Password == "" {
			logger.Warn("Registration validation failed", "reason", "missing username or password")
			http.Error(w, "Username and password are required", http.StatusBadRequest)

			return
		}
		if req.DisplayName == nil || strings.TrimSpace(*req.DisplayName) == "" {
			http.Error(w, "Display name is required", http.StatusBadRequest)
			return
		}
		hashedPassword, err := auth.HashPassword(req.Password)
		if err != nil {
			logger.Error("Password hashing failed", "error", err)
			http.Error(w, "Password hashing failed", http.StatusInternalServerError)
			return
		}
		user, err := repo.CreateUser(
			r.Context(),
			req.Username,
			req.DisplayName,
			hashedPassword,
		)
		if err != nil {
			var pgErr *pgconn.PgError

			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				http.Error(w, "Username already exists", http.StatusConflict)
				return
			}

			logger.Error("Create user error", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(user); err != nil {
			logger.Error("Failed to encode registration response", "error", err)
		}
	}
}

func logging(h http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,

	) {
		started := time.Now()

		h.ServeHTTP(w, r)

		logger.Info(
			"request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(started),
		)
	})
}

func getItem(repo itemGetter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		str := r.PathValue("id")
		tracer := otel.Tracer("pepeapi/handlers")
		ctx, span := tracer.Start(
			r.Context(),
			"items.get",
		)

		defer span.End()

		if err != nil || id <= 0 {
			logger.Warn(
				"invalid id",
				"method", r.Method,
				"path", r.URL.Path,
				"str", str,
			)
			http.Error(
				w,
				"invalid id",
				http.StatusBadRequest,
			)
			outcomeAttr := attribute.String("outcome", "invalid_id")
			span.SetAttributes(outcomeAttr)
			return
		}

		item, err := repo.GetItem(
			ctx,
			id,
		)

		if errors.Is(err, database.ErrItemNotFound) {
			logger.Info(
				"item not found",
				"method", r.Method,
				"path", r.URL.Path,
			)
			http.Error(
				w,
				"item not found",
				http.StatusNotFound,
			)
			outcomeAttr := attribute.String("outcome", "not_found")
			span.SetAttributes(outcomeAttr)
			return
		}

		if err != nil {
			logger.Error(
				"database error",
				"method", r.Method,
				"path", r.URL.Path,
				"error", err,
			)

			http.Error(
				w,
				"internal server error",
				http.StatusInternalServerError,
			)
			span.RecordError(err)
			span.SetStatus(
				codes.Error,
				"internal server error")
			outcomeAttr := attribute.String("outcome", "error")
			span.SetAttributes(outcomeAttr)
			return
		}

		w.Header().Set(
			"Content-Type",
			"application/json",
		)

		if err := json.NewEncoder(w).Encode(item); err != nil {
			logger.Error(
				"json error",
				"method", r.Method,
				"path", r.URL.Path,
				"error", err,
			)
			span.RecordError(err)
			span.SetStatus(
				codes.Error,
				"internal server error")
			outcomeAttr := attribute.String("outcome", "error")
			span.SetAttributes(outcomeAttr)
			return

		}

		outcomeAttr := attribute.String("outcome", "success")
		span.SetAttributes(outcomeAttr)

	}
}

func main() {

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	tp, err := telemetry.TraceInit(context.Background(), "pepeapi")
	if err != nil {
		logger.Warn("trace init error:", "error", err)
		return
	}

	// Контекст для подключения к БД
	dbCtx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		logger.Error("DATABASE_URL is not set")
		return
	}

	// Создаём пул соединений
	pool, err := database.NewPool(dbCtx, dsn)
	if err != nil {
		logger.Error("database connection error:", "error", err)
		return
	}
	defer pool.Close()

	logger.Info("database connected")

	repo := database.NewRepository(pool)

	// HTTP маршруты
	mux := http.NewServeMux()

	hub := NewHub()
	go hub.Run()

	mux.HandleFunc(
		"GET /",
		chatPageHandler,
	)

	mux.HandleFunc(
		"GET /users/{userID}/chats",
		getChatsHandler(repo, logger),
	)
	// Создание пользователя
	mux.HandleFunc(
		"POST /register",
		registerHandler(repo, logger),
	)

	// Создание чата
	mux.HandleFunc(
		"POST /chats",
		createChatHandler(repo, logger),
	)

	// Добавление пользователя в чат
	mux.HandleFunc(
		"POST /chats/{chatID}/members",
		addChatMemberHandler(repo, logger),
	)

	// Отправка сообщения
	mux.HandleFunc(
		"POST /chats/{chatID}/messages",
		createMessageHandler(repo, logger),
	)

	// Получение истории сообщений
	mux.HandleFunc(
		"GET /chats/{chatID}/messages",
		getMessagesHandler(repo, logger),
	)

	mux.HandleFunc(
		"GET /chats/{chatID}/ws",
		websocketHandler(hub, logger),
	)
	// Старый маршрут из учебного API
	mux.HandleFunc(
		"GET /items/{id}",
		getItem(repo, logger),
	)
	handler := logging(mux, logger)
	otelHandler := otelhttp.NewHandler(
		handler,
		"http.handler")

	srv := &http.Server{
		Addr:    ":8080",
		Handler: otelHandler,
	}

	// Запускаем HTTP сервер
	go func() {
		logger.Info("listening on :8080")

		err := srv.ListenAndServe()

		if err != nil && err != http.ErrServerClosed {
			logger.Error("server error:", "error", err)
		}
	}()

	// Ожидание сигнала остановки от докера
	sigCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	<-sigCtx.Done()

	logger.Info("shutting down...")

	// Даём серверу максимум 10 секунд на graceful shutdown.
	shutdownCtx, ShutdownCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer ShutdownCancel()

	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		logger.Error("shutdown error:", "error", err)
	}
	telCtx, TelCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer TelCancel()

	err = tp.Shutdown(telCtx)
	if err != nil {
		logger.Error("trace shutdown error:", "error", err)
	}
	logger.Info("server stopped")
}
