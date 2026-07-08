package core

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	airuntime "github.com/colinagent/openbrain/opagent-runtime/packages/ai/runtime"
)

type ModelClient struct {
	config    *op.ModelConfig
	Canonical ai.CanonicalProvider
	Responses ai.ResponsesProvider
	Ctx       context.Context
}

func validateModelConfig(cfg *op.ModelConfig) error {
	if cfg == nil {
		return fmt.Errorf("model config is nil")
	}
	if cfg.ID == "" {
		return fmt.Errorf("model config: model id is required")
	}
	if cfg.Name == "" {
		return fmt.Errorf("model config: model name is required")
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		return fmt.Errorf("model config: provider is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("model config: api key is required")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return fmt.Errorf("model config: baseURL is required")
	}
	return nil
}

func NewModelClient(ctx context.Context, modelID string, meta op.Meta) (*ModelClient, error) {
	if meta != nil {
		if modelKey, ok := meta["modelKey"].(string); ok && strings.TrimSpace(modelKey) != "" {
			modelID = strings.TrimSpace(modelKey)
		}
	}

	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, fmt.Errorf("modelKey is required")
	}

	cfg, err := config.GetModelConfig(modelID)
	if err != nil {
		return nil, err
	}

	if err := validateModelConfig(cfg); err != nil {
		return nil, err
	}

	provid, err := airuntime.NewSingleModelProvider(cfg)
	if err != nil {
		return nil, err
	}

	model := &ModelClient{config: cfg, Ctx: ctx}
	canonical, canonicalMode, err := resolveCanonicalProvider(provid)
	if err != nil {
		return nil, err
	}
	model.Canonical = canonical
	if strings.TrimSpace(cfg.API) == "openai-responses" {
		slog.Info("model canonical provider resolved",
			"modelID", strings.TrimSpace(cfg.ID),
			"api", strings.TrimSpace(cfg.API),
			"mode", canonicalMode,
			"providerType", fmt.Sprintf("%T", provid),
		)
	}
	if responsesProvider, ok := provid.(ai.ResponsesProvider); ok {
		model.Responses = responsesProvider
	}
	return model, nil
}

func (m *ModelClient) canonicalProvider() ai.CanonicalProvider {
	if m == nil {
		return nil
	}
	if m.Canonical != nil {
		return m.Canonical
	}
	return nil
}

func (m *ModelClient) responsesProvider() (ai.ResponsesProvider, bool) {
	if m == nil {
		return nil, false
	}
	if m.Responses != nil {
		return m.Responses, true
	}
	return nil, false
}

func resolveCanonicalProvider(prov ai.CanonicalProvider) (ai.CanonicalProvider, string, error) {
	if prov == nil {
		return nil, "", fmt.Errorf("provider is nil")
	}
	return prov, "native", nil
}
