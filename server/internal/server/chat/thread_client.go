package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/agentctx"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/colinagent/openbrain/server/internal/server/cache"
	"github.com/colinagent/openbrain/server/internal/server/chatindex"
)

type MetaResolver interface {
	GetThreadMeta(ctx context.Context, query op.ThreadMetaQuery) (*op.ThreadMeta, error)
}

type ThreadSnapshotOptions struct {
	ModelKey string
}

func decodeJSONContent[T any](content op.Content, out *T) error {
	jsonContent, ok := content.(*op.JsonContent)
	if !ok {
		return fmt.Errorf("content must be json")
	}
	return json.Unmarshal(jsonContent.Raw, out)
}

func decodeTextContent(content op.Content) string {
	textContent, ok := content.(*op.TextContent)
	if !ok || textContent == nil {
		return ""
	}
	return textContent.Text
}

func encodeJSONContent(value any) (*op.JsonContent, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &op.JsonContent{Raw: raw}, nil
}

func normalizeThreadAgentID(agentID string) string {
	return strings.TrimSpace(agentID)
}

func chatPathQueryCWD(chatPath string) string {
	normalized := filepath.Clean(strings.TrimSpace(chatPath))
	if normalized == "" || normalized == "." {
		return ""
	}
	marker := string(filepath.Separator) + filepath.Join(".agent", "chat")
	index := strings.Index(normalized, marker)
	if index <= 0 {
		return ""
	}
	return normalized[:index]
}

func resolveThreadIDByChatPath(chatPath string) string {
	cwd := strings.TrimSpace(chatPathQueryCWD(chatPath))
	if cwd == "" {
		return ""
	}
	normalizedPath := filepath.Clean(strings.TrimSpace(chatPath))
	if records, err := chatindex.ReadFileIndex(cwd); err == nil {
		for _, record := range records {
			if filepath.Clean(strings.TrimSpace(record.Path)) == normalizedPath {
				return strings.TrimSpace(record.ThreadID)
			}
		}
	}
	meta, err := agentctx.ReadChatFileMeta(normalizedPath)
	if err != nil {
		return ""
	}
	threadID := strings.TrimSpace(meta.ThreadID)
	if threadID == "" {
		return ""
	}
	agentID, bodyRecord, err := chatindex.FindThreadBody(threadID)
	if err != nil {
		return threadID
	}
	fileID := ""
	if record, err := chatindex.FindFileRecordByThreadID(cwd, threadID); err == nil && record != nil {
		fileID = strings.TrimSpace(record.FileID)
	}
	if fileID == "" {
		fileID = chatindex.GenerateFileID()
	}
	if err := chatindex.UpsertFileRecord(cwd, chatindex.FileRecord{
		FileID:   fileID,
		AgentID:  agentID,
		ThreadID: threadID,
		CWD:      cwd,
		Path:     normalizedPath,
	}); err != nil {
		return threadID
	}
	_ = chatindex.UpsertThreadRecord(agentID, chatindex.ThreadRecord{
		ThreadID: threadID,
		FileID:   fileID,
		CWD:      bodyRecord.CWD,
		ChatPath: normalizedPath,
		Path:     bodyRecord.Path,
		Title:    bodyRecord.Title,
	})
	return threadID
}

func (s *Service) CreateThread(ctx context.Context, params op.ThreadCreateParams) (*op.ThreadCreateResult, error) {
	session, err := s.getHostSession()
	if err != nil {
		return nil, err
	}
	content, err := encodeJSONContent(params)
	if err != nil {
		return nil, err
	}
	res, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpThreadCreate,
		Meta:    op.Meta{"agentID": normalizeThreadAgentID(params.AgentID)},
		Content: content,
	})
	if err != nil {
		return nil, err
	}
	var result op.ThreadCreateResult
	if err := decodeJSONContent(res.Content, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) ForkThread(ctx context.Context, params op.ThreadForkParams) (*op.ThreadMeta, error) {
	session, err := s.getHostSession()
	if err != nil {
		return nil, err
	}
	params.PlanPath = ""
	content, err := encodeJSONContent(params)
	if err != nil {
		return nil, err
	}
	meta := op.Meta{}
	if agentID := normalizeThreadAgentID(params.AgentID); agentID != "" {
		meta["agentID"] = agentID
	}
	res, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpThreadFork,
		Meta:    meta,
		Content: content,
	})
	if err != nil {
		return nil, err
	}
	var result op.ThreadMeta
	if err := decodeJSONContent(res.Content, &result); err != nil {
		return nil, err
	}
	return s.withResolvedThreadMeta(&result), nil
}

