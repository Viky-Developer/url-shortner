// Package contextutil provides shared context key types used across
// middleware and handler packages to store and retrieve values from
// request contexts.
package contextutil

// ContextKey is an unexported type for context keys defined in this package.
// This prevents collisions with keys defined in other packages.
type ContextKey string

// UserIDKey is the context key for storing the authenticated user ID.
// It is used by the auth middleware to set the value and by handlers to read it.
const UserIDKey ContextKey = "user_id"
