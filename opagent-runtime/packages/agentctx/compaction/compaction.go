package compaction

import (
	"context"
	"log/slog"
	"regexp"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const (
	DefaultReserveTokens    int64 = 16384
	DefaultKeepRecentTokens int64 = 20000

	prunedToolOutputPlaceholder = "[Old tool result content cleared]"
	defaultSummaryFallback      = "Earlier conversation history was truncated because compaction summary is unavailable."
)

type SummaryFunc func(ctx context.Context, conversation string) (string, error)

var overflowPattern = regexp.MustCompile(`(?i)(context length|too long|too many tokens|maximum context|exceeds.*context|input.*too long|token count.*exceeds|maximum prompt length|reduce the length|prompt is too long)`)

// ResolveSettings applies defaults and returns normalized compaction settings.
func ResolveSettings(cfg op.CompactionConfig) (enabled bool, reserveTokens, keepRecentTokens int64) {
	enabled = true
	if cfg.Enabled != nil {
		enabled = *cfg.Enabled
	}

	reserveTokens = cfg.ReserveTokens
	if reserveTokens <= 0 {
		reserveTokens = DefaultReserveTokens
	}

	keepRecentTokens = cfg.KeepRecentTokens
	if keepRecentTokens <= 0 {
		keepRecentTokens = DefaultKeepRecentTokens
	}
	return enabled, reserveTokens, keepRecentTokens
}

// ContextEstimate holds the result of hybrid context token estimation.
// When API usage is available, UsageTokens comes from the last assistant's
// real token count and TrailingTokens is estimated only for messages after it.
// When no usage is available, everything is estimated with chars/4.
type ContextEstimate struct {
	Tokens         int64 // total estimated context tokens
	UsageTokens    int64 // from last assistant's API usage (0 if none)
	TrailingTokens int64 // estimated tokens for messages after last usage
	LastUsageIndex int   // index of last assistant with usage, -1 if none
}

// EstimateContextTokens implements pi-mono's hybrid estimation:
// use real API usage from the last assistant message when available,
// then only estimate trailing messages with chars/4.
func EstimateContextTokens(msgs []op.Message) ContextEstimate {
	// Find last assistant message with non-nil, non-zero usage.
	lastIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == op.RoleAssistant && msgs[i].Usage != nil && msgs[i].Usage.TotalTokens > 0 {
			lastIdx = i
			break
		}
	}

	if lastIdx < 0 {
		// No usage data: estimate all messages.
		var total int64
		for _, msg := range msgs {
			total += op.EstimateMessageTokens(msg)
		}
		return ContextEstimate{Tokens: total, TrailingTokens: total, LastUsageIndex: -1}
	}

	// API usage already includes system prompt + all messages up to and including
	// the assistant response, so use it directly.
	usageTokens := msgs[lastIdx].Usage.TotalTokens

	var trailing int64
	for i := lastIdx + 1; i < len(msgs); i++ {
		trailing += op.EstimateMessageTokens(msgs[i])
	}

	return ContextEstimate{
		Tokens:         usageTokens + trailing,
		UsageTokens:    usageTokens,
		TrailingTokens: trailing,
		LastUsageIndex: lastIdx,
	}
}

// ShouldCompact returns true when estimated context tokens exceed available budget.
func ShouldCompact(estimatedTokens, contextWindow, reserveTokens int64) bool {
	if estimatedTokens <= 0 || contextWindow <= 0 {
		return false
	}
	if reserveTokens < 0 {
		reserveTokens = 0
	}
	return estimatedTokens > contextWindow-reserveTokens
}

// FindCutPoint returns the index of the first message to keep.
// Never cuts at a tool result (must stay with its preceding assistant message).
// Returns -1 when no compaction cut point is suitable.
func FindCutPoint(msgs []op.Message, keepRecentTokens int64) int {
	if len(msgs) <= 1 || keepRecentTokens <= 0 {
		return -1
	}

	accumulated := int64(0)
	start := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		accumulated += op.EstimateMessageTokens(msgs[i])
		if accumulated >= keepRecentTokens {
			start = i
			break
		}
	}
	if start < 0 {
		return -1
	}

	// Prefer cutting at the next user message for cleaner turn boundaries.
	for i := start; i < len(msgs); i++ {
		if msgs[i].Role == op.RoleUser {
			if i == 0 {
				return -1
			}
			return i
		}
	}

	// Fallback: cut at an assistant message (never at tool result).
	for i := start; i < len(msgs); i++ {
		if msgs[i].Role == op.RoleAssistant {
			if i == 0 {
				return -1
			}
			return i
		}
	}

	return -1
}

// IsOverflowError checks whether an error likely indicates context window overflow.
func IsOverflowError(err error) bool {
	if err == nil {
		return false
	}
	return overflowPattern.MatchString(err.Error())
}

// Compact summarizes older messages and returns compacted context:
// [system summary message] + kept recent messages.
func Compact(ctx context.Context, msgs []op.Message, keepRecentTokens int64, summarize SummaryFunc) ([]op.Message, error) {
	if len(msgs) == 0 || summarize == nil {
		return cloneMessages(msgs), nil
	}

	cutIdx := FindCutPoint(msgs, keepRecentTokens)
	if cutIdx <= 0 || cutIdx >= len(msgs) {
		return cloneMessages(msgs), nil
	}

	toSummarize := pruneMessagesForSummary(msgs[:cutIdx])
	conversation := BuildSummarizationInput(toSummarize)
	summary, err := summarize(ctx, conversation)
	if err != nil {
		slog.Warn("compaction summarization failed, using fallback", "error", err)
		summary = defaultSummaryFallback
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		slog.Warn("compaction summarization returned empty, using fallback")
		summary = defaultSummaryFallback
	}

	out := make([]op.Message, 0, len(msgs)-cutIdx+1)
	out = append(out, op.Message{
		Role:    op.RoleSystem,
		Content: "Context checkpoint summary:\n" + summary,
	})
	out = append(out, msgs[cutIdx:]...)
	return out, nil
}

func pruneMessagesForSummary(msgs []op.Message) []op.Message {
	out := cloneMessages(msgs)
	for i := range out {
		if out[i].Role == op.RoleTool && out[i].Content != "" {
			out[i].Content = prunedToolOutputPlaceholder
		}
	}
	return out
}

func cloneMessages(msgs []op.Message) []op.Message {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]op.Message, len(msgs))
	copy(out, msgs)
	return out
}
