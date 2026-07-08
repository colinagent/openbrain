package ai

import (
	"fmt"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestOpMessageFromCanonical_ConvertsCanonicalToolCalls(t *testing.T) {
	msg, err := OpMessageFromCanonical(ConversationMessage{
		Role: RoleCanonicalAssistant,
		Content: []ContentBlock{
			{Type: BlockText, Text: "partial"},
			{
				Type: BlockToolCall,
				ToolCall: &CanonicalToolCall{
					ID:           "call-2",
					Name:         "read",
					RawArguments: `{"path":"/tmp/in.md"}`,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("OpMessageFromCanonical(): %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].ID != "call-2" {
		t.Fatalf("tool call id = %q, want call-2", msg.ToolCalls[0].ID)
	}
}

func TestProviderResponseFromOpMessage(t *testing.T) {
	resp := ProviderResponseFromOpMessage(op.Message{
		Role:             op.RoleAssistant,
		Content:          "done",
		ReasoningContent: "think",
		ToolCalls: []op.MessageToolCall{{
			ID:        "call-1",
			Name:      "bash",
			Arguments: map[string]any{"command": "pwd"},
			Type:      "function",
		}},
	}, Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}, StopReasonToolUse)
	if resp.Message.Role != RoleCanonicalAssistant {
		t.Fatalf("role = %q, want assistant", resp.Message.Role)
	}
	if len(resp.Message.Content) != 3 {
		t.Fatalf("content blocks = %d, want 3", len(resp.Message.Content))
	}
	if resp.Message.Content[0].Type != BlockThinking || resp.Message.Content[1].Type != BlockText || resp.Message.Content[2].Type != BlockToolCall {
		t.Fatalf("unexpected blocks: %#v", resp.Message.Content)
	}
}

func TestProviderResponseFromResponsesResult_PreservesRawItems(t *testing.T) {
	resp := ProviderResponseFromResponsesResult(&ResponsesResult{
		ID: "resp_123",
		Output: []ResponseItem{
			ParseResponseItemRaw([]byte(`{"type":"reasoning","id":"rs_1","encrypted_content":"enc","summary":[{"type":"summary_text","text":"think"}]}`)),
			ParseResponseItemRaw([]byte(`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"done"}]}`)),
			ParseResponseItemRaw([]byte(`{"type":"function_call","call_id":"call_1","name":"bash","arguments":"{\"command\":\"pwd\"}"}`)),
		},
		Usage:      Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
		StopReason: StopReasonToolUse,
	})
	if got := resp.Message.ProviderState.ResponseID; got != "resp_123" {
		t.Fatalf("response id = %q, want resp_123", got)
	}
	if len(resp.Message.Content) != 3 {
		t.Fatalf("content blocks = %d, want 3", len(resp.Message.Content))
	}
	if resp.Message.Content[0].Type != BlockThinking || resp.Message.Content[0].EncryptedContent == "" {
		t.Fatalf("unexpected reasoning block: %#v", resp.Message.Content[0])
	}
}

func TestCanonicalMessagesFromReplayableOp_TruncatesOversizedHistoricalToolOutput(t *testing.T) {
	var builder strings.Builder
	for i := 1; i <= 2300; i++ {
		if i > 1 {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("line-%04d-%s", i, strings.Repeat("x", 40)))
	}
	builder.WriteString("\nreplay-tail-sentinel")

	messages := CanonicalMessagesFromReplayableOp([]op.Message{
		op.NewToolResultMessage("bash", "call-1", builder.String()),
	})
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if len(messages[0].Content) != 1 || messages[0].Content[0].ToolResult == nil {
		t.Fatalf("unexpected canonical tool message: %#v", messages[0])
	}
	output := messages[0].Content[0].ToolResult.OutputText
	if !strings.Contains(output, "replay-tail-sentinel") {
		t.Fatalf("expected truncated replay output to keep tail sentinel, got %q", output)
	}
	if strings.Contains(output, "line-0001-") {
		t.Fatalf("expected truncated replay output to drop old head lines, got %q", output)
	}
	if !strings.Contains(output, "Historical tool output truncated for replay") {
		t.Fatalf("expected replay truncation notice, got %q", output)
	}
}
