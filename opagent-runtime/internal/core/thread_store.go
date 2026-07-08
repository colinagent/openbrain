package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/packages/agentctx"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/google/uuid"
	"github.com/rs/xid"
)

const (
	currentThreadVersion         = 2
	defaultSnapshotEntryLimit    = 400
	maxSnapshotEntryLimit        = 1000
	snapshotPaginationEntryLimit = 200
)

type threadRecord struct {
	header   op.ThreadHeader
	filePath string
	leafID   *string
}

type threadStore struct {
	mu        sync.RWMutex
	byThread  map[string]*threadRecord
	threadMux map[string]*sync.Mutex
}

type threadContext struct {
	meta              op.ThreadMeta
	entries           []op.ThreadEntry
	canonicalMessages []ai.ConversationMessage
	queuedMessages    op.ThreadQueueSnapshot
	messageState      threadMessageState
}

type threadSnapshotContext struct {
	meta               op.ThreadMeta
	entries            []op.ThreadEntry
	entryWindow        op.ThreadEntryWindow
	revision           string
	tailStatus         op.ThreadTailStatus
	continuationReason op.ThreadContinuationReason
	queuedMessages     op.ThreadQueueSnapshot
	messageState       threadMessageState
	contextUsage       ai.ThreadContextUsage
}

type snapshotContextUsageState struct {
	totalEstimatedTokens        int64
	latestUsageTokens           int64
	trailingEstimatedTokens     int64
	hasValidUsage               bool
	checkpointAfterLatestUsage  bool
	providerModelIDsLatestFirst []string
}

var defaultThreadStore = &threadStore{
	byThread:  make(map[string]*threadRecord),
	threadMux: make(map[string]*sync.Mutex),
}

func normalizeThreadPath(path string) string {
	return strings.TrimSpace(path)
}

func normalizeThreadValue(value string) string {
	return strings.TrimSpace(value)
}

func normalizeThreadPositiveInt64(value int64) int64 {
	if value > 0 {
		return value
	}
	return 0
}

func normalizeThreadIDValue(value string) string {
	return strings.TrimSpace(value)
}

func stringPtr(value string) *string {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	return &v
}

func generateQueueItemID() string {
	return fmt.Sprintf("queue-%s", xid.New().String())
}

func cloneThreadQueueItem(item op.ThreadQueueItem) op.ThreadQueueItem {
	return op.ThreadQueueItem{
		ID:                   strings.TrimSpace(item.ID),
		Message:              item.Message,
		AgentID:              normalizeThreadAgentID(item.AgentID),
		AgentName:            normalizeThreadValue(item.AgentName),
		CWD:                  normalizeThreadPath(item.CWD),
		ModelKey:             normalizeThreadValue(item.ModelKey),
		ThinkingLevel:        normalizeThreadValue(item.ThinkingLevel),
		ContextWindow:        normalizeThreadPositiveInt64(item.ContextWindow),
		ServiceTier:          normalizeThreadValue(item.ServiceTier),
		SelectedSkillIDs:     append([]string(nil), item.SelectedSkillIDs...),
		SelectedSkillContext: item.SelectedSkillContext.Clone(),
		PlanTurn:             item.PlanTurn,
	}
}

func cloneThreadQueueItems(items []op.ThreadQueueItem) []op.ThreadQueueItem {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]op.ThreadQueueItem, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		cloned = append(cloned, cloneThreadQueueItem(item))
	}
	return cloned
}

func cloneThreadQueueSnapshot(snapshot op.ThreadQueueSnapshot) op.ThreadQueueSnapshot {
	return op.ThreadQueueSnapshot{
		Steering: cloneThreadQueueItems(snapshot.Steering),
		FollowUp: cloneThreadQueueItems(snapshot.FollowUp),
	}
}

func cloneThreadEntries(entries []op.ThreadEntry) []op.ThreadEntry {
	if len(entries) == 0 {
		return nil
	}
	cloned := make([]op.ThreadEntry, 0, len(entries))
	for _, entry := range entries {
		cloned = append(cloned, cloneThreadEntry(entry))
	}
	return cloned
}

func cloneThreadEntry(entry op.ThreadEntry) op.ThreadEntry {
	next := entry
	if entry.ParentID != nil {
		next.ParentID = stringPtr(*entry.ParentID)
	}
	next.Raw = append([]byte(nil), entry.Raw...)
	return next
}

func threadSnapshotRevision(entries []op.ThreadEntry, fallback string) string {
	for i := len(entries) - 1; i >= 0; i-- {
		if id := strings.TrimSpace(entries[i].ID); id != "" {
			return id
		}
	}
	return strings.TrimSpace(fallback)
}

func appendThreadQueueItem(snapshot *op.ThreadQueueSnapshot, queueKind op.ThreadQueueKind, item op.ThreadQueueItem) {
	if snapshot == nil {
		return
	}
	cloned := cloneThreadQueueItem(item)
	switch queueKind {
	case op.ThreadQueueKindSteering:
		snapshot.Steering = append(snapshot.Steering, cloned)
	case op.ThreadQueueKindFollowUp:
		snapshot.FollowUp = append(snapshot.FollowUp, cloned)
	}
}

func removeThreadQueueItem(items []op.ThreadQueueItem, itemID string) ([]op.ThreadQueueItem, *op.ThreadQueueItem) {
	trimmedID := strings.TrimSpace(itemID)
	if trimmedID == "" || len(items) == 0 {
		return items, nil
	}
	for index, item := range items {
		if strings.TrimSpace(item.ID) != trimmedID {
			continue
		}
		removed := cloneThreadQueueItem(item)
		items = append(append([]op.ThreadQueueItem(nil), items[:index]...), items[index+1:]...)
		return items, &removed
	}
	return items, nil
}

func removeThreadQueueSnapshotItem(snapshot *op.ThreadQueueSnapshot, queueKind op.ThreadQueueKind, itemID string) *op.ThreadQueueItem {
	if snapshot == nil {
		return nil
	}
	switch queueKind {
	case op.ThreadQueueKindSteering:
		next, removed := removeThreadQueueItem(snapshot.Steering, itemID)
		snapshot.Steering = next
		return removed
	case op.ThreadQueueKindFollowUp:
		next, removed := removeThreadQueueItem(snapshot.FollowUp, itemID)
		snapshot.FollowUp = next
		return removed
	default:
		return nil
	}
}

func promoteFollowUpQueueItem(snapshot *op.ThreadQueueSnapshot, itemID string) bool {
	if snapshot == nil {
		return false
	}
	next, removed := removeThreadQueueItem(snapshot.FollowUp, itemID)
	if removed == nil {
		return false
	}
	snapshot.FollowUp = next
	snapshot.Steering = append([]op.ThreadQueueItem{*removed}, snapshot.Steering...)
	return true
}

func threadBaseDir() (string, error) {
	sys := config.GetSystem()
	if sys == nil || strings.TrimSpace(sys.BaseDir) == "" {
		return "", fmt.Errorf("system baseDir is required")
	}
	return strings.TrimSpace(sys.BaseDir), nil
}

func defaultConversationWorkdir() (string, error) {
	baseDir, err := threadBaseDir()
	if err != nil {
		return "", err
	}
	workdir := agentctx.DefaultConversationWorkdir(baseDir)
	if strings.TrimSpace(workdir) == "" {
		return "", fmt.Errorf("default conversation workdir is required")
	}
	return workdir, nil
}

func defaultConversationWorkdirBestEffort() string {
	workdir, err := defaultConversationWorkdir()
	if err != nil {
		return ""
	}
	return workdir
}

