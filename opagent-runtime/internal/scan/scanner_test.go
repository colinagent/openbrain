package scan

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func TestResolveRefs_TreatsBareSystoolNamesAsSysTools(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()

	scanner := NewScanner("user", baseDir)
	ids, sysTools := scanner.resolveRefs([]string{"shell", "read", "message_publish", "agent_task"}, op.NodeKindTools, baseDir)
	if len(ids) != 0 {
		t.Fatalf("ids = %v, want empty", ids)
	}
	if !reflect.DeepEqual(sysTools, []string{"shell", "read", "message_publish", "agent_task"}) {
		t.Fatalf("sysTools = %v, want [shell read message_publish agent_task]", sysTools)
	}
}

func TestScannerAddNode_DefaultDedup_DropsSameIDDifferentCwd(t *testing.T) {
	scanner := NewScanner("user", "/tmp")
	if !scanner.addNode(&op.OpNode{ID: "agent-coder", Cwd: "/workspace"}) {
		t.Fatal("first addNode() = false, want true")
	}
	if scanner.addNode(&op.OpNode{ID: "agent-coder", Cwd: "/workspace/temp"}) {
		t.Fatal("second addNode() = true, want false")
	}
	if got := len(scanner.Nodes()); got != 1 {
		t.Fatalf("len(Nodes()) = %d, want 1", got)
	}
}

func TestScannerAddNode_PathAwareDedup_KeepsSameIDDifferentCwd(t *testing.T) {
	scanner := NewScanner("user", "/tmp").WithPathAwareAgentDedup()
	if !scanner.addNode(&op.OpNode{ID: "agent-coder", Cwd: "/workspace"}) {
		t.Fatal("first addNode() = false, want true")
	}
	if !scanner.addNode(&op.OpNode{ID: "agent-coder", Cwd: "/workspace/temp"}) {
		t.Fatal("second addNode() = false, want true")
	}
	if got := len(scanner.Nodes()); got != 2 {
		t.Fatalf("len(Nodes()) = %d, want 2", got)
	}
}

func TestScanSkills_RequiresNameAndDescription(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "skills", "missing-name", "SKILL.md"), "---\ndescription: Missing name\n---\nbody\n")
	writeScannerTestFile(t, filepath.Join(baseDir, "skills", "missing-description", "SKILL.md"), "---\nname: missing-description\n---\nbody\n")

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanSkills(filepath.Join(baseDir, "skills"), 0)
	if len(got) != 0 {
		t.Fatalf("ScanSkills() len = %d, want 0", len(got))
	}
}

func TestScanSkills_IgnoresNestedSkillFiles(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "skills", "search", "SKILL.md"), "---\nname: Search\ndescription: Search docs\n---\nbody\n")
	writeScannerTestFile(t, filepath.Join(baseDir, "skills", "search", "references", "nested", "SKILL.md"), "---\nname: Nested\ndescription: Should not be scanned\n---\nbody\n")

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanSkills(filepath.Join(baseDir, "skills"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanSkills() len = %d, want 1", len(got))
	}
	meta, ok := got[0].Meta.(*op.SkillMeta)
	if !ok || meta == nil {
		t.Fatalf("skill meta = %#v, want *op.SkillMeta", got[0].Meta)
	}
	if meta.Slug != "search" {
		t.Fatalf("skill slug = %q, want %q", meta.Slug, "search")
	}
}

