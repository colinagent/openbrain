package cache

import "time"

// GetValue returns a copy of the value stored under id/prefix.
// It accepts both T and *T in the cache (so mixed entries after migration still work).
// Second return is false if not found or type mismatch.
func GetValue[T any](key string, prefix string) (T, bool) {
	var zero T
	shard := getShardOrNil(prefix)
	if shard == nil {
		return zero, false
	}
	entry, ok := shard.Get(key)
	if !ok {
		return zero, false
	}
	if val, ok := entry.(T); ok {
		return val, true
	}
	if ptr, ok := entry.(*T); ok {
		return *ptr, true
	}
	return zero, false
}

// SetValue stores a copy of data under id/prefix (value semantics).
func SetValue[T any](key string, prefix string, data T, expiration time.Duration) {
	shard := getOrCreateShard(prefix)
	shard.Set(key, data, expiration)
}

// ListValuesByPrefix returns copies of all values in the shard with the given prefix.
// Accepts both T and *T in the cache (so mixed entries after migration still work).
func ListValuesByPrefix[T any](prefix string) []T {
	shard := getShardOrNil(prefix)
	if shard == nil {
		return nil
	}
	items := shard.Items()
	result := make([]T, 0, len(items))
	for _, item := range items {
		if val, ok := item.Object.(T); ok {
			result = append(result, val)
			continue
		}
		if ptr, ok := item.Object.(*T); ok {
			result = append(result, *ptr)
		}
	}
	return result
}
