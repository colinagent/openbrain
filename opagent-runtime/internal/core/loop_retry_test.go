package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type retrySequenceCanonicalProvider struct {
	errs      []error
	responses []*ai.ProviderResponse
	calls     int
}

type promptCacheCaptureCanonicalProvider struct {
	promptCacheKeys []string
}

type streamOnlyErrorCanonicalProvider struct {
	streamErr     error
	streamCalls   int
	completeCalls int
}

type partialStreamCanonicalProvider struct {
	events []ai.ProviderEvent
	err    error
}

func (p *retrySequenceCanonicalProvider) Capabilities() ai.ProviderCapabilities {
	return ai.ProviderCapabilities{
		SupportsThinkingBlocks:     true,
		SupportsToolCalls:          true,
		SupportsParallelToolCalls:  true,
		SupportsImages:             true,
		SupportsStatelessReplay:    true,
		SupportsPreviousResponseID: true,
		SupportsCompaction:         true,
	}
}

func (p *retrySequenceCanonicalProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return nil, errors.New("unexpected CompleteCanonical call")
}

func (p *retrySequenceCanonicalProvider) StreamCanonical(_ context.Context, _ *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	index := p.calls
	p.calls++

	stream := ai.NewProviderEventStream(2)
	go func() {
		if index < len(p.errs) && p.errs[index] != nil {
			stream.Finish(p.errs[index])
			return
		}
		if index >= len(p.responses) || p.responses[index] == nil {
			stream.Finish(errors.New("missing scripted response"))
			return
		}
		_ = stream.Emit(ai.ProviderEvent{
			Type:     ai.EventCanonicalDone,
			Response: ai.CloneProviderResponsePtr(p.responses[index]),
		})
		stream.Close()
	}()
	return stream, nil
}

func (p *streamOnlyErrorCanonicalProvider) Capabilities() ai.ProviderCapabilities {
	return ai.ProviderCapabilities{
		SupportsThinkingBlocks:     true,
		SupportsToolCalls:          true,
		SupportsParallelToolCalls:  true,
		SupportsImages:             true,
		SupportsStatelessReplay:    true,
		SupportsPreviousResponseID: true,
		SupportsCompaction:         true,
	}
}

func (p *streamOnlyErrorCanonicalProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	p.completeCalls++
	return assistantProviderResponse("unexpected-complete"), nil
}

func (p *streamOnlyErrorCanonicalProvider) StreamCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	p.streamCalls++
	return nil, p.streamErr
}

func (p *partialStreamCanonicalProvider) Capabilities() ai.ProviderCapabilities {
	return ai.ProviderCapabilities{
		SupportsThinkingBlocks:     true,
		SupportsToolCalls:          true,
		SupportsParallelToolCalls:  true,
		SupportsImages:             true,
		SupportsStatelessReplay:    true,
		SupportsPreviousResponseID: true,
		SupportsCompaction:         true,
	}
}

func (p *partialStreamCanonicalProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return nil, errors.New("unexpected CompleteCanonical call")
}

func (p *partialStreamCanonicalProvider) StreamCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	stream := ai.NewProviderEventStream(len(p.events) + 1)
	go func() {
		for _, event := range p.events {
			_ = stream.Emit(event)
		}
		stream.Finish(p.err)
	}()
	return stream, nil
}

func (p *promptCacheCaptureCanonicalProvider) Capabilities() ai.ProviderCapabilities {
	return ai.ProviderCapabilities{
		SupportsThinkingBlocks:     true,
		SupportsToolCalls:          true,
		SupportsParallelToolCalls:  true,
		SupportsImages:             true,
		SupportsStatelessReplay:    true,
		SupportsPreviousResponseID: true,
		SupportsCompaction:         true,
	}
}

func (p *promptCacheCaptureCanonicalProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return nil, errors.New("unexpected CompleteCanonical call")
}

