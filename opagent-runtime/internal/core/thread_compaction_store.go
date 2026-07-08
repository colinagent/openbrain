package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

const sessionEntryTypeCompaction = "compaction"

func canonicalMessageFromCompactionEntry(entry op.ThreadCompactionEntry) ai.ConversationMessage {
	summary := strings.TrimSpace(entry.Summary)
	if summary == "" {
		summary = "Earlier conversation history was truncated because compaction summary is unavailable."
	}
	return ai.ConversationMessage{
		Role: ai.RoleCanonicalSystem,
		Content: []ai.ContentBlock{{
			Type: ai.BlockText,
			Text: "Context checkpoint summary:\n" + summary,
		}},
	}
}

func compactionSummaryFromCanonicalMessage(message ai.ConversationMessage) string {
	text := strings.TrimSpace(canonicalSummaryText(message))
	return strings.TrimSpace(strings.TrimPrefix(text, "Context checkpoint summary:"))
}

func replaceThreadCanonicalMessagesWithCompaction(meta op.ThreadMeta, compacted []ai.ConversationMessage, tokensBefore int64) error {
	if len(compacted) == 0 {
		return nil
	}
	record, err := defaultThreadStore.loadRecord(threadMetaQuery(meta))
	if err != nil {
		return err
	}
	threadID := strings.TrimSpace(record.header.ID)
	lock := defaultThreadStore.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, _, err := readJSONLHeader(record.filePath)
	if err != nil {
		return err
	}

	oldIDs, err := canonicalEntryIDs(record.filePath)
	if err != nil {
		return err
	}
	firstKeptEntryID := ""
	keptCount := len(compacted) - 1
	if keptCount > 0 && keptCount <= len(oldIDs) {
		firstKeptEntryID = oldIDs[len(oldIDs)-keptCount]
	}

	entries := make([]any, 0, len(compacted))
	compactionID := generateThreadEntryID()
	entries = append(entries, op.ThreadCompactionEntry{
		ThreadEntryBase: op.ThreadEntryBase{
			Type:      sessionEntryTypeCompaction,
			ID:        compactionID,
			ParentID:  nil,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		},
		Summary:          compactionSummaryFromCanonicalMessage(compacted[0]),
		FirstKeptEntryID: firstKeptEntryID,
		TokensBefore:     tokensBefore,
	})
	parentID := stringPtr(compactionID)
	for _, msg := range compacted[1:] {
		entryID := generateThreadEntryID()
		entries = append(entries, op.ThreadCanonicalMessageEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeCanonicalMessage,
				ID:        entryID,
				ParentID:  parentID,
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			Message: msg,
		})
		parentID = stringPtr(entryID)
	}

	preservedLines, err := nonCanonicalThreadEntryLines(record.filePath)
	if err != nil {
		return err
	}
	if err := rewriteThreadMessages(record.filePath, *header, entries, preservedLines); err != nil {
		return err
	}
	record.header = *header
	_, leafID, err := readJSONLHeader(record.filePath)
	if err != nil {
		return err
	}
	record.leafID = leafID
	defaultThreadStore.cacheRecord(record)
	return nil
}

func canonicalEntryIDs(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	ids := make([]string, 0)
	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNo++
		if lineNo == 1 {
			continue
		}
		var base struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &base); err != nil {
			continue
		}
		if strings.TrimSpace(base.Type) == op.ThreadEntryTypeCanonicalMessage && strings.TrimSpace(base.ID) != "" {
			ids = append(ids, strings.TrimSpace(base.ID))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func nonCanonicalThreadEntryLines(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	lines := make([]string, 0)
	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNo++
		if lineNo == 1 {
			continue
		}
		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &base); err != nil {
			continue
		}
		switch strings.TrimSpace(base.Type) {
		case op.ThreadEntryTypeCanonicalMessage, sessionEntryTypeCompaction:
			continue
		default:
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func rewriteThreadMessages(filePath string, header op.ThreadHeader, entries []any, preservedLines []string) error {
	var buf bytes.Buffer
	if err := writeJSONLineToBuffer(&buf, header); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := writeJSONLineToBuffer(&buf, entry); err != nil {
			return err
		}
	}
	for _, line := range preservedLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, buf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func writeJSONLineToBuffer(buf *bytes.Buffer, value any) error {
	if buf == nil {
		return fmt.Errorf("buffer is nil")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	buf.Write(raw)
	buf.WriteByte('\n')
	return nil
}
