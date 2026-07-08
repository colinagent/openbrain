package ai

import (
	"context"
	"encoding/json"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type ConversationRole = op.ConversationRole

const (
	RoleCanonicalSystem     = op.RoleCanonicalSystem
	RoleCanonicalDeveloper  = op.RoleCanonicalDeveloper
	RoleCanonicalUser       = op.RoleCanonicalUser
	RoleCanonicalAssistant  = op.RoleCanonicalAssistant
	RoleCanonicalTool       = op.RoleCanonicalTool
	RoleCanonicalCompaction = op.RoleCanonicalCompaction
)

type ContentBlockType = op.ContentBlockType

const (
	BlockText       = op.BlockText
	BlockThinking   = op.BlockThinking
	BlockImage      = op.BlockImage
	BlockToolCall   = op.BlockToolCall
	BlockToolResult = op.BlockToolResult
	BlockCompaction = op.BlockCompaction
)

type ContentBlock = op.ContentBlock
type CanonicalToolCall = op.CanonicalToolCall
type CanonicalToolResult = op.CanonicalToolResult
type ConversationMessage = op.ConversationMessage

type StreamToolCall struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Arguments        map[string]any  `json:"arguments,omitempty"`
	RawArguments     string          `json:"rawArguments,omitempty"`
	ThoughtSignature string          `json:"thoughtSignature,omitempty"`
	Complete         bool            `json:"complete,omitempty"`
	Raw              json.RawMessage `json:"raw,omitempty"`
}

type StreamContentBlock struct {
	Type                ContentBlockType `json:"type"`
	Text                string           `json:"text,omitempty"`
	MimeType            string           `json:"mimeType,omitempty"`
	ImageData           string           `json:"imageData,omitempty"`
	TextSignature       string           `json:"textSignature,omitempty"`
	ThinkingReplayField string           `json:"thinkingReplayField,omitempty"`
	ThinkingSignature   string           `json:"thinkingSignature,omitempty"`
	ToolCall            *StreamToolCall  `json:"toolCall,omitempty"`
	EncryptedContent    string           `json:"encryptedContent,omitempty"`
	Raw                 json.RawMessage  `json:"raw,omitempty"`
}

type StreamConversationMessage struct {
	Role          ConversationRole     `json:"role"`
	Content       []StreamContentBlock `json:"content,omitempty"`
	Timestamp     int64                `json:"timestamp,omitempty"`
	ProviderState *ProviderState       `json:"providerState,omitempty"`
	Raw           json.RawMessage      `json:"raw,omitempty"`
}