func TestScanAgents_ResolvesGlobalSkillRefsWithRefBaseDir(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	workspaceDir := filepath.Join(baseDir, "workspace")
	writeScannerTestFile(t, filepath.Join(baseDir, "skills", "search", "SKILL.md"), "---\nname: Search\ndescription: Search docs\n---\nbody\n")
	writeScannerTestFile(t, filepath.Join(workspaceDir, "agent-a", ".agent", "AGENT.md"), "---\nname: agent-a\nskills:\n  - @skills/search\n---\nagent prompt\n")

	scanner := NewScanner("user", workspaceDir).WithRefBaseDir(baseDir)
	nodes := scanner.ScanAgents(workspaceDir, 0)
	if len(nodes) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(nodes))
	}
	meta, ok := nodes[0].Meta.(*op.AgentMeta)
	if !ok || meta == nil {
		t.Fatalf("agent meta = %#v, want *op.AgentMeta", nodes[0].Meta)
	}
	if len(meta.Skills) != 1 {
		t.Fatalf("len(agent skills) = %d, want 1", len(meta.Skills))
	}
	kind, ok := op.NodeKindFromID(meta.Skills[0])
	if !ok || kind != op.NodeKindSkill {
		t.Fatalf("resolved skill id kind = %q, want %q", kind, op.NodeKindSkill)
	}
}

func TestScannerScansOrgNamespacePackages(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "agents", "@org-acme", "review-agent", ".agent", "AGENT.md"), "---\nname: Review Agent\nskills:\n  - @skills/@org-acme/review\n---\nprompt\n")
	writeScannerTestFile(t, filepath.Join(baseDir, "skills", "@org-acme", "review", "SKILL.md"), "---\nname: Review\ndescription: Internal review\n---\nbody\n")
	writeScannerTestFile(t, filepath.Join(baseDir, "skills", "@org-acme", "review", "references", "nested", "SKILL.md"), "---\nname: Nested\ndescription: should not scan\n---\nbody\n")
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "@org-acme", "internal-tools", "TOOL.md"), "---\nname: internal-tools\nrun:\n  command: [\"./internal-tools\"]\n---\n")

	scanner := NewScanner("user", baseDir)
	tools := scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	skills := scanner.ScanSkills(filepath.Join(baseDir, "skills"), 0)
	agents := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner err = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(tools))
	}
	if len(skills) != 1 {
		t.Fatalf("skills = %d, want 1", len(skills))
	}
	if len(agents) != 1 {
		t.Fatalf("agents = %d, want 1", len(agents))
	}
	agentMeta, ok := agents[0].Meta.(*op.AgentMeta)
	if !ok || agentMeta == nil || len(agentMeta.Skills) != 1 || agentMeta.Skills[0] != skills[0].ID {
		t.Fatalf("agent skills = %#v, want org skill %s", agentMeta, skills[0].ID)
	}
}

func TestScanAgents_ResolvesBindByScanningTargetWhenCacheCold(t *testing.T) {
	cache.Flush()
	baseDir := t.TempDir()
	cfg := &op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal}
	config.SetSystem(cfg)

	agentFile := filepath.Join(baseDir, "agents", "coder", ".agent", "AGENT.md")
	writeScannerTestFile(t, agentFile, "---\nname: coder\n---\nprompt\n")
	agentID := op.BuildNodeID(op.LocalUser, cfg.HostID, op.NodeKindAgent, op.PathToURI(agentFile), cfg.Env)

	workspaceDir := filepath.Join(baseDir, "workspace")
	writeScannerTestFile(t, filepath.Join(workspaceDir, ".agent", "AGENT.md"), "---\nbind: @"+agentID+"\n---\n")

	scanner := NewScanner(op.LocalUser, workspaceDir).
		WithPathAwareAgentDedup().
		WithRefBaseDir(baseDir).
		WithNodeIndexBaseDir(baseDir)
	nodes := scanner.ScanAgents(workspaceDir, 0)
	if len(nodes) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(nodes))
	}
	if nodes[0].ID != agentID {
		t.Fatalf("bound node ID = %q, want %q", nodes[0].ID, agentID)
	}
	if nodes[0].Cwd != workspaceDir {
		t.Fatalf("bound node Cwd = %q, want %q", nodes[0].Cwd, workspaceDir)
	}
	meta, ok := nodes[0].Meta.(*op.AgentMeta)
	if !ok || meta == nil || meta.Name != "coder" {
		t.Fatalf("bound node meta = %#v, want coder agent meta", nodes[0].Meta)
	}
	if cached, ok := cache.GetValue[op.OpNode](agentID, cache.PrefixNode); !ok || cached.ID != agentID {
		t.Fatalf("bound target was not cached after cold bind scan")
	}
}

