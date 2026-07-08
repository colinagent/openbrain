package chatindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const (
	chatFilesIndexName = "chat-files.json"
	indexVersion       = 1
)

var baseDirConfig struct {
	sync.RWMutex
	value string
}

type FileRecord struct {
	FileID   string
	AgentID  string
	ThreadID string
	CWD      string
	Path     string
}

type ThreadRecord struct {
	ThreadID string
	AgentID  string
	FileID   string
	CWD      string
	ChatPath string
	Path     string
	Title    string
}

type chatFilesIndex struct {
	Version int              `json:"version"`
	Files   []fileRecordJSON `json:"files"`
}

type fileRecordJSON struct {
	ID       string `json:"id"`
	AgentID  string `json:"agentID"`
	ThreadID string `json:"threadID"`
	CWD      string `json:"cwd"`
	ChatPath string `json:"chatPath"`
}

type threadIndex struct {
	Version int                `json:"version"`
	Threads []threadRecordJSON `json:"threads"`
}

type threadRecordJSON struct {
	ID       string `json:"id"`
	FileID   string `json:"fileID,omitempty"`
	CWD      string `json:"cwd,omitempty"`
	ChatPath string `json:"chatPath,omitempty"`
	BodyPath string `json:"bodyPath"`
	Title    string `json:"title,omitempty"`
}

func GenerateFileID() string {
	return op.GenerateFileID()
}

func SetBaseDir(baseDir string) {
	baseDirConfig.Lock()
	defer baseDirConfig.Unlock()
	baseDir = filepath.Clean(strings.TrimSpace(baseDir))
	if baseDir == "." {
		baseDir = ""
	}
	baseDirConfig.value = baseDir
}

func FileIndexPath(cwd string) string {
	return filepath.Join(resolveBaseDir(cwd), "index", chatFilesIndexName)
}

func ThreadIndexPath(agentID string) (string, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", fmt.Errorf("agentID is required")
	}
	if !strings.HasPrefix(agentID, "agent-") {
		return "", fmt.Errorf("agentID must start with agent-: %q", agentID)
	}
	return filepath.Join(resolveBaseDir(""), "index", "threads", agentID+".json"), nil
}

func listThreadIndexAgentIDs() ([]string, error) {
	indexRoot := filepath.Join(resolveBaseDir(""), "index", "threads")
	entries, err := os.ReadDir(indexRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	agentIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		agentID := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if !strings.HasPrefix(agentID, "agent-") {
			continue
		}
		agentIDs = append(agentIDs, agentID)
	}
	sort.Strings(agentIDs)
	return agentIDs, nil
}

func ResolveAgentBaseDir(agentID string) (string, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", fmt.Errorf("agentID is required")
	}
	if !strings.HasPrefix(agentID, "agent-") {
		return "", fmt.Errorf("agentID must start with agent-: %q", agentID)
	}
	return filepath.Join(resolveBaseDir(""), "threads"), nil
}

func ReadFileIndex(cwd string) ([]FileRecord, error) {
	path := FileIndexPath(cwd)
	idx, err := readChatFilesIndex(path)
	if err != nil {
		return nil, err
	}
	filterCWD := filepath.Clean(strings.TrimSpace(cwd))
	if filterCWD == "." {
		filterCWD = ""
	}
	out := make([]FileRecord, 0, len(idx.Files))
	for _, item := range idx.Files {
		rec := fileRecordFromJSON(item)
		if rec.FileID == "" || rec.ThreadID == "" || rec.Path == "" {
			continue
		}
		if filterCWD != "" && rec.CWD != "" && filepath.Clean(rec.CWD) != filterCWD {
			continue
		}
		out = append(out, rec)
	}
	sortFileRecords(out)
	return out, nil
}

