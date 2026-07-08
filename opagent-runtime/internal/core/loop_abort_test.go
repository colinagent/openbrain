package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func TestNewAgentLoopAllowsPromptAfterAssistantAbort(t *testing.T) {
	baseDir := t.TempDir()
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	resetThreadTestState(baseDir)
	cache.Flush()
	t.Cleanup(cache.Flush)
	agentID, _, agentFilePath := createTestAgent(t, baseDir, "workspace/proj")
	cache.Set("test:auto", cache.PrefixDefault, &op.ModelConfig{
		Key:      "test:auto",
		ID:       "auto",
		Name:     "Auto",
		Provider: "openai",
		API:      "openai-completions",
		BaseURL:  "https://example.com/v1",
		APIKey:   "test-key",
	}, cache.NoExpiration)

	node := &op.OpNode{
		ID:   agentID,
		Kind: string(op.NodeKindAgent),
		Cwd:  cwd,
		URI:  op.PathToURI(agentFilePath),
		Meta: &op.AgentMeta{},
	}

	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "aborted-tail.md"),
		Title:    "aborted-tail",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	if err := appendMessagesToThread(*meta, []op.Message{
		op.NewUserMessage("start"),
		{
			Role:       op.RoleAssistant,
			Content:    "Turn interrupted before completion.",
			StopReason: op.StopReasonAborted,
		},
	}); err != nil {
		t.Fatalf("appendMessagesToThread: %v", err)
	}

	loop, err := NewAgentLoop(context.Background(), node, op.Meta{
		"chatPath": result.ChatPath,
		"threadID": result.ThreadID,
		"agentID":  agentID,
		"modelKey": "test:auto",
	}, &op.TextContent{Text: "start something else"})
	if err != nil {
		t.Fatalf("NewAgentLoop: %v", err)
	}
	if loop == nil {
		t.Fatal("NewAgentLoop returned nil loop")
	}
	loop.Cancel()
}

func TestNewAgentLoopStillRejectsToolUseTailPrompt(t *testing.T) {
	baseDir := t.TempDir()
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	resetThreadTestState(baseDir)
	cache.Flush()
	t.Cleanup(cache.Flush)
	agentID, _, agentFilePath := createTestAgent(t, baseDir, "workspace/proj")
	cache.Set("test:auto", cache.PrefixDefault, &op.ModelConfig{
		Key:      "test:auto",
		ID:       "auto",
		Name:     "Auto",
		Provider: "openai",
		API:      "openai-completions",
		BaseURL:  "https://example.com/v1",
		APIKey:   "test-key",
	}, cache.NoExpiration)

	node := &op.OpNode{
		ID:   agentID,
		Kind: string(op.NodeKindAgent),
		Cwd:  cwd,
		URI:  op.PathToURI(agentFilePath),
		Meta: &op.AgentMeta{},
	}

	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "tool-tail.md"),
		Title:    "tool-tail",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	if err := appendMessagesToThread(*meta, []op.Message{
		op.NewUserMessage("start"),
		{
			Role: op.RoleAssistant,
			ToolCalls: []op.MessageToolCall{{
				ID:        "call-1",
				Name:      "read",
				Arguments: map[string]any{"path": "a.txt"},
			}},
			StopReason: op.StopReasonToolUse,
		},
	}); err != nil {
		t.Fatalf("appendMessagesToThread: %v", err)
	}

	loop, err := NewAgentLoop(context.Background(), node, op.Meta{
		"chatPath": result.ChatPath,
		"threadID": result.ThreadID,
		"agentID":  agentID,
		"modelKey": "test:auto",
	}, &op.TextContent{Text: "start something else"})
	if err == nil || loop != nil {
		t.Fatalf("NewAgentLoop() = (%v, %v), want continuation error", loop, err)
	}
	if err.Error() != "thread requires continuation before accepting a new prompt" {
		t.Fatalf("NewAgentLoop error = %q, want continuation error", err.Error())
	}
}
