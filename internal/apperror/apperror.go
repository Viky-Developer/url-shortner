// Package apperror defines the sentinel errors shared across the service and
// handler layers so that handlers can map failures to HTTP status codes.
package apperror

import "errors"

var (
	// ErrNotFound is returned when a URL does not exist or has been deleted.
	ErrNotFound = errors.New("the requested resource was not found")
	// ErrInvalidURL is returned when a provided URL fails validation.
	ErrInvalidURL = errors.New("the provided URL is not valid")
	// ErrURLInactive is returned when a URL is inactive.
	ErrURLInactive = errors.New("this URL is currently inactive")
	// ErrURLDeleted is returned when a URL has already been deleted.
	ErrURLDeleted = errors.New("this URL has already been deleted")
	// ErrInternal is returned for unexpected internal failures.
	ErrInternal = errors.New("an unexpected error occurred. Please try again later")
	// ErrInvalidPayload is returned when the request body is missing or
	// otherwise not a valid JSON object.
	ErrInvalidPayload = errors.New("the request body is missing or invalid")
	// ErrConflict is returned when a resource already exists (e.g. duplicate
	// short code).
	ErrConflict = errors.New("the resource already exists")
	// ErrURLExpired is returned when a URL has passed its expiration time.
	ErrURLExpired = errors.New("this URL has expired and is no longer available")
	// ErrBlockedDomain is returned when the destination domain is blocked.
	ErrBlockedDomain = errors.New("the destination domain is blocked")
	// ErrUnauthorized is returned when the user is not authenticated.
	ErrUnauthorized = errors.New("unauthorized access")
	// ErrSessionExpired is returned when a session has passed its expiry time.
	ErrSessionExpired = errors.New("session expired, please try to login again")
)
