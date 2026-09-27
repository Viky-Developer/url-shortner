// Package cachekey defines cache key prefixes shared across internal application domains.
package cachekey

const (
	// SessionPrefix prefixes cache entries keyed by session ID.
	SessionPrefix = "session:"

	// RateLimitPrefix prefixes login rate-limit entries keyed by email address.
	RateLimitPrefix = "ratelimit:"
)
