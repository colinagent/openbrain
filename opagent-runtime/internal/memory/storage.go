package memory

import (
	"context"
	"log/slog"

	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/core"
)

type StorageType string

const (
	StorageTypeMongo StorageType = "mongodb"
)

func NewStorage(ctx context.Context) core.Storage {
	var storageType StorageType
	if cfg := config.GetConfig(); cfg != nil {
		storageType = StorageType(cfg.Memory.Storage)
	}
	storage := NewStorageWithType(ctx, storageType)
	core.SetDefaultStorage(storage)
	return storage
}

func NewStorageWithType(ctx context.Context, storageType StorageType) core.Storage {
	_ = ctx
	if storageType != "" {
		slog.Warn("runtime memory storage is disabled", "type", storageType)
	}
	return nil
}

// Backward compatibility adapter: expose memory.GetCache[T] for v2 handler usage
//
//	func GetCache[T any](id string) (*T, error) {
//		return go_cache.GetCacheTyped[T](id)
//	}
// type LayeredMemory struct {
// 	storage host.Storage
// }

// // NewLayeredMemory 创建分层存储实例
// func NewLayeredMemory(storage host.Storage) *LayeredMemory {
// 	return &LayeredMemory{
// 		storage: storage,
// 	}
// }
