package main

import (
	_ "embed"
	"net/http"
)

//go:embed web/index.html
var chatHTML []byte

func chatPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(chatHTML)
}
