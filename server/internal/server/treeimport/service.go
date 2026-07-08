package treeimport

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const sessionTTL = 10 * time.Minute

var (
	ErrSessionNotFound = errors.New("tree import session not found")
	ErrPathConflict    = errors.New("tree import path kind conflict")
)

type GrantResolver interface {
	ResolveGrant(token, authority string) ([]string, error)
}

type Service struct {
	baseDir       string
	grantResolver GrantResolver

	mu       sync.RWMutex
	sessions map[string]*sessionRecord
}

func NewService(baseDir string, grantResolver GrantResolver) *Service {
	return &Service{
		baseDir:       strings.TrimSpace(baseDir),
		grantResolver: grantResolver,
		sessions:      make(map[string]*sessionRecord),
	}
}

func (s *Service) CreateSession(req CreateTreeImportSessionRequest) (*CreateTreeImportSessionResponse, error) {
	if s.grantResolver == nil {
		return nil, fmt.Errorf("grant resolver is not configured")
	}

	targetDir, err := normalizeAbsolutePath(req.TargetDir)
	if err != nil {
		return nil, err
	}
	roots, err := s.grantResolver.ResolveGrant(req.GrantToken, "")
	if err != nil {
		return nil, err
	}
	if !isAllowedPath(targetDir, roots) {
		return nil, fmt.Errorf("access denied")
	}

	entries, err := validateEntries(req.Entries)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredSessionsLocked()

	sessionID := "tis-" + uuid.NewString()
	stageDir := filepath.Join(s.baseDir, "resources", "transfers", "tree-import", sessionID, "staging")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return nil, err
	}

	record := &sessionRecord{
		ID:        sessionID,
		TargetDir: targetDir,
		StageDir:  stageDir,
		Entries:   entries,
		ExpiresAt: time.Now().Add(sessionTTL),
	}
	s.sessions[sessionID] = record

	return &CreateTreeImportSessionResponse{
		SessionID:     sessionID,
		UploadBaseURL: "/v1/tree-import/sessions/" + sessionID + "/files",
		Conflicts:     detectConflicts(targetDir, entries),
	}, nil
}

func (s *Service) UploadFile(sessionID, relativePath string, body io.Reader, contentLength int64) error {
	normalizedRelativePath, err := normalizeRelativePath(relativePath)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.cleanupExpiredSessionsLocked()
	record, ok := s.sessions[strings.TrimSpace(sessionID)]
	s.mu.Unlock()
	if !ok {
		return ErrSessionNotFound
	}

	entry, ok := record.Entries[normalizedRelativePath]
	if !ok || entry.Kind != EntryKindFile {
		return fmt.Errorf("file is not declared in the import manifest")
	}
	if contentLength >= 0 && contentLength != entry.Size {
		return fmt.Errorf("content length mismatch")
	}

	stagePath := filepath.Join(record.StageDir, filepath.FromSlash(normalizedRelativePath))
	if err := os.MkdirAll(filepath.Dir(stagePath), 0o755); err != nil {
		return err
	}
	tmpPath := stagePath + ".part"
	_ = os.Remove(tmpPath)

	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(body, entry.Size+1))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if written != entry.Size {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("written size mismatch")
	}
	if err := os.Rename(tmpPath, stagePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (s *Service) CommitSession(sessionID string, overwrite bool) (*CommitTreeImportResponse, error) {
	s.mu.Lock()
	s.cleanupExpiredSessionsLocked()
	record, ok := s.sessions[strings.TrimSpace(sessionID)]
	s.mu.Unlock()
	if !ok {
		return nil, ErrSessionNotFound
	}

	conflicts := detectConflicts(record.TargetDir, record.Entries)
	if len(conflicts) > 0 && !overwrite {
		return nil, &ConflictError{Paths: conflicts}
	}

	importedFiles := 0
	importedDirs := 0
	sortedEntries := sortEntries(record.Entries)
	for _, entry := range sortedEntries {
		targetPath := filepath.Join(record.TargetDir, filepath.FromSlash(entry.RelativePath))
		info, statErr := os.Stat(targetPath)
		targetExists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}

		switch entry.Kind {
		case EntryKindDir:
			if targetExists && !info.IsDir() {
				return nil, ErrPathConflict
			}
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return nil, err
			}
			importedDirs++

		case EntryKindFile:
			if targetExists && info.IsDir() {
				return nil, ErrPathConflict
			}
			stagePath := filepath.Join(record.StageDir, filepath.FromSlash(entry.RelativePath))
			if _, err := os.Stat(stagePath); err != nil {
				if os.IsNotExist(err) && entry.Size == 0 {
					if err := os.MkdirAll(filepath.Dir(stagePath), 0o755); err != nil {
						return nil, err
					}
					if err := os.WriteFile(stagePath, nil, 0o644); err != nil {
						return nil, err
					}
				} else {
					return nil, fmt.Errorf("missing uploaded file for %s", entry.RelativePath)
				}
			}
			if err := copyFile(stagePath, targetPath); err != nil {
				return nil, err
			}
			importedFiles++
		}
	}

	if err := s.deleteSession(sessionID); err != nil {
		return nil, err
	}
	return &CommitTreeImportResponse{
		ImportedFiles: importedFiles,
		ImportedDirs:  importedDirs,
	}, nil
}

