package ai

import (
	"context"
	"encoding/json"
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
	events  chan ResponsesStreamEvent
	current ResponsesStreamEvent
	closed  bool
	err     error
}

func NewResponsesEventStream(buffer int) *ResponsesEventStream {
	if buffer < 0 {
		buffer = 0
	}
	return &ResponsesEventStream{events: make(chan ResponsesStreamEvent, buffer)}
}

func (s *ResponsesEventStream) Next() bool {
	event, ok := <-s.events
	if !ok {
		return false
	}
	s.current = event
	if event.Error != nil && s.err == nil {
		s.err = event.Error
	}
	return true
}

func (s *ResponsesEventStream) Event() ResponsesStreamEvent {
	return s.current
}

func (s *ResponsesEventStream) Err() error {
	return s.err
}

func (s *ResponsesEventStream) Emit(event ResponsesStreamEvent) bool {
	if s.closed {
		return false
	}
	s.events <- event
	return true
}

func (s *ResponsesEventStream) Finish(err error) {
	if err != nil && s.err == nil {
		s.err = err
		_ = s.Emit(ResponsesStreamEvent{Type: "response.failed", Error: err})
	}
	s.Close()
}

func (s *ResponsesEventStream) Close() {
	if s.closed {
		return
	}
	s.closed = true
	close(s.events)
}