func resolveThreadCreateCWD(cwd string) (string, error) {
	resolved := normalizeThreadPath(cwd)
	if resolved == "" {
		var err error
		resolved, err = defaultConversationWorkdir()
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(resolved, 0o755); err != nil {
		return "", err
	}
	return resolved, nil
}

func resolveThreadCreateChatPath(cwd, chatPath, title string) (string, error) {
	resolved := normalizeThreadPath(chatPath)
	if resolved != "" {
		if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			return "", err
		}
		return resolved, nil
	}
	return agentctx.BuildUniqueChatPath(cwd, title)
}

func resolveThreadCreateFileID(fileID string) string {
	resolved := normalizeThreadValue(fileID)
	if resolved != "" {
		return resolved
	}
	return op.GenerateFileID()
}

func ensureThreadProjectionFile(threadID, title, chatPath, parentThreadID string) error {
	return agentctx.EnsureChatProjectionFile(agentctx.ChatProjectionFile{
		ThreadID:       threadID,
		Title:          title,
		ChatPath:       chatPath,
		ParentThreadID: parentThreadID,
	})
}

func threadFilePath(threadID string) (string, error) {
	trimmedThreadID := strings.TrimSpace(threadID)
	if trimmedThreadID == "" {
		return "", fmt.Errorf("threadID is required")
	}
	baseDir, err := threadBaseDir()
	if err != nil {
		return "", err
	}
	return threadFilePathInRoot(threadStorageRootDir(baseDir), trimmedThreadID), nil
}

func normalizeThreadAgentID(agentID string) string {
	return normalizeThreadValue(agentID)
}

func generateThreadEntryID() string {
	return uuid.NewString()[:8]
}

func readJSONLHeader(filePath string) (*op.ThreadHeader, *string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var header *op.ThreadHeader
	var leafID *string
	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNo++
		if lineNo == 1 {
			var h op.ThreadHeader
			if err := json.Unmarshal([]byte(line), &h); err != nil {
				return nil, nil, err
			}
			header = &h
			continue
		}
		var base struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &base); err != nil {
			continue
		}
		if strings.TrimSpace(base.ID) != "" {
			leafID = stringPtr(base.ID)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, fmt.Errorf("thread header not found")
	}
	return header, leafID, nil
}

func threadMetaFromHeader(header *op.ThreadHeader, filePath string) op.ThreadMeta {
	if header == nil {
		return op.ThreadMeta{}
	}
	cwd := strings.TrimSpace(header.CWD)
	if cwd == "" {
		cwd = defaultConversationWorkdirBestEffort()
	}
	return op.ThreadMeta{
		ThreadID:          strings.TrimSpace(header.ID),
		FileID:            strings.TrimSpace(header.FileID),
		AgentID:           normalizeThreadAgentID(header.AgentID),
		CWD:               cwd,
		Path:              strings.TrimSpace(header.ChatPath),
		ChatPath:          strings.TrimSpace(header.ChatPath),
		ThreadFilePath:    normalizeThreadPath(filePath),
		Title:             strings.TrimSpace(header.Title),
		ParentThreadID:    strings.TrimSpace(header.ParentThreadID),
		PlanPath:          strings.TrimSpace(header.PlanPath),
		ExecutionPlanPath: strings.TrimSpace(header.ExecutionPlanPath),
	}
}

func applyThreadMetaUpdateToHeader(header *op.ThreadHeader, entry op.ThreadMetaUpdateEntry) {
	if header == nil {
		return
	}
	if strings.TrimSpace(entry.Title) != "" {
		header.Title = strings.TrimSpace(entry.Title)
	}
	if strings.TrimSpace(entry.ChatPath) != "" {
		header.ChatPath = strings.TrimSpace(entry.ChatPath)
	}
	if strings.TrimSpace(entry.FileID) != "" {
		header.FileID = strings.TrimSpace(entry.FileID)
	}
	if strings.TrimSpace(entry.PlanPath) != "" {
		header.PlanPath = strings.TrimSpace(entry.PlanPath)
	}
	if strings.TrimSpace(entry.ExecutionPlanPath) != "" {
		header.ExecutionPlanPath = strings.TrimSpace(entry.ExecutionPlanPath)
	}
}

func normalizeThreadSnapshotWindowQuery(query *op.ThreadEntryWindowQuery) op.ThreadEntryWindowQuery {
	mode := op.ThreadEntryWindowModeTail
	limit := defaultSnapshotEntryLimit
	anchorID := ""
	if query != nil {
		switch strings.TrimSpace(query.Mode) {
		case op.ThreadEntryWindowModeBefore:
			mode = op.ThreadEntryWindowModeBefore
			limit = snapshotPaginationEntryLimit
		case op.ThreadEntryWindowModeAfter:
			mode = op.ThreadEntryWindowModeAfter
			limit = snapshotPaginationEntryLimit
		case op.ThreadEntryWindowModeTail, "":
			mode = op.ThreadEntryWindowModeTail
			limit = defaultSnapshotEntryLimit
		default:
			mode = op.ThreadEntryWindowModeTail
			limit = defaultSnapshotEntryLimit
		}
		if query.Limit > 0 {
			limit = query.Limit
		}
		anchorID = strings.TrimSpace(query.AnchorID)
	}
	if limit <= 0 {
		limit = defaultSnapshotEntryLimit
	}
	if limit > maxSnapshotEntryLimit {
		limit = maxSnapshotEntryLimit
	}
	if (mode == op.ThreadEntryWindowModeBefore || mode == op.ThreadEntryWindowModeAfter) && anchorID == "" {
		mode = op.ThreadEntryWindowModeTail
	}
	if mode == op.ThreadEntryWindowModeTail {
		anchorID = ""
	}
	return op.ThreadEntryWindowQuery{
		Mode:     mode,
		AnchorID: anchorID,
		Limit:    limit,
	}
}

func appendBoundedThreadEntry(entries []op.ThreadEntry, entry op.ThreadEntry, limit int) []op.ThreadEntry {
	if limit <= 0 {
		return nil
	}
	next := append(entries, cloneThreadEntry(entry))
	if len(next) <= limit {
		return next
	}
	copy(next, next[len(next)-limit:])
	return next[:limit]
}

func moveSnapshotProviderModelIDLatestFirst(ids []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ids
	}
	out := make([]string, 0, len(ids)+1)
	out = append(out, value)
	for _, existing := range ids {
		if strings.TrimSpace(existing) == "" || existing == value {
			continue
		}
		out = append(out, existing)
	}
	return out
}

func updateSnapshotContextUsageState(state *snapshotContextUsageState, msg ai.ConversationMessage) {
	if state == nil {
		return
	}
	state.totalEstimatedTokens += estimateCanonicalMessageTokens(msg)
	if msg.ProviderState != nil {
		state.providerModelIDsLatestFirst = moveSnapshotProviderModelIDLatestFirst(
			state.providerModelIDsLatestFirst,
			msg.ProviderState.ProviderRef,
		)
		state.providerModelIDsLatestFirst = moveSnapshotProviderModelIDLatestFirst(
			state.providerModelIDsLatestFirst,
			msg.ProviderState.Model,
		)
	}
	if usage, ok := validCanonicalAssistantUsage(msg); ok {
		state.latestUsageTokens = resolveMessageUsageTotal(usage)
		state.trailingEstimatedTokens = 0
		state.hasValidUsage = true
		state.checkpointAfterLatestUsage = false
		return
	}
	if state.hasValidUsage {
		state.trailingEstimatedTokens += estimateCanonicalMessageTokens(msg)
	}
	if isCanonicalContextCheckpoint(msg) {
		state.checkpointAfterLatestUsage = true
	}
}

