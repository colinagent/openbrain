package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type failingScriptedProvider struct {
	responses []*ai.ProviderResponse
	errs      []error
	seen      [][]op.Message
}

func (p *failingScriptedProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-completions")
}

func (p *failingScriptedProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return nil, errors.New("unexpected CompleteCanonical call")
}

func (p *failingScriptedProvider) StreamCanonical(_ context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req == nil {
		return nil, errors.New("req is nil")
	}
	index := len(p.seen)
	msgs, err := opMessagesFromCanonicalWithoutTest(req.Context.Messages)
	if err != nil {
		return nil, err
	}
	p.seen = append(p.seen, msgs)
	if index < len(p.errs) && p.errs[index] != nil {
		return nil, p.errs[index]
	}
	if len(p.errs) > 0 && p.errs[len(p.errs)-1] != nil && len(p.responses) == 0 {
		return nil, p.errs[len(p.errs)-1]
	}
	if index >= len(p.responses) || p.responses[index] == nil {
		return nil, errors.New("unexpected StreamCanonical call")
	}
	stream := ai.NewProviderEventStream(1)
	resp := p.responses[index]
	go func() {
		_ = stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalDone, Response: resp})
		stream.Close()
	}()
	return stream, nil
}

func newPersistentLoop(
	threadID string,
	chatPath string,
	agentID string,
	cwd string,
	userMessage op.Message,
	provider ai.CanonicalProvider,
	toolSpecs map[string]*op.ToolSpec,
) *AgentLoop {
	return &AgentLoop{
		Ctx:              context.Background(),
		Cancel:           func() {},
		Agent:            &Agent{AgentID: agentID, ToolSpecs: toolSpecs},
		Meta:             op.Meta{"threadID": threadID, "chatPath": chatPath, "agentID": agentID},
		ThreadID:         threadID,
		ChatPath:         chatPath,
		Workdir:          cwd,
		Model:            &ModelClient{config: &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000}, Canonical: provider},
		userMessage:      userMessage,
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{userMessage}),
	}
}

func opMessagesFromCanonicalWithoutTest(messages []ai.ConversationMessage) ([]op.Message, error) {
	out := make([]op.Message, 0, len(messages))
	for _, msg := range messages {
		converted, err := ai.OpMessageFromCanonical(msg)
		if err != nil {
			return nil, err
		}
		if converted.Role == "" {
			continue
		}
		out = append(out, converted)
	}
	return out, nil
}

func opMessagesFromCanonicalForTest(t *testing.T, messages []ai.ConversationMessage) []op.Message {
	t.Helper()
	out, err := opMessagesFromCanonicalWithoutTest(messages)
	if err != nil {
		t.Fatalf("OpMessageFromCanonical(): %v", err)
	}
	return out
}

func TestAgentLoopRun_PersistsUserMessageBeforeFirstModelCall(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "persist-user.md"),
		Title:    "persist-user",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}

	provider := &failingScriptedProvider{
		errs: []error{errors.New("upstream 503"), errors.New("upstream 503")},
	}
	loop := newPersistentLoop(
		result.ThreadID,
		result.ChatPath,
		agentID,
		cwd,
		op.NewUserMessage("first question"),
		provider,
		map[string]*op.ToolSpec{},
	)

	if _, err := loop.run(); err == nil {
		t.Fatal("loop.run() succeeded, want stream failure")
	}

	sessionCtx, err := loadThreadContext(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("loadThreadContext: %v", err)
	}
	messages := opMessagesFromCanonicalForTest(t, sessionCtx.canonicalMessages)
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	if messages[0].Role != op.RoleUser || messages[0].Content != "first question" {
		t.Fatalf("persisted first message = %+v, want original user prompt", messages[0])
	}
	if messages[1].Role != op.RoleAssistant {
		t.Fatalf("messages[1].Role = %q, want %q", messages[1].Role, op.RoleAssistant)
	}
	if messages[1].StopReason != op.StopReasonError {
		t.Fatalf("messages[1].StopReason = %q, want %q", messages[1].StopReason, op.StopReasonError)
	}
	if messages[1].Content != "upstream 503" {
		t.Fatalf("messages[1].Content = %q, want %q", messages[1].Content, "upstream 503")
	}
}

