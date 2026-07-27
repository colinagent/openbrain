package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/gorilla/websocket"
)

func TestGatewayCanonicalWSProvider_StreamCanonicalUsesWebsocket(t *testing.T) {
	var gotModelID string
	var gotRequest *ai.ProviderRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/internal/canonical" {
			t.Fatalf("path = %s, want /v1/internal/canonical", r.URL.Path)
		}
		if !websocket.IsWebSocketUpgrade(r) {
			t.Fatalf("request was not websocket upgrade")
		}
		if got := r.Header.Get("X-Request-ID"); got != "req-canonical" {
			t.Fatalf("X-Request-ID = %q, want req-canonical", got)
		}
		if got := r.Header.Get("X-Thread-ID"); got != "thread-canonical" {
			t.Fatalf("X-Thread-ID = %q, want thread-canonical", got)
		}
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer conn.Close()
		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read websocket request: %v", err)
		}
		var decodeErr error
		gotModelID, gotRequest, decodeErr = ai.DecodeCanonicalWebsocketCreateJSON(payload)
		if decodeErr != nil {
			t.Fatalf("DecodeCanonicalWebsocketCreateJSON(): %v", decodeErr)
		}
		resp := &ai.ProviderResponse{
			Message: ai.ConversationMessage{
				Role:    ai.RoleCanonicalAssistant,
				Content: []ai.ContentBlock{{Type: ai.BlockText, Text: "hello"}},
			},
			Usage:      ai.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
			StopReason: ai.StopReasonStop,
		}
		writeCanonicalEvent(t, conn, ai.ProviderEvent{Type: ai.EventCanonicalDone, Response: resp})
		_, ackPayload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read canonical acknowledgement: %v", err)
		}
		terminalType, requestID, err := ai.DecodeCanonicalWebsocketAckJSON(ackPayload)
		if err != nil {
			t.Fatalf("DecodeCanonicalWebsocketAckJSON(): %v", err)
		}
		if terminalType != ai.EventCanonicalDone || requestID != "req-canonical" {
			t.Fatalf("ack = (%q, %q), want (done, req-canonical)", terminalType, requestID)
		}
		writeCanonicalNormalClose(t, conn)
	}))
	defer server.Close()

	prov, err := NewGatewayCanonicalWSProviderWithOptions(&op.ModelConfig{
		ID:      "gateway:claude-opus-4-6",
		Name:    "claude-opus-4-6",
		API:     "anthropic-messages",
		APIKey:  "session-token",
		BaseURL: server.URL + "/v1",
	}, &http.Client{Timeout: 5 * time.Second}, nil)
	if err != nil {
		t.Fatalf("NewGatewayCanonicalWSProviderWithOptions(): %v", err)
	}

	stream, err := prov.StreamCanonical(context.Background(), &ai.ProviderRequest{
		Context:   ai.ConversationContext{Messages: []ai.ConversationMessage{{Role: ai.RoleCanonicalUser, Content: []ai.ContentBlock{{Type: ai.BlockText, Text: "hi"}}}}},
		Config:    ai.GenerationConfig{Model: "claude-opus-4-6"},
		RequestID: "req-canonical",
		ThreadID:  "thread-canonical",
	})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}
	defer stream.Close()
	var final *ai.ProviderResponse
	for stream.Next() {
		event := stream.Event()
		if event.Type == ai.EventCanonicalDone {
			final = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err(): %v", err)
	}
	if final == nil || !ai.HasSemanticCanonicalResponse(final) {
		t.Fatalf("final = %#v, want semantic response", final)
	}
	if got := final.Message.Content[0].Text; got != "hello" {
		t.Fatalf("final text = %q, want hello", got)
	}
	if gotModelID != "gateway:claude-opus-4-6" {
		t.Fatalf("gotModelID = %q, want gateway:claude-opus-4-6", gotModelID)
	}
	if gotRequest == nil || gotRequest.Config.Model != "claude-opus-4-6" {
		t.Fatalf("gotRequest = %#v, want config model claude-opus-4-6", gotRequest)
	}
}

