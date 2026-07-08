package scan

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/internal/nodeindex"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
	"github.com/goccy/go-yaml"
)

const maxScanDepth = 10

// ---------------------------------------------------------------------------
// scanner — stateful scanner for a single refresh cycle
// ---------------------------------------------------------------------------

type Scanner struct {
	uid                     string
	scanDir                 string
	refBaseDir              string
	nodeIndex               *nodeindex.Index
	seen                    map[string]struct{}
	uriByID                 map[string]string
	allowSameKeyAcrossPaths bool
	nodes                   []*op.OpNode
	err                     error
}

func NewScanner(uid, scanDir string) *Scanner {
	idx, _ := nodeindex.Open(scanDir)
	return &Scanner{
		uid:        strings.TrimSpace(uid),
		scanDir:    scanDir,
		refBaseDir: scanDir,
		nodeIndex:  idx,
		seen:       make(map[string]struct{}),
		uriByID:    map[string]string{},
	}
}

// WithPathAwareAgentDedup keeps one logical agent visible at multiple workspace paths.
// This is only for dir agent scan results; global node refresh should still dedup by id.
func (s *Scanner) WithPathAwareAgentDedup() *Scanner {
	s.allowSameKeyAcrossPaths = true
	return s
}

// WithRefBaseDir overrides the base directory used for @agents/@skills/@tools references.
func (s *Scanner) WithRefBaseDir(baseDir string) *Scanner {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir != "" {
		s.refBaseDir = baseDir
	}
	return s
}

func (s *Scanner) WithNodeIndexBaseDir(baseDir string) *Scanner {
	idx, err := nodeindex.Open(baseDir)
	if err == nil {
		s.nodeIndex = idx
	} else {
		s.recordErr(fmt.Errorf("open node index: %w", err))
	}
	return s
}

func (s *Scanner) recordErr(err error) {
	if err == nil {
		return
	}
	s.err = errors.Join(s.err, err)
}

func (s *Scanner) dedupKey(node *op.OpNode) string {
	if node == nil {
		return ""
	}
	key := strings.TrimSpace(node.ID)
	if key == "" {
		return ""
	}
	if !s.allowSameKeyAcrossPaths {
		return key
	}
	cwd := strings.TrimSpace(node.Cwd)
	if cwd == "" {
		return key
	}
	return key + "::" + cwd
}

func (s *Scanner) addNode(node *op.OpNode) bool {
	if err := s.checkNodeConflict(node); err != nil {
		s.recordErr(err)
		return false
	}
	dedupKey := s.dedupKey(node)
	if dedupKey == "" {
		return false
	}
	if _, exists := s.seen[dedupKey]; exists {
		return false
	}
	s.seen[dedupKey] = struct{}{}
	s.nodes = append(s.nodes, node)
	return true
}

func (s *Scanner) checkNodeConflict(node *op.OpNode) error {
	if node == nil {
		return nil
	}
	id := strings.TrimSpace(node.ID)
	uri := strings.TrimSpace(node.URI)
	if id == "" || uri == "" {
		return nil
	}
	if existingURI, exists := s.uriByID[id]; exists && existingURI != uri {
		return fmt.Errorf("id conflict: %s maps to multiple URIs: %s and %s", id, existingURI, uri)
	}
	s.uriByID[id] = uri
	return nil
}

func (s *Scanner) assignNodeID(node *op.OpNode) *op.OpNode {
	if node == nil {
		return nil
	}
	if s.nodeIndex != nil {
		if err := s.nodeIndex.Assign(node); err != nil {
			s.recordErr(fmt.Errorf("record node index: %w", err))
		}
	}
	return node
}

// Nodes returns all nodes collected by ScanAgents / ScanTools / ScanSkills.
func (s *Scanner) Nodes() []*op.OpNode {
	return s.nodes
}

// Err returns hard scanner errors such as explicit id conflicts.
func (s *Scanner) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}

// ---------------------------------------------------------------------------
// Scan methods — recursive directory traversal
// ---------------------------------------------------------------------------

