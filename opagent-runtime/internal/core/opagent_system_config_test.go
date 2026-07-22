package core

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

func TestConfigSystemGetReturnsRuntimeDefaultWorkspace(t *testing.T) {
	previous := config.GetSystem()
	t.Cleanup(func() { config.SetSystem(previous) })

	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "host-test", Env: op.EnvLocal})

	result, err := ConfigSystemGetHandler(context.Background(), &op.OpNodeRequest{
		Params: &op.OpNodeParams{OpCode: op.ConfigSystemGet},
	})
	if err != nil {
		t.Fatalf("ConfigSystemGetHandler(): %v", err)
	}
	content, ok := result.Content.(*op.JsonContent)
	if !ok || content == nil {
		t.Fatalf("content = %T, want *op.JsonContent", result.Content)
	}
	var got op.SystemConfigResult
	if err := json.Unmarshal(content.Raw, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got.BaseDir != baseDir {
		t.Fatalf("baseDir = %q, want %q", got.BaseDir, baseDir)
	}
	wantWorkspace := filepath.Join(baseDir, "workspace")
	if got.DefaultWorkspace != wantWorkspace {
		t.Fatalf("defaultWorkspace = %q, want %q", got.DefaultWorkspace, wantWorkspace)
	}
}
