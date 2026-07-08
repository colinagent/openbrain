package chatcompletions

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type RequestPayload struct {
	Model               string           `json:"model"`
	Messages            []requestMessage `json:"messages"`
	Tools               []requestTool    `json:"tools"`
	Stream              bool             `json:"stream"`
	Temperature         *float64         `json:"temperature"`
	MaxTokens           *int64           `json:"max_tokens"`
	MaxCompletionTokens *int64           `json:"max_completion_tokens"`
	ReasoningEffort     string           `json:"reasoning_effort"`
	ToolChoice          json.RawMessage  `json:"tool_choice"`
}

type requestMessage struct {
	Role               string            `json:"role"`
	Content            json.RawMessage   `json:"content"`
	ReasoningContent   string            `json:"reasoning_content"`
	ReasoningSignature string            `json:"reasoning_signature"`
	ToolCalls          []requestToolCall `json:"tool_calls"`
	ToolCallID         string            `json:"tool_call_id"`
	Name               string            `json:"name"`
}

type requestToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type requestContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL *struct {
		URL    string `json:"url"`
		Detail string `json:"detail"`
	} `json:"image_url"`
}

type requestTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

func DecodeRequestJSON(raw []byte) (*RequestPayload, error) {
	var req RequestPayload
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("invalid JSON payload")
	}
	return &req, nil
}

func (r *RequestPayload) ToCanonicalRequest() (*ai.ProviderRequest, error) {
	if r == nil {
		return nil, fmt.Errorf("chat completions request is nil")
	}
	messages, err := convertMessages(r.Messages)
	if err != nil {
		return nil, err
	}
	tools, err := convertTools(r.Tools)
	if err != nil {
		return nil, err
	}
	canonical := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: ai.CanonicalMessagesFromOp(messages),
			Tools:    ai.CanonicalToolsFromOp(tools),
		},
		Config: ai.GenerationConfig{
			Model:           strings.TrimSpace(r.Model),
			Temperature:     r.Temperature,
			ReasoningEffort: strings.TrimSpace(r.ReasoningEffort),
		},
	}
	if r.MaxCompletionTokens != nil {
		canonical.Config.MaxTokens = r.MaxCompletionTokens
	} else {
		canonical.Config.MaxTokens = r.MaxTokens
	}
	if len(r.ToolChoice) > 0 {
		canonical.Config.ToolChoice = append(json.RawMessage(nil), r.ToolChoice...)
	}
	return canonical, nil
}

func NewResponseID() string {
	return fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
}

func FinishReason(reason ai.StopReason) string {
	switch reason {
	case ai.StopReasonLength:
		return "length"
	case ai.StopReasonToolUse:
		return "tool_calls"
	default:
		return "stop"
	}
}

func RenderResponseJSON(modelID string, resp *ai.ProviderResponse) json.RawMessage {
	if resp == nil {
		resp = &ai.ProviderResponse{}
	}
	message, err := ai.OpMessageFromCanonical(resp.Message)
	if err != nil {
		message = op.Message{Role: op.RoleAssistant}
	}
	payload := map[string]any{
		"id":      NewResponseID(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   strings.TrimSpace(modelID),
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]any{
				"role":                "assistant",
				"content":             message.Content,
				"reasoning_content":   message.ReasoningContent,
				"reasoning_signature": message.ReasoningSignature,
				"tool_calls":          renderToolCalls(message.ToolCalls),
			},
			"finish_reason": FinishReason(resp.StopReason),
		}},
		"usage": map[string]any{
			"prompt_tokens":     resp.Usage.PromptTokens(),
			"completion_tokens": resp.Usage.OutputTokens,
			"total_tokens":      resp.Usage.ResolvedTotalTokens(),
		},
	}
	if resp.Usage.CacheReadTokens > 0 || resp.Usage.CacheWriteTokens > 0 {
		promptDetails := map[string]any{}
		if resp.Usage.CacheReadTokens > 0 {
			promptDetails["cached_tokens"] = resp.Usage.CacheReadTokens
		}
		if resp.Usage.CacheWriteTokens > 0 {
			promptDetails["cache_write_tokens"] = resp.Usage.CacheWriteTokens
		}
		payload["usage"].(map[string]any)["prompt_tokens_details"] = promptDetails
	}
	data, _ := json.Marshal(payload)
	return data
}

func RenderChunkRoleJSON(id, modelID string, created int64) json.RawMessage {
	return renderChunkJSON(id, modelID, created, map[string]any{"role": "assistant"}, nil)
}

func RenderChunkTextDeltaJSON(id, modelID string, created int64, delta string) json.RawMessage {
	return renderChunkJSON(id, modelID, created, map[string]any{"content": delta}, nil)
}