func (s *Scanner) ScanAgents(dir string, depth int) []*op.OpNode {
	if depth > maxScanDepth || !isDir(dir) {
		return nil
	}
	var found []*op.OpNode

	// Allow scanning a single agent directory directly:
	// if dir contains .agent/AGENT.md or .agents/AGENTS.md, parse it first.
	// scan current dir
	for _, cfgPath := range findAgentConfigs(dir) {
		node := s.parseAndBuildAgent(cfgPath, dir)
		if node == nil {
			continue
		}
		if s.addNode(node) {
			found = append(found, node)
			found = append(found, s.scanAgentPrivateSubagents(dir, depth+1)...)
		}
	}

	// scan subdirs
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipDir(entry.Name()) {
			continue
		}
		childDir := filepath.Join(dir, entry.Name())
		if isOrgNamespaceDir(entry.Name()) {
			found = append(found, s.scanOrgAgentNamespace(childDir, depth+1)...)
			continue
		}
		for _, cfgPath := range findAgentConfigs(childDir) {
			node := s.parseAndBuildAgent(cfgPath, childDir)
			if node == nil {
				continue
			}
			if s.addNode(node) {
				found = append(found, node)
				found = append(found, s.scanAgentPrivateSubagents(childDir, depth+1)...)
				found = append(found, s.ScanAgents(childDir, depth+1)...)
			}
		}
	}
	return found
}

func (s *Scanner) scanAgentPrivateSubagents(agentDir string, depth int) []*op.OpNode {
	if depth > maxScanDepth {
		return nil
	}
	subagentsDir := filepath.Join(agentDir, ".agent", "subagents")
	if !isDir(subagentsDir) {
		return nil
	}
	var found []*op.OpNode
	entries, _ := os.ReadDir(subagentsDir)
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipDir(entry.Name()) {
			continue
		}
		childDir := filepath.Join(subagentsDir, entry.Name())
		for _, cfgPath := range findAgentConfigs(childDir) {
			node := s.parseAndBuildAgent(cfgPath, childDir)
			if node == nil {
				continue
			}
			if s.addNode(node) {
				found = append(found, node)
				found = append(found, s.scanAgentPrivateSubagents(childDir, depth+1)...)
			}
		}
	}
	return found
}

func (s *Scanner) ScanTools(dir string, depth int) []*op.OpNode {
	if depth > maxScanDepth || !isDir(dir) {
		return nil
	}
	var found []*op.OpNode
	toolsMd := filepath.Join(dir, "TOOL.md")
	if node := s.parseAndBuildTools(toolsMd); node != nil {
		if s.addNode(node) {
			found = append(found, node)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipDir(entry.Name()) {
			continue
		}
		found = append(found, s.ScanTools(filepath.Join(dir, entry.Name()), depth+1)...)
	}
	return found
}

func (s *Scanner) ScanSkills(dir string, depth int) []*op.OpNode {
	if depth > maxScanDepth || !isDir(dir) {
		return nil
	}
	var found []*op.OpNode
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipDir(entry.Name()) {
			continue
		}
		if isOrgNamespaceDir(entry.Name()) {
			found = append(found, s.scanOrgSkillNamespace(filepath.Join(dir, entry.Name()))...)
			continue
		}
		skillMd := filepath.Join(dir, entry.Name(), "SKILL.md")
		if node := s.parseAndBuildSkills(skillMd); node != nil {
			if s.addNode(node) {
				found = append(found, node)
			}
		}
	}
	return found
}

func (s *Scanner) scanOrgAgentNamespace(namespaceDir string, depth int) []*op.OpNode {
	if depth > maxScanDepth || !isDir(namespaceDir) {
		return nil
	}
	var found []*op.OpNode
	entries, _ := os.ReadDir(namespaceDir)
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipDir(entry.Name()) {
			continue
		}
		childDir := filepath.Join(namespaceDir, entry.Name())
		for _, cfgPath := range findAgentConfigs(childDir) {
			node := s.parseAndBuildAgent(cfgPath, childDir)
			if node == nil {
				continue
			}
			if s.addNode(node) {
				found = append(found, node)
			}
		}
	}
	return found
}

