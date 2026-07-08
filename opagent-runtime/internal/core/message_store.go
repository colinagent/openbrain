package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type threadMessageState struct {
	messages map[string]op.MessageRecord
	order    []string
	pending  map[string]bool
}

type messageStore struct {
	// Kept intentionally empty for production use. Message state is replayed from
	// thread JSONL files; this type remains as the small API facade used by tools.
	messages map[string]op.MessageRecord
	pending  map[string]bool
}

type messageThreadContext struct {
	record *threadRecord
	ctx    *threadContext
}

var defaultMessageStore = &messageStore{}

func newThreadMessageState() threadMessageState {
	return threadMessageState{
		messages: make(map[string]op.MessageRecord),
		pending:  make(map[string]bool),
	}
}

func ensureThreadMessageState(state *threadMessageState) {
	if state == nil {
		return
	}
	if state.messages == nil {
		state.messages = make(map[string]op.MessageRecord)
	}
	if state.pending == nil {
		state.pending = make(map[string]bool)
	}
}

func cloneThreadMessageState(src threadMessageState) threadMessageState {
	dst := newThreadMessageState()
	dst.order = append([]string(nil), src.order...)
	for id, record := range src.messages {
		dst.messages[id] = cloneMessageRecord(record)
	}
	for id, pending := range src.pending {
		if pending {
			dst.pending[id] = true
		}
	}
	return dst
}

func applyThreadMessageAppend(state *threadMessageState, record op.MessageRecord, pending bool) {
	if state == nil {
		return
	}
	ensureThreadMessageState(state)
	record = cloneMessageRecord(record)
	id := strings.TrimSpace(record.ID)
	if id == "" {
		return
	}
	record.ID = id
	if _, exists := state.messages[id]; !exists {
		state.order = append(state.order, id)
	}
	state.messages[id] = record
	if pending && record.Sender == op.MessageSenderUser && record.Status != op.MessageStatusArchived {
		state.pending[id] = true
		return
	}
	delete(state.pending, id)
}

func applyThreadMessageUpdate(state *threadMessageState, record op.MessageRecord) {
	if state == nil {
		return
	}
	ensureThreadMessageState(state)
	record = cloneMessageRecord(record)
	id := strings.TrimSpace(record.ID)
	if id == "" {
		return
	}
	record.ID = id
	if _, exists := state.messages[id]; !exists {
		state.order = append(state.order, id)
	}
	state.messages[id] = record
	if record.Status == op.MessageStatusArchived {
		delete(state.pending, id)
	}
}

func applyThreadMessageAck(state *threadMessageState, messageID string, pending bool) {
	if state == nil {
		return
	}
	ensureThreadMessageState(state)
	id := strings.TrimSpace(messageID)
	if id == "" {
		return
	}
	if pending {
		state.pending[id] = true
		return
	}
	delete(state.pending, id)
}

func normalizeMessageRecord(sender op.MessageSender, record op.MessageRecord) (op.MessageRecord, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		record.ID = "msg-" + generateThreadEntryID() + "-" + fmt.Sprint(time.Now().UnixNano())
	}
	record.ChannelID = strings.TrimSpace(record.ChannelID)
	record.ThreadID = strings.TrimSpace(record.ThreadID)
	record.AgentID = strings.TrimSpace(record.AgentID)
	if record.ChannelID == "" {
		record.ChannelID = defaultMessageChannelID(record.ThreadID, record.AgentID)
	}
	record.Sender = normalizeMessageSender(sender)
	record.Kind = normalizeMessageKind(record.Kind)
	record.Status = normalizeMessageStatus(record.Status)
	record.Title = strings.TrimSpace(record.Title)
	record.Body = strings.TrimSpace(record.Body)
	record.Actions = normalizeMessageActions(record.Actions)
	record.Questions = normalizeMessageQuestions(record.Questions)
	record.ReplyToMessageID = strings.TrimSpace(record.ReplyToMessageID)
	record.ActionID = strings.TrimSpace(record.ActionID)
	record.Answers = normalizeMessageAnswers(record.Answers)
	record.CreatedAt = strings.TrimSpace(record.CreatedAt)
	if record.CreatedAt == "" {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	if record.Meta != nil {
		record.Meta = record.Meta.Clone()
	}
	if record.ThreadID == "" {
		return op.MessageRecord{}, fmt.Errorf("threadID is required")
	}
	if record.AgentID == "" {
		return op.MessageRecord{}, fmt.Errorf("agentID is required")
	}
	if record.Body == "" && record.Title == "" {
		return op.MessageRecord{}, fmt.Errorf("message body or title is required")
	}
	return record, nil
}

