package op

import (
	"encoding/json"
	"testing"
)

func TestThreadCanonicalMessageEntryJSONRoundTrip(t *testing.T) {
	entry := ThreadCanonicalMessageEntry{
		ThreadEntryBase: ThreadEntryBase{
			Type:      ThreadEntryTypeCanonicalMessage,
			ID:        "entry-1",
			Timestamp: "2026-06-24T00:00:00Z",
		},
		Message: ConversationMessage{
			Role: RoleCanonicalAssistant,
			Content: []ContentBlock{
				{Type: BlockThinking, Text: "checking"},
				{
					Type: BlockToolCall,
					ToolCall: &CanonicalToolCall{
						ID:           "call-1",
						Name:         "read",
						Arguments:    map[string]any{"path": "/repo/main.go"},
						RawArguments: `{"path":"/repo/main.go"}`,
					},
				},
			},
			ProviderState: &ProviderState{
				Provider:   "openai",
				API:        "responses",
				Model:      "gpt-test",
				ResponseID: "resp-1",
			},
			Usage:      &MessageUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
			StopReason: StopReasonToolUse,
		},
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("json.Unmarshal wire: %v", err)
	}
	if wire["type"] != ThreadEntryTypeCanonicalMessage {
		t.Fatalf("wire type = %v, want %q", wire["type"], ThreadEntryTypeCanonicalMessage)
	}

	var decoded ThreadCanonicalMessageEntry
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal entry: %v", err)
	}
	if decoded.Message.Role != RoleCanonicalAssistant {
		t.Fatalf("role = %q, want %q", decoded.Message.Role, RoleCanonicalAssistant)
	}
	if decoded.Message.StopReason != StopReasonToolUse {
		t.Fatalf("stopReason = %q, want %q", decoded.Message.StopReason, StopReasonToolUse)
	}
	if decoded.Message.Usage == nil || decoded.Message.Usage.TotalTokens != 15 {
		t.Fatalf("usage = %+v, want total tokens 15", decoded.Message.Usage)
	}
	if decoded.Message.ProviderState == nil || decoded.Message.ProviderState.ResponseID != "resp-1" {
		t.Fatalf("providerState = %+v, want response id resp-1", decoded.Message.ProviderState)
	}
	if len(decoded.Message.Content) != 2 {
		t.Fatalf("content len = %d, want 2", len(decoded.Message.Content))
	}
	toolCall := decoded.Message.Content[1].ToolCall
	if toolCall == nil || toolCall.Name != "read" || toolCall.Arguments["path"] != "/repo/main.go" {
		t.Fatalf("toolCall = %+v, want read path argument", toolCall)
	}
}
