package core

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func TestNewAgent_ResolvesConfiguredSkillsWithoutInliningBody(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	agentPath := filepath.Join(baseDir, "agents", "agent-a", ".agent", "AGENT.md")
	skillPath := filepath.Join(baseDir, "skills", "search", "SKILL.md")

	writeAgentTestFile(t, agentPath, "---\nname: agent-a\n---\nBase agent prompt.\n")
	writeAgentTestFile(t, skillPath, "---\nname: Search\ndescription: Search docs\n---\nDetailed skill instructions should not be injected.\n")

	skillNode := op.BuildNode("user", "test-host", op.NodeKindSkill, op.PathToURI(skillPath), op.EnvLocal, nil, op.Run{}, nil, &op.SkillMeta{
		Slug:        "search",
		Name:        "Search",
		Description: "Search docs",
	})
	skillNode.Cwd = filepath.Dir(skillPath)
	cache.SetValue(skillNode.ID, cache.PrefixNode, *skillNode, cache.NoExpiration)

	agentNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(agentPath), op.EnvLocal, nil, op.Run{}, nil, &op.AgentMeta{
		Name:   "agent-a",
		Skills: []string{skillNode.ID},
	})

	agent, err := NewAgent(context.Background(), agentNode, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(): %v", err)
	}

	if !strings.Contains(agent.Sysprompt, "Base agent prompt.") {
		t.Fatalf("system prompt = %q, want base prompt content", agent.Sysprompt)
	}
	if strings.Contains(agent.Sysprompt, "Detailed skill instructions should not be injected.") {
		t.Fatalf("system prompt unexpectedly contains full skill body: %q", agent.Sysprompt)
	}
	if len(agent.AgentMeta.Skills) != 1 {
		t.Fatalf("len(agent skills) = %d, want 1", len(agent.AgentMeta.Skills))
	}
	if len(agent.AvailableSkills) != 1 {
		t.Fatalf("len(agent.AvailableSkills) = %d, want 1", len(agent.AvailableSkills))
	}
}

func TestNewAgent_DropsMissingSkillIDsFromPrompt(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	agentPath := filepath.Join(baseDir, "agents", "agent-a", ".agent", "AGENT.md")
	writeAgentTestFile(t, agentPath, "---\nname: agent-a\n---\nBase agent prompt.\n")

	agentNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(agentPath), op.EnvLocal, nil, op.Run{}, nil, &op.AgentMeta{
		Name:   "agent-a",
		Skills: []string{"skill-missing"},
	})

	agent, err := NewAgent(context.Background(), agentNode, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(): %v", err)
	}

	if len(agent.AgentMeta.Skills) != 0 {
		t.Fatalf("len(agent skills) = %d, want 0", len(agent.AgentMeta.Skills))
	}
	if len(agent.AvailableSkills) != 0 {
		t.Fatalf("len(agent.AvailableSkills) = %d, want 0", len(agent.AvailableSkills))
	}
}

