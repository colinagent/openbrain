package ai

import (
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type ReplayableOpMessagesResult struct {
	Messages             []op.Message
	ContinuationRequired bool
}

type ReplayableCanonicalMessagesResult struct {
	Messages             []ConversationMessage
	ContinuationRequired bool
}

func CanonicalMessagesFromReplayableOp(messages []op.Message) []ConversationMessage {
	result := NormalizeReplayableOpMessages(messages)
	return canonicalMessagesFromOp(result.Messages, true)
}

func NormalizeReplayableCanonicalMessages(messages []ConversationMessage) ReplayableCanonicalMessagesResult {
	if len(messages) == 0 {
		return ReplayableCanonicalMessagesResult{}
	}

	messages = pruneSkippedCanonicalTurns(messages)
	if len(messages) == 0 {
		return ReplayableCanonicalMessagesResult{}
	}

	out := make([]ConversationMessage, 0, len(messages))
	pendingToolCalls := make([]CanonicalToolCall, 0, 1)
	existingToolResultIDs := make(map[string]struct{}, 1)
	flushPendingToolCalls := func() {
		if len(pendingToolCalls) == 0 {
			return
		}
		for _, toolCall := range pendingToolCalls {
			callID := strings.TrimSpace(toolCall.ID)
			if callID == "" {
				continue
			}
			if _, ok := existingToolResultIDs[callID]; ok {
				continue
			}
			out = append(out, ConversationMessage{
				Role: RoleCanonicalTool,
				Content: []ContentBlock{{
					Type: BlockToolResult,
					ToolResult: &CanonicalToolResult{
						ToolCallID: callID,
						ToolName:   strings.TrimSpace(toolCall.Name),
						IsError:    true,
						OutputText: "No result provided",
					},
				}},
			})
		}
		pendingToolCalls = nil
		existingToolResultIDs = make(map[string]struct{}, 1)
	}

	for _, msg := range messages {
		switch msg.Role {
		case RoleCanonicalAssistant:
			flushPendingToolCalls()
			if isReplaySkippedCanonicalAssistant(msg) {
				continue
			}
			toolCalls := canonicalToolCallsFromMessage(msg)
			if len(toolCalls) > 0 {
				pendingToolCalls = append([]CanonicalToolCall(nil), toolCalls...)
				existingToolResultIDs = make(map[string]struct{}, len(toolCalls))
			}
			out = append(out, msg)
		case RoleCanonicalTool:
			for _, block := range msg.Content {
				if block.Type == BlockToolResult && block.ToolResult != nil {
					if callID := strings.TrimSpace(block.ToolResult.ToolCallID); callID != "" {
						existingToolResultIDs[callID] = struct{}{}
					}
				}
			}
			out = append(out, msg)
		case RoleCanonicalUser, RoleCanonicalSystem, RoleCanonicalDeveloper, RoleCanonicalCompaction:
			flushPendingToolCalls()
			out = append(out, msg)
		default:
			flushPendingToolCalls()
			out = append(out, msg)
		}
	}
	flushPendingToolCalls()

	return ReplayableCanonicalMessagesResult{
		Messages:             out,
		ContinuationRequired: ContinuationRequiredForCanonicalMessages(messages),
	}
}

func NormalizeReplayableOpMessages(messages []op.Message) ReplayableOpMessagesResult {
	if len(messages) == 0 {
		return ReplayableOpMessagesResult{}
	}

	messages = pruneSkippedOpTurns(messages)
	if len(messages) == 0 {
		return ReplayableOpMessagesResult{}
	}

	out := make([]op.Message, 0, len(messages))
	pendingToolCalls := make([]op.MessageToolCall, 0, 1)
	existingToolResultIDs := make(map[string]struct{}, 1)
	flushPendingToolCalls := func() {
		if len(pendingToolCalls) == 0 {
			return
		}
		for _, toolCall := range pendingToolCalls {
			callID := strings.TrimSpace(toolCall.ID)
			if callID == "" {
				continue
			}
			if _, ok := existingToolResultIDs[callID]; ok {
				continue
			}
			out = append(out, op.Message{
				Role:       op.RoleTool,
				Name:       strings.TrimSpace(toolCall.Name),
				ToolCallID: callID,
				Content:    "No result provided",
			})
		}
		pendingToolCalls = nil
		existingToolResultIDs = make(map[string]struct{}, 1)
	}

	for _, msg := range messages {
		switch msg.Role {
		case op.RoleAssistant:
			flushPendingToolCalls()
			if isReplaySkippedAssistant(msg) {
				continue
			}
			if len(msg.ToolCalls) > 0 {
				pendingToolCalls = append([]op.MessageToolCall(nil), msg.ToolCalls...)
				existingToolResultIDs = make(map[string]struct{}, len(msg.ToolCalls))
			}
			out = append(out, msg)
		case op.RoleTool:
			if callID := strings.TrimSpace(msg.ToolCallID); callID != "" {
				existingToolResultIDs[callID] = struct{}{}
			}
			out = append(out, msg)
		case op.RoleUser, op.RoleSystem, op.RoleDeveloper, op.RoleFunction:
			flushPendingToolCalls()
			out = append(out, msg)
		default:
			flushPendingToolCalls()
			out = append(out, msg)
		}
	}
	flushPendingToolCalls()

	return ReplayableOpMessagesResult{
		Messages:             out,
		ContinuationRequired: ContinuationRequiredForOpMessages(messages),
	}
}

func ContinuationRequiredForOpMessages(messages []op.Message) bool {
	if len(messages) == 0 {
		return false
	}
	last := messages[len(messages)-1]
	switch last.Role {
	case op.RoleAssistant:
		if isCompleteAssistantTail(last) {
			return false
		}
		return true
	case op.RoleUser, op.RoleTool, op.RoleFunction:
		return true
	default:
		return false
	}
}

func ContinuationRequiredForCanonicalMessages(messages []ConversationMessage) bool {
	status, _ := CanonicalMessagesTailState(messages)
	return status == op.ThreadTailNeedsContinuation
}

func CanonicalMessagesTailState(messages []ConversationMessage) (op.ThreadTailStatus, op.ThreadContinuationReason) {
	if len(messages) == 0 {
		return op.ThreadTailEmpty, op.ThreadContinuationNone
	}
	last := messages[len(messages)-1]
	switch last.Role {
	case RoleCanonicalAssistant:
		switch last.StopReason {
		case StopReasonStop, StopReasonLength:
			return op.ThreadTailComplete, op.ThreadContinuationNone
		case StopReasonToolUse:
			return op.ThreadTailNeedsContinuation, op.ThreadContinuationAssistantTool
		case StopReasonError:
			return op.ThreadTailComplete, op.ThreadContinuationAssistantError
		case StopReasonAborted:
			return op.ThreadTailComplete, op.ThreadContinuationAssistantAbort
		case "":
			if len(canonicalToolCallsFromMessage(last)) == 0 {
				return op.ThreadTailComplete, op.ThreadContinuationNone
			}
			return op.ThreadTailNeedsContinuation, op.ThreadContinuationAssistantTool
		default:
			return op.ThreadTailNeedsContinuation, op.ThreadContinuationAssistantTool
		}
	case RoleCanonicalUser:
		return op.ThreadTailNeedsContinuation, op.ThreadContinuationUserTail
	case RoleCanonicalTool:
		return op.ThreadTailNeedsContinuation, op.ThreadContinuationToolResultTail
	default:
		return op.ThreadTailComplete, op.ThreadContinuationNone
	}
}

// pruneSkippedOpTurns removes terminal assistant error/abort messages from
// provider replay while keeping the user/tool context that led to the failure.
// This preserves the failed turn's user intent for the next prompt without
// replaying the terminal failure text back into the provider.
func pruneSkippedOpTurns(messages []op.Message) []op.Message {
	out := make([]op.Message, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case op.RoleAssistant:
			if isReplaySkippedAssistant(msg) {
				continue
			}
			out = append(out, msg)
		default:
			out = append(out, msg)
		}
	}
	return out
}

