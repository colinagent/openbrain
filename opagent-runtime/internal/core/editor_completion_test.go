package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func TestResolveEditorCompletionAgentFindsCompletionAgent(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)

	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{
		BaseDir: baseDir,
		HostID:  "test-host",
		Env:     op.EnvLocal,
	})

	agentPath := filepath.Join(baseDir, "agents", "completion", ".agent", "AGENT.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	if err := os.WriteFile(agentPath, []byte("---\nname: completion\n---\nPrompt.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	node := op.BuildNode("local", "test-host", op.NodeKindAgent, op.PathToURI(agentPath), op.EnvLocal, nil, op.Run{}, nil, &op.AgentMeta{
		Name: "completion",
	})
	node.Cwd = filepath.Dir(filepath.Dir(agentPath))
	cache.SetValue(node.ID, cache.PrefixNode, *node, cache.NoExpiration)

	got, err := resolveEditorCompletionAgent("")
	if err != nil {
		t.Fatalf("resolveEditorCompletionAgent(): %v", err)
	}
	if got.ID != node.ID {
		t.Fatalf("resolved key = %q, want %q", got.ID, node.ID)
	}
}

func TestLoadSimpleCompletionAgentPromptAllowsPromptOnlyAgent(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)

	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{
		BaseDir: baseDir,
		HostID:  "test-host",
		Env:     op.EnvLocal,
	})

	agentPath := filepath.Join(baseDir, "agents", "completion", ".agent", "AGENT.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	if err := os.WriteFile(agentPath, []byte("---\nname: completion\n---\nCompletion prompt.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	node := op.BuildNode("local", "test-host", op.NodeKindAgent, op.PathToURI(agentPath), op.EnvLocal, nil, op.Run{}, nil, &op.AgentMeta{
		Name: "completion",
	})
	node.Cwd = filepath.Dir(filepath.Dir(agentPath))

	prompt, err := loadSimpleCompletionAgentPrompt(context.Background(), node, op.Meta{})
	if err != nil {
		t.Fatalf("loadSimpleCompletionAgentPrompt(): %v", err)
	}
	if prompt == "" {
		t.Fatal("prompt is empty")
	}
}

func TestSanitizeEditorCompletionTextPreservesLeadingNewlineAndIndent(t *testing.T) {
	got := sanitizeEditorCompletionText("\n    return nil\n", "")
	want := "\n    return nil\n"
	if got != want {
		t.Fatalf("sanitizeEditorCompletionText() = %q, want %q", got, want)
	}
}

func TestBuildEditorCompletionUserPromptUsesDocumentBaseName(t *testing.T) {
	prompt := buildEditorCompletionUserPrompt(op.EditorCompletionRequest{
		EditorKind:   "text",
		LanguageID:   "go",
		DocumentPath: "/Users/example/code/OpAgent/main.go",
	})
	if !strings.Contains(prompt, "documentName: main.go") {
		t.Fatalf("prompt missing base name: %q", prompt)
	}
	if strings.Contains(prompt, "/Users/example/code/OpAgent") {
		t.Fatalf("prompt leaked absolute path: %q", prompt)
	}
}
