package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache implements CacheService using Redis hash operations.
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

// NewRedisCache creates a Redis-backed cache and pings the server
// to verify connectivity. Returns an error if unreachable.
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
func (c *RedisCache) HGet(ctx context.Context, key, field string) (string, error) {
	val, err := c.client.HGet(ctx, key, field).Result()
	if err != nil {
		return "", err
	}
	return val, nil
}

// HMGet retrieves multiple fields from a Redis hash in a single round-trip.
func (c *RedisCache) HMGet(ctx context.Context, key string, fields ...string) (map[string]string, error) {
	vals, err := c.client.HMGet(ctx, key, fields...).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(fields))
	for i, v := range vals {
		if s, ok := v.(string); ok {
			out[fields[i]] = s
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("redis: nil")
	}
	return out, nil
}

// HGetAll retrieves all field-value pairs from a Redis hash.
func (c *RedisCache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	val, err := c.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(val) == 0 {
		return nil, fmt.Errorf("redis: nil")
	}
	return val, nil
}

// HSet stores one or more field-value pairs in a Redis hash.
// Use WithExpiration to set a TTL on the entire key.
func (c *RedisCache) HSet(ctx context.Context, key string, fields map[string]any, opts ...CacheOption) error {
	o := applyOptions(opts...)

	pipe := c.client.Pipeline()
	for field, value := range fields {
		pipe.HSet(ctx, key, field, value)
	}
	if o.expiration > 0 {
		pipe.Expire(ctx, key, o.expiration)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// HDel removes one or more fields from a Redis hash.
func (c *RedisCache) HDel(ctx context.Context, key string, fields ...string) error {
	return c.client.HDel(ctx, key, fields...).Err()
}

// Del removes an entire key from Redis.
func (c *RedisCache) Del(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// Close shuts down the Redis client connection pool.
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// String returns a human-readable label for logging.
func (c *RedisCache) String() string {
	return fmt.Sprintf("redis(%s)", c.client.Options().Addr)
}
