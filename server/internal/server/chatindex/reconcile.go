package chatindex

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/agentctx"
)

func ReconcileFileIndex(cwd string) ([]FileRecord, error) {
	chatRoot := filepath.Join(strings.TrimSpace(cwd), ".agent", "chat")
	candidates, err := scanChatCandidates(chatRoot)
	if err != nil {
		if os.IsNotExist(err) {
			_ = WriteFileIndex(cwd, nil)
			return nil, nil
		}
		return nil, err
	}
	existing, err := ReadFileIndex(cwd)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	existingByThread := make(map[string]string, len(existing))
	existingAgentByThread := make(map[string]string, len(existing))
	for _, record := range existing {
		if record.ThreadID == "" || record.FileID == "" {
			continue
		}
		existingByThread[record.ThreadID] = record.FileID
		if record.AgentID != "" {
			existingAgentByThread[record.ThreadID] = record.AgentID
		}
	}
	bodyByThread, ambiguousBodies, err := scanThreadBodies(resolveBaseDir(cwd))
	if err != nil {
		return nil, err
	}

	threadToPaths := make(map[string][]string)
	for _, candidate := range candidates {
		threadToPaths[candidate.ThreadID] = append(threadToPaths[candidate.ThreadID], candidate.Path)
	}
	threads := make([]string, 0, len(threadToPaths))
	for threadID := range threadToPaths {
		threads = append(threads, threadID)
	}
	sort.Strings(threads)

	records := make([]FileRecord, 0, len(threads))
	for _, threadID := range threads {
		paths := threadToPaths[threadID]
		if len(paths) != 1 {
			continue
		}
		body := bodyByThread[threadID]
		if body.Record.ThreadID == "" || ambiguousBodies[threadID] {
			continue
		}
		fileID := strings.TrimSpace(existingByThread[threadID])
		if fileID == "" {
			fileID = GenerateFileID()
		}
		agentID := strings.TrimSpace(body.AgentID)
		if agentID == "" {
			agentID = strings.TrimSpace(existingAgentByThread[threadID])
		}
		if agentID == "" {
			continue
		}
		records = append(records, FileRecord{FileID: fileID, AgentID: agentID, ThreadID: threadID, CWD: filepath.Clean(strings.TrimSpace(cwd)), Path: paths[0]})
	}
	if err := WriteFileIndex(cwd, records); err != nil {
		return nil, err
	}
	return records, nil
}

func ReconcileThreadIndex(agentID string, fileRecords []FileRecord) ([]ThreadRecord, error) {
	threadRoot, err := ResolveAgentBaseDir(agentID)
	if err != nil {
		return nil, err
	}
	return ReconcileThreadIndexAtRoot(threadRoot, fileRecords)
}

func ReconcileThreadIndexAtRoot(threadRoot string, fileRecords []FileRecord) ([]ThreadRecord, error) {
	cleanRoot := filepath.Clean(strings.TrimSpace(threadRoot))
	candidates, err := scanThreadCandidates(cleanRoot)
	if err != nil {
		if os.IsNotExist(err) {
			if err := writeReconciledThreadIndexes(nil); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, err
	}
	fileIDByThread := make(map[string]string, len(fileRecords))
	for _, record := range fileRecords {
		if record.ThreadID == "" || record.FileID == "" {
			continue
		}
		fileIDByThread[record.ThreadID] = record.FileID
	}
	threadToPaths := make(map[string][]string)
	for _, candidate := range candidates {
		threadToPaths[candidate.ThreadID] = append(threadToPaths[candidate.ThreadID], candidate.Path)
	}
	threads := make([]string, 0, len(threadToPaths))
	for threadID := range threadToPaths {
		threads = append(threads, threadID)
	}
	sort.Strings(threads)
	records := make([]ThreadRecord, 0, len(threads))
	for _, threadID := range threads {
		fileID := strings.TrimSpace(fileIDByThread[threadID])
		paths := threadToPaths[threadID]
		if len(paths) != 1 {
			continue
		}
		var selected ThreadRecord
		for _, candidate := range candidates {
			if candidate.ThreadID == threadID && candidate.Path == paths[0] {
				selected = candidate
				break
			}
		}
		selected.FileID = fileID
		records = append(records, selected)
	}
	recordsByAgent := make(map[string][]ThreadRecord)
	for _, record := range records {
		agentID := strings.TrimSpace(record.AgentID)
		if agentID == "" {
			continue
		}
		recordsByAgent[agentID] = append(recordsByAgent[agentID], record)
	}
	if err := writeReconciledThreadIndexes(recordsByAgent); err != nil {
		return nil, err
	}
	return records, nil
}

func writeReconciledThreadIndexes(recordsByAgent map[string][]ThreadRecord) error {
	if recordsByAgent == nil {
		recordsByAgent = make(map[string][]ThreadRecord)
	}
	existingAgentIDs, err := listThreadIndexAgentIDs()
	if err != nil {
		return err
	}
	for _, agentID := range existingAgentIDs {
		if _, ok := recordsByAgent[agentID]; !ok {
			recordsByAgent[agentID] = nil
		}
	}
	agentIDs := make([]string, 0, len(recordsByAgent))
	for agentID := range recordsByAgent {
		agentIDs = append(agentIDs, agentID)
	}
	sort.Strings(agentIDs)
	for _, agentID := range agentIDs {
		if err := WriteThreadIndex(agentID, recordsByAgent[agentID]); err != nil {
			return err
		}
	}
	return nil
}

type chatCandidate struct {
	ThreadID string
	Path     string
}

func scanChatCandidates(chatRoot string) ([]chatCandidate, error) {
	candidates := make([]chatCandidate, 0)
	err := filepath.WalkDir(chatRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.EqualFold(d.Name(), "assets") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		meta, err := agentctx.ReadChatFileMeta(path)
		if err != nil {
			return nil
		}
		threadID := strings.TrimSpace(meta.ThreadID)
		if threadID == "" {
			return nil
		}
		candidates = append(candidates, chatCandidate{ThreadID: threadID, Path: filepath.Clean(path)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ThreadID == candidates[j].ThreadID {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].ThreadID < candidates[j].ThreadID
	})
	return candidates, nil
}

func scanThreadCandidates(threadRoot string) ([]ThreadRecord, error) {
	candidates := make([]ThreadRecord, 0)
	entries, err := os.ReadDir(threadRoot)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".jsonl") {
			continue
		}
		threadID := strings.TrimSpace(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		if threadID == "" {
			continue
		}
		path := filepath.Join(threadRoot, entry.Name())
		record := ThreadRecord{ThreadID: threadID, Path: filepath.Clean(path)}
		if header, err := readThreadHeader(path); err == nil && header != nil {
			if id := strings.TrimSpace(header.ID); id != "" {
				record.ThreadID = id
			}
			record.AgentID = strings.TrimSpace(header.AgentID)
			record.CWD = strings.TrimSpace(header.CWD)
			record.ChatPath = strings.TrimSpace(header.ChatPath)
			record.Title = strings.TrimSpace(header.Title)
			record.FileID = strings.TrimSpace(header.FileID)
		}
		candidates = append(candidates, record)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ThreadID == candidates[j].ThreadID {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].ThreadID < candidates[j].ThreadID
	})
	return candidates, nil
}