func (s *Scanner) scanOrgSkillNamespace(namespaceDir string) []*op.OpNode {
	if !isDir(namespaceDir) {
		return nil
	}
	var found []*op.OpNode
	entries, _ := os.ReadDir(namespaceDir)
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipDir(entry.Name()) {
			continue
		}
		skillMd := filepath.Join(namespaceDir, entry.Name(), "SKILL.md")
		if node := s.parseAndBuildSkills(skillMd); node != nil {
			if s.addNode(node) {
				found = append(found, node)
			}
		}
	}
	return found
}

func findAgentConfigs(dir string) []string {
	var paths []string
	if fileExists(filepath.Join(dir, ".agent", "AGENT.md")) {
		paths = append(paths, filepath.Join(dir, ".agent", "AGENT.md"))
	}
	if fileExists(filepath.Join(dir, ".agents", "AGENTS.md")) {
		paths = append(paths, filepath.Join(dir, ".agents", "AGENTS.md"))
	}
	return paths
}

// ---------------------------------------------------------------------------
// Config parsers (phase 1: pure YAML → raw structs)
// ---------------------------------------------------------------------------

func unmarshalFrontMatterWithBareAt(frontMatter []byte, out any) error {
	firstErr := yaml.Unmarshal(frontMatter, out)
	if firstErr == nil {
		return nil
	}

	normalized, changed := normalizeBareAtScalars(frontMatter)
	if !changed {
		return firstErr
	}
	if err := yaml.Unmarshal(normalized, out); err != nil {
		return firstErr
	}
	return nil
}

func normalizeBareAtScalars(frontMatter []byte) ([]byte, bool) {
	lines := strings.Split(strings.ReplaceAll(string(frontMatter), "\r\n", "\n"), "\n")
	changed := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if next, ok := rewriteListBareAtLine(line); ok {
			lines[i] = next
			changed = true
			continue
		}
		if next, ok := rewriteMapBareAtLine(line); ok {
			lines[i] = next
			changed = true
		}
	}

	if !changed {
		return frontMatter, false
	}
	return []byte(strings.Join(lines, "\n")), true
}

func rewriteListBareAtLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "-") {
		return "", false
	}
	rest := strings.TrimSpace(trimmed[1:])
	if rest == "" || strings.Contains(rest, ":") {
		return "", false
	}

	value, comment := splitInlineComment(rest)
	value = strings.TrimSpace(value)
	if !isBareAtValue(value) {
		return "", false
	}

	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	quoted := strconv.Quote(value)
	if comment != "" {
		return fmt.Sprintf("%s- %s %s", indent, quoted, comment), true
	}
	return fmt.Sprintf("%s- %s", indent, quoted), true
}

func rewriteMapBareAtLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	colon := strings.IndexByte(trimmed, ':')
	if colon <= 0 {
		return "", false
	}

	keyPart := strings.TrimSpace(trimmed[:colon])
	if keyPart == "" {
		return "", false
	}
	valuePart := strings.TrimSpace(trimmed[colon+1:])
	if valuePart == "" {
		return "", false
	}

	value, comment := splitInlineComment(valuePart)
	value = strings.TrimSpace(value)
	if !isBareAtValue(value) {
		return "", false
	}

	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	quoted := strconv.Quote(value)
	prefix := strings.TrimRight(trimmed[:colon+1], " ")
	if comment != "" {
		return fmt.Sprintf("%s%s %s %s", indent, prefix, quoted, comment), true
	}
	return fmt.Sprintf("%s%s %s", indent, prefix, quoted), true
}

func splitInlineComment(value string) (string, string) {
	if idx := strings.Index(value, " #"); idx >= 0 {
		return strings.TrimSpace(value[:idx]), strings.TrimSpace(value[idx+1:])
	}
	return value, ""
}

func isBareAtValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
		return false
	}
	return strings.HasPrefix(value, "@")
}

// ---------------------------------------------------------------------------
// Reference resolution (phase 2)
// ---------------------------------------------------------------------------

type refKind int

