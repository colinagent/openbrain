package op

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MessageRole is the canonical role for chat messages within opagent host.
// It intentionally mirrors OpenAI-compatible roles we already convert to.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleDeveloper MessageRole = "developer"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
	RoleFunction  MessageRole = "function"
)

type MessageStopReason string

const (
	StopReasonStop    MessageStopReason = "stop"
	StopReasonLength  MessageStopReason = "length"
	StopReasonToolUse MessageStopReason = "tool_use"
	StopReasonError   MessageStopReason = "error"
	StopReasonAborted MessageStopReason = "aborted"
)

// MessageToolCall represents an assistant tool call (OpenAI "tool_calls").
// NOTE: host already has a ToolCall type for streaming aggregation; do not reuse the name.
type MessageToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	// Type is usually "function". Kept for completeness/future compatibility.
	Type string `json:"type,omitempty"`
}

// ContentPart is a multi-modal message block.
// We currently only consume text parts in opagent-runtime.
type ContentPart struct {
	Type       string    `json:"type"`
	Text       string    `json:"text,omitempty"`
	Name       string    `json:"name,omitempty"`
	DisplayRef string    `json:"display_ref,omitempty"`
	ImageURL   *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// Message is the strongly typed internal representation for messages.
// This is the single source-of-truth for:
// - LLM conversion (OpenAI SDK)
// - message history persistence (JSONL/Mongo)
type Message struct {
	Role MessageRole `json:"role"`

	// Content is used by system/developer/user/assistant/tool/function.
	// For assistant tool-call messages, Content is often empty.
	Content string `json:"content,omitempty"`

	// ContentParts enables future multi-modal inputs. If non-empty, it should be
	// treated as the primary content source.
	ContentParts []ContentPart `json:"content_parts,omitempty"`

	// ReasoningContent stores provider-specific "thinking" content for assistant messages.
	// Some providers require this field to be replayed across tool-calls.
	ReasoningContent string `json:"reasoning_content,omitempty"`

	// ReasoningReplayField records which OpenAI-compatible assistant message field
	// originally carried ReasoningContent (for example reasoning_content,
	// reasoning, or reasoning_text). This is replay metadata, not user-visible text,
	// and should only be reused for same-provider continuation.
	ReasoningReplayField string `json:"reasoning_replay_field,omitempty"`

	// ReasoningSignature stores provider-specific reasoning continuation state.
	// Anthropic extended thinking requires this field to be replayed together with
	// reasoning_content on the next turn, otherwise upstream validation fails.
	ReasoningSignature string `json:"reasoning_signature,omitempty"`

	// ToolCalls is only valid for assistant messages that invoke tools.
	ToolCalls []MessageToolCall `json:"tool_calls,omitempty"`

	// ToolCallID links a tool result message to a previous assistant tool call.
	ToolCallID string `json:"tool_call_id,omitempty"`

	// Name is used by "function" and sometimes "tool" messages in OpenAI formats.
	Name string `json:"name,omitempty"`

	// Timestamp is Unix milliseconds. Optional but useful for persistence / UI.
	Timestamp int64 `json:"timestamp,omitempty"`

	// Usage stores LLM-reported token usage. Only populated on assistant messages
	// after an LLM response. Used by hybrid context estimation (pi-mono pattern):
	// real API usage from the last assistant + estimate only trailing messages.
	Usage *MessageUsage `json:"usage,omitempty"`

	// StopReason marks how the assistant turn ended. This is persisted so
	// replay logic can distinguish complete turns from interrupted/error tails.
	StopReason MessageStopReason `json:"stop_reason,omitempty"`

	// ResponseID stores the upstream Responses API response.id for assistant
	// messages generated via /v1/responses.
	ResponseID string `json:"response_id,omitempty"`
}

// MessageUsage records token usage from an LLM response.
// InputTokens stores non-cached prompt tokens. CacheReadTokens and
// CacheWriteTokens store prompt-cache hits and writes when exposed by the
// provider. TotalTokens typically equals InputTokens + OutputTokens +
// CacheReadTokens + CacheWriteTokens, but some providers report it
// independently. For context estimation, TotalTokens is preferred because it
// includes everything the API saw (system prompt, cache, reasoning).
type MessageUsage struct {
	InputTokens      int64 `json:"inputTokens,omitempty"`
	OutputTokens     int64 `json:"outputTokens,omitempty"`
	CacheReadTokens  int64 `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64 `json:"cacheWriteTokens,omitempty"`
	TotalTokens      int64 `json:"totalTokens,omitempty"`
}

func (m Message) Validate() error {
	switch m.Role {
	case RoleSystem, RoleDeveloper, RoleUser:
		// ok
	case RoleAssistant:
		hasContent := strings.TrimSpace(m.Content) != "" || len(m.ContentParts) > 0 || len(m.ToolCalls) > 0
		if !hasContent {
			return fmt.Errorf("assistant message requires content/content_parts or tool_calls")
		}
		// If tool calls are present, they must be complete.
		for _, tc := range m.ToolCalls {
			if tc.ID == "" || tc.Name == "" {
				return fmt.Errorf("assistant tool call missing id/name")
			}
		}
	case RoleTool:
		if m.ToolCallID == "" {
			return fmt.Errorf("tool message requires tool_call_id")
		}
	case RoleFunction:
		if m.Name == "" {
			return fmt.Errorf("function message requires name")
		}
	default:
		return fmt.Errorf("unsupported message role: %q", m.Role)
	}
	return nil
}

func NewUserMessage(content string) Message {
	return Message{Role: RoleUser, Content: content}
}

func NewAssistantMessage(content string) Message {
	return Message{Role: RoleAssistant, Content: content}
}

func NewAssistantMessageWithReasoning(content, reasoningContent string) Message {
	return Message{Role: RoleAssistant, Content: content, ReasoningContent: reasoningContent}
}

func NewAssistantToolCalls(calls []MessageToolCall) Message {
	return Message{Role: RoleAssistant, ToolCalls: calls}
}

func NewAssistantToolCallsWithReasoning(content, reasoningContent string, calls []MessageToolCall) Message {
	return Message{Role: RoleAssistant, Content: content, ReasoningContent: reasoningContent, ToolCalls: calls}
}

func NewToolResultMessage(toolName, toolCallID, content string) Message {
	return Message{
		Role:       RoleTool,
		Name:       toolName,
		ToolCallID: toolCallID,
		Content:    content,
	}
}

// EstimateMessageTokens approximates token usage with a chars/4 heuristic.
// Includes Content, ReasoningContent (thinking), ContentParts, ToolCalls,
// text + thinking + toolCall blocks.
func EstimateMessageTokens(m Message) int64 {
	chars := len(m.Content) + len(m.ReasoningContent) + len(m.ReasoningSignature) + len(m.Name) + len(m.ToolCallID)
	for _, part := range m.ContentParts {
		chars += len(part.Type) + len(part.Text) + len(part.Name) + len(part.DisplayRef)
		if part.ImageURL != nil {
			chars += len(part.ImageURL.URL) + len(part.ImageURL.Detail)
		}
	}
	for _, tc := range m.ToolCalls {
		chars += len(tc.ID) + len(tc.Name) + len(tc.Type)
		if raw, err := json.Marshal(tc.Arguments); err == nil {
			chars += len(raw)
		}
	}
	if chars <= 0 {
		return 0
	}
	return int64((chars + 3) / 4)
}

func EstimateMessagesTokens(msgs []Message) int64 {
	var total int64
	for _, msg := range msgs {
		total += EstimateMessageTokens(msg)
	}
	return total
}

func SerializeMessagesForSummary(msgs []Message) string {
	var b strings.Builder
	for _, msg := range msgs {
		switch msg.Role {
		case RoleSystem:
			b.WriteString("[System]: ")
			b.WriteString(summaryText(msg))
			b.WriteString("\n\n")
		case RoleDeveloper:
			b.WriteString("[Developer]: ")
			b.WriteString(summaryText(msg))
			b.WriteString("\n\n")
		case RoleUser:
			b.WriteString("[User]: ")
			b.WriteString(summaryText(msg))
			b.WriteString("\n\n")
		case RoleAssistant:
			if text := summaryText(msg); text != "" {
				b.WriteString("[Assistant]: ")
				b.WriteString(text)
				b.WriteString("\n\n")
			}
			if len(msg.ToolCalls) > 0 {
				b.WriteString("[Assistant Tool Calls]: ")
				parts := make([]string, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					call := tc.Name
					if raw, err := json.Marshal(tc.Arguments); err == nil && len(raw) > 0 && string(raw) != "null" {
						call += "(" + string(raw) + ")"
					}
					parts = append(parts, call)
				}
				b.WriteString(strings.Join(parts, "; "))
				b.WriteString("\n\n")
			}
		case RoleTool:
			b.WriteString("[Tool Result")
			if msg.Name != "" {
				b.WriteString(": ")
				b.WriteString(msg.Name)
			}
			b.WriteString("]: ")
			b.WriteString(summaryText(msg))
			b.WriteString("\n\n")
		case RoleFunction:
			b.WriteString("[Function")
			if msg.Name != "" {
				b.WriteString(": ")
				b.WriteString(msg.Name)
			}
			b.WriteString("]: ")
			b.WriteString(summaryText(msg))
			b.WriteString("\n\n")
		default:
			raw, _ := json.Marshal(msg)
			b.WriteString("[Message]: ")
			b.Write(raw)
			b.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(b.String())
}
