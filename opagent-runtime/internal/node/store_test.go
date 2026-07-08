package node

// import (
// 	"context"
// 	"testing"

// 	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
// 	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
// 	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
// )

// func setEnvForTest(t *testing.T, env string) {
// 	t.Helper()
// 	prev := config.Get()
// 	config.Set(&op.SystemConfig{Env: env})
// 	t.Cleanup(func() {
// 		config.Set(prev)
// 	})
// }

// func TestCacheStoreListAndDeleteByID(t *testing.T) {
// 	setEnvForTest(t, "local")
// 	ctx := context.Background()
// 	cache.NewCache(ctx, nil)
// 	cache.CloseAll()
// 	store := NewCacheStore()

// 	toolNode := BuildNode(KindTools, "local", "file:///tmp/tools/TOOL.md", nil, op.ToolsMeta{
// 		Name: "system-tools",
// 	})
// 	agentNode := BuildNode(KindAgent, "local", "file:///tmp/agents/a/AGENT.md", nil, op.AgentMeta{
// 		Name: "a",
// 	})

// 	if err := store.Upsert(ctx, toolNode); err != nil {
// 		t.Fatalf("upsert tool node: %v", err)
// 	}
// 	if err := store.Upsert(ctx, agentNode); err != nil {
// 		t.Fatalf("upsert agent node: %v", err)
// 	}

// 	nodes, err := store.ListByUIDAndKind(ctx, "local", KindTools)
// 	if err != nil {
// 		t.Fatalf("list by uid and kind: %v", err)
// 	}
// 	if len(nodes) != 1 {
// 		t.Fatalf("expected 1 tools node, got %d", len(nodes))
// 	}
// 	if nodes[0].ID != toolNode.ID {
// 		t.Fatalf("unexpected node id: %s", nodes[0].ID)
// 	}

// 	if err := store.DeleteByID(ctx, toolNode.ID); err != nil {
// 		t.Fatalf("delete by id: %v", err)
// 	}
// 	nodes, err = store.ListByUIDAndKind(ctx, "local", KindTools)
// 	if err != nil {
// 		t.Fatalf("list after delete: %v", err)
// 	}
// 	if len(nodes) != 0 {
// 		t.Fatalf("expected empty list after delete, got %d", len(nodes))
// 	}
// }

// func TestGetByID(t *testing.T) {
// 	setEnvForTest(t, "local")
// 	ctx := context.Background()
// 	cache.NewCache(ctx, nil)
// 	cache.CloseAll()
// 	store := NewCacheStore()

// 	node := BuildNode(KindAgent, "local", "file:///tmp/agents/a/AGENT.md", nil, op.AgentMeta{
// 		Name: "a",
// 	})
// 	if err := store.Upsert(ctx, node); err != nil {
// 		t.Fatalf("upsert: %v", err)
// 	}

// 	got, err := store.GetByID(ctx, node.ID)
// 	if err != nil {
// 		t.Fatalf("get by id: %v", err)
// 	}
// 	if got.ID != node.ID {
// 		t.Fatalf("expected id %q, got %q", node.ID, got.ID)
// 	}
// }

// func TestNodeMeta(t *testing.T) {
// 	setEnvForTest(t, "local")
// 	node := BuildNode(KindAgent, "local", "file:///test", nil, op.AgentMeta{Name: "test"})
// 	meta, ok := NodeMeta[op.AgentMeta](node)
// 	if !ok {
// 		t.Fatal("expected meta extraction to succeed")
// 	}
// 	if meta.Name != "test" {
// 		t.Fatalf("expected name=test, got %q", meta.Name)
// 	}
// }

// func TestBuildNodeLocalEnvIgnoresUIDForID(t *testing.T) {
// 	setEnvForTest(t, "local")
// 	uri := "file:///tmp/agents/x/.agent/AGENT.md"
// 	a := BuildNode(KindAgent, "alice", uri, nil, op.AgentMeta{Name: "x"})
// 	b := BuildNode(KindAgent, "bob", uri, nil, op.AgentMeta{Name: "x"})
// 	if a.ID != b.ID {
// 		t.Fatalf("expected same node id in local env, got %q and %q", a.ID, b.ID)
// 	}
// 	if a.UID == b.UID {
// 		t.Fatalf("expected uid to remain original in node payload")
// 	}
// }

// func TestBuildNodeCloudEnvKeepsUIDIsolationForID(t *testing.T) {
// 	setEnvForTest(t, "cloud")
// 	uri := "file:///tmp/agents/x/.agent/AGENT.md"
// 	a := BuildNode(KindAgent, "alice", uri, nil, op.AgentMeta{Name: "x"})
// 	b := BuildNode(KindAgent, "bob", uri, nil, op.AgentMeta{Name: "x"})
// 	if a.ID == b.ID {
// 		t.Fatalf("expected different node ids in cloud env for different uids, got same %q", a.ID)
// 	}
// }
