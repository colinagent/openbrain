package core

import (
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

func TestApplyLoopThreadMeta_PopulatesWorkingDirectoryWithoutSessionThinkingLevel(t *testing.T) {
	meta, thinkingLevel := applyLoopThreadMeta(op.Meta{}, op.ThreadMeta{
		CWD: "/tmp/workspace",
	})

	if got := metaString(meta, "cwd"); got != "/tmp/workspace" {
		t.Fatalf("meta cwd = %q, want /tmp/workspace", got)
	}
	if got := metaString(meta, "workdir"); got != "" {
		t.Fatalf("meta workdir = %q, want empty", got)
	}
	if got := metaString(meta, "modelKey"); got != "" {
		t.Fatalf("meta modelKey = %q, want empty", got)
	}
	if got := metaString(meta, "model"); got != "" {
		t.Fatalf("meta model = %q, want empty", got)
	}
	if thinkingLevel != "" {
		t.Fatalf("thinkingLevel = %q, want empty", thinkingLevel)
	}
}

func TestApplyLoopThreadMeta_PreservesExplicitRequestThinkingLevel(t *testing.T) {
	meta, thinkingLevel := applyLoopThreadMeta(op.Meta{
		"modelKey":      "openai:gpt-5.4",
		"model":         "gpt-5.4",
		"thinkingLevel": "high",
	}, op.ThreadMeta{
		CWD: "/tmp/workspace",
	})

	if got := metaString(meta, "modelKey"); got != "openai:gpt-5.4" {
		t.Fatalf("meta modelKey = %q, want openai:gpt-5.4", got)
	}
	if got := metaString(meta, "model"); got != "gpt-5.4" {
		t.Fatalf("meta model = %q, want gpt-5.4", got)
	}
	if thinkingLevel != "high" {
		t.Fatalf("thinkingLevel = %q, want high", thinkingLevel)
	}
}

func TestApplyAgentModelMeta_PreservesExplicitRequestModelKey(t *testing.T) {
	config.SyncModelCache([]op.ModelConfig{{
		Key:      "openai:gpt-5-mini",
		ID:       "gpt-5-mini",
		Name:     "gpt-5-mini",
		Provider: "openai",
		API:      "openai-responses",
		APIKey:   "test-key",
		BaseURL:  "https://api.example.test/v1",
	}})

	meta, err := applyAgentModelMeta(op.Meta{
		"modelKey": "openai:gpt-5-mini",
		"model":    "legacy-model",
	}, &op.OpNode{
		Meta: &op.AgentMeta{Model: "local-openai:gpt-5.4"},
	})
	if err != nil {
		t.Fatalf("applyAgentModelMeta(): %v", err)
	}
	if got := metaString(meta, "modelKey"); got != "openai:gpt-5-mini" {
		t.Fatalf("meta modelKey = %q, want openai:gpt-5-mini", got)
	}
	if got := metaString(meta, "model"); got != "" {
		t.Fatalf("legacy model = %q, want empty", got)
	}
}

func TestApplyAgentModelMeta_RequiresExplicitModelKey(t *testing.T) {
	_, err := applyAgentModelMeta(op.Meta{}, &op.OpNode{
		Meta: &op.AgentMeta{Model: "local-openai:gpt-5.4"},
	})
	if err == nil {
		t.Fatal("applyAgentModelMeta() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "modelKey is required") {
		t.Fatalf("error = %q, want modelKey is required", err.Error())
	}
}

func TestApplyAgentModelMeta_RejectsUnavailableExplicitModel(t *testing.T) {
	_, err := applyAgentModelMeta(op.Meta{"modelKey": "missing:gpt-5.4"}, &op.OpNode{
		ID:   "agent-coder",
		Meta: &op.AgentMeta{},
	})
	if err == nil {
		t.Fatal("applyAgentModelMeta() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "missing:gpt-5.4") {
		t.Fatalf("error = %q, want missing model key", err.Error())
	}
}

func TestResolveAgentTaskModelMeta_ChildFrontmatterModelWins(t *testing.T) {
	config.SyncModelCache([]op.ModelConfig{{
		Key:      "local-openai:gpt-5.4",
		ID:       "gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "local-openai",
		API:      "openai-responses",
		APIKey:   "test-key",
		BaseURL:  "https://api.example.test/v1",
	}})

	meta, err := resolveAgentTaskModelMeta(op.Meta{"agentID": "agent-child"}, &op.OpNode{
		ID:   "agent-child",
		Meta: &op.AgentMeta{Model: "local-openai:gpt-5.4"},
	}, op.Meta{"modelKey": "cloud:gpt-5.5"})
	if err != nil {
		t.Fatalf("resolveAgentTaskModelMeta(): %v", err)
	}
	if got := metaString(meta, "modelKey"); got != "local-openai:gpt-5.4" {
		t.Fatalf("meta modelKey = %q, want local-openai:gpt-5.4", got)
	}
}

func TestResolveAgentTaskModelMeta_InheritsParentModelKey(t *testing.T) {
	meta, err := resolveAgentTaskModelMeta(op.Meta{"agentID": "agent-child"}, &op.OpNode{
		ID:   "agent-child",
		Meta: &op.AgentMeta{},
	}, op.Meta{"modelKey": "cloud:gpt-5.5"})
	if err != nil {
		t.Fatalf("resolveAgentTaskModelMeta(): %v", err)
	}
	if got := metaString(meta, "modelKey"); got != "cloud:gpt-5.5" {
		t.Fatalf("meta modelKey = %q, want cloud:gpt-5.5", got)
	}
}

func TestResolveAgentTaskModelMeta_UnavailableFrontmatterModelPublishesMessage(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)

	meta := op.Meta{
		"threadID": "thread-agent-model",
		"agentID":  "agent-child",
	}
	if _, err := createThreadWithID(op.ThreadCreateParams{
		AgentID: "agent-child",
		CWD:     baseDir,
		Title:   "agent model test",
	}, "thread-agent-model"); err != nil {
		t.Fatalf("createThreadWithID(): %v", err)
	}
	_, err := resolveAgentTaskModelMeta(meta, &op.OpNode{
		ID:   "agent-child",
		Meta: &op.AgentMeta{Model: "missing:gpt-5.4"},
	}, op.Meta{"modelKey": "parent:model"})
	if err == nil {
		t.Fatal("resolveAgentTaskModelMeta() succeeded, want error")
	}

	read, err := defaultMessageStore.read(op.MessageReadParams{ThreadID: "thread-agent-model"})
	if err != nil {
		t.Fatalf("read message store: %v", err)
	}
	if len(read.Messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(read.Messages))
	}
	if !strings.Contains(read.Messages[0].Body, "AGENT.md frontmatter model") ||
		!strings.Contains(read.Messages[0].Body, "missing:gpt-5.4") {
		t.Fatalf("message body = %q, want AGENT.md model guidance", read.Messages[0].Body)
	}
}