const (
	refName    refKind = iota // shell, read — system tool name
	refPath                   // ./tools, /abs/path, ~/path
	refBaseDir                // @agents/xxx, @tools/xxx, @skills/xxx
)

func classifyRef(ref string) refKind {
	if strings.HasPrefix(ref, "@") {
		return refBaseDir
	}
	if strings.HasPrefix(ref, ".") ||
		strings.HasPrefix(ref, "/") ||
		strings.HasPrefix(ref, "~") {
		return refPath
	}
	return refName
}

func (s *Scanner) resolveRefs(entries []string, kind op.NodeKind, workdir string, sourcePath ...string) (ids []string, sysTools []string) {
	seenIDs := make(map[string]struct{})
	source := ""
	if len(sourcePath) > 0 {
		source = strings.TrimSpace(sourcePath[0])
	}

	appendSysTool := func(name string) {
		if !slices.Contains(sysTools, name) {
			sysTools = append(sysTools, name)
		}
	}
	appendNodeID := func(nodeID string) {
		nodeID = strings.TrimSpace(nodeID)
		if nodeID == "" {
			return
		}
		if _, exists := seenIDs[nodeID]; exists {
			return
		}
		seenIDs[nodeID] = struct{}{}
		ids = append(ids, nodeID)
	}
	appendResolvedNodes := func(nodes []*op.OpNode) bool {
		appended := false
		for _, n := range nodes {
			nodeID := strings.TrimSpace(n.ID)
			if nodeID == "" {
				continue
			}
			appendNodeID(nodeID)
			appended = true
		}
		return appended
	}
	warnUnresolvedRef := func(ref string, resolvedPath string) {
		if kind != op.NodeKindTools {
			return
		}
		attrs := []any{"ref", ref, "workdir", workdir, "baseDir", s.refBaseDir}
		if source != "" {
			attrs = append(attrs, "manifest", source)
		}
		if strings.TrimSpace(resolvedPath) != "" {
			attrs = append(attrs, "resolvedPath", resolvedPath)
		}
		slog.Warn("failed to resolve tool ref", attrs...)
	}
	appendDirectNodeIDRef := func(ref string) bool {
		deref := strings.TrimSpace(strings.TrimPrefix(ref, "@"))
		refKind, ok := op.NodeKindFromID(deref)
		if !ok || refKind != kind {
			return false
		}
		appendNodeID(deref)
		return true
	}
	resolveExistingNodes := func(path string) []*op.OpNode {
		path = strings.TrimSpace(path)
		if path == "" {
			return nil
		}
		uri := ""
		switch kind {
		case op.NodeKindTools:
			if strings.HasSuffix(path, "TOOL.md") {
				uri = op.PathToURI(path)
			}
		case op.NodeKindSkill:
			if strings.HasSuffix(path, "SKILL.md") {
				uri = op.PathToURI(path)
			}
		}
		if uri == "" {
			return nil
		}
		found := make([]*op.OpNode, 0, 1)
		for _, node := range s.nodes {
			if node == nil || node.Kind != string(kind) || strings.TrimSpace(node.URI) != uri {
				continue
			}
			found = append(found, node)
		}
		return found
	}

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		switch classifyRef(entry) {
		case refName:
			if slices.Contains(op.SystoolNames, entry) {
				appendSysTool(entry)
			}
			continue
		case refBaseDir:
			if appendDirectNodeIDRef(entry) {
				continue
			}
			_, dirPath, ok := resolveBaseDirRef(s.refBaseDir, entry)
			if !ok {
				warnUnresolvedRef(entry, "")
				continue
			}
			if nodes := resolveExistingNodes(dirPath); len(nodes) > 0 {
				if !appendResolvedNodes(nodes) {
					warnUnresolvedRef(entry, dirPath)
				}
				continue
			}
			if !appendResolvedNodes(s.scanByKind(dirPath, kind)) {
				warnUnresolvedRef(entry, dirPath)
			}
		case refPath:
			absPath := resolvePath(workdir, entry)
			if nodes := resolveExistingNodes(absPath); len(nodes) > 0 {
				if !appendResolvedNodes(nodes) {
					warnUnresolvedRef(entry, absPath)
				}
				continue
			}
			if !appendResolvedNodes(s.scanByKind(absPath, kind)) {
				warnUnresolvedRef(entry, absPath)
			}
		}
	}
	return
}

