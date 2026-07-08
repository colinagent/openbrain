package op

import "testing"

func TestEstimateMessageTokens_Basic(t *testing.T) {
	tests := []struct {
		name    string
		msg     Message
		wantMin int64
	}{
		{"empty message", Message{}, 0},
		{"text only", NewUserMessage("hello world"), 2},
		{"assistant with reasoning", NewAssistantMessageWithReasoning("ok", "long thinking content here"), 7},
		{"reasoning only", Message{Role: RoleAssistant, ReasoningContent: "thinking..."}, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateMessageTokens(tc.msg)
			if got < tc.wantMin {
				t.Errorf("EstimateMessageTokens() = %d, want >= %d", got, tc.wantMin)
			}
		})
	}
}

func TestEstimateMessageTokens_IncludesReasoning(t *testing.T) {
	reasoning := "This is a long reasoning block that should be counted in the token estimate"
	withReasoning := Message{Role: RoleAssistant, Content: "ok", ReasoningContent: reasoning}
	withoutReasoning := Message{Role: RoleAssistant, Content: "ok"}

	tokensWith := EstimateMessageTokens(withReasoning)
	tokensWithout := EstimateMessageTokens(withoutReasoning)

	if tokensWith <= tokensWithout {
		t.Errorf("with reasoning (%d) should be > without (%d)", tokensWith, tokensWithout)
	}

	expectedDelta := int64(len(reasoning)+3) / 4
	actualDelta := tokensWith - tokensWithout
	if actualDelta < expectedDelta-1 || actualDelta > expectedDelta+1 {
		t.Errorf("reasoning delta = %d, expected ~%d", actualDelta, expectedDelta)
	}
}

func TestEstimateMessageTokens_ToolCalls(t *testing.T) {
	msg := NewAssistantToolCallsWithReasoning("", "thinking", []MessageToolCall{
		{ID: "call_1", Name: "read_file", Arguments: map[string]any{"path": "/foo/bar.ts"}, Type: "function"},
	})
	tokens := EstimateMessageTokens(msg)
	if tokens <= 0 {
		t.Errorf("tool call message should have positive tokens, got %d", tokens)
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	msgs := []Message{
		NewUserMessage("hello"),
		NewAssistantMessageWithReasoning("world", "let me think about this"),
		NewToolResultMessage("read_file", "call_1", "file content here"),
	}
	total := EstimateMessagesTokens(msgs)
	var sum int64
	for _, m := range msgs {
		sum += EstimateMessageTokens(m)
	}
	if total != sum {
		t.Errorf("EstimateMessagesTokens() = %d, want sum %d", total, sum)
	}
}