func (s *Service) GetThreadMeta(ctx context.Context, query op.ThreadMetaQuery) (*op.ThreadMeta, error) {
	session, err := s.getHostSession()
	if err != nil {
		return nil, err
	}
	runtimeQuery := query
	if strings.TrimSpace(runtimeQuery.ThreadID) == "" && strings.TrimSpace(runtimeQuery.ChatPath) != "" {
		runtimeQuery.ThreadID = resolveThreadIDByChatPath(runtimeQuery.ChatPath)
	}
	runtimeQuery.ChatPath = ""
	runtimeQuery.FileID = ""
	runtimeQuery.AgentID = ""
	content, err := encodeJSONContent(runtimeQuery)
	if err != nil {
		return nil, err
	}
	res, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpThreadMetaGet,
		Meta:    op.Meta{},
		Content: content,
	})
	if err != nil {
		return nil, err
	}
	var result op.ThreadMeta
	if err := decodeJSONContent(res.Content, &result); err != nil {
		return nil, err
	}
	return s.withResolvedThreadMeta(&result), nil
}

func (s *Service) GetThreadSnapshot(ctx context.Context, query op.ThreadMetaQuery) (*ai.ThreadSnapshot, error) {
	return s.GetThreadSnapshotWithOptions(ctx, query, ThreadSnapshotOptions{})
}

func (s *Service) GetThreadSnapshotWithOptions(ctx context.Context, query op.ThreadMetaQuery, options ThreadSnapshotOptions) (*ai.ThreadSnapshot, error) {
	session, err := s.getHostSession()
	if err != nil {
		return nil, err
	}
	runtimeQuery := query
	if strings.TrimSpace(runtimeQuery.ThreadID) == "" && strings.TrimSpace(runtimeQuery.ChatPath) != "" {
		runtimeQuery.ThreadID = resolveThreadIDByChatPath(runtimeQuery.ChatPath)
	}
	runtimeQuery.ChatPath = ""
	runtimeQuery.FileID = ""
	runtimeQuery.AgentID = ""
	content, err := encodeJSONContent(runtimeQuery)
	if err != nil {
		return nil, err
	}
	meta := op.Meta{}
	if modelKey := strings.TrimSpace(options.ModelKey); modelKey != "" {
		meta["modelKey"] = modelKey
	}
	res, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpThreadSnapshotGet,
		Meta:    meta,
		Content: content,
	})
	if err != nil {
		return nil, err
	}
	var result ai.ThreadSnapshot
	if err := decodeJSONContent(res.Content, &result); err != nil {
		return nil, err
	}
	result.Meta = *s.withResolvedThreadMeta(&result.Meta)
	return &result, nil
}

func (s *Service) UpdateThreadMeta(ctx context.Context, params op.ThreadMetaUpdateParams) (*op.ThreadMeta, error) {
	session, err := s.getHostSession()
	if err != nil {
		return nil, err
	}
	params.PlanPath = ""
	content, err := encodeJSONContent(params)
	if err != nil {
		return nil, err
	}
	res, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpThreadMetaUpdate,
		Content: content,
	})
	if err != nil {
		return nil, err
	}
	var result op.ThreadMeta
	if err := decodeJSONContent(res.Content, &result); err != nil {
		return nil, err
	}
	return s.withResolvedThreadMeta(&result), nil
}