func (s *messageStore) appendRecord(sender op.MessageSender, record op.MessageRecord) (op.MessageRecord, error) {
	return defaultThreadStore.AppendMessageRecord(sender, record)
}

func (s *messageStore) update(params op.MessageUpdateParams) (op.MessageRecord, error) {
	return defaultThreadStore.UpdateMessageRecord(params)
}

func (s *messageStore) read(params op.MessageReadParams) (op.MessageReadResult, error) {
	return defaultThreadStore.ReadMessages(params)
}

func (s *messageStore) list(params op.MessageListParams) (op.MessageListResult, error) {
	return defaultThreadStore.ListMessages(params)
}

func (s *messageStore) reply(params op.MessageReplyParams) (op.MessageRecord, error) {
	return defaultThreadStore.ReplyMessage(params)
}

func (s *messageStore) replyWithResolved(params op.MessageReplyParams) (op.MessageRecord, *op.MessageRecord, error) {
	return defaultThreadStore.ReplyMessageWithResolved(params)
}

func (s *messageStore) ack(params op.MessageAckParams) (op.MessageAckResult, error) {
	return defaultThreadStore.AckMessages(params)
}

func (s *messageStore) archive(params op.MessageArchiveParams) (op.MessageArchiveResult, error) {
	return defaultThreadStore.ArchiveMessages(params)
}

func (s *threadStore) AppendMessageRecord(sender op.MessageSender, input op.MessageRecord) (op.MessageRecord, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return op.MessageRecord{}, err
	}
	record, err := normalizeMessageRecord(sender, input)
	if err != nil {
		return op.MessageRecord{}, err
	}
	threadRecord, err := s.loadRecord(op.ThreadMetaQuery{
		ThreadID: record.ThreadID,
		AgentID:  record.AgentID,
	})
	if err != nil {
		return op.MessageRecord{}, err
	}
	threadID := strings.TrimSpace(threadRecord.header.ID)
	lock := s.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, leafID, err := readJSONLHeader(threadRecord.filePath)
	if err != nil {
		return op.MessageRecord{}, err
	}
	threadRecord.header = *header
	threadRecord.leafID = leafID

	pending := record.Sender == op.MessageSenderUser && record.Status != op.MessageStatusArchived
	nextLeafID, err := appendThreadEntries(threadRecord.filePath, threadRecord.leafID, func(parentID *string) []any {
		entryID := generateThreadEntryID()
		return []any{op.ThreadMessageAppendEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeMessageAppend,
				ID:        entryID,
				ParentID:  parentID,
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			Record:  cloneMessageRecord(record),
			Pending: pending,
		}}
	})
	if err != nil {
		return op.MessageRecord{}, err
	}
	threadRecord.leafID = nextLeafID
	s.cacheRecord(threadRecord)
	return cloneMessageRecord(record), nil
}

