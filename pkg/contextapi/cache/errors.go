package cache

import "errors"

// Cache errors
var (
	// ErrCacheMiss is returned when a cache entry is not found
	ErrCacheMiss = errors.New("cache miss")

	// ErrCacheUnavailable is returned when cache is unavailable
	ErrCacheUnavailable = errors.New("cache unavailable")
)