func resolveSnapshotContextWindowFromProviderIDs(providerModelIDs []string, meta op.Meta) int64 {
	if meta != nil {
		requestedContextWindow := metaPositiveInt64(meta, "contextWindow")
		if value, ok := meta["modelKey"].(string); ok {
			modelContextWindow := contextWindowForModelID(value)
			if contextWindow := effectiveContextWindowForMeta(meta, modelContextWindow); contextWindow > 0 {
				return contextWindow
			}
		}
		if requestedContextWindow > 0 {
			return requestedContextWindow
		}
	}
	for _, modelID := range providerModelIDs {
		if contextWindow := contextWindowForModelID(modelID); contextWindow > 0 {
			return contextWindow
		}
	}
	return 0
}

func buildSnapshotContextUsage(state snapshotContextUsageState, contextWindow int64) ai.ThreadContextUsage {
	if contextWindow <= 0 {
		return ai.ThreadContextUsage{}
	}
	if state.checkpointAfterLatestUsage {
		return ai.ThreadContextUsage{
			ContextWindow: contextWindow,
			Known:         false,
		}
	}
	tokens := state.totalEstimatedTokens
	if state.hasValidUsage {
		tokens = state.latestUsageTokens + state.trailingEstimatedTokens
	}
	if tokens <= 0 {
		return ai.ThreadContextUsage{}
	}
	return ai.ThreadContextUsage{
		Tokens:        tokens,
		ContextWindow: contextWindow,
		PercentMilli:  contextPercentMilli(tokens, contextWindow),
		Known:         true,
	}
}

func readThreadSnapshotFromFile(filePath string, query *op.ThreadEntryWindowQuery, meta op.Meta) (*threadSnapshotContext, *string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	windowQuery := normalizeThreadSnapshotWindowQuery(query)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var (
		header             *op.ThreadHeader
		leafID             *string
		tailEntries        []op.ThreadEntry
		beforeAnchorBuffer []op.ThreadEntry
		windowEntries      []op.ThreadEntry
		windowStart        int
		windowEnd          int
		total              int
		anchorFound        bool
		queuedMsgs         op.ThreadQueueSnapshot
		messageState       = newThreadMessageState()
		lastCanonical      ai.ConversationMessage
		hasLastCanonical   bool
		contextUsageState  snapshotContextUsageState
	)

	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNo++
		if lineNo == 1 {
			var h op.ThreadHeader
			if err := json.Unmarshal([]byte(line), &h); err != nil {
				return nil, nil, err
			}
			header = &h
			continue
		}

		var typ struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &typ); err != nil {
			continue
		}
		entryType := strings.TrimSpace(typ.Type)
		if entryType == "" {
			continue
		}
		entry, err := op.DecodeThreadEntry([]byte(line))
		if err != nil {
			continue
		}

		entryIndex := total
		total++
		entryID := strings.TrimSpace(typ.ID)
		if entryID != "" {
			leafID = stringPtr(entryID)
		}
		tailEntries = appendBoundedThreadEntry(tailEntries, entry, windowQuery.Limit)

		switch windowQuery.Mode {
		case op.ThreadEntryWindowModeBefore:
			if !anchorFound {
				if entryID != "" && entryID == windowQuery.AnchorID {
					anchorFound = true
					windowEntries = cloneThreadEntries(beforeAnchorBuffer)
					windowStart = entryIndex - len(windowEntries)
					windowEnd = entryIndex
				} else {
					beforeAnchorBuffer = appendBoundedThreadEntry(beforeAnchorBuffer, entry, windowQuery.Limit)
				}
			}
		case op.ThreadEntryWindowModeAfter:
			if anchorFound {
				if len(windowEntries) < windowQuery.Limit {
					windowEntries = append(windowEntries, cloneThreadEntry(entry))
					windowEnd = windowStart + len(windowEntries)
				}
			} else if entryID != "" && entryID == windowQuery.AnchorID {
				anchorFound = true
				windowStart = entryIndex + 1
				windowEnd = windowStart
			}
		}

		switch entryType {
		case op.ThreadEntryTypeCanonicalMessage:
			var entry op.ThreadCanonicalMessageEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			lastCanonical = entry.Message
			hasLastCanonical = true
			updateSnapshotContextUsageState(&contextUsageState, entry.Message)
		case "compaction":
			var entry op.ThreadCompactionEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			lastCanonical = canonicalMessageFromCompactionEntry(entry)
			hasLastCanonical = true
			updateSnapshotContextUsageState(&contextUsageState, lastCanonical)
		case op.ThreadEntryTypeQueueEnqueue:
			var entry op.ThreadQueueEnqueueEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			appendThreadQueueItem(&queuedMsgs, entry.QueueKind, entry.Item)
		case op.ThreadEntryTypeQueueDequeue:
			var entry op.ThreadQueueDequeueEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			removeThreadQueueSnapshotItem(&queuedMsgs, entry.QueueKind, entry.ItemID)
		case op.ThreadEntryTypeQueueRemove:
			var entry op.ThreadQueueRemoveEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			removeThreadQueueSnapshotItem(&queuedMsgs, entry.QueueKind, entry.Item.ID)
		case op.ThreadEntryTypeQueuePromote:
			var entry op.ThreadQueuePromoteEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			promoteFollowUpQueueItem(&queuedMsgs, entry.ItemID)
		case op.ThreadEntryTypeMessageAppend:
			var entry op.ThreadMessageAppendEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			applyThreadMessageAppend(&messageState, entry.Record, entry.Pending)
		case op.ThreadEntryTypeMessageUpdate:
			var entry op.ThreadMessageUpdateEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			applyThreadMessageUpdate(&messageState, entry.Record)
		case op.ThreadEntryTypeMessageAck:
			var entry op.ThreadMessageAckEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			applyThreadMessageAck(&messageState, entry.MessageID, entry.Pending)
		case op.ThreadEntryTypeMetaUpdate:
			var entry op.ThreadMetaUpdateEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			applyThreadMetaUpdateToHeader(header, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, fmt.Errorf("thread header not found")
	}

	mode := windowQuery.Mode
	anchorID := windowQuery.AnchorID
	entries := windowEntries
	if mode == op.ThreadEntryWindowModeTail || !anchorFound {
		mode = op.ThreadEntryWindowModeTail
		anchorID = ""
		entries = tailEntries
		windowStart = total - len(entries)
		windowEnd = total
	} else {
		windowEnd = windowStart + len(entries)
	}
	if windowStart < 0 {
		windowStart = 0
	}
	if windowEnd < windowStart {
		windowEnd = windowStart
	}

	tailStatus := op.ThreadTailEmpty
	continuationReason := op.ThreadContinuationNone
	if hasLastCanonical {
		tailStatus, continuationReason = ai.CanonicalMessagesTailState([]ai.ConversationMessage{lastCanonical})
	}
	contextWindow := resolveSnapshotContextWindowFromProviderIDs(contextUsageState.providerModelIDsLatestFirst, meta)
	revision := strings.TrimSpace(header.ID)
	if leafID != nil && strings.TrimSpace(*leafID) != "" {
		revision = strings.TrimSpace(*leafID)
	}

	return &threadSnapshotContext{
		meta:    threadMetaFromHeader(header, filePath),
		entries: cloneThreadEntries(entries),
		entryWindow: op.ThreadEntryWindow{
			Mode:      mode,
			AnchorID:  anchorID,
			Limit:     windowQuery.Limit,
			Start:     windowStart,
			End:       windowEnd,
			Total:     total,
			HasBefore: windowStart > 0,
			HasAfter:  windowEnd < total,
		},
		revision:           revision,
		tailStatus:         tailStatus,
		continuationReason: continuationReason,
		queuedMessages:     cloneThreadQueueSnapshot(queuedMsgs),
		messageState:       cloneThreadMessageState(messageState),
		contextUsage:       buildSnapshotContextUsage(contextUsageState, contextWindow),
	}, leafID, nil
}

