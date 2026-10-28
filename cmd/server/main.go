package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"my-api/internal/database"
	telemetry "my-api/internal/telemetry"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

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

type userCreator interface {
	CreateUser(
		context.Context,
		string,
	) (database.User, error)
}
type chatCreator interface {
	CreateChat(
		context.Context,
	) (database.Chat, error)
}
type createUserRequest struct {
	Username string `json:"username"`
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
func createUserHandler(repo userCreator, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Warn("invalid request body", "error", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Username == "" {
			logger.Warn("username is required")
			http.Error(w, "username is required", http.StatusBadRequest)
			return
		}
		user, err := repo.CreateUser(
			r.Context(),
			req.Username,
		)
		if err != nil {
			logger.Error("create user error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(user); err != nil {
			logger.Error("json error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
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

	dsn := "postgres://app:secret@localhost:5432/myapi?sslmode=disable"

	// Создаём пул соединений
	pool, err := database.NewPool(dbCtx, dsn)
	if err != nil {
		logger.Error("database connection error:", "error", err)
		return
	}
	defer pool.Close()

	logger.Info("database connected")

	repo := database.NewRepository(pool)

	err = database.CreateTables(dbCtx, pool)
	if err != nil {
		logger.Error("create tables error:", "error", err)
		return
	}

	logger.Info("database tables ready")

	// HTTP маршруты
	mux := http.NewServeMux()
	mux.HandleFunc(
		"POST /chats",
		createChatHandler(repo, logger),
	)
	mux.HandleFunc(
		"GET /items/{id}",
		getItem(repo, logger),
	)
	mux.HandleFunc(
		"POST /users",
		createUserHandler(repo, logger),
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