func TestGatewayCanonicalWSProvider_StreamCanonicalReturnsWebsocketError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read websocket request: %v", err)
		}
		event := ai.ProviderEvent{Type: ai.EventCanonicalError, Error: ai.WrapRetryError(context.DeadlineExceeded, 504, "timeout", "gateway timeout", 1500)}
		writeCanonicalEvent(t, conn, event)
		_, ackPayload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read canonical acknowledgement: %v", err)
		}
		terminalType, _, err := ai.DecodeCanonicalWebsocketAckJSON(ackPayload)
		if err != nil {
			t.Fatalf("DecodeCanonicalWebsocketAckJSON(): %v", err)
		}
		if terminalType != ai.EventCanonicalError {
			t.Fatalf("ack terminal type = %q, want error", terminalType)
		}
		writeCanonicalNormalClose(t, conn)
	}))
	defer server.Close()

	prov, err := NewGatewayCanonicalWSProviderWithOptions(&op.ModelConfig{
		ID:      "gateway:claude-opus-4-6",
		Name:    "claude-opus-4-6",
		API:     "anthropic-messages",
		APIKey:  "session-token",
		BaseURL: server.URL + "/v1",
	}, &http.Client{Timeout: 5 * time.Second}, nil)
	if err != nil {
		t.Fatalf("NewGatewayCanonicalWSProviderWithOptions(): %v", err)
	}

	stream, err := prov.StreamCanonical(context.Background(), &ai.ProviderRequest{Config: ai.GenerationConfig{Model: "claude-opus-4-6"}})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}
	defer stream.Close()
	if !stream.Next() {
		t.Fatalf("stream.Next() = false, want websocket failure event")
	}
	if err := stream.Err(); err == nil || !strings.Contains(err.Error(), "gateway timeout") {
		t.Fatalf("stream.Err() = %v, want gateway timeout", err)
	}
}

func TestGatewayCanonicalWSProvider_StreamCanonicalStreamTimesOutWhenGatewayStaysSilent(t *testing.T) {
	oldTimeout := gatewayWebsocketReadTimeout
	gatewayWebsocketReadTimeout = 20 * time.Millisecond
	defer func() {
		gatewayWebsocketReadTimeout = oldTimeout
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read websocket request: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	prov, err := NewGatewayCanonicalWSProviderWithOptions(&op.ModelConfig{
		ID:      "gateway:claude-opus-4-6",
		Name:    "claude-opus-4-6",
		API:     "anthropic-messages",
		APIKey:  "session-token",
		BaseURL: server.URL + "/v1",
	}, &http.Client{Timeout: 5 * time.Second}, nil)
	if err != nil {
		t.Fatalf("NewGatewayCanonicalWSProviderWithOptions(): %v", err)
	}

	stream, err := prov.StreamCanonical(context.Background(), &ai.ProviderRequest{Config: ai.GenerationConfig{Model: "claude-opus-4-6"}})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}
	defer stream.Close()
	if !stream.Next() {
		t.Fatalf("stream.Next() = false, want timeout error event")
	}
	if err := stream.Err(); err == nil || !strings.Contains(err.Error(), "read canonical websocket event") {
		t.Fatalf("stream.Err() = %v, want websocket read error", err)
	}
}

