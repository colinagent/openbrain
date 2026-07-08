package core

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

func ensureThreadStorageReady() error {
	_, err := threadBaseDir()
	return err
}

func threadStorageRootDir(baseDir string) string {
	return filepath.Join(filepath.Clean(strings.TrimSpace(baseDir)), "threads")
}

func threadFilePathInRoot(threadRoot, threadID string) string {
	return filepath.Join(filepath.Clean(threadRoot), strings.TrimSpace(threadID)+".jsonl")
}

func threadReviewRootForFile(filePath string) string {
	return strings.TrimSuffix(filepath.Clean(filePath), ".jsonl") + ".review"
}

func threadIDFromPath(filePath string) string {
	name := filepath.Base(strings.TrimSpace(filePath))
	if !strings.HasSuffix(strings.ToLower(name), ".jsonl") {
		return ""
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func headerMatchesThreadQuery(header *op.ThreadHeader, query op.ThreadMetaQuery) bool {
	if header == nil {
		return false
	}
	threadID := strings.TrimSpace(query.ThreadID)
	if threadID != "" && strings.TrimSpace(header.ID) != threadID {
		return false
	}
	return true
}

func recordMatchesThreadQuery(record *threadRecord, query op.ThreadMetaQuery) bool {
	if record == nil {
		return false
	}
	return headerMatchesThreadQuery(&record.header, query)
}

func loadThreadRecordFromFile(filePath string) (*threadRecord, error) {
	header, leafID, err := readJSONLHeader(filePath)
	if err != nil {
		return nil, err
	}
	return threadRecordFromHeader(header, filePath, leafID), nil
}

type threadRetentionCandidate struct {
	threadID string
	filePath string
	modTime  time.Time
}

func maxThreads() int {
	sys := config.GetSystem()
	if sys != nil && sys.ThreadStorage.MaxThreads > 0 {
		return sys.ThreadStorage.MaxThreads
	}
	return config.DefaultMaxThreads
}

func pruneOldThreadFiles(threadRoot, keepThreadID string, maxThreads int) ([]string, error) {
	if maxThreads <= 0 {
		maxThreads = config.DefaultMaxThreads
	}
	entries, err := os.ReadDir(threadRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	candidates := make([]threadRetentionCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".jsonl") {
			continue
		}
		threadID := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if threadID == "" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, threadRetentionCandidate{
			threadID: threadID,
			filePath: filepath.Join(threadRoot, entry.Name()),
			modTime:  info.ModTime(),
		})
	}
	if len(candidates) <= maxThreads {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].modTime.Before(candidates[j].modTime)
		}
		return candidates[i].filePath < candidates[j].filePath
	})
	keepThreadID = strings.TrimSpace(keepThreadID)
	remaining := len(candidates)
	removed := make([]string, 0, remaining-maxThreads)
	for _, candidate := range candidates {
		if remaining <= maxThreads {
			break
		}
		if keepThreadID != "" && candidate.threadID == keepThreadID {
			continue
		}
		if err := os.Remove(candidate.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, err
		}
		reviewRoot := threadReviewRootForFile(candidate.filePath)
		if err := os.RemoveAll(reviewRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, err
		}
		removed = append(removed, candidate.threadID)
		remaining--
	}
	return removed, nil
}
