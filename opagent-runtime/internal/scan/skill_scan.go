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

type rawSkillsConfig struct {
	ID          string
	Slug        string
	Name        string
	Description string
	Tags        []string
}

func (s *Scanner) parseAndBuildSkills(skillsMdPath string) *op.OpNode {
	if !fileExists(skillsMdPath) {
		return nil
	}
	data, err := os.ReadFile(skillsMdPath)
	if err != nil {
		return nil
	}
	meta, tags, id, err := parseRawSkillsConfig(data, skillsMdPath)
	if err != nil {
		if isManifestIDError(err) {
			s.recordErr(fmt.Errorf("%s: %w", skillsMdPath, err))
		}
		slog.Warn("failed to parse skill config", "path", skillsMdPath, "error", err)
		return nil
	}
	uri := op.PathToURI(skillsMdPath)
	system := config.GetSystem()
	n := op.BuildNode(s.uid, system.HostID, op.NodeKindSkill, uri, system.Env, tags, op.Run{}, nil, meta)
	if id != "" {
		n.ID = id
	}
	if n != nil {
		// For skills nodes, use the directory containing SKILL.md.
		n.Cwd = filepath.Dir(skillsMdPath)
	}
	return s.assignNodeID(n)
}

func parseRawSkillsConfig(data []byte, path string) (*op.SkillMeta, []string, string, error) {
	frontMatter, _, ok := splitMarkdownFrontMatter(data)
	if !ok {
		return nil, nil, "", fmt.Errorf("no valid YAML front matter")
	}
	var raw map[string]any
	if err := unmarshalFrontMatterWithBareAt(frontMatter, &raw); err != nil {
		return nil, nil, "", fmt.Errorf("parse front matter: %w", err)
	}

	slug := strings.TrimSpace(filepath.Base(filepath.Dir(path)))
	cfg := &rawSkillsConfig{
		ID:          getStringScalar(raw, "id"),
		Slug:        slug,
		Name:        getStringScalar(raw, "name"),
		Description: getString(raw, "description"),
		Tags:        getStringSlice(raw, "tags"),
	}
	if _, hasRun := raw["run"]; hasRun {
		if _, err := ParseRun(raw); err != nil {
			return nil, nil, "", err
		}
	}

	if cfg.Slug == "" {
		return nil, nil, "", fmt.Errorf("skill slug is required")
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, nil, "", fmt.Errorf("skill name is required")
	}
	if strings.TrimSpace(cfg.Description) == "" {
		return nil, nil, "", fmt.Errorf("skill description is required")
	}
	if err := validateManifestID(cfg.ID, op.NodeKindSkill); err != nil {
		return nil, nil, "", err
	}

	skillMeta := &op.SkillMeta{
		Slug:        cfg.Slug,
		Name:        cfg.Name,
		Description: cfg.Description,
		Tags:        append([]string(nil), cfg.Tags...),
	}

	return skillMeta, append([]string(nil), cfg.Tags...), strings.TrimSpace(cfg.ID), nil
}
