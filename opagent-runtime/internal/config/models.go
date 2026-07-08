package config

import (
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func GetModelConfig(modelKey string) (*op.ModelConfig, error) {
	if modelKey == "" {
		return nil, fmt.Errorf("modelKey is required")
	}
	modelKey = strings.TrimSpace(modelKey)
	model := cache.Get[op.ModelConfig](modelKey, cache.PrefixDefault)
	if model == nil {
		warmModelCacheFromUserConfig()
		model = cache.Get[op.ModelConfig](modelKey, cache.PrefixDefault)
	}
	if model == nil {
		if _, err := ReloadLocalUserConfig(); err == nil {
			warmModelCacheFromUserConfig()
			model = cache.Get[op.ModelConfig](modelKey, cache.PrefixDefault)
		}
	}
	if model == nil {
		return nil, fmt.Errorf("modelKey not found: %s", modelKey)
	}
	return model, nil
}

func warmModelCacheFromUserConfig() {
	userCfg := GetUserConfig()
	if userCfg == nil || len(userCfg.Models) == 0 {
		return
	}
	cacheModelConfigs(userCfg.Models)
}

func SyncModelCache(models []op.ModelConfig) {
	cacheModelConfigs(models)
}

func cacheModelConfigs(models []op.ModelConfig) {
	for i := range models {
		model := models[i]
		if key := strings.TrimSpace(model.Key); key != "" {
			cache.Set(key, cache.PrefixDefault, &model, cache.NoExpiration)
		}
	}
}
