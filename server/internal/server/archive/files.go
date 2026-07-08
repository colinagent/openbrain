package archive

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zip"
)

type fileRecord struct {
	path    string
	name    string
	modTime time.Time
}

type historyDayDir struct {
	path string
	day  time.Time
}

type historyMonthDir struct {
	path  string
	month time.Time
}

type threadFileRef struct {
	ThreadID          string
	ChatPath          string
	PlanPath          string
	ExecutionPlanPath string
	Title             string
	FilePath          string
	ThreadRoot        string
	IsLive            bool
	LeafID            *string
}

type threadMetaPatch struct {
	ChatPath          string
	PlanPath          string
	ExecutionPlanPath string
	Title             string
}

type legacyPathMove struct {
	oldPath string
	newPath string
}

var flatAssetReferencePattern = regexp.MustCompile(`(?:\./)?assets/[^)\s"'#?]+`)

func collectThreadRoots(searchRoots []string) ([]string, error) {
	roots := make([]string, 0, 16)
	seen := make(map[string]struct{}, 16)
	addRoot := func(path string) {
		cleaned := filepath.Clean(path)
		if _, ok := seen[cleaned]; ok {
			return
		}
		seen[cleaned] = struct{}{}
		roots = append(roots, cleaned)
	}
	for _, searchRoot := range normalizePathList(searchRoots) {
		info, err := os.Stat(searchRoot)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		if filepath.Base(searchRoot) == "threads" {
			addRoot(searchRoot)
			continue
		}
		err = filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !d.IsDir() {
				return nil
			}
			if path != searchRoot {
				switch d.Name() {
				case ".git", ".agent", "node_modules", "dist", "history":
					return filepath.SkipDir
				case "thread":
					addRoot(path)
					return filepath.SkipDir
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(roots)
	return roots, nil
}

func scanThreadFiles(threadRoots []string) ([]*threadFileRef, error) {
	out := make([]*threadFileRef, 0, 32)
	for _, threadRoot := range threadRoots {
		entries, err := os.ReadDir(threadRoot)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".jsonl") {
				continue
			}
			ref, err := readThreadFileRef(filepath.Join(threadRoot, entry.Name()), threadRoot, true)
			if err != nil {
				return nil, err
			}
			out = append(out, ref)
		}
	}
	return out, nil
}

func migrateLegacyMonthlyZipArchives(root string, kind fileKind) ([]legacyPathMove, error) {
	historyRoot := filepath.Join(root, "history")
	entries, err := os.ReadDir(historyRoot)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	moves := make([]legacyPathMove, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".zip") {
			continue
		}
		monthKey := strings.TrimSuffix(strings.TrimSpace(entry.Name()), filepath.Ext(entry.Name()))
		if _, ok := parseHistoryMonth(monthKey); !ok {
			continue
		}
		extracted, err := extractLegacyMonthlyZipArchive(filepath.Join(historyRoot, entry.Name()), historyRoot, monthKey, kind)
		if err != nil {
			return moves, err
		}
		moves = append(moves, extracted...)
		if err := os.Remove(filepath.Join(historyRoot, entry.Name())); err != nil && !os.IsNotExist(err) {
			return moves, err
		}
	}
	return moves, nil
}

func extractLegacyMonthlyZipArchive(archivePath, historyRoot, monthKey string, kind fileKind) ([]legacyPathMove, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	moves := make([]legacyPathMove, 0)
	for _, entry := range reader.File {
		targetPath, oldPath, trackMove, err := legacyZipTargetPaths(historyRoot, monthKey, entry.Name, kind)
		if err != nil {
			return moves, err
		}
		if targetPath == "" {
			continue
		}
		if err := extractZipEntry(entry, targetPath); err != nil {
			return moves, err
		}
		if trackMove {
			moves = append(moves, legacyPathMove{
				oldPath: oldPath,
				newPath: targetPath,
			})
		}
	}
	return moves, nil
}

func legacyZipTargetPaths(historyRoot, monthKey, entryName string, kind fileKind) (targetPath string, oldPath string, trackMove bool, err error) {
	normalized := filepath.Clean(filepath.FromSlash(strings.TrimSpace(entryName)))
	if normalized == "." || normalized == "" {
		return "", "", false, nil
	}
	if strings.HasPrefix(normalized, ".."+string(filepath.Separator)) || filepath.IsAbs(normalized) {
		return "", "", false, fmt.Errorf("invalid legacy archive entry %q", entryName)
	}
	targetPath = filepath.Join(historyRoot, monthKey, normalized)
	oldPath = filepath.Join(historyRoot, normalized)
	ext := strings.ToLower(filepath.Ext(normalized))
	switch kind {
	case fileKindChat, fileKindPlan:
		trackMove = ext == ".md"
	default:
		trackMove = false
	}
	return targetPath, oldPath, trackMove, nil
}