func (s *Service) withDynamicPlanPath(meta *op.ThreadMeta) *op.ThreadMeta {
	return s.withResolvedThreadMeta(meta)
}

func (s *Service) withResolvedThreadMeta(meta *op.ThreadMeta) *op.ThreadMeta {
	if meta == nil {
		return nil
	}
	next := *meta
	if strings.TrimSpace(next.ThreadFilePath) != "" && strings.TrimSpace(next.ThreadID) != "" && strings.TrimSpace(next.FileID) == "" {
		if record, err := chatindex.FindThreadRecordByThreadIDAtRoot(filepath.Dir(strings.TrimSpace(next.ThreadFilePath)), strings.TrimSpace(next.ThreadID)); err == nil && record != nil {
			next.FileID = strings.TrimSpace(record.FileID)
		}
	}
	if strings.TrimSpace(next.CWD) != "" && strings.TrimSpace(next.ThreadID) != "" && strings.TrimSpace(next.FileID) == "" {
		if fileRecord, err := chatindex.FindFileRecordByThreadID(strings.TrimSpace(next.CWD), strings.TrimSpace(next.ThreadID)); err == nil && fileRecord != nil {
			next.FileID = strings.TrimSpace(fileRecord.FileID)
		}
	}
	if agentID := normalizeThreadAgentID(next.AgentID); agentID != "" && strings.TrimSpace(next.ThreadID) != "" && strings.TrimSpace(next.FileID) == "" {
		if record, err := chatindex.ResolveThreadRecord(agentID, strings.TrimSpace(next.ThreadID)); err == nil && record != nil {
			next.FileID = strings.TrimSpace(record.FileID)
		}
	}
	if strings.TrimSpace(next.ThreadFilePath) != "" && strings.TrimSpace(next.ThreadID) != "" && strings.TrimSpace(next.FileID) != "" {
		_ = chatindex.UpsertThreadRecordForThreadFile(strings.TrimSpace(next.ThreadFilePath), chatindex.ThreadRecord{
			ThreadID: strings.TrimSpace(next.ThreadID),
			AgentID:  strings.TrimSpace(next.AgentID),
			FileID:   strings.TrimSpace(next.FileID),
			CWD:      strings.TrimSpace(next.CWD),
			ChatPath: strings.TrimSpace(next.ChatPath),
			Path:     strings.TrimSpace(next.ThreadFilePath),
			Title:    strings.TrimSpace(next.Title),
		})
	}
	if strings.TrimSpace(next.CWD) != "" && strings.TrimSpace(next.FileID) != "" {
		if fileRecord, err := chatindex.ResolveFileRecord(strings.TrimSpace(next.CWD), strings.TrimSpace(next.FileID)); err == nil && fileRecord != nil {
			next.Path = strings.TrimSpace(fileRecord.Path)
			next.ChatPath = strings.TrimSpace(fileRecord.Path)
		}
	}
	if strings.TrimSpace(next.Path) == "" {
		next.Path = strings.TrimSpace(next.ChatPath)
	}
	planPath, err := resolveLatestPlanPath(&next)
	if err != nil {
		slog.Warn("resolve latest plan path", "threadID", strings.TrimSpace(next.ThreadID), "error", err)
		next.PlanPath = ""
		return &next
	}
	next.PlanPath = strings.TrimSpace(planPath)
	return &next
}