func TestNewAgent_ResolvesThreadSubmitSubagentsAndInjectsAgentTask(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	parentPath := filepath.Join(baseDir, "agents", "parent", ".agent", "AGENT.md")
	gbrainPath := filepath.Join(baseDir, "agents", "gbrain", ".agent", "AGENT.md")
	promptOnlyPath := filepath.Join(baseDir, "agents", "prompt-only", ".agent", "AGENT.md")
	writeAgentTestFile(t, parentPath, "---\nname: parent\n---\nParent prompt.\n")
	writeAgentTestFile(t, gbrainPath, "---\nname: GBrain\n---\nBrain prompt.\n")
	writeAgentTestFile(t, promptOnlyPath, "---\nname: prompt-only\n---\nPrompt only.\n")

	brainNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(gbrainPath), op.EnvLocal, nil, op.Run{}, []op.OpCode{op.OpPromptGet, op.OpThreadSubmit}, &op.AgentMeta{
		Name:        "GBrain",
		Description: "Knowledge agent",
	})
	promptOnlyNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(promptOnlyPath), op.EnvLocal, nil, op.Run{}, []op.OpCode{op.OpPromptGet}, &op.AgentMeta{
		Name: "prompt-only",
	})
	cache.SetValue(brainNode.ID, cache.PrefixNode, *brainNode, cache.NoExpiration)
	cache.SetValue(promptOnlyNode.ID, cache.PrefixNode, *promptOnlyNode, cache.NoExpiration)

	parentNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(parentPath), op.EnvLocal, nil, op.Run{}, []op.OpCode{op.OpPromptGet, op.OpThreadSubmit}, &op.AgentMeta{
		Name:      "parent",
		SubAgents: []string{brainNode.ID, promptOnlyNode.ID},
	})

	agent, err := NewAgent(context.Background(), parentNode, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(): %v", err)
	}

	if got, want := len(agent.AvailableSubagents), 1; got != want {
		t.Fatalf("len(AvailableSubagents) = %d, want %d", got, want)
	}
	if got := agent.AvailableSubagents[0].ID; got != brainNode.ID {
		t.Fatalf("AvailableSubagents[0].ID = %q, want %q", got, brainNode.ID)
	}
	if agent.ToolSpecs[agentTaskToolName] == nil {
		t.Fatal("agent_task tool missing from assembled tool specs")
	}
	if !strings.Contains(agent.Sysprompt, "## Available Subagents") || !strings.Contains(agent.Sysprompt, brainNode.ID) {
		t.Fatalf("system prompt = %q, want available subagents appendix", agent.Sysprompt)
	}
	if !strings.Contains(agent.Sysprompt, "agent file: "+gbrainPath) {
		t.Fatalf("system prompt = %q, want subagent agent file path", agent.Sysprompt)
	}
	if !strings.Contains(agent.Sysprompt, "agentRoot: "+filepath.Join(baseDir, "agents", "gbrain")) {
		t.Fatalf("system prompt = %q, want subagent agentRoot", agent.Sysprompt)
	}
	if !strings.Contains(agent.Sysprompt, "agentHome: "+filepath.Join(baseDir, "agents", "gbrain", ".agent")) {
		t.Fatalf("system prompt = %q, want subagent agentHome", agent.Sysprompt)
	}
	if strings.Contains(agent.Sysprompt, promptOnlyNode.ID) {
		t.Fatalf("system prompt unexpectedly contains prompt-only subagent: %q", agent.Sysprompt)
	}
}

func TestNewAgentRespectsSystoolAllowlistForAgentTask(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	parentPath := filepath.Join(baseDir, "agents", "parent", ".agent", "AGENT.md")
	childPath := filepath.Join(baseDir, "agents", "child", ".agent", "AGENT.md")
	writeAgentTestFile(t, parentPath, "---\nname: parent\n---\nParent prompt.\n")
	writeAgentTestFile(t, childPath, "---\nname: child\n---\nChild prompt.\n")

	childNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(childPath), op.EnvLocal, nil, op.Run{}, []op.OpCode{op.OpThreadSubmit}, &op.AgentMeta{Name: "child"})
	cache.SetValue(childNode.ID, cache.PrefixNode, *childNode, cache.NoExpiration)

	parentNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(parentPath), op.EnvLocal, nil, op.Run{}, []op.OpCode{op.OpThreadSubmit}, &op.AgentMeta{
		Name:        "parent",
		SubAgents:   []string{childNode.ID},
		SysToolMode: op.SystoolModeAllowlist,
		SysTools:    []string{"read"},
	})

	agent, err := NewAgent(context.Background(), parentNode, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(): %v", err)
	}
	if agent.ToolSpecs[agentTaskToolName] != nil {
		t.Fatalf("agent_task mounted without allowlist entry")
	}

	parentNode.Meta = &op.AgentMeta{
		Name:        "parent",
		SubAgents:   []string{childNode.ID},
		SysToolMode: op.SystoolModeAllowlist,
		SysTools:    []string{agentTaskToolName},
	}
	agent, err = NewAgent(context.Background(), parentNode, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(with agent_task allowlist): %v", err)
	}
	if agent.ToolSpecs[agentTaskToolName] == nil {
		t.Fatalf("agent_task missing with allowlist entry")
	}
}

func TestNewAgent_ExpandsPromptVariablesForLocalAgentPrompt(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	agentDir := filepath.Join(baseDir, "agents", "agent-a")
	agentPath := filepath.Join(agentDir, ".agent", "AGENT.md")
	writeAgentTestFile(t, agentPath, "---\nname: agent-a\n---\nplatform=${platform}\nroot=${agentRoot}\nhome=${agentHome}\n")

	agentNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(agentPath), op.EnvLocal, nil, op.Run{}, nil, &op.AgentMeta{
		Name: "agent-a",
	})

	agent, err := NewAgent(context.Background(), agentNode, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(): %v", err)
	}
	if !strings.Contains(agent.Sysprompt, "platform=") {
		t.Fatalf("system prompt = %q, want expanded platform", agent.Sysprompt)
	}
	if !strings.Contains(agent.Sysprompt, "root="+agentDir) {
		t.Fatalf("system prompt = %q, want expanded agentRoot", agent.Sysprompt)
	}
	if !strings.Contains(agent.Sysprompt, "home="+filepath.Join(agentDir, ".agent")) {
		t.Fatalf("system prompt = %q, want expanded agentHome", agent.Sysprompt)
	}
	if strings.Contains(agent.Sysprompt, "${agentRoot}") || strings.Contains(agent.Sysprompt, "${agentHome}") {
		t.Fatalf("system prompt = %q, want variables expanded", agent.Sysprompt)
	}
}

