package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
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

func getItem(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")
	id, err := strconv.Atoi(rawID)

	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if id != 42 {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}

	item := Item{
		ID:     42,
		Name:   "pipiska",
		Vremya: "5",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)

}
func ping(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "pong")
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /ping", ping)
	mux.HandleFunc("GET /items/{id}", getItem)

	handler := logging(mux)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	go func() {
		log.Println("listening on :8080")

		err := srv.ListenAndServe()

		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	<-ctx.Done()

	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	err := srv.Shutdown(shutdownCtx)
	if err != nil {
		log.Printf("shutdown error: %v", err)
	}

	log.Println("server stopped")
}