func (s *Service) HandleHostNotification(req *op.InfoNotificationServerRequest) {
	if req == nil || req.Params == nil {
		return
	}
	typ, _ := req.Params.Meta["type"].(string)
	switch strings.TrimSpace(typ) {
	case "user_step", "assistant_step", "tool_result_step":
		return
	case "turn_result":
		var payload op.TurnResultPayload
		if err := decodeJSONContent(req.Params.Content, &payload); err != nil {
			slog.Error("decode turn result", "error", err)
			return
		}
		if payload.ThreadID == "" {
			payload.ThreadID = projectionMetaString(req.Params.Meta, "threadID")
		}
		if payload.TurnID == "" {
			payload.TurnID = projectionMetaString(req.Params.Meta, "turnID")
		}
		if payload.ChatPath == "" {
			payload.ChatPath = projectionMetaString(req.Params.Meta, "chatPath")
			if payload.ChatPath == "" {
				payload.ChatPath = strings.TrimSpace(payload.Path)
			}
		}
		if payload.Path == "" {
			payload.Path = projectionMetaString(req.Params.Meta, "path")
			if payload.Path == "" {
				payload.Path = strings.TrimSpace(payload.ChatPath)
			}
		}
		if payload.Title == "" {
			payload.Title = projectionMetaString(req.Params.Meta, "title")
		}
		if payload.AgentID == "" {
			payload.AgentID = normalizeThreadAgentID(projectionMetaString(req.Params.Meta, "agentID"))
		}
		if err := finalizeTurnProjection(payload); err != nil {
			slog.Error("finalize turn projection", "error", err, "threadID", payload.ThreadID)
		}
	}
}

func normalizeTranscriptAgentID(agentID string) string {
	trimmed := strings.TrimSpace(agentID)
	if trimmed == "" {
		return "agent-unknown"
	}
	if strings.HasPrefix(trimmed, "agent-") {
		return trimmed
	}
	return "agent-" + trimmed
}

func isCanonicalUserID(uid string) bool {
	trimmed := strings.TrimSpace(uid)
	const prefix = "user-"
	if !strings.HasPrefix(trimmed, prefix) || len(trimmed) == len(prefix) {
		return false
	}
	for _, ch := range trimmed[len(prefix):] {
		if (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '_' ||
			ch == '-' {
			continue
		}
		return false
	}
	return true
}

func projectionUserMarkerLine() (string, error) {
	uid, err := cache.GetUserID()
	if err != nil {
		return "", err
	}
	uid = strings.TrimSpace(uid)
	if !isCanonicalUserID(uid) {
		return "", fmt.Errorf("uid must match user-*")
	}
	return "@" + uid, nil
}

func projectionAgentMarkerLine(agentID string) string {
	return "@" + normalizeTranscriptAgentID(agentID)
}

func projectionFrontmatter(payload op.TurnResultPayload) string {
	var b strings.Builder
	threadID := strings.TrimSpace(payload.ThreadID)
	title := strings.TrimSpace(payload.Title)
	b.WriteString("---\n")
	b.WriteString("thread: ")
	b.WriteString(threadID)
	b.WriteString("\n")
	b.WriteString("title: ")
	b.WriteString(strconv.Quote(title))
	if parentThreadID := strings.TrimSpace(payload.ParentThreadID); parentThreadID != "" {
		b.WriteString("\nparent_thread: ")
		b.WriteString(parentThreadID)
	}
	b.WriteString("\n")
	b.WriteString("---\n\n")
	return b.String()
}

func balanceFencedCodeBlocks(body string) string {
	openChar, openWidth := lastOpenFence(body)
	if openWidth == 0 {
		return body
	}
	var b strings.Builder
	b.Grow(len(body) + openWidth + 1)
	b.WriteString(body)
	if body != "" && !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat(string(openChar), openWidth))
	return b.String()
}

func lastOpenFence(body string) (byte, int) {
	var (
		openChar  byte
		openWidth int
	)
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		char, width, rest, ok := parseFenceLine(line)
		if !ok {
			continue
		}
		if openWidth == 0 {
			openChar = char
			openWidth = width
			_ = rest
			continue
		}
		if char == openChar && width >= openWidth && strings.TrimSpace(rest) == "" {
			openChar = 0
			openWidth = 0
		}
	}
	return openChar, openWidth
}

func parseFenceLine(line string) (byte, int, string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(trimmed) < 3 {
		return 0, 0, "", false
	}
	char := trimmed[0]
	if char != '`' && char != '~' {
		return 0, 0, "", false
	}
	width := 0
	for width < len(trimmed) && trimmed[width] == char {
		width++
	}
	if width < 3 {
		return 0, 0, "", false
	}
	return char, width, trimmed[width:], true
}
