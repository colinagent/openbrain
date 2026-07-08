package chat

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type transcriptTurnState struct {
	turnID    string
	seenSteps map[int]struct{}
}

var (
	transcriptMu    sync.Mutex
	transcriptTurns = make(map[string]*transcriptTurnState)
)

func ensureTranscriptTurnStateLocked(threadID, turnID string) *transcriptTurnState {
	state, ok := transcriptTurns[threadID]
	if !ok || state == nil || state.turnID != turnID {
		state = &transcriptTurnState{
			turnID:    turnID,
			seenSteps: map[int]struct{}{},
		}
		transcriptTurns[threadID] = state
	}
	return state
}

func ensureTranscriptFileFromMeta(meta op.Meta) error {
	payload := projectionPayloadFromMeta(meta)
	if strings.TrimSpace(payload.Title) == "" {
		base := strings.TrimSpace(filepath.Base(strings.TrimSpace(payload.ChatPath)))
		payload.Title = strings.TrimSpace(strings.TrimSuffix(base, filepath.Ext(base)))
		if payload.Title == "" {
			payload.Title = "Chat"
		}
	}
	return ensureProjectionFile(payload)
}

func transcriptMetaInt(meta op.Meta, key string) int {
	if meta == nil {
		return -1
	}
	switch value := meta[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return -1
		}
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return -1
}

func shouldAppendTranscriptStep(meta op.Meta) bool {
	threadID := projectionMetaString(meta, "threadID")
	turnID := projectionMetaString(meta, "turnID")
	stepSeq := transcriptMetaInt(meta, "stepSeq")
	if threadID == "" || turnID == "" || stepSeq < 0 {
		return true
	}

	transcriptMu.Lock()
	defer transcriptMu.Unlock()

	state := ensureTranscriptTurnStateLocked(threadID, turnID)
	if _, exists := state.seenSteps[stepSeq]; exists {
		return false
	}
	state.seenSteps[stepSeq] = struct{}{}
	return true
}

func appendUserStepToTranscript(meta op.Meta, msg op.Message) error {
	if !shouldAppendTranscriptStep(meta) {
		return nil
	}
	chatPath := projectionMetaString(meta, "chatPath")
	if chatPath == "" {
		return nil
	}
	if err := ensureTranscriptFileFromMeta(meta); err != nil {
		return err
	}
	userLead, err := projectionUserMarkerLine()
	if err != nil {
		return err
	}
	chunk := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(userLead),
		projectionReadableMarkdown(msg),
	}, "\n\n"))
	if err := appendProjectionChunk(chatPath, chunk); err != nil {
		return err
	}
	threadID := projectionMetaString(meta, "threadID")
	if threadID != "" {
		projectionMu.Lock()
		state := ensureProjectionStateLocked(threadID, chatPath, normalizeThreadAgentID(projectionMetaString(meta, "agentID")))
		state.userWritten = true
		projectionMu.Unlock()
	}
	return nil
}