type ProviderState = op.ProviderState

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  any             `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
	Raw         json.RawMessage `json:"raw,omitempty"`
}

type ConversationContext struct {
	SystemPrompt string                `json:"systemPrompt,omitempty"`
	Messages     []ConversationMessage `json:"messages,omitempty"`
	Tools        []ToolDefinition      `json:"tools,omitempty"`
}

type GenerationConfig struct {
	Model             string          `json:"model,omitempty"`
	ServiceTier       string          `json:"serviceTier,omitempty"`
	MaxTokens         *int64          `json:"maxTokens,omitempty"`
	Temperature       *float64        `json:"temperature,omitempty"`
	ReasoningEffort   string          `json:"reasoningEffort,omitempty"`
	ReasoningEnabled  *bool           `json:"reasoningEnabled,omitempty"`
	ReasoningSummary  string          `json:"reasoningSummary,omitempty"`
	ToolChoice        json.RawMessage `json:"toolChoice,omitempty"`
	ParallelToolCalls *bool           `json:"parallelToolCalls,omitempty"`
	Include           []string        `json:"include,omitempty"`
	PromptCacheKey    string          `json:"promptCacheKey,omitempty"`
}

type ProviderRequest struct {
	Context            ConversationContext `json:"context"`
	Config             GenerationConfig    `json:"config"`
	PreviousResponseID string              `json:"previousResponseID,omitempty"`
	RequestID          string              `json:"requestID,omitempty"`
}

type ProviderResponse struct {
	Message    ConversationMessage `json:"message"`
	Usage      Usage               `json:"usage"`
	StopReason StopReason          `json:"stopReason,omitempty"`
}

type ProviderEventType string

const (
	EventCanonicalStart         ProviderEventType = "start"
	EventCanonicalTextStart     ProviderEventType = "text_start"
	EventCanonicalTextDelta     ProviderEventType = "text_delta"
	EventCanonicalTextEnd       ProviderEventType = "text_end"
	EventCanonicalThinkingStart ProviderEventType = "thinking_start"
	EventCanonicalThinkingDelta ProviderEventType = "thinking_delta"
	EventCanonicalThinkingEnd   ProviderEventType = "thinking_end"
	EventCanonicalToolCallStart ProviderEventType = "toolcall_start"
	EventCanonicalToolCallDelta ProviderEventType = "toolcall_delta"
	EventCanonicalToolCallEnd   ProviderEventType = "toolcall_end"
	EventCanonicalDone          ProviderEventType = "done"
	EventCanonicalError         ProviderEventType = "error"
)

type ProviderEvent struct {
	Type         ProviderEventType
	ContentIndex int
	Delta        string
	Content      string
	Block        *StreamContentBlock
	Partial      *StreamConversationMessage
	Response     *ProviderResponse
	Error        error
	Raw          json.RawMessage
}

type ProviderEventStream struct {
	events  chan ProviderEvent
	current ProviderEvent
	closed  bool
	err     error
}

func NewProviderEventStream(buffer int) *ProviderEventStream {
	if buffer < 0 {
		buffer = 0
	}
	return &ProviderEventStream{events: make(chan ProviderEvent, buffer)}
}

func (s *ProviderEventStream) Next() bool {
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

func (s *ProviderEventStream) Event() ProviderEvent {
	return s.current
}

func (s *ProviderEventStream) Err() error {
	return s.err
}

func (s *ProviderEventStream) Emit(event ProviderEvent) bool {
	if s.closed {
		return false
	}
	s.events <- cloneProviderEvent(event)
	return true
}

func (s *ProviderEventStream) Finish(err error) {
	if err != nil && s.err == nil {
		s.err = err
		_ = s.Emit(ProviderEvent{Type: EventCanonicalError, Error: err})
	}
	s.Close()
}

func (s *ProviderEventStream) Close() {
	if s.closed {
		return
	}
	s.closed = true
	close(s.events)
}

func cloneProviderEvent(event ProviderEvent) ProviderEvent {
	cloned := event
	cloned.Block = CloneStreamContentBlockPtr(event.Block)
	cloned.Partial = CloneStreamConversationMessagePtr(event.Partial)
	cloned.Response = CloneProviderResponsePtr(event.Response)
	if len(event.Raw) > 0 {
		cloned.Raw = append(json.RawMessage(nil), event.Raw...)
	}
	return cloned
}

func CloneContentBlockPtr(block *ContentBlock) *ContentBlock {
	if block == nil {
		return nil
	}
	cloned := *block
	if block.ToolCall != nil {
		toolCall := *block.ToolCall
		if len(block.ToolCall.Arguments) > 0 {
			toolCall.Arguments = make(map[string]any, len(block.ToolCall.Arguments))
			for key, value := range block.ToolCall.Arguments {
				toolCall.Arguments[key] = value
			}
		}
		if len(block.ToolCall.Raw) > 0 {
			toolCall.Raw = append(json.RawMessage(nil), block.ToolCall.Raw...)
		}
		cloned.ToolCall = &toolCall
	}
	if block.ToolResult != nil {
		toolResult := *block.ToolResult
		toolResult.OutputContent = cloneContentBlocks(block.ToolResult.OutputContent)
		if len(block.ToolResult.Raw) > 0 {
			toolResult.Raw = append(json.RawMessage(nil), block.ToolResult.Raw...)
		}
		cloned.ToolResult = &toolResult
	}
	if len(block.Raw) > 0 {
		cloned.Raw = append(json.RawMessage(nil), block.Raw...)
	}
	return &cloned
}

func CloneConversationMessagePtr(msg *ConversationMessage) *ConversationMessage {
	if msg == nil {
		return nil
	}
	cloned := *msg
	if len(msg.Content) > 0 {
		cloned.Content = make([]ContentBlock, 0, len(msg.Content))
		for _, block := range msg.Content {
			cloned.Content = append(cloned.Content, *CloneContentBlockPtr(&block))
		}
	}
	if msg.ProviderState != nil {
		providerState := *msg.ProviderState
		cloned.ProviderState = &providerState
	}
	if msg.Usage != nil {
		usage := *msg.Usage
		cloned.Usage = &usage
	}
	if len(msg.Raw) > 0 {
		cloned.Raw = append(json.RawMessage(nil), msg.Raw...)
	}
	return &cloned
}

func CloneStreamContentBlockPtr(block *StreamContentBlock) *StreamContentBlock {
	if block == nil {
		return nil
	}
	cloned := *block
	if block.ToolCall != nil {
		toolCall := *block.ToolCall
		if len(block.ToolCall.Arguments) > 0 {
			toolCall.Arguments = make(map[string]any, len(block.ToolCall.Arguments))
			for key, value := range block.ToolCall.Arguments {
				toolCall.Arguments[key] = value
			}
		}
		if len(block.ToolCall.Raw) > 0 {
			toolCall.Raw = append(json.RawMessage(nil), block.ToolCall.Raw...)
		}
		cloned.ToolCall = &toolCall
	}
	if len(block.Raw) > 0 {
		cloned.Raw = append(json.RawMessage(nil), block.Raw...)
	}
	return &cloned
}

func CloneStreamConversationMessagePtr(msg *StreamConversationMessage) *StreamConversationMessage {
	if msg == nil {
		return nil
	}
	cloned := *msg
	if len(msg.Content) > 0 {
		cloned.Content = make([]StreamContentBlock, 0, len(msg.Content))
		for _, block := range msg.Content {
			cloned.Content = append(cloned.Content, *CloneStreamContentBlockPtr(&block))
		}
	}
	if msg.ProviderState != nil {
		providerState := *msg.ProviderState
		cloned.ProviderState = &providerState
	}
	if len(msg.Raw) > 0 {
		cloned.Raw = append(json.RawMessage(nil), msg.Raw...)
	}
	return &cloned
}

func FinalizeStreamConversationMessage(partial *StreamConversationMessage) ConversationMessage {
	if partial == nil {
		return ConversationMessage{}
	}
	final := ConversationMessage{
		Role:      partial.Role,
		Timestamp: partial.Timestamp,
	}
	if partial.ProviderState != nil {
		providerState := *partial.ProviderState
		final.ProviderState = &providerState
	}
	if len(partial.Raw) > 0 {
		final.Raw = append(json.RawMessage(nil), partial.Raw...)
	}
	if len(partial.Content) == 0 {
		return final
	}
	final.Content = make([]ContentBlock, 0, len(partial.Content))
	for _, block := range partial.Content {
		finalBlock, ok := canonicalContentBlockFromStream(block)
		if !ok {
			continue
		}
		final.Content = append(final.Content, finalBlock)
	}
	return final
}

func StreamConversationMessageFromCanonical(msg ConversationMessage) *StreamConversationMessage {
	stream := &StreamConversationMessage{
		Role:      msg.Role,
		Timestamp: msg.Timestamp,
	}
	if msg.ProviderState != nil {
		providerState := *msg.ProviderState
		stream.ProviderState = &providerState
	}
	if len(msg.Raw) > 0 {
		stream.Raw = append(json.RawMessage(nil), msg.Raw...)
	}
	if len(msg.Content) == 0 {
		return stream
	}
	stream.Content = make([]StreamContentBlock, 0, len(msg.Content))
	for _, block := range msg.Content {
		stream.Content = append(stream.Content, StreamContentBlockFromCanonical(block))
	}
	return stream
}

func canonicalContentBlockFromStream(block StreamContentBlock) (ContentBlock, bool) {
	finalBlock := ContentBlock{
		Type:                block.Type,
		Text:                block.Text,
		MimeType:            block.MimeType,
		ImageData:           block.ImageData,
		TextSignature:       block.TextSignature,
		ThinkingReplayField: block.ThinkingReplayField,
		ThinkingSignature:   block.ThinkingSignature,
		EncryptedContent:    block.EncryptedContent,
	}
	if len(block.Raw) > 0 {
		finalBlock.Raw = append(json.RawMessage(nil), block.Raw...)
	}
	switch block.Type {
	case BlockToolCall:
		if block.ToolCall == nil || !block.ToolCall.Complete {
			return ContentBlock{}, false
		}
		toolCall := CanonicalToolCall{
			ID:               block.ToolCall.ID,
			Name:             block.ToolCall.Name,
			RawArguments:     block.ToolCall.RawArguments,
			ThoughtSignature: block.ToolCall.ThoughtSignature,
		}
		if len(block.ToolCall.Arguments) > 0 {
			toolCall.Arguments = make(map[string]any, len(block.ToolCall.Arguments))
			for key, value := range block.ToolCall.Arguments {
				toolCall.Arguments[key] = value
			}
		}
		if len(block.ToolCall.Raw) > 0 {
			toolCall.Raw = append(json.RawMessage(nil), block.ToolCall.Raw...)
		}
		finalBlock.ToolCall = &toolCall
	}
	return finalBlock, true
}

func StreamContentBlockFromCanonical(block ContentBlock) StreamContentBlock {
	streamBlock := StreamContentBlock{
		Type:                block.Type,
		Text:                block.Text,
		MimeType:            block.MimeType,
		ImageData:           block.ImageData,
		TextSignature:       block.TextSignature,
		ThinkingReplayField: block.ThinkingReplayField,
		ThinkingSignature:   block.ThinkingSignature,
		EncryptedContent:    block.EncryptedContent,
	}
	if len(block.Raw) > 0 {
		streamBlock.Raw = append(json.RawMessage(nil), block.Raw...)
	}
	if block.ToolCall != nil {
		toolCall := &StreamToolCall{
			ID:               block.ToolCall.ID,
			Name:             block.ToolCall.Name,
			RawArguments:     block.ToolCall.RawArguments,
			ThoughtSignature: block.ToolCall.ThoughtSignature,
			Complete:         true,
		}
		if len(block.ToolCall.Arguments) > 0 {
			toolCall.Arguments = make(map[string]any, len(block.ToolCall.Arguments))
			for key, value := range block.ToolCall.Arguments {
				toolCall.Arguments[key] = value
			}
		}
		if len(block.ToolCall.Raw) > 0 {
			toolCall.Raw = append(json.RawMessage(nil), block.ToolCall.Raw...)
		}
		streamBlock.ToolCall = toolCall
	}
	return streamBlock
}

func CloneProviderResponsePtr(resp *ProviderResponse) *ProviderResponse {
	if resp == nil {
		return nil
	}
	cloned := *resp
	cloned.Message = *CloneConversationMessagePtr(&resp.Message)
	return &cloned
}

type ProviderCapabilities struct {
	SupportsThinkingBlocks     bool
	SupportsToolCalls          bool
	SupportsParallelToolCalls  bool
	SupportsImages             bool
	SupportsStatelessReplay    bool
	SupportsPreviousResponseID bool
	SupportsCompaction         bool
	SupportsWebsocketStream    bool
}

type CanonicalProvider interface {
	Capabilities() ProviderCapabilities
	CompleteCanonical(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error)
	StreamCanonical(ctx context.Context, req *ProviderRequest) (*ProviderEventStream, error)
}
