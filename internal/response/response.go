// Package response provides HTTP response helpers used across handlers.
package response

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/payload"
)

// JSON serialises v as JSON and writes it to the response with the given
// HTTP status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// Success writes a unified success envelope containing the status code,
// a message, optional data array, and optional pagination metadata.
func Success(w http.ResponseWriter, status int, message string, data []any, pagination ...*payload.Pagination) {
	resp := payload.SuccessResponse{
		StatusCode: status,
		Message:    message,
		Data:       data,
	}
	if len(pagination) > 0 {
		resp.Pagination = pagination[0]
	}
	JSON(w, status, resp)
}

// Error writes a unified error envelope containing the status code and
// the error message.
func Error(w http.ResponseWriter, status int, err error) {
	JSON(w, status, payload.ErrorResponse{
		StatusCode: status,
		Message:    err.Error(),
	})
}

// StatusCodeFromError maps sentinel app errors to HTTP status codes.
func StatusCodeFromError(err error) int {
	switch {
	case errors.Is(err, apperror.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, apperror.ErrInvalidURL),
		errors.Is(err, apperror.ErrInvalidPayload),
		errors.Is(err, apperror.ErrURLExpired),
		errors.Is(err, apperror.ErrURLInactive),
		errors.Is(err, apperror.ErrBlockedDomain),
		errors.Is(err, apperror.ErrPasswordReuse):
		return http.StatusBadRequest
	case errors.Is(err, apperror.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, apperror.ErrURLDeleted):
		return http.StatusGone
	case errors.Is(err, apperror.ErrUnauthorized),
		errors.Is(err, apperror.ErrSessionExpired),
		errors.Is(err, apperror.ErrSessionRevoked),
		errors.Is(err, apperror.ErrInvalidToken),
		errors.Is(err, apperror.ErrInvalidRefreshToken),
		errors.Is(err, apperror.ErrInvalidCurrentPassword):
		return http.StatusUnauthorized
	case errors.Is(err, apperror.ErrRateLimited):
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}
