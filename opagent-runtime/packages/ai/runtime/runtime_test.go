package runtime

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai/provider"
)

type scriptedProvider struct {
	completeErr error
	stream      []ai.ProviderEvent
	streamErr   error
}

type scriptedCanonicalProvider struct {
	response *ai.ProviderResponse
}

type recordingProvider struct {
	id    string
	calls *[]string
}

func (p *scriptedCanonicalProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-responses")
}

func (p *scriptedCanonicalProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	if p.response != nil {
		return p.response, nil
	}
	return &ai.ProviderResponse{
		Message: ai.ConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "ok",
			}},
		},
		StopReason: ai.StopReasonStop,
	}, nil
}

func (p *scriptedCanonicalProvider) StreamCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	out := ai.NewProviderEventStream(1)
	go func() {
		resp, _ := p.CompleteCanonical(context.Background(), nil)
		_ = out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalDone, Response: resp})
		out.Close()
	}()
	return out, nil
}

func (p *scriptedProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-completions")
}

func (p *scriptedProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	if p.completeErr != nil {
		return nil, p.completeErr
	}
	return &ai.ProviderResponse{
		Message: ai.ConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "ok",
			}},
		},
		StopReason: ai.StopReasonStop,
	}, nil
}

func (p *scriptedProvider) StreamCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	out := ai.NewProviderEventStream(len(p.stream) + 1)
	go func() {
		for _, event := range p.stream {
			_ = out.Emit(event)
		}
		if p.streamErr != nil {
			out.Finish(p.streamErr)
			return
		}
		out.Close()
	}()
	return out, nil
}

func (p *recordingProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-completions")
}

func (p *recordingProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	if p.calls != nil {
		*p.calls = append(*p.calls, p.id)
	}
	return &ai.ProviderResponse{
		Message: ai.ConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "ok",
			}},
		},
		StopReason: ai.StopReasonStop,
	}, nil
}

func (p *recordingProvider) StreamCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	return nil, errors.New("unexpected canonical Stream call")
}

type scriptedStreamingCanonicalProvider struct {
	events []ai.ProviderEvent
	err    error
}

type captureResponsesProvider struct {
	lastRequest *ai.ResponsesRequest
}

func (p *captureResponsesProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-responses")
}

func (p *captureResponsesProvider) CompleteCanonical(_ context.Context, _ *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return &ai.ProviderResponse{Message: ai.ConversationMessage{Role: ai.RoleCanonicalAssistant, Content: []ai.ContentBlock{{Type: ai.BlockText, Text: "ok"}}}, StopReason: ai.StopReasonStop}, nil
}

func (p *captureResponsesProvider) StreamCanonical(_ context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	resp, err := p.CompleteCanonical(context.Background(), req)
	if err != nil {
		return nil, err
	}
	out := ai.NewProviderEventStream(1)
	go func() {
		_ = out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalDone, Response: resp})
		out.Close()
	}()
	return out, nil
}

func (p *captureResponsesProvider) CompleteResponses(_ context.Context, req *ai.ResponsesRequest) (*ai.ResponsesResult, error) {
	p.lastRequest = cloneResponsesRequest(req)
	return &ai.ResponsesResult{ID: "resp_capture", Model: req.Model, Status: "completed"}, nil
}

func (p *captureResponsesProvider) StreamResponses(_ context.Context, req *ai.ResponsesRequest) (*ai.ResponsesEventStream, error) {
	p.lastRequest = cloneResponsesRequest(req)
	out := ai.NewResponsesEventStream(1)
	go func() {
		_ = out.Emit(ai.ResponsesStreamEvent{Type: "response.completed", Response: &ai.ResponsesResult{ID: "resp_capture", Model: req.Model, Status: "completed"}})
		out.Close()
	}()
	return out, nil
}

type captureCanonicalProvider struct {
	lastRequest *ai.ProviderRequest
}

func resetSingleModelProviderCacheForTest() {
	singleModelProviderCache.mu.Lock()
	defer singleModelProviderCache.mu.Unlock()
	singleModelProviderCache.providers = make(map[string]ai.CanonicalProvider)
}

func (p *captureCanonicalProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-responses")
}

func (p *captureCanonicalProvider) CompleteCanonical(_ context.Context, req *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	p.lastRequest = cloneCanonicalRequest(req)
	return &ai.ProviderResponse{
		Message: ai.ConversationMessage{
			Role:    ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{Type: ai.BlockText, Text: "ok"}},
		},
		StopReason: ai.StopReasonStop,
	}, nil
}

func (p *captureCanonicalProvider) StreamCanonical(_ context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	p.lastRequest = cloneCanonicalRequest(req)
	out := ai.NewProviderEventStream(1)
	go func() {
		_ = out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalDone, Response: &ai.ProviderResponse{
			Message:    ai.ConversationMessage{Role: ai.RoleCanonicalAssistant, Content: []ai.ContentBlock{{Type: ai.BlockText, Text: "ok"}}},
			StopReason: ai.StopReasonStop,
		}})
		out.Close()
	}()
	return out, nil
}

func (p *scriptedStreamingCanonicalProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-responses")
}

func (p *scriptedStreamingCanonicalProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return nil, errors.New("unexpected CompleteCanonical call")
}