func WriteFileIndex(cwd string, records []FileRecord) error {
	indexPath := FileIndexPath(cwd)
	existing, err := readChatFilesIndex(indexPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	filterCWD := filepath.Clean(strings.TrimSpace(cwd))
	if filterCWD == "." {
		filterCWD = ""
	}
	next := make([]FileRecord, 0, len(existing.Files)+len(records))
	for _, item := range existing.Files {
		rec := fileRecordFromJSON(item)
		if filterCWD != "" && filepath.Clean(rec.CWD) == filterCWD {
			continue
		}
		next = append(next, rec)
	}
	next = append(next, records...)
	return writeChatFiles(indexPath, next)
}

func ResolveFileRecord(cwd, fileID string) (*FileRecord, error) {
	records, err := ReadFileIndex(cwd)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(fileID)
	for _, record := range records {
		if record.FileID == trimmed {
			copy := record
			return &copy, nil
		}
	}
	return nil, os.ErrNotExist
}

func FindFileRecordByThreadID(cwd, threadID string) (*FileRecord, error) {
	records, err := ReadFileIndex(cwd)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(threadID)
	for _, record := range records {
		if record.ThreadID == trimmed {
			copy := record
			return &copy, nil
		}
	}
	return nil, os.ErrNotExist
}

func UpsertFileRecord(cwd string, record FileRecord) error {
	record = normalizeFileRecord(record)
	if record.CWD == "" {
		record.CWD = filepath.Clean(strings.TrimSpace(cwd))
	}
	if record.FileID == "" || record.ThreadID == "" || record.Path == "" {
		return fmt.Errorf("file record requires fileID, threadID, and path")
	}
	records, err := ReadFileIndex(record.CWD)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	next := make([]FileRecord, 0, len(records)+1)
	replaced := false
	for _, existing := range records {
		if existing.FileID == record.FileID {
			next = append(next, record)
			replaced = true
			continue
		}
		next = append(next, existing)
	}
	if !replaced {
		next = append(next, record)
	}
	return WriteFileIndex(record.CWD, next)
}

func RemoveFileRecord(cwd, fileID string) error {
	records, err := ReadFileIndex(cwd)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	trimmed := strings.TrimSpace(fileID)
	next := make([]FileRecord, 0, len(records))
	for _, record := range records {
		if record.FileID == trimmed {
			continue
		}
		next = append(next, record)
	}
	return WriteFileIndex(cwd, next)
}

func ReadThreadIndex(agentID string) ([]ThreadRecord, error) {
	path, err := ThreadIndexPath(agentID)
	if err != nil {
		return nil, err
	}
	records, err := readThreadIndexAtPath(path)
	if err != nil {
		return nil, err
	}
	for i := range records {
		records[i].AgentID = strings.TrimSpace(agentID)
	}
	return records, nil
}

func WriteThreadIndex(agentID string, records []ThreadRecord) error {
	path, err := ThreadIndexPath(agentID)
	if err != nil {
		return err
	}
	return writeThreadIndexAtPath(path, records)
}

func ResolveThreadRecord(agentID, threadID string) (*ThreadRecord, error) {
	records, err := ReadThreadIndex(agentID)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(threadID)
	for _, record := range records {
		if record.ThreadID == trimmed {
			copy := record
			return &copy, nil
		}
	}
	return nil, os.ErrNotExist
}

func FindThreadRecordByThreadIDAtRoot(threadRoot, threadID string) (*ThreadRecord, error) {
	candidates, err := scanThreadCandidates(threadRoot)
	if err != nil {
		return nil, err
	}
	trimmedThreadID := strings.TrimSpace(threadID)
	for _, candidate := range candidates {
		if candidate.ThreadID != trimmedThreadID {
			continue
		}
		copy := candidate
		return &copy, nil
	}
	return nil, os.ErrNotExist
}

func UpsertThreadRecord(agentID string, record ThreadRecord) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		agentID = strings.TrimSpace(record.AgentID)
	}
	if agentID == "" {
		return fmt.Errorf("agentID is required")
	}
	records, err := ReadThreadIndex(agentID)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	record = normalizeThreadRecord(record)
	if record.ThreadID == "" || record.Path == "" {
		return fmt.Errorf("thread record requires threadID and path")
	}
	next := make([]ThreadRecord, 0, len(records)+1)
	replaced := false
	for _, existing := range records {
		if existing.ThreadID == record.ThreadID {
			next = append(next, record)
			replaced = true
			continue
		}
		next = append(next, existing)
	}
	if !replaced {
		next = append(next, record)
	}
	return WriteThreadIndex(agentID, next)
}

func UpsertThreadRecordForThreadFile(threadFilePath string, record ThreadRecord) error {
	threadFilePath = filepath.Clean(strings.TrimSpace(threadFilePath))
	if threadFilePath == "" || threadFilePath == "." {
		return fmt.Errorf("threadFilePath is required")
	}
	if record.Path == "" {
		record.Path = threadFilePath
	}
	return UpsertThreadRecord(record.AgentID, record)
}

func UpsertThreadRecordAtRoot(threadRoot string, record ThreadRecord) error {
	return UpsertThreadRecord(record.AgentID, record)
}

func RemoveThreadRecord(agentID, threadID string) error {
	records, err := ReadThreadIndex(agentID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	trimmed := strings.TrimSpace(threadID)
	next := make([]ThreadRecord, 0, len(records))
	for _, record := range records {
		if record.ThreadID == trimmed {
			continue
		}
		next = append(next, record)
	}
	return WriteThreadIndex(agentID, next)
}

func readThreadIndexAtPath(path string) ([]ThreadRecord, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var idx threadIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, err
	}
	out := make([]ThreadRecord, 0, len(idx.Threads))
	for _, item := range idx.Threads {
		rec := normalizeThreadRecord(ThreadRecord{
			ThreadID: item.ID,
			FileID:   item.FileID,
			CWD:      item.CWD,
			ChatPath: item.ChatPath,
			Path:     pathFromIndexValue(item.BodyPath),
			Title:    item.Title,
		})
		if rec.ThreadID == "" || rec.Path == "" {
			continue
		}
		out = append(out, rec)
	}
	sortThreadRecords(out)
	return out, nil
}

