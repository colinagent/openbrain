package node

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/core"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/internal/scan"
)

func TestRefreshNodes_BestEffortSkipsBrokenToolServer(t *testing.T) {
	cache.Flush()
	t.Cleanup(func() {
		core.CloseDaemonConnections()
		cache.Flush()
	})
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	writeTestFile(t, filepath.Join(baseDir, "tools", "broken", "TOOL.md"), "---\nname: broken\nrun:\n  daemon: true\n  command: [\"./missing-tool\"]\n---\n")
	writeTestFile(t, filepath.Join(baseDir, "agents", "agent-a", ".agent", "AGENT.md"), "---\nname: agent-a\n---\nagent prompt\n")
	writeTestFile(t, filepath.Join(baseDir, "skills", "skill-a", "SKILL.md"), "---\nname: Skill A\ndescription: Test skill\n---\nskill prompt\n")

	if err := RefreshNodes(context.Background(), scan.ScanOptions{UID: "user-test", BaseDir: baseDir}); err != nil {
		t.Fatalf("RefreshNodes(): %v", err)
	}

	nodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)
	if got := countNodesByKind(nodes, op.NodeKindAgent); got != 1 {
		t.Fatalf("agent node count = %d, want 1", got)
	}
	if got := countNodesByKind(nodes, op.NodeKindSkill); got != 1 {
		t.Fatalf("skill node count = %d, want 1", got)
	}
	if got := countNodesByKind(nodes, op.NodeKindTools); got != 0 {
		t.Fatalf("tools node count = %d, want 0", got)
	}
}

func TestRefreshNodes_DefaultSystoolIsBuiltin(t *testing.T) {
	cache.Flush()
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	t.Cleanup(func() {
		core.CloseDaemonConnections()
		cache.Flush()
	})

	baseDir := t.TempDir()
	writeTestFile(t, filepath.Join(baseDir, "agents", "agent-a", ".agent", "AGENT.md"), "---\nname: agent-a\n---\nagent prompt\n")

	if err := RefreshNodes(context.Background(), scan.ScanOptions{UID: "user-test", BaseDir: baseDir}); err != nil {
		t.Fatalf("RefreshNodes(): %v", err)
	}

	nodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)
	if got := countNodesByKind(nodes, op.NodeKindAgent); got != 1 {
		t.Fatalf("agent node count = %d, want 1", got)
	}
	agent := nodes[0]
	assembled, err := core.NewAgent(context.Background(), &agent, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(): %v", err)
	}
	if assembled.ToolSpecs["shell"] == nil {
		t.Fatal("shell tool missing from assembled tool specs")
	}
	if assembled.ToolSpecs["read"] == nil {
		t.Fatal("read tool missing from assembled tool specs")
	}
	if assembled.ToolSpecs["write"] == nil || assembled.ToolSpecs["edit"] == nil {
		t.Fatal("default built-in systool set is incomplete")
	}
}

func TestRefreshNodes_KeepsToolNodeWithoutEndpoint(t *testing.T) {
	cache.Flush()
	t.Cleanup(func() {
		core.CloseDaemonConnections()
		cache.Flush()
	})
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	writeTestFile(t, filepath.Join(baseDir, "tools", "rg-search", "TOOL.md"), "---\nname: rg-search\ntags: system\n---\n")

	if err := RefreshNodes(context.Background(), scan.ScanOptions{UID: "user-test", BaseDir: baseDir}); err != nil {
		t.Fatalf("RefreshNodes(): %v", err)
	}

	nodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)
	if got := countNodesByKind(nodes, op.NodeKindTools); got != 1 {
		t.Fatalf("tools node count = %d, want 1", got)
	}
	if len(nodes[0].Tags) != 1 || nodes[0].Tags[0] != "system" {
		t.Fatalf("tool node tags = %v, want [system]", nodes[0].Tags)
	}
}