func extractZipEntry(entry *zip.File, targetPath string) error {
	if entry.FileInfo().IsDir() {
		return os.MkdirAll(targetPath, 0o755)
	}
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	reader, err := entry.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	if _, err := io.Copy(file, reader); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func scanThreadDir(dir, threadRoot string) ([]*threadFileRef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	refs := make([]*threadFileRef, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".jsonl") {
			continue
		}
		ref, err := readThreadFileRef(filepath.Join(dir, entry.Name()), threadRoot, false)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func readThreadFileRef(filePath, threadRoot string, isLive bool) (*threadFileRef, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var (
		header *op.ThreadHeader
		leafID *string
	)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if header == nil {
			var h op.ThreadHeader
			if err := json.Unmarshal([]byte(line), &h); err != nil {
				return nil, err
			}
			header = &h
			continue
		}

		var base struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &base); err != nil {
			continue
		}
		if strings.TrimSpace(base.ID) != "" {
			leafID = stringPtr(base.ID)
		}
		if strings.TrimSpace(base.Type) != op.ThreadEntryTypeMetaUpdate {
			continue
		}
		var entry op.ThreadMetaUpdateEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}
		if strings.TrimSpace(entry.ChatPath) != "" {
			header.ChatPath = strings.TrimSpace(entry.ChatPath)
		}
		if strings.TrimSpace(entry.PlanPath) != "" {
			header.PlanPath = strings.TrimSpace(entry.PlanPath)
		}
		if strings.TrimSpace(entry.ExecutionPlanPath) != "" {
			header.ExecutionPlanPath = strings.TrimSpace(entry.ExecutionPlanPath)
		}
		if strings.TrimSpace(entry.Title) != "" {
			header.Title = strings.TrimSpace(entry.Title)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if header == nil {
		return nil, fmt.Errorf("thread header not found in %s", filePath)
	}

	return &threadFileRef{
		ThreadID:          strings.TrimSpace(header.ID),
		ChatPath:          normalizePath(header.ChatPath),
		PlanPath:          normalizePath(header.PlanPath),
		ExecutionPlanPath: normalizePath(header.ExecutionPlanPath),
		Title:             strings.TrimSpace(header.Title),
		FilePath:          filepath.Clean(filePath),
		ThreadRoot:        filepath.Clean(threadRoot),
		IsLive:            isLive,
		LeafID:            leafID,
	}, nil
}

func rewriteThreadFileMeta(filePath string, leafID *string, patch threadMetaPatch) (*string, bool, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return leafID, false, err
	}

	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return leafID, false, fmt.Errorf("thread header not found in %s", filePath)
	}

	var header op.ThreadHeader
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		return leafID, false, err
	}

	nextHeader := header
	entry := op.ThreadMetaUpdateEntry{}
	changed := false
	if patch.ChatPath != "" && normalizePath(header.ChatPath) != normalizePath(patch.ChatPath) {
		nextHeader.ChatPath = normalizePath(patch.ChatPath)
		entry.ChatPath = nextHeader.ChatPath
		changed = true
	}
	if patch.PlanPath != "" && normalizePath(header.PlanPath) != normalizePath(patch.PlanPath) {
		nextHeader.PlanPath = normalizePath(patch.PlanPath)
		entry.PlanPath = nextHeader.PlanPath
		changed = true
	}
	if patch.ExecutionPlanPath != "" && normalizePath(header.ExecutionPlanPath) != normalizePath(patch.ExecutionPlanPath) {
		nextHeader.ExecutionPlanPath = normalizePath(patch.ExecutionPlanPath)
		entry.ExecutionPlanPath = nextHeader.ExecutionPlanPath
		changed = true
	}
	if patch.Title != "" && strings.TrimSpace(header.Title) != strings.TrimSpace(patch.Title) {
		nextHeader.Title = strings.TrimSpace(patch.Title)
		entry.Title = nextHeader.Title
		changed = true
	}
	if !changed {
		return leafID, false, nil
	}

	entryID := uuid.NewString()[:8]
	entry.ThreadEntryBase = op.ThreadEntryBase{
		Type:      op.ThreadEntryTypeMetaUpdate,
		ID:        entryID,
		ParentID:  leafID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}

	headerRaw, err := json.Marshal(nextHeader)
	if err != nil {
		return leafID, false, err
	}
	lines[0] = string(headerRaw)
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	entryRaw, err := json.Marshal(entry)
	if err != nil {
		return leafID, false, err
	}
	lines = append(lines, string(entryRaw), "")
	if err := writeFileAtomic(filePath, []byte(strings.Join(lines, "\n"))); err != nil {
		return leafID, false, err
	}
	return stringPtr(entryID), true, nil
}