func (s *threadStore) UpdateMessageRecord(params op.MessageUpdateParams) (op.MessageRecord, error) {
	messageID := strings.TrimSpace(params.MessageID)
	if messageID == "" {
		return op.MessageRecord{}, fmt.Errorf("messageID is required")
	}
	ref, err := s.findMessageByID(messageID)
	if err != nil {
		return op.MessageRecord{}, err
	}
	threadRecord := ref.record
	threadID := strings.TrimSpace(threadRecord.header.ID)
	lock := s.mutexForThread(threadID)
	lock.Lock()
	defer lock.Unlock()

	header, leafID, err := readJSONLHeader(threadRecord.filePath)
	if err != nil {
		return op.MessageRecord{}, err
	}
	threadRecord.header = *header
	threadRecord.leafID = leafID
	ctx, _, err := readThreadContextFromFile(threadRecord.filePath)
	if err != nil {
		return op.MessageRecord{}, err
	}
	record, ok := ctx.messageState.messages[messageID]
	if !ok {
		return op.MessageRecord{}, fmt.Errorf("message not found: %s", messageID)
	}
	if params.Body != nil {
		record.Body = strings.TrimSpace(*params.Body)
	}
	if params.Title != nil {
		record.Title = strings.TrimSpace(*params.Title)
	}
	if params.Status != "" {
		record.Status = normalizeMessageStatus(params.Status)
	}
	if params.Actions != nil {
		record.Actions = normalizeMessageActions(params.Actions)
	}
	if params.Questions != nil {
		record.Questions = normalizeMessageQuestions(params.Questions)
	}
	if params.Meta != nil {
		record.Meta = params.Meta.Clone()
	}
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	nextLeafID, err := appendThreadEntries(threadRecord.filePath, threadRecord.leafID, func(parentID *string) []any {
		entryID := generateThreadEntryID()
		return []any{op.ThreadMessageUpdateEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeMessageUpdate,
				ID:        entryID,
				ParentID:  parentID,
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			Record: cloneMessageRecord(record),
		}}
	})
	if err != nil {
		return op.MessageRecord{}, err
	}
	threadRecord.leafID = nextLeafID
	s.cacheRecord(threadRecord)
	return cloneMessageRecord(record), nil
}

func (s *threadStore) ReadMessages(params op.MessageReadParams) (op.MessageReadResult, error) {
	contexts, err := s.loadLiveMessageContexts(op.ThreadMetaQuery{ThreadID: params.ThreadID, AgentID: params.AgentID})
	if err != nil {
		return op.MessageReadResult{}, err
	}
	limit := params.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	messages, _ := filterMessageContexts(contexts, messageContextFilter{
		ChannelID:   strings.TrimSpace(params.ChannelID),
		ThreadID:    strings.TrimSpace(params.ThreadID),
		AgentID:     strings.TrimSpace(params.AgentID),
		PendingOnly: params.PendingOnly,
		IncludeArch: true,
		Limit:       limit,
	})
	return op.MessageReadResult{
		ChannelID: strings.TrimSpace(params.ChannelID),
		ThreadID:  strings.TrimSpace(params.ThreadID),
		AgentID:   strings.TrimSpace(params.AgentID),
		Messages:  messages,
	}, nil
}

func (s *threadStore) ListMessages(params op.MessageListParams) (op.MessageListResult, error) {
	contexts, err := s.loadLiveMessageContexts(op.ThreadMetaQuery{ThreadID: params.ThreadID, AgentID: params.AgentID})
	if err != nil {
		return op.MessageListResult{}, err
	}
	limit := maxMessageListLimit(params.Limit)
	messages, pending := filterMessageContexts(contexts, messageContextFilter{
		ThreadID: strings.TrimSpace(params.ThreadID),
		AgentID:  strings.TrimSpace(params.AgentID),
		Limit:    limit,
	})
	return op.MessageListResult{
		Channels: messageChannelSummaries(messages, pending),
		Messages: messages,
	}, nil
}