func (s *Service) CancelSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredSessionsLocked()
	return s.deleteSessionLocked(strings.TrimSpace(sessionID))
}

func (s *Service) deleteSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteSessionLocked(strings.TrimSpace(sessionID))
}

func (s *Service) deleteSessionLocked(sessionID string) error {
	record, ok := s.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	delete(s.sessions, sessionID)
	return os.RemoveAll(filepath.Dir(record.StageDir))
}

func (s *Service) cleanupExpiredSessionsLocked() {
	now := time.Now()
	expired := make([]string, 0)
	for id, record := range s.sessions {
		if now.After(record.ExpiresAt) {
			expired = append(expired, id)
		}
	}
	for _, id := range expired {
		record := s.sessions[id]
		delete(s.sessions, id)
		_ = os.RemoveAll(filepath.Dir(record.StageDir))
	}
}

func validateEntries(rawEntries []TreeImportEntry) (map[string]sessionEntry, error) {
	if len(rawEntries) == 0 {
		return nil, fmt.Errorf("entries are required")
	}

	entries := make(map[string]sessionEntry, len(rawEntries))
	for _, raw := range rawEntries {
		relativePath, err := normalizeRelativePath(raw.RelativePath)
		if err != nil {
			return nil, err
		}

		switch raw.Kind {
		case EntryKindFile:
			if raw.Size == nil || *raw.Size < 0 {
				return nil, fmt.Errorf("file size is required for %s", relativePath)
			}
			if _, exists := entries[relativePath]; exists {
				return nil, fmt.Errorf("duplicate manifest path: %s", relativePath)
			}
			entries[relativePath] = sessionEntry{
				Kind:         EntryKindFile,
				RelativePath: relativePath,
				Size:         *raw.Size,
			}

		case EntryKindDir:
			if _, exists := entries[relativePath]; exists {
				return nil, fmt.Errorf("duplicate manifest path: %s", relativePath)
			}
			entries[relativePath] = sessionEntry{
				Kind:         EntryKindDir,
				RelativePath: relativePath,
			}

		default:
			return nil, fmt.Errorf("unsupported entry kind %q", raw.Kind)
		}
	}

	paths := make([]string, 0, len(entries))
	for relativePath := range entries {
		paths = append(paths, relativePath)
	}
	sort.Strings(paths)
	for _, relativePath := range paths {
		parent := path.Dir(relativePath)
		for parent != "." && parent != "/" {
			if existing, ok := entries[parent]; ok && existing.Kind != EntryKindDir {
				return nil, fmt.Errorf("manifest path %s conflicts with file ancestor %s", relativePath, parent)
			}
			parent = path.Dir(parent)
		}
	}

	return entries, nil
}

func normalizeRelativePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if trimmed == "" {
		return "", fmt.Errorf("relative path is required")
	}
	if strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("relative path must not be absolute")
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("relative path is required")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("relative path must stay within the drop root")
	}
	return cleaned, nil
}

func normalizeAbsolutePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("targetDir is required")
	}
	cleaned := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("targetDir must be absolute")
	}
	return cleaned, nil
}

func isAllowedPath(target string, roots []string) bool {
	cleanTarget := filepath.Clean(target)
	for _, root := range roots {
		cleanRoot := filepath.Clean(strings.TrimSpace(root))
		if cleanRoot == "" {
			continue
		}
		if cleanTarget == cleanRoot || strings.HasPrefix(cleanTarget, cleanRoot+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func detectConflicts(targetDir string, entries map[string]sessionEntry) []string {
	paths := make([]string, 0)
	for relativePath := range entries {
		paths = append(paths, relativePath)
	}
	sort.Strings(paths)

	conflicts := make([]string, 0)
	for _, relativePath := range paths {
		targetPath := filepath.Join(targetDir, filepath.FromSlash(relativePath))
		if _, err := os.Stat(targetPath); err == nil {
			conflicts = append(conflicts, relativePath)
		}
	}
	return conflicts
}

func sortEntries(entries map[string]sessionEntry) []sessionEntry {
	sorted := make([]sessionEntry, 0, len(entries))
	for _, entry := range entries {
		sorted = append(sorted, entry)
	}
	sort.Slice(sorted, func(i, j int) bool {
		leftDepth := strings.Count(sorted[i].RelativePath, "/")
		rightDepth := strings.Count(sorted[j].RelativePath, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		if sorted[i].Kind != sorted[j].Kind {
			return sorted[i].Kind == EntryKindDir
		}
		return sorted[i].RelativePath < sorted[j].RelativePath
	})
	return sorted
}

func copyFile(sourcePath, targetPath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}
	if sourceInfo.IsDir() {
		return fmt.Errorf("source is a directory: %s", sourcePath)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	tmpPath := targetPath + ".tree-import-" + uuid.NewString()
	_ = os.Remove(tmpPath)
	targetFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, sourceInfo.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		targetFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := targetFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
