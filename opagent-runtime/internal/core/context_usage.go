package core

import (
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

func validCanonicalAssistantUsage(msg ai.ConversationMessage) (*op.MessageUsage, bool) {
	if msg.Role != ai.RoleCanonicalAssistant || msg.Usage == nil {
		return nil, false
	}
	if msg.StopReason == ai.StopReasonAborted || msg.StopReason == ai.StopReasonError {
		return nil, false
	}
	if resolveMessageUsageTotal(msg.Usage) <= 0 {
		return nil, false
	}
	return msg.Usage, true
}

func latestContextCheckpointIndex(messages []ai.ConversationMessage) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if isCanonicalContextCheckpoint(messages[i]) {
			return i
		}
	}
	return -1
}

func hasValidAssistantUsageAfter(messages []ai.ConversationMessage, index int) bool {
	for i := index + 1; i < len(messages); i++ {
		if _, ok := validCanonicalAssistantUsage(messages[i]); ok {
			return true
		}
	}
	return false
}

func contextPercentMilli(tokens int64, contextWindow int64) int64 {
	if tokens <= 0 || contextWindow <= 0 {
		return 0
	}
	return (tokens * 100000) / contextWindow
}

func buildCanonicalContextUsage(messages []ai.ConversationMessage, systemPrompt string, contextWindow int64) ai.ThreadContextUsage {
	if len(messages) == 0 || contextWindow <= 0 {
		return ai.ThreadContextUsage{}
	}

	checkpointIndex := latestContextCheckpointIndex(messages)
	if checkpointIndex >= 0 && !hasValidAssistantUsageAfter(messages, checkpointIndex) {
		return ai.ThreadContextUsage{
			ContextWindow: contextWindow,
			Known:         false,
		}
	}

	tokens := estimateCanonicalContextTokens(messages, systemPrompt)
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

func contextWindowForModelID(modelID string) int64 {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return 0
	}
	cfg, err := config.GetModelConfig(modelID)
	if err != nil || cfg == nil {
		return 0
	}
	return cfg.ContextWindow
}

func latestProviderStateModelIDs(messages []ai.ConversationMessage) []string {
	out := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	appendID := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for i := len(messages) - 1; i >= 0; i-- {
		state := messages[i].ProviderState
		if state == nil {
			continue
		}
		appendID(state.ProviderRef)
		appendID(state.Model)
	}
	return out
}

func resolveSnapshotContextWindow(messages []ai.ConversationMessage, meta op.Meta) int64 {
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
	for _, modelID := range latestProviderStateModelIDs(messages) {
		if contextWindow := contextWindowForModelID(modelID); contextWindow > 0 {
			return contextWindow
		}
	}
	return 0
}