func TestScanAgents_UsesManifestID(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "agents", "shared", ".agent", "AGENT.md"), "---\nid: agent-shared\nname: shared\n---\nprompt\n")

	scanner := NewScanner("user", baseDir).WithNodeIndexBaseDir(baseDir)
	nodes := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner.Err(): %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(nodes))
	}
	if nodes[0].ID != "agent-shared" {
		t.Fatalf("node ID = %q, want agent-shared", nodes[0].ID)
	}
}

func TestScanSkillsAndTools_UseManifestID(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "skills", "plan", "SKILL.md"), "---\nid: skill-plan\nname: Plan\ndescription: Plan work\n---\nbody\n")
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "rg-search", "TOOL.md"), "---\nid: tools-rg-search\nname: rg-search\nrun:\n  command: [\"./rg-search\"]\n---\n")

	scanner := NewScanner("user", baseDir).WithNodeIndexBaseDir(baseDir)
	skills := scanner.ScanSkills(filepath.Join(baseDir, "skills"), 0)
	tools := scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner.Err(): %v", err)
	}
	if len(skills) != 1 || skills[0].ID != "skill-plan" {
		t.Fatalf("skills = %#v, want skill-plan", skills)
	}
	if len(tools) != 1 || tools[0].ID != "tools-rg-search" {
		t.Fatalf("tools = %#v, want tools-rg-search", tools)
	}
}

func TestScanAgents_RejectsManifestIDKindMismatch(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "agents", "bad", ".agent", "AGENT.md"), "---\nid: skill-bad\nname: bad\n---\nprompt\n")

	scanner := NewScanner("user", baseDir)
	nodes := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(nodes) != 0 {
		t.Fatalf("ScanAgents() len = %d, want 0", len(nodes))
	}
	err := scanner.Err()
	if err == nil {
		t.Fatal("scanner.Err() = nil, want id mismatch")
	}
	if !strings.Contains(err.Error(), "id") || !strings.Contains(err.Error(), "agent-") {
		t.Fatalf("scanner.Err() = %v, want agent id mismatch", err)
	}
}

func TestScanAgents_ReportsExplicitIDConflict(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "agents", "one", ".agent", "AGENT.md"), "---\nid: agent-shared\nname: one\n---\none\n")
	writeScannerTestFile(t, filepath.Join(baseDir, "agents", "two", ".agent", "AGENT.md"), "---\nid: agent-shared\nname: two\n---\ntwo\n")

	scanner := NewScanner("user", baseDir).WithNodeIndexBaseDir(baseDir)
	_ = scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	err := scanner.Err()
	if err == nil {
		t.Fatal("scanner.Err() = nil, want id conflict")
	}
	if !strings.Contains(err.Error(), "id conflict") || !strings.Contains(err.Error(), "agent-shared") {
		t.Fatalf("scanner.Err() = %v, want explicit conflict for agent-shared", err)
	}
}

func TestScanAgents_ResolvesRelativeToolServerRefs(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	agentRoot := filepath.Join(baseDir, "agents", "researcher")
	writeScannerTestFile(t, filepath.Join(agentRoot, ".agent", "AGENT.md"), "---\nname: researcher\ntools:\n  - read\n  - shell\n  - ./tools/research-tools\n---\nprompt\n")
	writeScannerTestFile(t, filepath.Join(agentRoot, ".agent", "tools", "research-tools", "TOOL.md"), "---\nname: research-tools\nrun:\n  command: [\"./bin/research-tools\"]\n---\n")

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(got))
	}
	meta, ok := got[0].Meta.(*op.AgentMeta)
	if !ok || meta == nil {
		t.Fatalf("agent meta = %#v, want *op.AgentMeta", got[0].Meta)
	}
	if len(meta.ToolServers) != 1 {
		t.Fatalf("len(agent tool servers) = %d, want 1", len(meta.ToolServers))
	}
	if len(meta.SysTools) != 2 {
		t.Fatalf("len(agent system tools) = %d, want 2", len(meta.SysTools))
	}
	if meta.SysToolMode != op.SystoolModeAllowlist {
		t.Fatalf("sysToolMode = %q, want allowlist", meta.SysToolMode)
	}
	kind, ok := op.NodeKindFromID(meta.ToolServers[0])
	if !ok || kind != op.NodeKindTools {
		t.Fatalf("resolved tool id kind = %q, want %q", kind, op.NodeKindTools)
	}
}