func TestRefreshNodes_ResolvesAgentSkillRefsFromGlobalSkills(t *testing.T) {
	cache.Flush()
	t.Cleanup(func() {
		core.CloseDaemonConnections()
		cache.Flush()
	})
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	writeTestFile(t, filepath.Join(baseDir, "skills", "search", "SKILL.md"), "---\nname: Search\ndescription: Search docs\n---\nskill prompt\n")
	writeTestFile(t, filepath.Join(baseDir, "agents", "agent-a", ".agent", "AGENT.md"), "---\nname: agent-a\nskills:\n  - @skills/search\n---\nagent prompt\n")

	if err := RefreshNodes(context.Background(), scan.ScanOptions{UID: "user-test", BaseDir: baseDir}); err != nil {
		t.Fatalf("RefreshNodes(): %v", err)
	}

	nodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)
	if got := countNodesByKind(nodes, op.NodeKindSkill); got != 1 {
		t.Fatalf("skill node count = %d, want 1", got)
	}
	if got := countNodesByKind(nodes, op.NodeKindAgent); got != 1 {
		t.Fatalf("agent node count = %d, want 1", got)
	}

	var agentNode *op.OpNode
	for i := range nodes {
		if nodes[i].Kind == string(op.NodeKindAgent) {
			agentNode = &nodes[i]
			break
		}
	}
	if agentNode == nil {
		t.Fatal("agent node is nil")
	}
	agentMeta, ok := agentNode.Meta.(*op.AgentMeta)
	if !ok || agentMeta == nil {
		t.Fatalf("agent meta = %#v, want *op.AgentMeta", agentNode.Meta)
	}
	if len(agentMeta.Skills) != 1 {
		t.Fatalf("len(agent skills) = %d, want 1", len(agentMeta.Skills))
	}
}

func TestRefreshNodes_CachesAgentSubagentRefs(t *testing.T) {
	cache.Flush()
	t.Cleanup(func() {
		core.CloseDaemonConnections()
		cache.Flush()
	})
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	writeTestFile(t, filepath.Join(baseDir, "agents", "gbrain", ".agent", "AGENT.md"), "---\nid: agent-gbrain\nname: GBrain\n---\nbrain prompt\n")
	writeTestFile(t, filepath.Join(baseDir, "agents", "coder", ".agent", "AGENT.md"), "---\nname: coder\nsubagents:\n  - ./subagents/helper\n  - \"@agent-gbrain\"\n---\nagent prompt\n")
	writeTestFile(t, filepath.Join(baseDir, "agents", "coder", ".agent", "subagents", "helper", ".agent", "AGENT.md"), "---\nname: helper\n---\nhelper prompt\n")

	if err := RefreshNodes(context.Background(), scan.ScanOptions{UID: "user-test", BaseDir: baseDir}); err != nil {
		t.Fatalf("RefreshNodes(): %v", err)
	}

	nodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)
	if got := countNodesByKind(nodes, op.NodeKindAgent); got != 3 {
		t.Fatalf("agent node count = %d, want 3", got)
	}

	var parentNode, helperNode, brainNode *op.OpNode
	for i := range nodes {
		if nodes[i].Kind != string(op.NodeKindAgent) {
			continue
		}
		meta, ok := nodes[i].Meta.(*op.AgentMeta)
		if !ok || meta == nil {
			continue
		}
		switch meta.Name {
		case "coder":
			parentNode = &nodes[i]
		case "helper":
			helperNode = &nodes[i]
		case "GBrain":
			brainNode = &nodes[i]
		}
	}
	if parentNode == nil {
		t.Fatal("parent coder node is nil")
	}
	if helperNode == nil {
		t.Fatal("helper subagent node is nil")
	}
	if brainNode == nil {
		t.Fatal("brain subagent node is nil")
	}
	parentMeta, ok := parentNode.Meta.(*op.AgentMeta)
	if !ok || parentMeta == nil {
		t.Fatalf("parent meta = %#v, want *op.AgentMeta", parentNode.Meta)
	}
	if len(parentMeta.SubAgents) != 2 ||
		!containsString(parentMeta.SubAgents, helperNode.ID) ||
		!containsString(parentMeta.SubAgents, "agent-gbrain") {
		t.Fatalf("parent subagents = %v, want [%s agent-gbrain]", parentMeta.SubAgents, helperNode.ID)
	}
	if _, ok := cache.GetValue[op.OpNode](helperNode.ID, cache.PrefixNode); !ok {
		t.Fatalf("helper subagent %s missing from node cache", helperNode.ID)
	}
	if _, ok := cache.GetValue[op.OpNode]("agent-gbrain", cache.PrefixNode); !ok {
		t.Fatalf("brain subagent agent-gbrain missing from node cache")
	}
}