func scanTopLevelFiles(root string, entries []os.DirEntry, ext string) ([]fileRecord, error) {
	files := make([]fileRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, fileRecord{
			path:    filepath.Join(root, entry.Name()),
			name:    entry.Name(),
			modTime: info.ModTime(),
		})
	}
	return files, nil
}

func sortFileRecordsNewestFirst(files []fileRecord) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].name < files[j].name
		}
		return files[i].modTime.After(files[j].modTime)
	})
}

func listHistoryDayDirs(historyRoot string) ([]historyDayDir, error) {
	entries, err := os.ReadDir(historyRoot)
	if err != nil {
		return nil, err
	}
	dirs := make([]historyDayDir, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		day, ok := parseHistoryDay(entry.Name())
		if !ok {
			continue
		}
		dirs = append(dirs, historyDayDir{
			path: filepath.Join(historyRoot, entry.Name()),
			day:  day,
		})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].day.Before(dirs[j].day)
	})
	return dirs, nil
}

func listHistoryMonthDirs(historyRoot string) ([]historyMonthDir, error) {
	entries, err := os.ReadDir(historyRoot)
	if err != nil {
		return nil, err
	}
	dirs := make([]historyMonthDir, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		month, ok := parseHistoryMonth(entry.Name())
		if !ok {
			continue
		}
		dirs = append(dirs, historyMonthDir{
			path:  filepath.Join(historyRoot, entry.Name()),
			month: month,
		})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].month.After(dirs[j].month)
	})
	return dirs, nil
}

func parseHistoryDay(name string) (time.Time, bool) {
	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(name), time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return day, true
}