func (p *promptCacheCaptureCanonicalProvider) StreamCanonical(_ context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req != nil {
		p.promptCacheKeys = append(p.promptCacheKeys, strings.TrimSpace(req.Config.PromptCacheKey))
	} else {
		p.promptCacheKeys = append(p.promptCacheKeys, "")
	}
	stream := ai.NewProviderEventStream(1)
	go func() {
		_ = stream.Emit(ai.ProviderEvent{Type: ai.EventCanonicalDone, Response: assistantProviderResponse("ok")})
		stream.Close()
	}()
	return stream, nil
}

func decodeRetryStartPayload(t *testing.T, content op.Content) autoRetryStartPayload {
	t.Helper()
	jsonContent, ok := content.(*op.JsonContent)
	if !ok {
		t.Fatalf("expected json content, got %T", content)
	}
	var payload autoRetryStartPayload
	if err := json.Unmarshal(jsonContent.Raw, &payload); err != nil {
		t.Fatalf("unmarshal auto_retry_start payload: %v", err)
	}
	return payload
}

func decodeRetryEndPayload(t *testing.T, content op.Content) autoRetryEndPayload {
	t.Helper()
	jsonContent, ok := content.(*op.JsonContent)
	if !ok {
		t.Fatalf("expected json content, got %T", content)
	}
	var payload autoRetryEndPayload
	if err := json.Unmarshal(jsonContent.Raw, &payload); err != nil {
		t.Fatalf("unmarshal auto_retry_end payload: %v", err)
	}
	return payload
}

func assistantProviderResponse(text string) *ai.ProviderResponse {
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

func partialTextEvents(text string) []ai.ProviderEvent {
	partial := &ai.StreamConversationMessage{
		Role: ai.RoleCanonicalAssistant,
		Content: []ai.StreamContentBlock{{
			Type: ai.BlockText,
			Text: text,
		}},
	}
	return []ai.ProviderEvent{
		{
			Type:    ai.EventCanonicalStart,
			Partial: ai.CloneStreamConversationMessagePtr(partial),
		},
		{
			Type:         ai.EventCanonicalTextStart,
			ContentIndex: 0,
			Block:        &partial.Content[0],
			Partial:      ai.CloneStreamConversationMessagePtr(partial),
		},
		{
			Type:         ai.EventCanonicalTextDelta,
			ContentIndex: 0,
			Delta:        text,
			Block:        &partial.Content[0],
			Partial:      ai.CloneStreamConversationMessagePtr(partial),
		},
		{
			Type:         ai.EventCanonicalTextEnd,
			ContentIndex: 0,
			Content:      text,
			Block:        &partial.Content[0],
			Partial:      ai.CloneStreamConversationMessagePtr(partial),
		},
	}
}

func partialThinkingEvents(thinking string) []ai.ProviderEvent {
	partial := &ai.StreamConversationMessage{
		Role: ai.RoleCanonicalAssistant,
		Content: []ai.StreamContentBlock{{
			Type: ai.BlockThinking,
			Text: thinking,
		}},
	}
	return []ai.ProviderEvent{
		{
			Type:    ai.EventCanonicalStart,
			Partial: ai.CloneStreamConversationMessagePtr(partial),
		},
		{
			Type:         ai.EventCanonicalThinkingStart,
			ContentIndex: 0,
			Block:        &partial.Content[0],
			Partial:      ai.CloneStreamConversationMessagePtr(partial),
		},
		{
			Type:         ai.EventCanonicalThinkingDelta,
			ContentIndex: 0,
			Delta:        thinking,
			Block:        &partial.Content[0],
			Partial:      ai.CloneStreamConversationMessagePtr(partial),
		},
		{
			Type:         ai.EventCanonicalThinkingEnd,
			ContentIndex: 0,
			Content:      thinking,
			Block:        &partial.Content[0],
			Partial:      ai.CloneStreamConversationMessagePtr(partial),
		},
	}
}

func partialTextThinkingEvents(text, thinking string) []ai.ProviderEvent {
	partial := &ai.StreamConversationMessage{
		Role: ai.RoleCanonicalAssistant,
		Content: []ai.StreamContentBlock{
			{
				Type: ai.BlockText,
				Text: text,
			},
			{
				Type:                ai.BlockThinking,
				Text:                thinking,
				ThinkingReplayField: "reasoning_content",
				ThinkingSignature:   "sig_1",
			},
		},
	}
	return []ai.ProviderEvent{
		{
			Type:    ai.EventCanonicalStart,
			Partial: ai.CloneStreamConversationMessagePtr(partial),
		},
		{
			Type:         ai.EventCanonicalTextEnd,
			ContentIndex: 0,
			Content:      text,
			Block:        &partial.Content[0],
			Partial:      ai.CloneStreamConversationMessagePtr(partial),
		},
		{
			Type:         ai.EventCanonicalThinkingEnd,
			ContentIndex: 1,
			Content:      thinking,
			Block:        &partial.Content[1],
			Partial:      ai.CloneStreamConversationMessagePtr(partial),
		},
	}
}

func newRetryTestLoop(provider ai.CanonicalProvider) *AgentLoop {
	return &AgentLoop{
		Ctx:  context.Background(),
		Meta: op.Meta{"threadID": "thread-retry-test"},
		Agent: &Agent{
			ToolSpecs: map[string]*op.ToolSpec{},
		},
		Model: &ModelClient{
			config: &op.ModelConfig{
				ID:       "gpt-5.4",
				Name:     "gpt-5.4",
				Provider: "opagent-ai-gateway",
				API:      "openai-responses",
			},
			Canonical: provider,
		},
		canonicalHistory: []ai.ConversationMessage{{
			Role: ai.RoleCanonicalUser,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "hello",
			}},
		}},
	}
}

