// Package routes registers the application's HTTP routes on a ServeMux.
package routes

import (
	"net/http"

	"github.com/vicky/url-shortner/internal/enum"
	"github.com/vicky/url-shortner/internal/handler"
	"github.com/vicky/url-shortner/internal/middleware"
	"github.com/vicky/url-shortner/internal/service"
)

// New builds a new ServeMux with every application route wired to the given
// handlers and returns it as an http.Handler.
func New(urlHandler *handler.URLHandler, authHandler *handler.AuthHandler, adminHandler *handler.AdminHandler, accountHandler *handler.AccountHandler, authService *service.AuthService) http.Handler {
	mux := http.NewServeMux()

	// Health check (public)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Auth routes (public)
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/forgot-password", authHandler.ForgotPassword)

	// Protected auth routes (need access token)
	authMiddleware := middleware.AuthMiddleware(authService, nil)
	mux.Handle("POST /api/v1/auth/refresh", authMiddleware(http.HandlerFunc(authHandler.RefreshToken)))
	mux.Handle("POST /api/v1/auth/change-password", authMiddleware(http.HandlerFunc(authHandler.ChangePassword)))
	mux.Handle("POST /api/v1/auth/logout", authMiddleware(http.HandlerFunc(authHandler.Logout)))
	mux.Handle("GET /api/v1/auth/sessions", authMiddleware(http.HandlerFunc(authHandler.ListSessions)))
	mux.Handle("DELETE /api/v1/auth/sessions/{id}", authMiddleware(http.HandlerFunc(authHandler.RevokeSession)))
	mux.Handle("POST /api/v1/auth/sessions/revoke-others", authMiddleware(http.HandlerFunc(authHandler.RevokeOtherDevices)))
	mux.Handle("POST /api/v1/auth/sessions/revoke-all", authMiddleware(http.HandlerFunc(authHandler.RevokeAllSessions)))

	// URL routes (protected) — userId is derived from the JWT access token
	mux.Handle("POST /api/v1/shorten", authMiddleware(http.HandlerFunc(urlHandler.CreateShortURL)))
	mux.Handle("GET /api/v1/urls/status-counts", authMiddleware(http.HandlerFunc(urlHandler.GetURLStatusCounts)))
	mux.Handle("GET /api/v1/urls", authMiddleware(http.HandlerFunc(urlHandler.ListURLs)))
	mux.Handle("GET /api/v1/urls/{id}", authMiddleware(http.HandlerFunc(urlHandler.GetURLByID)))
	mux.Handle("PATCH /api/v1/urls/{id}", authMiddleware(http.HandlerFunc(urlHandler.UpdateURL)))
	mux.Handle("DELETE /api/v1/urls/{id}", authMiddleware(http.HandlerFunc(urlHandler.DeleteURL)))
	mux.Handle("DELETE /api/v1/urls/{id}/approve", authMiddleware(http.HandlerFunc(urlHandler.ApproveHardDelete)))
	mux.Handle("GET /api/v1/urls/{id}/clicks", authMiddleware(http.HandlerFunc(urlHandler.ListClickLogs)))
	mux.Handle("GET /api/v1/urls/{id}/analytics", authMiddleware(http.HandlerFunc(urlHandler.GetAnalytics)))

	// Account routes (protected) — self-service account deletion lifecycle
	mux.Handle("DELETE /api/v1/account", authMiddleware(http.HandlerFunc(accountHandler.DeleteAccount)))
	mux.Handle("POST /api/v1/account/cancel-deletion", authMiddleware(http.HandlerFunc(accountHandler.CancelDeletion)))
	mux.Handle("GET /api/v1/account/status", authMiddleware(http.HandlerFunc(accountHandler.GetAccountStatus)))

	// Admin routes (protected) — require authentication + ADMIN role
	adminMiddleware := middleware.RequireRole(enum.RoleAdmin, nil)
	mux.Handle("GET /api/v1/admin/blocked-domains", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler.ListBlockedDomains))))
	mux.Handle("POST /api/v1/admin/blocked-domains", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler.CreateBlockedDomain))))
	mux.Handle("DELETE /api/v1/admin/blocked-domains/{id}", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler.DeleteBlockedDomain))))
	mux.Handle("GET /api/v1/admin/blocked-ip-ranges", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler.ListBlockedIPRanges))))
	mux.Handle("POST /api/v1/admin/blocked-ip-ranges", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler.CreateBlockedIPRange))))
	mux.Handle("DELETE /api/v1/admin/blocked-ip-ranges/{id}", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler.DeleteBlockedIPRange))))
	mux.Handle("DELETE /api/v1/admin/users/{id}/soft-delete", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler.SoftDeleteUser))))
	mux.Handle("DELETE /api/v1/admin/users/{id}/hard-delete", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler.HardDeleteUser))))
	mux.Handle("POST /api/v1/admin/maintenance/purge-sessions", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler.PurgeSessions))))
	mux.Handle("POST /api/v1/admin/maintenance/purge-password-history", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler.PurgePasswordHistory))))

	// Public redirect (no auth needed)
	mux.HandleFunc("GET /api/v1/{shortCode}", urlHandler.RedirectShortURL)

	return mux
}
