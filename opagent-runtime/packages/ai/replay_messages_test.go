package ai

import (
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestNormalizeReplayableOpMessages_KeepsFailedUserTurnButDropsErroredAssistant(t *testing.T) {
	result := NormalizeReplayableOpMessages([]op.Message{
		op.NewUserMessage("hello"),
		{
			Role:       op.RoleAssistant,
			Content:    "partial",
			StopReason: op.StopReasonError,
		},
	})

	if result.ContinuationRequired {
		// expected
	} else {
		t.Fatal("ContinuationRequired = false, want true")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(result.Messages))
	}
	if got := result.Messages[0].Content; got != "hello" {
		t.Fatalf("Messages[0].Content = %q, want hello", got)
	}
}

func TestNormalizeReplayableCanonicalMessages_KeepsFailedUserTurnAfterCompletedHistory(t *testing.T) {
	result := NormalizeReplayableCanonicalMessages([]ConversationMessage{
		{
			Role: RoleCanonicalUser,
			Content: []ContentBlock{{
				Type: BlockText,
				Text: "kept user",
			}},
		},
		{
			Role: RoleCanonicalAssistant,
			Content: []ContentBlock{{
				Type: BlockText,
				Text: "kept assistant",
			}},
			StopReason: StopReasonStop,
		},
		{
			Role: RoleCanonicalUser,
			Content: []ContentBlock{{
				Type: BlockText,
				Text: "failed user",
			}},
		},
		{
			Role: RoleCanonicalAssistant,
			Content: []ContentBlock{{
				Type: BlockText,
				Text: "upstream 503",
			}},
			StopReason: StopReasonError,
		},
	})

	if result.ContinuationRequired {
		// expected
	} else {
		t.Fatal("ContinuationRequired = false, want true")
	}
	if len(result.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3", len(result.Messages))
	}
	if got := result.Messages[0].Content[0].Text; got != "kept user" {
		t.Fatalf("Messages[0] text = %q, want kept user", got)
	}
	if got := result.Messages[1].Content[0].Text; got != "kept assistant" {
		t.Fatalf("Messages[1] text = %q, want kept assistant", got)
	}
	if got := result.Messages[2].Content[0].Text; got != "failed user" {
		t.Fatalf("Messages[2] text = %q, want failed user", got)
	}
}

func TestNormalizeReplayableCanonicalMessages_KeepsErroredToolTurnContext(t *testing.T) {
	result := NormalizeReplayableCanonicalMessages([]ConversationMessage{
		{
			Role: RoleCanonicalUser,
			Content: []ContentBlock{{
				Type: BlockText,
				Text: "use tool",
			}},
		},
		{
			Role: RoleCanonicalAssistant,
			Content: []ContentBlock{{
				Type: BlockToolCall,
				ToolCall: &CanonicalToolCall{
					ID:   "call-1",
					Name: "read",
				},
			}},
			StopReason: StopReasonToolUse,
		},
		{
			Role: RoleCanonicalTool,
			Content: []ContentBlock{{
				Type: BlockToolResult,
				ToolResult: &CanonicalToolResult{
					ToolCallID: "call-1",
					ToolName:   "read",
					OutputText: "result",
				},
			}},
		},
		{
			Role: RoleCanonicalAssistant,
			Content: []ContentBlock{{
				Type: BlockText,
				Text: "upstream 503",
			}},
			StopReason: StopReasonError,
		},
	})

	if result.ContinuationRequired {
		// expected
	} else {
		t.Fatal("ContinuationRequired = false, want true")
	}
	if len(result.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3", len(result.Messages))
	}
	if result.Messages[0].Role != RoleCanonicalUser {
		t.Fatalf("Messages[0].Role = %q, want %q", result.Messages[0].Role, RoleCanonicalUser)
	}
	if result.Messages[1].Role != RoleCanonicalAssistant {
		t.Fatalf("Messages[1].Role = %q, want %q", result.Messages[1].Role, RoleCanonicalAssistant)
	}
	if result.Messages[2].Role != RoleCanonicalTool {
		t.Fatalf("Messages[2].Role = %q, want %q", result.Messages[2].Role, RoleCanonicalTool)
	}
}