func TestGatewayCanonicalWSProvider_StreamCanonicalReadHonorsContextCancel(t *testing.T) {
	requestRead := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read websocket request: %v", err)
		}
		close(requestRead)
		<-r.Context().Done()
	}))
	defer server.Close()

	prov, err := NewGatewayCanonicalWSProviderWithOptions(&op.ModelConfig{
		ID:      "gateway:claude-opus-4-6",
		Name:    "claude-opus-4-6",
		API:     "anthropic-messages",
		APIKey:  "session-token",
		BaseURL: server.URL + "/v1",
	}, &http.Client{Timeout: 5 * time.Second}, nil)
	if err != nil {
		t.Fatalf("NewGatewayCanonicalWSProviderWithOptions(): %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := prov.StreamCanonical(ctx, &ai.ProviderRequest{Config: ai.GenerationConfig{Model: "claude-opus-4-6"}})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}
	defer stream.Close()

	select {
	case <-requestRead:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for websocket request")
	}
	cancel()

	errCh := make(chan error, 1)
	go func() {
		if !stream.Next() {
			errCh <- stream.Err()
			return
		}
		errCh <- stream.Err()
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("stream.Err() = nil, want cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("stream.Next() did not return after context cancellation")
	}
}

func TestGatewayCanonicalWSProvider_StreamCanonicalCarriesStreamingPartialToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read websocket request: %v", err)
		}
		start := ai.ProviderEvent{
			Type: ai.EventCanonicalStart,
			Partial: &ai.StreamConversationMessage{
				Role: ai.RoleCanonicalAssistant,
			},
		}
		delta := ai.ProviderEvent{
			Type:         ai.EventCanonicalToolCallDelta,
			ContentIndex: 0,
			Delta:        `{"path":"/tmp/out.md"`,
			Block: &ai.StreamContentBlock{
				Type: ai.BlockToolCall,
				ToolCall: &ai.StreamToolCall{
					ID:           "call-1",
					Name:         "write",
					RawArguments: `{"path":"/tmp/out.md"`,
					Complete:     false,
				},
			},
			Partial: &ai.StreamConversationMessage{
				Role: ai.RoleCanonicalAssistant,
				Content: []ai.StreamContentBlock{{
					Type: ai.BlockToolCall,
					ToolCall: &ai.StreamToolCall{
						ID:           "call-1",
						Name:         "write",
						RawArguments: `{"path":"/tmp/out.md"`,
						Complete:     false,
					},
				}},
			},
		}
		done := ai.ProviderEvent{
			Type: ai.EventCanonicalDone,
			Response: &ai.ProviderResponse{
				Message: ai.ConversationMessage{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "ok",
					}},
				},
				StopReason: ai.StopReasonStop,
			},
		}
		for _, event := range []ai.ProviderEvent{start, delta, done} {
			writeCanonicalEvent(t, conn, event)
		}
		_, ackPayload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read canonical acknowledgement: %v", err)
		}
		terminalType, _, err := ai.DecodeCanonicalWebsocketAckJSON(ackPayload)
		if err != nil || terminalType != ai.EventCanonicalDone {
			t.Fatalf("canonical acknowledgement = (%q, %v), want done", terminalType, err)
		}
		writeCanonicalNormalClose(t, conn)
	}))
	defer server.Close()

	prov, err := NewGatewayCanonicalWSProviderWithOptions(&op.ModelConfig{
		ID:      "gateway:claude-opus-4-6",
		Name:    "claude-opus-4-6",
		API:     "anthropic-messages",
		APIKey:  "session-token",
		BaseURL: server.URL + "/v1",
	}, &http.Client{Timeout: 5 * time.Second}, nil)
	if err != nil {
		t.Fatalf("NewGatewayCanonicalWSProviderWithOptions(): %v", err)
	}

	stream, err := prov.StreamCanonical(context.Background(), &ai.ProviderRequest{Config: ai.GenerationConfig{Model: "claude-opus-4-6"}})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}
	defer stream.Close()

	var sawDelta bool
	for stream.Next() {
		event := stream.Event()
		if event.Type != ai.EventCanonicalToolCallDelta {
			continue
		}
		if event.Partial != nil {
			t.Fatalf("event.Partial = %#v, want no cumulative snapshot on gateway wire", event.Partial)
		}
		if event.Block != nil {
			t.Fatalf("event.Block = %#v, want compact delta-only event", event.Block)
		}
		if event.ContentIndex != 0 || event.Delta != `{"path":"/tmp/out.md"` {
			t.Fatalf("event = %#v, want indexed raw argument delta", event)
		}
		sawDelta = true
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err(): %v", err)
	}
	if !sawDelta {
		t.Fatal("expected toolcall delta event")
	}
}

