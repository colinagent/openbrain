package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestShouldBootstrapLegacyCanonicalWorkspaceDefaultOff(t *testing.T) {
	t.Setenv("OPAGENT_ENABLE_LEGACY_WORKSPACE_BOOTSTRAP", "")
	if shouldBootstrapLegacyCanonicalWorkspace() {
		t.Fatalf("legacy workspace bootstrap should default to off")
	}
	t.Setenv("OPAGENT_ENABLE_LEGACY_WORKSPACE_BOOTSTRAP", "true")
	if !shouldBootstrapLegacyCanonicalWorkspace() {
		t.Fatalf("legacy workspace bootstrap should be enabled by explicit opt-in")
	}
}

func TestEnsureDefaultConversationWorkspace(t *testing.T) {
	baseDir := t.TempDir()
	workspaceRoot, err := ensureDefaultConversationWorkspace(&op.SystemConfig{BaseDir: baseDir})
	if err != nil {
		t.Fatalf("ensureDefaultConversationWorkspace(): %v", err)
	}
	want := filepath.Join(baseDir, "workspace")
	if workspaceRoot != want {
		t.Fatalf("workspaceRoot = %q, want %q", workspaceRoot, want)
	}
	info, err := os.Stat(workspaceRoot)
	if err != nil {
		t.Fatalf("stat workspace: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("workspaceRoot is not a directory: %s", workspaceRoot)
	}
}

func TestEnsureCanonicalWorkspaceBootstrapSeedsDefaultWorkspace(t *testing.T) {
	baseDir := t.TempDir()
	cfg := &op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal}

	installBootstrapAgent(t, baseDir, "opagent")
	installBootstrapAgent(t, baseDir, "researcher")

	if err := ensureCanonicalWorkspaceBootstrap(cfg); err != nil {
		t.Fatalf("ensureCanonicalWorkspaceBootstrap(): %v", err)
	}

	workspaceRoot := filepath.Join(baseDir, "workspace")
	for _, path := range []string{
		filepath.Join(workspaceRoot, "AGENTS.md"),
		filepath.Join(workspaceRoot, "index.md"),
		filepath.Join(workspaceRoot, "raw"),
		filepath.Join(workspaceRoot, "wiki"),
		filepath.Join(workspaceRoot, "research"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected seeded path %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "deep-research")); !os.IsNotExist(err) {
		t.Fatalf("deep-research should not be created, stat err = %v", err)
	}

	agentsData, err := os.ReadFile(filepath.Join(workspaceRoot, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agentsData), "LLM Wiki") {
		t.Fatalf("AGENTS.md should describe LLM Wiki defaults, got %q", string(agentsData))
	}
	if !strings.Contains(string(agentsData), "Do not create `log.md`") {
		t.Fatalf("AGENTS.md should explicitly avoid log.md, got %q", string(agentsData))
	}

	rootAgentPath := filepath.Join(workspaceRoot, ".agent", "AGENT.md")
	rootData, err := os.ReadFile(rootAgentPath)
	if err != nil {
		t.Fatalf("read root agent bind: %v", err)
	}
	wantRoot := buildBoundAgentReferenceMarkdown(canonicalInstalledAgentID(cfg, "opagent"))
	if string(rootData) != wantRoot {
		t.Fatalf("root bind = %q, want %q", string(rootData), wantRoot)
	}

	researchAgentPath := filepath.Join(workspaceRoot, "research", ".agent", "AGENT.md")
	researchData, err := os.ReadFile(researchAgentPath)
	if err != nil {
		t.Fatalf("read research bind: %v", err)
	}
	wantResearch := buildBoundAgentReferenceMarkdown(canonicalInstalledAgentID(cfg, "researcher"))
	if string(researchData) != wantResearch {
		t.Fatalf("research bind = %q, want %q", string(researchData), wantResearch)
	}

	state, err := loadCanonicalWorkspaceBootstrapState(baseDir)
	if err != nil {
		t.Fatalf("loadCanonicalWorkspaceBootstrapState(): %v", err)
	}
	for _, key := range []string{
		"workspace/AGENTS.md",
		"workspace/index.md",
		"workspace/raw",
		"workspace/wiki",
		"workspace/.agent/AGENT.md",
		"workspace/research",
		"workspace/research/.agent/AGENT.md",
	} {
		assertBootstrapSeedCreated(t, state, key)
	}
}