func TestBuildAgentSystemPrompt_AppendsConfiguredAndSelectedSkillsWithoutDuplicates(t *testing.T) {
	baseDir := t.TempDir()
	searchPath := filepath.Join(baseDir, "skills", "search", "SKILL.md")
	planPath := filepath.Join(baseDir, "skills", "plan", "SKILL.md")

	search := op.BuildNode("user", "test-host", op.NodeKindSkill, op.PathToURI(searchPath), op.EnvLocal, nil, op.Run{}, nil, &op.SkillMeta{
		Slug:        "search",
		Name:        "Search",
		Description: "Search docs",
	})
	search.Cwd = filepath.Dir(searchPath)

	plan := op.BuildNode("user", "test-host", op.NodeKindSkill, op.PathToURI(planPath), op.EnvLocal, nil, op.Run{}, nil, &op.SkillMeta{
		Slug:        "plan",
		Name:        "Plan",
		Description: "Maintain plan files",
	})
	plan.Cwd = filepath.Dir(planPath)

	prompt := buildAgentSystemPrompt(
		"Base agent prompt.",
		[]op.OpNode{*search, *plan},
		[]op.OpNode{*plan},
		op.Meta{"planFilePath": "/tmp/demo/.agent/context/thread-1.plan.md"},
	)

	if !strings.Contains(prompt, "## Available Skills") {
		t.Fatalf("prompt = %q, want available skills appendix", prompt)
	}
	if !strings.Contains(prompt, "## Selected Skills") {
		t.Fatalf("prompt = %q, want selected skills appendix", prompt)
	}
	if strings.Count(prompt, "@skills/plan") != 1 {
		t.Fatalf("prompt = %q, want selected skill listed once", prompt)
	}
	if !strings.Contains(prompt, "@skills/search") {
		t.Fatalf("prompt = %q, want configured search skill", prompt)
	}
	if !strings.Contains(prompt, "planFilePath: /tmp/demo/.agent/context/thread-1.plan.md") {
		t.Fatalf("prompt = %q, want runtime context", prompt)
	}
}

func TestCurrentSystemPrompt_UsesFinalPromptWithoutAppendingSkills(t *testing.T) {
	baseDir := t.TempDir()
	searchPath := filepath.Join(baseDir, "skills", "search", "SKILL.md")
	search := op.BuildNode("user", "test-host", op.NodeKindSkill, op.PathToURI(searchPath), op.EnvLocal, nil, op.Run{}, nil, &op.SkillMeta{
		Slug:        "search",
		Name:        "Search",
		Description: "Search docs",
	})
	search.Cwd = filepath.Dir(searchPath)

	loop := &AgentLoop{
		Agent: &Agent{
			Sysprompt:       "Resolved final prompt.",
			PromptIsFinal:   true,
			AvailableSkills: []op.OpNode{*search},
		},
		SelectedSkillIDs: []string{search.ID},
	}

	if got := loop.currentSystemPrompt(); got != "Resolved final prompt." {
		t.Fatalf("currentSystemPrompt() = %q, want %q", got, "Resolved final prompt.")
	}
}

func TestCurrentSystemPrompt_AppendsGBrainSourceScope(t *testing.T) {
	loop := &AgentLoop{
		Agent: &Agent{
			AgentID:   "agent-gbrain",
			Sysprompt: "Base prompt.",
		},
		Meta: op.Meta{
			"gbrainQueryScope": op.Meta{
				"kind":     "source",
				"label":    "note",
				"sourceID": "ws-note",
			},
		},
	}

	got := loop.currentSystemPrompt()
	for _, want := range []string{
		"Base prompt.",
		"## OpenBrain GBrain Query Scope",
		`This turn was started from OpenBrain graph scope "note".`,
		`include source_id "ws-note"`,
		"Do not use search or unscoped query",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("currentSystemPrompt() = %q, want substring %q", got, want)
		}
	}
}

