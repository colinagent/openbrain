package factory

import (
	"context"
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/objectstore"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/objectstore/fsstore"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/objectstore/mongostore"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/objectstore/s3store"
)

func NewDefault(ctx context.Context) (objectstore.Store, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	switch strings.ToLower(strings.TrimSpace(cfg.ObjectStore.Type)) {
	case "", "fs":
		return fsstore.New(cfg.ObjectStore.FS.BaseDir), nil
	case "s3":
		return s3store.New(ctx, &cfg.ObjectStore.S3)
	case "mongodb":
		return mongostore.New(ctx, cfg)
	default:
		return nil, fmt.Errorf("unknown objectStore.type: %q", cfg.ObjectStore.Type)
	}
}
