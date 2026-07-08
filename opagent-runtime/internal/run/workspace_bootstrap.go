package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/nodeindex"
)

const workspaceBootstrapStateVersion = 2

type canonicalWorkspaceBootstrapState struct {
	Version      int                                    `json:"version"`
	Seeds        map[string]canonicalWorkspaceSeedState `json:"seeds,omitempty"`
	DeepResearch *canonicalWorkspaceSpecialDirState     `json:"deepResearch,omitempty"`
}

type canonicalWorkspaceSeedState struct {
	SeenPresent     bool `json:"seenPresent,omitempty"`
	CreatedBySystem bool `json:"createdBySystem,omitempty"`
	Suppressed      bool `json:"suppressed,omitempty"`
}

type canonicalWorkspaceSpecialDirState = canonicalWorkspaceSeedState

func shouldBootstrapLegacyCanonicalWorkspace() bool {
	value := strings.TrimSpace(os.Getenv("OPAGENT_ENABLE_LEGACY_WORKSPACE_BOOTSTRAP"))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

type workspaceSeedKind string

const (
	workspaceSeedDir       workspaceSeedKind = "dir"
	workspaceSeedFile      workspaceSeedKind = "file"
	workspaceSeedAgentBind workspaceSeedKind = "agentBind"
)

type workspaceSeed struct {
	Key     string
	Path    string
	Kind    workspaceSeedKind
	Content string
	AgentID string
}

func ensureCanonicalWorkspaceBootstrap(cfg *op.SystemConfig) error {
	if cfg == nil {
		return fmt.Errorf("system config is required")
	}
	baseDir := strings.TrimSpace(cfg.BaseDir)
	if baseDir == "" {
		return fmt.Errorf("system baseDir is required")
	}
	workspaceRoot := filepath.Join(baseDir, "workspace")
	state, err := loadCanonicalWorkspaceBootstrapState(baseDir)
	if err != nil {
		return err
	}
	state.Version = workspaceBootstrapStateVersion
	if state.Seeds == nil {
		state.Seeds = make(map[string]canonicalWorkspaceSeedState)
	}

	for _, seed := range canonicalWorkspaceSeeds(workspaceRoot) {
		if err := ensureCanonicalWorkspaceSeed(cfg, state, seed); err != nil {
			return err
		}
	}

	return saveCanonicalWorkspaceBootstrapState(baseDir, state)
}

func canonicalWorkspaceSeeds(workspaceRoot string) []workspaceSeed {
	return []workspaceSeed{
		{
			Key:     "workspace/AGENTS.md",
			Path:    filepath.Join(workspaceRoot, "AGENTS.md"),
			Kind:    workspaceSeedFile,
			Content: defaultWorkspaceAgentsMarkdown(),
		},
		{
			Key:     "workspace/index.md",
			Path:    filepath.Join(workspaceRoot, "index.md"),
			Kind:    workspaceSeedFile,
			Content: defaultWorkspaceIndexMarkdown(),
		},
		{
			Key:  "workspace/raw",
			Path: filepath.Join(workspaceRoot, "raw"),
			Kind: workspaceSeedDir,
		},
		{
			Key:  "workspace/wiki",
			Path: filepath.Join(workspaceRoot, "wiki"),
			Kind: workspaceSeedDir,
		},
		{
			Key:     "workspace/.agent/AGENT.md",
			Path:    filepath.Join(workspaceRoot, ".agent", "AGENT.md"),
			Kind:    workspaceSeedAgentBind,
			AgentID: "opagent",
		},
		{
			Key:  "workspace/research",
			Path: filepath.Join(workspaceRoot, "research"),
			Kind: workspaceSeedDir,
		},
		{
			Key:     "workspace/research/.agent/AGENT.md",
			Path:    filepath.Join(workspaceRoot, "research", ".agent", "AGENT.md"),
			Kind:    workspaceSeedAgentBind,
			AgentID: "researcher",
		},
	}
}

func ensureCanonicalWorkspaceSeed(cfg *op.SystemConfig, state *canonicalWorkspaceBootstrapState, seed workspaceSeed) error {
	if state == nil {
		return fmt.Errorf("workspace bootstrap state is required")
	}
	if state.Seeds == nil {
		state.Seeds = make(map[string]canonicalWorkspaceSeedState)
	}
	key := strings.TrimSpace(seed.Key)
	if key == "" {
		return fmt.Errorf("workspace seed key is required")
	}
	path := strings.TrimSpace(seed.Path)
	if path == "" {
		return fmt.Errorf("workspace seed path is required")
	}

	seedState := state.Seeds[key]
	exists, err := pathExists(path)
	if err != nil {
		return err
	}
	if exists {
		seedState.SeenPresent = true
		state.Seeds[key] = seedState
		return nil
	}
	if seedState.Suppressed {
		state.Seeds[key] = seedState
		return nil
	}
	if seedState.SeenPresent || seedState.CreatedBySystem {
		seedState.Suppressed = true
		state.Seeds[key] = seedState
		return nil
	}

	created, err := createCanonicalWorkspaceSeed(cfg, seed)
	if err != nil {
		return err
	}
	if !created {
		delete(state.Seeds, key)
		return nil
	}
	seedState.SeenPresent = true
	seedState.CreatedBySystem = true
	state.Seeds[key] = seedState
	return nil
}

func createCanonicalWorkspaceSeed(cfg *op.SystemConfig, seed workspaceSeed) (bool, error) {
	switch seed.Kind {
	case workspaceSeedDir:
		if err := os.MkdirAll(seed.Path, 0o755); err != nil {
			return false, err
		}
		return true, nil
	case workspaceSeedFile:
		return writeSeedFile(seed.Path, seed.Content)
	case workspaceSeedAgentBind:
		targetDir := filepath.Dir(filepath.Dir(seed.Path))
		exists, err := dirExists(targetDir)
		if err != nil || !exists {
			return false, err
		}
		agentInstallRoot := canonicalInstalledAgentRoot(cfg, seed.AgentID)
		if !directoryHasAgentConfig(agentInstallRoot) {
			return false, nil
		}
		content := buildBoundAgentReferenceMarkdown(canonicalInstalledAgentID(cfg, seed.AgentID))
		if content == "" {
			return false, nil
		}
		return writeSeedFile(seed.Path, content)
	default:
		return false, fmt.Errorf("unknown workspace seed kind %q", seed.Kind)
	}
}

func writeSeedFile(path string, content string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, err
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	return true, nil
}

func defaultWorkspaceAgentsMarkdown() string {
	return "# Workspace Instructions\n\n" +
		"This workspace is a small, text-first LLM Wiki.\n\n" +
		"## Structure\n" +
		"- `raw/`: source material. Treat it as read-only unless the user explicitly asks to edit it.\n" +
		"- `wiki/`: maintained knowledge derived from sources and conversations.\n" +
		"- `research/`: research work area bound to the researcher agent.\n" +
		"- `index.md`: current navigation and status. Update it when durable wiki pages are created, renamed, or removed.\n" +
		"- `.agent/`: OpAgent internal state; do not use it as knowledge storage.\n\n" +
		"## Workflow\n" +
		"- Read `index.md` before broad questions.\n" +
		"- Put new source material in `raw/`; write durable knowledge in `wiki/`.\n" +
		"- Keep `wiki/` flat until there is clear pressure to create subdirectories.\n" +
		"- Cite source paths or links for important claims.\n" +
		"- Mark unsupported conclusions as inference.\n" +
		"- Do not create `log.md`; use `.agent/chat` if process history is needed.\n" +
		"- Personal, research, business/team, and reading work are modes, not required directories.\n"
}

func defaultWorkspaceIndexMarkdown() string {
	return "# Index\n\n" +
		"## Wiki\n\n" +
		"No wiki pages yet.\n\n" +
		"## Sources\n\n" +
		"Add source material to `raw/`.\n\n" +
		"## Research\n\n" +
		"Research workspace: [research/](research/).\n"
}

func canonicalInstalledAgentRoot(cfg *op.SystemConfig, agentID string) string {
	return filepath.Join(strings.TrimSpace(cfg.BaseDir), "agents", strings.TrimSpace(agentID))
}

func canonicalInstalledAgentID(cfg *op.SystemConfig, agentID string) string {
	agentPath := filepath.Join(canonicalInstalledAgentRoot(cfg, agentID), ".agent", "AGENT.md")
	node := op.BuildNode(op.LocalUser, strings.TrimSpace(cfg.HostID), op.NodeKindAgent, op.PathToURI(agentPath), strings.TrimSpace(cfg.Env), nil, op.Run{}, nil, &op.AgentMeta{})
	node.Cwd = canonicalInstalledAgentRoot(cfg, agentID)
	if idx, err := nodeindex.Open(strings.TrimSpace(cfg.BaseDir)); err == nil {
		_ = idx.Assign(node)
	}
	return strings.TrimSpace(node.ID)
}

func buildBoundAgentReferenceMarkdown(agentID string) string {
	nodeID, err := normalizeAgentBindNodeID(agentID)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("---\nbind: @%s\n---\n", nodeID)
}

func normalizeAgentBindNodeID(agentID string) (string, error) {
	nodeID := strings.TrimSpace(agentID)
	if strings.HasPrefix(nodeID, "@") {
		nodeID = strings.TrimSpace(strings.TrimPrefix(nodeID, "@"))
	}
	if kind, ok := op.NodeKindFromID(nodeID); !ok || kind != op.NodeKindAgent {
		return "", fmt.Errorf("agent node id is required")
	}
	return nodeID, nil
}

func directoryHasAgentConfig(dir string) bool {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(trimmed, ".agent", "AGENT.md")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(trimmed, ".agents", "AGENTS.md")); err == nil {
		return true
	}
	return false
}

