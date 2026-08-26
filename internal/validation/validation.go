// Package validation provides a generic request-binding and struct-validation
// helper for net/http handlers. It decodes JSON into a typed struct, runs
// go-playground/validator tags, and writes a standardised error response on failure.
package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-playground/validator/v10"

	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/response"
)

// validate is the package-level validator instance reused across calls.
var validate = validator.New(validator.WithRequiredStructEnabled())

// BindAndValidate decodes the JSON request body into T, validates it
// against the struct's validate:"..." tags, and writes a 400 error on
// failure. Returns (*T, true) on success.
func BindAndValidate[T any](r *http.Request, w http.ResponseWriter) (*T, bool) {
	var req T

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: request body is required", apperror.ErrInvalidPayload))
		} else {
			response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %v", apperror.ErrInvalidPayload, err))
		}
		return nil, false
	}

	if err := validate.Struct(req); err != nil {
		message := messageForError(err)
		if message == "" {
			message = "the request body is missing or invalid"
		}
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %s", apperror.ErrInvalidPayload, message))
		return nil, false
	}

	return &req, true
}

// messageForError extracts a human-readable message from a ValidationErrors.
func messageForError(err error) string {
	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		return ""
	}

	for _, fe := range validationErrs {
		field := fe.StructField()
		tag := fe.Tag()

		// Try "Field.Tag" first (most specific), then just "Field".
		if msg, ok := validationMessages[field+"."+tag]; ok {
			return msg
		}
		if msg, ok := validationMessages[field]; ok {
			return msg
		}
	}

	return ""
}

// validationMessages maps struct field names (optionally with ".tag") to
// human-readable error messages. Add new entries when adding validate tags
// to payload structs.
var validationMessages = map[string]string{
	// RegisterRequest
	"Email.required":    "Email address is required",
	"Email.email":       "Please provide a valid email address",
	"Password.required": "Password is required",
	"Password.min":      "Password must be at least 8 characters",

	// LoginRequest
	"LoginEmail.required":    "Email address is required",
	"LoginPassword.required": "Password is required",

	// ForgotPasswordRequest
	"NewPassword.required": "New password is required",
	"NewPassword.min":      "New password must be at least 8 characters",

	// UpdatePasswordRequest
	"CurrentPassword.required": "Current password is required",

	// RefreshTokenRequest
	"RefreshToken.required": "Refresh token is required",

	// ShortenRequest
	"OriginalURL.required": "Original URL is required",
	"OriginalURL.url":      "Please provide a valid URL",
}
