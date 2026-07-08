package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/notify"
)

// Request represents a chat request from the HTTP layer.
type Request struct {
	Meta    op.Meta
	Content op.Content
}

const unauthorizedMessage = "Please sign in first."
const unauthorizedError = "unauthorized: please sign in first"

// Service handles chat workflows.
type Service struct {
	notify  *notify.Service
	mu      sync.RWMutex
	session *op.ServerSession
}

func NewService(notifySvc *notify.Service) *Service {
	return &Service{
		notify: notifySvc,
	}
}

// SetSession updates the host session used for chat thread submission.
func (s *Service) SetHostSession(session *op.ServerSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = session
}

func (s *Service) getHostSession() (*op.ServerSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.session == nil {
		return nil, fmt.Errorf("host session not initialized")
	}
	return s.session, nil
}

func (s *Service) notifyError(meta op.Meta, text string) {
	if s == nil || s.notify == nil {
		return
	}
	s.notify.NotifyError(meta, &op.TextContent{Text: text})
}

func (s *Service) notifyEnd(meta op.Meta) {
	if s == nil || s.notify == nil {
		return
	}
	s.notify.NotifyEnd(meta, &op.TextContent{Text: ""})
}

// Stream starts a chat workflow and streams results via SSE.
func (s *Service) Stream(ctx context.Context, meta op.Meta, content op.Content) error {
	codeStr, ok := meta["opcode"].(string)
	if !ok || codeStr == "" {
		s.notifyError(meta, "opcode is required")
		s.notifyEnd(meta)
		return fmt.Errorf("opcode is required")
	}

	switch op.OpCode(codeStr) {
	case op.OpThreadSubmit:
		return s.callThreadSubmit(ctx, meta, content)
	default:
		s.notifyError(meta, "unsupported opcode")
		s.notifyEnd(meta)
		return fmt.Errorf("unsupported opcode: %s", codeStr)
	}
}

func (s *Service) Control(ctx context.Context, meta op.Meta, content op.Content) (*op.ThreadControlAck, error) {
	codeStr, ok := meta["opcode"].(string)
	if !ok || codeStr == "" {
		return nil, fmt.Errorf("opcode is required")
	}
	if _, err := s.validateThreadMeta(ctx, meta); err != nil {
		return nil, err
	}

	switch op.OpCode(codeStr) {
	case op.OpThreadSteer,
		op.OpThreadCompact,
		op.OpThreadFollowUp,
		op.OpThreadFollowUpPromote,
		op.OpThreadQueueGet,
		op.OpThreadQueueRemove,
		op.OpThreadInterrupted:
		return s.callThreadControl(ctx, op.OpCode(codeStr), meta, content)
	default:
		return nil, fmt.Errorf("unsupported control opcode: %s", codeStr)
	}
}

func (s *Service) validateThreadMeta(ctx context.Context, meta op.Meta) (*op.ThreadMeta, error) {
	threadID, _ := meta["threadID"].(string)
	fileID, _ := meta["fileID"].(string)
	threadID = strings.TrimSpace(threadID)
	fileID = strings.TrimSpace(fileID)
	if threadID == "" {
		return nil, fmt.Errorf("threadID is required")
	}

	threadMeta, err := s.GetThreadMeta(ctx, op.ThreadMetaQuery{ThreadID: threadID})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(threadMeta.ThreadID) != threadID {
		return nil, fmt.Errorf("threadID does not match thread metadata")
	}
	resolvedFileID := strings.TrimSpace(threadMeta.FileID)
	if fileID != "" && resolvedFileID != "" && resolvedFileID != fileID {
		return nil, fmt.Errorf("fileID does not match thread metadata")
	}
	if fileID != "" && resolvedFileID == "" {
		return nil, fmt.Errorf("fileID does not match thread metadata")
	}
	resolvedPath := strings.TrimSpace(threadMeta.Path)
	if resolvedPath == "" {
		resolvedPath = strings.TrimSpace(threadMeta.ChatPath)
	}
	threadMeta.FileID = resolvedFileID
	threadMeta.Path = resolvedPath
	threadMeta.ChatPath = resolvedPath
	return threadMeta, nil
}

