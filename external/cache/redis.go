package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache implements the cache using Redis hash operations (HGet/HSet).
type RedisCache struct {
	client *redis.Client
}

// RedisConfig holds the configuration for connecting to Redis.
type RedisConfig struct {
	Addr       string // host:port (e.g. "localhost:6379")
	UserName   string // Redis AUTH username (empty = default)
	Password   string // Redis AUTH password (empty = no password)
	DB         int    // Redis database number (default 0)
	MaxRetries int    // Max retries for failed commands.
}

// NewRedisCache creates a new Redis-backed cache and pings the server
// to verify connectivity. An error is returned if the server is unreachable.
func NewRedisCache(cfg RedisConfig) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:       cfg.Addr,
		Username:   cfg.UserName,
		Password:   cfg.Password,
		DB:         cfg.DB,
		MaxRetries: cfg.MaxRetries,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisCache{client: client}, nil
}

// HGet retrieves a single field from a Redis hash.
// Returns the field value or an error on miss/connection failure.
func (c *RedisCache) HGet(key, field string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	val, err := c.client.HGet(ctx, key, field).Result()
	if err != nil {
		return "", err
	}
	return val, nil
}

// HSet stores a single field in a Redis hash.
func (c *RedisCache) HSet(key, field string, value any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return c.client.HSet(ctx, key, field, value).Err()
}

// HSetWithTTL stores a single field in a Redis hash and sets an expiry on the
// entire key. The TTL applies to the hash key, not individual fields.
func (c *RedisCache) HSetWithTTL(key, field string, value any, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipe := c.client.Pipeline()
	pipe.HSet(ctx, key, field, value)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// HSetFields stores multiple fields in a Redis hash using a single pipeline
// (one TCP round-trip). This is the preferred way to set multiple session
// fields atomically.
func (c *RedisCache) HSetFields(key string, fields map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipe := c.client.Pipeline()
	for field, value := range fields {
		pipe.HSet(ctx, key, field, value)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// HDel removes one or more fields from a Redis hash.
func (c *RedisCache) HDel(key string, fields ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return c.client.HDel(ctx, key, fields...).Err()
}

// Close shuts down the Redis client connection pool.
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// String returns a human-readable representation for logging.
func (c *RedisCache) String() string {
	return fmt.Sprintf("redis(%s)", c.client.Options().Addr)
}
