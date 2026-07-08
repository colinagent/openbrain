package provider

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type gatewayResponsesFallbackStub struct {
	streamCalls   int
	completeCalls int
	response      *ai.ResponsesResult
}

func (s *gatewayResponsesFallbackStub) CompleteResponses(context.Context, *ai.ResponsesRequest) (*ai.ResponsesResult, error) {
	s.completeCalls++
	if s.response != nil {
		return s.response, nil
	}
	return &ai.ResponsesResult{ID: "resp_http_fallback", Model: "gpt-5.4", Status: "completed"}, nil
}

func (s *gatewayResponsesFallbackStub) StreamResponses(context.Context, *ai.ResponsesRequest) (*ai.ResponsesEventStream, error) {
	s.streamCalls++
	out := ai.NewResponsesEventStream(2)
	response := s.response
	if response == nil {
		response = &ai.ResponsesResult{
			ID:     "resp_http_fallback",
			Model:  "gpt-5.4",
			Status: "completed",
			Output: []ai.ResponseItem{
				ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"fallback"}]}`)),
			},
		}
	}
	go func() {
		_ = out.Emit(ai.ResponsesStreamEvent{Type: "response.created", Response: &ai.ResponsesResult{ID: response.ID, Model: response.Model, Status: "in_progress"}})
		_ = out.Emit(ai.ResponsesStreamEvent{Type: "response.completed", Response: response})
		out.Close()
	}()
	return out, nil
}

func TestGatewayResponsesWSProvider_StreamResponsesUsesWebsocket(t *testing.T) {
	requestSeen := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()
		_, message, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read message: %v", err)
			return
		}
		requestSeen <- string(message)
		_ = conn.WriteJSON(map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":     "resp_ws_123",
				"model":  "gpt-5.4",
				"status": "in_progress",
				"output": []any{},
				"usage": map[string]any{
					"input_tokens":  0,
					"output_tokens": 0,
					"total_tokens":  0,
				},
			},
		})
		_ = conn.WriteJSON(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":     "resp_ws_123",
				"model":  "gpt-5.4",
				"status": "completed",
				"output": []any{
					map[string]any{
						"type":    "message",
						"role":    "assistant",
						"content": []any{map[string]any{"type": "output_text", "text": "hello"}},
					},
				},
				"usage": map[string]any{
					"input_tokens":  12,
					"output_tokens": 3,
					"total_tokens":  15,
				},
			},
		})
	}))
	defer server.Close()

	fallback := &gatewayResponsesFallbackStub{}
	prov := &GatewayResponsesWSProvider{
		httpFallback: fallback,
		cfg:          &op.ModelConfig{APIKey: "session-token"},
		wsURL:        "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/responses",
		headers:      gatewayResponsesWSHeaders(&op.ModelConfig{APIKey: "session-token"}, nil),
		dialer:       &websocket.Dialer{HandshakeTimeout: 5 * time.Second},
	}

	store := true
	stream, err := prov.StreamResponses(context.Background(), &ai.ResponsesRequest{
		Model:              "gpt-5.4",
		PreviousResponseID: "resp_prev_123",
		ToolChoice:         json.RawMessage(`"auto"`),
		Store:              &store,
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)),
		},
	})
	if err != nil {
		t.Fatalf("StreamResponses(): %v", err)
	}
	var completed *ai.ResponsesResult
	for stream.Next() {
		event := stream.Event()
		if event.Type == "response.completed" {
			completed = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err(): %v", err)
	}
	if completed == nil || completed.ID != "resp_ws_123" {
		t.Fatalf("completed response = %#v, want resp_ws_123", completed)
	}
	if fallback.streamCalls != 0 {
		t.Fatalf("fallback.streamCalls = %d, want 0", fallback.streamCalls)
	}
	payload := <-requestSeen
	if !strings.Contains(payload, `"type":"response.create"`) {
		t.Fatalf("websocket payload missing response.create: %s", payload)
	}
	if !strings.Contains(payload, `"tool_choice":"auto"`) {
		t.Fatalf("websocket payload missing tool_choice: %s", payload)
	}
	if !strings.Contains(payload, `"previous_response_id":"resp_prev_123"`) {
		t.Fatalf("websocket payload missing previous_response_id: %s", payload)
	}
	if !strings.Contains(payload, `"store":true`) {
		t.Fatalf("websocket payload missing store=true: %s", payload)
	}
}

func TestGatewayResponsesWSProvider_StreamResponsesFallsBackToHTTP(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	fallback := &gatewayResponsesFallbackStub{}
	prov := &GatewayResponsesWSProvider{
		httpFallback: fallback,
		cfg:          &op.ModelConfig{APIKey: "session-token"},
		wsURL:        "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/responses",
		headers:      gatewayResponsesWSHeaders(&op.ModelConfig{APIKey: "session-token"}, nil),
		dialer:       &websocket.Dialer{HandshakeTimeout: 5 * time.Second},
	}

	stream, err := prov.StreamResponses(context.Background(), &ai.ResponsesRequest{
		Model: "gpt-5.4",
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)),
		},
	})
	if err != nil {
		t.Fatalf("StreamResponses(): %v", err)
	}
	var completed *ai.ResponsesResult
	for stream.Next() {
		event := stream.Event()
		if event.Type == "response.completed" {
			completed = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err(): %v", err)
	}
	if completed == nil || completed.ID != "resp_http_fallback" {
		t.Fatalf("completed response = %#v, want resp_http_fallback", completed)
	}
	if fallback.streamCalls != 1 {
		t.Fatalf("fallback.streamCalls = %d, want 1", fallback.streamCalls)
	}
}

func TestGatewayResponsesWSProvider_DoesNotFallbackToHTTPOnResponseFailed(t *testing.T) {
	requestSeen := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			http.NotFound(w, r)
			return
		}
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("read message: %v", err)
			return
		}
		requestSeen <- struct{}{}
		_ = conn.WriteJSON(map[string]any{
			"type":  "response.failed",
			"error": "upstream 503 service unavailable",
		})
	}))
	defer server.Close()

	fallback := &gatewayResponsesFallbackStub{}
	prov := &GatewayResponsesWSProvider{
		httpFallback: fallback,
		cfg:          &op.ModelConfig{APIKey: "session-token"},
		wsURL:        "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/responses",
		headers:      gatewayResponsesWSHeaders(&op.ModelConfig{APIKey: "session-token"}, nil),
		dialer:       &websocket.Dialer{HandshakeTimeout: 5 * time.Second},
	}

	stream, err := prov.StreamResponses(context.Background(), &ai.ResponsesRequest{
		Model: "gpt-5.4",
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)),
		},
	})
	if err == nil {
		if stream != nil {
			stream.Close()
		}
		t.Fatal("expected response.failed error")
	}
	<-requestSeen
	if fallback.streamCalls != 0 {
		t.Fatalf("fallback.streamCalls = %d, want 0", fallback.streamCalls)
	}
	retryErr, ok := ai.AsRetryError(err)
	if !ok || retryErr == nil {
		t.Fatalf("expected retry error, got %T %v", err, err)
	}
	if !retryErr.Retryable {
		t.Fatalf("Retryable = false, want true: %#v", retryErr)
	}
	if !strings.Contains(retryErr.Error(), "503") {
		t.Fatalf("error = %q, want preserved response.failed detail", retryErr.Error())
	}
}

func TestParseGatewayResponsesWSEvent_PreservesFailureErrorString(t *testing.T) {
	event, err := parseGatewayResponsesWSEvent([]byte(`{"type":"response.failed","error":"response failed without details","request_id":"req_x"}`))
	if err != nil {
		t.Fatalf("parseGatewayResponsesWSEvent(): %v", err)
	}
	if event.Error == nil {
		t.Fatal("event.Error is nil")
	}
	retryErr, ok := ai.AsRetryError(event.Error)
	if !ok || retryErr == nil {
		t.Fatalf("expected retry error, got %T %v", event.Error, event.Error)
	}
	if !retryErr.Retryable {
		t.Fatalf("Retryable = false, want true: %#v", retryErr)
	}
	if !strings.Contains(retryErr.Error(), "response failed without details") {
		t.Fatalf("error = %q, want preserved detail", retryErr.Error())
	}
	if !strings.Contains(retryErr.Error(), "req_x") {
		t.Fatalf("error = %q, want request id", retryErr.Error())
	}
}

func TestNewGatewayResponsesWSDialer_ForcesHTTP11TLSNextProto(t *testing.T) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{NextProtos: []string{"h2", "http/1.1"}},
		},
	}
	dialer := newGatewayResponsesWSDialer(httpClient)
	if dialer.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if got := dialer.TLSClientConfig.NextProtos; len(got) != 1 || got[0] != "http/1.1" {
		t.Fatalf("NextProtos = %#v, want [http/1.1]", got)
	}
	if got := httpClient.Transport.(*http.Transport).TLSClientConfig.NextProtos; len(got) != 2 || got[0] != "h2" {
		t.Fatalf("source transport NextProtos mutated: %#v", got)
	}
}

func TestGatewayResponsesWSProvider_StreamResponsesNormalizesReplayToolOutput(t *testing.T) {
	requestSeen := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			http.NotFound(w, r)
			return
		}
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()
		_, message, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read message: %v", err)
			return
		}
		requestSeen <- string(message)
		_ = conn.WriteJSON(map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":     "resp_ws_norm",
				"model":  "gpt-5.4",
				"status": "in_progress",
				"output": []any{},
			},
		})
		_ = conn.WriteJSON(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":     "resp_ws_norm",
				"model":  "gpt-5.4",
				"status": "completed",
				"output": []any{},
			},
		})
	}))
	defer server.Close()

	prov := &GatewayResponsesWSProvider{
		httpFallback: &gatewayResponsesFallbackStub{},
		cfg:          &op.ModelConfig{APIKey: "session-token"},
		wsURL:        "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/responses",
		headers:      gatewayResponsesWSHeaders(&op.ModelConfig{APIKey: "session-token"}, nil),
		dialer:       &websocket.Dialer{HandshakeTimeout: 5 * time.Second},
	}

	lines := []string{"START_ONLY_LINE"}
	for i := 0; i < 1600; i++ {
		lines = append(lines, fmt.Sprintf("mid-line-%04d-%s", i, strings.Repeat("x", 16)))
	}
	lines = append(lines, "END_ONLY_LINE")
	hugeOutput := strings.Join(lines, "\n")

	stream, err := prov.StreamResponses(context.Background(), &ai.ResponsesRequest{
		Model: "gpt-5.4",
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)),
			{Type: "function_call_output", CallID: "call_123", OutputText: hugeOutput},
		},
	})
	if err != nil {
		t.Fatalf("StreamResponses(): %v", err)
	}
	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err(): %v", err)
	}
	payload := <-requestSeen
	if strings.Contains(payload, "START_ONLY_LINE") {
		t.Fatalf("payload unexpectedly kept head of oversized tool output: %s", payload)
	}
	if !strings.Contains(payload, "END_ONLY_LINE") {
		t.Fatalf("payload missing tail of oversized tool output: %s", payload)
	}
	if !strings.Contains(payload, "Historical tool output truncated for replay") {
		t.Fatalf("payload missing truncation notice: %s", payload)
	}
}

func TestGatewayResponsesWSProvider_ReusesPromptCacheKeySessionWebsocket(t *testing.T) {
	var upgrades atomic.Int32
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			http.NotFound(w, r)
			return
		}
		upgrades.Add(1)
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if !strings.Contains(string(message), `"prompt_cache_key":"th_test_123"`) {
				t.Errorf("websocket payload missing prompt_cache_key: %s", string(message))
				return
			}
			seq := requests.Add(1)
			respID := fmt.Sprintf("resp_ws_%d", seq)
			_ = conn.WriteJSON(map[string]any{
				"type": "response.created",
				"response": map[string]any{
					"id":     respID,
					"model":  "gpt-5.4",
					"status": "in_progress",
					"output": []any{},
				},
			})
			_ = conn.WriteJSON(map[string]any{
				"type": "response.completed",
				"response": map[string]any{
					"id":     respID,
					"model":  "gpt-5.4",
					"status": "completed",
					"output": []any{},
				},
			})
		}
	}))
	defer server.Close()

	fallback := &gatewayResponsesFallbackStub{}
	prov := &GatewayResponsesWSProvider{
		httpFallback: fallback,
		cfg:          &op.ModelConfig{APIKey: "session-token"},
		wsURL:        "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/responses",
		headers:      gatewayResponsesWSHeaders(&op.ModelConfig{APIKey: "session-token"}, nil),
		dialer:       &websocket.Dialer{HandshakeTimeout: 5 * time.Second},
		sessions:     make(map[string]*gatewayResponsesWSSessionEntry),
		sessionTTL:   time.Second,
	}
	defer closeGatewayResponsesWSSessionsForTest(prov)

	req := &ai.ResponsesRequest{
		Model:          "gpt-5.4",
		PromptCacheKey: "th_test_123",
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)),
		},
	}
	completed1 := collectCompletedResponsesResult(t, mustGatewayResponsesStream(t, prov, req))
	completed2 := collectCompletedResponsesResult(t, mustGatewayResponsesStream(t, prov, req))

	if completed1 == nil || completed1.ID != "resp_ws_1" {
		t.Fatalf("completed1 = %#v, want resp_ws_1", completed1)
	}
	if completed2 == nil || completed2.ID != "resp_ws_2" {
		t.Fatalf("completed2 = %#v, want resp_ws_2", completed2)
	}
	if got := upgrades.Load(); got != 1 {
		t.Fatalf("websocket upgrades = %d, want 1", got)
	}
	if fallback.streamCalls != 0 {
		t.Fatalf("fallback.streamCalls = %d, want 0", fallback.streamCalls)
	}
}

func TestGatewayResponsesWSProvider_DoesNotReuseCachedSessionAfterToolReplayInput(t *testing.T) {
	var upgrades atomic.Int32
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			http.NotFound(w, r)
			return
		}
		upgrades.Add(1)
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			seq := requests.Add(1)
			respID := fmt.Sprintf("resp_ws_%d", seq)
			if !strings.Contains(string(message), `"prompt_cache_key":"th_test_123"`) {
				t.Errorf("websocket payload missing prompt_cache_key: %s", string(message))
				return
			}
			_ = conn.WriteJSON(map[string]any{
				"type": "response.created",
				"response": map[string]any{
					"id":     respID,
					"model":  "gpt-5.4",
					"status": "in_progress",
					"output": []any{},
				},
			})
			_ = conn.WriteJSON(map[string]any{
				"type": "response.completed",
				"response": map[string]any{
					"id":     respID,
					"model":  "gpt-5.4",
					"status": "completed",
					"output": []any{},
				},
			})
		}
	}))
	defer server.Close()

	fallback := &gatewayResponsesFallbackStub{}
	prov := &GatewayResponsesWSProvider{
		httpFallback: fallback,
		cfg:          &op.ModelConfig{APIKey: "session-token"},
		wsURL:        "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/responses",
		headers:      gatewayResponsesWSHeaders(&op.ModelConfig{APIKey: "session-token"}, nil),
		dialer:       &websocket.Dialer{HandshakeTimeout: 5 * time.Second},
		sessions:     make(map[string]*gatewayResponsesWSSessionEntry),
		sessionTTL:   time.Second,
	}
	defer closeGatewayResponsesWSSessionsForTest(prov)

	firstReq := &ai.ResponsesRequest{
		Model:          "gpt-5.4",
		PromptCacheKey: "th_test_123",
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)),
		},
	}
	secondReq := &ai.ResponsesRequest{
		Model:          "gpt-5.4",
		PromptCacheKey: "th_test_123",
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)),
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"function_call","call_id":"call_123","name":"read","arguments":"{\"path\":\"foo.md\"}"}`)),
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"function_call_output","call_id":"call_123","output":"ok"}`)),
		},
	}
	completed1 := collectCompletedResponsesResult(t, mustGatewayResponsesStream(t, prov, firstReq))
	completed2 := collectCompletedResponsesResult(t, mustGatewayResponsesStream(t, prov, secondReq))

	if completed1 == nil || completed2 == nil {
		t.Fatalf("completed responses = %#v %#v, want non-nil", completed1, completed2)
	}
	if got := upgrades.Load(); got != 2 {
		t.Fatalf("websocket upgrades = %d, want 2 when tool replay disables session reuse", got)
	}
	if fallback.streamCalls != 0 {
		t.Fatalf("fallback.streamCalls = %d, want 0", fallback.streamCalls)
	}
}

func TestGatewayResponsesWSProvider_RedialsStaleCachedSessionWebsocket(t *testing.T) {
	var upgrades atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			http.NotFound(w, r)
			return
		}
		upgrades.Add(1)
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()
		_, _, err = conn.ReadMessage()
		if err != nil {
			return
		}
		respID := fmt.Sprintf("resp_ws_%d", upgrades.Load())
		_ = conn.WriteJSON(map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":     respID,
				"model":  "gpt-5.4",
				"status": "in_progress",
				"output": []any{},
			},
		})
		_ = conn.WriteJSON(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":     respID,
				"model":  "gpt-5.4",
				"status": "completed",
				"output": []any{},
			},
		})
	}))
	defer server.Close()

	fallback := &gatewayResponsesFallbackStub{}
	prov := &GatewayResponsesWSProvider{
		httpFallback: fallback,
		cfg:          &op.ModelConfig{APIKey: "session-token"},
		wsURL:        "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/responses",
		headers:      gatewayResponsesWSHeaders(&op.ModelConfig{APIKey: "session-token"}, nil),
		dialer:       &websocket.Dialer{HandshakeTimeout: 5 * time.Second},
		sessions:     make(map[string]*gatewayResponsesWSSessionEntry),
		sessionTTL:   time.Second,
	}
	defer closeGatewayResponsesWSSessionsForTest(prov)

	req := &ai.ResponsesRequest{
		Model:          "gpt-5.4",
		PromptCacheKey: "th_test_123",
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)),
		},
	}
	completed1 := collectCompletedResponsesResult(t, mustGatewayResponsesStream(t, prov, req))
	completed2 := collectCompletedResponsesResult(t, mustGatewayResponsesStream(t, prov, req))

	if completed1 == nil || completed1.ID != "resp_ws_1" {
		t.Fatalf("completed1 = %#v, want resp_ws_1", completed1)
	}
	if completed2 == nil || completed2.ID != "resp_ws_2" {
		t.Fatalf("completed2 = %#v, want resp_ws_2", completed2)
	}
	if got := upgrades.Load(); got != 2 {
		t.Fatalf("websocket upgrades = %d, want 2 after stale cached redial", got)
	}
	if fallback.streamCalls != 0 {
		t.Fatalf("fallback.streamCalls = %d, want 0", fallback.streamCalls)
	}
}

func mustGatewayResponsesStream(t *testing.T, prov *GatewayResponsesWSProvider, req *ai.ResponsesRequest) *ai.ResponsesEventStream {
	t.Helper()
	stream, err := prov.StreamResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("StreamResponses(): %v", err)
	}
	return stream
}

func collectCompletedResponsesResult(t *testing.T, stream *ai.ResponsesEventStream) *ai.ResponsesResult {
	t.Helper()
	var completed *ai.ResponsesResult
	for stream.Next() {
		event := stream.Event()
		if event.Type == "response.completed" {
			completed = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err(): %v", err)
	}
	return completed
}

func closeGatewayResponsesWSSessionsForTest(prov *GatewayResponsesWSProvider) {
	if prov == nil {
		return
	}
	prov.mu.Lock()
	defer prov.mu.Unlock()
	for key, entry := range prov.sessions {
		delete(prov.sessions, key)
		if entry == nil {
			continue
		}
		if entry.idleTimer != nil {
			entry.idleTimer.Stop()
			entry.idleTimer = nil
		}
		if entry.conn != nil {
			_ = entry.conn.Close()
		}
	}
}
