package archive

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-runtime/packages/agentctx"
	"github.com/colinagent/openbrain/server/internal/server/chatindex"
	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

const (
	topLevelKeepCount              = 10
	dailyRetentionDays             = 15
	chatMonthlyCompressAfterMonths = 12
	recentWriteGrace               = 10 * time.Minute
)

type Service struct {
	core CoreClient
	now  func() time.Time

	mu sync.Mutex
}

func NewService(core CoreClient) *Service {
	return &Service{
		core: core,
		now:  time.Now,
	}
}

type runner struct {
	core   CoreClient
	now    time.Time
	result *protocol.ArchiveCleanupResult

	workspaceRoots []string
	openFiles      map[string]struct{}
	openChatFiles  map[string]struct{}
	openPlanFiles  map[string]struct{}
	openChatBodies map[string]string

	agentCwds   []string
	chatRoots   []string
	planRoots   []string
	threadRoots []string

	activeThreadIDs map[string]struct{}
	openThreadIDs   map[string]struct{}
	activePlanPaths map[string]struct{}

	threadsByID map[string]*threadFileRef
	allThreads  []*threadFileRef
}

func (s *Service) Run(ctx context.Context, params protocol.ArchiveCleanupParams) (*protocol.ArchiveCleanupResult, error) {
	if s == nil || s.core == nil {
		return nil, fmt.Errorf("archive core client is not initialized")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sysCfg, err := s.core.GetSystemConfig(ctx)
	if err != nil {
		return nil, err
	}
	baseDir := normalizePath(sysCfg.BaseDir)
	if baseDir == "" {
		return nil, fmt.Errorf("system baseDir is required")
	}
	defaultWorkspace := normalizePath(sysCfg.DefaultWorkspace)
	if defaultWorkspace == "" {
		return nil, fmt.Errorf("system defaultWorkspace is required")
	}
	chatindex.SetBaseDir(baseDir)
	workspaceRoots := normalizePathList(append(
		normalizePathList(params.WorkspaceRoots),
		defaultWorkspace,
	))

	r := &runner{
		core:            s.core,
		now:             s.now().In(time.Local),
		result:          &protocol.ArchiveCleanupResult{},
		workspaceRoots:  workspaceRoots,
		openFiles:       make(map[string]struct{}),
		openChatFiles:   make(map[string]struct{}),
		openPlanFiles:   make(map[string]struct{}),
		openChatBodies:  make(map[string]string),
		activeThreadIDs: make(map[string]struct{}),
		openThreadIDs:   make(map[string]struct{}),
		activePlanPaths: make(map[string]struct{}),
		threadsByID:     make(map[string]*threadFileRef),
	}
	for _, path := range normalizePathList(params.OpenFilePaths) {
		r.openFiles[path] = struct{}{}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md":
			lower := strings.ToLower(path)
			if strings.Contains(lower, string(filepath.Separator)+filepath.Join(".agent", "chat")+string(filepath.Separator)) ||
				strings.HasSuffix(lower, string(filepath.Separator)+filepath.Join(".agent", "chat")) {
				r.openChatFiles[path] = struct{}{}
			}
			if strings.Contains(lower, string(filepath.Separator)+filepath.Join(".agent", "context")+string(filepath.Separator)) ||
				strings.HasSuffix(lower, string(filepath.Separator)+filepath.Join(".agent", "context")) {
				r.openPlanFiles[path] = struct{}{}
			}
		}
	}

	r.agentCwds = discoverActiveAgentCwds(r.workspaceRoots)
	r.chatRoots = existingDirs(workspaceAgentSubdirs(r.agentCwds, "chat"))
	r.planRoots = existingDirs(workspaceAgentSubdirs(r.agentCwds, "context"))

	threadSearchRoots := make([]string, 0, 1)
	threadSearchRoots = append(threadSearchRoots, filepath.Join(baseDir, "threads"))

	threadRoots, err := collectThreadRoots(threadSearchRoots)
	if err != nil {
		return nil, err
	}
	r.threadRoots = threadRoots

	threads, err := scanThreadFiles(r.threadRoots)
	if err != nil {
		return nil, err
	}
	r.allThreads = threads
	for _, thread := range threads {
		if thread == nil || thread.ThreadID == "" {
			continue
		}
		r.threadsByID[thread.ThreadID] = thread
		if _, ok := r.openChatFiles[thread.ChatPath]; ok {
			r.openThreadIDs[thread.ThreadID] = struct{}{}
		}
		if _, ok := r.openPlanFiles[thread.PlanPath]; ok {
			r.openThreadIDs[thread.ThreadID] = struct{}{}
		}
		if _, ok := r.openPlanFiles[thread.ExecutionPlanPath]; ok {
			r.openThreadIDs[thread.ThreadID] = struct{}{}
		}
	}

	activeThreads, err := r.core.ListActiveThreads(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range activeThreads {
		threadID := strings.TrimSpace(item.ThreadID)
		if threadID == "" {
			continue
		}
		r.activeThreadIDs[threadID] = struct{}{}
		if thread := r.threadsByID[threadID]; thread != nil {
			if thread.PlanPath != "" {
				r.activePlanPaths[thread.PlanPath] = struct{}{}
			}
			if thread.ExecutionPlanPath != "" {
				r.activePlanPaths[thread.ExecutionPlanPath] = struct{}{}
			}
		}
	}

	for chatPath := range r.openChatFiles {
		body, err := os.ReadFile(chatPath)
		if err != nil {
			continue
		}
		r.openChatBodies[chatPath] = string(body)
	}

	for _, root := range r.chatRoots {
		if err := r.migrateLegacyDocumentZips(ctx, root, fileKindChat); err != nil {
			r.result.Errors = append(r.result.Errors, err.Error())
		}
	}
	for _, root := range r.planRoots {
		if err := r.migrateLegacyDocumentZips(ctx, root, fileKindPlan); err != nil {
			r.result.Errors = append(r.result.Errors, err.Error())
		}
	}

	for _, root := range r.chatRoots {
		if err := r.processChatRoot(ctx, root); err != nil {
			r.result.Errors = append(r.result.Errors, err.Error())
		}
	}
	for _, root := range r.planRoots {
		if err := r.processPlanRoot(ctx, root); err != nil {
			r.result.Errors = append(r.result.Errors, err.Error())
		}
	}
	fileRecordsForThreads := make([]chatindex.FileRecord, 0)
	for _, agentCwd := range r.agentCwds {
		fileRecords, err := chatindex.ReconcileFileIndex(agentCwd)
		if err != nil {
			r.result.Errors = append(r.result.Errors, err.Error())
			continue
		}
		for _, record := range fileRecords {
			fileRecordsForThreads = append(fileRecordsForThreads, record)
		}
	}
	for _, threadRoot := range r.threadRoots {
		if _, err := chatindex.ReconcileThreadIndexAtRoot(threadRoot, fileRecordsForThreads); err != nil {
			r.result.Errors = append(r.result.Errors, err.Error())
		}
	}

	return r.result, nil
}

func (r *runner) migrateLegacyDocumentZips(ctx context.Context, root string, kind fileKind) error {
	moves, err := migrateLegacyMonthlyZipArchives(root, kind)
	if err != nil {
		return err
	}
	for _, move := range moves {
		switch kind {
		case fileKindChat:
			meta, err := agentctx.ReadChatFileMeta(move.newPath)
			if err != nil {
				r.result.Errors = append(r.result.Errors, fmt.Sprintf("legacy chat zip migrated but frontmatter read failed for %s: %v", move.newPath, err))
				continue
			}
			if err := r.updateChatPathRefs(ctx, strings.TrimSpace(meta.ThreadID), move.oldPath, move.newPath, strings.TrimSpace(meta.Title)); err != nil {
				return err
			}
		case fileKindPlan:
			if err := r.updatePlanPathRefs(ctx, move.oldPath, move.newPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func workspaceAgentSubdirs(workspaceRoots []string, leaf string) []string {
	dirs := make([]string, 0, len(workspaceRoots))
	for _, root := range workspaceRoots {
		if root == "" {
			continue
		}
		dirs = append(dirs, filepath.Join(root, ".agent", leaf))
	}
	return dirs
}

func archiveFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func chatPathOwnerCwd(path string) string {
	normalized := normalizePath(path)
	marker := string(filepath.Separator) + filepath.Join(".agent", "chat")
	idx := strings.Index(normalized, marker)
	if idx <= 0 {
		return ""
	}
	return normalized[:idx]
}

func threadPathOwnerRoot(path string) string {
	normalized := normalizePath(path)
	sep := string(filepath.Separator)
	threadsMarker := sep + "threads"
	if idx := strings.Index(normalized, threadsMarker+sep); idx >= 0 {
		return normalized[:idx+len(threadsMarker)]
	}
	if strings.HasSuffix(normalized, threadsMarker) {
		return normalized
	}
	marker := sep + "thread"
	idx := strings.Index(normalized, marker)
	if idx < 0 {
		return ""
	}
	return normalized[:idx+len(marker)]
}

func discoverActiveAgentCwds(workspaceRoots []string) []string {
	found := make(map[string]struct{})
	for _, root := range normalizePathList(workspaceRoots) {
		rootAgentDir := filepath.Join(root, ".agent")
		if archiveFileExists(filepath.Join(rootAgentDir, "AGENT.md")) || archiveFileExists(filepath.Join(rootAgentDir, "agent.md")) {
			found[normalizePath(root)] = struct{}{}
		}
		if dirExists(filepath.Join(rootAgentDir, "chat")) || dirExists(filepath.Join(rootAgentDir, "context")) {
			found[normalizePath(root)] = struct{}{}
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			name := strings.ToLower(strings.TrimSpace(d.Name()))
			switch name {
			case ".git", "node_modules", "dist", "history":
				return filepath.SkipDir
			}
			agentDir := filepath.Join(path, ".agent")
			if archiveFileExists(filepath.Join(agentDir, "AGENT.md")) || archiveFileExists(filepath.Join(agentDir, "agent.md")) {
				found[normalizePath(path)] = struct{}{}
				return filepath.SkipDir
			}
			return nil
		})
	}
	out := make([]string, 0, len(found))
	for cwd := range found {
		out = append(out, cwd)
	}
	sort.Strings(out)
	return out
}

func existingDirs(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = normalizePath(path)
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func normalizePathList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := normalizePath(value)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func (r *runner) processChatRoot(ctx context.Context, chatRoot string) error {
	entries, err := os.ReadDir(chatRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	files, err := scanTopLevelFiles(chatRoot, entries, ".md")
	if err != nil {
		return err
	}
	sortFileRecordsNewestFirst(files)

	touchedDayDirs := make(map[string]struct{})
	for index, file := range files {
		if index < topLevelKeepCount {
			continue
		}
		if !r.canMoveChatFile(file.path, file.modTime) {
			continue
		}
		targetDir := historyDayDirForTime(chatRoot, file.modTime)
		if err := r.moveChatFile(ctx, file.path, targetDir); err != nil {
			r.result.Errors = append(r.result.Errors, err.Error())
			continue
		}
		r.result.MovedChats++
		touchedDayDirs[targetDir] = struct{}{}
	}

	return r.rollHistoryDays(chatRoot, fileKindChat, touchedDayDirs, ctx)
}

func (r *runner) processPlanRoot(ctx context.Context, planRoot string) error {
	entries, err := os.ReadDir(planRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	files, err := scanTopLevelFiles(planRoot, entries, ".md")
	if err != nil {
		return err
	}
	sortFileRecordsNewestFirst(files)

	touchedDayDirs := make(map[string]struct{})
	for index, file := range files {
		if index < topLevelKeepCount {
			continue
		}
		if !r.canMovePlanFile(file.path, file.modTime) {
			continue
		}
		targetDir := historyDayDirForTime(planRoot, file.modTime)
		if err := r.movePlanFile(ctx, file.path, targetDir); err != nil {
			r.result.Errors = append(r.result.Errors, err.Error())
			continue
		}
		r.result.MovedPlans++
		touchedDayDirs[targetDir] = struct{}{}
	}

	return r.rollHistoryDays(planRoot, fileKindPlan, touchedDayDirs, ctx)
}

func (r *runner) canMoveChatFile(path string, modTime time.Time) bool {
	if _, ok := r.openFiles[path]; ok {
		r.result.SkippedOpenFiles++
		return false
	}
	return !r.isRecentlyModified(modTime)
}

func (r *runner) canMovePlanFile(path string, modTime time.Time) bool {
	if _, ok := r.openFiles[path]; ok {
		r.result.SkippedOpenFiles++
		return false
	}
	if _, ok := r.activePlanPaths[path]; ok {
		r.result.SkippedActiveThreads++
		return false
	}
	for _, body := range r.openChatBodies {
		if strings.Contains(body, path) {
			r.result.SkippedOpenFiles++
			return false
		}
	}
	return !r.isRecentlyModified(modTime)
}

func (r *runner) isRecentlyModified(modTime time.Time) bool {
	return modTime.After(r.now.Add(-recentWriteGrace))
}

type fileKind string

const (
	fileKindChat fileKind = "chat"
	fileKindPlan fileKind = "plan"
)

func (r *runner) rollHistoryDays(root string, kind fileKind, touchedDayDirs map[string]struct{}, ctx context.Context) error {
	historyRoot := filepath.Join(root, "history")
	dayDirs, err := listHistoryDayDirs(historyRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	thresholdDay := startOfLocalDay(r.now).AddDate(0, 0, -dailyRetentionDays)
	for _, dayDir := range dayDirs {
		if _, skip := touchedDayDirs[dayDir.path]; skip {
			continue
		}
		if !dayDir.day.Before(thresholdDay) {
			continue
		}
		switch kind {
		case fileKindChat:
			if err := r.rollChatDayDir(ctx, root, dayDir.path, dayDir.day); err != nil {
				r.result.Errors = append(r.result.Errors, err.Error())
			}
		case fileKindPlan:
			if err := r.rollPlanDayDir(ctx, root, dayDir.path, dayDir.day); err != nil {
				r.result.Errors = append(r.result.Errors, err.Error())
			}
		}
	}

	switch kind {
	case fileKindChat:
		compressed, err := compressOldMonthlyDirsIntoYearZips(historyRoot, startOfLocalMonth(r.now), chatMonthlyCompressAfterMonths, r.canCompressChatMonthDir)
		if err != nil {
			return err
		}
		r.result.CompressedArchives += compressed
	}

	return nil
}

func (r *runner) canCompressChatMonthDir(monthDir historyMonthDir) bool {
	for path := range r.openFiles {
		if pathIsInsideDir(path, monthDir.path) {
			r.result.SkippedOpenFiles++
			return false
		}
	}
	for _, thread := range r.allThreads {
		if thread == nil {
			continue
		}
		threadID := strings.TrimSpace(thread.ThreadID)
		if pathIsInsideDir(thread.ChatPath, monthDir.path) {
			if thread.IsLive {
				r.result.SkippedActiveThreads++
				return false
			}
			if _, ok := r.activeThreadIDs[threadID]; ok {
				r.result.SkippedActiveThreads++
				return false
			}
		}
	}
	return true
}

func (r *runner) rollChatDayDir(ctx context.Context, chatRoot, dayDir string, day time.Time) error {
	entries, err := os.ReadDir(dayDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		path := filepath.Join(dayDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !r.canMoveChatFile(path, info.ModTime()) {
			continue
		}
		targetDir := historyMonthDayDir(chatRoot, day)
		if err := r.moveChatFile(ctx, path, targetDir); err != nil {
			return err
		}
		r.result.RolledIntoMonthlyDirs++
	}
	return cleanupEmptyHistoryDir(dayDir)
}

func (r *runner) rollPlanDayDir(ctx context.Context, planRoot, dayDir string, day time.Time) error {
	entries, err := os.ReadDir(dayDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		path := filepath.Join(dayDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !r.canMovePlanFile(path, info.ModTime()) {
			continue
		}
		targetDir := historyMonthDayDir(planRoot, day)
		if err := r.movePlanFile(ctx, path, targetDir); err != nil {
			return err
		}
		r.result.RolledIntoMonthlyDirs++
	}
	return cleanupEmptyHistoryDir(dayDir)
}

func (r *runner) moveChatFile(ctx context.Context, sourcePath, targetDir string) error {
	meta, err := agentctx.ReadChatFileMeta(sourcePath)
	if err != nil {
		return err
	}
	targetPath, err := allocateUniquePath(targetDir, filepath.Base(sourcePath))
	if err != nil {
		return err
	}
	threadID := strings.TrimSpace(meta.ThreadID)
	if err := moveChatWithAssets(sourcePath, threadID, targetPath); err != nil {
		return err
	}
	cwd := chatPathOwnerCwd(sourcePath)
	if threadID != "" && cwd != "" {
		if record, err := chatindex.FindFileRecordByThreadID(cwd, threadID); err == nil && record != nil {
			_ = chatindex.UpsertFileRecord(cwd, chatindex.FileRecord{
				FileID:   record.FileID,
				ThreadID: threadID,
				Path:     targetPath,
			})
		}
		if err := r.updateChatPathRefs(ctx, threadID, sourcePath, targetPath, strings.TrimSpace(meta.Title)); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) movePlanFile(ctx context.Context, sourcePath, targetDir string) error {
	targetPath, err := allocateUniquePath(targetDir, filepath.Base(sourcePath))
	if err != nil {
		return err
	}
	if err := moveFileAtomic(sourcePath, targetPath); err != nil {
		return err
	}
	return r.updatePlanPathRefs(ctx, sourcePath, targetPath)
}
