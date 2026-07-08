package cache

import (
	"errors"
	"time"

	gocache "github.com/patrickmn/go-cache"
)

var (
	cache                 *gocache.Cache
	ErrCacheAlreadyExists = errors.New("cache already exists")
)

type Options struct {
	Expiration      time.Duration
	CleanupInterval time.Duration
}

const (
	DefaultExpiration      = 12 * time.Hour
	DefaultCleanupInterval = 10 * time.Minute
	NoExpiration           = gocache.NoExpiration
	ShortExpiration        = 5 * time.Minute
)

func init() {
	cache = gocache.New(DefaultExpiration, DefaultCleanupInterval)
}

func Get[T any](key string) (T, bool) {
	var zero T
	value, ok := cache.Get(key)
	if !ok {
		return zero, false
	}
	return value.(T), true
}

func Set(key string, data any, expiration time.Duration) {
	cache.Set(key, data, expiration)
}

func Delete(key string) {
	cache.Delete(key)
}

func Flush() {
	cache.Flush()
}
