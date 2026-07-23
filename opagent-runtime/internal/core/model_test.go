package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	modelcache "github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func tenantBoundAuthFixture(raw string) string {
	raw = strings.Replace(raw, `"version": 1`, `"version": 2`, 1)
	versionField := ""
	if !strings.Contains(raw, `"version"`) {
		versionField = `"version":2,`
	}
	const fields = `"deploymentID":"dep-test","orgID":"org-test","identityID":"idn-test","connectionID":"conn-test","authMethod":"email","authTime":"2026-07-23T00:00:00Z","expiresAt":"2026-07-24T00:00:00Z",`
	return strings.Replace(raw, "{", "{"+versionField+fields, 1)
}

func TestNewModelClient_OpenAIResponsesUsesNativeCanonicalProvider(t *testing.T) {
	prevSystem := config.GetSystem()
	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{
		BaseDir: baseDir,
		HostID:  "test-host",
		Env:     op.EnvLocal,
	})
	modelcache.Flush()
	t.Cleanup(func() {
		modelcache.Flush()
		config.SetSystem(prevSystem)
		if prevSystem != nil && prevSystem.BaseDir != "" {
			_, _ = config.LoadLocalUserProfile()
		}
	})

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
  "version": 4,
  "defaultModelKey": "opagent:gpt-5.4",
  "providers": {
    "opagent": {
      "models": [
        {
          "id": "gpt-5.4",
          "label": "gpt-5.4",
          "enabled": true,
          "api": "openai-responses",
          "reasoning": true,
          "reasoningLevels": ["minimal", "low", "medium", "high", "xhigh"]
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
	if _, err := config.LoadLocalUserProfile(); err != nil {
		t.Fatalf("LoadLocalUserProfile(): %v", err)
	}

	model, err := NewModelClient(context.Background(), "opagent:gpt-5.4", nil)
	if err != nil {
		t.Fatalf("NewModelClient(): %v", err)
	}
	if model.Canonical == nil {
		t.Fatal("model.Canonical is nil")
	}
	if model.Responses == nil {
		t.Fatal("model.Responses is nil")
	}
}

func TestNewModelClient_RequiresExplicitModelKey(t *testing.T) {
	baseDir := setupModelClientTestBase(t)
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	modelsJSON := `{
  "version": 5,
  "defaultModelKey": "local-one:first",
  "providers": {
    "local-one": {
      "api": "openai-responses",
      "baseUrl": "https://api.example.test/v1",
      "apiKey": "test-key",
      "models": [
        { "id": "first", "enabled": true }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
	if _, err := config.LoadLocalUserProfile(); err != nil {
		t.Fatalf("LoadLocalUserProfile(): %v", err)
	}

	_, err := NewModelClient(context.Background(), "", nil)
	if err == nil {
		t.Fatal("NewModelClient() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "modelKey is required") {
		t.Fatalf("error = %q, want modelKey is required", err.Error())
	}
}

func TestNewModelClient_UsesMetaModelKeyWhenModelIDIsEmpty(t *testing.T) {
	baseDir := setupModelClientTestBase(t)
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	modelsJSON := `{
  "version": 5,
  "defaultModelKey": "local-one:first",
  "providers": {
    "local-one": {
      "api": "openai-responses",
      "baseUrl": "https://api.example.test/v1",
      "apiKey": "test-key",
      "models": [
        { "id": "first", "enabled": true }
      ]
    },
    "local-two": {
      "api": "openai-responses",
      "baseUrl": "https://api2.example.test/v1",
      "apiKey": "test-key",
      "models": [
        { "id": "second", "enabled": true }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
	if _, err := config.LoadLocalUserProfile(); err != nil {
		t.Fatalf("LoadLocalUserProfile(): %v", err)
	}

	model, err := NewModelClient(context.Background(), "", op.Meta{"modelKey": "local-two:second"})
	if err != nil {
		t.Fatalf("NewModelClient(): %v", err)
	}
	if got := model.config.Key; got != "local-two:second" {
		t.Fatalf("Key = %q, want local-two:second", got)
	}
}

func TestNewModelClient_IgnoresLegacyMetaModel(t *testing.T) {
	baseDir := setupModelClientTestBase(t)
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}
	modelsJSON := `{
  "version": 5,
  "defaultModelKey": "local-one:first",
  "providers": {
    "local-one": {
      "api": "openai-responses",
      "baseUrl": "https://api.example.test/v1",
      "apiKey": "test-key",
      "models": [
        { "id": "first", "enabled": true }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
	if _, err := config.LoadLocalUserProfile(); err != nil {
		t.Fatalf("LoadLocalUserProfile(): %v", err)
	}

	_, err := NewModelClient(context.Background(), "", op.Meta{"model": "local-one:first"})
	if err == nil {
		t.Fatal("NewModelClient() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "modelKey is required") {
		t.Fatalf("error = %q, want modelKey is required", err.Error())
	}
}

func TestNewModelClient_ReusesSharedSingleModelProvider(t *testing.T) {
	prevSystem := config.GetSystem()
	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{
		BaseDir: baseDir,
		HostID:  "test-host",
		Env:     op.EnvLocal,
	})
	modelcache.Flush()
	t.Cleanup(func() {
		modelcache.Flush()
		config.SetSystem(prevSystem)
		if prevSystem != nil && prevSystem.BaseDir != "" {
			_, _ = config.LoadLocalUserProfile()
		}
	})

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
  "version": 4,
  "defaultModelKey": "opagent:gpt-5.4",
  "providers": {
    "opagent": {
      "models": [
        {
          "id": "gpt-5.4",
          "label": "gpt-5.4",
          "enabled": true,
          "api": "openai-responses",
          "reasoning": true,
          "reasoningLevels": ["minimal", "low", "medium", "high", "xhigh"]
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
	if _, err := config.LoadLocalUserProfile(); err != nil {
		t.Fatalf("LoadLocalUserProfile(): %v", err)
	}

	model1, err := NewModelClient(context.Background(), "gpt-5.4", op.Meta{"modelKey": "opagent:gpt-5.4"})
	if err != nil {
		t.Fatalf("NewModelClient() first: %v", err)
	}
	model2, err := NewModelClient(context.Background(), "gpt-5.4", op.Meta{"modelKey": "opagent:gpt-5.4"})
	if err != nil {
		t.Fatalf("NewModelClient() second: %v", err)
	}
	if model1.Canonical != model2.Canonical {
		t.Fatalf("Canonical pointers differ: %p != %p", model1.Canonical, model2.Canonical)
	}
	if model1.Responses != model2.Responses {
		t.Fatalf("Responses pointers differ: %p != %p", model1.Responses, model2.Responses)
	}
}

func TestNewModelClient_PrefersModelKeyOverProviderFacingModelID(t *testing.T) {
	prevSystem := config.GetSystem()
	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{
		BaseDir: baseDir,
		HostID:  "test-host",
		Env:     op.EnvLocal,
	})
	modelcache.Flush()
	t.Cleanup(func() {
		modelcache.Flush()
		config.SetSystem(prevSystem)
		if prevSystem != nil && prevSystem.BaseDir != "" {
			_, _ = config.LoadLocalUserProfile()
		}
	})

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
  "version": 4,
  "defaultModelKey": "opagent:gpt-5.4",
  "providers": {
    "opagent": {
      "models": [
        {
          "id": "gpt-5.4",
          "enabled": true,
          "api": "openai-responses"
        }
      ]
    },
    "openai": {
      "api": "openai-responses",
      "baseUrl": "https://api.openai.com/v1",
      "apiKey": "custom-token",
      "models": [
        {
          "id": "gpt-5.4",
          "enabled": true
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
	if _, err := config.LoadLocalUserProfile(); err != nil {
		t.Fatalf("LoadLocalUserProfile(): %v", err)
	}

	model, err := NewModelClient(context.Background(), "gpt-5.4", op.Meta{
		"model":    "gpt-5.4",
		"modelKey": "opagent:gpt-5.4",
	})
	if err != nil {
		t.Fatalf("NewModelClient(): %v", err)
	}
	if model.config == nil {
		t.Fatal("model.config is nil")
	}
	if got := model.config.Provider; got != "opagent-ai-gateway" {
		t.Fatalf("Provider = %q, want opagent-ai-gateway", got)
	}
	if got := model.config.Key; got != "opagent:gpt-5.4" {
		t.Fatalf("Key = %q, want opagent:gpt-5.4", got)
	}
}

func setupModelClientTestBase(t *testing.T) string {
	t.Helper()
	prevSystem := config.GetSystem()
	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{
		BaseDir: baseDir,
		HostID:  "test-host",
		Env:     op.EnvLocal,
	})
	modelcache.Flush()
	t.Cleanup(func() {
		modelcache.Flush()
		config.SetSystem(prevSystem)
		if prevSystem != nil && prevSystem.BaseDir != "" {
			_, _ = config.LoadLocalUserProfile()
		}
	})
	return baseDir
}
