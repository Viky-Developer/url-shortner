// Package middleware provides authentication middleware.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/contextutil"
	"github.com/vicky/url-shortner/internal/service"
)

// writeJSONError writes a JSON error response with the given status code.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"statusCode": status,
		"error":      message,
	}); err != nil {
		return
	}
}

// AuthMiddleware validates JWT access tokens, verifies the session is
// still alive, decodes the encoded user ID, and adds the internal
// integer user ID to the request context.
func AuthMiddleware(authService *service.AuthService, log logger.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log, _ = logger.New()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				log.Warn("missing authorization header")
				writeJSONError(w, http.StatusUnauthorized, "authorization header required")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				log.Warn("invalid authorization header format")
				writeJSONError(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			tokenString := parts[1]
			claims, err := authService.ValidateAccessToken(tokenString)
			if err != nil {
				log.Warn("invalid access token", logger.Error(err))
				writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			// Validate that the session is still alive (not revoked / expired)
			if claims.SessionID > 0 {
				alive, err := authService.ValidateSession(r.Context(), claims.SessionID)
				if err != nil {
					log.Error("session validation query failed", logger.Error(err), logger.Int64("sessionID", claims.SessionID))
					writeJSONError(w, http.StatusInternalServerError, "failed to validate session")
					return
				}
				if !alive {
					log.Warn("session is no longer active", logger.Int64("sessionID", claims.SessionID))
					writeJSONError(w, http.StatusUnauthorized, "session expired or revoked")
					return
				}
			}

			// Decode the HMAC-encoded display user ID to the internal int64
			userID, err := authService.DecodeUserID(claims.UserID)
			if err != nil {
				log.Warn("invalid encoded user ID in token", logger.Error(err))
				writeJSONError(w, http.StatusUnauthorized, "invalid token claims")
				return
			}

			log.Debug("request authenticated", logger.Int64("userID", userID), logger.Int64("sessionID", claims.SessionID))
			ctx := context.WithValue(r.Context(), contextutil.UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