func TestScanAgents_ResolvesGBrainCloudToolServerRef(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "gbrain-cloud", "TOOL.md"), `---
name: gbrain-cloud
run:
  daemon: true
  url: "https://api.op-agent.com/gbrain/mcp"
  header:
    Authorization: "Bearer {openbrain_session}"
---
`)
	writeScannerTestFile(t, filepath.Join(baseDir, "agents", "gbrain", ".agent", "AGENT.md"), `---
id: agent-gbrain
name: gbrain
tools:
  - "@tools/gbrain-cloud"
  - shell
---
prompt
`)

	scanner := NewScanner("user", baseDir).WithNodeIndexBaseDir(baseDir)
	_ = scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	got := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(got))
	}
	meta, ok := got[0].Meta.(*op.AgentMeta)
	if !ok || meta == nil {
		t.Fatalf("agent meta = %#v, want *op.AgentMeta", got[0].Meta)
	}
	if len(meta.ToolServers) != 1 {
		t.Fatalf("tool servers = %v, want exactly gbrain-cloud", meta.ToolServers)
	}
	kind, ok := op.NodeKindFromID(meta.ToolServers[0])
	if !ok || kind != op.NodeKindTools {
		t.Fatalf("resolved tool id kind = %q, want %q", kind, op.NodeKindTools)
	}
	if !reflect.DeepEqual(meta.SysTools, []string{"shell"}) {
		t.Fatalf("sys tools = %v, want [shell]", meta.SysTools)
	}
	if meta.SysToolMode != op.SystoolModeAllowlist {
		t.Fatalf("sysToolMode = %q, want allowlist", meta.SysToolMode)
	}
}

func TestScanAgents_ToolsFieldEnablesEmptySystoolAllowlist(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "gbrain-cloud", "TOOL.md"), `---
name: gbrain-cloud
run:
  daemon: true
  url: "https://api.op-agent.com/gbrain/mcp"
---
`)
	writeScannerTestFile(t, filepath.Join(baseDir, "agents", "gbrain", ".agent", "AGENT.md"), `---
id: agent-gbrain
name: gbrain
tools:
  - "@tools/gbrain-cloud"
---
prompt
`)

	scanner := NewScanner("user", baseDir).WithNodeIndexBaseDir(baseDir)
	_ = scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	got := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(got))
	}
	meta := got[0].Meta.(*op.AgentMeta)
	if len(meta.SysTools) != 0 {
		t.Fatalf("sys tools = %v, want empty default marker", meta.SysTools)
	}
	if meta.SysToolMode != op.SystoolModeAllowlist {
		t.Fatalf("sysToolMode = %q, want allowlist", meta.SysToolMode)
	}
	if len(meta.ToolServers) != 1 {
		t.Fatalf("tool servers = %v, want gbrain-cloud", meta.ToolServers)
	}
}

func TestScanAgents_DefaultSystoolModeWhenToolsFieldMissing(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "agents", "default-tools", ".agent", "AGENT.md"), `---
id: agent-default-tools
name: default-tools
---
prompt
`)

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(got))
	}
	meta := got[0].Meta.(*op.AgentMeta)
	if meta.SysToolMode != op.SystoolModeDefault {
		t.Fatalf("sysToolMode = %q, want default", meta.SysToolMode)
	}
	if len(meta.SysTools) != 0 {
		t.Fatalf("sys tools = %v, want none", meta.SysTools)
	}
}