func TestAgentLoopRun_PersistsAssistantToolCallAndToolResultBeforeFollowUpFailure(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")
	connectTestToolServer(t, "sys-server", func(input testToolInput) string {
		return "read: " + input.Path
	})

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "persist-tool.md"),
		Title:    "persist-tool",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}

	provider := &failingScriptedProvider{
		responses: []*ai.ProviderResponse{
			ai.ProviderResponseFromOpMessage(op.Message{
				Role: op.RoleAssistant,
				ToolCalls: []op.MessageToolCall{{
					ID:        "call-1",
					Name:      "read_file",
					Arguments: map[string]any{"path": "/etc/passwd"},
					Type:      "function",
				}},
			}, ai.Usage{}, ai.StopReasonToolUse),
		},
		errs: []error{
			nil,
			errors.New("follow-up assistant failed"),
			errors.New("follow-up assistant failed"),
		},
	}
	loop := newPersistentLoop(
		result.ThreadID,
		result.ChatPath,
		agentID,
		cwd,
		op.NewUserMessage("cat /etc/passwd"),
		provider,
		map[string]*op.ToolSpec{
			"read_file": {ServerID: "sys-server", Name: "read_file"},
		},
	)

	if _, err := loop.run(); err == nil {
		t.Fatal("loop.run() succeeded, want second model call failure")
	}

	sessionCtx, err := loadThreadContext(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("loadThreadContext: %v", err)
	}
	messages := opMessagesFromCanonicalForTest(t, sessionCtx.canonicalMessages)
	if len(messages) != 4 {
		t.Fatalf("len(messages) = %d, want 4", len(messages))
	}
	if messages[0].Role != op.RoleUser || messages[0].Content != "cat /etc/passwd" {
		t.Fatalf("messages[0] = %+v, want original user prompt", messages[0])
	}
	if messages[1].Role != op.RoleAssistant || len(messages[1].ToolCalls) != 1 {
		t.Fatalf("messages[1] = %+v, want persisted assistant tool call", messages[1])
	}
	if messages[1].ToolCalls[0].ID != "call-1" {
		t.Fatalf("assistant tool call id = %q, want call-1", messages[1].ToolCalls[0].ID)
	}
	if messages[2].Role != op.RoleTool || messages[2].ToolCallID != "call-1" {
		t.Fatalf("messages[2] = %+v, want persisted tool result", messages[2])
	}
	if messages[2].Content != "read: /etc/passwd" {
		t.Fatalf("tool result content = %q, want read output", messages[2].Content)
	}
	if messages[3].Role != op.RoleAssistant {
		t.Fatalf("messages[3].Role = %q, want %q", messages[3].Role, op.RoleAssistant)
	}
	if messages[3].StopReason != op.StopReasonError {
		t.Fatalf("messages[3].StopReason = %q, want %q", messages[3].StopReason, op.StopReasonError)
	}
	if messages[3].Content != "follow-up assistant failed" {
		t.Fatalf("messages[3].Content = %q, want %q", messages[3].Content, "follow-up assistant failed")
	}
}

func TestNewAgentLoop_ReplaysFailedUserTurnOnNextPrompt(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	cache.Set("test:auto", cache.PrefixDefault, &op.ModelConfig{
		Key:      "test:auto",
		ID:       "auto",
		Name:     "auto",
		Provider: "openai",
		API:      "openai-completions",
		APIKey:   "test-key",
		BaseURL:  "https://example.com/v1",
	}, 0)
	agentID, agentDir, agentFile := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "failed-turn-replay.md"),
		Title:    "failed-turn-replay",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}

	firstProvider := &failingScriptedProvider{
		errs: []error{errors.New("read canonical websocket event: websocket: close 1006 (abnormal closure): unexpected EOF")},
	}
	firstLoop := newPersistentLoop(
		result.ThreadID,
		result.ChatPath,
		agentID,
		cwd,
		op.NewUserMessage("帮我做成html页面我要导出成pdf"),
		firstProvider,
		map[string]*op.ToolSpec{},
	)
	if _, err := firstLoop.run(); err == nil {
		t.Fatal("first loop.run() succeeded, want stream failure")
	}

	secondProvider := &failingScriptedProvider{
		responses: []*ai.ProviderResponse{
			ai.ProviderResponseFromOpMessage(op.NewAssistantMessage("later"), ai.Usage{}, ai.StopReasonStop),
		},
	}
	secondNode := &op.OpNode{
		ID:      agentID,
		Kind:    string(op.NodeKindAgent),
		Cwd:     agentDir,
		URI:     op.PathToURI(agentFile),
		OpCodes: []op.OpCode{op.OpPromptGet, op.OpThreadSubmit},
		Meta: &op.AgentMeta{
			Name: "demo",
		},
	}
	secondLoop, err := NewAgentLoop(context.Background(), secondNode, op.Meta{
		"threadID": result.ThreadID,
		"agentID":  agentID,
		"chatPath": result.ChatPath,
		"path":     result.ChatPath,
		"cwd":      cwd,
		"modelKey": "test:auto",
	}, &op.TextContent{Text: "好了吗"})
	if err != nil {
		t.Fatalf("NewAgentLoop(): %v", err)
	}
	secondLoop.Model = &ModelClient{
		config:    &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000},
		Canonical: secondProvider,
	}
	if _, err := secondLoop.run(); err != nil {
		t.Fatalf("second loop.run(): %v", err)
	}

	if len(secondProvider.seen) != 1 {
		t.Fatalf("second provider seen = %d, want 1", len(secondProvider.seen))
	}
	seen := secondProvider.seen[0]
	if len(seen) != 2 {
		t.Fatalf("second provider context len = %d, want 2", len(seen))
	}
	if seen[0].Role != op.RoleUser || seen[0].Content != "帮我做成html页面我要导出成pdf" {
		t.Fatalf("seen[0] = %+v, want original failed user prompt", seen[0])
	}
	if seen[1].Role != op.RoleUser || seen[1].Content != "好了吗" {
		t.Fatalf("seen[1] = %+v, want follow-up user prompt", seen[1])
	}
}

func TestAppendMessagesToSession_PreservesThreadHeaderAgentID(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	originalAgentID, _, _ := createTestAgent(t, baseDir, "agents/original")
	latestAgentID, _, _ := createTestAgent(t, baseDir, "agents/latest")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  originalAgentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "switch-agent.md"),
		Title:    "switch-agent",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}

	if err := appendMessagesToThread(op.ThreadMeta{
		ThreadID: result.ThreadID,
		AgentID:  latestAgentID,
		CWD:      cwd,
		ChatPath: result.ChatPath,
	}, []op.Message{op.NewAssistantMessage("hello from latest agent")}); err != nil {
		t.Fatalf("appendMessagesToThread: %v", err)
	}

	meta, err := getThreadMeta(result.ThreadID, "")
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	if meta.AgentID != originalAgentID {
		t.Fatalf("meta.AgentID = %q, want legacy header agent %q", meta.AgentID, originalAgentID)
	}
}
