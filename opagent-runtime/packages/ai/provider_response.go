package ai

import "github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"

func MessageUsageFromUsage(usage Usage) *op.MessageUsage {
	totalTokens := usage.ResolvedTotalTokens()
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CacheReadTokens == 0 && usage.CacheWriteTokens == 0 && totalTokens == 0 {
		return nil
	}
	return &op.MessageUsage{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		TotalTokens:      totalTokens,
	}
}

func ProviderResponseFromOpMessage(msg op.Message, usage Usage, stopReason StopReason) *ProviderResponse {
	canonical := ConversationMessage{Role: RoleCanonicalAssistant}
	if messages := CanonicalMessagesFromOp([]op.Message{msg}); len(messages) > 0 {
		canonical = messages[0]
	}
	canonical.StopReason = stopReason
	canonical.Usage = MessageUsageFromUsage(usage)
	return &ProviderResponse{
		Message:    canonical,
		Usage:      usage,
		StopReason: stopReason,
	}
}
