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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

func getItem(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawID := r.PathValue("id")

		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 {
			http.Error(
				w,
				"invalid id",
				http.StatusBadRequest,
			)
			return
		}

		item, err := database.GetItem(
			r.Context(),
			pool,
			id,
		)

		if errors.Is(err, pgx.ErrNoRows) {
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
		getItem(pool),
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
