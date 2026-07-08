package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestResolveAgentFileFromExecutablePath(t *testing.T) {
	exePath := filepath.Join("/tmp", "coder", ".agent", "bin", "coder")
	got := resolveAgentFileFromExecutablePath(exePath)
	want := filepath.Join("/tmp", "coder", ".agent", "AGENT.md")
	if got != want {
		t.Fatalf("resolveAgentFileFromExecutablePath() = %q, want %q", got, want)
	}
}

func TestHandleCallAgent_ForwardsToAgentLoopCreate(t *testing.T) {
	ctx := context.Background()
	server := op.NewServer(&op.Implementation{Name: "coder", Version: "v0.0.1"}, nil)
	server.AddAgent(&op.AgentMeta{Name: "coder"}, handleCallAgent)

	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	defer serverSession.Close()

	var captured *op.OpNodeParams
	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpNodeHandler: func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req == nil || req.Params == nil {
				t.Fatal("OpNodeHandler received nil params")
			}
			clone := *req.Params
			clone.Meta = cloneMeta(req.Params.Meta)
			captured = &clone
			return &op.OpNodeResult{
				OpCode:  req.Params.OpCode,
				Meta:    op.Meta{"forwarded": true},
				Content: &op.TextContent{Text: "ok"},
			}, nil
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer clientSession.Close()

	originalMeta := op.Meta{"threadID": "thread-test", "chatPath": "/tmp/chat.md"}
	result, err := clientSession.CallAgent(ctx, &op.CallAgentParams{
		AgentID: "coder",
		Meta:    originalMeta,
		Content: &op.TextContent{Text: "hello"},
	})
	if err != nil {
		t.Fatalf("CallAgent(): %v", err)
	}
	if captured == nil {
		t.Fatal("captured OpNode params = nil")
	}
	if captured.OpCode != op.OpAgentLoopCreate {
		t.Fatalf("forwarded OpCode = %q, want %q", captured.OpCode, op.OpAgentLoopCreate)
	}
	if got := captured.Meta["opcode"]; got != nil {
		t.Fatalf("forwarded meta opcode = %#v, want nil", got)
	}
	if got := originalMeta["opcode"]; got != nil {
		t.Fatalf("original meta opcode = %#v, want nil", got)
	}
	content, ok := captured.Content.(*op.TextContent)
	if !ok {
		t.Fatalf("forwarded content type = %T, want *op.TextContent", captured.Content)
	}
	if content.Text != "hello" {
		t.Fatalf("forwarded content = %q, want hello", content.Text)
	}
	if result == nil {
		t.Fatal("CallAgent() result = nil")
	}
	if result.AgentID != "coder" {
		t.Fatalf("result.AgentID = %q, want coder", result.AgentID)
	}
	if got, _ := result.Meta["forwarded"].(bool); !got {
		t.Fatalf("result.Meta[forwarded] = %#v, want true", result.Meta["forwarded"])
	}
}

func TestHandleCallAgent_ReturnsForwardError(t *testing.T) {
	ctx := context.Background()
	server := op.NewServer(&op.Implementation{Name: "coder", Version: "v0.0.1"}, nil)
	server.AddAgent(&op.AgentMeta{Name: "coder"}, handleCallAgent)

	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	defer serverSession.Close()

	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpNodeHandler: func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			return nil, fmt.Errorf("forward failed")
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer clientSession.Close()

	_, err = clientSession.CallAgent(ctx, &op.CallAgentParams{
		AgentID: "coder",
		Meta:    op.Meta{"threadID": "thread-test"},
		Content: &op.TextContent{Text: "hello"},
	})
	if err == nil {
		t.Fatal("CallAgent() succeeded, want error")
	}
}

func TestBuildPromptDoesNotInjectMemoryPath(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	agentFile := filepath.Join(tempDir, "AGENT.md")
	if err := os.WriteFile(agentFile, []byte("---\nname: coder\n---\nBase prompt"), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	server := op.NewServer(&op.Implementation{Name: "coder", Version: "v0.0.1"}, nil)
	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	defer serverSession.Close()

	nodes := []listedNode{{
		ID:   "agent-coder",
		Kind: string(op.NodeKindAgent),
		Meta: rawJSON(t, listedAgentMeta{Name: "coder"}),
	}}
	rawNodes, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("marshal nodes: %v", err)
	}

	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpNodeHandler: func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			switch req.Params.OpCode {
			case op.OpNodeList:
				return &op.OpNodeResult{Content: &op.JsonContent{Raw: rawNodes}}, nil
			default:
				return nil, fmt.Errorf("unexpected opcode: %s", req.Params.OpCode)
			}
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer clientSession.Close()

	prompt, err := buildPrompt(ctx, serverSession, agentFile, op.Meta{"agentID": "agent-coder"})
	if err != nil {
		t.Fatalf("buildPrompt(): %v", err)
	}
	if strings.Contains(prompt, "memory.md") || strings.Contains(prompt, "OpAgent Memory") {
		t.Fatalf("prompt unexpectedly contains memory instructions:\n%s", prompt)
	}
}