func TestNormalizeReplayableOpMessages_InsertsSyntheticToolResultForOrphanedTail(t *testing.T) {
	result := NormalizeReplayableOpMessages([]op.Message{
		op.NewUserMessage("hello"),
		{
			Role: op.RoleAssistant,
			ToolCalls: []op.MessageToolCall{{
				ID:        "call-1",
				Name:      "read",
				Arguments: map[string]any{"path": "a"},
			}},
			StopReason: op.StopReasonToolUse,
		},
	})

	if !result.ContinuationRequired {
		t.Fatal("ContinuationRequired = false, want true")
	}
	if len(result.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3", len(result.Messages))
	}
	last := result.Messages[2]
	if last.Role != op.RoleTool {
		t.Fatalf("last.Role = %q, want %q", last.Role, op.RoleTool)
	}
	if last.ToolCallID != "call-1" {
		t.Fatalf("last.ToolCallID = %q, want call-1", last.ToolCallID)
	}
	if last.Content != "No result provided" {
		t.Fatalf("last.Content = %q, want %q", last.Content, "No result provided")
	}
}

func TestNormalizeReplayableOpMessages_KeepsCompletedAssistantTailAsComplete(t *testing.T) {
	result := NormalizeReplayableOpMessages([]op.Message{
		op.NewUserMessage("hello"),
		{
			Role:       op.RoleAssistant,
			Content:    "done",
			StopReason: op.StopReasonStop,
		},
	})

	if result.ContinuationRequired {
		t.Fatal("ContinuationRequired = true, want false")
	}
	if len(result.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(result.Messages))
	}
}

func TestCanonicalMessagesTailState_TreatsAssistantErrorAsComplete(t *testing.T) {
	status, reason := CanonicalMessagesTailState([]ConversationMessage{
		{
			Role: RoleCanonicalUser,
			Content: []ContentBlock{{
				Type: BlockText,
				Text: "hello",
			}},
		},
		{
			Role: RoleCanonicalAssistant,
			Content: []ContentBlock{{
				Type: BlockText,
				Text: "upstream 503",
			}},
			StopReason: StopReasonError,
		},
	})

	if status != op.ThreadTailComplete {
		t.Fatalf("status = %q, want %q", status, op.ThreadTailComplete)
	}
	if reason != op.ThreadContinuationAssistantError {
		t.Fatalf("reason = %q, want %q", reason, op.ThreadContinuationAssistantError)
	}
}

func TestCanonicalMessagesTailState_TreatsAssistantAbortAsComplete(t *testing.T) {
	status, reason := CanonicalMessagesTailState([]ConversationMessage{
		{
			Role: RoleCanonicalUser,
			Content: []ContentBlock{{
				Type: BlockText,
				Text: "hello",
			}},
		},
		{
			Role: RoleCanonicalAssistant,
			Content: []ContentBlock{{
				Type: BlockText,
				Text: "interrupted",
			}},
			StopReason: StopReasonAborted,
		},
	})

	if status != op.ThreadTailComplete {
		t.Fatalf("status = %q, want %q", status, op.ThreadTailComplete)
	}
	if reason != op.ThreadContinuationAssistantAbort {
		t.Fatalf("reason = %q, want %q", reason, op.ThreadContinuationAssistantAbort)
	}
}

func TestCanonicalMessagesTailState_TreatsAssistantWithoutToolCallsAsComplete(t *testing.T) {
	status, reason := CanonicalMessagesTailState([]ConversationMessage{
		{
			Role:    RoleCanonicalAssistant,
			Content: []ContentBlock{},
		},
	})

	if status != op.ThreadTailComplete {
		t.Fatalf("status = %q, want %q", status, op.ThreadTailComplete)
	}
	if reason != op.ThreadContinuationNone {
		t.Fatalf("reason = %q, want %q", reason, op.ThreadContinuationNone)
	}
}
