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

	mux.HandleFunc("POST /api/v1/{userId}/shorten", urlHandler.CreateShortURL)
	mux.HandleFunc("GET /api/v1/{userId}/{shortCode}", urlHandler.RedirectShortURL)
	mux.HandleFunc("GET /api/v1/{userId}/urls", urlHandler.ListURLs)
	mux.HandleFunc("GET /api/v1/{userId}/urls/{id}", urlHandler.GetURLByID)
	mux.HandleFunc("PATCH /api/v1/{userId}/urls/{id}", urlHandler.UpdateURL)
	mux.HandleFunc("DELETE /api/v1/{userId}/urls/{id}", urlHandler.DeleteURL)
	mux.HandleFunc("DELETE /api/v1/{userId}/urls/{id}/approve", urlHandler.ApproveHardDelete)

	return mux
}