func (p *scriptedStreamingCanonicalProvider) StreamCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	out := ai.NewProviderEventStream(len(p.events) + 1)
	go func() {
		for _, event := range p.events {
			_ = out.Emit(event)
		}
		if p.err != nil {
			out.Finish(p.err)
			return
		}
		out.Close()
	}()
	return out, nil
}

func TestHTTPClient_KeepsDefaultHTTP2AndCompressionBehaviorForResponses(t *testing.T) {
	rt, err := New(Config{
		Providers: []ProviderEndpoint{{
			ID:       "resp-endpoint",
			Provider: "openai",
			BaseURL:  "https://example.com/v1",
			APIKey:   "test-key",
		}},
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	typed, ok := rt.(*router)
	if !ok {
		t.Fatalf("runtime type = %T, want *router", rt)
	}
	client := typed.httpClient(ProviderEndpoint{ID: "resp-endpoint"})
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.DisableCompression {
		t.Fatalf("DisableCompression = true, want false")
	}
	if transport.TLSNextProto != nil {
		t.Fatalf("TLSNextProto = %#v, want nil default behavior", transport.TLSNextProto)
	}
}

func TestDefaultProviderFactory_UsesGatewayResponsesWSProviderForGatewayResponses(t *testing.T) {
	prov, err := defaultProviderFactory(context.Background(), ProviderEndpoint{
		ID:       "gateway-responses",
		Provider: "opagent-ai-gateway",
		BaseURL:  "https://ai-gateway.example/v1",
		APIKey:   "session-token",
	}, LogicalModel{
		ID:       "gateway:gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "opagent-ai-gateway",
		API:      "openai-responses",
	}, RouteCandidate{ProviderRef: "gateway-responses", UpstreamModel: "gpt-5.4"}, &http.Client{})
	if err != nil {
		t.Fatalf("defaultProviderFactory(): %v", err)
	}
	if _, ok := prov.(*provider.GatewayResponsesWSProvider); !ok {
		t.Fatalf("provider type = %T, want *provider.GatewayResponsesWSProvider", prov)
	}
}

func TestDefaultProviderFactory_DoesNotUseGatewayWSProviderForUpstreamResponsesInsideGateway(t *testing.T) {
	prov, err := defaultProviderFactory(context.Background(), ProviderEndpoint{
		ID:       "upstream-responses",
		Provider: "aigocode",
		BaseURL:  "https://api.aigocode.com/v1",
		APIKey:   "upstream-key",
	}, LogicalModel{
		ID:       "gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "opagent-ai-gateway",
		API:      "openai-responses",
	}, RouteCandidate{ProviderRef: "upstream-responses", UpstreamModel: "gpt-5.4"}, &http.Client{})
	if err != nil {
		t.Fatalf("defaultProviderFactory(): %v", err)
	}
	if _, ok := prov.(*provider.GatewayResponsesWSProvider); ok {
		t.Fatalf("provider type = %T, want upstream HTTP responses provider", prov)
	}
	if _, ok := prov.(*provider.ResponsesProvider); !ok {
		t.Fatalf("provider type = %T, want *provider.ResponsesProvider", prov)
	}
}

func TestDefaultProviderFactory_KeepsDirectResponsesOnHTTPProvider(t *testing.T) {
	prov, err := defaultProviderFactory(context.Background(), ProviderEndpoint{
		ID:       "direct-responses",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
		APIKey:   "sk-test",
	}, LogicalModel{
		ID:       "custom:gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "openai",
		API:      "openai-responses",
	}, RouteCandidate{ProviderRef: "direct-responses", UpstreamModel: "gpt-5.4"}, &http.Client{})
	if err != nil {
		t.Fatalf("defaultProviderFactory(): %v", err)
	}
	if _, ok := prov.(*provider.ResponsesProvider); !ok {
		t.Fatalf("provider type = %T, want *provider.ResponsesProvider", prov)
	}
}

func TestDefaultProviderFactory_UsesModelAPIOverEndpointAPI(t *testing.T) {
	prov, err := defaultProviderFactory(context.Background(), ProviderEndpoint{
		ID:       "gateway-mismatch",
		Provider: "opagent-ai-gateway",
		BaseURL:  "https://ai-gateway.example/v1",
		APIKey:   "session-token",
	}, LogicalModel{
		ID:       "gateway:gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "opagent-ai-gateway",
		API:      "openai-responses",
	}, RouteCandidate{ProviderRef: "gateway-mismatch", UpstreamModel: "gpt-5.4"}, &http.Client{})
	if err != nil {
		t.Fatalf("defaultProviderFactory(): %v", err)
	}
	if _, ok := prov.(*provider.GatewayResponsesWSProvider); !ok {
		t.Fatalf("provider type = %T, want *provider.GatewayResponsesWSProvider", prov)
	}
}

func TestDefaultProviderFactory_UsesGatewayCanonicalWSProviderForGatewayAnthropicModel(t *testing.T) {
	prov, err := defaultProviderFactory(context.Background(), ProviderEndpoint{
		ID:       "gateway-claude",
		Provider: "opagent-ai-gateway",
		BaseURL:  "https://ai-gateway.example/v1",
		APIKey:   "session-token",
	}, LogicalModel{
		ID:       "gateway:claude-opus-4-6",
		Name:     "claude-opus-4-6",
		Provider: "opagent-ai-gateway",
		API:      "anthropic-messages",
	}, RouteCandidate{ProviderRef: "gateway-claude", UpstreamModel: "claude-opus-4-6"}, &http.Client{})
	if err != nil {
		t.Fatalf("defaultProviderFactory(): %v", err)
	}
	if _, ok := prov.(*provider.GatewayCanonicalWSProvider); !ok {
		t.Fatalf("provider type = %T, want *provider.GatewayCanonicalWSProvider", prov)
	}
}

func TestDefaultProviderFactory_UsesGatewayCanonicalWSProviderForGatewayGeminiModel(t *testing.T) {
	prov, err := defaultProviderFactory(context.Background(), ProviderEndpoint{
		ID:       "gateway-gemini",
		Provider: "opagent-ai-gateway",
		BaseURL:  "https://ai-gateway.example/v1",
		APIKey:   "session-token",
	}, LogicalModel{
		ID:       "gateway:gemini-3.1-pro-preview",
		Name:     "gemini-3.1-pro-preview",
		Provider: "opagent-ai-gateway",
		API:      "gemini-native",
	}, RouteCandidate{ProviderRef: "gateway-gemini", UpstreamModel: "gemini-3.1-pro-preview"}, &http.Client{})
	if err != nil {
		t.Fatalf("defaultProviderFactory(): %v", err)
	}
	if _, ok := prov.(*provider.GatewayCanonicalWSProvider); !ok {
		t.Fatalf("provider type = %T, want *provider.GatewayCanonicalWSProvider", prov)
	}
}

func newTestRuntime(t *testing.T, providers map[string]APIProvider) Runtime {
	t.Helper()
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{
			{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"},
			{ID: "p2", Provider: "openai", BaseURL: "https://two.example/v1", APIKey: "k2"},
		},
		Models: []LogicalModel{{
			ID:   "m1",
			Name: "model-one",
			API:  "openai-completions",
			Routes: []RouteCandidate{
				{ProviderRef: "p1", UpstreamModel: "model-one", Priority: 0, Weight: 2},
				{ProviderRef: "p2", UpstreamModel: "model-one", Priority: 1, Weight: 1},
			},
		}},
		RetryPolicy: RetryPolicy{
			MaxAttempts:      3,
			InitialBackoff:   time.Millisecond,
			MaxBackoff:       2 * time.Millisecond,
			TotalBudget:      time.Second,
			FirstByteTimeout: 50 * time.Millisecond,
			Cooldown:         20 * time.Millisecond,
		},
	}, func(_ context.Context, endpoint ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		prov, ok := providers[endpoint.ID]
		if !ok {
			return nil, errors.New("missing provider")
		}
		return prov, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	return rt
}

func TestRuntimeCompleteFailsOverToNextCandidate(t *testing.T) {
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{
			{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"},
			{ID: "p2", Provider: "openai", BaseURL: "https://two.example/v1", APIKey: "k2"},
		},
		Models: []LogicalModel{{
			ID:   "m1",
			Name: "model-one",
			Routes: []RouteCandidate{
				{ProviderRef: "p1", UpstreamModel: "model-one"},
				{ProviderRef: "p2", UpstreamModel: "model-one", Priority: 1},
			},
		}},
	}, func(_ context.Context, endpoint ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		if endpoint.ID == "p1" {
			return &scriptedProvider{completeErr: errors.New("503 upstream unavailable")}, nil
		}
		return &scriptedProvider{}, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	resp, err := rt.CompleteCanonical(context.Background(), &CanonicalRequest{
		ModelID: "m1",
		Params: &ai.ProviderRequest{
			Context: ai.ConversationContext{
				Messages: []ai.ConversationMessage{{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "ping",
					}},
				}},
			},
			Config: ai.GenerationConfig{},
		},
	})
	if err != nil {
		t.Fatalf("CompleteCanonical(): %v", err)
	}
	msg, err := ai.OpMessageFromCanonical(resp.Message)
	if err != nil {
		t.Fatalf("OpMessageFromCanonical(): %v", err)
	}
	if got := msg.Content; got != "ok" {
		t.Fatalf("Content = %q, want ok", got)
	}
}

func TestRuntimeStreamFailsOverBeforeFirstEvent(t *testing.T) {
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{
			{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"},
			{ID: "p2", Provider: "openai", BaseURL: "https://two.example/v1", APIKey: "k2"},
		},
		Models: []LogicalModel{{
			ID:   "m1",
			Name: "model-one",
			Routes: []RouteCandidate{
				{ProviderRef: "p1", UpstreamModel: "model-one"},
				{ProviderRef: "p2", UpstreamModel: "model-one", Priority: 1},
			},
		}},
	}, func(_ context.Context, endpoint ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		if endpoint.ID == "p1" {
			return &scriptedProvider{
				stream:    []ai.ProviderEvent{{Type: ai.EventCanonicalError, Error: errors.New("503 upstream unavailable")}},
				streamErr: errors.New("503 upstream unavailable"),
			}, nil
		}
		return &scriptedProvider{
			stream: []ai.ProviderEvent{
				{Type: ai.EventCanonicalTextDelta, Delta: "ok"},
				{Type: ai.EventCanonicalDone, Response: ai.ProviderResponseFromOpMessage(op.Message{Role: op.RoleAssistant, Content: "ok"}, ai.Usage{}, ai.StopReasonStop)},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	stream, err := rt.StreamCanonical(context.Background(), &CanonicalRequest{
		ModelID: "m1",
		Params: &ai.ProviderRequest{
			Context: ai.ConversationContext{
				Messages: []ai.ConversationMessage{{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "ping",
					}},
				}},
			},
			Config: ai.GenerationConfig{},
		},
	})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}
	var sawText bool
	for stream.Next() {
		event := stream.Event()
		if event.Type == ai.EventCanonicalTextDelta && event.Delta == "ok" {
			sawText = true
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if !sawText {
		t.Fatal("expected failover stream to emit text delta")
	}
}

func TestRuntimeStreamFailsOverAfterCanonicalStartBeforeBody(t *testing.T) {
	retryErr := ai.WrapRetryError(errors.New("unexpected end of JSON input"), 0, "", "unexpected end of JSON input", 0)
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{
			{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"},
			{ID: "p2", Provider: "openai", BaseURL: "https://two.example/v1", APIKey: "k2"},
		},
		Models: []LogicalModel{{
			ID:   "m1",
			Name: "gpt-5.4",
			API:  "openai-responses",
			Routes: []RouteCandidate{
				{ProviderRef: "p1", UpstreamModel: "gpt-5.4"},
				{ProviderRef: "p2", UpstreamModel: "gpt-5.4", Priority: 1},
			},
		}},
		RetryPolicy: RetryPolicy{
			MaxAttempts:      3,
			InitialBackoff:   time.Millisecond,
			MaxBackoff:       2 * time.Millisecond,
			TotalBudget:      time.Second,
			FirstByteTimeout: 50 * time.Millisecond,
		},
	}, func(_ context.Context, endpoint ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		if endpoint.ID == "p1" {
			return &scriptedStreamingCanonicalProvider{
				events: []ai.ProviderEvent{{Type: ai.EventCanonicalStart}},
				err:    retryErr,
			}, nil
		}
		return &scriptedStreamingCanonicalProvider{
			events: []ai.ProviderEvent{
				{Type: ai.EventCanonicalStart},
				{Type: ai.EventCanonicalTextDelta, Delta: "ok"},
				{Type: ai.EventCanonicalDone, Response: &ai.ProviderResponse{
					Message: ai.ConversationMessage{
						Role: ai.RoleCanonicalAssistant,
						Content: []ai.ContentBlock{{
							Type: ai.BlockText,
							Text: "ok",
						}},
					},
					StopReason: ai.StopReasonStop,
				}},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	stream, err := rt.StreamCanonical(context.Background(), &CanonicalRequest{
		ModelID: "m1",
		Params: &ai.ProviderRequest{
			Context: ai.ConversationContext{
				Messages: []ai.ConversationMessage{{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "ping",
					}},
				}},
			},
			Config: ai.GenerationConfig{Model: "gpt-5.4"},
		},
	})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}
	var text string
	for stream.Next() {
		event := stream.Event()
		if event.Type == ai.EventCanonicalTextDelta {
			text += event.Delta
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if text != "ok" {
		t.Fatalf("stream text = %q, want ok", text)
	}
}

func TestRuntimeStickyPrefersLastSuccessfulProvider(t *testing.T) {
	var calls []string
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{
			{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"},
			{ID: "p2", Provider: "openai", BaseURL: "https://two.example/v1", APIKey: "k2"},
		},
		Models: []LogicalModel{{
			ID:   "m1",
			Name: "model-one",
			Routes: []RouteCandidate{
				{ProviderRef: "p1", UpstreamModel: "model-one"},
				{ProviderRef: "p2", UpstreamModel: "model-one", Priority: 1},
			},
		}},
		StickyPolicy: StickyPolicy{Scope: StickyScopeThread, TTL: time.Minute},
	}, func(_ context.Context, endpoint ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		return &recordingProvider{id: endpoint.ID, calls: &calls}, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	req := &CanonicalRequest{
		ModelID:  "m1",
		ThreadID: "thread-1",
		Params: &ai.ProviderRequest{
			Context: ai.ConversationContext{
				Messages: []ai.ConversationMessage{{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "ping",
					}},
				}},
			},
			Config: ai.GenerationConfig{},
		},
	}
	if _, err := rt.CompleteCanonical(context.Background(), req); err != nil {
		t.Fatalf("CompleteCanonical() first: %v", err)
	}
	if _, err := rt.CompleteCanonical(context.Background(), req); err != nil {
		t.Fatalf("CompleteCanonical() second: %v", err)
	}
	if len(calls) < 2 || calls[0] != "p1" || calls[1] != "p1" {
		t.Fatalf("calls = %#v, want sticky reuse of p1", calls)
	}
}

func TestRuntimeSamePriorityUsesWeightedShuffle(t *testing.T) {
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{
			{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"},
			{ID: "p2", Provider: "openai", BaseURL: "https://two.example/v1", APIKey: "k2"},
		},
		Models: []LogicalModel{{
			ID:   "m1",
			Name: "model-one",
			Routes: []RouteCandidate{
				{ProviderRef: "p1", UpstreamModel: "model-one", Priority: 0, Weight: 9},
				{ProviderRef: "p2", UpstreamModel: "model-one", Priority: 0, Weight: 1},
			},
		}},
	}, func(_ context.Context, endpoint ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		return &scriptedProvider{}, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	typed, ok := rt.(*router)
	if !ok {
		t.Fatalf("runtime type = %T, want *router", rt)
	}
	typed.rng.Seed(1)
	firstCounts := map[string]int{}
	for i := 0; i < 1000; i++ {
		resolved, err := typed.ResolveModel("m1", ResolveOptions{})
		if err != nil {
			t.Fatalf("ResolveModel(): %v", err)
		}
		if len(resolved.Candidates) == 0 {
			t.Fatal("expected candidates")
		}
		firstCounts[resolved.Candidates[0].Endpoint.ID]++
	}
	if firstCounts["p1"] <= firstCounts["p2"] {
		t.Fatalf("weighted shuffle should favor p1, got %#v", firstCounts)
	}
}

func TestResolveModel_ReturnsCooledRoutesWhenCooldownWouldEliminateAllProviders(t *testing.T) {
	rt, err := New(Config{
		Providers: []ProviderEndpoint{{
			ID:       "p1",
			Provider: "openai",
			BaseURL:  "https://one.example/v1",
			APIKey:   "k1",
		}},
		Models: []LogicalModel{{
			ID:   "m1",
			Name: "gpt-5.4",
			API:  "openai-responses",
			Routes: []RouteCandidate{{
				ProviderRef:   "p1",
				UpstreamModel: "gpt-5.4",
			}},
		}},
		RetryPolicy: RetryPolicy{Cooldown: time.Minute},
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	typed, ok := rt.(*router)
	if !ok {
		t.Fatalf("runtime type = %T, want *router", rt)
	}
	typed.recordFailure("p1")
	resolved, err := typed.ResolveModel("m1", ResolveOptions{})
	if err != nil {
		t.Fatalf("ResolveModel(): %v", err)
	}
	if len(resolved.Candidates) != 1 || resolved.Candidates[0].Endpoint.ID != "p1" {
		t.Fatalf("resolved candidates = %#v, want cooled single route to remain selectable", resolved.Candidates)
	}
}

func TestSingleModelProvider_DisablesSelfCooldown(t *testing.T) {
	prov, err := NewSingleModelProvider(&op.ModelConfig{
		ID:              "gpt-5.4",
		Name:            "gpt-5.4",
		Provider:        "openai",
		API:             "openai-responses",
		APIKey:          "secret",
		BaseURL:         "https://example.com/v1",
		Reasoning:       true,
		ReasoningLevels: []string{"minimal", "low", "medium", "high", "xhigh"},
	})
	if err != nil {
		t.Fatalf("NewSingleModelProvider(): %v", err)
	}
	single, ok := prov.(*singleModelProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *singleModelProvider", prov)
	}
	router, ok := single.runtime.(*router)
	if !ok {
		t.Fatalf("runtime type = %T, want *router", single.runtime)
	}
	if router.retry.Cooldown != 0 {
		t.Fatalf("router.retry.Cooldown = %s, want 0 for single-provider runtime", router.retry.Cooldown)
	}
}

func TestSingleModelProvider_OpenAIResponsesImplementsCanonicalProvider(t *testing.T) {
	prov, err := NewSingleModelProvider(&op.ModelConfig{
		ID:              "gpt-5.4",
		Name:            "gpt-5.4",
		Provider:        "openai",
		API:             "openai-responses",
		APIKey:          "secret",
		BaseURL:         "https://example.com/v1",
		Reasoning:       true,
		ReasoningLevels: []string{"minimal", "low", "medium", "high", "xhigh"},
	})
	if err != nil {
		t.Fatalf("NewSingleModelProvider(): %v", err)
	}
	canonical, ok := prov.(ai.CanonicalProvider)
	if !ok {
		t.Fatalf("provider type %T does not implement ai.CanonicalProvider", prov)
	}
	if _, ok := prov.(ai.ResponsesProvider); !ok {
		t.Fatalf("provider type %T does not implement ai.ResponsesProvider", prov)
	}
	caps := canonical.Capabilities()
	if !caps.SupportsPreviousResponseID || !caps.SupportsStatelessReplay {
		t.Fatalf("unexpected capabilities: %#v", caps)
	}
}

func TestRuntimeBlocksLegacyFallbackForOpenAIResponsesCanonical(t *testing.T) {
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{
			{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"},
		},
		Models: []LogicalModel{{
			ID:   "m1",
			Name: "gpt-5.4",
			API:  "openai-responses",
			Routes: []RouteCandidate{
				{ProviderRef: "p1", UpstreamModel: "gpt-5.4"},
			},
		}},
	}, func(_ context.Context, _ ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		return &scriptedProvider{}, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	_, err = rt.CompleteCanonical(context.Background(), &CanonicalRequest{
		ModelID: "m1",
		Params: &ai.ProviderRequest{
			Context: ai.ConversationContext{
				Messages: []ai.ConversationMessage{{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "ping",
					}},
				}},
			},
			Config: ai.GenerationConfig{Model: "gpt-5.4"},
		},
	})
	if err != nil {
		t.Fatalf("CompleteCanonical() error = %v, want native canonical provider path", err)
	}
}

func TestRuntimeUsesNativeCanonicalProviderForOpenAIResponses(t *testing.T) {
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{
			{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"},
		},
		Models: []LogicalModel{{
			ID:   "m1",
			Name: "gpt-5.4",
			API:  "openai-responses",
			Routes: []RouteCandidate{
				{ProviderRef: "p1", UpstreamModel: "gpt-5.4"},
			},
		}},
	}, func(_ context.Context, _ ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		return &scriptedCanonicalProvider{}, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	resp, err := rt.CompleteCanonical(context.Background(), &CanonicalRequest{
		ModelID: "m1",
		Params: &ai.ProviderRequest{
			Context: ai.ConversationContext{
				Messages: []ai.ConversationMessage{{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "ping",
					}},
				}},
			},
			Config: ai.GenerationConfig{Model: "gpt-5.4"},
		},
	})
	if err != nil {
		t.Fatalf("CompleteCanonical(): %v", err)
	}
	msg, err := ai.OpMessageFromCanonical(resp.Message)
	if err != nil {
		t.Fatalf("OpMessageFromCanonical(): %v", err)
	}
	if got := msg.Content; got != "ok" {
		t.Fatalf("canonical content = %q, want ok", got)
	}
}

func TestRuntimeStream_BridgesCanonicalStreamingToolCallDeltaWithPartialState(t *testing.T) {
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{
			{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"},
		},
		Models: []LogicalModel{{
			ID:   "m1",
			Name: "gpt-5.4",
			API:  "openai-responses",
			Routes: []RouteCandidate{
				{ProviderRef: "p1", UpstreamModel: "gpt-5.4"},
			},
		}},
	}, func(_ context.Context, _ ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		return &scriptedStreamingCanonicalProvider{
			events: []ai.ProviderEvent{
				{
					Type: ai.EventCanonicalStart,
					Partial: &ai.StreamConversationMessage{
						Role: ai.RoleCanonicalAssistant,
					},
				},
				{
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
				},
				{
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
				},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}

	stream, err := rt.StreamCanonical(context.Background(), &CanonicalRequest{
		ModelID: "m1",
		Params: &ai.ProviderRequest{
			Context: ai.ConversationContext{
				Messages: []ai.ConversationMessage{{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "ping",
					}},
				}},
			},
			Config: ai.GenerationConfig{Model: "gpt-5.4"},
		},
	})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}

	var toolDelta string
	var final *ai.ProviderResponse
	for stream.Next() {
		event := stream.Event()
		switch event.Type {
		case ai.EventCanonicalToolCallDelta:
			toolDelta += event.Delta
		case ai.EventCanonicalDone:
			final = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err(): %v", err)
	}
	if toolDelta != `{"path":"/tmp/out.md"` {
		t.Fatalf("toolDelta = %q, want partial JSON delta", toolDelta)
	}
	if final == nil {
		t.Fatalf("final = %#v, want assistant content ok", final)
	}
	msg, err := ai.OpMessageFromCanonical(final.Message)
	if err != nil {
		t.Fatalf("OpMessageFromCanonical(): %v", err)
	}
	if strings.TrimSpace(msg.Content) != "ok" {
		t.Fatalf("final = %#v, want assistant content ok", final)
	}
}

func TestCanonicalProviderForEndpoint_UsesNativeCanonicalForOpenAICompletions(t *testing.T) {
	prov := &provider.Provider{}
	canonical, err := canonicalProviderForEndpoint(prov, ProviderEndpoint{ID: "p1"}, LogicalModel{ID: "m1", API: "openai-completions"})
	if err != nil {
		t.Fatalf("canonicalProviderForEndpoint(): %v", err)
	}
	if canonical != prov {
		t.Fatalf("canonical provider = %T, want %T", canonical, prov)
	}
	caps := canonical.Capabilities()
	want := ai.DefaultCapabilitiesForAPI("openai-completions")
	if caps != want {
		t.Fatalf("capabilities = %#v, want %#v", caps, want)
	}
}

func TestCanonicalProviderForEndpoint_UsesCanonicalProviderDirectly(t *testing.T) {
	prov := &scriptedProvider{}
	canonical, err := canonicalProviderForEndpoint(prov, ProviderEndpoint{ID: "p1"}, LogicalModel{ID: "m1", API: "openai-responses"})
	if err != nil {
		t.Fatalf("canonicalProviderForEndpoint(): %v", err)
	}
	if canonical != prov {
		t.Fatalf("canonical provider = %T, want %T", canonical, prov)
	}
}

func TestRuntimeProviderFactoryReusesProviderPerRouteCandidate(t *testing.T) {
	factoryCalls := 0
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"}},
		Models: []LogicalModel{{
			ID:     "m1",
			Name:   "gpt-5.4",
			API:    "openai-responses",
			Routes: []RouteCandidate{{ProviderRef: "p1", UpstreamModel: "gpt-5.4"}},
		}},
	}, func(_ context.Context, _ ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		factoryCalls++
		return &captureResponsesProvider{}, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	for i := 0; i < 2; i++ {
		_, err = rt.CompleteResponses(context.Background(), &ResponsesRequest{ModelID: "m1", ThreadID: "th_test_123", Params: &ai.ResponsesRequest{}})
		if err != nil {
			t.Fatalf("CompleteResponses() call %d: %v", i+1, err)
		}
	}
	if factoryCalls != 1 {
		t.Fatalf("factoryCalls = %d, want 1", factoryCalls)
	}
}

func TestRuntimeProviderFactoryCacheSeparatesLogicalModelsOnSameEndpoint(t *testing.T) {
	factoryCalls := 0
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"}},
		Models: []LogicalModel{
			{ID: "m1", Name: "gpt-5.4", API: "openai-responses", Routes: []RouteCandidate{{ProviderRef: "p1", UpstreamModel: "gpt-5.4"}}},
			{ID: "m2", Name: "gpt-5.3", API: "openai-responses", Routes: []RouteCandidate{{ProviderRef: "p1", UpstreamModel: "gpt-5.3"}}},
		},
	}, func(_ context.Context, _ ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		factoryCalls++
		return &captureResponsesProvider{}, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	for _, modelID := range []string{"m1", "m2", "m1"} {
		_, err = rt.CompleteResponses(context.Background(), &ResponsesRequest{ModelID: modelID, ThreadID: "th_test_123", Params: &ai.ResponsesRequest{}})
		if err != nil {
			t.Fatalf("CompleteResponses(%s): %v", modelID, err)
		}
	}
	if factoryCalls != 2 {
		t.Fatalf("factoryCalls = %d, want 2", factoryCalls)
	}
}

func TestNewSingleModelProvider_ReusesCachedProviderForSameConfig(t *testing.T) {
	resetSingleModelProviderCacheForTest()
	t.Cleanup(resetSingleModelProviderCacheForTest)

	cfg := &op.ModelConfig{
		Key:      "gateway:gpt-5.4",
		ID:       "gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "opagent-ai-gateway",
		API:      "openai-responses",
		APIKey:   "session-token",
		BaseURL:  "https://ai-gateway.openbrain.work/v1",
	}
	p1, err := NewSingleModelProvider(cfg)
	if err != nil {
		t.Fatalf("NewSingleModelProvider() first: %v", err)
	}
	p2, err := NewSingleModelProvider(cfg)
	if err != nil {
		t.Fatalf("NewSingleModelProvider() second: %v", err)
	}
	if p1 != p2 {
		t.Fatalf("provider pointers differ: %p != %p", p1, p2)
	}
	if _, ok := p1.(*provider.GatewayResponsesWSProvider); !ok {
		t.Fatalf("provider type = %T, want *provider.GatewayResponsesWSProvider", p1)
	}
}

func TestNewSingleModelProvider_UsesGatewayCanonicalProviderForGatewayChatModel(t *testing.T) {
	resetSingleModelProviderCacheForTest()
	t.Cleanup(resetSingleModelProviderCacheForTest)

	prov, err := NewSingleModelProvider(&op.ModelConfig{
		Key:      "opagent:kimi-k2.6",
		ID:       "kimi-k2.6",
		Name:     "kimi-k2.6",
		Provider: "opagent-ai-gateway",
		API:      "openai-completions",
		APIKey:   "session-token",
		BaseURL:  "https://ai-gateway.openbrain.work/v1",
	})
	if err != nil {
		t.Fatalf("NewSingleModelProvider(): %v", err)
	}
	if _, ok := prov.(*provider.GatewayCanonicalWSProvider); !ok {
		t.Fatalf("provider type = %T, want *provider.GatewayCanonicalWSProvider", prov)
	}
}

func TestNewSingleModelProvider_ReusesGatewayWebsocketSessionAcrossProviders(t *testing.T) {
	resetSingleModelProviderCacheForTest()
	t.Cleanup(resetSingleModelProviderCacheForTest)

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

	cfg := &op.ModelConfig{
		Key:      "gateway:gpt-5.4",
		ID:       "gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "opagent-ai-gateway",
		API:      "openai-responses",
		APIKey:   "session-token",
		BaseURL:  server.URL + "/v1",
	}
	p1, err := NewSingleModelProvider(cfg)
	if err != nil {
		t.Fatalf("NewSingleModelProvider() first: %v", err)
	}
	p2, err := NewSingleModelProvider(cfg)
	if err != nil {
		t.Fatalf("NewSingleModelProvider() second: %v", err)
	}
	responses1, ok := p1.(ai.ResponsesProvider)
	if !ok {
		t.Fatalf("provider1 type %T does not implement ai.ResponsesProvider", p1)
	}
	responses2, ok := p2.(ai.ResponsesProvider)
	if !ok {
		t.Fatalf("provider2 type %T does not implement ai.ResponsesProvider", p2)
	}
	req := &ai.ResponsesRequest{
		Model:          "gpt-5.4",
		PromptCacheKey: "th_test_123",
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)),
		},
	}
	completed1 := collectCompletedResponsesFromRuntimeStream(t, mustRuntimeResponsesStream(t, responses1, req))
	completed2 := collectCompletedResponsesFromRuntimeStream(t, mustRuntimeResponsesStream(t, responses2, req))
	if completed1 == nil || completed1.ID != "resp_ws_1" {
		t.Fatalf("completed1 = %#v, want resp_ws_1", completed1)
	}
	if completed2 == nil || completed2.ID != "resp_ws_2" {
		t.Fatalf("completed2 = %#v, want resp_ws_2", completed2)
	}
	if got := upgrades.Load(); got != 1 {
		t.Fatalf("websocket upgrades = %d, want 1", got)
	}
}

func TestRuntimeCompleteResponses_DefaultsPromptCacheKeyFromThreadID(t *testing.T) {
	capture := &captureResponsesProvider{}
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"}},
		Models: []LogicalModel{{
			ID:     "m1",
			Name:   "gpt-5.4",
			API:    "openai-responses",
			Routes: []RouteCandidate{{ProviderRef: "p1", UpstreamModel: "gpt-5.4"}},
		}},
	}, func(_ context.Context, _ ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		return capture, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	_, err = rt.CompleteResponses(context.Background(), &ResponsesRequest{
		ModelID:   "m1",
		ThreadID:  "th_test_123",
		SessionID: "sess_test_456",
		Params:    &ai.ResponsesRequest{},
	})
	if err != nil {
		t.Fatalf("CompleteResponses(): %v", err)
	}
	if capture.lastRequest == nil {
		t.Fatal("capture.lastRequest is nil")
	}
	if got := capture.lastRequest.PromptCacheKey; got != "th_test_123" {
		t.Fatalf("PromptCacheKey = %q, want th_test_123", got)
	}
}

func TestRuntimeCompleteCanonical_DefaultsPromptCacheKeyFromThreadID(t *testing.T) {
	capture := &captureCanonicalProvider{}
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"}},
		Models: []LogicalModel{{
			ID:     "m1",
			Name:   "gpt-5.4",
			API:    "openai-responses",
			Routes: []RouteCandidate{{ProviderRef: "p1", UpstreamModel: "gpt-5.4"}},
		}},
	}, func(_ context.Context, _ ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		return capture, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	_, err = rt.CompleteCanonical(context.Background(), &CanonicalRequest{
		ModelID:   "m1",
		ThreadID:  "th_test_123",
		SessionID: "sess_test_456",
		Params: &ai.ProviderRequest{
			Context: ai.ConversationContext{},
			Config:  ai.GenerationConfig{},
		},
	})
	if err != nil {
		t.Fatalf("CompleteCanonical(): %v", err)
	}
	if capture.lastRequest == nil {
		t.Fatal("capture.lastRequest is nil")
	}
	if got := capture.lastRequest.Config.PromptCacheKey; got != "th_test_123" {
		t.Fatalf("PromptCacheKey = %q, want th_test_123", got)
	}
}

func TestRuntimeCompleteResponses_DefaultsPromptCacheKeyFromSessionIDWhenThreadMissing(t *testing.T) {
	capture := &captureResponsesProvider{}
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{{ID: "p1", Provider: "openai", BaseURL: "https://one.example/v1", APIKey: "k1"}},
		Models: []LogicalModel{{
			ID:     "m1",
			Name:   "gpt-5.4",
			API:    "openai-responses",
			Routes: []RouteCandidate{{ProviderRef: "p1", UpstreamModel: "gpt-5.4"}},
		}},
	}, func(_ context.Context, _ ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		return capture, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}
	_, err = rt.CompleteResponses(context.Background(), &ResponsesRequest{
		ModelID:   "m1",
		SessionID: "sess_test_456",
		Params:    &ai.ResponsesRequest{},
	})
	if err != nil {
		t.Fatalf("CompleteResponses(): %v", err)
	}
	if capture.lastRequest == nil {
		t.Fatal("capture.lastRequest is nil")
	}
	if got := capture.lastRequest.PromptCacheKey; got != "sess_test_456" {
		t.Fatalf("PromptCacheKey = %q, want sess_test_456", got)
	}
}

func TestRuntimeStreamCanonical_SemanticHandoffDropsCrossProviderThinkingForAnthropic(t *testing.T) {
	capture := &captureCanonicalProvider{}
	rt, err := NewWithFactory(Config{
		Providers: []ProviderEndpoint{{ID: "p1", Provider: "anthropic", BaseURL: "https://one.example/v1", APIKey: "k1"}},
		Models: []LogicalModel{{
			ID:     "m1",
			Name:   "claude-opus-4-6",
			API:    "anthropic-messages",
			Routes: []RouteCandidate{{ProviderRef: "p1", UpstreamModel: "claude-opus-4-6"}},
		}},
	}, func(_ context.Context, _ ProviderEndpoint, _ LogicalModel, _ RouteCandidate, _ *http.Client) (APIProvider, error) {
		return capture, nil
	})
	if err != nil {
		t.Fatalf("NewWithFactory(): %v", err)
	}

	stream, err := rt.StreamCanonical(context.Background(), &CanonicalRequest{
		ModelID: "m1",
		Params: &ai.ProviderRequest{
			Context: ai.ConversationContext{
				Messages: []ai.ConversationMessage{{
					Role: ai.RoleCanonicalAssistant,
					ProviderState: &ai.ProviderState{
						API: "openai-responses",
					},
					Content: []ai.ContentBlock{{
						Type:              ai.BlockThinking,
						Text:              "reasoning",
						ThinkingSignature: "rs_test_reasoning",
						EncryptedContent:  "enc_reasoning",
					}},
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("StreamCanonical(): %v", err)
	}
	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err(): %v", err)
	}
	if capture.lastRequest == nil {
		t.Fatal("capture.lastRequest is nil")
	}
	if got := len(capture.lastRequest.Context.Messages); got != 0 {
		t.Fatalf("messages = %d, want cross-provider thinking-only assistant dropped", got)
	}
}

func mustRuntimeResponsesStream(t *testing.T, provider ai.ResponsesProvider, req *ai.ResponsesRequest) *ai.ResponsesEventStream {
	t.Helper()
	stream, err := provider.StreamResponses(context.Background(), cloneResponsesRequest(req))
	if err != nil {
		t.Fatalf("StreamResponses(): %v", err)
	}
	return stream
}

func collectCompletedResponsesFromRuntimeStream(t *testing.T, stream *ai.ResponsesEventStream) *ai.ResponsesResult {
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

func TestIsRetryableError_UsesStructuredRetryError(t *testing.T) {
	err := ai.WrapRetryError(errors.New("response failed without details"), 0, "", "response failed without details", 1500)
	if !isRetryableError(err) {
		t.Fatalf("isRetryableError(%v) = false, want true", err)
	}
}