func TestStreamAssistantTurnResultWithRetry_RetriesTransientFailure(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	prevMaxRetries := autoRetryMaxRetries
	prevBaseDelay := autoRetryBaseDelay
	prevMaxDelay := autoRetryMaxDelay
	autoRetryMaxRetries = 3
	autoRetryBaseDelay = time.Millisecond
	autoRetryMaxDelay = time.Second
	t.Cleanup(func() {
		autoRetryMaxRetries = prevMaxRetries
		autoRetryBaseDelay = prevBaseDelay
		autoRetryMaxDelay = prevMaxDelay
	})

	provider := &retrySequenceCanonicalProvider{
		errs: []error{
			ai.WrapRetryError(errors.New("upstream server_error"), 503, "server_error", "upstream server_error", 0),
			nil,
		},
		responses: []*ai.ProviderResponse{
			nil,
			assistantProviderResponse("ok"),
		},
	}
	loop := newRetryTestLoop(provider)

	result, err := loop.streamAssistantTurnResultWithRetry()
	if err != nil {
		t.Fatalf("streamAssistantTurnResultWithRetry(): %v", err)
	}
	if got := provider.calls; got != 2 {
		t.Fatalf("provider calls = %d, want 2", got)
	}
	if got := result.message.Content; got != "ok" {
		t.Fatalf("assistant content = %q, want ok", got)
	}

	events := drainNotifyMessagesAfter(t, 50*time.Millisecond)
	var startPayload *autoRetryStartPayload
	var endPayload *autoRetryEndPayload
	for _, event := range events {
		typ, _ := event.Meta["type"].(string)
		switch typ {
		case "auto_retry_start":
			payload := decodeRetryStartPayload(t, event.Content)
			startPayload = &payload
		case "auto_retry_end":
			payload := decodeRetryEndPayload(t, event.Content)
			endPayload = &payload
		}
	}
	if startPayload == nil {
		t.Fatal("expected auto_retry_start event")
	}
	if startPayload.Attempt != 1 || startPayload.MaxAttempts != 3 {
		t.Fatalf("auto_retry_start = %+v, want attempt=1 maxAttempts=3", *startPayload)
	}
	if startPayload.DelayMs <= 0 {
		t.Fatalf("auto_retry_start delay = %d, want > 0", startPayload.DelayMs)
	}
	if endPayload == nil {
		t.Fatal("expected auto_retry_end event")
	}
	if !endPayload.Success || endPayload.Attempt != 1 {
		t.Fatalf("auto_retry_end = %+v, want success after attempt 1", *endPayload)
	}
}