func writeThreadIndexAtPath(path string, records []ThreadRecord) error {
	normalized := make([]ThreadRecord, 0, len(records))
	for _, record := range records {
		record = normalizeThreadRecord(record)
		if record.ThreadID == "" || record.Path == "" {
			continue
		}
		normalized = append(normalized, record)
	}
	sortThreadRecords(normalized)
	items := make([]threadRecordJSON, 0, len(normalized))
	for _, rec := range normalized {
		items = append(items, threadRecordJSON{
			ID:       rec.ThreadID,
			FileID:   rec.FileID,
			CWD:      rec.CWD,
			ChatPath: rec.ChatPath,
			BodyPath: indexPathValue(rec.Path),
			Title:    rec.Title,
		})
	}
	return writeJSONAtomic(path, threadIndex{Version: indexVersion, Threads: items})
}

func readChatFilesIndex(path string) (chatFilesIndex, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return chatFilesIndex{Version: indexVersion}, err
	}
	var idx chatFilesIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return chatFilesIndex{}, err
	}
	return idx, nil
}

func writeChatFiles(path string, records []FileRecord) error {
	normalized := make([]FileRecord, 0, len(records))
	for _, record := range records {
		record = normalizeFileRecord(record)
		if record.FileID == "" || record.ThreadID == "" || record.Path == "" {
			continue
		}
		normalized = append(normalized, record)
	}
	sortFileRecords(normalized)
	items := make([]fileRecordJSON, 0, len(normalized))
	for _, rec := range normalized {
		items = append(items, fileRecordJSON{
			ID:       rec.FileID,
			AgentID:  rec.AgentID,
			ThreadID: rec.ThreadID,
			CWD:      rec.CWD,
			ChatPath: rec.Path,
		})
	}
	return writeJSONAtomic(path, chatFilesIndex{Version: indexVersion, Files: items})
}

func fileRecordFromJSON(item fileRecordJSON) FileRecord {
	return normalizeFileRecord(FileRecord{
		FileID:   item.ID,
		AgentID:  item.AgentID,
		ThreadID: item.ThreadID,
		CWD:      item.CWD,
		Path:     item.ChatPath,
	})
}

func sortFileRecords(records []FileRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].FileID == records[j].FileID {
			if records[i].ThreadID == records[j].ThreadID {
				return records[i].Path < records[j].Path
			}
			return records[i].ThreadID < records[j].ThreadID
		}
		return records[i].FileID < records[j].FileID
	})
}

func sortThreadRecords(records []ThreadRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].ThreadID == records[j].ThreadID {
			if records[i].FileID == records[j].FileID {
				return records[i].Path < records[j].Path
			}
			return records[i].FileID < records[j].FileID
		}
		return records[i].ThreadID < records[j].ThreadID
	})
}

func normalizeFileRecord(record FileRecord) FileRecord {
	return FileRecord{
		FileID:   strings.TrimSpace(record.FileID),
		AgentID:  strings.TrimSpace(record.AgentID),
		ThreadID: strings.TrimSpace(record.ThreadID),
		CWD:      cleanOptionalPath(record.CWD),
		Path:     cleanOptionalPath(record.Path),
	}
}

func normalizeThreadRecord(record ThreadRecord) ThreadRecord {
	return ThreadRecord{
		ThreadID: strings.TrimSpace(record.ThreadID),
		AgentID:  strings.TrimSpace(record.AgentID),
		FileID:   strings.TrimSpace(record.FileID),
		CWD:      cleanOptionalPath(record.CWD),
		ChatPath: cleanOptionalPath(record.ChatPath),
		Path:     cleanOptionalPath(record.Path),
		Title:    strings.TrimSpace(record.Title),
	}
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func resolveBaseDir(hint string) string {
	baseDirConfig.RLock()
	configured := baseDirConfig.value
	baseDirConfig.RUnlock()
	if configured != "" {
		return configured
	}
	if baseDir := strings.TrimSpace(os.Getenv("OPENBRAIN_BASE_DIR")); baseDir != "" {
		return filepath.Clean(baseDir)
	}
	if baseDir := inferBaseDirFromPath(hint); baseDir != "" {
		return baseDir
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".openbrain")
	}
	return "."
}

func inferBaseDirFromPath(path string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" || cleaned == "." {
		return ""
	}
	parts := strings.Split(cleaned, string(filepath.Separator))
	for i, part := range parts {
		if part == ".openbrain" {
			prefix := filepath.Join(parts[:i+1]...)
			if filepath.IsAbs(cleaned) {
				return string(filepath.Separator) + prefix
			}
			return prefix
		}
	}
	return ""
}

func cleanOptionalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func indexPathValue(path string) string {
	path = cleanOptionalPath(path)
	baseDir := resolveBaseDir(path)
	if rel, err := filepath.Rel(baseDir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return path
}

func pathFromIndexValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(resolveBaseDir(""), filepath.FromSlash(value))
}
