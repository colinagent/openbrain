package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
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
		_ = conn.WriteMessage(websocket.TextMessage, ai.RenderCanonicalStreamEventJSON(ai.ProviderEvent{Type: ai.EventCanonicalDone, Response: resp}))
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
		_ = conn.WriteMessage(websocket.TextMessage, ai.RenderCanonicalStreamEventJSON(event))
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
			if err := conn.WriteMessage(websocket.TextMessage, ai.RenderCanonicalStreamEventJSON(event)); err != nil {
				t.Fatalf("write websocket event: %v", err)
			}
		}
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

	var sawPartial bool
	for stream.Next() {
		event := stream.Event()
		if event.Type != ai.EventCanonicalToolCallDelta {
			continue
		}
		if event.Partial == nil || len(event.Partial.Content) != 1 {
			t.Fatalf("event.Partial = %#v, want one toolcall block", event.Partial)
		}
		if event.Block == nil || event.Block.ToolCall == nil || event.Block.ToolCall.Complete {
			t.Fatalf("event.Block = %#v, want incomplete streaming tool call", event.Block)
		}
		sawPartial = true
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err(): %v", err)
	}
	if !sawPartial {
		t.Fatal("expected toolcall delta event")
	}
}