func parseHistoryMonth(name string) (time.Time, bool) {
	month, err := time.ParseInLocation("2006-01", strings.TrimSpace(name), time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return month, true
}

func historyDayDirForTime(root string, modTime time.Time) string {
	return filepath.Join(root, "history", modTime.In(time.Local).Format("2006-01-02"))
}

func historyMonthDayDir(root string, day time.Time) string {
	localDay := day.In(time.Local)
	return filepath.Join(root, "history", localDay.Format("2006-01"), localDay.Format("2006-01-02"))
}

func pruneMonthlyDirs(historyRoot string, currentMonth time.Time, retain int) (int, error) {
	monthDirs, err := listHistoryMonthDirs(historyRoot)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for index, monthDir := range monthDirs {
		if index < retain {
			continue
		}
		if err := os.RemoveAll(monthDir.path); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func compressOldMonthlyDirsIntoYearZips(historyRoot string, currentMonth time.Time, compressAfterMonths int, canCompress func(historyMonthDir) bool) (int, error) {
	monthDirs, err := listHistoryMonthDirs(historyRoot)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	threshold := startOfLocalMonth(currentMonth).AddDate(0, -compressAfterMonths, 0)
	compressed := 0
	for _, monthDir := range monthDirs {
		if !monthDir.month.Before(threshold) {
			continue
		}
		if canCompress != nil && !canCompress(monthDir) {
			continue
		}
		if err := compressMonthDirIntoYearZip(historyRoot, monthDir); err != nil {
			return compressed, err
		}
		if err := os.RemoveAll(monthDir.path); err != nil {
			return compressed, err
		}
		compressed++
	}
	return compressed, nil
}

func compressMonthDirIntoYearZip(historyRoot string, monthDir historyMonthDir) error {
	yearZipPath := filepath.Join(historyRoot, monthDir.month.Format("2006")+".zip")
	tempFile, err := os.CreateTemp(historyRoot, "."+filepath.Base(yearZipPath)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()

	writer := zip.NewWriter(tempFile)
	written := make(map[string]struct{})
	if err := copyExistingZipEntries(yearZipPath, writer, written); err != nil {
		_ = writer.Close()
		_ = tempFile.Close()
		return err
	}
	if err := addDirToZip(writer, monthDir.path, filepath.Base(monthDir.path), written); err != nil {
		_ = writer.Close()
		_ = tempFile.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, yearZipPath)
}

func copyExistingZipEntries(zipPath string, writer *zip.Writer, written map[string]struct{}) error {
	reader, err := zip.OpenReader(zipPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, entry := range reader.File {
		name := strings.TrimSpace(strings.ReplaceAll(entry.Name, "\\", "/"))
		if name == "" {
			continue
		}
		if _, exists := written[name]; exists {
			continue
		}
		if err := copyZipEntry(writer, entry); err != nil {
			return err
		}
		written[name] = struct{}{}
	}
	return nil
}

func copyZipEntry(writer *zip.Writer, entry *zip.File) error {
	reader, err := entry.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	header := entry.FileHeader
	target, err := writer.CreateHeader(&header)
	if err != nil {
		return err
	}
	if entry.FileInfo().IsDir() {
		return nil
	}
	_, err = io.Copy(target, reader)
	return err
}

func addDirToZip(writer *zip.Writer, dir string, zipPrefix string, written map[string]struct{}) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(zipPrefix, rel))
		if name == "." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
			return fmt.Errorf("invalid archive path: %s", name)
		}
		if _, exists := written[name]; exists {
			return fmt.Errorf("archive entry already exists: %s", name)
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = name
		header.Method = zip.Deflate
		target, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(target, source)
		closeErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		written[name] = struct{}{}
		return nil
	})
}

func pathIsInsideDir(path string, dir string) bool {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	cleanDir := filepath.Clean(strings.TrimSpace(dir))
	if cleanPath == "" || cleanDir == "" || cleanPath == "." || cleanDir == "." {
		return false
	}
	rel, err := filepath.Rel(cleanDir, cleanPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func startOfLocalDay(value time.Time) time.Time {
	localValue := value.In(time.Local)
	return time.Date(localValue.Year(), localValue.Month(), localValue.Day(), 0, 0, 0, 0, time.Local)
}

func startOfLocalMonth(value time.Time) time.Time {
	localValue := value.In(time.Local)
	return time.Date(localValue.Year(), localValue.Month(), 1, 0, 0, 0, 0, time.Local)
}

func allocateUniquePath(dir, name string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
		return candidate, nil
	} else if err != nil {
		return "", err
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; i < 10000; i++ {
		next := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if _, err := os.Stat(next); errors.Is(err, os.ErrNotExist) {
			return next, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("unable to allocate unique path for %q", name)
}

func moveChatWithAssets(sourcePath, threadID, targetPath string) error {
	sourcePath = filepath.Clean(sourcePath)
	targetPath = filepath.Clean(targetPath)
	chatBody, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := moveFileAtomic(sourcePath, targetPath); err != nil {
		return err
	}

	threadID = strings.TrimSpace(threadID)
	for _, relativeAssetPath := range collectReferencedRelativeAssets(string(chatBody), threadID) {
		sourceAssetPath := filepath.Join(filepath.Dir(sourcePath), filepath.FromSlash(relativeAssetPath))
		targetAssetPath := filepath.Join(filepath.Dir(targetPath), filepath.FromSlash(relativeAssetPath))
		if err := copyFileIfExists(sourceAssetPath, targetAssetPath); err != nil {
			return err
		}
	}

	if threadID != "" {
		sourceAssets := filepath.Join(filepath.Dir(sourcePath), "assets", threadID)
		if _, err := os.Stat(sourceAssets); err == nil {
			targetAssets := filepath.Join(filepath.Dir(targetPath), "assets", threadID)
			if err := moveDirAtomic(sourceAssets, targetAssets); err != nil {
				return err
			}
			cleanupEmptyParents(filepath.Dir(sourceAssets), filepath.Dir(sourcePath))
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func moveFileAtomic(sourcePath, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return os.Rename(sourcePath, targetPath)
}

func moveDirAtomic(sourcePath, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return os.Rename(sourcePath, targetPath)
}

func writeFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

func copyFileIfExists(sourcePath, targetPath string) error {
	sourceInfo, err := os.Stat(sourcePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if sourceInfo.IsDir() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return err
	}
	return target.Close()
}

func collectReferencedRelativeAssets(chatContent string, threadID string) []string {
	matches := flatAssetReferencePattern.FindAllString(chatContent, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	paths := make([]string, 0, len(matches))
	threadPrefix := strings.TrimSpace(threadID)
	for _, match := range matches {
		normalized := filepath.ToSlash(strings.TrimSpace(match))
		normalized = strings.TrimPrefix(normalized, "./")
		if !strings.HasPrefix(normalized, "assets/") {
			continue
		}
		relative := strings.TrimPrefix(normalized, "assets/")
		relative = strings.TrimPrefix(relative, "/")
		if relative == "" {
			continue
		}
		if threadPrefix != "" && strings.HasPrefix(relative, threadPrefix+"/") {
			continue
		}
		target := filepath.ToSlash(filepath.Join("assets", filepath.FromSlash(relative)))
		if _, exists := seen[target]; exists {
			continue
		}
		seen[target] = struct{}{}
		paths = append(paths, target)
	}
	sort.Strings(paths)
	return paths
}

func cleanupEmptyHistoryDir(dir string) error {
	if err := removeEmptyDirs(dir); err != nil {
		return err
	}
	return nil
}

func removeEmptyDirs(root string) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := removeEmptyDirs(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	entries, err = os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		if err := os.Remove(root); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func cleanupEmptyParents(startDir, stopDir string) {
	current := filepath.Clean(startDir)
	stop := filepath.Clean(stopDir)
	for current != stop && current != string(filepath.Separator) {
		entries, err := os.ReadDir(current)
		if err != nil || len(entries) != 0 {
			return
		}
		if err := os.Remove(current); err != nil {
			return
		}
		current = filepath.Dir(current)
	}
}

func stringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (r *runner) updateChatPathRefs(ctx context.Context, threadID, oldPath, newPath, title string) error {
	for _, thread := range r.allThreads {
		if thread == nil {
			continue
		}
		if thread.ThreadID != threadID && thread.ChatPath != oldPath {
			continue
		}
		if thread.ChatPath == newPath && (title == "" || thread.Title == title) {
			continue
		}
		if thread.IsLive {
			if _, err := r.core.UpdateThreadMeta(ctx, op.ThreadMetaUpdateParams{
				ThreadID: thread.ThreadID,
				ChatPath: newPath,
				Title:    title,
			}); err != nil {
				return err
			}
		} else {
			nextLeafID, changed, err := rewriteThreadFileMeta(thread.FilePath, thread.LeafID, threadMetaPatch{
				ChatPath: newPath,
				Title:    title,
			})
			if err != nil {
				return err
			}
			if !changed {
				continue
			}
			thread.LeafID = nextLeafID
		}
		thread.ChatPath = newPath
		if title != "" {
			thread.Title = title
		}
		r.result.RewrittenThreads++
	}
	return nil
}

func (r *runner) updatePlanPathRefs(ctx context.Context, oldPath, newPath string) error {
	for _, thread := range r.allThreads {
		if thread == nil {
			continue
		}

		patch := threadMetaPatch{}
		needsUpdate := false
		if thread.PlanPath == oldPath {
			patch.PlanPath = newPath
			needsUpdate = true
		}
		if thread.ExecutionPlanPath == oldPath {
			patch.ExecutionPlanPath = newPath
			needsUpdate = true
		}
		if !needsUpdate {
			continue
		}

		if thread.IsLive {
			if _, err := r.core.UpdateThreadMeta(ctx, op.ThreadMetaUpdateParams{
				ThreadID:          thread.ThreadID,
				PlanPath:          patch.PlanPath,
				ExecutionPlanPath: patch.ExecutionPlanPath,
			}); err != nil {
				return err
			}
		} else {
			nextLeafID, changed, err := rewriteThreadFileMeta(thread.FilePath, thread.LeafID, patch)
			if err != nil {
				return err
			}
			if !changed {
				continue
			}
			thread.LeafID = nextLeafID
		}
		if patch.PlanPath != "" {
			thread.PlanPath = patch.PlanPath
		}
		if patch.ExecutionPlanPath != "" {
			thread.ExecutionPlanPath = patch.ExecutionPlanPath
		}
		r.result.RewrittenThreads++
	}

	for _, chatRoot := range r.chatRoots {
		err := filepath.WalkDir(chatRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.EqualFold(filepath.Ext(path), ".md") {
				return nil
			}
			path = filepath.Clean(path)
			if _, open := r.openFiles[path]; open {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(raw)
			if !strings.Contains(text, oldPath) {
				return nil
			}
			next := strings.ReplaceAll(text, oldPath, newPath)
			if next == text {
				return nil
			}
			if err := writeFileAtomic(path, []byte(next)); err != nil {
				return err
			}
			r.result.RewrittenChats++
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
