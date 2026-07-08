package scan

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

type rawAgentConfig struct {
	ID           string
	Name         string
	Description  string
	Avatar       string
	MaxToken     int64
	Model        string
	Run          op.Run
	Tags         []string
	BindEntry    string
	OpCodes      []op.OpCode
	ToolEntries  []string // raw refs: "shell", "./tools", "@tools/browser"
	SysToolMode  string
	SkillEntries []string
	AgentEntries []string
}

// ---------------------------------------------------------------------------
// Parse-and-build methods
// ---------------------------------------------------------------------------

func (s *Scanner) parseAndBuildAgent(cfgPath, cwd string) *op.OpNode {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil
	}
	raw, err := parseRawAgentConfig(data)
	if err != nil {
		if isManifestIDError(err) {
			s.recordErr(fmt.Errorf("%s: %w", cfgPath, err))
		}
		slog.Warn("failed to parse agent config", "path", cfgPath, "error", err)
		return nil
	}

	resourceRoot := agentResourceRootDir(cfgPath, cwd)
	ResolveRunPaths(resourceRoot, &raw.Run)
	raw.Avatar = resolveAgentAvatarURI(resourceRoot, raw.Avatar)

	if raw.BindEntry != "" {
		node := s.resolveBindAgent(raw.BindEntry, cwd)
		if node == nil {
			slog.Warn("failed to resolve bind target agent node", "path", cfgPath, "bind", raw.BindEntry)
			return nil
		}
		return node
	}

	toolServers, sysTools := s.resolveRefs(raw.ToolEntries, op.NodeKindTools, resourceRoot, cfgPath)
	skillIDs := s.resolveSkillRefs(raw.SkillEntries, resourceRoot)
	agentIDs, _ := s.resolveRefs(raw.AgentEntries, op.NodeKindAgent, resourceRoot, cfgPath)
	if raw.SysToolMode == op.SystoolModeDisabled && len(sysTools) > 0 {
		s.recordErr(fmt.Errorf("%s: @systool: null conflicts with explicit systool tools: %s", cfgPath, strings.Join(sysTools, ", ")))
		return nil
	}

	meta := op.AgentMeta{
		Name:        raw.Name,
		Description: raw.Description,
		Avatar:      raw.Avatar,
		MaxToken:    raw.MaxToken,
		Model:       raw.Model,
		ToolServers: toolServers,
		SysTools:    sysTools,
		SysToolMode: raw.SysToolMode,
		Skills:      skillIDs,
		SubAgents:   agentIDs,
	}

	uri := op.PathToURI(cfgPath)
	system := config.GetSystem()
	n := op.BuildNode(s.uid, system.HostID, op.NodeKindAgent, uri, system.Env, raw.Tags, raw.Run, raw.OpCodes, &meta)
	if raw.ID != "" {
		n.ID = raw.ID
	}
	if n != nil {
		n.Cwd = cwd
	}
	return s.assignNodeID(n)
}

func parseRawAgentConfig(data []byte) (*rawAgentConfig, error) {
	frontMatter, _, ok := splitMarkdownFrontMatter(data)
	if !ok {
		return nil, fmt.Errorf("no valid YAML front matter")
	}
	var raw map[string]any
	if err := unmarshalFrontMatterWithBareAt(frontMatter, &raw); err != nil {
		return nil, fmt.Errorf("parse front matter: %w", err)
	}
	cfg := &rawAgentConfig{
		ID:           getStringScalar(raw, "id"),
		Name:         getStringScalar(raw, "name"),
		Description:  mergeDescription(getString(raw, "description"), getString(raw, "bio")),
		Avatar:       getString(raw, "avatar"),
		MaxToken:     getInt64(raw, "maxToken"),
		Model:        getString(raw, "model"),
		BindEntry:    getString(raw, "bind"),
		Tags:         getStringSlice(raw, "tags"),
		ToolEntries:  getStringSlice(raw, "tools"),
		SysToolMode:  parseSystoolMode(raw),
		SkillEntries: getStringSlice(raw, "skills"),
		AgentEntries: getStringSlice(raw, "subagents"),
	}
	run, err := ParseRun(raw)
	if err != nil {
		return nil, err
	}
	cfg.Run = run
	if strings.TrimSpace(cfg.Name) == "" && strings.TrimSpace(cfg.BindEntry) == "" {
		return nil, fmt.Errorf("agent name or bind is required")
	}
	if err := validateManifestID(cfg.ID, op.NodeKindAgent); err != nil {
		return nil, err
	}
	for _, code := range getStringSlice(raw, "opcodes") {
		cfg.OpCodes = append(cfg.OpCodes, op.OpCode(code))
	}
	return cfg, nil
}

func parseSystoolMode(raw map[string]any) string {
	if value, exists := raw["@systool"]; exists && value == nil {
		return op.SystoolModeDisabled
	}
	if _, exists := raw["tools"]; exists {
		return op.SystoolModeAllowlist
	}
	return op.SystoolModeDefault
}