type threadBodyRecord struct {
	AgentID string
	Record  ThreadRecord
}

func FindThreadBody(threadID string) (string, ThreadRecord, error) {
	trimmedThreadID := strings.TrimSpace(threadID)
	if trimmedThreadID == "" {
		return "", ThreadRecord{}, os.ErrNotExist
	}
	bodies, ambiguous, err := scanThreadBodies(resolveBaseDir(""))
	if err != nil {
		return "", ThreadRecord{}, err
	}
	if ambiguous[trimmedThreadID] {
		return "", ThreadRecord{}, os.ErrNotExist
	}
	body := bodies[trimmedThreadID]
	if body.Record.ThreadID == "" {
		return "", ThreadRecord{}, os.ErrNotExist
	}
	return body.AgentID, body.Record, nil
}

func scanThreadBodies(baseDir string) (map[string]threadBodyRecord, map[string]bool, error) {
	threadsRoot := filepath.Join(filepath.Clean(strings.TrimSpace(baseDir)), "threads")
	entries, err := os.ReadDir(threadsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]threadBodyRecord{}, map[string]bool{}, nil
		}
		return nil, nil, err
	}
	byThread := make(map[string]threadBodyRecord)
	ambiguous := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".jsonl") {
			continue
		}
		path := filepath.Join(threadsRoot, entry.Name())
		candidate := ThreadRecord{
			ThreadID: strings.TrimSpace(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))),
			Path:     filepath.Clean(path),
		}
		if header, err := readThreadHeader(path); err == nil && header != nil {
			if id := strings.TrimSpace(header.ID); id != "" {
				candidate.ThreadID = id
			}
			candidate.AgentID = strings.TrimSpace(header.AgentID)
			candidate.CWD = strings.TrimSpace(header.CWD)
			candidate.ChatPath = strings.TrimSpace(header.ChatPath)
			candidate.Title = strings.TrimSpace(header.Title)
			candidate.FileID = strings.TrimSpace(header.FileID)
		}
		agentID := strings.TrimSpace(candidate.AgentID)
		threadID := strings.TrimSpace(candidate.ThreadID)
		if agentID == "" || threadID == "" || candidate.Path == "" {
			continue
		}
		record := threadBodyRecord{AgentID: agentID, Record: candidate}
		if existing, ok := byThread[threadID]; ok && existing.Record.Path != record.Record.Path {
			ambiguous[threadID] = true
			continue
		}
		byThread[threadID] = record
	}
	return byThread, ambiguous, nil
}

func readThreadHeader(path string) (*op.ThreadHeader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var header op.ThreadHeader
		if err := json.Unmarshal([]byte(line), &header); err != nil {
			return nil, err
		}
		return &header, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, os.ErrNotExist
}
