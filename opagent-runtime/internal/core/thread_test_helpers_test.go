package core

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

func resetThreadTestState(baseDir string) {
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir})
	defaultThreadStore = &threadStore{
		byThread:  make(map[string]*threadRecord),
		threadMux: make(map[string]*sync.Mutex),
	}
}

func createTestAgent(t *testing.T, baseDir string, relPath string) (string, string, string) {
	t.Helper()

	agentBaseDir := filepath.Join(baseDir, filepath.FromSlash(relPath))
	agentMetaDir := filepath.Join(agentBaseDir, ".agent")
	if err := os.MkdirAll(agentMetaDir, 0o755); err != nil {
		t.Fatalf("mkdir agent meta dir: %v", err)
	}
	agentFilePath := filepath.Join(agentMetaDir, "AGENT.md")
	if err := os.WriteFile(agentFilePath, []byte("name: Test Agent\n"), 0o644); err != nil {
		t.Fatalf("write agent config: %v", err)
	}
	agentID := op.BuildNodeID(op.LocalUser, "test-host", op.NodeKindAgent, op.PathToURI(agentFilePath), op.EnvLocal)
	return agentID, agentBaseDir, agentFilePath
}
