package core

import (
	"context"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

var (
	defaultStorage Storage
)

// SetDefaultStorage allows external initialization of the default storage instance
func SetDefaultStorage(storage Storage) {
	defaultStorage = storage
}

// GetStorage returns the default storage instance
func GetStorage() Storage {
	return defaultStorage
}

// Storage defines the underlying storage interface.
// Agent/skill/tool records have been moved to nodestore (in-memory cache).
// This interface retains only thread, user, and model persistence.
type Storage interface {
	// thread
	// GetThreadStorage(ctx context.Context, threadID string) (*op.ThreadStorage, error)
	// UpsertThreadStorage(ctx context.Context, thread *op.ThreadStorage) error
	// DeleteThreadStorage(ctx context.Context, threadID string) error
	// QueryThreadStorage(ctx context.Context, query *op.ThreadStorageQuery) ([]*op.ThreadStorage, int64, error)

	// user task
	GetUserTask(ctx context.Context, taskID string) (*op.UserTask, error)
	ListUserTasks(ctx context.Context, uid string) ([]*op.UserTask, error)
	UpsertUserTask(ctx context.Context, task *op.UserTask) error
	BindThreadIDToUserTaskID(ctx context.Context, taskID, threadID string) error
	DeleteThreadIDFromUserTask(ctx context.Context, taskID, threadID string) error
	DeleteUserTask(ctx context.Context, taskID string) error

	// user settings
	UpsertUserSettings(ctx context.Context, settings *op.UserSettings) error
	GetUserSettings(ctx context.Context, uid string) (*op.UserSettings, error)

	// ListUIDs returns distinct UIDs that exist in storage.
	ListUIDs(ctx context.Context) ([]string, error)

	// models
	// ListModels(ctx context.Context) ([]*op.ModelConfig, error)
	// ListModelIDs(ctx context.Context) ([]string, error)
	// UpsertModel(ctx context.Context, model *op.ModelConfig) error
	// GetModel(ctx context.Context, id string) (*op.ModelConfig, error)
	// DeleteModel(ctx context.Context, id string) error
}