func readThreadContextFromFile(filePath string) (*threadContext, *string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var (
		header        *op.ThreadHeader
		leafID        *string
		entries       []op.ThreadEntry
		canonicalMsgs []ai.ConversationMessage
		queuedMsgs    op.ThreadQueueSnapshot
		messageState  = newThreadMessageState()
	)

	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNo++
		if lineNo == 1 {
			var h op.ThreadHeader
			if err := json.Unmarshal([]byte(line), &h); err != nil {
				return nil, nil, err
			}
			header = &h
			continue
		}

		var typ struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &typ); err != nil {
			continue
		}
		if strings.TrimSpace(typ.ID) != "" {
			leafID = stringPtr(typ.ID)
		}
		if strings.TrimSpace(typ.Type) != "" {
			entry, err := op.DecodeThreadEntry([]byte(line))
			if err != nil {
				continue
			}
			entries = append(entries, entry)
		}

		switch strings.TrimSpace(typ.Type) {
		case op.ThreadEntryTypeCanonicalMessage:
			var entry op.ThreadCanonicalMessageEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			canonicalMsgs = append(canonicalMsgs, entry.Message)
		case "compaction":
			var entry op.ThreadCompactionEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			canonicalMsgs = append(canonicalMsgs, canonicalMessageFromCompactionEntry(entry))
		case op.ThreadEntryTypeQueueEnqueue:
			var entry op.ThreadQueueEnqueueEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			appendThreadQueueItem(&queuedMsgs, entry.QueueKind, entry.Item)
		case op.ThreadEntryTypeQueueDequeue:
			var entry op.ThreadQueueDequeueEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			removeThreadQueueSnapshotItem(&queuedMsgs, entry.QueueKind, entry.ItemID)
		case op.ThreadEntryTypeQueueRemove:
			var entry op.ThreadQueueRemoveEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			removeThreadQueueSnapshotItem(&queuedMsgs, entry.QueueKind, entry.Item.ID)
		case op.ThreadEntryTypeQueuePromote:
			var entry op.ThreadQueuePromoteEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			promoteFollowUpQueueItem(&queuedMsgs, entry.ItemID)
		case op.ThreadEntryTypeMessageAppend:
			var entry op.ThreadMessageAppendEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			applyThreadMessageAppend(&messageState, entry.Record, entry.Pending)
		case op.ThreadEntryTypeMessageUpdate:
			var entry op.ThreadMessageUpdateEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			applyThreadMessageUpdate(&messageState, entry.Record)
		case op.ThreadEntryTypeMessageAck:
			var entry op.ThreadMessageAckEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			applyThreadMessageAck(&messageState, entry.MessageID, entry.Pending)
		case op.ThreadEntryTypeMetaUpdate:
			var entry op.ThreadMetaUpdateEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			applyThreadMetaUpdateToHeader(header, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, fmt.Errorf("thread header not found")
	}
	meta := threadMetaFromHeader(header, filePath)
	return &threadContext{
		meta:              meta,
		entries:           cloneThreadEntries(entries),
		canonicalMessages: canonicalMsgs,
		queuedMessages:    cloneThreadQueueSnapshot(queuedMsgs),
		messageState:      cloneThreadMessageState(messageState),
	}, leafID, nil
}

func writeJSONLLine(f *os.File, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	return err
}

func rewriteJSONLHeader(filePath string, header op.ThreadHeader) error {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	headerRaw, err := json.Marshal(header)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		lines = []string{string(headerRaw)}
	} else {
		lines[0] = string(headerRaw)
	}
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, filePath)
}

func (s *threadStore) mutexForThread(threadID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	mu := s.threadMux[threadID]
	if mu != nil {
		return mu
	}
	mu = &sync.Mutex{}
	s.threadMux[threadID] = mu
	return mu
}

func threadRecordFromHeader(header *op.ThreadHeader, filePath string, leafID *string) *threadRecord {
	if header == nil {
		return nil
	}
	copy := *header
	return &threadRecord{
		header:   copy,
		filePath: filePath,
		leafID:   leafID,
	}
}

