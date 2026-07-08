package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func TestResolveThreadSubmitAgentNodeRefreshesFileBackedAgent(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)

	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{
		BaseDir: baseDir,
		HostID:  "test-host",
		Env:     op.EnvLocal,
	})

	agentDir := filepath.Join(baseDir, "agents", "gbrain")
	agentPath := filepath.Join(agentDir, ".agent", "AGENT.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	manifest := "---\nid: agent-gbrain\nname: gbrain\ndescription: reloaded\nopcodes:\n  - thread/submit\n---\nPrompt.\n"
	if err := os.WriteFile(agentPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	cache.SetValue("agent-gbrain", cache.PrefixNode, op.OpNode{
		ID:      "agent-gbrain",
		Kind:    string(op.NodeKindAgent),
		URI:     op.PathToURI(agentPath),
		Cwd:     agentDir,
		UID:     op.LocalUser,
		HostID:  "test-host",
		OpCodes: nil,
		Meta: &op.AgentMeta{
			Name:   "gbrain",
			Skills: []string{"skill-stale"},
		},
	}, cache.NoExpiration)

	node, err := resolveThreadSubmitAgentNode("agent-gbrain")
	if err != nil {
		t.Fatalf("resolveThreadSubmitAgentNode(): %v", err)
	}
	meta, ok := node.Meta.(*op.AgentMeta)
	if !ok || meta == nil {
		t.Fatalf("resolved meta = %#v, want *op.AgentMeta", node.Meta)
	}
	if len(meta.Skills) != 0 {
		t.Fatalf("resolved skills = %#v, want empty after reload", meta.Skills)
	}
	if len(node.OpCodes) != 1 || node.OpCodes[0] != op.OpThreadSubmit {
		t.Fatalf("resolved opcodes = %#v, want thread/submit from disk manifest", node.OpCodes)
	}

	cached, ok := cache.GetValue[op.OpNode]("agent-gbrain", cache.PrefixNode)
	if !ok {
		t.Fatal("reloaded agent missing from cache")
	}
	cachedMeta, ok := cached.Meta.(*op.AgentMeta)
	if !ok || cachedMeta == nil {
		t.Fatalf("cached meta = %#v, want *op.AgentMeta", cached.Meta)
	}
	if len(cachedMeta.Skills) != 0 {
		t.Fatalf("cached skills = %#v, want empty after reload", cachedMeta.Skills)
	}
}