func TestCurrentSystemPrompt_IgnoresLegacyPublicBrainScope(t *testing.T) {
	loop := &AgentLoop{
		Agent: &Agent{
			AgentID:   "agent-gbrain",
			Sysprompt: "Base prompt.",
		},
		Meta: op.Meta{
			"gbrainQueryScope": op.Meta{
				"kind":  "publicBrain",
				"label": "OpenBrain's Brain",
				"sources": []op.Meta{
					{"sourceID": "ws-alpha", "name": "Alpha"},
					{"sourceID": "ws-beta"},
				},
			},
		},
	}

	if got := loop.currentSystemPrompt(); got != "Base prompt." {
		t.Fatalf("currentSystemPrompt() = %q, want legacy public-brain scope ignored", got)
	}
}

func TestCurrentSystemPrompt_DoesNotAppendGBrainScopeForOtherAgents(t *testing.T) {
	loop := &AgentLoop{
		Agent: &Agent{
			AgentID:   "agent-other",
			Sysprompt: "Base prompt.",
		},
		Meta: op.Meta{
			"gbrainQueryScope": op.Meta{
				"kind":     "source",
				"sourceID": "ws-note",
			},
		},
	}

	if got := loop.currentSystemPrompt(); got != "Base prompt." {
		t.Fatalf("currentSystemPrompt() = %q, want base prompt", got)
	}
}

func TestApplyGBrainQueryScopeToToolCallInjectsSingleSource(t *testing.T) {
	params, err := applyGBrainQueryScopeToToolCall(op.Meta{
		"gbrainQueryScope": op.Meta{
			"kind":     "source",
			"sourceID": "ws-note",
		},
	}, "tools-gbrain-cloud", "query", gbrainSourceIDToolSchema(), map[string]any{"query": "who am I"})
	if err != nil {
		t.Fatalf("applyGBrainQueryScopeToToolCall(): %v", err)
	}
	args, ok := params.(map[string]any)
	if !ok {
		t.Fatalf("params type = %T, want map", params)
	}
	if args["source_id"] != "ws-note" {
		t.Fatalf("source_id = %#v, want ws-note", args["source_id"])
	}
}

func TestApplyGBrainQueryScopeToToolCallInjectsSingleSourceWithoutSchema(t *testing.T) {
	params, err := applyGBrainQueryScopeToToolCall(op.Meta{
		"gbrainQueryScope": op.Meta{
			"kind":     "source",
			"sourceID": "ws-note",
		},
	}, "tools-gbrain-cloud", "query", nil, map[string]any{"query": "who am I"})
	if err != nil {
		t.Fatalf("applyGBrainQueryScopeToToolCall(): %v", err)
	}
	args, ok := params.(map[string]any)
	if !ok {
		t.Fatalf("params type = %T, want map", params)
	}
	if args["source_id"] != "ws-note" {
		t.Fatalf("source_id = %#v, want ws-note", args["source_id"])
	}
}

func TestApplyGBrainQueryScopeToToolCallRejectsOutsideSource(t *testing.T) {
	_, err := applyGBrainQueryScopeToToolCall(op.Meta{
		"gbrainQueryScope": op.Meta{
			"kind":     "source",
			"sourceID": "ws-note",
		},
	}, "tools-gbrain-cloud", "query", gbrainSourceIDToolSchema(), map[string]any{"query": "who am I", "source_id": "ws-other"})
	if err == nil || !strings.Contains(err.Error(), "outside the OpenBrain graph scope") {
		t.Fatalf("err = %v, want outside scope rejection", err)
	}
}

func TestApplyGBrainQueryScopeToToolCallIgnoresLegacyPublicBrainScope(t *testing.T) {
	input := map[string]any{"query": "architecture"}
	params, err := applyGBrainQueryScopeToToolCall(op.Meta{
		"gbrainQueryScope": op.Meta{
			"kind": "publicBrain",
			"sources": []op.Meta{
				{"sourceID": "ws-alpha"},
				{"sourceID": "ws-beta"},
			},
		},
	}, "tools-gbrain-cloud", "query", gbrainSourceIDToolSchema(), input)
	if err != nil || !reflect.DeepEqual(params, input) {
		t.Fatalf("params/err = %#v/%v, want unscoped input", params, err)
	}
}

func gbrainSourceIDToolSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":     map[string]any{"type": "string"},
			"source_id": map[string]any{"type": "string"},
		},
	}
}

