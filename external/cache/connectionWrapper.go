package cache

import (
	"sync"
)

type ConnectionWrapper struct {
	once   sync.Once
	cache  *RedisCache
	err    error
	config RedisConfig
}

// GetRedisCache returns a shared RedisCache instance.
// If config is provided, it will be used to create the connection.
// Returns error if connection fails.
func (w *ConnectionWrapper) GetRedisCache(cfg RedisConfig) (*RedisCache, error) {
	w.once.Do(func() {
		w.cache, w.err = NewRedisCache(cfg)
		w.config = cfg
	})
	return w.cache, w.err
}
