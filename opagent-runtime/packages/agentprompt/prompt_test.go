package agentprompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractMarkdownBody_StripsFrontmatter(t *testing.T) {
	got := ExtractMarkdownBody("---\nname: test\n---\nBody text\n")
	if got != "Body text" {
		t.Fatalf("ExtractMarkdownBody() = %q, want %q", got, "Body text")
	}
}

func TestExpandPlatformVariables(t *testing.T) {
	got := ExpandPlatformVariables("running on ${platform}", "linux")
	if got != "running on linux" {
		t.Fatalf("ExpandPlatformVariables() = %q, want %q", got, "running on linux")
	}
}

func TestExpandVariablesIncludesAgentPaths(t *testing.T) {
	got := ExpandVariables(
		"${platform} ${agentRoot} ${agentHome} ${cwd}",
		Variables{Platform: "linux", AgentRoot: "/tmp/agent", AgentHome: "/tmp/agent/.agent", CWD: "/tmp/workspace"},
	)
	want := "linux /tmp/agent /tmp/agent/.agent /tmp/workspace"
	if got != want {
		t.Fatalf("ExpandVariables() = %q, want %q", got, want)
	}
}

func TestBuildSystemPrompt_AppendsCwdAgentsAndSkills(t *testing.T) {
	available := []SkillContext{{
		ID:          "skill-search",
		Slug:        "search",
		Name:        "Search",
		Description: "Search docs",
		SkillFile:   "/tmp/search/SKILL.md",
		SkillDir:    "/tmp/search",
	}}
	selected := []SkillContext{{
		ID:          "skill-plan",
		Slug:        "plan",
		Name:        "Plan",
		Description: "Maintain plan files",
		SkillFile:   "/tmp/plan/SKILL.md",
		SkillDir:    "/tmp/plan",
	}}

	got := BuildSystemPrompt(
		"Base prompt.",
		"Project instructions",
		available,
		selected,
		map[string]any{"planFilePath": "/tmp/demo/.agent/context/thread-1.plan.md"},
	)

	for _, want := range []string{
		"Base prompt.",
		"## Working Directory AGENTS.md",
		"Project instructions",
		"## Available Skills",
		"## Selected Skills",
		"Before acting, use the read tool to read each selected skill's SKILL.md.",
		"Use the selected skills in addition to the base prompt.",
		"@skills/search",
		"@skills/plan",
		"planFilePath: /tmp/demo/.agent/context/thread-1.plan.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("BuildSystemPrompt() missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "this turn") {
		t.Fatalf("BuildSystemPrompt() should not mention this turn: %q", got)
	}
}

func TestBuildSystemPromptWithCwdAgentsPath_AppendsPathInstructionAndSkills(t *testing.T) {
	available := []SkillContext{{
		ID:          "skill-search",
		Slug:        "search",
		Name:        "Search",
		Description: "Search docs",
		SkillFile:   "/tmp/search/SKILL.md",
		SkillDir:    "/tmp/search",
	}}
	selected := []SkillContext{{
		ID:          "skill-plan",
		Slug:        "plan",
		Name:        "Plan",
		Description: "Maintain plan files",
		SkillFile:   "/tmp/plan/SKILL.md",
		SkillDir:    "/tmp/plan",
	}}

	got := BuildSystemPromptWithCwdAgentsPath(
		"Base prompt.",
		"/tmp/demo/AGENTS.md",
		available,
		selected,
		map[string]any{"planFilePath": "/tmp/demo/.agent/context/thread-1.plan.md"},
	)

	for _, want := range []string{
		"Base prompt.",
		"## Working Directory AGENTS.md",
		"If this thread has not already read the workspace AGENTS.md, first use the read tool to read: /tmp/demo/AGENTS.md",
		"## Available Skills",
		"## Selected Skills",
		"Before acting, use the read tool to read each selected skill's SKILL.md.",
		"Use the selected skills in addition to the base prompt.",
		"@skills/search",
		"@skills/plan",
		"planFilePath: /tmp/demo/.agent/context/thread-1.plan.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("BuildSystemPromptWithCwdAgentsPath() missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "this turn") {
		t.Fatalf("BuildSystemPromptWithCwdAgentsPath() should not mention this turn: %q", got)
	}
	if strings.Contains(got, "Project instructions") {
		t.Fatalf("BuildSystemPromptWithCwdAgentsPath() unexpectedly contains AGENTS body text: %q", got)
	}
}

func TestReadCwdAgentsContext_ReadsOnlyCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("MkdirAll(child): %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, "AGENTS.md"), []byte("parent"), 0o644); err != nil {
		t.Fatalf("WriteFile(parent): %v", err)
	}
	if err := os.WriteFile(filepath.Join(child, "AGENTS.md"), []byte("child"), 0o644); err != nil {
		t.Fatalf("WriteFile(child): %v", err)
	}

	got, err := ReadCwdAgentsContext(child)
	if err != nil {
		t.Fatalf("ReadCwdAgentsContext(): %v", err)
	}
	if got != "child" {
		t.Fatalf("ReadCwdAgentsContext() = %q, want %q", got, "child")
	}
}

func TestResolveCwdAgentsPath_UsesCurrentDirectoryOnly(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("MkdirAll(child): %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, "AGENTS.md"), []byte("parent"), 0o644); err != nil {
		t.Fatalf("WriteFile(parent): %v", err)
	}
	if err := os.WriteFile(filepath.Join(child, "AGENTS.md"), []byte("child"), 0o644); err != nil {
		t.Fatalf("WriteFile(child): %v", err)
	}

	got, err := ResolveCwdAgentsPath(child)
	if err != nil {
		t.Fatalf("ResolveCwdAgentsPath(): %v", err)
	}
	want := filepath.Join(child, "AGENTS.md")
	if got != want {
		t.Fatalf("ResolveCwdAgentsPath() = %q, want %q", got, want)
	}
}