func TestNewAgent_AssemblesSystemAndLocalToolServers(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	agentPath := filepath.Join(baseDir, "agents", "researcher", ".agent", "AGENT.md")
	writeAgentTestFile(t, agentPath, "---\nname: researcher\n---\nResearch prompt.\n")

	toolNode := op.BuildNode("user", "test-host", op.NodeKindTools, op.PathToURI(filepath.Join(baseDir, "agents", "researcher", "tools", "research-tools", "TOOL.md")), op.EnvLocal, nil, op.Run{}, nil, &op.ToolsMeta{
		Name: "research-tools",
		Tools: []*op.ToolSpec{
			{ServerID: "research-tools-node", Name: "web_search", Description: "Search"},
			{ServerID: "research-tools-node", Name: "browser_fetch", Description: "Fetch with browser"},
		},
	})
	cache.SetValue(toolNode.ID, cache.PrefixNode, *toolNode, cache.NoExpiration)

	agentNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(agentPath), op.EnvLocal, nil, op.Run{}, nil, &op.AgentMeta{
		Name:        "researcher",
		SysTools:    []string{"read", "shell"},
		SysToolMode: op.SystoolModeAllowlist,
		ToolServers: []string{toolNode.ID},
	})

	agent, err := NewAgent(context.Background(), agentNode, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(): %v", err)
	}

	if agent.ToolSpecs["read"] == nil {
		t.Fatal("read tool missing from assembled tool specs")
	}
	if agent.ToolSpecs["shell"] == nil {
		t.Fatal("shell tool missing from assembled tool specs")
	}
	if agent.ToolSpecs["web_search"] == nil {
		t.Fatal("web_search tool missing from assembled tool specs")
	}
	if agent.ToolSpecs["browser_fetch"] == nil {
		t.Fatal("browser_fetch tool missing from assembled tool specs")
	}
	if agent.ToolSpecs["write"] != nil || agent.ToolSpecs["edit"] != nil {
		t.Fatalf("undeclared systool tools leaked into model specs: %+v", agent.ToolSpecs)
	}
}

func TestNewAgent_IncludesSystemTaggedToolServersWhenMounted(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})

	baseDir := t.TempDir()
	agentPath := filepath.Join(baseDir, "agents", "agent-a", ".agent", "AGENT.md")
	writeAgentTestFile(t, agentPath, "---\nname: agent-a\n---\nBase agent prompt.\n")

	systemToolNode := op.BuildNode("user", "test-host", op.NodeKindTools, op.PathToURI(filepath.Join(baseDir, "tools", "rg-search", "TOOL.md")), op.EnvLocal, []string{"system"}, op.Run{}, nil, &op.ToolsMeta{
		Name: "rg-search",
		Tools: []*op.ToolSpec{
			{ServerID: "rg-search", Name: "rg_search", Description: "Search with rg"},
		},
	})
	cache.SetValue(systemToolNode.ID, cache.PrefixNode, *systemToolNode, cache.NoExpiration)

	agentNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(agentPath), op.EnvLocal, []string{"builtin"}, op.Run{}, nil, &op.AgentMeta{
		Name:        "agent-a",
		SysToolMode: op.SystoolModeDisabled,
		ToolServers: []string{systemToolNode.ID},
	})

	agent, err := NewAgent(context.Background(), agentNode, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(): %v", err)
	}
	if agent.ToolSpecs["rg_search"] == nil {
		t.Fatalf("rg_search tool missing from assembled tool specs: %+v", agent.ToolSpecs)
	}
}

func TestNewAgent_ExpandsCWDWhenLoadingPromptFromFile(t *testing.T) {
	baseDir := t.TempDir()
	agentPath := filepath.Join(baseDir, "agents", "coder", ".agent", "AGENT.md")
	writeAgentTestFile(t, agentPath, "---\nname: coder\n---\nCurrent working directory: ${cwd}\n")
	agentNode := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(agentPath), op.EnvLocal, nil, op.Run{}, nil, &op.AgentMeta{
		Name: "coder",
	})
	cwd := filepath.Join(baseDir, "workspace")

	agent, err := NewAgent(context.Background(), agentNode, op.Meta{"cwd": cwd})
	if err != nil {
		t.Fatalf("NewAgent(): %v", err)
	}
	if !strings.Contains(agent.Sysprompt, "Current working directory: "+cwd) || strings.Contains(agent.Sysprompt, "${cwd}") {
		t.Fatalf("system prompt did not expand cwd: %q", agent.Sysprompt)
	}

	if _, err := NewAgent(context.Background(), agentNode, op.Meta{}); err == nil || !strings.Contains(err.Error(), "meta.cwd") {
		t.Fatalf("NewAgent() missing cwd error = %v", err)
	}
}

func writeAgentTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