func (s *Service) callThreadSubmit(ctx context.Context, meta op.Meta, content op.Content) error {
	session, err := s.getHostSession()
	if err != nil {
		s.notifyError(meta, err.Error())
		s.notifyEnd(meta)
		return err
	}

	res, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpThreadSubmit,
		Meta:    meta,
		Content: content,
	})
	if err != nil {
		s.notifyError(meta, err.Error())
		s.notifyEnd(meta)
		return err
	}
	if res == nil || res.Content == nil {
		s.notifyError(meta, "agent returned empty response")
		s.notifyEnd(meta)
		return fmt.Errorf("agent returned empty response")
	}

	return s.handleNodeResult(meta, res)
}

func (s *Service) callTool(ctx context.Context, meta op.Meta, content op.Content) error {
	jsonContent, ok := content.(*op.JsonContent)
	if !ok {
		s.notifyError(meta, "invalid tool call payload")
		s.notifyEnd(meta)
		return fmt.Errorf("failed to cast content to JsonContent")
	}

	var toolCallParams op.CallToolParamsRaw
	if err := json.Unmarshal(jsonContent.Raw, &toolCallParams); err != nil {
		s.notifyError(meta, err.Error())
		s.notifyEnd(meta)
		return err
	}
	if toolCallParams.Name == "" {
		s.notifyError(meta, "name is required")
		s.notifyEnd(meta)
		return fmt.Errorf("name is required")
	}

	session, err := s.getHostSession()
	if err != nil {
		s.notifyError(meta, err.Error())
		s.notifyEnd(meta)
		return err
	}

	_, err = session.OpNode(
		ctx,
		&op.OpNodeParams{
			OpCode:  op.OpThreadSubmit,
			Meta:    meta,
			Content: content,
		})
	if err != nil {
		s.notifyError(meta, err.Error())
		s.notifyEnd(meta)
		return err
	}

	return nil
}

func (s *Service) callThreadControl(ctx context.Context, opcode op.OpCode, meta op.Meta, content op.Content) (*op.ThreadControlAck, error) {
	session, err := s.getHostSession()
	if err != nil {
		return nil, err
	}
	res, err := session.OpAgent(
		ctx,
		&op.OpAgentParams{
			OpCode:  opcode,
			Meta:    meta,
			Content: content,
		})
	if err != nil {
		return nil, err
	}
	if res == nil || res.Content == nil {
		threadID, _ := meta["threadID"].(string)
		return &op.ThreadControlAck{
			OK:       true,
			ThreadID: strings.TrimSpace(threadID),
			OpCode:   opcode,
		}, nil
	}
	jsonContent, ok := res.Content.(*op.JsonContent)
	if !ok {
		return nil, fmt.Errorf("thread control returned invalid response content: %T", res.Content)
	}
	var ack op.ThreadControlAck
	if err := json.Unmarshal(jsonContent.Raw, &ack); err != nil {
		return nil, fmt.Errorf("decode thread control ack: %w", err)
	}
	if ack.ThreadID == "" {
		threadID, _ := meta["threadID"].(string)
		ack.ThreadID = strings.TrimSpace(threadID)
	}
	if ack.OpCode == "" {
		ack.OpCode = opcode
	}
	ack.OK = true
	return &ack, nil
}

func (s *Service) handleNodeResult(meta op.Meta, result *op.OpNodeResult) error {
	if result == nil || result.Content == nil {
		s.notifyEnd(meta)
		return nil
	}

	if streamEndEmitted(result.Meta) {
		return nil
	}

	s.notifyEnd(meta)
	return nil
}

func streamEndEmitted(meta op.Meta) bool {
	if meta == nil {
		return false
	}
	switch value := meta["streamEndEmitted"].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}
func (s *Service) logStreamError(err error, threadID string) {
	if err == nil {
		return
	}
	slog.Error("chat stream failed", "error", err, "threadID", threadID)
}
