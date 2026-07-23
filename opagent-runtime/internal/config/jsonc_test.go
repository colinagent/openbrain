package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestReadOptionalJSONAllowsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	raw := `{
  // User identity.
  "uid": "user-jsonc",
  "username": "JSONC User",
  "avatar": "https://example.com/avatar.png // keep"
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	profile, ok, err := readOptionalJSON[op.UserProfile](path)
	if err != nil {
		t.Fatalf("readOptionalJSON(): %v", err)
	}
	if !ok {
		t.Fatal("readOptionalJSON() ok = false, want true")
	}
	if profile.UID != "user-jsonc" {
		t.Fatalf("UID = %q, want user-jsonc", profile.UID)
	}
	if profile.Avatar != "https://example.com/avatar.png // keep" {
		t.Fatalf("Avatar = %q", profile.Avatar)
	}
}

func TestLoadLocalUserConfigAllowsComments(t *testing.T) {
	baseDir := t.TempDir()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}

	authJSON := `{
  // Auth config can be hand-edited.
  "uid": "user-jsonc",
  "token": "token-value"
}`
	modelsJSON := `{
  "version": 5,
  "defaultModelKey": "openai:gpt-jsonc",
  "providers": {
    "openai": {
      "api": "openai-completions",
      "baseUrl": "https://example.com/v1",
      "apiKey": "secret",
      "models": [
        {
          /* The custom model used by the test. */
          "id": "gpt-jsonc",
          "label": "GPT JSONC",
          "enabled": true
        }
      ]
    }
  }
}`
	nodesJSON := `{
  "agent-jsonc": {
    "uri": "file:///tmp/agent/.agent/AGENT.md" // comment after field
  }
}`

	if err := os.WriteFile(filepath.Join(userDir, "auth.json"), []byte(tenantBoundAuthFixture(authJSON)), 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "nodes.json"), []byte(nodesJSON), 0o644); err != nil {
		t.Fatalf("write nodes.json: %v", err)
	}

	SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "host-jsonc", Env: op.EnvLocal})
	userCfg, err := loadLocalUserConfig(baseDir)
	if err != nil {
		t.Fatalf("loadLocalUserConfig(): %v", err)
	}
	if userCfg.Auth == nil || userCfg.Auth.UID != "user-jsonc" {
		t.Fatalf("auth = %#v, want uid user-jsonc", userCfg.Auth)
	}
	if len(userCfg.Models) != 1 || userCfg.Models[0].Key != "openai:gpt-jsonc" {
		t.Fatalf("models = %#v, want openai:gpt-jsonc", userCfg.Models)
	}
	node, ok := userCfg.Nodes["agent-jsonc"]
	if !ok {
		t.Fatalf("nodes missing agent-jsonc: %#v", userCfg.Nodes)
	}
	if node.ID != "agent-jsonc" || node.Kind != string(op.NodeKindAgent) || node.HostID != "host-jsonc" {
		t.Fatalf("node = %#v", node)
	}
}

func tenantBoundAuthFixture(raw string) string {
	raw = strings.Replace(raw, `"version": 1`, `"version": 2`, 1)
	versionField := ""
	if !strings.Contains(raw, `"version"`) {
		versionField = `
  "version": 2,`
	}
	const fields = `
  "deploymentID": "dep-test",
  "orgID": "org-test",
  "identityID": "idn-test",
  "connectionID": "conn-test",
  "authMethod": "email",
  "authTime": "2026-07-23T00:00:00Z",
  "expiresAt": "2026-07-24T00:00:00Z",`
	return strings.Replace(raw, "{", "{"+versionField+fields, 1)
}
