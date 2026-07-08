package ai

import "github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"

type ThreadContextUsage struct {
	Tokens        int64 `json:"tokens,omitempty"`
	ContextWindow int64 `json:"contextWindow,omitempty"`
	PercentMilli  int64 `json:"percentMilli,omitempty"`
	Known         bool  `json:"known,omitempty"`
}

type ThreadSnapshot struct {
	Meta               op.ThreadMeta               `json:"meta"`
	Entries            []op.ThreadEntry            `json:"entries,omitempty"`
	EntryWindow        op.ThreadEntryWindow        `json:"entryWindow,omitempty"`
	Revision           string                      `json:"revision,omitempty"`
	RunStatus          op.ThreadRunStatus          `json:"runStatus,omitempty"`
	TailStatus         op.ThreadTailStatus         `json:"tailStatus,omitempty"`
	ContinuationReason op.ThreadContinuationReason `json:"continuationReason,omitempty"`
	QueuedMessages     op.ThreadQueueSnapshot      `json:"queuedMessages,omitempty"`
	MessageRecords     []op.MessageRecord          `json:"messageRecords,omitempty"`
	ChannelSummaries   []op.MessageChannelSummary  `json:"channelSummaries,omitempty"`
	ContextUsage       ThreadContextUsage          `json:"contextUsage,omitempty"`
}
