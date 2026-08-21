// Package routes registers the application's HTTP routes on a ServeMux.
package routes

import (
	"net/http"

	"github.com/vicky/url-shortner/internal/handler"
	"github.com/vicky/url-shortner/internal/middleware"
	"github.com/vicky/url-shortner/internal/service"
)

// New builds a new ServeMux with every application route wired to the given
// URL handler and returns it as an http.Handler.
func New(urlHandler *handler.URLHandler, authHandler *handler.AuthHandler, authService *service.AuthService) http.Handler {
	mux := http.NewServeMux()

	// Auth routes (public)
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/forgot-password", authHandler.ForgotPassword)

	// Protected auth routes (need access token)
	authMiddleware := middleware.AuthMiddleware(authService, nil)
	mux.Handle("POST /api/v1/auth/refresh", authMiddleware(http.HandlerFunc(authHandler.RefreshToken)))
	mux.Handle("POST /api/v1/auth/logout", authMiddleware(http.HandlerFunc(authHandler.Logout)))
	mux.Handle("GET /api/v1/auth/sessions", authMiddleware(http.HandlerFunc(authHandler.ListSessions)))
	mux.Handle("DELETE /api/v1/auth/sessions/{id}", authMiddleware(http.HandlerFunc(authHandler.RevokeSession)))
	mux.Handle("PATCH /api/v1/auth/password", authMiddleware(http.HandlerFunc(authHandler.UpdatePassword)))

	// URL routes (protected) — userId is derived from the JWT access token
	mux.Handle("POST /api/v1/shorten", authMiddleware(http.HandlerFunc(urlHandler.CreateShortURL)))
	mux.Handle("GET /api/v1/urls", authMiddleware(http.HandlerFunc(urlHandler.ListURLs)))
	mux.Handle("GET /api/v1/urls/{id}", authMiddleware(http.HandlerFunc(urlHandler.GetURLByID)))
	mux.Handle("PATCH /api/v1/urls/{id}", authMiddleware(http.HandlerFunc(urlHandler.UpdateURL)))
	mux.Handle("DELETE /api/v1/urls/{id}", authMiddleware(http.HandlerFunc(urlHandler.DeleteURL)))
	mux.Handle("DELETE /api/v1/urls/{id}/approve", authMiddleware(http.HandlerFunc(urlHandler.ApproveHardDelete)))
	mux.Handle("GET /api/v1/urls/{id}/clicks", authMiddleware(http.HandlerFunc(urlHandler.ListClickLogs)))
	mux.Handle("GET /api/v1/urls/{id}/analytics", authMiddleware(http.HandlerFunc(urlHandler.GetAnalytics)))

	// Public redirect (no auth needed)
	mux.HandleFunc("GET /api/v1/{shortCode}", urlHandler.RedirectShortURL)

	return mux
}