func TestRefreshNodes_CachesPrivateSubagentCandidates(t *testing.T) {
	cache.Flush()
	t.Cleanup(func() {
		core.CloseDaemonConnections()
		cache.Flush()
	})
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	writeTestFile(t, filepath.Join(baseDir, "agents", "coder", ".agent", "AGENT.md"), "---\nid: agent-coder\nname: coder\n---\nagent prompt\n")
	writeTestFile(t, filepath.Join(baseDir, "agents", "coder", ".agent", "subagents", "helper", ".agent", "AGENT.md"), "---\nid: agent-helper\nname: helper\nopcodes:\n  - thread/submit\n---\nhelper prompt\n")

	if err := RefreshNodes(context.Background(), scan.ScanOptions{UID: "user-test", BaseDir: baseDir}); err != nil {
		t.Fatalf("RefreshNodes(): %v", err)
	}
	parentNode, ok := cache.GetValue[op.OpNode]("agent-coder", cache.PrefixNode)
	if !ok {
		t.Fatalf("agent-coder missing from node cache")
	}
	parentMeta, ok := parentNode.Meta.(*op.AgentMeta)
	if !ok || parentMeta == nil {
		t.Fatalf("parent meta = %#v, want *op.AgentMeta", parentNode.Meta)
	}
	if len(parentMeta.SubAgents) != 0 {
		t.Fatalf("parent subagents = %v, want none", parentMeta.SubAgents)
	}
	helperNode, ok := cache.GetValue[op.OpNode]("agent-helper", cache.PrefixNode)
	if !ok {
		t.Fatalf("agent-helper missing from node cache")
	}
	if helperNode.Cwd != filepath.Join(baseDir, "agents", "coder", ".agent", "subagents", "helper") {
		t.Fatalf("helper cwd = %q", helperNode.Cwd)
	}
}

func TestRefreshNodes_FailsOnExplicitIDConflict(t *testing.T) {
	cache.Flush()
	t.Cleanup(func() {
		core.CloseDaemonConnections()
		cache.Flush()
	})
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	writeTestFile(t, filepath.Join(baseDir, "agents", "one", ".agent", "AGENT.md"), "---\nid: agent-shared\nname: one\n---\none\n")
	writeTestFile(t, filepath.Join(baseDir, "agents", "two", ".agent", "AGENT.md"), "---\nid: agent-shared\nname: two\n---\ntwo\n")

	err := RefreshNodes(context.Background(), scan.ScanOptions{UID: "user-test", BaseDir: baseDir})
	if err == nil {
		t.Fatal("RefreshNodes() = nil, want id conflict")
	}
	if !strings.Contains(err.Error(), "id conflict") || !strings.Contains(err.Error(), "agent-shared") {
		t.Fatalf("RefreshNodes() error = %v, want id conflict for agent-shared", err)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func countNodesByKind(nodes []op.OpNode, kind op.NodeKind) int {
	count := 0
	for _, node := range nodes {
		if node.Kind == string(kind) {
			count++
		}
	}
	return count
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
