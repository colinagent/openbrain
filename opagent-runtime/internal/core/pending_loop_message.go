package core

import (
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type PendingLoopMessage struct {
	Message              op.Message
	QueueKind            op.ThreadQueueKind
	QueueItemID          string
	SelectedSkillIDs     []string
	SelectedSkillContext op.Meta
	PlanTurn             bool
}

func pendingLoopMessageFromMessage(msg op.Message) PendingLoopMessage {
	return PendingLoopMessage{Message: msg}
}

func pendingLoopMessagesFromMessages(messages []op.Message) []PendingLoopMessage {
	if len(messages) == 0 {
		return nil
	}
	pending := make([]PendingLoopMessage, 0, len(messages))
	for _, msg := range messages {
		pending = append(pending, pendingLoopMessageFromMessage(msg))
	}
	return pending
}

func pendingLoopMessageFromQueueItem(item op.ThreadQueueItem, queueKind op.ThreadQueueKind) PendingLoopMessage {
	return PendingLoopMessage{
		Message:              item.Message,
		QueueKind:            queueKind,
		QueueItemID:          strings.TrimSpace(item.ID),
		SelectedSkillIDs:     append([]string(nil), item.SelectedSkillIDs...),
		SelectedSkillContext: item.SelectedSkillContext.Clone(),
		PlanTurn:             item.PlanTurn,
	}
}

func pendingLoopMessagesRaw(messages []PendingLoopMessage) []op.Message {
	if len(messages) == 0 {
		return nil
	}
	raw := make([]op.Message, 0, len(messages))
	for _, pending := range messages {
		raw = append(raw, pending.Message)
	}
	return raw
}

func hasPendingLoopQueueSource(pending PendingLoopMessage) bool {
	return pending.QueueKind != "" && strings.TrimSpace(pending.QueueItemID) != ""
}

func hasAnyPendingLoopQueueSource(messages []PendingLoopMessage) bool {
	for _, pending := range messages {
		if hasPendingLoopQueueSource(pending) {
			return true
		}
	}
	return false
}
