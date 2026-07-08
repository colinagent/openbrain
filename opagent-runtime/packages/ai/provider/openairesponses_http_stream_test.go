package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

func newResponsesHTTPTestProvider(t *testing.T, handler http.HandlerFunc) *ResponsesProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	provider, err := NewResponsesProviderWithTransport(&op.ModelConfig{
		ID:       "gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "openai",
		API:      "openai-responses",
		APIKey:   "test-key",
		BaseURL:  server.URL + "/v1",
	}, server.Client(), map[string]string{"X-Test-Header": "ok"})
	if err != nil {
		t.Fatalf("NewResponsesProviderWithTransport(): %v", err)
	}
	return provider
}

func newResponsesHTTPCodexLikeURLTestProvider(t *testing.T, handler http.HandlerFunc) *ResponsesProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	provider, err := NewResponsesProviderWithTransport(&op.ModelConfig{
		ID:       "gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "openai",
		API:      "openai-responses",
		APIKey:   "test-key",
		BaseURL:  server.URL + "/api/codex/backend-api/codex",
	}, server.Client(), map[string]string{"X-Test-Header": "ok"})
	if err != nil {
		t.Fatalf("NewResponsesProviderWithTransport(): %v", err)
	}
	return provider
}

func TestResponsesProviderStreamResponses_UsesSDKStream(t *testing.T) {
	provider := newResponsesHTTPTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}
		if got := strings.TrimSpace(r.Header.Get("Authorization")); got != "Bearer test-key" {
			t.Fatalf("authorization = %q, want Bearer test-key", got)
		}
		if got := strings.TrimSpace(r.Header.Get("X-Test-Header")); got != "ok" {
			t.Fatalf("X-Test-Header = %q, want ok", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", `{"type":"response.created","response":{"id":"resp_1","model":"gpt-5.4","status":"in_progress","output":[]}}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"type":"response.output_item.added","item":{"type":"message","id":"msg_1","role":"assistant","content":[],"status":"in_progress"}}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"type":"response.output_text.delta","delta":"hi"}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"type":"response.output_item.done","item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hi","annotations":[]}]}}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hi","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)
	})

	stream, err := provider.StreamResponses(context.Background(), &ai.ResponsesRequest{
		Model: "gpt-5.4",
		Input: []ai.ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ai.ResponseContentPart{{Type: "input_text", Text: "hello"}},
		}},
	})
	if err != nil {
		t.Fatalf("StreamResponses(): %v", err)
	}
	var types []string
	var delta string
	var final *ai.ResponsesResult
	for stream.Next() {
		event := stream.Event()
		types = append(types, event.Type)
		if event.Type == "response.output_text.delta" {
			delta += event.Delta
		}
		if event.Type == "response.completed" {
			final = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if delta != "hi" {
		t.Fatalf("delta = %q, want hi", delta)
	}
	if final == nil || final.ID != "resp_1" || final.Usage.TotalTokens != 2 {
		t.Fatalf("final response = %#v, want completed response with usage", final)
	}
	if got := strings.Join(types, ","); !strings.Contains(got, "response.created") || !strings.Contains(got, "response.completed") {
		t.Fatalf("event types = %s, want created and completed", got)
	}
}

func TestResponsesProviderStreamResponses_CodexLikeBaseURLStaysGenericOpenAIResponses(t *testing.T) {
	provider := newResponsesHTTPCodexLikeURLTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/codex/backend-api/codex/responses" {
			t.Fatalf("path = %s, want /api/codex/backend-api/codex/responses", r.URL.Path)
		}
		if got := strings.TrimSpace(r.Header.Get("Authorization")); got != "Bearer test-key" {
			t.Fatalf("authorization = %q, want Bearer test-key", got)
		}
		if got := strings.TrimSpace(r.Header.Get("X-Test-Header")); got != "ok" {
			t.Fatalf("X-Test-Header = %q, want ok", got)
		}
		if got := strings.TrimSpace(r.Header.Get("chatgpt-account-id")); got != "" {
			t.Fatalf("chatgpt-account-id = %q, want empty", got)
		}
		if got := strings.TrimSpace(r.Header.Get("originator")); got != "" {
			t.Fatalf("originator = %q, want empty", got)
		}
		if got := strings.TrimSpace(r.Header.Get("OpenAI-Beta")); got != "" {
			t.Fatalf("OpenAI-Beta = %q, want empty", got)
		}
		if got := strings.TrimSpace(r.Header.Get("session_id")); got != "" {
			t.Fatalf("session_id = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", `{"type":"response.completed","response":{"id":"resp_codex_like","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)
	})

	stream, err := provider.StreamResponses(context.Background(), &ai.ResponsesRequest{
		Model:          "gpt-5.4",
		PromptCacheKey: "sess_codex_123",
	})
	if err != nil {
		t.Fatalf("StreamResponses(): %v", err)
	}
	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
}

func TestResponsesProviderStreamResponses_PrematureEOFIsRetryable(t *testing.T) {
	provider := newResponsesHTTPTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", `{"type":"response.created","response":{"id":"resp_partial","model":"gpt-5.4","status":"in_progress","output":[]}}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"type":"response.output_text.delta","delta":"partial"}`)
	})

	stream, err := provider.StreamResponses(context.Background(), &ai.ResponsesRequest{Model: "gpt-5.4"})
	if err != nil {
		t.Fatalf("StreamResponses(): %v", err)
	}
	var sawDelta bool
	for stream.Next() {
		event := stream.Event()
		if event.Type == "response.output_text.delta" && event.Delta == "partial" {
			sawDelta = true
		}
	}
	if !sawDelta {
		t.Fatal("expected partial delta before EOF")
	}
	retryErr, ok := ai.AsRetryError(stream.Err())
	if !ok || retryErr == nil {
		t.Fatalf("stream.Err() = %v, want retry error", stream.Err())
	}
	if !retryErr.Retryable {
		t.Fatalf("retryErr.Retryable = false, want true: %#v", retryErr)
	}
	if !strings.Contains(retryErr.Error(), "before response.completed") {
		t.Fatalf("retryErr = %q, want premature EOF detail", retryErr.Error())
	}
}

func TestResponsesProviderStreamResponses_ErrorEventBecomesResponseFailed(t *testing.T) {
	provider := newResponsesHTTPTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", `{"type":"error","code":"server_error","message":"boom"}`)
	})

	stream, err := provider.StreamResponses(context.Background(), &ai.ResponsesRequest{Model: "gpt-5.4"})
	if err != nil {
		t.Fatalf("StreamResponses(): %v", err)
	}
	var sawFailed bool
	for stream.Next() {
		event := stream.Event()
		if event.Type == "response.failed" {
			sawFailed = true
		}
	}
	if !sawFailed {
		t.Fatal("expected response.failed event")
	}
	retryErr, ok := ai.AsRetryError(stream.Err())
	if !ok || retryErr == nil {
		t.Fatalf("stream.Err() = %v, want retry error", stream.Err())
	}
	if !strings.Contains(retryErr.Error(), "boom") {
		t.Fatalf("retryErr = %q, want boom", retryErr.Error())
	}
}

func TestResponsesProviderStreamResponses_HTTP503IsRetryable(t *testing.T) {
	provider := newResponsesHTTPTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":"Service temporarily unavailable","message":"Service failed: pool exhausted"}`)
	})

	stream, err := provider.StreamResponses(context.Background(), &ai.ResponsesRequest{Model: "gpt-5.4"})
	if err != nil {
		t.Fatalf("StreamResponses(): %v", err)
	}
	for stream.Next() {
	}
	retryErr, ok := ai.AsRetryError(stream.Err())
	if !ok || retryErr == nil {
		t.Fatalf("stream.Err() = %v, want retry error", stream.Err())
	}
	if retryErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", retryErr.StatusCode, http.StatusServiceUnavailable)
	}
	if !strings.Contains(retryErr.Error(), "pool exhausted") {
		t.Fatalf("retryErr = %q, want pool exhausted", retryErr.Error())
	}
}