func (s *threadStore) cacheRecord(record *threadRecord) {
	if record == nil {
		return
	}
	threadID := strings.TrimSpace(record.header.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byThread[threadID] = record
}

func (s *threadStore) evictRecordsUnderThreadRoot(threadRoot string) {
	root := filepath.Clean(strings.TrimSpace(threadRoot))
	if root == "" || root == "." {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for threadID, record := range s.byThread {
		if record == nil {
			delete(s.byThread, threadID)
			continue
		}
		if strings.HasPrefix(filepath.Clean(record.filePath), root+string(filepath.Separator)) {
			delete(s.byThread, threadID)
		}
	}
}

func (s *threadStore) evictThreadRecord(threadID string) {
	trimmedThreadID := strings.TrimSpace(threadID)
	if trimmedThreadID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byThread, trimmedThreadID)
}

func (s *threadStore) loadRecord(query op.ThreadMetaQuery) (*threadRecord, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return nil, err
	}
	threadID := strings.TrimSpace(query.ThreadID)

	if threadID == "" {
		return nil, os.ErrNotExist
	}

	s.mu.RLock()
	if record := s.byThread[threadID]; record != nil {
		s.mu.RUnlock()
		if recordMatchesThreadQuery(record, query) {
			if _, err := os.Stat(record.filePath); err == nil {
				return record, nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			s.evictThreadRecord(threadID)
		}
	} else {
		s.mu.RUnlock()
	}
	filePath, err := threadFilePath(threadID)
	if err != nil {
		return nil, err
	}
	record, err := loadThreadRecordFromFile(filePath)
	if err == nil && recordMatchesThreadQuery(record, query) {
		s.cacheRecord(record)
		return record, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, os.ErrNotExist
}

func (s *threadStore) enforceThreadRetention(keepThreadID string) error {
	baseDir, err := threadBaseDir()
	if err != nil {
		return err
	}
	removed, err := pruneOldThreadFiles(threadStorageRootDir(baseDir), keepThreadID, maxThreads())
	for _, threadID := range removed {
		s.evictThreadRecord(threadID)
	}
	return err
}

func (s *threadStore) Create(params op.ThreadCreateParams) (*op.ThreadCreateResult, error) {
	return s.createWithThreadID(params, "")
}

func (s *threadStore) createWithThreadID(params op.ThreadCreateParams, requestedThreadID string) (*op.ThreadCreateResult, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return nil, err
	}
	agentID := normalizeThreadAgentID(params.AgentID)
	title := normalizeThreadValue(params.Title)
	if agentID == "" {
		return nil, fmt.Errorf("agentID is required")
	}
	if title == "" {
		title = "Untitled Chat"
	}
	cwd, err := resolveThreadCreateCWD(params.CWD)
	if err != nil {
		return nil, err
	}
	chatPath, err := resolveThreadCreateChatPath(cwd, params.ChatPath, title)
	if err != nil {
		return nil, err
	}
	fileID := resolveThreadCreateFileID(params.FileID)
	threadID := strings.TrimSpace(requestedThreadID)
	if threadID == "" {
		threadID = op.GenerateThreadID()
	}
	filePath, err := threadFilePath(threadID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}
	header := op.ThreadHeader{
		Type:              "thread",
		Version:           currentThreadVersion,
		ID:                threadID,
		Timestamp:         time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:           agentID,
		CWD:               cwd,
		ChatPath:          chatPath,
		FileID:            fileID,
		Title:             title,
		ParentThreadID:    normalizeThreadIDValue(params.ParentThreadID),
		PlanPath:          "",
		ExecutionPlanPath: "",
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if writeErr := writeJSONLLine(f, header); writeErr != nil {
		_ = f.Close()
		return nil, writeErr
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := ensureThreadProjectionFile(threadID, title, chatPath, header.ParentThreadID); err != nil {
		return nil, err
	}
	s.cacheRecord(threadRecordFromHeader(&header, filePath, nil))
	if err := s.enforceThreadRetention(threadID); err != nil {
		return nil, err
	}
	return &op.ThreadCreateResult{
		ThreadID:       threadID,
		FileID:         fileID,
		Title:          title,
		CWD:            cwd,
		Path:           chatPath,
		ChatPath:       chatPath,
		ThreadFilePath: normalizeThreadPath(filePath),
	}, nil
}

func (s *threadStore) GetMeta(query op.ThreadMetaQuery) (*op.ThreadMeta, error) {
	record, err := s.loadRecord(query)
	if err != nil {
		return nil, err
	}
	meta := threadMetaFromHeader(&record.header, record.filePath)
	return &meta, nil
}

func (s *threadStore) UpdateMeta(params op.ThreadMetaUpdateParams) (*op.ThreadMeta, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return nil, err
	}
	record, err := s.loadRecord(op.ThreadMetaQuery{ThreadID: params.ThreadID})
	if err != nil {
		return nil, err
	}

	threadID := strings.TrimSpace(record.header.ID)
	lock := s.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, leafID, err := readJSONLHeader(record.filePath)
	if err != nil {
		return nil, err
	}
	record.header = *header
	record.leafID = leafID

	nextHeader := *header
	nextTitle := normalizeThreadValue(params.Title)
	nextFileID := normalizeThreadValue(params.FileID)
	nextChatPath := normalizeThreadPath(params.ChatPath)
	nextPlanPath := normalizeThreadPath(params.PlanPath)
	nextExecutionPlanPath := normalizeThreadPath(params.ExecutionPlanPath)

	var entries []any
	parentID := record.leafID
	nextParent := func() *string {
		if parentID == nil {
			return nil
		}
		value := strings.TrimSpace(*parentID)
		if value == "" {
			return nil
		}
		return stringPtr(value)
	}

	if nextTitle != "" && nextTitle != strings.TrimSpace(nextHeader.Title) {
		nextHeader.Title = nextTitle
		entryID := generateThreadEntryID()
		entries = append(entries, op.ThreadMetaUpdateEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeMetaUpdate,
				ID:        entryID,
				ParentID:  nextParent(),
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			Title: nextTitle,
		})
		parentID = stringPtr(entryID)
	}
	if nextFileID != "" && nextFileID != normalizeThreadValue(nextHeader.FileID) {
		nextHeader.FileID = nextFileID
		entryID := generateThreadEntryID()
		entries = append(entries, op.ThreadMetaUpdateEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeMetaUpdate,
				ID:        entryID,
				ParentID:  nextParent(),
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			FileID: nextFileID,
		})
		parentID = stringPtr(entryID)
	}
	if nextChatPath != "" && nextChatPath != normalizeThreadPath(nextHeader.ChatPath) {
		nextHeader.ChatPath = nextChatPath
		entryID := generateThreadEntryID()
		entries = append(entries, op.ThreadMetaUpdateEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeMetaUpdate,
				ID:        entryID,
				ParentID:  nextParent(),
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			ChatPath: nextChatPath,
		})
		parentID = stringPtr(entryID)
	}
	if nextPlanPath != "" && nextPlanPath != normalizeThreadPath(nextHeader.PlanPath) {
		nextHeader.PlanPath = nextPlanPath
		entryID := generateThreadEntryID()
		entries = append(entries, op.ThreadMetaUpdateEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeMetaUpdate,
				ID:        entryID,
				ParentID:  nextParent(),
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			PlanPath: nextPlanPath,
		})
		parentID = stringPtr(entryID)
	}
	if params.ExecutionPlanPath != "" && nextExecutionPlanPath != normalizeThreadPath(nextHeader.ExecutionPlanPath) {
		nextHeader.ExecutionPlanPath = nextExecutionPlanPath
		entryID := generateThreadEntryID()
		entries = append(entries, op.ThreadMetaUpdateEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeMetaUpdate,
				ID:        entryID,
				ParentID:  nextParent(),
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			ExecutionPlanPath: nextExecutionPlanPath,
		})
		parentID = stringPtr(entryID)
	}

	if err := rewriteJSONLHeader(record.filePath, nextHeader); err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		f, err := os.OpenFile(record.filePath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if err := writeJSONLLine(f, entry); err != nil {
				_ = f.Close()
				return nil, err
			}
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}

	record.header = nextHeader
	record.leafID = parentID
	s.cacheRecord(record)

	meta := threadMetaFromHeader(&nextHeader, record.filePath)
	return &meta, nil
}

func (s *threadStore) Fork(params op.ThreadForkParams) (*op.ThreadMeta, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return nil, err
	}
	sourceRecord, err := s.loadRecord(op.ThreadMetaQuery{
		ThreadID: params.SourceThreadID,
	})
	if err != nil {
		return nil, err
	}

	sourceHeader, _, err := readJSONLHeader(sourceRecord.filePath)
	if err != nil {
		return nil, err
	}

	agentID := normalizeThreadAgentID(params.AgentID)
	if agentID == "" {
		agentID = normalizeThreadAgentID(sourceHeader.AgentID)
	}
	cwd := normalizeThreadPath(params.CWD)
	if cwd == "" {
		cwd = normalizeThreadPath(sourceHeader.CWD)
	}
	title := normalizeThreadValue(params.Title)
	if title == "" {
		title = normalizeThreadValue(sourceHeader.Title)
	}
	planPath := normalizeThreadPath(params.PlanPath)
	if planPath == "" {
		planPath = normalizeThreadPath(sourceHeader.PlanPath)
	}
	executionPlanPath := normalizeThreadPath(params.ExecutionPlanPath)
	parentThreadID := normalizeThreadIDValue(sourceHeader.ID)
	if agentID == "" {
		return nil, fmt.Errorf("agentID is required")
	}
	if title == "" {
		title = "Untitled Chat"
	}
	cwd, err = resolveThreadCreateCWD(cwd)
	if err != nil {
		return nil, err
	}
	chatPath, err := resolveThreadCreateChatPath(cwd, params.ChatPath, title)
	if err != nil {
		return nil, err
	}
	fileID := resolveThreadCreateFileID(params.FileID)

	threadID := op.GenerateThreadID()
	filePath, err := threadFilePath(threadID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}
	header := op.ThreadHeader{
		Type:              "thread",
		Version:           currentThreadVersion,
		ID:                threadID,
		Timestamp:         time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:           agentID,
		CWD:               cwd,
		ChatPath:          chatPath,
		FileID:            fileID,
		Title:             title,
		ParentThreadID:    parentThreadID,
		PlanPath:          planPath,
		ExecutionPlanPath: executionPlanPath,
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if writeErr := writeJSONLLine(f, header); writeErr != nil {
		_ = f.Close()
		return nil, writeErr
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := ensureThreadProjectionFile(threadID, title, chatPath, parentThreadID); err != nil {
		return nil, err
	}

	record := threadRecordFromHeader(&header, filePath, nil)
	s.cacheRecord(record)
	if err := s.enforceThreadRetention(threadID); err != nil {
		return nil, err
	}
	return &op.ThreadMeta{
		ThreadID:          threadID,
		FileID:            fileID,
		AgentID:           agentID,
		CWD:               cwd,
		Path:              chatPath,
		ChatPath:          chatPath,
		ThreadFilePath:    normalizeThreadPath(filePath),
		Title:             title,
		ParentThreadID:    parentThreadID,
		PlanPath:          planPath,
		ExecutionPlanPath: executionPlanPath,
	}, nil
}

func (s *threadStore) LoadThreadContext(threadID, agentID string) (*threadContext, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return nil, err
	}
	record, err := s.loadRecord(op.ThreadMetaQuery{
		ThreadID: threadID,
		AgentID:  agentID,
	})
	if err != nil {
		return nil, err
	}
	ctx, leafID, err := readThreadContextFromFile(record.filePath)
	if err != nil {
		return nil, err
	}
	record.leafID = leafID
	record.header.Title = ctx.meta.Title
	record.header.FileID = ctx.meta.FileID
	record.header.ChatPath = ctx.meta.ChatPath
	record.header.PlanPath = ctx.meta.PlanPath
	s.cacheRecord(record)
	return ctx, nil
}