func TestScanAgents_DisablesSystoolWithNullMarker(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "agents", "no-tools", ".agent", "AGENT.md"), `---
id: agent-no-tools
name: no-tools
"@systool": null
---
prompt
`)

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(got))
	}
	meta := got[0].Meta.(*op.AgentMeta)
	if meta.SysToolMode != op.SystoolModeDisabled {
		t.Fatalf("sysToolMode = %q, want disabled", meta.SysToolMode)
	}
	if len(meta.SysTools) != 0 {
		t.Fatalf("sys tools = %v, want none", meta.SysTools)
	}
}

func TestScanAgents_RejectsDisabledSystoolWithAllowlist(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "agents", "bad", ".agent", "AGENT.md"), `---
id: agent-bad
name: bad
"@systool": null
tools:
  - shell
---
prompt
`)

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(got) != 0 {
		t.Fatalf("ScanAgents() len = %d, want 0", len(got))
	}
	if err := scanner.Err(); err == nil || !strings.Contains(err.Error(), "@systool: null conflicts") {
		t.Fatalf("scanner.Err() = %v, want @systool conflict", err)
	}
}

func TestScanAgents_WarnsWhenExplicitToolRefIsMissing(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	agentPath := filepath.Join(baseDir, "agents", "gbrain", ".agent", "AGENT.md")
	writeScannerTestFile(t, agentPath, `---
id: agent-gbrain
name: gbrain
tools:
  - "@tools/gbrain-cloud"
  - shell
---
prompt
`)

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)

	scanner := NewScanner("user", baseDir).WithNodeIndexBaseDir(baseDir)
	got := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(got))
	}
	output := logs.String()
	if !strings.Contains(output, "failed to resolve tool ref") ||
		!strings.Contains(output, "@tools/gbrain-cloud") ||
		!strings.Contains(output, agentPath) {
		t.Fatalf("log output = %q, want unresolved tool ref with ref and manifest path", output)
	}
}

func TestScanAgents_ResolvesRelativeSkillDirectoryRef(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	agentRoot := filepath.Join(baseDir, "agents", "researcher")
	writeScannerTestFile(t, filepath.Join(agentRoot, ".agent", "AGENT.md"), "---\nname: researcher\nskills:\n  - ./skills\n---\nprompt\n")
	writeScannerTestFile(t, filepath.Join(agentRoot, ".agent", "skills", "foo", "SKILL.md"), "---\nname: Foo\ndescription: First skill\n---\nbody\n")
	writeScannerTestFile(t, filepath.Join(agentRoot, ".agent", "skills", "bar", "SKILL.md"), "---\nname: Bar\ndescription: Second skill\n---\nbody\n")

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(got))
	}
	meta, ok := got[0].Meta.(*op.AgentMeta)
	if !ok || meta == nil {
		t.Fatalf("agent meta = %#v, want *op.AgentMeta", got[0].Meta)
	}
	if len(meta.Skills) != 2 {
		t.Fatalf("len(agent skills) = %d, want 2", len(meta.Skills))
	}
	for _, skillID := range meta.Skills {
		kind, ok := op.NodeKindFromID(skillID)
		if !ok || kind != op.NodeKindSkill {
			t.Fatalf("resolved skill id kind = %q, want %q", kind, op.NodeKindSkill)
		}
	}
}

func TestScanToolsParsesRunHeader(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "gbrain-cloud", "TOOL.md"), `---
name: gbrain-cloud
run:
  daemon: true
  url: "https://api.op-agent.com/gbrain/mcp"
  header:
    Authorization: "Bearer {openbrain_session}"
---
`)
	scanner := NewScanner("user", baseDir)
	got := scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanTools() len = %d, want 1", len(got))
	}
	if got[0].Run.Header["Authorization"] != "Bearer {openbrain_session}" {
		t.Fatalf("run.header.Authorization = %q", got[0].Run.Header["Authorization"])
	}
	if !got[0].Run.Daemon {
		t.Fatal("run.daemon = false, want true")
	}
}

