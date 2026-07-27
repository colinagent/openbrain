package ai

import (
	"errors"
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
	raw, err := MarshalCanonicalStreamEventJSON(event)
	if err != nil {
		t.Fatalf("MarshalCanonicalStreamEventJSON(): %v", err)
	}
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
	raw, err := MarshalCanonicalStreamEventJSON(event)
	if err != nil {
		t.Fatalf("MarshalCanonicalStreamEventJSON(): %v", err)
	}
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

func TestCanonicalStreamEventJSON_ToolCallDeltaUsesCompactWireShape(t *testing.T) {
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

	raw, err := MarshalCanonicalStreamEventJSON(event)
	if err != nil {
		t.Fatalf("MarshalCanonicalStreamEventJSON(): %v", err)
	}
	if strings.Contains(string(raw), `"partial"`) || strings.Contains(string(raw), `"block"`) {
		t.Fatalf("wire event = %s, want delta-only payload", raw)
	}
	parsed, err := ParseCanonicalStreamEventJSON(raw)
	if err != nil {
		t.Fatalf("ParseCanonicalStreamEventJSON(): %v", err)
	}
	if parsed.Partial != nil {
		t.Fatalf("parsed.Partial = %#v, want nil compact wire partial", parsed.Partial)
	}
	if parsed.Block != nil {
		t.Fatalf("parsed.Block = %#v, want nil delta block", parsed.Block)
	}
	if parsed.Delta != event.Delta || parsed.ContentIndex != event.ContentIndex {
		t.Fatalf("parsed delta = %#v, want delta/index preserved", parsed)
	}
}

func TestCanonicalStreamEventJSON_RejectsSerializationFailure(t *testing.T) {
	_, err := MarshalCanonicalStreamEventJSON(ProviderEvent{
		Type: EventCanonicalDone,
		Response: &ProviderResponse{Message: ConversationMessage{
			Role: RoleCanonicalAssistant,
			Raw:  []byte(`{"broken":`),
		}},
	})
	if err == nil {
		t.Fatal("MarshalCanonicalStreamEventJSON() error = nil, want invalid raw JSON failure")
	}
}

func TestCanonicalStreamEventJSON_RejectsMissingTerminal(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("   "), []byte("[DONE]")} {
		if _, err := ParseCanonicalStreamEventJSON(raw); err == nil {
			t.Fatalf("ParseCanonicalStreamEventJSON(%q) error = nil", raw)
		}
	}
}

func TestCanonicalStreamEventJSON_RejectsTerminalPayloadOnNonterminalEvent(t *testing.T) {
	response := &ProviderResponse{Message: ConversationMessage{
		Role:    RoleCanonicalAssistant,
		Content: []ContentBlock{{Type: BlockText, Text: "hello"}},
	}}
	if _, err := MarshalCanonicalStreamEventJSON(ProviderEvent{Type: EventCanonicalStart, Response: response}); err == nil {
		t.Fatal("MarshalCanonicalStreamEventJSON() accepted response on start event")
	}
	if _, err := MarshalCanonicalStreamEventJSON(ProviderEvent{Type: EventCanonicalTextDelta, Error: errors.New("boom")}); err == nil {
		t.Fatal("MarshalCanonicalStreamEventJSON() accepted error payload on delta event")
	}
	if _, err := ParseCanonicalStreamEventJSON([]byte(`{"type":"start","error":{"message":"boom"}}`)); err == nil {
		t.Fatal("ParseCanonicalStreamEventJSON() accepted error payload on start event")
	}
}

func TestCanonicalWebsocketAckJSON_RoundTrip(t *testing.T) {
	raw, err := MarshalCanonicalWebsocketAckJSON(EventCanonicalDone, "request-1")
	if err != nil {
		t.Fatalf("MarshalCanonicalWebsocketAckJSON(): %v", err)
	}
	terminalType, requestID, err := DecodeCanonicalWebsocketAckJSON(raw)
	if err != nil {
		t.Fatalf("DecodeCanonicalWebsocketAckJSON(): %v", err)
	}
	if terminalType != EventCanonicalDone || requestID != "request-1" {
		t.Fatalf("ack = (%q, %q), want (done, request-1)", terminalType, requestID)
	}
}
