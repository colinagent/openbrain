package scan

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

type rawToolsConfig struct {
	ID          string
	Name        string
	Description string
	tags        []string
	Run         op.Run
}

func (s *Scanner) parseAndBuildTools(toolsMdPath string) *op.OpNode {
	if !fileExists(toolsMdPath) {
		return nil
	}
	data, err := os.ReadFile(toolsMdPath)
	if err != nil {
		return nil
	}
	meta, tags, run, id, err := parseRawToolsConfig(s.uid, data, toolsMdPath)
	if err != nil {
		if isManifestIDError(err) {
			s.recordErr(fmt.Errorf("%s: %w", toolsMdPath, err))
		}
		slog.Warn("failed to parse tools config", "path", toolsMdPath, "error", err)
		return nil
	}
	uri := op.PathToURI(toolsMdPath)
	system := config.GetSystem()
	n := op.BuildNode(s.uid, system.HostID, op.NodeKindTools, uri, system.Env, tags, run, nil, meta)
	if id != "" {
		n.ID = id
	}
	if n != nil {
		// For tools nodes, use the directory containing TOOL.md.
		n.Cwd = filepath.Dir(toolsMdPath)
	}
	return s.assignNodeID(n)
}

func parseRawToolsConfig(uid string, data []byte, path string) (*op.ToolsMeta, []string, op.Run, string, error) {
	frontMatter, _, ok := splitMarkdownFrontMatter(data)
	if !ok {
		return nil, nil, op.Run{}, "", fmt.Errorf("no valid YAML front matter")
	}
	var raw map[string]any
	if err := unmarshalFrontMatterWithBareAt(frontMatter, &raw); err != nil {
		return nil, nil, op.Run{}, "", fmt.Errorf("parse front matter: %w", err)
	}

	cfg := &rawToolsConfig{
		ID:          getStringScalar(raw, "id"),
		Name:        getStringScalar(raw, "name"),
		Description: mergeDescription(getString(raw, "description"), getString(raw, "bio")),
		tags:        getStringSlice(raw, "tags"),
	}

	run, err := ParseRun(raw)
	if err != nil {
		return nil, nil, op.Run{}, "", err
	}
	cfg.Run = run
	if err := validateManifestID(cfg.ID, op.NodeKindTools); err != nil {
		return nil, nil, op.Run{}, "", err
	}

	// run.command 中的相对路径解析
	baseDir := filepath.Dir(path)
	ResolveRunPaths(baseDir, &run)

	toolsMeta := &op.ToolsMeta{
		Name:        cfg.Name,
		Description: cfg.Description,
	}

	return toolsMeta, append([]string(nil), cfg.tags...), run, strings.TrimSpace(cfg.ID), nil
}