func TestStreamAssistantTurnResultWithRetry_RebuildsModelAfterNoProvidersArtifact(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	initialProvider := &retrySequenceCanonicalProvider{
		errs: []error{
			ai.WrapRetryError(errors.New("model gpt-5.4 has no available providers"), 0, "", "model gpt-5.4 has no available providers", 0),
		},
	}
	rebuiltProvider := &retrySequenceCanonicalProvider{
		responses: []*ai.ProviderResponse{assistantProviderResponse("ok")},
	}
	loop := newRetryTestLoop(initialProvider)
	loop.rebuildModel = func(context.Context, op.Meta) (*ModelClient, error) {
		return &ModelClient{
			config:    loop.Model.config,
			Canonical: rebuiltProvider,
		}, nil
	}

	result, err := loop.streamAssistantTurnResultWithRetry()
	if err != nil {
		t.Fatalf("streamAssistantTurnResultWithRetry(): %v", err)
	}
	if got := result.message.Content; got != "ok" {
		t.Fatalf("assistant content = %q, want ok", got)
	}
	if got := initialProvider.calls; got != 1 {
		t.Fatalf("initial provider calls = %d, want 1", got)
	}
	if got := rebuiltProvider.calls; got != 1 {
		t.Fatalf("rebuilt provider calls = %d, want 1", got)
	}

	events := drainNotifyMessagesAfter(t, 50*time.Millisecond)
	for _, event := range events {
		typ, _ := event.Meta["type"].(string)
		if typ == "auto_retry_start" || typ == "auto_retry_end" {
			t.Fatalf("unexpected retry lifecycle event %q", typ)
		}
	}
}

func TestStreamAssistantTurnResult_DoesNotFallbackToCompleteOnStreamOpenError(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &streamOnlyErrorCanonicalProvider{streamErr: errors.New("stream open failed")}
	loop := newRetryTestLoop(provider)

	_, err := loop.streamAssistantTurnResult()
	if err == nil {
		t.Fatal("expected stream open error")
	}
	if got := provider.streamCalls; got != 1 {
		t.Fatalf("stream calls = %d, want 1", got)
	}
	if got := provider.completeCalls; got != 0 {
		t.Fatalf("complete calls = %d, want 0", got)
	}
}

func TestStreamAssistantTurnResult_UsesPartialTextWhenDoneIsEmpty(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &partialStreamCanonicalProvider{
		events: append(partialTextEvents("visible final text"), ai.ProviderEvent{
			Type:     ai.EventCanonicalDone,
			Response: &ai.ProviderResponse{},
		}),
	}
	loop := newRetryTestLoop(provider)

	result, err := loop.streamAssistantTurnResult()
	if err != nil {
		t.Fatalf("streamAssistantTurnResult(): %v", err)
	}
	if got := result.message.Content; got != "visible final text" {
		t.Fatalf("assistant content = %q, want partial text", got)
	}
	if got := result.message.StopReason; got != op.StopReasonStop {
		t.Fatalf("stop reason = %q, want %q", got, op.StopReasonStop)
	}
}

