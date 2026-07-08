package run

import (
	"context"
	"log/slog"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

func loadModels(ctx context.Context) {
	userCfg := config.GetUserConfig()
	if userCfg == nil {
		return
	}
	if len(userCfg.Models) == 0 {
		slog.Info("no user models to load")
		return
	}

	configured := make([]op.ModelConfig, 0, len(userCfg.Models))
	for i := range userCfg.Models {
		model := userCfg.Models[i]
		id := strings.TrimSpace(model.ID)
		if id == "" {
			slog.Warn("skip invalid model entry: empty id")
			continue
		}
		configured = append(configured, model)
	}
	config.SyncModelCache(configured)
	for i := range configured {
		model := configured[i]
		slog.Info("model upserted", "id", model.ID, "key", strings.TrimSpace(model.Key), "modelSource", strings.ToLower(strings.TrimSpace(model.Source)), "baseURLEmpty", strings.TrimSpace(model.BaseURL) == "", "name", model.Name)
	}
}
