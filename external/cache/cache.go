// Package cache defines the CacheService interface for Redis-backed caching.
// Implementations live in this package; consumers depend only on the interface.
package cache

import (
	"context"
	"time"
)

// CacheOption configures optional behavior for cache write operations.
type CacheOption func(*cacheOptions)

type cacheOptions struct {
	expiration time.Duration
}

// WithExpiration sets a TTL on the key being written.
func WithExpiration(d time.Duration) CacheOption {
	return func(o *cacheOptions) { o.expiration = d }
}

func applyOptions(opts ...CacheOption) cacheOptions {
	var o cacheOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// CacheService defines the Redis cache operations used by the application.
// Only the commands the application actually needs are exposed.
type CacheService interface {
	// Get retrieves a string value by key. Returns an error on miss.
	Get(ctx context.Context, key string) (string, error)

	// Set stores a string value by key. Use WithExpiration to set a TTL.
	Set(ctx context.Context, key, value string, opts ...CacheOption) error

	// HGet retrieves a single field from a Redis hash.
	// Returns the field value or an error on miss/connection failure.
	HGet(ctx context.Context, key, field string) (string, error)

	// HMGet retrieves multiple fields from a Redis hash in a single round-trip.
	// Returns a map of field→value; missing fields are omitted from the map.
	HMGet(ctx context.Context, key string, fields ...string) (map[string]string, error)

	// HGetAll retrieves all field-value pairs from a Redis hash.
	// Returns a map of field→value or an error on miss/connection failure.
	HGetAll(ctx context.Context, key string) (map[string]string, error)

	// HSet stores one or more field-value pairs in a Redis hash.
	// Use WithExpiration to set a TTL on the key.
	HSet(ctx context.Context, key string, fields map[string]any, opts ...CacheOption) error

	// HDel removes one or more fields from a Redis hash.
	HDel(ctx context.Context, key string, fields ...string) error

	// Del removes an entire key from Redis.
	Del(ctx context.Context, key string) error
}
