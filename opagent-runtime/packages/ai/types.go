package ai

import "github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"

type Usage struct {
	InputTokens      int64 `json:"inputTokens,omitempty"`
	OutputTokens     int64 `json:"outputTokens,omitempty"`
	CacheReadTokens  int64 `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64 `json:"cacheWriteTokens,omitempty"`
	TotalTokens      int64 `json:"totalTokens,omitempty"`
}

func (u Usage) PromptTokens() int64 {
	return u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens
}

func (u Usage) ResolvedTotalTokens() int64 {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.PromptTokens() + u.OutputTokens
}

type StopReason = op.MessageStopReason

const (
	StopReasonStop    = op.StopReasonStop
	StopReasonLength  = op.StopReasonLength
	StopReasonToolUse = op.StopReasonToolUse
	StopReasonError   = op.StopReasonError
	StopReasonAborted = op.StopReasonAborted
)