func dirExists(dir string) (bool, error) {
	info, err := os.Stat(strings.TrimSpace(dir))
	if err == nil {
		return info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(strings.TrimSpace(path))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func canonicalWorkspaceBootstrapStatePath(baseDir string) string {
	return filepath.Join(strings.TrimSpace(baseDir), "configs", "system", "workspace-bootstrap.json")
}

func loadCanonicalWorkspaceBootstrapState(baseDir string) (*canonicalWorkspaceBootstrapState, error) {
	path := canonicalWorkspaceBootstrapStatePath(baseDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &canonicalWorkspaceBootstrapState{Version: workspaceBootstrapStateVersion}, nil
		}
		return nil, err
	}
	var state canonicalWorkspaceBootstrapState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Version == 0 {
		state.Version = workspaceBootstrapStateVersion
	}
	return &state, nil
}

func saveCanonicalWorkspaceBootstrapState(baseDir string, state *canonicalWorkspaceBootstrapState) error {
	if state == nil {
		return fmt.Errorf("workspace bootstrap state is required")
	}
	state.Version = workspaceBootstrapStateVersion
	return writeJSONAtomic(canonicalWorkspaceBootstrapStatePath(baseDir), state)
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tempPath := path + fmt.Sprintf(".%d.tmp", time.Now().UnixNano())
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tempPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