type scriptedResponsesCanonicalStreamProvider struct {
	events []ai.ResponsesStreamEvent
}

func (p scriptedResponsesCanonicalStreamProvider) CompleteResponses(context.Context, *ai.ResponsesRequest) (*ai.ResponsesResult, error) {
	return nil, nil
}

func (p scriptedResponsesCanonicalStreamProvider) StreamResponses(context.Context, *ai.ResponsesRequest) (*ai.ResponsesEventStream, error) {
	out := ai.NewResponsesEventStream(len(p.events))
	go func() {
		for _, event := range p.events {
			_ = out.Emit(event)
		}
		out.Close()
	}()
	return out, nil
}

func TestStreamCanonicalFromResponsesProvider_FallsBackToStreamedToolCallWhenCompletedOutputIsEmpty(t *testing.T) {
	added := ai.ParseResponseItemRaw(json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read","arguments":"","status":"in_progress"}`))
	done := ai.ParseResponseItemRaw(json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read","arguments":"{\"path\":\"AGENTS.md\"}","status":"completed"}`))
	stream, err := streamCanonicalFromResponsesProvider(
		context.Background(),
		&ai.ProviderRequest{Config: ai.GenerationConfig{Model: "gpt-5.4"}},
		&op.ModelConfig{Provider: "opagent-ai-gateway", API: "openai-responses"},
		scriptedResponsesCanonicalStreamProvider{events: []ai.ResponsesStreamEvent{
			{Type: "response.created", Response: &ai.ResponsesResult{ID: "resp_1", Model: "gpt-5.4", Status: "in_progress"}},
			{Type: "response.output_item.added", Item: &added},
			{Type: "response.output_item.done", Item: &done},
			{Type: "response.completed", Response: &ai.ResponsesResult{
				ID:     "resp_1",
				Model:  "gpt-5.4",
				Status: "completed",
				Usage:  ai.Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12},
			}},
		}},
	)
	if err != nil {
		t.Fatalf("streamCanonicalFromResponsesProvider(): %v", err)
	}
	var final *ai.ProviderResponse
	for stream.Next() {
		event := stream.Event()
		if event.Type == ai.EventCanonicalDone {
			final = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if final == nil {
		t.Fatal("missing canonical done response")
	}
	msg, err := ai.OpMessageFromCanonical(final.Message)
	if err != nil {
		t.Fatalf("OpMessageFromCanonical(): %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Name != "read" {
		t.Fatalf("tool calls = %#v, want read tool call", msg.ToolCalls)
	}
	if got := msg.ToolCalls[0].Arguments["path"]; got != "AGENTS.md" {
		t.Fatalf("tool call path = %#v, want AGENTS.md", got)
	}
	if final.StopReason != ai.StopReasonToolUse {
		t.Fatalf("stop reason = %q, want tool_use", final.StopReason)
	}
	if final.Usage.TotalTokens != 12 {
		t.Fatalf("usage total = %d, want 12", final.Usage.TotalTokens)
	}
}

func TestStreamCanonicalFromResponsesProvider_FinalizesPartialWhenCompletedOutputOmitsText(t *testing.T) {
	textAdded := ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}`))
	textDone := ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"streamed text","annotations":[]}]}`))
	toolAdded := ai.ParseResponseItemRaw(json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read","arguments":"","status":"in_progress"}`))
	toolDone := ai.ParseResponseItemRaw(json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read","arguments":"{\"path\":\"AGENTS.md\"}","status":"completed"}`))

	stream, err := streamCanonicalFromResponsesProvider(
		context.Background(),
		&ai.ProviderRequest{Config: ai.GenerationConfig{Model: "gpt-5.4"}},
		&op.ModelConfig{Provider: "opagent-ai-gateway", API: "openai-responses"},
		scriptedResponsesCanonicalStreamProvider{events: []ai.ResponsesStreamEvent{
			{Type: "response.created", Response: &ai.ResponsesResult{ID: "resp_1", Model: "gpt-5.4", Status: "in_progress"}},
			{Type: "response.output_item.added", Item: &textAdded},
			{Type: "response.output_item.done", Item: &textDone},
			{Type: "response.output_item.added", Item: &toolAdded},
			{Type: "response.output_item.done", Item: &toolDone},
			{Type: "response.completed", Response: &ai.ResponsesResult{
				ID:         "resp_1",
				Model:      "gpt-5.4",
				Status:     "completed",
				Output:     []ai.ResponseItem{toolDone},
				Usage:      ai.Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12},
				StopReason: ai.StopReasonToolUse,
			}},
		}},
	)
	if err != nil {
		t.Fatalf("streamCanonicalFromResponsesProvider(): %v", err)
	}
	var final *ai.ProviderResponse
	for stream.Next() {
		event := stream.Event()
		if event.Type == ai.EventCanonicalDone {
			final = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if final == nil {
		t.Fatal("missing canonical done response")
	}
	msg, err := ai.OpMessageFromCanonical(final.Message)
	if err != nil {
		t.Fatalf("OpMessageFromCanonical(): %v", err)
	}
	if msg.Content != "streamed text" {
		t.Fatalf("assistant content = %q, want streamed text", msg.Content)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Name != "read" {
		t.Fatalf("tool calls = %#v, want read tool call", msg.ToolCalls)
	}
	if len(final.Message.Content) != 2 || final.Message.Content[0].Type != ai.BlockText || final.Message.Content[1].Type != ai.BlockToolCall {
		t.Fatalf("final content = %#v, want streamed text before tool call", final.Message.Content)
	}
}
