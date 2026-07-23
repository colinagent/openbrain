package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	modelcache "github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func TestLoadLocalUserConfig_PreservesReasoningLevels(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}

	modelsJSON := `{
  "version": 4,
  "defaultModelKey": "openai:custom-gpt",
  "providers": {
    "opagent": {
      "models": [
        {
          "id": "auto",
          "enabled": true,
          "api": "openai-completions",
          "reasoning": false
        }
      ]
    },
    "openai": {
      "api": "openai-completions",
      "baseUrl": "https://example.com/v1",
      "apiKey": "secret",
      "models": [
        {
          "id": "custom-gpt",
          "label": "Custom GPT",
          "enabled": true,
          "reasoning": true,
          "reasoningLevels": [" minimal ", "low", "medium", "HIGH", "xhigh", "low", ""]
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 1 {
		t.Fatalf("len(userCfg.Models) = %d, want 1", len(userCfg.Models))
	}
	if got := userCfg.Models[0].ID; got != "custom-gpt" {
		t.Fatalf("ID = %q, want custom-gpt", got)
	}
	if got := userCfg.Models[0].Name; got != "custom-gpt" {
		t.Fatalf("Name = %q, want custom-gpt", got)
	}

	got := userCfg.Models[0].ReasoningLevels
	want := []string{"minimal", "low", "medium", "high", "xhigh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReasoningLevels = %#v, want %#v", got, want)
	}
	if got := userCfg.Models[0].ReasoningControl; got != "level" {
		t.Fatalf("ReasoningControl = %q, want level", got)
	}
}

func TestLoadLocalUserConfig_InfersToggleReasoningControlWithoutLevels(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}

	modelsJSON := `{
  "version": 4,
  "defaultModelKey": "openai:kimi-4.6",
  "providers": {
    "openai": {
      "api": "openai-completions",
      "baseUrl": "https://example.com/v1",
      "apiKey": "secret",
      "models": [
        {
          "id": "kimi-4.6",
          "enabled": true,
          "reasoning": true
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 1 {
		t.Fatalf("len(userCfg.Models) = %d, want 1", len(userCfg.Models))
	}
	if got := userCfg.Models[0].ReasoningControl; got != "toggle" {
		t.Fatalf("ReasoningControl = %q, want toggle", got)
	}
}

func TestLoadLocalUserConfig_CustomModelUsesIDAsRuntimeName(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}

	modelsJSON := `{
  "version": 4,
  "defaultModelKey": "anthropic:claude-opus-4-6",
  "providers": {
    "opagent": {
      "models": [
        {
          "id": "auto",
          "enabled": true,
          "api": "openai-completions",
          "reasoning": false
        }
      ]
    },
    "anthropic": {
      "api": "anthropic-messages",
      "baseUrl": "https://api.anthropic.com/v1",
      "apiKey": "secret",
      "models": [
        {
          "id": "claude-opus-4-6",
          "label": "ltp-claude",
          "enabled": true
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 1 {
		t.Fatalf("len(userCfg.Models) = %d, want 1", len(userCfg.Models))
	}
	model := userCfg.Models[0]
	if model.ID != "claude-opus-4-6" {
		t.Fatalf("ID = %q, want claude-opus-4-6", model.ID)
	}
	if model.Name != "claude-opus-4-6" {
		t.Fatalf("Name = %q, want claude-opus-4-6", model.Name)
	}
}

func TestGetUserConfig_ClonesModelReasoningLevels(t *testing.T) {
	previous := GetUserConfig()
	t.Cleanup(func() {
		setUserConfig(previous)
	})

	setUserConfig(&op.UserConfig{
		Models: []op.ModelConfig{{
			ID:               "gpt-5.4",
			Name:             "gpt-5.4",
			Provider:         "openai",
			APIKey:           "secret",
			BaseURL:          "https://example.com/v1",
			Reasoning:        true,
			ReasoningControl: "level",
			ReasoningLevels:  []string{"minimal", "xhigh"},
			ServiceTiers:     []string{"priority"},
		}},
	})

	cfg := GetUserConfig()
	if cfg == nil || len(cfg.Models) != 1 {
		t.Fatalf("GetUserConfig() = %#v, want one model", cfg)
	}

	cfg.Models[0].ReasoningLevels[0] = "mutated"
	cfg.Models[0].ReasoningControl = "toggle"
	cfg.Models[0].ServiceTiers[0] = "flex"
	fresh := GetUserConfig()
	if got := fresh.Models[0].ReasoningLevels[0]; got != "minimal" {
		t.Fatalf("stored ReasoningLevels[0] = %q, want minimal", got)
	}
	if got := fresh.Models[0].ReasoningControl; got != "level" {
		t.Fatalf("stored ReasoningControl = %q, want level", got)
	}
	if got := fresh.Models[0].ServiceTiers[0]; got != "priority" {
		t.Fatalf("stored ServiceTiers[0] = %q, want priority", got)
	}
}

func TestGetModelConfig_UsesModelKeyOnly(t *testing.T) {
	previous := GetUserConfig()
	t.Cleanup(func() {
		setUserConfig(previous)
	})

	modelID := "gpt-5.4-runtime-id"
	setUserConfig(&op.UserConfig{
		Models: []op.ModelConfig{{
			Key:      "openai:gpt-5.4-runtime-id",
			ID:       modelID,
			Name:     modelID,
			Provider: "openai",
			API:      "openai-responses",
			APIKey:   "secret",
			BaseURL:  "https://example.com/v1",
			Enabled:  true,
			Source:   "provider",
		}},
	})

	warmModelCacheFromUserConfig()

	modelKey := "openai:gpt-5.4-runtime-id"
	model, err := GetModelConfig(modelKey)
	if err != nil {
		t.Fatalf("GetModelConfig(%q): %v", modelKey, err)
	}
	if model.ID != modelID {
		t.Fatalf("ID = %q, want %q", model.ID, modelID)
	}
	if model.Name != modelID {
		t.Fatalf("Name = %q, want %q", model.Name, modelID)
	}
	if _, err := GetModelConfig(modelID); err == nil {
		t.Fatal("GetModelConfig(provider-facing id) = nil error, want modelKey-only lookup")
	}
}

func TestGetModelConfig_PrefersStableKeyWhenIDsCollide(t *testing.T) {
	previous := GetUserConfig()
	t.Cleanup(func() {
		setUserConfig(previous)
	})

	setUserConfig(&op.UserConfig{
		Models: []op.ModelConfig{
			{
				Key:      "opagent:gpt-5.4",
				ID:       "gpt-5.4",
				Name:     "gpt-5.4",
				Provider: "opagent-ai-gateway",
				API:      "openai-responses",
				APIKey:   "gateway-token",
				BaseURL:  "https://ai-gateway.openbrain.work/v1",
				Enabled:  true,
				Source:   "opagent",
			},
			{
				Key:      "openai:gpt-5.4",
				ID:       "gpt-5.4",
				Name:     "gpt-5.4",
				Provider: "openai",
				API:      "openai-responses",
				APIKey:   "custom-token",
				BaseURL:  "https://api.openai.com/v1",
				Enabled:  true,
				Source:   "provider",
			},
		},
	})

	modelcache.Flush()
	warmModelCacheFromUserConfig()

	opagentModel, err := GetModelConfig("opagent:gpt-5.4")
	if err != nil {
		t.Fatalf("GetModelConfig(opagent:gpt-5.4): %v", err)
	}
	if opagentModel.Provider != "opagent-ai-gateway" {
		t.Fatalf("opagent provider = %q, want opagent-ai-gateway", opagentModel.Provider)
	}

	providerModel, err := GetModelConfig("openai:gpt-5.4")
	if err != nil {
		t.Fatalf("GetModelConfig(openai:gpt-5.4): %v", err)
	}
	if providerModel.Provider != "openai" {
		t.Fatalf("provider = %q, want openai", providerModel.Provider)
	}

	if _, err := GetModelConfig("gpt-5.4"); err == nil {
		t.Fatal("GetModelConfig(gpt-5.4) = nil error, want ambiguity to block bare id lookup")
	}
}

func TestLoadLocalUserConfig_ExpandsOpagentModelToOpagentAIGateway(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	authJSON := `{
  "version": 1,
  "baseUrl": "https://www.openbrain.io",
  "gateway": "https://gateway.openbrain.work",
  "aiGateway": "https://ai-gateway.openbrain.work",
  "token": "session-token",
  "uid": "user-test",
  "key": "user-test",
  "activeOrgID": "org-acme",
  "activeOrgName": "Acme"
}`
	modelsJSON := `{
  "version": 4,
  "defaultModelKey": "opagent:gpt-5",
  "providers": {
    "opagent": {
      "models": [
        {
          "id": "auto",
          "enabled": true,
          "api": "openai-completions",
          "reasoning": false
        },
        {
          "id": "gpt-5",
          "enabled": true,
          "api": "openai-responses"
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "auth.json"), []byte(tenantBoundAuthFixture(authJSON)), 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 2 {
		t.Fatalf("len(userCfg.Models) = %d, want 2", len(userCfg.Models))
	}
	model := userCfg.Models[1]
	if model.Name != "gpt-5" {
		t.Fatalf("Name = %q, want gpt-5", model.Name)
	}
	if model.ID != "gpt-5" {
		t.Fatalf("ID = %q, want gpt-5", model.ID)
	}
	if model.Provider != "opagent-ai-gateway" {
		t.Fatalf("Provider = %q, want opagent-ai-gateway", model.Provider)
	}
	if model.BaseURL != "https://ai-gateway.openbrain.work/v1" {
		t.Fatalf("BaseURL = %q, want gateway llm url", model.BaseURL)
	}
	if model.APIKey != "session-token" {
		t.Fatalf("APIKey = %q, want session-token", model.APIKey)
	}
	if got := model.Headers["X-Org-ID"]; got != "" {
		t.Fatalf("default opagent model X-Org-ID header = %q, want empty", got)
	}
}

func TestLoadLocalUserConfig_ExpandsOrgModelToOpagentAIGatewayWithOrgHeader(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	authJSON := `{
  "version": 1,
  "baseUrl": "https://www.openbrain.io",
  "gateway": "https://gateway.openbrain.work",
  "aiGateway": "https://ai-gateway.openbrain.work",
  "token": "session-token",
  "uid": "user-test"
}`
	modelsJSON := `{
  "version": 4,
  "defaultModelKey": "org-acme:gpt-5",
  "providers": {
    "org-acme": {
      "label": "Acme",
      "models": [
        {
          "id": "gpt-5",
          "enabled": true,
          "api": "openai-responses"
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "auth.json"), []byte(tenantBoundAuthFixture(authJSON)), 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 1 {
		t.Fatalf("len(userCfg.Models) = %d, want 1", len(userCfg.Models))
	}
	model := userCfg.Models[0]
	if model.Key != "org-acme:gpt-5" {
		t.Fatalf("Key = %q, want org-acme:gpt-5", model.Key)
	}
	if model.Provider != "opagent-ai-gateway" {
		t.Fatalf("Provider = %q, want opagent-ai-gateway", model.Provider)
	}
	if model.Source != "org-acme" {
		t.Fatalf("Source = %q, want org-acme", model.Source)
	}
	if model.BaseURL != "https://ai-gateway.openbrain.work/v1" {
		t.Fatalf("BaseURL = %q, want gateway llm url", model.BaseURL)
	}
	if model.APIKey != "session-token" {
		t.Fatalf("APIKey = %q, want session-token", model.APIKey)
	}
	if model.Headers["X-Org-ID"] != "org-acme" {
		t.Fatalf("X-Org-ID header = %q, want org-acme", model.Headers["X-Org-ID"])
	}
}

func TestLoadLocalUserConfig_ExpandsManagedOrgModelWithoutOrgPrefix(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	authJSON := `{
  "version": 1,
  "baseUrl": "https://www.openbrain.io",
  "aiGateway": "https://www.openbrain.io/ai",
  "token": "session-token",
  "uid": "user-test"
}`
	modelsJSON := `{
  "version": 5,
  "defaultModelKey": "lt:openai/gpt-5.5",
  "providers": {
    "lt": {
      "label": "lt",
      "managed": true,
      "models": [
        {
          "id": "openai/gpt-5.5",
          "label": "GPT-5.5",
          "enabled": true,
          "api": "openai-completions",
          "reasoning": true,
          "reasoningLevels": ["low", "medium", "high", "xhigh"]
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "auth.json"), []byte(tenantBoundAuthFixture(authJSON)), 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 1 {
		t.Fatalf("len(userCfg.Models) = %d, want 1", len(userCfg.Models))
	}
	model := userCfg.Models[0]
	if userCfg.DefaultModelKey != "lt:gpt-5.5" {
		t.Fatalf("DefaultModelKey = %q, want lt:gpt-5.5", userCfg.DefaultModelKey)
	}
	if model.Key != "lt:gpt-5.5" {
		t.Fatalf("Key = %q, want lt:gpt-5.5", model.Key)
	}
	if model.ID != "gpt-5.5" {
		t.Fatalf("ID = %q, want gpt-5.5", model.ID)
	}
	if model.Name != "gpt-5.5" {
		t.Fatalf("Name = %q, want gpt-5.5", model.Name)
	}
	if model.Provider != "opagent-ai-gateway" {
		t.Fatalf("Provider = %q, want opagent-ai-gateway", model.Provider)
	}
	if model.Source != "lt" {
		t.Fatalf("Source = %q, want lt", model.Source)
	}
	if model.BaseURL != "https://www.openbrain.io/ai/v1" {
		t.Fatalf("BaseURL = %q, want https://www.openbrain.io/ai/v1", model.BaseURL)
	}
	if model.APIKey != "session-token" {
		t.Fatalf("APIKey = %q, want session-token", model.APIKey)
	}
	if model.Headers["X-Org-ID"] != "lt" {
		t.Fatalf("X-Org-ID header = %q, want lt", model.Headers["X-Org-ID"])
	}
}

func TestLoadLocalUserConfig_KeepsClaudeOpagentModelOnGatewayWhenNativeEnvPresent(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	authJSON := `{
  "version": 1,
  "baseUrl": "https://www.openbrain.io",
  "gateway": "https://gateway.openbrain.work",
  "aiGateway": "https://ai-gateway.openbrain.work",
  "token": "session-token",
  "uid": "user-test",
  "key": "user-test"
}`
	modelsJSON := `{
  "version": 4,
  "defaultModelKey": "opagent:claude-opus-4-6",
  "providers": {
    "opagent": {
      "models": [
        {
          "id": "auto",
          "enabled": true,
          "api": "openai-completions",
          "reasoning": false
        },
        {
          "id": "claude-opus-4-6",
          "enabled": true,
          "api": "openai-completions",
          "reasoning": true,
          "reasoningLevels": ["low","medium","high","max"]
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "auth.json"), []byte(tenantBoundAuthFixture(authJSON)), 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
	t.Setenv("ANTHROPIC_BASE_URL", "https://native-anthropic.example/v1")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "anthropic-token")

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 2 {
		t.Fatalf("len(userCfg.Models) = %d, want 2", len(userCfg.Models))
	}
	model := userCfg.Models[1]
	if model.Name != "claude-opus-4-6" {
		t.Fatalf("Name = %q, want claude-opus-4-6", model.Name)
	}
	if model.ID != "claude-opus-4-6" {
		t.Fatalf("ID = %q, want claude-opus-4-6", model.ID)
	}
	if model.API != "openai-completions" {
		t.Fatalf("API = %q, want openai-completions", model.API)
	}
	if model.Provider != "opagent-ai-gateway" {
		t.Fatalf("Provider = %q, want opagent-ai-gateway", model.Provider)
	}
	if model.BaseURL != "https://ai-gateway.openbrain.work/v1" {
		t.Fatalf("BaseURL = %q, want gateway llm url", model.BaseURL)
	}
	if model.APIKey != "session-token" {
		t.Fatalf("APIKey = %q, want session-token", model.APIKey)
	}
}

func TestLoadLocalUserConfig_MigratesLegacySourceBasedSchema(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	authJSON := `{
  "version": 1,
  "aiGateway": "https://ai-gateway.openbrain.work",
  "token": "session-token",
  "uid": "user-test"
}`
	modelsJSON := `{
  "version": 2,
  "defaultModelKey": "gateway:gpt-5.4",
  "models": [
    {
      "key": "auto",
      "id": "auto",
      "enabled": true,
      "source": "gateway",
      "api": "openai-completions"
    },
    {
      "key": "gateway:gpt-5.4",
      "id": "gpt-5.4",
      "enabled": true,
      "source": "gateway",
      "api": "openai-responses"
    },
    {
      "key": "custom:gpt-5.4",
      "id": "gpt-5.4",
      "enabled": true,
      "source": "custom",
      "provider": "openai",
      "api": "openai-responses",
      "baseUrl": "https://api.openai.com/v1",
      "apiKey": "custom-token"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(userDir, "auth.json"), []byte(tenantBoundAuthFixture(authJSON)), 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 3 {
		t.Fatalf("len(userCfg.Models) = %d, want 3", len(userCfg.Models))
	}
	if userCfg.Models[1].Key != "opagent:gpt-5.4" {
		t.Fatalf("migrated opagent ID = %q, want opagent:gpt-5.4", userCfg.Models[1].Key)
	}
	if userCfg.Models[2].Key != "openai:gpt-5.4" {
		t.Fatalf("migrated provider key = %q, want openai:gpt-5.4", userCfg.Models[2].Key)
	}
}

func TestLoadLocalUserConfig_MigratesVersion3ProviderKeySchemaToProviders(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	authJSON := `{
  "version": 1,
  "aiGateway": "https://ai-gateway.openbrain.work",
  "token": "session-token",
  "uid": "user-test"
}`
	modelsJSON := `{
  "version": 3,
  "defaultModelKey": "openai:gpt-5-mini",
  "models": [
    {
      "key": "auto",
      "id": "auto",
      "enabled": true,
      "provider": "opagent",
      "api": "openai-completions"
    },
    {
      "key": "openai:gpt-5.4",
      "id": "gpt-5.4",
      "enabled": true,
      "provider": "openai",
      "api": "openai-responses",
      "baseUrl": "https://api.openai.com/v1",
      "apiKey": "openai-key"
    },
    {
      "key": "openai:gpt-5-mini",
      "id": "gpt-5-mini",
      "enabled": true,
      "provider": "openai",
      "api": "openai-responses",
      "baseUrl": "https://api.openai.com/v1",
      "apiKey": "openai-key"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(userDir, "auth.json"), []byte(tenantBoundAuthFixture(authJSON)), 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 3 {
		t.Fatalf("len(userCfg.Models) = %d, want 3", len(userCfg.Models))
	}
	if userCfg.Models[1].Key != "openai:gpt-5.4" {
		t.Fatalf("migrated provider key = %q, want openai:gpt-5.4", userCfg.Models[1].Key)
	}
	if userCfg.Models[2].Key != "openai:gpt-5-mini" {
		t.Fatalf("migrated provider key = %q, want openai:gpt-5-mini", userCfg.Models[2].Key)
	}
}

func TestLoadLocalUserConfig_MigratesVersion3ProviderKeySchemaWithPerModelEndpointOverrides(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	modelsJSON := `{
  "version": 3,
  "defaultModelKey": "openai:gpt-5.4",
  "models": [
    {
      "key": "auto",
      "id": "auto",
      "enabled": true,
      "provider": "opagent",
      "api": "openai-completions"
    },
    {
      "key": "openai:gpt-5.4",
      "id": "gpt-5.4",
      "enabled": true,
      "provider": "openai",
      "api": "openai-responses",
      "baseUrl": "https://api.openai.com/v1",
      "apiKey": "key-a"
    },
    {
      "key": "openai:gpt-5-mini",
      "id": "gpt-5-mini",
      "enabled": true,
      "provider": "openai",
      "api": "openai-responses",
      "baseUrl": "https://proxy.example.com/v1",
      "apiKey": "key-a"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 2 {
		t.Fatalf("len(userCfg.Models) = %d, want 2", len(userCfg.Models))
	}
	baseURLs := []string{userCfg.Models[0].BaseURL, userCfg.Models[1].BaseURL}
	sort.Strings(baseURLs)
	if !reflect.DeepEqual(baseURLs, []string{"https://api.openai.com/v1", "https://proxy.example.com/v1"}) {
		t.Fatalf("baseURLs = %#v, want migrated per-model endpoints", baseURLs)
	}
}

func TestLoadLocalUserConfig_AllowsVersion4CustomProviderWithPerModelOverrides(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}

	modelsJSON := `{
  "version": 4,
  "defaultModelKey": "codemirror:gpt-5.4",
  "providers": {
    "opagent": {
      "models": [
        {
          "id": "auto",
          "enabled": true,
          "api": "openai-completions",
          "reasoning": false
        }
      ]
    },
    "codemirror": {
      "label": "codemirror",
      "apiKey": "secret",
      "models": [
        {
          "id": "gpt-5.4",
          "enabled": true,
          "api": "openai-responses",
          "baseUrl": "https://api.aicodemirror.com/api/codex/backend-api/codex",
          "reasoning": true
        },
        {
          "id": "claude-opus-4-6",
          "enabled": true,
          "api": "anthropic-messages",
          "baseUrl": "https://api.aicodemirror.com/api/claudecode",
          "reasoning": true
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 2 {
		t.Fatalf("len(userCfg.Models) = %d, want 2", len(userCfg.Models))
	}
	var codemirrorModels []op.ModelConfig
	for _, model := range userCfg.Models {
		if model.Source == "provider" && model.Provider == "codemirror" {
			codemirrorModels = append(codemirrorModels, model)
		}
	}
	if len(codemirrorModels) != 2 {
		t.Fatalf("len(codemirrorModels) = %d, want 2", len(codemirrorModels))
	}
	baseURLs := []string{codemirrorModels[0].BaseURL, codemirrorModels[1].BaseURL}
	sort.Strings(baseURLs)
	if !reflect.DeepEqual(baseURLs, []string{
		"https://api.aicodemirror.com/api/claudecode",
		"https://api.aicodemirror.com/api/codex/backend-api/codex",
	}) {
		t.Fatalf("baseURLs = %#v, want codemirror model-specific endpoints", baseURLs)
	}
}

func TestLoadLocalUserConfig_RejectsDefaultModelKeyWhenOnlyLegacyAutoExists(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}

	modelsJSON := `{
  "version": 4,
  "defaultModelKey": "auto",
  "providers": {
    "openai": {
      "api": "openai-responses",
      "baseUrl": "https://api.openai.com/v1",
      "apiKey": "secret",
      "models": [
        {
          "id": "auto",
          "enabled": true,
          "reasoning": false
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	_, err := loadLocalUserConfig(baseDir)
	if err == nil {
		t.Fatalf("loadLocalUserConfig() expected error")
	}
	if !strings.Contains(err.Error(), "defaultModelKey auto not found") {
		t.Fatalf("error = %v, want missing default model validation", err)
	}
}

func TestLoadLocalUserConfig_RejectsDisabledDefaultChatModel(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}

	modelsJSON := `{
  "version": 5,
  "defaultModelKey": "openai:gpt-5.4",
  "strategies": {
    "auto": {
      "defaultChatModelID": "openai:gpt-5.4"
    }
  },
  "providers": {
    "openai": {
      "api": "openai-responses",
      "baseUrl": "https://api.openai.com/v1",
      "apiKey": "secret",
      "models": [
        {
          "id": "gpt-5.4",
          "enabled": false,
          "reasoning": true
        },
        {
          "id": "gpt-5-mini",
          "enabled": true,
          "reasoning": true
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	_, err := loadLocalUserConfig(baseDir)
	if err == nil {
		t.Fatalf("loadLocalUserConfig() expected error")
	}
	if !strings.Contains(err.Error(), "Default Chat Model openai:gpt-5.4 must reference an enabled model") {
		t.Fatalf("error = %v, want Default Chat Model enabled validation", err)
	}
}

func TestLoadLocalUserConfig_RejectsLegacyModelSchema(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}

	modelsJSON := `{
  "models": [
    {
      "id": "gpt-5",
      "enabled": true,
      "provider": "opagent",
      "api": "openai-responses"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	_, err := loadLocalUserConfig(baseDir)
	if err == nil {
		t.Fatalf("loadLocalUserConfig() expected error")
	}
	if !strings.Contains(err.Error(), "version must be 5") {
		t.Fatalf("error = %v, want version validation", err)
	}
}

func TestResolveAIGatewayBaseURL_DoesNotInferFromGateway(t *testing.T) {
	got := resolveAIGatewayBaseURL(&op.AuthConfig{
		Gateway: "https://gateway.openbrain.work",
	})
	if got != "" {
		t.Fatalf("resolveAIGatewayBaseURL inferred %q from gateway, want empty", got)
	}
}

func TestLoadLocalUserConfig_IgnoresInvalidNodesJSON(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	authJSON := `{
  "version": 1,
  "baseUrl": "https://www.openbrain.io",
  "aiGateway": "https://www.openbrain.io/ai",
  "token": "session-token",
  "uid": "user-test"
}`
	modelsJSON := `{
  "version": 5,
  "defaultModelKey": "opagent:gpt-5",
  "providers": {
    "opagent": {
      "models": [
        {
          "id": "gpt-5",
          "enabled": true,
          "api": "openai-responses"
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "auth.json"), []byte(tenantBoundAuthFixture(authJSON)), 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "nodes.json"), []byte(`{"agent-coder":{}}trailing`), 0o644); err != nil {
		t.Fatalf("write nodes.json: %v", err)
	}

	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if len(userCfg.Models) != 1 {
		t.Fatalf("len(userCfg.Models) = %d, want 1", len(userCfg.Models))
	}
	if userCfg.Nodes != nil {
		t.Fatalf("Nodes = %#v, want nil for invalid nodes.json", userCfg.Nodes)
	}
	if got := userCfg.Models[0].BaseURL; got != "https://www.openbrain.io/ai/v1" {
		t.Fatalf("BaseURL = %q, want /ai/v1", got)
	}
}
