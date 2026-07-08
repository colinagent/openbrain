package core

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

type toolInputSchema struct {
	Path string `json:"path"`
}

func TestListToolSpecs_PreservesInputSchema(t *testing.T) {
	server := op.NewServer(&op.Implementation{Name: "tool-server", Version: "v0.0.1"}, nil)
	op.AddTool(server, &op.Tool{
		Name:        "read",
		Description: "Read a file",
	}, func(_ context.Context, _ *op.CallToolRequest, input toolInputSchema) (*op.CallToolResult, any, error) {
		return &op.CallToolResult{}, nil, nil
	})

	t1, t2 := op.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), t1, nil); err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer session.Close()

	conn := &Connection{NodeID: "sys-server", Session: session, Ctx: context.Background()}
	specs, err := conn.ListToolSpecs()
	if err != nil {
		t.Fatalf("ListToolSpecs(): %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want 1", len(specs))
	}
	if specs[0].InputSchema == nil {
		t.Fatalf("InputSchema is nil")
	}
	params, ok := specs[0].InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("InputSchema type = %T, want map[string]any", specs[0].InputSchema)
	}
	if params["type"] != "object" {
		t.Fatalf("schema type = %v, want object", params["type"])
	}
	properties, ok := params["properties"].(map[string]any)
	if !ok || properties["path"] == nil {
		t.Fatalf("properties.path missing: %#v", params)
	}
}

func TestExecuteAgentCallEndpointAppliesAgentModel(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	config.SyncModelCache([]op.ModelConfig{{
		Key:      "cloud:gpt-5.5",
		ID:       "gpt-5.5",
		Name:     "gpt-5.5",
		Provider: "cloud",
		API:      "openai-responses",
		APIKey:   "test-key",
		BaseURL:  "https://api.example.test/v1",
	}})

	var gotMeta op.Meta
	server := op.NewServer(&op.Implementation{Name: "endpoint-agent", Version: "v0.0.1"}, nil)
	server.AddAgent(&op.AgentMeta{Name: "gbrain"}, func(_ context.Context, req *op.CallAgentRequest) (*op.CallAgentResult, error) {
		gotMeta = req.Params.Meta.Clone()
		return &op.CallAgentResult{
			AgentID: "gbrain",
			Content: &op.TextContent{Text: "ok"},
		}, nil
	})

	t1, t2 := op.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), t1, nil); err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer session.Close()

	node := &op.OpNode{
		ID:   "agent-gbrain",
		Kind: string(op.NodeKindAgent),
		Run:  op.Run{URL: "memory://endpoint"},
		Meta: &op.AgentMeta{
			Name:  "gbrain",
			Model: "cloud:MiniMax-M2.7-highspeed",
		},
	}
	cache.Set(node.ID, cache.PrefixConnection, &Connection{NodeID: node.ID, Session: session, Ctx: context.Background()}, cache.NoExpiration)

	_, err = executeAgentCall(context.Background(), node, op.Meta{"modelKey": "cloud:gpt-5.5"}, &op.TextContent{Text: "hello"}, agentCallOptions{})
	if err != nil {
		t.Fatalf("executeAgentCall(): %v", err)
	}
	if got := metaString(gotMeta, "modelKey"); got != "cloud:gpt-5.5" {
		t.Fatalf("endpoint modelKey = %q, want cloud:gpt-5.5", got)
	}
}

func TestExecuteAgentCallEndpointEnsureSessionCreatesContext(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	config.SyncModelCache([]op.ModelConfig{{
		Key:      "test:model",
		ID:       "model",
		Name:     "model",
		Provider: "test",
		API:      "openai-responses",
		APIKey:   "test-key",
		BaseURL:  "https://api.example.test/v1",
	}})

	node := &op.OpNode{
		ID:   "agent-endpoint",
		Kind: string(op.NodeKindAgent),
		Cwd:  filepath.Join(baseDir, "agents", "endpoint"),
		Run:  op.Run{URL: "memory://endpoint"},
		Meta: &op.AgentMeta{Name: "gbrain"},
	}
	threadID := "thread-endpoint-run"
	chatPath := filepath.Join(baseDir, "cron", "chats", "task-check", "run-endpoint.md")

	server := op.NewServer(&op.Implementation{Name: "endpoint-agent", Version: "v0.0.1"}, nil)
	server.AddAgent(&op.AgentMeta{Name: "gbrain"}, func(_ context.Context, req *op.CallAgentRequest) (*op.CallAgentResult, error) {
		meta := req.Params.Meta
		if _, err := loadThreadContext(metaString(meta, "threadID"), metaString(meta, "agentID")); err != nil {
			return nil, fmt.Errorf("load thread context: %w", err)
		}
		return &op.CallAgentResult{
			AgentID: "gbrain",
			Content: &op.TextContent{Text: "ok"},
		}, nil
	})

	t1, t2 := op.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), t1, nil); err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer session.Close()
	cache.Set(node.ID, cache.PrefixConnection, &Connection{NodeID: node.ID, Session: session, Ctx: context.Background()}, cache.NoExpiration)

	_, err = executeAgentCall(
		context.Background(),
		node,
		op.Meta{"threadID": threadID, "chatPath": chatPath, "agentID": node.ID, "modelKey": "test:model"},
		&op.TextContent{Text: "hello"},
		agentCallOptions{ensureSession: true},
	)
	if err != nil {
		t.Fatalf("executeAgentCall(): %v", err)
	}
	meta, err := getThreadMeta(threadID, node.ID)
	if err != nil {
		t.Fatalf("getThreadMeta(): %v", err)
	}
	if meta.ChatPath != chatPath {
		t.Fatalf("session chatPath = %q, want %q", meta.ChatPath, chatPath)
	}
}
