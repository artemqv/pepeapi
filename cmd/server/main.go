package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"my-api/internal/database"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

//go:generate mockgen -source=main.go -destination=item_repository_mock_test.go -package=main

type itemGetter interface {
	GetItem(
		context.Context,
		int,
	) (database.Item, error)
}

func logging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		started := time.Now()

		h.ServeHTTP(w, r)

		log.Printf(
			"%s %s took %s",
			r.Method,
			r.URL.Path,
			time.Since(started),
		)
	})
}

func getItem(repo itemGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))

		if err != nil || id <= 0 {
			http.Error(
				w,
				"invalid id",
				http.StatusBadRequest,
			)
			return
		}

		item, err := repo.GetItem(
			r.Context(),
			id,
		)

		if errors.Is(err, database.ErrItemNotFound) {
			http.Error(
				w,
				"item not found",
				http.StatusNotFound,
			)
			return
		}

		if err != nil {
			log.Println("database error:", err)

			http.Error(
				w,
				"internal server error",
				http.StatusInternalServerError,
			)
			return
		}

		w.Header().Set(
			"Content-Type",
			"application/json",
		)

		if err := json.NewEncoder(w).Encode(item); err != nil {
			log.Println("json error:", err)
		}
	}
}
func ping(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "pong")
}

func main() {
	// Контекст для подключения к БД
	dbCtx := context.Background()

	dsn := "postgres://app:secret@localhost:5432/myapi?sslmode=disable"

	// Создаём пул соединений
	pool, err := database.NewPool(dbCtx, dsn)
	if err != nil {
		log.Fatal("database connection error:", err)
	}
	defer pool.Close()

	repo := database.NewRepository(pool)

	log.Println("database connected")

	err = database.CreateTables(dbCtx, pool)
	if err != nil {
		log.Fatal("create tables error:", err)
	}

	log.Println("database tables ready")

	// HTTP маршруты
	mux := http.NewServeMux()

	mux.HandleFunc(
		"GET /ping",
		ping,
	)

	mux.HandleFunc(
		"GET /items/{id}",
		getItem(repo),
	)

	handler := logging(mux)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	// Запускаем HTTP сервер
	go func() {
		log.Println("listening on :8080")

		err := srv.ListenAndServe()

		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
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

	log.Println("shutting down...")

	// Даём серверу максимум 10 секунд на graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		log.Printf("shutdown error: %v", err)
	}

	log.Println("server stopped")
}