func (s *threadStore) GetThreadSnapshot(query op.ThreadMetaQuery) (*ai.ThreadSnapshot, error) {
	return s.GetThreadSnapshotWithMeta(query, nil)
}

func (s *threadStore) GetThreadSnapshotWithMeta(query op.ThreadMetaQuery, meta op.Meta) (*ai.ThreadSnapshot, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return nil, err
	}
	record, err := s.loadRecord(query)
	if err != nil {
		return nil, err
	}
	ctx, leafID, err := readThreadSnapshotFromFile(record.filePath, query.EntryWindow, meta)
	if err != nil {
		return nil, err
	}
	record.leafID = leafID
	record.header.Title = ctx.meta.Title
	record.header.FileID = ctx.meta.FileID
	record.header.ChatPath = ctx.meta.ChatPath
	record.header.PlanPath = ctx.meta.PlanPath
	s.cacheRecord(record)
	runStatus := op.ThreadRunIdle
	if isRuntimeThreadActive(strings.TrimSpace(ctx.meta.ThreadID)) {
		runStatus = op.ThreadRunRunning
	}
	return &ai.ThreadSnapshot{
		Meta:               ctx.meta,
		Entries:            cloneThreadEntries(ctx.entries),
		EntryWindow:        ctx.entryWindow,
		Revision:           ctx.revision,
		RunStatus:          runStatus,
		TailStatus:         ctx.tailStatus,
		ContinuationReason: ctx.continuationReason,
		QueuedMessages:     cloneThreadQueueSnapshot(ctx.queuedMessages),
		MessageRecords:     threadMessageRecords(ctx.messageState, false, 0),
		ChannelSummaries:   threadMessageChannelSummaries(ctx.messageState, 0),
		ContextUsage:       ctx.contextUsage,
	}, nil
}

func (s *threadStore) GetQueueSnapshot(query op.ThreadMetaQuery) (op.ThreadQueueSnapshot, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record, err := s.loadRecord(query)
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	ctx, leafID, err := readThreadContextFromFile(record.filePath)
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record.leafID = leafID
	s.cacheRecord(record)
	return cloneThreadQueueSnapshot(ctx.queuedMessages), nil
}

func (s *threadStore) EnqueueQueuedMessage(
	meta op.ThreadMeta,
	queueKind op.ThreadQueueKind,
	message op.Message,
	agentName string,
	modelKey string,
	thinkingLevel string,
	contextWindow int64,
	serviceTier string,
	selectedSkillIDs []string,
	selectedSkillContext op.Meta,
	planTurn bool,
) (*op.ThreadQueueItem, op.ThreadQueueSnapshot, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return nil, op.ThreadQueueSnapshot{}, err
	}
	record, err := s.loadRecord(threadMetaQuery(meta))
	if err != nil {
		return nil, op.ThreadQueueSnapshot{}, err
	}
	threadID := strings.TrimSpace(record.header.ID)
	lock := s.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, leafID, err := readJSONLHeader(record.filePath)
	if err != nil {
		return nil, op.ThreadQueueSnapshot{}, err
	}
	record.header = *header
	record.leafID = leafID
	ctx, _, err := readThreadContextFromFile(record.filePath)
	if err != nil {
		return nil, op.ThreadQueueSnapshot{}, err
	}
	nextSnapshot := cloneThreadQueueSnapshot(ctx.queuedMessages)
	item := op.ThreadQueueItem{
		ID:                   generateQueueItemID(),
		Message:              message,
		AgentID:              normalizeThreadAgentID(meta.AgentID),
		AgentName:            normalizeThreadValue(agentName),
		CWD:                  normalizeThreadPath(meta.CWD),
		ModelKey:             normalizeThreadValue(modelKey),
		ThinkingLevel:        normalizeThreadValue(thinkingLevel),
		ContextWindow:        normalizeThreadPositiveInt64(contextWindow),
		ServiceTier:          normalizeThreadValue(serviceTier),
		SelectedSkillIDs:     append([]string(nil), selectedSkillIDs...),
		SelectedSkillContext: selectedSkillContext.Clone(),
		PlanTurn:             planTurn,
	}
	appendThreadQueueItem(&nextSnapshot, queueKind, item)
	nextLeafID, err := appendThreadEntries(record.filePath, record.leafID, func(parentID *string) []any {
		entryID := generateThreadEntryID()
		return []any{op.ThreadQueueEnqueueEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeQueueEnqueue,
				ID:        entryID,
				ParentID:  parentID,
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			QueueKind: queueKind,
			Item:      cloneThreadQueueItem(item),
		}}
	})
	if err != nil {
		return nil, op.ThreadQueueSnapshot{}, err
	}
	record.leafID = nextLeafID
	s.cacheRecord(record)
	cloned := cloneThreadQueueItem(item)
	return &cloned, nextSnapshot, nil
}

func (s *threadStore) PromoteQueuedMessage(meta op.ThreadMeta, itemID string) (op.ThreadQueueSnapshot, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record, err := s.loadRecord(threadMetaQuery(meta))
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	threadID := strings.TrimSpace(record.header.ID)
	lock := s.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, leafID, err := readJSONLHeader(record.filePath)
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record.header = *header
	record.leafID = leafID
	ctx, _, err := readThreadContextFromFile(record.filePath)
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	nextSnapshot := cloneThreadQueueSnapshot(ctx.queuedMessages)
	if !promoteFollowUpQueueItem(&nextSnapshot, itemID) {
		return op.ThreadQueueSnapshot{}, fmt.Errorf("no follow-up message pending for thread: %s", threadID)
	}
	nextLeafID, err := appendThreadEntries(record.filePath, record.leafID, func(parentID *string) []any {
		entryID := generateThreadEntryID()
		return []any{op.ThreadQueuePromoteEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeQueuePromote,
				ID:        entryID,
				ParentID:  parentID,
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			ItemID: strings.TrimSpace(itemID),
		}}
	})
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record.leafID = nextLeafID
	s.cacheRecord(record)
	return nextSnapshot, nil
}

