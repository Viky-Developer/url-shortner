package apperror

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrorsExist(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrNotFound", ErrNotFound, "the requested resource was not found"},
		{"ErrInvalidURL", ErrInvalidURL, "the provided URL is not valid"},
		{"ErrURLInactive", ErrURLInactive, "this URL is currently inactive"},
		{"ErrURLDeleted", ErrURLDeleted, "this URL has already been deleted"},
		{"ErrInternal", ErrInternal, "an unexpected error occurred. Please try again later"},
		{"ErrInvalidPayload", ErrInvalidPayload, "the request body is missing or invalid"},
		{"ErrConflict", ErrConflict, "the resource already exists"},
		{"ErrURLExpired", ErrURLExpired, "this URL has expired and is no longer available"},
		{"ErrBlockedDomain", ErrBlockedDomain, "the destination domain is blocked"},
		{"ErrUnauthorized", ErrUnauthorized, "unauthorized access"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatalf("%s should not be nil", tt.name)
			}
			if tt.err.Error() != tt.msg {
				t.Errorf("%s message = %q, want %q", tt.name, tt.err.Error(), tt.msg)
			}
		})
	}
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	errs := []error{
		ErrNotFound,
		ErrInvalidURL,
		ErrURLInactive,
		ErrURLDeleted,
		ErrInternal,
		ErrInvalidPayload,
		ErrConflict,
		ErrURLExpired,
		ErrBlockedDomain,
		ErrUnauthorized,
	}

	seen := make(map[error]bool)
	for _, err := range errs {
		if seen[err] {
			t.Errorf("duplicate sentinel error: %v", err)
		}
		seen[err] = true
	}
}

func TestErrorsWrapCorrectly(t *testing.T) {
	wrapped := fmt.Errorf("context: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("expected errors.Is to match the wrapped error")
	}
}
