package main

import (
	"context"
	"encoding/json"
	"errors"

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

type Item struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Vremya string `json:"vremya"`
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

func getItem(pool *pgxpool.Pool) http.HandlerFunc {
	return func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		id, err := strconv.Atoi(r.PathValue("id"))

		if err != nil || id <= 0 {
			http.Error(
				w,
				"invalid id",
				http.StatusBadRequest,
			)
			return
		}

		var item Item

		err = pool.QueryRow(
			r.Context(),
			`
				SELECT id, name
				FROM items
				WHERE id = $1
			`,
			id,
		).Scan(
			&item.ID,
			&item.Name,
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
			log.Println(err)
		}
	}
}

func main() {
	// 1. Подключаемся к постгресу

	dbCtx := context.Background()

	dsn := "postgres://app:secret@localhost:5432/myapi?sslmode=disable"

	pool, err := database.NewPool(dbCtx, dsn)
	if err != nil {
		log.Fatal("database connection error:", err)
	}
	defer pool.Close()

	log.Println("database connected")

	// 2. Создаём хттп маршрутизатор

	mux := http.NewServeMux()

	mux.HandleFunc("GET /items/{id}",
		getItem(pool),
	)

	handler := logging(mux)

	// 3. Создаём хттп сервак

	srv := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	// 4. Запускаем хттп сервер

	go func() {
		log.Println("listening on :8080")

		err := srv.ListenAndServe()

		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// 5. Ждём Ctrl+C или SIGTERM

	sigCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	<-sigCtx.Done()

	log.Println("shutting down...")

	// 6. Даём серверу максимум 10 секунд на остановку

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