func (s *threadStore) DequeueQueuedMessage(meta op.ThreadMeta, queueKind op.ThreadQueueKind, itemID string) (op.ThreadQueueSnapshot, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record, err := s.loadRecord(threadMetaQuery(meta))
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	threadID := strings.TrimSpace(record.header.ID)
	lock := s.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, leafID, err := readJSONLHeader(record.filePath)
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record.header = *header
	record.leafID = leafID
	ctx, _, err := readThreadContextFromFile(record.filePath)
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	nextSnapshot := cloneThreadQueueSnapshot(ctx.queuedMessages)
	if removed := removeThreadQueueSnapshotItem(&nextSnapshot, queueKind, itemID); removed == nil {
		return op.ThreadQueueSnapshot{}, fmt.Errorf("no %s message pending for thread: %s", queueKind, threadID)
	}
	nextLeafID, err := appendThreadEntries(record.filePath, record.leafID, func(parentID *string) []any {
		entryID := generateThreadEntryID()
		return []any{op.ThreadQueueDequeueEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeQueueDequeue,
				ID:        entryID,
				ParentID:  parentID,
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			QueueKind: queueKind,
			ItemID:    strings.TrimSpace(itemID),
		}}
	})
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record.leafID = nextLeafID
	s.cacheRecord(record)
	return nextSnapshot, nil
}

func (s *threadStore) RemoveQueuedMessage(meta op.ThreadMeta, queueKind op.ThreadQueueKind, itemID string) (*op.ThreadQueueItem, op.ThreadQueueSnapshot, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return nil, op.ThreadQueueSnapshot{}, err
	}
	record, err := s.loadRecord(threadMetaQuery(meta))
	if err != nil {
		return nil, op.ThreadQueueSnapshot{}, err
	}
	threadID := strings.TrimSpace(record.header.ID)
	lock := s.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, leafID, err := readJSONLHeader(record.filePath)
	if err != nil {
		return nil, op.ThreadQueueSnapshot{}, err
	}
	record.header = *header
	record.leafID = leafID
	ctx, _, err := readThreadContextFromFile(record.filePath)
	if err != nil {
		return nil, op.ThreadQueueSnapshot{}, err
	}
	nextSnapshot := cloneThreadQueueSnapshot(ctx.queuedMessages)
	removed := removeThreadQueueSnapshotItem(&nextSnapshot, queueKind, itemID)
	if removed == nil {
		return nil, op.ThreadQueueSnapshot{}, fmt.Errorf("no %s message pending for thread: %s", queueKind, threadID)
	}
	nextLeafID, err := appendThreadEntries(record.filePath, record.leafID, func(parentID *string) []any {
		entryID := generateThreadEntryID()
		return []any{op.ThreadQueueRemoveEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeQueueRemove,
				ID:        entryID,
				ParentID:  parentID,
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			QueueKind: queueKind,
			Item:      cloneThreadQueueItem(*removed),
		}}
	})
	if err != nil {
		return nil, op.ThreadQueueSnapshot{}, err
	}
	record.leafID = nextLeafID
	s.cacheRecord(record)
	return removed, nextSnapshot, nil
}

func (s *threadStore) ClearQueuedMessages(meta op.ThreadMeta) (op.ThreadQueueSnapshot, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record, err := s.loadRecord(threadMetaQuery(meta))
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	threadID := strings.TrimSpace(record.header.ID)
	lock := s.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, leafID, err := readJSONLHeader(record.filePath)
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record.header = *header
	record.leafID = leafID
	ctx, _, err := readThreadContextFromFile(record.filePath)
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	snapshot := cloneThreadQueueSnapshot(ctx.queuedMessages)
	entriesNeeded := len(snapshot.Steering) + len(snapshot.FollowUp)
	if entriesNeeded == 0 {
		return op.ThreadQueueSnapshot{}, nil
	}
	nextLeafID, err := appendThreadEntries(record.filePath, record.leafID, func(parentID *string) []any {
		entries := make([]any, 0, entriesNeeded)
		currentParentID := parentID
		appendRemoveEntries := func(queueKind op.ThreadQueueKind, items []op.ThreadQueueItem) {
			for _, item := range items {
				entryID := generateThreadEntryID()
				entries = append(entries, op.ThreadQueueRemoveEntry{
					ThreadEntryBase: op.ThreadEntryBase{
						Type:      op.ThreadEntryTypeQueueRemove,
						ID:        entryID,
						ParentID:  currentParentID,
						Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
					},
					QueueKind: queueKind,
					Item:      cloneThreadQueueItem(item),
				})
				currentParentID = stringPtr(entryID)
			}
		}
		appendRemoveEntries(op.ThreadQueueKindSteering, snapshot.Steering)
		appendRemoveEntries(op.ThreadQueueKindFollowUp, snapshot.FollowUp)
		return entries
	})
	if err != nil {
		return op.ThreadQueueSnapshot{}, err
	}
	record.leafID = nextLeafID
	s.cacheRecord(record)
	return snapshot, nil
}

func (s *threadStore) AppendOpMessages(meta op.ThreadMeta, messages []op.Message) error {
	return s.AppendCanonicalMessages(meta, ai.CanonicalMessagesFromOp(messages))
}