func RenderChunkReasoningDeltaJSON(id, modelID string, created int64, delta string) json.RawMessage {
	return renderChunkJSON(id, modelID, created, map[string]any{"reasoning_content": delta}, nil)
}

func RenderChunkToolCallDeltaJSON(id, modelID string, created int64, callID, name, rawArguments string) json.RawMessage {
	return renderChunkJSON(id, modelID, created, map[string]any{"tool_calls": []map[string]any{{
		"id":   strings.TrimSpace(callID),
		"type": "function",
		"function": map[string]any{
			"name":      strings.TrimSpace(name),
			"arguments": strings.TrimSpace(rawArguments),
		},
	}}}, nil)
}

func RenderChunkFinishJSON(id, modelID string, created int64, reason ai.StopReason) json.RawMessage {
	return renderChunkJSON(id, modelID, created, map[string]any{}, FinishReason(reason))
}

func renderChunkJSON(id, modelID string, created int64, delta map[string]any, finishReason any) json.RawMessage {
	if strings.TrimSpace(id) == "" {
		id = NewResponseID()
	}
	if created <= 0 {
		created = time.Now().Unix()
	}
	payload := map[string]any{
		"id":      strings.TrimSpace(id),
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   strings.TrimSpace(modelID),
		"choices": []map[string]any{{
			"index":         0,
			"delta":         delta,
			"finish_reason": finishReason,
		}},
	}
	data, _ := json.Marshal(payload)
	return data
}

func renderToolCalls(calls []op.MessageToolCall) []map[string]any {
	if len(calls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		out = append(out, map[string]any{
			"id":   strings.TrimSpace(call.ID),
			"type": firstNonEmptyString(strings.TrimSpace(call.Type), "function"),
			"function": map[string]any{
				"name":      strings.TrimSpace(call.Name),
				"arguments": ai.MarshalToolArgumentsJSON(call.Arguments),
			},
		})
	}
	return out
}

func convertMessages(messages []requestMessage) ([]op.Message, error) {
	out := make([]op.Message, 0, len(messages))
	for _, message := range messages {
		item := op.Message{
			Role:               op.MessageRole(strings.TrimSpace(message.Role)),
			ReasoningContent:   strings.TrimSpace(message.ReasoningContent),
			ReasoningSignature: strings.TrimSpace(message.ReasoningSignature),
			ToolCallID:         strings.TrimSpace(message.ToolCallID),
			Name:               strings.TrimSpace(message.Name),
		}
		if item.ReasoningContent != "" {
			item.ReasoningReplayField = "reasoning_content"
		}
		content, parts, err := parseContent(message.Content)
		if err != nil {
			return nil, err
		}
		item.Content = content
		item.ContentParts = parts
		if len(message.ToolCalls) > 0 {
			item.ToolCalls = make([]op.MessageToolCall, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				item.ToolCalls = append(item.ToolCalls, op.MessageToolCall{
					ID:        strings.TrimSpace(call.ID),
					Name:      strings.TrimSpace(call.Function.Name),
					Arguments: ai.ParseToolArgumentsObject(strings.TrimSpace(call.Function.Arguments)),
					Type:      strings.TrimSpace(call.Type),
				})
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func parseContent(raw json.RawMessage) (string, []op.ContentPart, error) {
	if len(raw) == 0 {
		return "", nil, nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil, nil
	}
	var parts []requestContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", nil, fmt.Errorf("unsupported message content")
	}
	out := make([]op.ContentPart, 0, len(parts))
	textParts := make([]string, 0, len(parts))
	for _, part := range parts {
		switch strings.TrimSpace(part.Type) {
		case "text", "input_text", "output_text":
			value := strings.TrimSpace(part.Text)
			if value == "" {
				continue
			}
			out = append(out, op.ContentPart{Type: "text", Text: value})
			textParts = append(textParts, value)
		case "image_url", "input_image":
			if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
				continue
			}
			out = append(out, op.ContentPart{
				Type: "image_url",
				ImageURL: &op.ImageURL{
					URL:    strings.TrimSpace(part.ImageURL.URL),
					Detail: strings.TrimSpace(part.ImageURL.Detail),
				},
			})
		}
	}
	return strings.Join(textParts, "\n"), out, nil
}

func convertTools(tools []requestTool) ([]op.ToolSpec, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make([]op.ToolSpec, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Type) != "" && strings.TrimSpace(tool.Type) != "function" {
			return nil, fmt.Errorf("unsupported tool type %q", tool.Type)
		}
		out = append(out, op.ToolSpec{
			Name:        strings.TrimSpace(tool.Function.Name),
			Description: strings.TrimSpace(tool.Function.Description),
			InputSchema: tool.Function.Parameters,
		})
	}
	return out, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