func TestBuildSystemPromptWithPathsIncludesOpAgentShellContext(t *testing.T) {
	prompt := BuildSystemPromptWithPaths(
		"Base prompt.",
		ResolveOpAgentShellContext("windows"),
		"",
		nil,
		nil,
		nil,
	)
	if !strings.Contains(prompt, "## OpAgent Shell Context") {
		t.Fatalf("prompt missing shell context:\n%s", prompt)
	}
	if !strings.Contains(prompt, "powershell.exe -NoProfile -ExecutionPolicy Bypass -Command <command>") {
		t.Fatalf("prompt missing powershell execution:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Write PowerShell syntax") {
		t.Fatalf("prompt missing powershell syntax guidance:\n%s", prompt)
	}

	prompt = BuildSystemPromptWithPaths(
		"Base prompt.",
		ResolveOpAgentShellContext("linux"),
		"",
		nil,
		nil,
		nil,
	)
	if !strings.Contains(prompt, "sh -c <command>") {
		t.Fatalf("prompt missing POSIX shell execution:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Write POSIX sh syntax") {
		t.Fatalf("prompt missing POSIX shell syntax guidance:\n%s", prompt)
	}
}

func TestResolveSkillContextsUsesNodeIDs(t *testing.T) {
	ctx := context.Background()
	server := op.NewServer(&op.Implementation{Name: "opagent", Version: "v0.0.1"}, nil)

	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	defer serverSession.Close()

	nodes := []listedNode{
		{
			ID:   "agent-alpha",
			Kind: string(op.NodeKindAgent),
			Meta: rawJSON(t, listedAgentMeta{Skills: []string{"skill-plan", "skill-execute"}}),
		},
		{
			ID:   "skill-plan",
			Kind: string(op.NodeKindSkill),
			URI:  op.PathToURI("/tmp/opagent/skills/plan/SKILL.md"),
			Cwd:  "/tmp/opagent/skills/plan",
			Meta: rawJSON(t, listedSkillMeta{Slug: "plan", Name: "Plan", Description: "Make a plan"}),
		},
		{
			ID:   "skill-execute",
			Kind: string(op.NodeKindSkill),
			URI:  op.PathToURI("/tmp/opagent/skills/execute-plan/SKILL.md"),
			Cwd:  "/tmp/opagent/skills/execute-plan",
			Meta: rawJSON(t, listedSkillMeta{Slug: "execute-plan", Name: "Execute Plan", Description: "Execute a plan"}),
		},
	}
	rawNodes, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("marshal nodes: %v", err)
	}

	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpNodeHandler: func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req == nil || req.Params == nil {
				t.Fatal("OpNodeHandler received nil params")
			}
			if req.Params.OpCode != op.OpNodeList {
				t.Fatalf("OpCode = %q, want %q", req.Params.OpCode, op.OpNodeList)
			}
			return &op.OpNodeResult{Content: &op.JsonContent{Raw: rawNodes}}, nil
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer clientSession.Close()

	available, selected, err := resolveSkillContexts(ctx, serverSession, op.Meta{
		"agentID":          "agent-alpha",
		"selectedSkillIDs": []any{"skill-execute"},
	})
	if err != nil {
		t.Fatalf("resolveSkillContexts(): %v", err)
	}
	if len(available) != 1 || available[0].ID != "skill-plan" {
		t.Fatalf("available = %+v, want only skill-plan", available)
	}
	if len(selected) != 1 || selected[0].ID != "skill-execute" {
		t.Fatalf("selected = %+v, want only skill-execute", selected)
	}
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}
	return raw
}

func rawJSONContent(t *testing.T, value any) *op.JsonContent {
	t.Helper()
	return op.NewJsonContentRaw(rawJSON(t, value))
}