func (s *threadStore) AppendCanonicalMessages(meta op.ThreadMeta, messages []ai.ConversationMessage) error {
	if err := ensureThreadStorageReady(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	query := threadMetaQuery(meta)
	record, err := s.loadRecord(query)
	if err != nil {
		return err
	}
	threadID := strings.TrimSpace(record.header.ID)
	lock := s.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, leafID, err := readJSONLHeader(record.filePath)
	if err != nil {
		return err
	}
	record.header = *header
	record.leafID = leafID

	parentID, err := appendThreadEntries(record.filePath, record.leafID, func(parentID *string) []any {
		entries := make([]any, 0, len(messages))
		currentParentID := parentID
		for _, msg := range messages {
			entryID := generateThreadEntryID()
			entries = append(entries, op.ThreadCanonicalMessageEntry{
				ThreadEntryBase: op.ThreadEntryBase{
					Type:      op.ThreadEntryTypeCanonicalMessage,
					ID:        entryID,
					ParentID:  currentParentID,
					Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				},
				Message: msg,
			})
			currentParentID = stringPtr(entryID)
		}
		return entries
	})
	if err != nil {
		return err
	}

	record.leafID = parentID
	s.cacheRecord(record)
	return nil
}

func (s *threadStore) AppendQueueDelivery(meta op.ThreadMeta, pending []PendingLoopMessage) error {
	if err := ensureThreadStorageReady(); err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	record, err := s.loadRecord(threadMetaQuery(meta))
	if err != nil {
		return err
	}
	threadID := strings.TrimSpace(record.header.ID)
	lock := s.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, leafID, err := readJSONLHeader(record.filePath)
	if err != nil {
		return err
	}
	record.header = *header
	record.leafID = leafID

	ctx, _, err := readThreadContextFromFile(record.filePath)
	if err != nil {
		return err
	}
	nextSnapshot := cloneThreadQueueSnapshot(ctx.queuedMessages)
	nextLeafID, err := appendThreadEntries(record.filePath, record.leafID, func(parentID *string) []any {
		entries := make([]any, 0, len(pending)*2)
		currentParentID := parentID
		for _, current := range pending {
			if hasPendingLoopQueueSource(current) {
				removed := removeThreadQueueSnapshotItem(&nextSnapshot, current.QueueKind, current.QueueItemID)
				if removed == nil {
					entries = nil
					return entries
				}
				dequeueID := generateThreadEntryID()
				entries = append(entries, op.ThreadQueueDequeueEntry{
					ThreadEntryBase: op.ThreadEntryBase{
						Type:      op.ThreadEntryTypeQueueDequeue,
						ID:        dequeueID,
						ParentID:  currentParentID,
						Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
					},
					QueueKind: current.QueueKind,
					ItemID:    strings.TrimSpace(current.QueueItemID),
				})
				currentParentID = stringPtr(dequeueID)
			}
			for _, canonical := range ai.CanonicalMessagesFromOp([]op.Message{current.Message}) {
				entryID := generateThreadEntryID()
				entries = append(entries, op.ThreadCanonicalMessageEntry{
					ThreadEntryBase: op.ThreadEntryBase{
						Type:      op.ThreadEntryTypeCanonicalMessage,
						ID:        entryID,
						ParentID:  currentParentID,
						Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
					},
					Message: canonical,
				})
				currentParentID = stringPtr(entryID)
			}
		}
		return entries
	})
	if err != nil {
		return err
	}
	if nextLeafID == record.leafID {
		for _, current := range pending {
			if hasPendingLoopQueueSource(current) {
				return fmt.Errorf("no %s message pending for thread: %s", current.QueueKind, threadID)
			}
		}
	}
	record.leafID = nextLeafID
	s.cacheRecord(record)
	return nil
}

func appendThreadEntries(filePath string, parentID *string, build func(parentID *string) []any) (*string, error) {
	entries := build(parentID)
	if len(entries) == 0 {
		return parentID, nil
	}
	var buf bytes.Buffer
	for _, entry := range entries {
		raw, err := json.Marshal(entry)
		if err != nil {
			return parentID, err
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return parentID, err
	}
	written, err := f.Write(buf.Bytes())
	if err != nil {
		_ = f.Close()
		return parentID, err
	}
	if written != buf.Len() {
		_ = f.Close()
		return parentID, fmt.Errorf("short write appending thread entries: wrote %d of %d bytes", written, buf.Len())
	}
	if err := f.Close(); err != nil {
		return parentID, err
	}

	lastID := parentID
	for _, entry := range entries {
		switch typed := entry.(type) {
		case op.ThreadCanonicalMessageEntry:
			lastID = stringPtr(typed.ID)
		case op.ThreadMetaUpdateEntry:
			lastID = stringPtr(typed.ID)
		case op.ThreadCompactionEntry:
			lastID = stringPtr(typed.ID)
		case op.ThreadQueueEnqueueEntry:
			lastID = stringPtr(typed.ID)
		case op.ThreadQueueDequeueEntry:
			lastID = stringPtr(typed.ID)
		case op.ThreadQueueRemoveEntry:
			lastID = stringPtr(typed.ID)
		case op.ThreadQueuePromoteEntry:
			lastID = stringPtr(typed.ID)
		case op.ThreadMessageAppendEntry:
			lastID = stringPtr(typed.ID)
		case op.ThreadMessageUpdateEntry:
			lastID = stringPtr(typed.ID)
		case op.ThreadMessageAckEntry:
			lastID = stringPtr(typed.ID)
		}
	}
	return lastID, nil
}

func (s *threadStore) AppendTurn(meta op.ThreadMeta, userMessage op.Message, newMessages []op.Message) error {
	messages := make([]op.Message, 0, len(newMessages)+1)
	messages = append(messages, userMessage)
	messages = append(messages, newMessages...)
	return s.AppendOpMessages(meta, messages)
}

func getThreadMeta(threadID, agentID string) (*op.ThreadMeta, error) {
	return defaultThreadStore.GetMeta(op.ThreadMetaQuery{
		ThreadID: threadID,
		AgentID:  agentID,
	})
}

func createThread(params op.ThreadCreateParams) (*op.ThreadCreateResult, error) {
	return defaultThreadStore.Create(params)
}

func createThreadWithID(params op.ThreadCreateParams, threadID string) (*op.ThreadCreateResult, error) {
	return defaultThreadStore.createWithThreadID(params, threadID)
}

func updateThreadMeta(params op.ThreadMetaUpdateParams) (*op.ThreadMeta, error) {
	return defaultThreadStore.UpdateMeta(params)
}

func forkThread(params op.ThreadForkParams) (*op.ThreadMeta, error) {
	return defaultThreadStore.Fork(params)
}

func loadThreadContext(threadID, agentID string) (*threadContext, error) {
	return defaultThreadStore.LoadThreadContext(threadID, agentID)
}

func getThreadSnapshot(query op.ThreadMetaQuery) (*ai.ThreadSnapshot, error) {
	return defaultThreadStore.GetThreadSnapshot(query)
}

func getThreadSnapshotWithMeta(query op.ThreadMetaQuery, meta op.Meta) (*ai.ThreadSnapshot, error) {
	return defaultThreadStore.GetThreadSnapshotWithMeta(query, meta)
}

func appendMessagesToThread(meta op.ThreadMeta, messages []op.Message) error {
	return defaultThreadStore.AppendOpMessages(meta, messages)
}

func appendCanonicalMessagesToThread(meta op.ThreadMeta, messages []ai.ConversationMessage) error {
	return defaultThreadStore.AppendCanonicalMessages(meta, messages)
}

func appendQueueDeliveryToThread(meta op.ThreadMeta, pending []PendingLoopMessage) error {
	return defaultThreadStore.AppendQueueDelivery(meta, pending)
}

func appendTurnToThread(meta op.ThreadMeta, userMessage op.Message, newMessages []op.Message) error {
	return defaultThreadStore.AppendTurn(meta, userMessage, newMessages)
}

func getQueuedMessagesSnapshot(query op.ThreadMetaQuery) (op.ThreadQueueSnapshot, error) {
	return defaultThreadStore.GetQueueSnapshot(query)
}

func appendQueuedMessageToThread(
	meta op.ThreadMeta,
	queueKind op.ThreadQueueKind,
	message op.Message,
	agentName string,
	modelKey string,
	thinkingLevel string,
	contextWindow int64,
	serviceTier string,
	selectedSkillIDs []string,
	selectedSkillContext op.Meta,
	planTurn bool,
) (*op.ThreadQueueItem, op.ThreadQueueSnapshot, error) {
	return defaultThreadStore.EnqueueQueuedMessage(meta, queueKind, message, agentName, modelKey, thinkingLevel, contextWindow, serviceTier, selectedSkillIDs, selectedSkillContext, planTurn)
}

func promoteQueuedMessageInThread(meta op.ThreadMeta, itemID string) (op.ThreadQueueSnapshot, error) {
	return defaultThreadStore.PromoteQueuedMessage(meta, itemID)
}

func dequeueQueuedMessageFromThread(meta op.ThreadMeta, queueKind op.ThreadQueueKind, itemID string) (op.ThreadQueueSnapshot, error) {
	return defaultThreadStore.DequeueQueuedMessage(meta, queueKind, itemID)
}

func removeQueuedMessageFromThread(meta op.ThreadMeta, queueKind op.ThreadQueueKind, itemID string) (*op.ThreadQueueItem, op.ThreadQueueSnapshot, error) {
	return defaultThreadStore.RemoveQueuedMessage(meta, queueKind, itemID)
}

func clearQueuedMessagesInThread(meta op.ThreadMeta) (op.ThreadQueueSnapshot, error) {
	return defaultThreadStore.ClearQueuedMessages(meta)
}

func isThreadNotFound(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