func TestStreamAssistantTurnResult_MergesPartialTextBeforeFinalToolCall(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &partialStreamCanonicalProvider{
		events: append(partialTextEvents("visible final text"), ai.ProviderEvent{
			Type: ai.EventCanonicalDone,
			Response: &ai.ProviderResponse{
				Message: ai.ConversationMessage{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{{
						Type: ai.BlockToolCall,
						ToolCall: &ai.CanonicalToolCall{
							ID:           "call_1",
							Name:         "read",
							RawArguments: `{"path":"AGENTS.md"}`,
							Arguments:    map[string]any{"path": "AGENTS.md"},
						},
					}},
				},
				StopReason: ai.StopReasonToolUse,
			},
		}),
	}
	loop := newRetryTestLoop(provider)

	result, err := loop.streamAssistantTurnResult()
	if err != nil {
		t.Fatalf("streamAssistantTurnResult(): %v", err)
	}
	if got := result.message.Content; got != "visible final text" {
		t.Fatalf("assistant content = %q, want partial text merged", got)
	}
	if len(result.message.ToolCalls) != 1 || result.message.ToolCalls[0].Name != "read" {
		t.Fatalf("tool calls = %#v, want read call preserved", result.message.ToolCalls)
	}
	if len(result.canonical.Content) != 2 || result.canonical.Content[0].Type != ai.BlockText || result.canonical.Content[1].Type != ai.BlockToolCall {
		t.Fatalf("canonical content order = %#v, want text before tool call", result.canonical.Content)
	}
	if got := result.message.StopReason; got != op.StopReasonToolUse {
		t.Fatalf("stop reason = %q, want %q", got, op.StopReasonToolUse)
	}
}

func TestStreamAssistantTurnResult_DoesNotMergeWhenFinalHasText(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &partialStreamCanonicalProvider{
		events: append(partialTextEvents("partial text"), ai.ProviderEvent{
			Type: ai.EventCanonicalDone,
			Response: &ai.ProviderResponse{
				Message: ai.ConversationMessage{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "authoritative final text",
					}},
				},
				StopReason: ai.StopReasonStop,
			},
		}),
	}
	loop := newRetryTestLoop(provider)

	result, err := loop.streamAssistantTurnResult()
	if err != nil {
		t.Fatalf("streamAssistantTurnResult(): %v", err)
	}
	if got := result.message.Content; got != "authoritative final text" {
		t.Fatalf("assistant content = %q, want final text unchanged", got)
	}
}

func TestStreamAssistantTurnResult_PreservesPartialTextOnCanceledStream(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &partialStreamCanonicalProvider{
		events: partialTextEvents("visible before cancel"),
		err:    context.Canceled,
	}
	loop := newRetryTestLoop(provider)

	result, err := loop.streamAssistantTurnResult()
	if err != nil {
		t.Fatalf("streamAssistantTurnResult(): %v", err)
	}
	if got := result.message.Content; got != "visible before cancel" {
		t.Fatalf("assistant content = %q, want partial text", got)
	}
	if got := result.message.StopReason; got != op.StopReasonAborted {
		t.Fatalf("stop reason = %q, want %q", got, op.StopReasonAborted)
	}
}

func TestStreamAssistantTurnResult_PreservesPartialTextAndThinkingOnStreamError(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &partialStreamCanonicalProvider{
		events: partialTextThinkingEvents("visible before error", "private reasoning"),
		err:    errors.New("upstream 503"),
	}
	loop := newRetryTestLoop(provider)

	result, err := loop.streamAssistantTurnResult()
	if err != nil {
		t.Fatalf("streamAssistantTurnResult(): %v", err)
	}
	if got := result.message.Content; got != "visible before error" {
		t.Fatalf("assistant content = %q, want partial text", got)
	}
	if got := result.message.ReasoningContent; got != "private reasoning" {
		t.Fatalf("reasoning content = %q, want partial thinking", got)
	}
	if result.message.ReasoningReplayField != "reasoning_content" || result.message.ReasoningSignature != "sig_1" {
		t.Fatalf("reasoning metadata = (%q, %q), want replay field/signature", result.message.ReasoningReplayField, result.message.ReasoningSignature)
	}
	if got := result.message.StopReason; got != op.StopReasonError {
		t.Fatalf("stop reason = %q, want %q", got, op.StopReasonError)
	}
	if !strings.Contains(string(result.canonical.Raw), `"errorMessage":"upstream 503"`) {
		t.Fatalf("canonical raw = %s, want errorMessage", result.canonical.Raw)
	}
}