func TestGatewayCanonicalWSProvider_StreamCanonicalRejectsUnexpectedEOFBeforeDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read websocket request: %v", err)
		}
		for i := 0; i < 256; i++ {
			writeCanonicalEvent(t, conn, ai.ProviderEvent{Type: ai.EventCanonicalTextDelta, ContentIndex: 0, Delta: "x"})
		}
		_ = conn.Close()
	}))
	defer server.Close()

	prov := newTestGatewayCanonicalWSProvider(t, server.URL)
	stream, err := prov.StreamCanonical(context.Background(), &ai.ProviderRequest{Config: ai.GenerationConfig{Model: "claude-opus-4-6"}})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}
	defer stream.Close()
	for stream.Next() {
	}
	streamErr := stream.Err()
	if streamErr == nil || !strings.Contains(streamErr.Error(), "read canonical websocket event") {
		t.Fatalf("stream.Err() = %v, want unexpected websocket EOF", streamErr)
	}
	if retryErr := ai.NormalizeRetryError(streamErr); retryErr == nil || !retryErr.Retryable {
		t.Fatalf("NormalizeRetryError() = %#v, want retryable", retryErr)
	}
}

func TestGatewayCanonicalWSProvider_StreamCanonicalDeliversHighVolumeBeforeDone(t *testing.T) {
	const deltaCount = 6000
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read websocket request: %v", err)
		}
		for i := 0; i < deltaCount; i++ {
			writeCanonicalEvent(t, conn, ai.ProviderEvent{Type: ai.EventCanonicalTextDelta, ContentIndex: 0, Delta: "x"})
		}
		writeCanonicalEvent(t, conn, ai.ProviderEvent{Type: ai.EventCanonicalDone, Response: assistantGatewayResponse("complete")})
		_, ackPayload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read canonical acknowledgement: %v", err)
		}
		terminalType, _, err := ai.DecodeCanonicalWebsocketAckJSON(ackPayload)
		if err != nil || terminalType != ai.EventCanonicalDone {
			t.Fatalf("canonical acknowledgement = (%q, %v), want done", terminalType, err)
		}
		writeCanonicalNormalClose(t, conn)
	}))
	defer server.Close()

	prov := newTestGatewayCanonicalWSProvider(t, server.URL)
	stream, err := prov.StreamCanonical(context.Background(), &ai.ProviderRequest{Config: ai.GenerationConfig{Model: "claude-opus-4-6"}})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}
	defer stream.Close()
	time.Sleep(25 * time.Millisecond)
	gotDeltas := 0
	sawDone := false
	for stream.Next() {
		switch stream.Event().Type {
		case ai.EventCanonicalTextDelta:
			gotDeltas++
		case ai.EventCanonicalDone:
			sawDone = true
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err(): %v", err)
	}
	if gotDeltas != deltaCount || !sawDone {
		t.Fatalf("events = (%d deltas, done=%v), want (%d, true)", gotDeltas, sawDone, deltaCount)
	}
}

func newTestGatewayCanonicalWSProvider(t *testing.T, serverURL string) *GatewayCanonicalWSProvider {
	t.Helper()
	prov, err := NewGatewayCanonicalWSProviderWithOptions(&op.ModelConfig{
		ID:      "gateway:claude-opus-4-6",
		Name:    "claude-opus-4-6",
		API:     "anthropic-messages",
		APIKey:  "session-token",
		BaseURL: serverURL + "/v1",
	}, &http.Client{Timeout: 5 * time.Second}, nil)
	if err != nil {
		t.Fatalf("NewGatewayCanonicalWSProviderWithOptions(): %v", err)
	}
	return prov
}

func writeCanonicalEvent(t *testing.T, conn *websocket.Conn, event ai.ProviderEvent) {
	t.Helper()
	payload, err := ai.MarshalCanonicalStreamEventJSON(event)
	if err != nil {
		t.Fatalf("MarshalCanonicalStreamEventJSON(): %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write websocket event: %v", err)
	}
}

func writeCanonicalNormalClose(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "canonical terminal delivered"),
		time.Now().Add(time.Second),
	); err != nil {
		t.Fatalf("write canonical normal close: %v", err)
	}
}

func assistantGatewayResponse(text string) *ai.ProviderResponse {
	return &ai.ProviderResponse{
		Message: ai.ConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: text,
			}},
		},
		StopReason: ai.StopReasonStop,
	}
}
