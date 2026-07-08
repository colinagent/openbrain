package ai

import (
	"strings"
	"testing"
)

func TestCanonicalWebsocketCreateJSON_RoundTrip(t *testing.T) {
	req := &ProviderRequest{
		Context: ConversationContext{
			Messages: []ConversationMessage{{
				Role:    RoleCanonicalUser,
				Content: []ContentBlock{{Type: BlockText, Text: "hello"}},
			}},
		},
		Config:    GenerationConfig{Model: "claude-opus-4-6", PromptCacheKey: "thread-123"},
		RequestID: "req-123",
	}
	raw, err := MarshalCanonicalWebsocketCreateJSON("gateway:claude-opus-4-6", req)
	if err != nil {
		t.Fatalf("MarshalCanonicalWebsocketCreateJSON(): %v", err)
	}
	modelID, decoded, err := DecodeCanonicalWebsocketCreateJSON(raw)
	if err != nil {
		t.Fatalf("DecodeCanonicalWebsocketCreateJSON(): %v", err)
	}
	if modelID != "gateway:claude-opus-4-6" {
		t.Fatalf("modelID = %q, want gateway:claude-opus-4-6", modelID)
	}
	if got := decoded.Config.Model; got != "claude-opus-4-6" {
		t.Fatalf("decoded.Config.Model = %q, want claude-opus-4-6", got)
	}
	if got := decoded.RequestID; got != "req-123" {
		t.Fatalf("decoded.RequestID = %q, want req-123", got)
	}
}

func TestCanonicalStreamEventJSON_RoundTripError(t *testing.T) {
	event := ProviderEvent{
		Type: EventCanonicalError,
		Error: &RetryError{
			Retryable:    true,
			StatusCode:   503,
			Code:         "service_unavailable",
			Message:      "provider overloaded",
			RetryAfterMs: 1200,
		},
	}
	raw := RenderCanonicalStreamEventJSON(event)
	parsed, err := ParseCanonicalStreamEventJSON(raw)
	if err != nil {
		t.Fatalf("ParseCanonicalStreamEventJSON(): %v", err)
	}
	retryErr, ok := AsRetryError(parsed.Error)
	if !ok {
		t.Fatalf("parsed.Error = %T, want *RetryError", parsed.Error)
	}
	if !retryErr.Retryable || retryErr.StatusCode != 503 || retryErr.Code != "service_unavailable" || retryErr.RetryAfterMs != 1200 {
		t.Fatalf("retryErr = %#v", retryErr)
	}
	if !strings.Contains(retryErr.Error(), "provider overloaded") {
		t.Fatalf("retryErr.Error() = %q, want provider overloaded", retryErr.Error())
	}
}

func TestCanonicalStreamEventJSON_RoundTripDone(t *testing.T) {
	event := ProviderEvent{
		Type: EventCanonicalDone,
		Response: &ProviderResponse{
			Message: ConversationMessage{
				Role:    RoleCanonicalAssistant,
				Content: []ContentBlock{{Type: BlockText, Text: "hello"}},
			},
			Usage:      Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
			StopReason: StopReasonStop,
		},
	}
	raw := RenderCanonicalStreamEventJSON(event)
	parsed, err := ParseCanonicalStreamEventJSON(raw)
	if err != nil {
		t.Fatalf("ParseCanonicalStreamEventJSON(): %v", err)
	}
	if parsed.Type != EventCanonicalDone {
		t.Fatalf("parsed.Type = %q, want done", parsed.Type)
	}
	if parsed.Response == nil || !HasSemanticCanonicalResponse(parsed.Response) {
		t.Fatalf("parsed.Response = %#v, want semantic response", parsed.Response)
	}
	if got := parsed.Response.Message.Content[0].Text; got != "hello" {
		t.Fatalf("response text = %q, want hello", got)
	}
}

func TestCanonicalStreamEventJSON_RoundTripPartialToolCall(t *testing.T) {
	event := ProviderEvent{
		Type:         EventCanonicalToolCallDelta,
		ContentIndex: 1,
		Delta:        `{"path":"/tmp/out.md"`,
		Block: &StreamContentBlock{
			Type: BlockToolCall,
			ToolCall: &StreamToolCall{
				ID:           "call-1",
				Name:         "write",
				RawArguments: `{"path":"/tmp/out.md"`,
				Complete:     false,
			},
		},
		Partial: &StreamConversationMessage{
			Role: RoleCanonicalAssistant,
			Content: []StreamContentBlock{
				{Type: BlockText, Text: "draft"},
				{
					Type: BlockToolCall,
					ToolCall: &StreamToolCall{
						ID:           "call-1",
						Name:         "write",
						RawArguments: `{"path":"/tmp/out.md"`,
						Complete:     false,
					},
				},
			},
		},
	}

	raw := RenderCanonicalStreamEventJSON(event)
	parsed, err := ParseCanonicalStreamEventJSON(raw)
	if err != nil {
		t.Fatalf("ParseCanonicalStreamEventJSON(): %v", err)
	}
	if parsed.Partial == nil || len(parsed.Partial.Content) != 2 {
		t.Fatalf("parsed.Partial = %#v, want 2 content blocks", parsed.Partial)
	}
	if parsed.Block == nil || parsed.Block.ToolCall == nil || parsed.Block.ToolCall.Complete {
		t.Fatalf("parsed.Block = %#v, want incomplete stream tool call", parsed.Block)
	}
}