func maxMessageListLimit(limit int) int {
	if limit <= 0 {
		return 500
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func (s *threadStore) ReplyMessage(params op.MessageReplyParams) (op.MessageRecord, error) {
	record, _, err := s.ReplyMessageWithResolved(params)
	return record, err
}

func (s *threadStore) ReplyMessageWithResolved(params op.MessageReplyParams) (op.MessageRecord, *op.MessageRecord, error) {
	channelID := strings.TrimSpace(params.ChannelID)
	if channelID == "" {
		return op.MessageRecord{}, nil, fmt.Errorf("channelID is required")
	}
	text := strings.TrimSpace(params.Text)
	actionID := strings.TrimSpace(params.ActionID)
	answers := normalizeMessageAnswers(params.Answers)
	if text == "" && actionID == "" && len(answers) == 0 {
		return op.MessageRecord{}, nil, fmt.Errorf("text, actionID, or answers are required")
	}
	contexts, err := s.loadLiveMessageContexts(op.ThreadMetaQuery{})
	if err != nil {
		return op.MessageRecord{}, nil, err
	}
	messages, _ := filterMessageContexts(contexts, messageContextFilter{ChannelID: channelID, IncludeArch: true})
	if len(messages) == 0 {
		return op.MessageRecord{}, nil, fmt.Errorf("channel not found: %s", channelID)
	}
	parent := messages[len(messages)-1]
	replyToMessageID := strings.TrimSpace(params.ReplyToMessageID)
	var replyTo *op.MessageRecord
	if replyToMessageID != "" {
		for i := range messages {
			if strings.TrimSpace(messages[i].ID) == replyToMessageID {
				replyTo = &messages[i]
				break
			}
		}
		if replyTo == nil {
			return op.MessageRecord{}, nil, fmt.Errorf("replyToMessageID not found in channel: %s", replyToMessageID)
		}
		if replyTo.Kind == op.MessageKindRequest && (replyTo.Sender == op.MessageSenderUser || replyTo.Status != op.MessageStatusOpen) {
			return op.MessageRecord{}, nil, fmt.Errorf("request is not open: %s", replyToMessageID)
		}
		if len(answers) > 0 && replyTo.Kind != op.MessageKindRequest {
			return op.MessageRecord{}, nil, fmt.Errorf("answers require an open request reply target")
		}
	}
	if len(answers) > 0 && replyToMessageID == "" {
		return op.MessageRecord{}, nil, fmt.Errorf("answers require replyToMessageID")
	}
	if text == "" {
		if len(answers) > 0 {
			text = messageAnswersText(answers)
		} else {
			text = actionID
		}
	}
	// Routing target defaults to the channel tail so free-text replies land in
	// the most recent conversation. When the user answers a specific request,
	// the reply must land in the original request's thread so the follow-up
	// dispatch wakes the correct agent run instead of whichever thread most
	// recently published to the shared channel.
	routingThreadID := strings.TrimSpace(parent.ThreadID)
	routingAgentID := strings.TrimSpace(parent.AgentID)
	if replyTo != nil {
		routingThreadID = strings.TrimSpace(replyTo.ThreadID)
		routingAgentID = strings.TrimSpace(replyTo.AgentID)
	}
	record, err := s.AppendMessageRecord(op.MessageSenderUser, op.MessageRecord{
		ChannelID:        channelID,
		ThreadID:         routingThreadID,
		AgentID:          routingAgentID,
		Kind:             op.MessageKindMessage,
		Status:           op.MessageStatusOpen,
		Body:             text,
		ReplyToMessageID: replyToMessageID,
		ActionID:         actionID,
		Answers:          answers,
	})
	if err != nil {
		return op.MessageRecord{}, nil, err
	}
	if replyTo != nil && replyTo.Kind == op.MessageKindRequest && replyTo.Status == op.MessageStatusOpen {
		resolved, err := s.UpdateMessageRecord(op.MessageUpdateParams{
			MessageID: replyToMessageID,
			Status:    op.MessageStatusResolved,
		})
		if err != nil {
			return op.MessageRecord{}, nil, err
		}
		return record, &resolved, nil
	}
	return record, nil, nil
}

func (s *threadStore) AckMessages(params op.MessageAckParams) (op.MessageAckResult, error) {
	contexts, err := s.loadLiveMessageContexts(op.ThreadMetaQuery{ThreadID: params.ThreadID, AgentID: params.AgentID})
	if err != nil {
		return op.MessageAckResult{}, err
	}
	channelID := strings.TrimSpace(params.ChannelID)
	threadID := strings.TrimSpace(params.ThreadID)
	agentID := strings.TrimSpace(params.AgentID)
	ids := make(map[string]struct{})
	for _, id := range params.MessageIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ids[id] = struct{}{}
		}
	}
	acked := 0
	for _, current := range contexts {
		if current.record == nil {
			continue
		}
		threadRecord := current.record
		lock := s.mutexForThread(threadRecord.header.ID)
		lock.Lock()
		header, leafID, err := readJSONLHeader(threadRecord.filePath)
		if err != nil {
			lock.Unlock()
			return op.MessageAckResult{}, err
		}
		threadRecord.header = *header
		threadRecord.leafID = leafID
		ctx, _, err := readThreadContextFromFile(threadRecord.filePath)
		if err != nil {
			lock.Unlock()
			return op.MessageAckResult{}, err
		}
		entries := make([]op.ThreadMessageAckEntry, 0)
		for id := range ctx.messageState.pending {
			record, ok := ctx.messageState.messages[id]
			if !ok {
				continue
			}
			if len(ids) > 0 {
				if _, ok := ids[id]; !ok {
					continue
				}
			}
			if channelID != "" && strings.TrimSpace(record.ChannelID) != channelID {
				continue
			}
			if threadID != "" && strings.TrimSpace(record.ThreadID) != threadID {
				continue
			}
			if agentID != "" && strings.TrimSpace(record.AgentID) != agentID {
				continue
			}
			entries = append(entries, op.ThreadMessageAckEntry{MessageID: id, Pending: false})
		}
		if len(entries) == 0 {
			lock.Unlock()
			continue
		}
		nextLeafID, err := appendThreadEntries(threadRecord.filePath, threadRecord.leafID, func(parentID *string) []any {
			out := make([]any, 0, len(entries))
			currentParentID := parentID
			for _, entry := range entries {
				entryID := generateThreadEntryID()
				entry.ThreadEntryBase = op.ThreadEntryBase{
					Type:      op.ThreadEntryTypeMessageAck,
					ID:        entryID,
					ParentID:  currentParentID,
					Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				}
				out = append(out, entry)
				currentParentID = stringPtr(entryID)
			}
			return out
		})
		if err != nil {
			lock.Unlock()
			return op.MessageAckResult{}, err
		}
		threadRecord.leafID = nextLeafID
		s.cacheRecord(threadRecord)
		acked += len(entries)
		lock.Unlock()
	}
	return op.MessageAckResult{
		ChannelID: channelID,
		ThreadID:  threadID,
		AgentID:   agentID,
		Acked:     acked,
	}, nil
}