func TestScanTools_RunAuthFieldIsRejected(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "gbrain-cloud", "TOOL.md"), `---
name: gbrain-cloud
run:
  daemon: true
  url: "https://api.op-agent.com/gbrain/mcp"
  auth:
    type: openbrain_session
---
`)
	scanner := NewScanner("user", baseDir)
	got := scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	if len(got) != 0 {
		t.Fatalf("ScanTools() len = %d, want 0", len(got))
	}
}

func TestScanTools_RunHeaderWithCommandIsRejected(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "local", "TOOL.md"), `---
name: local
run:
  command: ["./bin/local"]
  header:
    Authorization: "Bearer token"
---
`)
	scanner := NewScanner("user", baseDir)
	got := scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	if len(got) != 0 {
		t.Fatalf("ScanTools() len = %d, want 0", len(got))
	}
}

func TestScanTools_RunHeaderValueMustBeString(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "remote", "TOOL.md"), `---
name: remote
run:
  url: "https://example.com/mcp"
  header:
    X-Retry: 3
---
`)
	scanner := NewScanner("user", baseDir)
	got := scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	if len(got) != 0 {
		t.Fatalf("ScanTools() len = %d, want 0", len(got))
	}
}

func TestScanAgents_DoesNotResolveLegacySiblingResourceRefs(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	agentRoot := filepath.Join(baseDir, "agents", "researcher")
	writeScannerTestFile(t, filepath.Join(agentRoot, ".agent", "AGENT.md"), "---\nname: researcher\ntools:\n  - ./tools/research-tools\n---\nprompt\n")
	writeScannerTestFile(t, filepath.Join(agentRoot, "tools", "research-tools", "TOOL.md"), "---\nname: research-tools\nrun:\n  command: [\"./bin/research-tools\"]\n---\n")

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(got))
	}
	meta, ok := got[0].Meta.(*op.AgentMeta)
	if !ok || meta == nil {
		t.Fatalf("agent meta = %#v, want *op.AgentMeta", got[0].Meta)
	}
	if len(meta.ToolServers) != 0 {
		t.Fatalf("len(agent tool servers) = %d, want 0 for legacy sibling resources", len(meta.ToolServers))
	}
}

func TestScanAgents_DiscoversPrivateSubagentsWithoutMountingThem(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	agentRoot := filepath.Join(baseDir, "agents", "coder")
	writeScannerTestFile(t, filepath.Join(agentRoot, ".agent", "AGENT.md"), "---\nid: agent-coder\nname: coder\n---\nprompt\n")
	writeScannerTestFile(t, filepath.Join(agentRoot, ".agent", "subagents", "helper", ".agent", "AGENT.md"), "---\nid: agent-helper\nname: helper\nopcodes:\n  - thread/submit\n---\nhelper prompt\n")

	scanner := NewScanner("user", baseDir).WithNodeIndexBaseDir(baseDir)
	got := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if len(got) != 2 {
		t.Fatalf("ScanAgents() len = %d, want 2", len(got))
	}
	ids := make(map[string]*op.OpNode, len(got))
	for _, node := range got {
		ids[node.ID] = node
	}
	parent := ids["agent-coder"]
	if parent == nil {
		t.Fatalf("agent-coder missing from scan result: %#v", got)
	}
	meta, ok := parent.Meta.(*op.AgentMeta)
	if !ok || meta == nil {
		t.Fatalf("parent meta = %#v, want *op.AgentMeta", parent.Meta)
	}
	if len(meta.SubAgents) != 0 {
		t.Fatalf("parent subagents = %v, want none because private candidates are not auto-mounted", meta.SubAgents)
	}
	helper := ids["agent-helper"]
	if helper == nil {
		t.Fatalf("agent-helper missing from scan result: %#v", got)
	}
	if helper.Cwd != filepath.Join(agentRoot, ".agent", "subagents", "helper") {
		t.Fatalf("helper cwd = %q", helper.Cwd)
	}
}

