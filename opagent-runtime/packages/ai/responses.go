package ai

import (
	"context"
	"encoding/json"
	"sync"
)

// ResponsesProvider is the native Responses API surface used when callers need
// item-level protocol semantics instead of the canonical conversation surface.
type ResponsesProvider interface {
	CompleteResponses(ctx context.Context, req *ResponsesRequest) (*ResponsesResult, error)
	StreamResponses(ctx context.Context, req *ResponsesRequest) (*ResponsesEventStream, error)
}

type ResponsesRequest struct {
	Model              string
	Instructions       string
	PreviousResponseID string
	RequestID          string
	ServiceTier        string
	Input              []ResponseItem
	Tools              []ResponseTool
	ToolChoice         json.RawMessage
	ParallelToolCalls  *bool
	Reasoning          *ResponsesReasoning
	Store              *bool
	Stream             bool
	Include            []string
	PromptCacheKey     string
	Text               *ResponsesTextConfig
	Temperature        *float64
	MaxOutputTokens    *int64
}

type ResponsesReasoning struct {
	Effort  string
	Summary string
}

type ResponsesTextConfig struct {
	Verbosity string
	FormatRaw json.RawMessage
}

type ResponseTool struct {
	Type        string
	Name        string
	Description string
	Parameters  any
	Strict      *bool
	Raw         json.RawMessage
}

type ResponseItem struct {
	Type             string
	Role             string
	ID               string
	Status           string
	CallID           string
	Name             string
	Arguments        string
	Content          []ResponseContentPart
	Summary          []ResponseSummaryPart
	EncryptedContent string
	OutputText       string
	OutputContent    []ResponseContentPart
	Raw              json.RawMessage
}

type ResponseContentPart struct {
	Type     string
	Text     string
	ImageURL string
	Detail   string
}

type ResponseSummaryPart struct {
	Type string
	Text string
}

type ResponsesResult struct {
	ID          string
	ProviderRef string
	Model       string
	Status      string
	Output      []ResponseItem
	Usage       Usage
	StopReason  StopReason
}

type ResponsesStreamEvent struct {
	Type     string
	Delta    string
	Item     *ResponseItem
	Response *ResponsesResult
	Error    error
	Raw      json.RawMessage
}

type ResponsesEventStream struct {
	events    chan ResponsesStreamEvent
	done      chan struct{}
	closeOnce sync.Once
	errMu     sync.RWMutex
	current   ResponsesStreamEvent
	err       error
}

func NewResponsesEventStream(buffer int) *ResponsesEventStream {
	if buffer < 0 {
		buffer = 0
	}
	return &ResponsesEventStream{
		events: make(chan ResponsesStreamEvent, buffer),
		done:   make(chan struct{}),
	}
}

func (s *ResponsesEventStream) Next() bool {
	select {
	case event := <-s.events:
		s.setCurrent(event)
		return true
	default:
	}
	select {
	case event := <-s.events:
		s.setCurrent(event)
		return true
	case <-s.done:
		select {
		case event := <-s.events:
			s.setCurrent(event)
			return true
		default:
			return false
		}
	}
}

func (s *ResponsesEventStream) Event() ResponsesStreamEvent {
	return s.current
}

func (s *ResponsesEventStream) Err() error {
	s.errMu.RLock()
	defer s.errMu.RUnlock()
	return s.err
}

func (s *ResponsesEventStream) Emit(event ResponsesStreamEvent) bool {
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case <-s.done:
		return false
	case s.events <- event:
		return true
	}
}

func (s *ResponsesEventStream) Finish(err error) {
	if err != nil {
		s.setError(err)
		_ = s.Emit(ResponsesStreamEvent{Type: "response.failed", Error: err})
	}
	s.Close()
}

func (s *ResponsesEventStream) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
	})
}

func (s *ResponsesEventStream) setCurrent(event ResponsesStreamEvent) {
	s.current = event
	if event.Error != nil {
		s.setError(event.Error)
	}
}

func (s *ResponsesEventStream) setError(err error) {
	if err == nil {
		return
	}
	s.errMu.Lock()
	defer s.errMu.Unlock()
	if s.err == nil {
		s.err = err
	}
}