func TestBuildBoundAgentReferenceMarkdownRejectsLegacyNodeKeys(t *testing.T) {
	if got := buildBoundAgentReferenceMarkdown("@local:host:agent:file:///tmp/.agent/AGENT.md"); got != "" {
		t.Fatalf("legacy bind markdown = %q, want empty", got)
	}
	if _, err := normalizeAgentBindNodeID("@agent-demo"); err != nil {
		t.Fatalf("normalizeAgentBindNodeID(@agent-demo): %v", err)
	}
}

func TestEnsureCanonicalWorkspaceBootstrapPreservesExistingSeedFiles(t *testing.T) {
	baseDir := t.TempDir()
	cfg := &op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal}
	workspaceRoot := filepath.Join(baseDir, "workspace")

	writeBootstrapTestFile(t, filepath.Join(workspaceRoot, "AGENTS.md"), "# Custom\n")
	writeBootstrapTestFile(t, filepath.Join(workspaceRoot, "index.md"), "# Custom Index\n")
	installBootstrapAgent(t, baseDir, "opagent")
	installBootstrapAgent(t, baseDir, "researcher")

	if err := ensureCanonicalWorkspaceBootstrap(cfg); err != nil {
		t.Fatalf("ensureCanonicalWorkspaceBootstrap(): %v", err)
	}

	assertBootstrapFileContent(t, filepath.Join(workspaceRoot, "AGENTS.md"), "# Custom\n")
	assertBootstrapFileContent(t, filepath.Join(workspaceRoot, "index.md"), "# Custom Index\n")

	state, err := loadCanonicalWorkspaceBootstrapState(baseDir)
	if err != nil {
		t.Fatalf("loadCanonicalWorkspaceBootstrapState(): %v", err)
	}
	for _, key := range []string{"workspace/AGENTS.md", "workspace/index.md"} {
		seed := state.Seeds[key]
		if !seed.SeenPresent {
			t.Fatalf("%s SeenPresent = false, want true", key)
		}
		if seed.CreatedBySystem {
			t.Fatalf("%s CreatedBySystem = true, want false for preexisting file", key)
		}
	}
}

func TestEnsureCanonicalWorkspaceBootstrapSuppressesDeletedSeeds(t *testing.T) {
	baseDir := t.TempDir()
	cfg := &op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal}
	workspaceRoot := filepath.Join(baseDir, "workspace")

	installBootstrapAgent(t, baseDir, "opagent")
	installBootstrapAgent(t, baseDir, "researcher")

	if err := ensureCanonicalWorkspaceBootstrap(cfg); err != nil {
		t.Fatalf("initial ensureCanonicalWorkspaceBootstrap(): %v", err)
	}
	for _, path := range []string{
		filepath.Join(workspaceRoot, "AGENTS.md"),
		filepath.Join(workspaceRoot, "raw"),
		filepath.Join(workspaceRoot, "research", ".agent", "AGENT.md"),
	} {
		if err := os.RemoveAll(path); err != nil {
			t.Fatalf("RemoveAll(%s): %v", path, err)
		}
	}

	if err := ensureCanonicalWorkspaceBootstrap(cfg); err != nil {
		t.Fatalf("second ensureCanonicalWorkspaceBootstrap(): %v", err)
	}

	for _, path := range []string{
		filepath.Join(workspaceRoot, "AGENTS.md"),
		filepath.Join(workspaceRoot, "raw"),
		filepath.Join(workspaceRoot, "research", ".agent", "AGENT.md"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should remain deleted, stat err = %v", path, err)
		}
	}

	state := readBootstrapStateFile(t, baseDir)
	for _, key := range []string{
		"workspace/AGENTS.md",
		"workspace/raw",
		"workspace/research/.agent/AGENT.md",
	} {
		if !state.Seeds[key].Suppressed {
			t.Fatalf("%s Suppressed = false, want true", key)
		}
	}
}