// pruneSkippedCanonicalTurns is the canonical-history equivalent of
// pruneSkippedOpTurns: failed assistant terminal messages are omitted from
// replay, but their preceding user/tool context remains replayable.
func pruneSkippedCanonicalTurns(messages []ConversationMessage) []ConversationMessage {
	out := make([]ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case RoleCanonicalAssistant:
			if isReplaySkippedCanonicalAssistant(msg) {
				continue
			}
			out = append(out, msg)
		default:
			out = append(out, msg)
		}
	}
	return out
}

func isReplaySkippedAssistant(msg op.Message) bool {
	return msg.Role == op.RoleAssistant &&
		(msg.StopReason == op.StopReasonError || msg.StopReason == op.StopReasonAborted)
}

func isReplaySkippedCanonicalAssistant(msg ConversationMessage) bool {
	return msg.Role == RoleCanonicalAssistant &&
		(msg.StopReason == StopReasonError || msg.StopReason == StopReasonAborted)
}

func isCompleteAssistantTail(msg op.Message) bool {
	if msg.Role != op.RoleAssistant {
		return false
	}
	switch msg.StopReason {
	case op.StopReasonStop, op.StopReasonLength:
		return true
	case op.StopReasonToolUse:
		return false
	case op.StopReasonError, op.StopReasonAborted:
		return true
	case "":
		return len(msg.ToolCalls) == 0
	default:
		return false
	}
}

func isCompleteCanonicalAssistantTail(msg ConversationMessage) bool {
	if msg.Role != RoleCanonicalAssistant {
		return false
	}
	switch msg.StopReason {
	case StopReasonStop, StopReasonLength:
		return true
	case StopReasonToolUse, StopReasonError:
		return false
	case StopReasonAborted:
		return true
	case "":
		return len(canonicalToolCallsFromMessage(msg)) == 0
	default:
		return false
	}
}

func canonicalToolCallsFromMessage(msg ConversationMessage) []CanonicalToolCall {
	if len(msg.Content) == 0 {
		return nil
	}
	toolCalls := make([]CanonicalToolCall, 0, len(msg.Content))
	for _, block := range msg.Content {
		if block.Type != BlockToolCall || block.ToolCall == nil {
			continue
		}
		toolCalls = append(toolCalls, *block.ToolCall)
	}
	return toolCalls
}
