package cache

import (
	"errors"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

var ErrCacheAlreadyExists = errors.New("cache already exists")

type Options struct {
	Expiration      time.Duration
	CleanupInterval time.Duration
}

const (
	DefaultExpiration      = 12 * time.Hour
	DefaultCleanupInterval = 10 * time.Minute
	NoExpiration           = cache.NoExpiration
	ShortExpiration        = 5 * time.Minute
	OneHourExpiration      = 1 * time.Hour

	//prefixes
	PrefixDefault    = "default:"
	PrefixConnection = "connection:"
	PrefixNode       = "node:"
	PrefixThread     = "thread:"
)

var (
	mu          sync.RWMutex
	shards      = make(map[string]*cache.Cache)
	initialized bool
	defaultOpts = &Options{
		Expiration:      DefaultExpiration,
		CleanupInterval: DefaultCleanupInterval,
	}
)

func NewCache(opts *Options) {
	mu.Lock()
	defer mu.Unlock()

	if initialized {
		return
	}
	initialized = true

	if opts == nil {
		return
	}
	defaultOpts = &Options{
		Expiration:      opts.Expiration,
		CleanupInterval: opts.CleanupInterval,
	}
}

// func normalizeKey(key string, prefix string) string {
// 	key = strings.TrimSpace(key)
// 	return strings.TrimPrefix(key, prefix)
// }

func getShardOrNil(prefix string) *cache.Cache {
	mu.RLock()
	shard := shards[prefix]
	mu.RUnlock()
	return shard
}

func getOrCreateShard(prefix string) *cache.Cache {
	mu.RLock()
	shard, ok := shards[prefix]
	mu.RUnlock()
	if ok {
		return shard
	}

	mu.Lock()
	defer mu.Unlock()
	if shard, ok = shards[prefix]; ok {
		return shard
	}
	shard = cache.New(defaultOpts.Expiration, defaultOpts.CleanupInterval)
	shards[prefix] = shard
	return shard
}

func Get[T any](key string, prefix string) *T {
	shard := getShardOrNil(prefix)
	if shard == nil {
		return nil
	}
	entry, ok := shard.Get(key)
	if !ok {
		return nil
	}
	val, ok := entry.(*T)
	if !ok {
		return nil
	}
	return val
}

func Set(key string, prefix string, data any, expiration time.Duration) {
	shard := getOrCreateShard(prefix)
	shard.Set(key, data, expiration)
}

func Add(key string, prefix string, data any, expiration time.Duration) error {
	shard := getOrCreateShard(prefix)
	if err := shard.Add(key, data, expiration); err != nil {
		return ErrCacheAlreadyExists
	}
	return nil
}

func Delete(key string, prefix string) {
	shard := getShardOrNil(prefix)
	if shard == nil {
		return
	}
	shard.Delete(key)
}

func Flush() {
	mu.RLock()
	snapshot := make([]*cache.Cache, 0, len(shards))
	for _, shard := range shards {
		snapshot = append(snapshot, shard)
	}
	mu.RUnlock()

	for _, shard := range snapshot {
		shard.Flush()
	}
}

func ListByPrefix[T any](prefix string) []*T {
	shard := getShardOrNil(prefix)
	if shard == nil {
		return nil
	}
	items := shard.Items()
	result := make([]*T, 0, len(items))
	for _, item := range items {
		val, ok := item.Object.(*T)
		if !ok {
			continue
		}
		result = append(result, val)
	}
	return result
}
