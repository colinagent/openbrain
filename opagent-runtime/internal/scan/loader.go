package scan

import (
	"fmt"
	"os"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

// ---------------------------------------------------------------------------
// ScanOptions
// ---------------------------------------------------------------------------

type ScanOptions struct {
	UID     string // resource uid (must be non-empty)
	BaseDir string // base directory for agents/tools/skills
}

// ---------------------------------------------------------------------------
// RefreshNodes — batch scan basedir/{agents,tools,skills} and cache all nodes.
// Called at startup and on manual rescan.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// ScanAndCache — scan a single directory for a specific kind and cache results.
// Used at runtime for workdir / @ reference resolution.
// ---------------------------------------------------------------------------

// func ScanAndCache(ctx context.Context, dir, kind, uid string) ([]*op.OpNode, error) {
// 	return ScanAndCacheWithBaseDir(ctx, dir, kind, uid, dir)
// }

// ScanAndCacheWithBaseDir scans a single directory for a specific kind and caches results.
// baseDir is used for resolving @agents/@tools/@skills references.
// func ScanAndCacheWithBaseDir(ctx context.Context, dir string, kind op.NodeKind, uid, baseDir string) ([]*op.OpNode, error) {

// 	s := NewScanner(uid, baseDir)
// 	var nodes []*op.OpNode
// 	switch kind {
// 	case op.NodeKindAgent:
// 		nodes = s.ScanAgents(dir, 0)
// 	case op.NodeKindSkill:
// 		nodes = s.ScanSkills(dir, 0)
// 	case op.NodeKindTools:
// 		nodes = s.ScanTools(dir, 0)
// 	default:
// 		return nil, fmt.Errorf("unknown kind: %s", kind)
// 	}

// 	for _, node := range nodes {
// 		if err := node.Upsert(ctx, node); err != nil {
// 			slog.Error("failed to upsert node", "error", err, "id", node.ID)
// 		}
// 	}
// 	return nodes, nil
// }

// ---------------------------------------------------------------------------
// LoadPromptByURI — load markdown body from a file:// URI on demand.
// Called at conversation start, not during scanning.
// ---------------------------------------------------------------------------

func LoadPromptByURI(uri string) (string, error) {
	path := op.URIToPath(uri)
	if path == "" {
		return "", fmt.Errorf("unsupported URI: %s", uri)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	_, body, ok := splitMarkdownFrontMatter(data)
	if !ok {
		// If no frontmatter, treat entire content as prompt
		return strings.TrimSpace(string(data)), nil
	}
	return body, nil
}

// ---------------------------------------------------------------------------
// NodeLoader implementation — used by nodestore cache-aside.
// ---------------------------------------------------------------------------

// type DefaultNodeLoader struct {
// 	UID string
// }

// func (l *DefaultNodeLoader) LoadByID(ctx context.Context, id string) (*op.OpNode, error) {
// 	_ = ctx
// 	_ = id
// 	_ = l
// 	return nil, fmt.Errorf("load by id is not supported without explicit id->uri index")
// }

// func loadSingleAgent(uid, cfgPath string) (*op.OpNode, error) {
// 	data, err := os.ReadFile(cfgPath)
// 	if err != nil {
// 		return nil, err
// 	}
// 	raw, tags, err := parseRawAgentConfig(data)
// 	if err != nil {
// 		return nil, err
// 	}
// 	workdir := filepath.Dir(cfgPath)
// 	ResolveRunPaths(workdir, &raw.Run)
// 	raw.Avatar = resolveAgentAvatarURI(workdir, raw.Avatar)

// 	meta := op.AgentMeta{
// 		Name:        raw.Name,
// 		Description: raw.Description,
// 		Avatar:      raw.Avatar,
// 		Model:       raw.Model,
// 		MaxToken:    raw.MaxToken,
// 		Run:         raw.Run,
// 		OpCodes:     raw.OpCodes,
// 	}
// 	uri := op.PathToURI(cfgPath)
// 	return node_cache.BuildNode(nodestore.KindAgent, uid, uri, tags, meta), nil
// }

// func loadSingleSkill(uid, skillMdPath string) (*op.OpNode, error) {
// 	data, err := os.ReadFile(skillMdPath)
// 	if err != nil {
// 		return nil, err
// 	}
// 	meta, err := parseRawSkillConfig(data)
// 	if err != nil {
// 		return nil, err
// 	}
// 	dir := filepath.Dir(skillMdPath)
// 	ResolveRunPaths(dir, &meta.Run)
// 	if err := meta.Run.Validate(); err != nil {
// 		return nil, fmt.Errorf("invalid run config in %s: %w", skillMdPath, err)
// 	}
// 	if meta.Name == "" {
// 		meta.Name = filepath.Base(dir)
// 	}
// 	uri := op.PathToURI(skillMdPath)
// 	return nodestore.BuildNode(nodestore.KindSkill, uid, uri, nil, *meta), nil
// }

// func loadSingleTools(uid, toolsMdPath string) (*op.OpNode, error) {
// 	data, err := os.ReadFile(toolsMdPath)
// 	if err != nil {
// 		return nil, err
// 	}
// 	meta, tags, err := parseRawToolsConfig(data, toolsMdPath)
// 	if err != nil {
// 		return nil, err
// 	}
// 	uri := op.PathToURI(toolsMdPath)
// 	return nodestore.BuildNode(nodestore.KindTools, uid, uri, tags, *meta), nil
// }
