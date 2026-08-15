// Package apperror defines the sentinel errors shared across the service and
// handler layers so that handlers can map failures to HTTP status codes.
package apperror

import "errors"

var (
	// ErrNotFound is returned when a URL does not exist or has been deleted.
	ErrNotFound = errors.New("not found")
	// ErrInvalidURL is returned when a provided URL fails validation.
	ErrInvalidURL = errors.New("invalid url")
	// ErrURLInactive is returned when a URL is inactive.
	ErrURLInactive = errors.New("url is inactive")
	// ErrURLDeleted is returned when a URL has already been deleted.
	ErrURLDeleted = errors.New("url already deleted")
	// ErrInternal is returned for unexpected internal failures.
	ErrInternal = errors.New("internal server error")
	// ErrInvalidPayload is returned when the request body is missing or
	// otherwise not a valid JSON object.
	ErrInvalidPayload = errors.New("invalid payload")
	// ErrConflict is returned when a resource already exists (e.g. duplicate
	// short code).
	ErrConflict = errors.New("already exists")
	// ErrURLExpired is returned when a URL has passed its expiration time.
	ErrURLExpired = errors.New("your url has expired")
)