func TestEnsureCanonicalWorkspaceBootstrapSkipsMissingResearcherUntilInstalled(t *testing.T) {
	baseDir := t.TempDir()
	cfg := &op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal}
	workspaceRoot := filepath.Join(baseDir, "workspace")

	installBootstrapAgent(t, baseDir, "opagent")

	if err := ensureCanonicalWorkspaceBootstrap(cfg); err != nil {
		t.Fatalf("ensureCanonicalWorkspaceBootstrap(): %v", err)
	}

	if _, err := os.Stat(filepath.Join(workspaceRoot, "research")); err != nil {
		t.Fatalf("research dir should exist even when researcher is missing, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".agent", "AGENT.md")); err != nil {
		t.Fatalf("root bind should exist, stat err = %v", err)
	}
	researchAgentPath := filepath.Join(workspaceRoot, "research", ".agent", "AGENT.md")
	if _, err := os.Stat(researchAgentPath); !os.IsNotExist(err) {
		t.Fatalf("research bind should be absent when researcher is missing, stat err = %v", err)
	}

	installBootstrapAgent(t, baseDir, "researcher")
	if err := ensureCanonicalWorkspaceBootstrap(cfg); err != nil {
		t.Fatalf("ensureCanonicalWorkspaceBootstrap() after researcher install: %v", err)
	}
	if _, err := os.Stat(researchAgentPath); err != nil {
		t.Fatalf("research bind should be created after researcher install, stat err = %v", err)
	}
}

func TestEnsureCanonicalWorkspaceBootstrapSkipsMissingOpagentUntilInstalled(t *testing.T) {
	baseDir := t.TempDir()
	cfg := &op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal}
	workspaceRoot := filepath.Join(baseDir, "workspace")

	if err := ensureCanonicalWorkspaceBootstrap(cfg); err != nil {
		t.Fatalf("ensureCanonicalWorkspaceBootstrap(): %v", err)
	}

	rootAgentPath := filepath.Join(workspaceRoot, ".agent", "AGENT.md")
	if _, err := os.Stat(rootAgentPath); !os.IsNotExist(err) {
		t.Fatalf("root bind should be absent when opagent is missing, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS.md should still be created, stat err = %v", err)
	}
}

func TestEnsureCanonicalWorkspaceBootstrapLeavesExistingDeepResearchAlone(t *testing.T) {
	baseDir := t.TempDir()
	cfg := &op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal}
	deepResearchFile := filepath.Join(baseDir, "workspace", "deep-research", "notes.md")
	writeBootstrapTestFile(t, deepResearchFile, "# Notes\n")

	if err := ensureCanonicalWorkspaceBootstrap(cfg); err != nil {
		t.Fatalf("ensureCanonicalWorkspaceBootstrap(): %v", err)
	}

	assertBootstrapFileContent(t, deepResearchFile, "# Notes\n")
	if _, err := os.Stat(filepath.Join(baseDir, "workspace", "deep-research", ".agent", "AGENT.md")); !os.IsNotExist(err) {
		t.Fatalf("deep-research should not be modified or bound, stat err = %v", err)
	}
}

func installBootstrapAgent(t *testing.T, baseDir string, agentID string) {
	t.Helper()
	writeBootstrapTestFile(t, filepath.Join(baseDir, "agents", agentID, ".agent", "AGENT.md"), "---\nname: "+agentID+"\n---\n")
}

func writeBootstrapTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func assertBootstrapFileContent(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}

func assertBootstrapSeedCreated(t *testing.T, state *canonicalWorkspaceBootstrapState, key string) {
	t.Helper()
	seed, ok := state.Seeds[key]
	if !ok {
		t.Fatalf("state missing seed %s", key)
	}
	if !seed.SeenPresent {
		t.Fatalf("%s SeenPresent = false, want true", key)
	}
	if !seed.CreatedBySystem {
		t.Fatalf("%s CreatedBySystem = false, want true", key)
	}
	if seed.Suppressed {
		t.Fatalf("%s Suppressed = true, want false", key)
	}
}

func readBootstrapStateFile(t *testing.T, baseDir string) canonicalWorkspaceBootstrapState {
	t.Helper()
	data, err := os.ReadFile(canonicalWorkspaceBootstrapStatePath(baseDir))
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	var state canonicalWorkspaceBootstrapState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	return state
}