func TestStreamAssistantTurnResult_PreservesThinkingOnlyOnCanceledStream(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &partialStreamCanonicalProvider{
		events: partialThinkingEvents("thinking before cancel"),
		err:    context.Canceled,
	}
	loop := newRetryTestLoop(provider)

	result, err := loop.streamAssistantTurnResult()
	if err != nil {
		t.Fatalf("streamAssistantTurnResult(): %v", err)
	}
	if got := result.message.Content; got != "" {
		t.Fatalf("assistant content = %q, want empty visible text", got)
	}
	if got := result.message.ReasoningContent; got != "thinking before cancel" {
		t.Fatalf("reasoning content = %q, want partial thinking", got)
	}
	if got := result.message.StopReason; got != op.StopReasonAborted {
		t.Fatalf("stop reason = %q, want %q", got, op.StopReasonAborted)
	}
	if len(result.canonical.Content) != 1 || result.canonical.Content[0].Type != ai.BlockThinking {
		t.Fatalf("canonical content = %#v, want one thinking block", result.canonical.Content)
	}
}

// TestStreamAssistantTurnResult_AbortsWhenTurnContextCancelledEvenWithWrappedStreamError
// models the production gateway websocket path: context cancellation surfaces as a
// wrapped transport error that no longer satisfies errors.Is(err, context.Canceled),
// while the turn context itself is cancelled. The fallback must still be labeled aborted.
func TestStreamAssistantTurnResult_AbortsWhenTurnContextCancelledEvenWithWrappedStreamError(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	ctx, cancel := context.WithCancel(context.Background())
	provider := &partialStreamCanonicalProvider{
		events: partialTextEvents("visible before cancel"),
		err:    errors.New("read canonical websocket event: websocket: close 1006 abnormal closure"),
	}
	loop := newRetryTestLoop(provider)
	loop.Ctx = ctx
	cancel()

	result, err := loop.streamAssistantTurnResult()
	if err != nil {
		t.Fatalf("streamAssistantTurnResult(): %v", err)
	}
	if got := result.message.Content; got != "visible before cancel" {
		t.Fatalf("assistant content = %q, want partial text", got)
	}
	if got := result.message.StopReason; got != op.StopReasonAborted {
		t.Fatalf("stop reason = %q, want %q (turn context cancelled should override wrapped stream error)", got, op.StopReasonAborted)
	}
}

func TestStreamAssistantTurnResult_DefaultsPromptCacheKeyFromThreadID(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &promptCacheCaptureCanonicalProvider{}
	loop := newRetryTestLoop(provider)
	loop.ThreadID = "thread-retry-test"

	result, err := loop.streamAssistantTurnResult()
	if err != nil {
		t.Fatalf("streamAssistantTurnResult(): %v", err)
	}
	if got := result.message.Content; got != "ok" {
		t.Fatalf("assistant content = %q, want ok", got)
	}
	if got := provider.promptCacheKeys; len(got) != 1 || got[0] != "thread-retry-test" {
		t.Fatalf("promptCacheKeys = %#v, want [thread-retry-test]", got)
	}
}

func TestStreamAssistantTurnResultWithRetry_DoesNotRetryDeterministicFailure(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &retrySequenceCanonicalProvider{
		errs: []error{
			ai.WrapRetryError(
				errors.New(`Invalid 'input[1].id': ''`),
				400,
				"invalid_request_error",
				`Invalid 'input[1].id': ''`,
				0,
			),
		},
	}
	loop := newRetryTestLoop(provider)

	if _, err := loop.streamAssistantTurnResultWithRetry(); err == nil {
		t.Fatal("expected deterministic error")
	}
	if got := provider.calls; got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}

	events := drainNotifyMessagesAfter(t, 50*time.Millisecond)
	for _, event := range events {
		typ, _ := event.Meta["type"].(string)
		if typ == "auto_retry_start" || typ == "auto_retry_end" {
			t.Fatalf("unexpected retry lifecycle event %q", typ)
		}
	}
}