func (s *threadStore) ArchiveMessages(params op.MessageArchiveParams) (op.MessageArchiveResult, error) {
	messageID := strings.TrimSpace(params.MessageID)
	channelID := strings.TrimSpace(params.ChannelID)
	agentID := strings.TrimSpace(params.AgentID)
	if messageID == "" && channelID == "" && agentID == "" {
		return op.MessageArchiveResult{}, fmt.Errorf("messageID, channelID, or agentID is required")
	}
	contexts, err := s.loadLiveMessageContexts(op.ThreadMetaQuery{AgentID: agentID})
	if err != nil {
		return op.MessageArchiveResult{}, err
	}
	messages, _ := filterMessageContexts(contexts, messageContextFilter{
		ChannelID:   channelID,
		MessageID:   messageID,
		AgentID:     agentID,
		IncludeArch: true,
	})
	archived := 0
	for _, record := range messages {
		if record.Status == op.MessageStatusArchived {
			continue
		}
		if params.PendingRequestsOnly && !isOpenAgentRequest(record) {
			continue
		}
		updated, err := s.UpdateMessageRecord(op.MessageUpdateParams{MessageID: record.ID, Status: op.MessageStatusArchived})
		if err != nil {
			return op.MessageArchiveResult{}, err
		}
		_ = notifyMessageRecord(op.Meta{
			"threadID": updated.ThreadID,
			"agentID":  updated.AgentID,
		}, updated)
		archived++
	}
	return op.MessageArchiveResult{Archived: archived}, nil
}

