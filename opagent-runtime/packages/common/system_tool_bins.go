package common

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const ToolManifestName = "TOOL.md"

type toolSystemManifest struct {
	Tags []string
}

func ProjectSystemToolBins(baseDir string) error {
	toolsRoot := filepath.Join(baseDir, "tools")
	entries, err := os.ReadDir(toolsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := SyncToolBinProjection(baseDir, filepath.Join(toolsRoot, entry.Name()), nil); err != nil {
			return err
		}
	}
	return nil
}

func SyncToolBinProjection(baseDir string, toolRoot string, previousPaths []string) ([]string, error) {
	manifest, err := loadToolSystemManifest(filepath.Join(toolRoot, ToolManifestName))
	if err != nil {
		return nil, err
	}
	if !manifest.hasTag("system") {
		if err := removeProjectedPaths(previousPaths); err != nil {
			return nil, err
		}
		return nil, nil
	}

	sourceBinDir := filepath.Join(toolRoot, "bin")
	entries, err := os.ReadDir(sourceBinDir)
	if err != nil {
		if os.IsNotExist(err) {
			if err := removeProjectedPaths(previousPaths); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, err
	}

	targetBinDir := filepath.Join(baseDir, "bin")
	nextPaths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		sourcePath := filepath.Join(sourceBinDir, entry.Name())
		targetPath := filepath.Join(targetBinDir, entry.Name())
		if err := copySystemBinFile(sourcePath, targetPath, info.Mode().Perm()); err != nil {
			return nil, err
		}
		nextPaths = append(nextPaths, targetPath)
	}
	if len(nextPaths) > 0 {
		if err := EnsureUserPathContains(targetBinDir); err != nil {
			return nil, err
		}
	}
	if err := removeStaleProjectedPaths(previousPaths, nextPaths); err != nil {
		return nil, err
	}
	sort.Strings(nextPaths)
	return nextPaths, nil
}

func (m toolSystemManifest) hasTag(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for _, item := range m.Tags {
		if strings.ToLower(strings.TrimSpace(item)) == tag {
			return true
		}
	}
	return false
}

func loadToolSystemManifest(path string) (toolSystemManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return toolSystemManifest{}, nil
		}
		return toolSystemManifest{}, err
	}
	frontMatter, ok := extractMarkdownFrontMatter(data)
	if !ok {
		return toolSystemManifest{}, fmt.Errorf("%s missing YAML front matter: %s", ToolManifestName, path)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(frontMatter, &raw); err != nil {
		return toolSystemManifest{}, err
	}
	return toolSystemManifest{Tags: normalizeManifestTags(raw["tags"])}, nil
}

func normalizeManifestTags(raw any) []string {
	switch typed := raw.(type) {
	case string:
		return splitManifestTags(typed)
	case []any:
		next := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				next = append(next, splitManifestTags(text)...)
			}
		}
		return next
	case []string:
		next := make([]string, 0, len(typed))
		for _, item := range typed {
			next = append(next, splitManifestTags(item)...)
		}
		return next
	default:
		return nil
	}
}

func splitManifestTags(value string) []string {
	items := strings.Split(value, ",")
	next := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			next = append(next, item)
		}
	}
	return next
}

func extractMarkdownFrontMatter(data []byte) ([]byte, bool) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, false
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" || line == "..." {
			return []byte(strings.Join(lines[1:i], "\n")), true
		}
	}
	return nil, false
}

func removeStaleProjectedPaths(previousPaths []string, nextPaths []string) error {
	nextSet := make(map[string]struct{}, len(nextPaths))
	for _, path := range nextPaths {
		nextSet[path] = struct{}{}
	}
	stale := make([]string, 0)
	for _, path := range previousPaths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		if _, ok := nextSet[trimmed]; ok {
			continue
		}
		stale = append(stale, trimmed)
	}
	return removeProjectedPaths(stale)
}

func removeProjectedPaths(paths []string) error {
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		if err := os.Remove(trimmed); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func RemoveProjectedPaths(paths []string) error {
	return removeProjectedPaths(paths)
}

func copySystemBinFile(src string, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if mode == 0 {
		mode = 0o755
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, mode); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
	}
	return os.Rename(tmpPath, dst)
}