func containsSystoolName(value string) bool {
	value = strings.TrimSpace(value)
	for _, name := range op.SystoolNames {
		if value == name {
			return true
		}
	}
	return false
}

func (s *Scanner) resolveSkillRefs(entries []string, workdir string) []string {
	ids := make([]string, 0, len(entries))
	seen := make(map[string]struct{})

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		resolvedIDs, _ := s.resolveRefs([]string{entry}, op.NodeKindSkill, workdir)
		if len(resolvedIDs) == 0 {
			slog.Warn("failed to resolve skill ref", "ref", entry, "workdir", workdir, "baseDir", s.refBaseDir)
			continue
		}
		for _, id := range resolvedIDs {
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	return ids
}

func (s *Scanner) resolveBindAgent(bind string, cwd string) (node *op.OpNode) {
	ref := strings.TrimSpace(bind)
	if ref == "" || !strings.HasPrefix(ref, "@") {
		return nil
	}
	deref := strings.TrimSpace(strings.TrimPrefix(ref, "@"))

	if kind, ok := op.NodeKindFromID(deref); !ok || kind != op.NodeKindAgent {
		return nil
	}
	if n, ok := cache.GetValue[op.OpNode](deref, cache.PrefixNode); ok {
		n.Cwd = cwd
		return &n
	}

	if n := s.resolveBindAgentFromIndex(deref); n != nil {
		cache.SetValue(n.ID, cache.PrefixNode, *n, cache.NoExpiration)
		n.Cwd = cwd
		return n
	}

	if n := s.resolveBindAgentByScanningBaseDir(deref); n != nil {
		cache.SetValue(n.ID, cache.PrefixNode, *n, cache.NoExpiration)
		n.Cwd = cwd
		return n
	}

	return nil
}

func (s *Scanner) resolveBindAgentFromIndex(nodeID string) *op.OpNode {
	if s.nodeIndex == nil {
		return nil
	}
	rec, ok := s.nodeIndex.Resolve(nodeID)
	if !ok || rec.Kind != string(op.NodeKindAgent) {
		return nil
	}
	cfgPath := op.URIToPath(rec.URI)
	if cfgPath == "" || !fileExists(cfgPath) {
		return nil
	}
	node := s.parseAndBuildAgent(cfgPath, agentConfigRootDir(cfgPath))
	if node == nil || strings.TrimSpace(node.ID) != nodeID {
		return nil
	}
	return node
}

func (s *Scanner) resolveBindAgentByScanningBaseDir(nodeID string) *op.OpNode {
	baseDir := strings.TrimSpace(s.refBaseDir)
	if baseDir == "" {
		baseDir = strings.TrimSpace(s.scanDir)
	}
	if baseDir == "" {
		return nil
	}
	scanner := NewScanner(s.uid, baseDir).
		WithRefBaseDir(baseDir).
		WithNodeIndexBaseDir(baseDir)
	nodes := scanner.ScanAgents(filepath.Join(baseDir, "agents"), 0)
	if err := scanner.Err(); err != nil {
		s.recordErr(err)
		return nil
	}
	for _, node := range nodes {
		if node == nil || strings.TrimSpace(node.ID) == "" {
			continue
		}
		cache.SetValue(node.ID, cache.PrefixNode, *node, cache.NoExpiration)
		if node.ID == nodeID {
			copyNode := *node
			return &copyNode
		}
	}
	return nil
}

func agentConfigRootDir(cfgPath string) string {
	cfgPath = strings.TrimSpace(cfgPath)
	if cfgPath == "" {
		return ""
	}
	return filepath.Dir(filepath.Dir(cfgPath))
}

func agentResourceRootDir(cfgPath, fallbackRoot string) string {
	cfgPath = strings.TrimSpace(cfgPath)
	if cfgPath == "" {
		return strings.TrimSpace(fallbackRoot)
	}
	dir := filepath.Dir(cfgPath)
	switch filepath.Base(dir) {
	case ".agent", ".agents":
		return dir
	default:
		return strings.TrimSpace(fallbackRoot)
	}
}

// func (s *Scanner) resolveAgentNameByID(id string) string {
// 	id = strings.TrimSpace(id)
// 	if id == "" {
// 		return ""
// 	}
// 	for _, node := range s.nodes {
// 		if node == nil || node.ID != id || node.Kind != string(op.NodeKindAgent) {
// 			continue
// 		}
// 		if meta, ok := node.Meta.(*op.AgentMeta); ok && meta != nil {
// 			return strings.TrimSpace(meta.Name)
// 		}
// 		return ""
// 	}
// 	return ""
// }

func resolveBaseDirRef(basedir, ref string) (kind op.NodeKind, path string, ok bool) {
	ref = strings.TrimPrefix(ref, "@")
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	category, name := parts[0], parts[1]
	switch category {
	case "agents":
		return op.NodeKindAgent, filepath.Join(basedir, "agents", name), true
	case "skills":
		return op.NodeKindSkill, filepath.Join(basedir, "skills", name, "SKILL.md"), true
	case "tools":
		return op.NodeKindTools, filepath.Join(basedir, "tools", name, "TOOL.md"), true
	}
	return "", "", false
}

func isOrgNamespaceDir(name string) bool {
	name = strings.TrimSpace(name)
	return strings.HasPrefix(name, "@org-") && len(name) > len("@org-")
}

func (s *Scanner) scanByKind(path string, kind op.NodeKind) []*op.OpNode {
	switch kind {
	case op.NodeKindAgent:
		return s.ScanAgents(path, 0)
	case op.NodeKindSkill:
		// If path points to a SKILL.md file directly
		if strings.HasSuffix(path, "SKILL.md") {
			if node := s.parseAndBuildSkills(path); node != nil {
				if s.addNode(node) {
					return []*op.OpNode{node}
				}
			}
			return nil
		}
		return s.ScanSkills(path, 0)
	case op.NodeKindTools:
		// If path points to a TOOL.md file directly
		if strings.HasSuffix(path, "TOOL.md") {
			if node := s.parseAndBuildTools(path); node != nil {
				if s.addNode(node) {
					return []*op.OpNode{node}
				}
			}
			return nil
		}
		return s.ScanTools(path, 0)
	}
	return nil
}

func resolvePath(workdir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		if home != "" {
			return filepath.Join(home, path[1:])
		}
	}
	return filepath.Join(workdir, path)
}