func isOpenAgentRequest(record op.MessageRecord) bool {
	return record.Kind == op.MessageKindRequest &&
		record.Status == op.MessageStatusOpen &&
		record.Sender != op.MessageSenderUser
}

func (s *threadStore) findMessageByID(messageID string) (*messageThreadContext, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("messageID is required")
	}
	contexts, err := s.loadLiveMessageContexts(op.ThreadMetaQuery{})
	if err != nil {
		return nil, err
	}
	for _, current := range contexts {
		if current.ctx == nil {
			continue
		}
		if _, ok := current.ctx.messageState.messages[messageID]; ok {
			cp := current
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("message not found: %s", messageID)
}

func (s *threadStore) loadLiveMessageContexts(query op.ThreadMetaQuery) ([]messageThreadContext, error) {
	if err := ensureThreadStorageReady(); err != nil {
		return nil, err
	}
	threadID := strings.TrimSpace(query.ThreadID)
	if threadID != "" {
		record, err := s.loadRecord(query)
		if err != nil {
			return nil, err
		}
		ctx, leafID, err := readThreadContextFromFile(record.filePath)
		if err != nil {
			return nil, err
		}
		record.leafID = leafID
		s.cacheRecord(record)
		return []messageThreadContext{{record: record, ctx: ctx}}, nil
	}
	baseDir, err := threadBaseDir()
	if err != nil {
		return nil, err
	}
	threadRoot := threadStorageRootDir(baseDir)
	contexts := make([]messageThreadContext, 0)
	entries, err := os.ReadDir(filepath.Clean(threadRoot))
	if os.IsNotExist(err) {
		return contexts, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".jsonl" {
			continue
		}
		filePath := filepath.Join(filepath.Clean(threadRoot), entry.Name())
		record, err := loadThreadRecordFromFile(filePath)
		if err != nil {
			return nil, err
		}
		ctx, leafID, err := readThreadContextFromFile(filePath)
		if err != nil {
			return nil, err
		}
		record.leafID = leafID
		s.cacheRecord(record)
		contexts = append(contexts, messageThreadContext{record: record, ctx: ctx})
	}
	sort.SliceStable(contexts, func(i, j int) bool {
		left := ""
		right := ""
		if contexts[i].record != nil {
			left = strings.TrimSpace(contexts[i].record.header.Timestamp)
		}
		if contexts[j].record != nil {
			right = strings.TrimSpace(contexts[j].record.header.Timestamp)
		}
		if left == right {
			leftID, rightID := "", ""
			if contexts[i].record != nil {
				leftID = strings.TrimSpace(contexts[i].record.header.ID)
			}
			if contexts[j].record != nil {
				rightID = strings.TrimSpace(contexts[j].record.header.ID)
			}
			return leftID < rightID
		}
		return left < right
	})
	return contexts, nil
}

type messageContextFilter struct {
	ChannelID   string
	ThreadID    string
	AgentID     string
	MessageID   string
	PendingOnly bool
	IncludeArch bool
	Limit       int
}

func filterMessageContexts(contexts []messageThreadContext, filter messageContextFilter) ([]op.MessageRecord, map[string]bool) {
	pending := make(map[string]bool)
	messages := make([]op.MessageRecord, 0)
	for _, current := range contexts {
		if current.ctx == nil {
			continue
		}
		for _, id := range current.ctx.messageState.order {
			record, ok := current.ctx.messageState.messages[id]
			if !ok {
				continue
			}
			id = strings.TrimSpace(record.ID)
			if current.ctx.messageState.pending[id] {
				pending[id] = true
			}
			if filter.MessageID != "" && strings.TrimSpace(record.ID) != filter.MessageID {
				continue
			}
			if filter.ChannelID != "" && strings.TrimSpace(record.ChannelID) != filter.ChannelID {
				continue
			}
			if filter.ThreadID != "" && strings.TrimSpace(record.ThreadID) != filter.ThreadID {
				continue
			}
			if filter.AgentID != "" && strings.TrimSpace(record.AgentID) != filter.AgentID {
				continue
			}
			if filter.PendingOnly && !current.ctx.messageState.pending[id] {
				continue
			}
			if !filter.IncludeArch && record.Status == op.MessageStatusArchived {
				continue
			}
			messages = append(messages, cloneMessageRecord(record))
		}
	}
	sortMessageRecordsAscending(messages)
	if filter.Limit > 0 && len(messages) > filter.Limit {
		messages = messages[len(messages)-filter.Limit:]
	}
	return messages, pending
}

func threadMessageRecords(state threadMessageState, includeArchived bool, limit int) []op.MessageRecord {
	contexts := []messageThreadContext{{ctx: &threadContext{messageState: cloneThreadMessageState(state)}}}
	messages, _ := filterMessageContexts(contexts, messageContextFilter{IncludeArch: includeArchived, Limit: limit})
	return messages
}

func threadMessageChannelSummaries(state threadMessageState, limit int) []op.MessageChannelSummary {
	records := threadMessageRecords(state, false, limit)
	return messageChannelSummaries(records, state.pending)
}

func messageChannelSummaries(records []op.MessageRecord, pending map[string]bool) []op.MessageChannelSummary {
	channels := make(map[string]*op.MessageChannelSummary)
	for _, record := range records {
		channelID := strings.TrimSpace(record.ChannelID)
		if channelID == "" || record.Status == op.MessageStatusArchived {
			continue
		}
		summary := channels[channelID]
		if summary == nil {
			summary = &op.MessageChannelSummary{
				ChannelID: channelID,
				ThreadID:  strings.TrimSpace(record.ThreadID),
				AgentID:   strings.TrimSpace(record.AgentID),
			}
			channels[channelID] = summary
		}
		if summary.Title == "" {
			summary.Title = strings.TrimSpace(record.Title)
		}
		if isOpenAgentRequest(record) {
			summary.OpenCount++
		}
		if record.Sender == op.MessageSenderUser && pending[strings.TrimSpace(record.ID)] {
			summary.UnreadUserCount++
		}
		cloned := cloneMessageRecord(record)
		summary.LastMessage = &cloned
		summary.UpdatedAt = strings.TrimSpace(record.UpdatedAt)
		if summary.Title == "" {
			summary.Title = strings.TrimSpace(record.Title)
		}
	}
	out := make([]op.MessageChannelSummary, 0, len(channels))
	for _, summary := range channels {
		out = append(out, *summary)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := strings.TrimSpace(out[i].UpdatedAt)
		right := strings.TrimSpace(out[j].UpdatedAt)
		if left == right {
			return strings.TrimSpace(out[i].ChannelID) < strings.TrimSpace(out[j].ChannelID)
		}
		return left > right
	})
	return out
}

func sortMessageRecordsAscending(records []op.MessageRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		left := messageRecordSortTimestamp(records[i])
		right := messageRecordSortTimestamp(records[j])
		if left == right {
			return strings.TrimSpace(records[i].ID) < strings.TrimSpace(records[j].ID)
		}
		return left < right
	})
}

func messageRecordSortTimestamp(record op.MessageRecord) string {
	if updated := strings.TrimSpace(record.UpdatedAt); updated != "" {
		return updated
	}
	return strings.TrimSpace(record.CreatedAt)
}
