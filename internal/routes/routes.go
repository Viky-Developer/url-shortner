// Package routes registers the application's HTTP routes on a ServeMux.
package routes

import (
	"net/http"

	"github.com/vicky/url-shortner/internal/handler"
)

// New builds a new ServeMux with every application route wired to the given
// URL handler and returns it as an http.Handler.
func New(urlHandler *handler.URLHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /shorten", urlHandler.CreateShortURL)
	mux.HandleFunc("GET /{shortCode}", urlHandler.RedirectShortURL)
	mux.HandleFunc("GET /urls", urlHandler.ListURLs)
	mux.HandleFunc("GET /urls/{id}", urlHandler.GetURLByID)
	mux.HandleFunc("PUT /urls/{id}", urlHandler.UpdateURL)
	mux.HandleFunc("DELETE /urls/{id}", urlHandler.DeleteURL)
	mux.HandleFunc("DELETE /urls/{id}/approve", urlHandler.ApproveHardDelete)

	return mux
}