// ---------------------------------------------------------------------------
// Avatar resolution (inlined from former avatar_resolve.go)
// ---------------------------------------------------------------------------

func resolveAgentAvatarURI(agentRoot string, avatar string) string {
	avatar = strings.TrimSpace(avatar)
	if avatar == "" {
		return ""
	}
	if strings.Contains(avatar, "://") {
		return avatar
	}
	if !filepath.IsAbs(agentRoot) {
		if filepath.IsAbs(avatar) || avatar == "~" || strings.HasPrefix(avatar, "~/") || strings.HasPrefix(avatar, "~\\") {
			if abs, err := common.ResolveAbsolutePath("", avatar); err == nil && abs != "" {
				return op.PathToURI(abs)
			}
		}
		return avatar
	}
	abs, err := common.ResolveAbsolutePath(agentRoot, avatar)
	if err != nil || abs == "" {
		return avatar
	}
	return op.PathToURI(abs)
}

// ---------------------------------------------------------------------------
// Dir/file filter (inlined from former scan_filters.go)
// ---------------------------------------------------------------------------

var skipDirNames = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	".venv":        {},
	"__pycache__":  {},
	"dist":         {},
	"build":        {},
	"bin":          {},
	"output":       {},
	"outputs":      {},
	"logs":         {},
	"configs":      {},
}

func shouldSkipDir(name string) bool {
	if _, ok := skipDirNames[name]; ok {
		return true
	}
	if strings.HasPrefix(name, ".") && name != ".agent" && name != ".agents" {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Filesystem helpers
// ---------------------------------------------------------------------------

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