func TestScanAgents_ParsesModelFrontmatterAsModelKey(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "agent-a", ".agent", "AGENT.md"), "---\nname: agent-a\nmodel: opagent:gpt-5.4\n---\nagent prompt\n")

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanAgents(baseDir, 0)
	if len(got) != 1 {
		t.Fatalf("ScanAgents() len = %d, want 1", len(got))
	}
	meta, ok := got[0].Meta.(*op.AgentMeta)
	if !ok || meta == nil {
		t.Fatalf("agent meta = %#v, want *op.AgentMeta", got[0].Meta)
	}
	if meta.Name != "agent-a" {
		t.Fatalf("agent name = %q, want agent-a", meta.Name)
	}
	if meta.Model != "opagent:gpt-5.4" {
		t.Fatalf("agent model = %q, want opagent:gpt-5.4", meta.Model)
	}
}

func TestScanAgents_RunLifecycleFieldIsRejected(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "agent-a", ".agent", "AGENT.md"), "---\nname: agent-a\nrun:\n  lifecycle: daemon\n---\nagent prompt\n")

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanAgents(baseDir, 0)
	if len(got) != 0 {
		t.Fatalf("ScanAgents() len = %d, want 0", len(got))
	}
}

func TestScanTools_RunScheduleFieldIsRejected(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "scheduled", "TOOL.md"), "---\nname: scheduled-tools\nrun:\n  schedule:\n    every: \"1h\"\n---\n")

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	if len(got) != 0 {
		t.Fatalf("ScanTools() len = %d, want 0", len(got))
	}
}

func TestScanTools_PreservesSystemTagAndRunEndpoint(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "rg-search", "TOOL.md"), `---
name: rg-search
description: system rg
tags: system
run:
  command: ["./rg-search"]
---
`)

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanTools() len = %d, want 1", len(got))
	}
	if !reflect.DeepEqual(got[0].Tags, []string{"system"}) {
		t.Fatalf("tool node tags = %v, want [system]", got[0].Tags)
	}
	meta, ok := got[0].Meta.(*op.ToolsMeta)
	if !ok || meta == nil {
		t.Fatalf("tool meta = %#v, want *op.ToolsMeta", got[0].Meta)
	}
	if len(got[0].Run.Command) != 1 || got[0].Run.Command[0] != filepath.Join(baseDir, "tools", "rg-search", "rg-search") {
		t.Fatalf("run command = %v, want resolved rg-search command", got[0].Run.Command)
	}
}

func TestScanTools_AcceptsSystemToolWithoutRun(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "tools", "local-bin", "TOOL.md"), `---
name: local-bin
description: Packaged local CLI
tags: system
---
`)

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanTools(filepath.Join(baseDir, "tools"), 0)
	if len(got) != 1 {
		t.Fatalf("ScanTools() len = %d, want 1", len(got))
	}
	meta, ok := got[0].Meta.(*op.ToolsMeta)
	if !ok || meta == nil {
		t.Fatalf("tool meta = %#v, want *op.ToolsMeta", got[0].Meta)
	}
	if !reflect.DeepEqual(got[0].Tags, []string{"system"}) {
		t.Fatalf("tool node tags = %v, want [system]", got[0].Tags)
	}
	if len(got[0].Run.Command) != 0 {
		t.Fatalf("run command = %v, want empty for system tool without run", got[0].Run.Command)
	}
}

func TestScanSkills_RunScheduleFieldIsRejected(t *testing.T) {
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	baseDir := t.TempDir()
	writeScannerTestFile(t, filepath.Join(baseDir, "skills", "scheduled", "SKILL.md"), "---\nname: Scheduled Skill\ndescription: test\nrun:\n  schedule:\n    time: \"09:00\"\n---\nbody\n")

	scanner := NewScanner("user", baseDir)
	got := scanner.ScanSkills(filepath.Join(baseDir, "skills"), 0)
	if len(got) != 0 {
		t.Fatalf("ScanSkills() len = %d, want 0", len(got))
	}
}

func writeScannerTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
