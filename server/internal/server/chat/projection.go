package chat

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/agentctx"
	"github.com/colinagent/openbrain/server/internal/server/chatindex"
)

func buildChatDir(baseDir string) (string, error) {
	return agentctx.BuildChatDir(baseDir)
}

func buildUniqueChatPath(cwd, title string) (string, error) {
	return agentctx.BuildUniqueChatPath(cwd, title)
}

func buildUniqueChatPathForFileName(baseDir, fileName string) (string, error) {
	return agentctx.BuildUniqueChatPathForFileName(baseDir, fileName)
}

func buildUniqueChatPathInDir(chatDir, fileName string) (string, error) {
	return agentctx.BuildUniqueChatPathInDir(chatDir, fileName)
}

func normalizeChatFileName(fileName string) (string, error) {
	return agentctx.NormalizeChatFileName(fileName)
}

type turnProjectionState struct {
	threadID           string
	chatPath           string
	agentID            string
	userWritten        bool
	agentHeaderWritten bool
	assistantWritten   bool
}

var (
	projectionMu             sync.Mutex
	projectionTurns          = make(map[string]*turnProjectionState)
	projectionCompletedTurns = make(map[string]struct{})
)

func ensureProjectionFile(payload op.TurnResultPayload) error {
	threadID := strings.TrimSpace(payload.ThreadID)
	chatPath := strings.TrimSpace(payload.ChatPath)
	title := strings.TrimSpace(payload.Title)
	fileID := strings.TrimSpace(payload.FileID)
	if err := agentctx.EnsureChatProjectionFile(agentctx.ChatProjectionFile{
		ThreadID:       threadID,
		Title:          title,
		ChatPath:       chatPath,
		ParentThreadID: strings.TrimSpace(payload.ParentThreadID),
	}); err != nil {
		return err
	}
	if fileID != "" {
		cwd := chatPathQueryCWD(chatPath)
		if cwd != "" {
			if err := chatindex.UpsertFileRecord(cwd, chatindex.FileRecord{
				FileID:   fileID,
				AgentID:  normalizeThreadAgentID(payload.AgentID),
				ThreadID: threadID,
				CWD:      cwd,
				Path:     chatPath,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func createProjectionFile(threadID, fileID, title, chatPath, parentThreadID string) error {
	payload := op.TurnResultPayload{
		ThreadID:       strings.TrimSpace(threadID),
		FileID:         strings.TrimSpace(fileID),
		Title:          strings.TrimSpace(title),
		ChatPath:       strings.TrimSpace(chatPath),
		Path:           strings.TrimSpace(chatPath),
		ParentThreadID: strings.TrimSpace(parentThreadID),
	}
	return ensureProjectionFile(payload)
}

func projectionMetaString(meta op.Meta, key string) string {
	if meta == nil {
		return ""
	}
	value, _ := meta[key].(string)
	return strings.TrimSpace(value)
}

func projectionPayloadFromMeta(meta op.Meta) op.TurnResultPayload {
	return op.TurnResultPayload{
		ThreadID:       projectionMetaString(meta, "threadID"),
		FileID:         projectionMetaString(meta, "fileID"),
		AgentID:        normalizeThreadAgentID(projectionMetaString(meta, "agentID")),
		Title:          projectionMetaString(meta, "title"),
		ChatPath:       projectionMetaString(meta, "chatPath"),
		Path:           projectionMetaString(meta, "path"),
		ParentThreadID: projectionMetaString(meta, "parentThreadID"),
	}
}

func ensureProjectionStateLocked(threadID, chatPath, agentID string) *turnProjectionState {
	state, ok := projectionTurns[threadID]
	if !ok || state == nil || (chatPath != "" && strings.TrimSpace(state.chatPath) != "" && strings.TrimSpace(state.chatPath) != chatPath) {
		state = &turnProjectionState{
			threadID: threadID,
			chatPath: chatPath,
			agentID:  agentID,
		}
		projectionTurns[threadID] = state
	}
	if chatPath != "" {
		state.chatPath = chatPath
	}
	if agentID != "" {
		state.agentID = agentID
	}
	return state
}

func projectionCompletedTurnKey(payload op.TurnResultPayload) string {
	threadID := strings.TrimSpace(payload.ThreadID)
	turnID := strings.TrimSpace(payload.TurnID)
	if threadID == "" || turnID == "" {
		return ""
	}
	return threadID + "/" + turnID
}

func appendProjectionChunk(chatPath, chunk string) error {
	trimmedPath := strings.TrimSpace(chatPath)
	trimmedChunk := strings.TrimSpace(chunk)
	if trimmedPath == "" || trimmedChunk == "" {
		return nil
	}
	return appendMarkdownChunk(trimmedPath, "\n"+trimmedChunk+"\n")
}

func projectionMessageMarkdown(msg op.Message) string {
	if len(msg.ContentParts) == 0 {
		return strings.TrimSpace(msg.Content)
	}
	segments := make([]string, 0, len(msg.ContentParts))
	for _, part := range msg.ContentParts {
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "", "text":
			if text := strings.TrimSpace(part.Text); text != "" {
				segments = append(segments, text)
			}
		}
	}
	if len(segments) == 0 {
		return strings.TrimSpace(msg.Content)
	}
	return strings.TrimSpace(strings.Join(segments, "\n\n"))
}

func isProjectionParticipantMarkerLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	var rest string
	switch {
	case strings.HasPrefix(trimmed, "@user-"):
		rest = strings.TrimPrefix(trimmed, "@user-")
	case strings.HasPrefix(trimmed, "@agent-"):
		rest = strings.TrimPrefix(trimmed, "@agent-")
	default:
		return false
	}
	if rest == "" {
		return false
	}
	for _, ch := range rest {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			continue
		}
		return false
	}
	return true
}

func escapeProjectionParticipantMarkerLines(markdown string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(markdown), "\r\n", "\n")
	if normalized == "" {
		return ""
	}
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		if !isProjectionParticipantMarkerLine(line) {
			continue
		}
		at := strings.Index(line, "@")
		if at < 0 {
			continue
		}
		lines[i] = line[:at] + `\` + line[at:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func projectionReadableMarkdown(msg op.Message) string {
	return escapeProjectionParticipantMarkerLines(balanceFencedCodeBlocks(projectionMessageMarkdown(msg)))
}

func appendProjectionUserLocked(state *turnProjectionState, userMessage op.Message) error {
	if state == nil || state.userWritten {
		return nil
	}
	userLead, err := projectionUserMarkerLine()
	if err != nil {
		return err
	}
	chunk := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(userLead),
		projectionReadableMarkdown(userMessage),
	}, "\n\n"))
	if err := appendProjectionChunk(state.chatPath, chunk); err != nil {
		return err
	}
	state.userWritten = true
	return nil
}

func appendProjectionAgentHeaderLocked(state *turnProjectionState) error {
	if state == nil || state.agentHeaderWritten {
		return nil
	}
	lead := strings.TrimSpace(projectionAgentMarkerLine(state.agentID))
	if lead == "" {
		state.agentHeaderWritten = true
		return nil
	}
	if err := appendProjectionChunk(state.chatPath, lead); err != nil {
		return err
	}
	state.agentHeaderWritten = true
	return nil
}

func appendProjectionAssistantLocked(state *turnProjectionState, text string) error {
	trimmed := strings.TrimSpace(text)
	if state == nil || trimmed == "" {
		return nil
	}
	if err := appendProjectionAgentHeaderLocked(state); err != nil {
		return err
	}
	if err := appendProjectionChunk(state.chatPath, escapeProjectionParticipantMarkerLines(balanceFencedCodeBlocks(trimmed))); err != nil {
		return err
	}
	state.assistantWritten = true
	return nil
}

func appendMarkdownChunk(chatPath, chunk string) error {
	if strings.TrimSpace(chunk) == "" {
		return nil
	}
	f, err := os.OpenFile(chatPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.WriteString(chunk); err != nil {
		return err
	}
	return nil
}

func finalizeTurnProjection(payload op.TurnResultPayload) error {
	threadID := strings.TrimSpace(payload.ThreadID)
	chatPath := strings.TrimSpace(payload.ChatPath)
	if threadID == "" {
		return fmt.Errorf("threadID is required")
	}
	if chatPath == "" {
		return nil
	}

	projectionMu.Lock()
	defer projectionMu.Unlock()

	completedKey := projectionCompletedTurnKey(payload)
	if completedKey != "" {
		if _, ok := projectionCompletedTurns[completedKey]; ok {
			return nil
		}
	}

	state := ensureProjectionStateLocked(threadID, chatPath, normalizeThreadAgentID(payload.AgentID))
	if err := ensureProjectionFile(payload); err != nil {
		return err
	}
	if payload.PlanTurn {
		if completedKey != "" {
			projectionCompletedTurns[completedKey] = struct{}{}
		}
		delete(projectionTurns, threadID)
		return nil
	}
	if !state.userWritten && (strings.TrimSpace(payload.UserMessage.Content) != "" || len(payload.UserMessage.ContentParts) > 0) {
		if err := appendProjectionUserLocked(state, payload.UserMessage); err != nil {
			return err
		}
	}
	if !state.assistantWritten && strings.TrimSpace(payload.AssistantText) != "" {
		if err := appendProjectionAssistantLocked(state, payload.AssistantText); err != nil {
			return err
		}
	}
	if completedKey != "" {
		projectionCompletedTurns[completedKey] = struct{}{}
	}
	delete(projectionTurns, threadID)
	return nil
}
