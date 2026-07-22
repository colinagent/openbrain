package remotecontrol

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/colinagent/openbrain/server/internal/server/chat"
)

const (
	defaultRemoteThreadLimit = 50
	maxRemoteThreadLimit     = 100
	defaultRemoteEntryLimit  = 80
	maxRemoteEntryLimit      = 200
	maxRemoteUserTextBytes   = 64 * 1024
)

type ConversationRuntime interface {
	RuntimeView
	ListActiveThreads(context.Context) ([]op.ThreadRuntimeInfo, error)
	CallAgent(context.Context, op.OpCode, op.Meta, op.Content) (*op.OpAgentResult, error)
}

type conversationService struct {
	runtime ConversationRuntime
	chat    *chat.Service
}

type remoteThreadSummary struct {
	ThreadID  string `json:"threadID"`
	Title     string `json:"title"`
	AgentID   string `json:"agentID"`
	UpdatedAt string `json:"updatedAt"`
	Running   bool   `json:"running"`
}

type threadListInput struct {
	WorkspaceID    string `json:"workspaceID"`
	BeforeThreadID string `json:"beforeThreadID,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

type threadCreateInput struct {
	WorkspaceID string `json:"workspaceID"`
	AgentID     string `json:"agentID"`
	Title       string `json:"title"`
}

type threadSnapshotInput struct {
	WorkspaceID string `json:"workspaceID"`
	ThreadID    string `json:"threadID"`
	ModelKey    string `json:"modelKey,omitempty"`
	Window      struct {
		Mode     string `json:"mode,omitempty"`
		AnchorID string `json:"anchorID,omitempty"`
		Limit    int    `json:"limit,omitempty"`
	} `json:"window,omitempty"`
}

type threadExecutionInput struct {
	WorkspaceID   string `json:"workspaceID"`
	ThreadID      string `json:"threadID"`
	AgentID       string `json:"agentID,omitempty"`
	ModelKey      string `json:"modelKey,omitempty"`
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
	Text          string `json:"text,omitempty"`
	TurnRequestID string `json:"turnRequestID,omitempty"`
	ItemID        string `json:"itemID,omitempty"`
	QueueKind     string `json:"queueKind,omitempty"`
}

type messageReplyInput struct {
	WorkspaceID      string             `json:"workspaceID"`
	ThreadID         string             `json:"threadID"`
	ChannelID        string             `json:"channelID"`
	ReplyToMessageID string             `json:"replyToMessageID"`
	Text             string             `json:"text,omitempty"`
	ActionID         string             `json:"actionID,omitempty"`
	Answers          []op.MessageAnswer `json:"answers,omitempty"`
	ModelKey         string             `json:"modelKey,omitempty"`
	ThinkingLevel    string             `json:"thinkingLevel,omitempty"`
}

type messageMarkReadInput struct {
	WorkspaceID string   `json:"workspaceID"`
	ThreadID    string   `json:"threadID"`
	ChannelID   string   `json:"channelID"`
	MessageIDs  []string `json:"messageIDs"`
}

func RegisterConversationHandlers(
	dispatcher *Dispatcher,
	runtime ConversationRuntime,
	chatService *chat.Service,
) error {
	if dispatcher == nil || runtime == nil || chatService == nil {
		return errors.New("remote conversation dependencies are required")
	}
	service := &conversationService{runtime: runtime, chat: chatService}
	registrations := []struct {
		operation  protocol.Operation
		capability protocol.Capability
		handler    Handler
	}{
		{protocol.OperationThreadList, protocol.CapabilityThreadRead, service.listThreads},
		{protocol.OperationThreadCreate, protocol.CapabilityThreadExecute, service.createThread},
		{protocol.OperationThreadSnapshot, protocol.CapabilityThreadRead, service.threadSnapshot},
		{protocol.OperationThreadSubmit, protocol.CapabilityThreadExecute, service.submitThread},
		{protocol.OperationThreadInterrupt, protocol.CapabilityThreadExecute, service.interruptThread},
		{protocol.OperationThreadContinue, protocol.CapabilityThreadExecute, service.continueThread},
		{protocol.OperationThreadSteer, protocol.CapabilityThreadExecute, service.steerThread},
		{protocol.OperationThreadFollowUp, protocol.CapabilityThreadExecute, service.followUpThread},
		{protocol.OperationThreadQueueRemove, protocol.CapabilityThreadExecute, service.removeQueuedThreadMessage},
		{protocol.OperationThreadQueuePromote, protocol.CapabilityThreadExecute, service.promoteQueuedThreadMessage},
		{protocol.OperationMessageReply, protocol.CapabilityMessageReply, service.replyMessage},
		{protocol.OperationMessageMarkRead, protocol.CapabilityMessageReply, service.markMessagesRead},
	}
	for _, registration := range registrations {
		if err := dispatcher.Register(registration.operation, registration.capability, registration.handler); err != nil {
			return err
		}
	}
	return nil
}

func (s *conversationService) listThreads(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input threadListInput
	if err := decodeRemotePayload(raw, &input); err != nil {
		return nil, invalidRemoteConversationRequest()
	}
	workspace, remoteErr := s.resolveWorkspace(ctx, input.WorkspaceID)
	if remoteErr != nil {
		return nil, remoteErr
	}
	config, err := s.runtime.GetSystemConfig(ctx)
	if err != nil {
		return nil, internalRemoteError()
	}
	records, err := readRemoteThreadHeaders(filepath.Join(config.BaseDir, "threads"), workspace.Path)
	if err != nil {
		return nil, internalRemoteError()
	}
	active, err := s.runtime.ListActiveThreads(ctx)
	if err != nil {
		return nil, internalRemoteError()
	}
	running := make(map[string]struct{}, len(active))
	for _, item := range active {
		running[strings.TrimSpace(item.ThreadID)] = struct{}{}
	}

	start := 0
	if anchor := strings.TrimSpace(input.BeforeThreadID); anchor != "" {
		start = len(records)
		for index, record := range records {
			if record.ThreadID == anchor {
				start = index + 1
				break
			}
		}
	}
	limit := clamp(input.Limit, defaultRemoteThreadLimit, maxRemoteThreadLimit)
	end := start + limit
	if end > len(records) {
		end = len(records)
	}
	threads := make([]remoteThreadSummary, 0, end-start)
	for _, record := range records[start:end] {
		if meta, err := s.chat.GetThreadMeta(ctx, op.ThreadMetaQuery{ThreadID: record.ThreadID}); err == nil && meta != nil {
			if filepath.Clean(strings.TrimSpace(meta.CWD)) != workspace.Path {
				continue
			}
			if title := strings.TrimSpace(meta.Title); title != "" {
				record.Title = title
			}
			if agentID := strings.TrimSpace(meta.AgentID); agentID != "" {
				record.AgentID = agentID
			}
		}
		_, isRunning := running[record.ThreadID]
		threads = append(threads, remoteThreadSummary{
			ThreadID: record.ThreadID, Title: record.Title, AgentID: record.AgentID,
			UpdatedAt: record.UpdatedAt.UTC().Format(time.RFC3339Nano), Running: isRunning,
		})
	}
	nextBefore := ""
	if end < len(records) && end > start {
		nextBefore = records[end-1].ThreadID
	}
	return marshalRemote(map[string]any{
		"workspaceID":        workspace.ID,
		"threads":            threads,
		"nextBeforeThreadID": nextBefore,
	})
}

func (s *conversationService) createThread(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input threadCreateInput
	if err := decodeRemotePayload(raw, &input); err != nil {
		return nil, invalidRemoteConversationRequest()
	}
	workspace, remoteErr := s.resolveWorkspace(ctx, input.WorkspaceID)
	if remoteErr != nil {
		return nil, remoteErr
	}
	agentID := strings.TrimSpace(input.AgentID)
	if !s.validAgent(ctx, agentID) {
		return nil, remoteError(protocol.ErrorOperationDenied, "selected agent is unavailable")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "New conversation"
	}
	if len(title) > 200 {
		return nil, invalidRemoteConversationRequest()
	}
	created, err := s.chat.CreateThread(ctx, op.ThreadCreateParams{
		AgentID: agentID,
		CWD:     workspace.Path,
		Title:   title,
	})
	if err != nil {
		return nil, remoteConversationError(err)
	}
	return marshalRemote(map[string]any{
		"workspaceID": workspace.ID,
		"thread": remoteThreadSummary{
			ThreadID:  created.ThreadID,
			Title:     created.Title,
			AgentID:   agentID,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
}

func (s *conversationService) threadSnapshot(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input threadSnapshotInput
	if err := decodeRemotePayload(raw, &input); err != nil {
		return nil, invalidRemoteConversationRequest()
	}
	workspace, threadMeta, remoteErr := s.resolveThread(ctx, input.WorkspaceID, input.ThreadID)
	if remoteErr != nil {
		return nil, remoteErr
	}
	window := &op.ThreadEntryWindowQuery{
		Mode:     strings.TrimSpace(input.Window.Mode),
		AnchorID: strings.TrimSpace(input.Window.AnchorID),
		Limit:    clamp(input.Window.Limit, defaultRemoteEntryLimit, maxRemoteEntryLimit),
	}
	if window.Mode == "" {
		window.Mode = op.ThreadEntryWindowModeTail
	}
	if window.Mode != op.ThreadEntryWindowModeTail && window.Mode != op.ThreadEntryWindowModeBefore && window.Mode != op.ThreadEntryWindowModeAfter {
		return nil, invalidRemoteConversationRequest()
	}
	if window.Mode != op.ThreadEntryWindowModeTail && window.AnchorID == "" {
		return nil, invalidRemoteConversationRequest()
	}
	snapshot, err := s.chat.GetThreadSnapshotWithOptions(ctx, op.ThreadMetaQuery{
		ThreadID:    threadMeta.ThreadID,
		EntryWindow: window,
	}, chat.ThreadSnapshotOptions{ModelKey: strings.TrimSpace(input.ModelKey)})
	if err != nil {
		return nil, remoteConversationError(err)
	}
	sanitizeThreadSnapshot(snapshot)
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, internalRemoteError()
	}
	if len(payload) > protocol.MaxMessageBytes-1024 {
		return nil, &protocol.RemoteError{
			Code: protocol.ErrorRateLimited, Message: "thread snapshot window exceeds the transport limit",
		}
	}
	var response map[string]any
	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, internalRemoteError()
	}
	response["workspaceID"] = workspace.ID
	return marshalRemote(response)
}

func (s *conversationService) submitThread(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input threadExecutionInput
	if err := decodeRemotePayload(raw, &input); err != nil {
		return nil, invalidRemoteConversationRequest()
	}
	text := strings.TrimSpace(input.Text)
	if text == "" || len([]byte(text)) > maxRemoteUserTextBytes || strings.TrimSpace(input.TurnRequestID) == "" {
		return nil, invalidRemoteConversationRequest()
	}
	meta, remoteErr := s.executionMeta(ctx, input, op.OpThreadSubmit)
	if remoteErr != nil {
		return nil, remoteErr
	}
	if err := s.chat.Stream(ctx, meta, &op.TextContent{Text: text}); err != nil {
		return nil, remoteConversationError(err)
	}
	return marshalRemote(map[string]any{"ok": true, "threadID": input.ThreadID, "turnRequestID": input.TurnRequestID})
}

func (s *conversationService) continueThread(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input threadExecutionInput
	if err := decodeRemotePayload(raw, &input); err != nil || strings.TrimSpace(input.TurnRequestID) == "" {
		return nil, invalidRemoteConversationRequest()
	}
	meta, remoteErr := s.executionMeta(ctx, input, op.OpThreadSubmit)
	if remoteErr != nil {
		return nil, remoteErr
	}
	if err := s.chat.Stream(ctx, meta, nil); err != nil {
		return nil, remoteConversationError(err)
	}
	return marshalRemote(map[string]any{"ok": true, "threadID": input.ThreadID, "turnRequestID": input.TurnRequestID})
}

func (s *conversationService) interruptThread(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	return s.controlThread(ctx, raw, op.OpThreadInterrupted, false)
}

func (s *conversationService) steerThread(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	return s.controlThread(ctx, raw, op.OpThreadSteer, true)
}

func (s *conversationService) followUpThread(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	return s.controlThread(ctx, raw, op.OpThreadFollowUp, true)
}

func (s *conversationService) removeQueuedThreadMessage(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	return s.controlThread(ctx, raw, op.OpThreadQueueRemove, false)
}

func (s *conversationService) promoteQueuedThreadMessage(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	return s.controlThread(ctx, raw, op.OpThreadFollowUpPromote, false)
}

func (s *conversationService) controlThread(ctx context.Context, raw json.RawMessage, opcode op.OpCode, requiresText bool) (json.RawMessage, *protocol.RemoteError) {
	var input threadExecutionInput
	if err := decodeRemotePayload(raw, &input); err != nil {
		return nil, invalidRemoteConversationRequest()
	}
	meta, remoteErr := s.executionMeta(ctx, input, opcode)
	if remoteErr != nil {
		return nil, remoteErr
	}
	var content op.Content
	if requiresText {
		text := strings.TrimSpace(input.Text)
		if text == "" || len([]byte(text)) > maxRemoteUserTextBytes {
			return nil, invalidRemoteConversationRequest()
		}
		content = &op.TextContent{Text: text}
	}
	if opcode == op.OpThreadQueueRemove {
		queueKind := strings.TrimSpace(input.QueueKind)
		if queueKind != string(op.ThreadQueueKindSteering) && queueKind != string(op.ThreadQueueKindFollowUp) {
			return nil, invalidRemoteConversationRequest()
		}
		meta["queueKind"] = queueKind
	}
	if opcode == op.OpThreadQueueRemove || opcode == op.OpThreadFollowUpPromote {
		if strings.TrimSpace(input.ItemID) == "" {
			return nil, invalidRemoteConversationRequest()
		}
		meta["itemID"] = strings.TrimSpace(input.ItemID)
	}
	ack, err := s.chat.Control(ctx, meta, content)
	if err != nil {
		return nil, remoteConversationError(err)
	}
	return marshalRemote(map[string]any{"ok": true, "threadID": ack.ThreadID})
}

func (s *conversationService) replyMessage(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input messageReplyInput
	if err := decodeRemotePayload(raw, &input); err != nil {
		return nil, invalidRemoteConversationRequest()
	}
	_, _, snapshot, remoteErr := s.snapshotForMessage(ctx, input.WorkspaceID, input.ThreadID)
	if remoteErr != nil {
		return nil, remoteErr
	}
	replyTo := strings.TrimSpace(input.ReplyToMessageID)
	channelID := strings.TrimSpace(input.ChannelID)
	var requestRecord *op.MessageRecord
	for index := range snapshot.MessageRecords {
		record := &snapshot.MessageRecords[index]
		if record.ID == replyTo && record.ChannelID == channelID && record.Status == op.MessageStatusOpen {
			requestRecord = record
			break
		}
	}
	if requestRecord == nil || (strings.TrimSpace(input.Text) == "" && strings.TrimSpace(input.ActionID) == "" && len(input.Answers) == 0) {
		return nil, invalidRemoteConversationRequest()
	}
	if len([]byte(strings.TrimSpace(input.Text))) > maxRemoteUserTextBytes {
		return nil, invalidRemoteConversationRequest()
	}
	actionID := strings.TrimSpace(input.ActionID)
	if actionID != "" {
		valid := false
		for _, action := range requestRecord.Actions {
			if action.ID == actionID {
				valid = true
				break
			}
		}
		if !valid {
			return nil, invalidRemoteConversationRequest()
		}
	}
	normalizedAnswers, valid := normalizeMessageReplyAnswers(requestRecord.Questions, input.Answers, actionID)
	if !valid {
		return nil, invalidRemoteConversationRequest()
	}
	modelKey := strings.TrimSpace(input.ModelKey)
	thinking := strings.TrimSpace(input.ThinkingLevel)
	if modelKey != "" && !s.validModel(ctx, modelKey, thinking) {
		return nil, remoteError(protocol.ErrorOperationDenied, "selected model is unavailable")
	}
	params := op.MessageReplyParams{
		ChannelID: channelID, ReplyToMessageID: replyTo,
		Text: strings.TrimSpace(input.Text), ActionID: actionID,
		Answers: normalizedAnswers,
	}
	content, err := jsonContent(params)
	if err != nil {
		return nil, internalRemoteError()
	}
	meta := op.Meta{}
	if modelKey != "" {
		meta["modelKey"] = modelKey
	}
	if thinking != "" {
		meta["thinkingLevel"] = thinking
	}
	result, err := s.runtime.CallAgent(ctx, op.OpMessageReply, meta, content)
	if err != nil || result == nil || result.Content == nil {
		return nil, remoteConversationError(err)
	}
	var reply op.MessageReplyResult
	if err := decodeJSONContent(result.Content, &reply); err != nil {
		return nil, internalRemoteError()
	}
	dispatch := reply.Dispatch
	reply.Dispatch = nil
	reply.Queue = nil
	if dispatch != nil {
		go func() {
			dispatchMeta := dispatch.Meta.Clone()
			if dispatchMeta == nil {
				dispatchMeta = op.Meta{}
			}
			dispatchMeta["opcode"] = string(op.OpThreadSubmit)
			_ = s.chat.Stream(context.WithoutCancel(ctx), dispatchMeta, nil)
		}()
	}
	resolvedMessageID := ""
	if reply.Resolved != nil {
		resolvedMessageID = reply.Resolved.ID
	}
	return marshalRemote(map[string]any{
		"ok": true, "threadID": input.ThreadID,
		"messageID": reply.Record.ID, "resolvedMessageID": resolvedMessageID,
	})
}

func normalizeMessageReplyAnswers(questions []op.MessageQuestion, answers []op.MessageAnswer, actionID string) ([]op.MessageAnswer, bool) {
	if strings.TrimSpace(actionID) != "" && len(answers) == 0 {
		return nil, true
	}
	return validateMessageAnswers(questions, answers)
}

func validateMessageAnswers(questions []op.MessageQuestion, answers []op.MessageAnswer) ([]op.MessageAnswer, bool) {
	if len(questions) == 0 {
		return nil, len(answers) == 0
	}
	if len(answers) != len(questions) {
		return nil, false
	}
	byQuestion := make(map[string]op.MessageAnswer, len(answers))
	for _, answer := range answers {
		questionID := strings.TrimSpace(answer.QuestionID)
		if questionID == "" {
			return nil, false
		}
		if _, duplicate := byQuestion[questionID]; duplicate {
			return nil, false
		}
		byQuestion[questionID] = answer
	}
	normalized := make([]op.MessageAnswer, 0, len(questions))
	for _, question := range questions {
		answer, ok := byQuestion[question.ID]
		if !ok {
			return nil, false
		}
		if answer.Other {
			text := strings.TrimSpace(answer.Text)
			if text == "" || len([]byte(text)) > maxRemoteUserTextBytes {
				return nil, false
			}
			normalized = append(normalized, op.MessageAnswer{QuestionID: question.ID, Other: true, Text: text})
			continue
		}
		optionID := strings.TrimSpace(answer.OptionID)
		var selected *op.MessageQuestionOption
		for index := range question.Options {
			if question.Options[index].ID == optionID {
				selected = &question.Options[index]
				break
			}
		}
		if selected == nil {
			return nil, false
		}
		normalized = append(normalized, op.MessageAnswer{
			QuestionID: question.ID, OptionID: selected.ID, Label: selected.Label,
		})
	}
	return normalized, true
}

func (s *conversationService) markMessagesRead(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input messageMarkReadInput
	if err := decodeRemotePayload(raw, &input); err != nil || len(input.MessageIDs) == 0 || len(input.MessageIDs) > 100 {
		return nil, invalidRemoteConversationRequest()
	}
	_, _, snapshot, remoteErr := s.snapshotForMessage(ctx, input.WorkspaceID, input.ThreadID)
	if remoteErr != nil {
		return nil, remoteErr
	}
	channelID := strings.TrimSpace(input.ChannelID)
	allowed := make(map[string]struct{}, len(snapshot.MessageRecords))
	for _, record := range snapshot.MessageRecords {
		if record.ChannelID == channelID {
			allowed[record.ID] = struct{}{}
		}
	}
	messageIDs := make([]string, 0, len(input.MessageIDs))
	for _, messageID := range input.MessageIDs {
		messageID = strings.TrimSpace(messageID)
		if _, ok := allowed[messageID]; !ok {
			return nil, invalidRemoteConversationRequest()
		}
		messageIDs = append(messageIDs, messageID)
	}
	content, err := jsonContent(op.MessageAckParams{
		ChannelID: channelID, ThreadID: strings.TrimSpace(input.ThreadID), MessageIDs: messageIDs,
	})
	if err != nil {
		return nil, internalRemoteError()
	}
	result, err := s.runtime.CallAgent(ctx, op.OpMessageAck, nil, content)
	if err != nil || result == nil || result.Content == nil {
		return nil, remoteConversationError(err)
	}
	var ack op.MessageAckResult
	if err := decodeJSONContent(result.Content, &ack); err != nil {
		return nil, internalRemoteError()
	}
	return marshalRemote(ack)
}

func (s *conversationService) executionMeta(ctx context.Context, input threadExecutionInput, opcode op.OpCode) (op.Meta, *protocol.RemoteError) {
	_, threadMeta, remoteErr := s.resolveThread(ctx, input.WorkspaceID, input.ThreadID)
	if remoteErr != nil {
		return nil, remoteErr
	}
	agentID := strings.TrimSpace(input.AgentID)
	if agentID == "" {
		agentID = strings.TrimSpace(threadMeta.AgentID)
	}
	modelKey := strings.TrimSpace(input.ModelKey)
	thinking := strings.TrimSpace(input.ThinkingLevel)
	requiresExecutionSelection := opcode == op.OpThreadSubmit || opcode == op.OpThreadSteer || opcode == op.OpThreadFollowUp
	if requiresExecutionSelection && !s.validAgent(ctx, agentID) {
		return nil, remoteError(protocol.ErrorOperationDenied, "selected agent is unavailable")
	}
	if requiresExecutionSelection {
		if !s.validModel(ctx, modelKey, thinking) {
			return nil, remoteError(protocol.ErrorOperationDenied, "selected model is unavailable")
		}
	}
	meta := op.Meta{
		"opcode": string(opcode), "threadID": threadMeta.ThreadID, "fileID": threadMeta.FileID,
		"cwd": threadMeta.CWD, "path": threadMeta.Path, "chatPath": threadMeta.ChatPath,
		"title": threadMeta.Title, "agentID": agentID,
	}
	if modelKey != "" {
		meta["modelKey"] = modelKey
	}
	if thinking != "" {
		meta["thinkingLevel"] = thinking
	}
	if turnRequestID := strings.TrimSpace(input.TurnRequestID); turnRequestID != "" {
		meta["turnRequestID"] = turnRequestID
	}
	return meta, nil
}

func (s *conversationService) resolveWorkspace(ctx context.Context, workspaceID string) (workspaceAccess, *protocol.RemoteError) {
	workspace, err := defaultWorkspace(ctx, s.runtime)
	if err != nil {
		return workspaceAccess{}, internalRemoteError()
	}
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(workspaceID) != workspace.ID {
		return workspaceAccess{}, remoteError(protocol.ErrorWorkspaceNotFound, "workspace was not found")
	}
	return workspace, nil
}

func (s *conversationService) resolveThread(ctx context.Context, workspaceID, threadID string) (workspaceAccess, *op.ThreadMeta, *protocol.RemoteError) {
	workspace, remoteErr := s.resolveWorkspace(ctx, workspaceID)
	if remoteErr != nil {
		return workspaceAccess{}, nil, remoteErr
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return workspaceAccess{}, nil, invalidRemoteConversationRequest()
	}
	meta, err := s.chat.GetThreadMeta(ctx, op.ThreadMetaQuery{ThreadID: threadID})
	if err != nil || meta == nil || strings.TrimSpace(meta.ThreadID) != threadID {
		return workspaceAccess{}, nil, remoteError(protocol.ErrorThreadNotFound, "thread was not found")
	}
	if filepath.Clean(strings.TrimSpace(meta.CWD)) != workspace.Path {
		return workspaceAccess{}, nil, remoteError(protocol.ErrorThreadNotFound, "thread was not found")
	}
	return workspace, meta, nil
}

func (s *conversationService) snapshotForMessage(ctx context.Context, workspaceID, threadID string) (workspaceAccess, *op.ThreadMeta, *ai.ThreadSnapshot, *protocol.RemoteError) {
	workspace, meta, remoteErr := s.resolveThread(ctx, workspaceID, threadID)
	if remoteErr != nil {
		return workspaceAccess{}, nil, nil, remoteErr
	}
	snapshot, err := s.chat.GetThreadSnapshot(ctx, op.ThreadMetaQuery{ThreadID: meta.ThreadID})
	if err != nil {
		return workspaceAccess{}, nil, nil, remoteConversationError(err)
	}
	return workspace, meta, snapshot, nil
}

func (s *conversationService) validAgent(ctx context.Context, agentID string) bool {
	if agentID == "" {
		return false
	}
	nodes, err := s.runtime.ListNodes(ctx)
	if err != nil {
		return false
	}
	for _, node := range nodes {
		if node != nil && node.ID == agentID && nodeSupportsThreadSubmit(node) {
			return true
		}
	}
	return false
}

func (s *conversationService) validModel(ctx context.Context, modelKey, thinking string) bool {
	if modelKey == "" {
		return false
	}
	config, err := s.runtime.GetConfigContext(ctx)
	if err != nil || config.User == nil {
		return false
	}
	for _, model := range config.User.Models {
		if !model.Enabled || strings.TrimSpace(model.Key) != modelKey {
			continue
		}
		if thinking == "" {
			return true
		}
		for _, level := range model.ReasoningLevels {
			if strings.TrimSpace(level) == thinking {
				return true
			}
		}
		return false
	}
	return false
}

type remoteThreadHeader struct {
	ThreadID  string
	Title     string
	AgentID   string
	UpdatedAt time.Time
}

func readRemoteThreadHeaders(root, workspacePath string) ([]remoteThreadHeader, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []remoteThreadHeader{}, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]remoteThreadHeader, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(io.LimitReader(file, 1024*1024))
		found := scanner.Scan()
		_ = file.Close()
		if !found {
			continue
		}
		var header op.ThreadHeader
		if json.Unmarshal(scanner.Bytes(), &header) != nil || header.Type != "thread" || strings.TrimSpace(header.ID) == "" {
			continue
		}
		if filepath.Clean(strings.TrimSpace(header.CWD)) != workspacePath {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		title := strings.TrimSpace(header.Title)
		if title == "" {
			title = "Conversation"
		}
		records = append(records, remoteThreadHeader{
			ThreadID: strings.TrimSpace(header.ID), Title: title,
			AgentID: strings.TrimSpace(header.AgentID), UpdatedAt: info.ModTime(),
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].UpdatedAt.Equal(records[j].UpdatedAt) {
			return records[i].ThreadID > records[j].ThreadID
		}
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	return records, nil
}

func sanitizeThreadSnapshot(snapshot *ai.ThreadSnapshot) {
	if snapshot == nil {
		return
	}
	snapshot.Meta.CWD = ""
	snapshot.Meta.Path = ""
	snapshot.Meta.ChatPath = ""
	snapshot.Meta.ThreadFilePath = ""
	snapshot.Meta.PlanPath = ""
	snapshot.Meta.ExecutionPlanPath = ""
	for index := range snapshot.Entries {
		entry := &snapshot.Entries[index]
		if len(entry.Raw) == 0 {
			continue
		}
		var object map[string]any
		if json.Unmarshal(entry.Raw, &object) != nil {
			continue
		}
		for _, key := range []string{"cwd", "path", "chatPath", "threadFilePath", "planPath", "executionPlanPath"} {
			delete(object, key)
		}
		if item, ok := object["item"].(map[string]any); ok {
			delete(item, "cwd")
		}
		if raw, err := json.Marshal(object); err == nil {
			entry.Raw = raw
		}
	}
	for index := range snapshot.QueuedMessages.Steering {
		snapshot.QueuedMessages.Steering[index].CWD = ""
	}
	for index := range snapshot.QueuedMessages.FollowUp {
		snapshot.QueuedMessages.FollowUp[index].CWD = ""
	}
	for index := range snapshot.MessageRecords {
		sanitizeMessageRecord(&snapshot.MessageRecords[index])
	}
	for index := range snapshot.ChannelSummaries {
		if snapshot.ChannelSummaries[index].LastMessage != nil {
			sanitizeMessageRecord(snapshot.ChannelSummaries[index].LastMessage)
		}
	}
}

func sanitizeMessageRecord(record *op.MessageRecord) {
	if record == nil || record.Meta == nil {
		return
	}
	for _, key := range []string{"cwd", "path", "chatPath", "threadFilePath", "planPath", "executionPlanPath"} {
		delete(record.Meta, key)
	}
}

func decodeRemotePayload(raw json.RawMessage, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("remote payload has trailing content")
	}
	return nil
}

func jsonContent(value any) (*op.JsonContent, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &op.JsonContent{Raw: raw}, nil
}

func decodeJSONContent(content op.Content, out any) error {
	jsonContent, ok := content.(*op.JsonContent)
	if !ok || jsonContent == nil {
		return errors.New("expected JSON content")
	}
	return json.Unmarshal(jsonContent.Raw, out)
}

func clamp(value, fallback, maximum int) int {
	if value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func invalidRemoteConversationRequest() *protocol.RemoteError {
	return remoteError(protocol.ErrorInvalidEnvelope, "remote conversation request is invalid")
}

func remoteConversationError(err error) *protocol.RemoteError {
	if err == nil {
		return internalRemoteError()
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not found"):
		return remoteError(protocol.ErrorThreadNotFound, "thread was not found")
	case strings.Contains(message, "already running"), strings.Contains(message, "busy"):
		return remoteError(protocol.ErrorThreadBusy, "thread is already running")
	case strings.Contains(message, "required"), strings.Contains(message, "invalid"):
		return invalidRemoteConversationRequest()
	default:
		return internalRemoteError()
	}
}
