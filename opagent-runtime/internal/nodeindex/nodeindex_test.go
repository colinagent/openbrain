package nodeindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestAssignPersistsDeterministicIDByURI(t *testing.T) {
	baseDir := t.TempDir()
	agentPath := filepath.Join(baseDir, "agents", "demo", ".agent", "AGENT.md")
	agentURI := op.PathToURI(agentPath)

	idx, err := Open(baseDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	first := op.BuildNode("user-test", "host-test", op.NodeKindAgent, agentURI, op.EnvLocal, nil, op.Run{}, nil, &op.AgentMeta{Name: "demo"})
	first.Cwd = filepath.Dir(filepath.Dir(agentPath))
	if err := idx.Assign(first); err != nil {
		t.Fatalf("Assign first: %v", err)
	}
	if !strings.HasPrefix(first.ID, "agent-") {
		t.Fatalf("first.ID = %q, want agent-*", first.ID)
	}

	reopened, err := Open(baseDir)
	if err != nil {
		t.Fatalf("Open reopened: %v", err)
	}
	second := op.BuildNode("another-user", "host-test", op.NodeKindAgent, agentURI, op.EnvLocal, nil, op.Run{}, nil, &op.AgentMeta{Name: "demo"})
	if err := reopened.Assign(second); err != nil {
		t.Fatalf("Assign second: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second.ID = %q, want stable %q", second.ID, first.ID)
	}

	raw, err := os.ReadFile(filepath.Join(baseDir, "index", "nodes.json"))
	if err != nil {
		t.Fatalf("ReadFile nodes.json: %v", err)
	}
	var stored File
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("Unmarshal nodes.json: %v", err)
	}
	if len(stored.Nodes) != 1 {
		t.Fatalf("stored nodes len = %d, want 1", len(stored.Nodes))
	}
	if stored.Nodes[0].ID != first.ID || stored.Nodes[0].URI != agentURI {
		t.Fatalf("stored node = %+v", stored.Nodes[0])
	}
}

func TestAssignRejectsSameIDForDifferentURI(t *testing.T) {
	baseDir := t.TempDir()
	idx, err := Open(baseDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	first := &op.OpNode{
		ID:   "agent-shared",
		Kind: string(op.NodeKindAgent),
		URI:  op.PathToURI(filepath.Join(baseDir, "agents", "one", ".agent", "AGENT.md")),
	}
	if err := idx.Assign(first); err != nil {
		t.Fatalf("Assign first: %v", err)
	}
	second := &op.OpNode{
		ID:   "agent-shared",
		Kind: string(op.NodeKindAgent),
		URI:  op.PathToURI(filepath.Join(baseDir, "agents", "two", ".agent", "AGENT.md")),
	}
	err = idx.Assign(second)
	if err == nil {
		t.Fatal("Assign second = nil, want conflict")
	}
	if !strings.Contains(err.Error(), "id conflict") || !strings.Contains(err.Error(), "agent-shared") {
		t.Fatalf("Assign second error = %v, want id conflict for agent-shared", err)
	}
}
