// Package middleware provides authentication and authorization middleware.
package middleware

import (
	"net/http"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/contextutil"
	"github.com/vicky/url-shortner/internal/enum"
)

// RequireRole returns middleware that checks whether the authenticated user
// has the required role. The role is expected to be in the request context
// (set by AuthMiddleware). Returns 403 Forbidden if the role doesn't match.
func RequireRole(required enum.Role, log logger.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log, _ = logger.New()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, _ := r.Context().Value(contextutil.RoleKey).(string)
			if enum.Role(role) != required {
				log.Warn("access denied: insufficient role",
					logger.String("required", string(required)),
					logger.String("actual", role),
				)
				writeJSONError(w, http.StatusForbidden, "access denied: insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
