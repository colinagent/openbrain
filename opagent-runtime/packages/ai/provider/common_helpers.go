package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type providerRequestOptions struct {
	MaxTokens        *int64
	Temperature      *float64
	ReasoningEffort  string
	ReasoningEnabled *bool
	ReasoningSummary string
	ToolChoice       string
}

type providerCanonicalRequest struct {
	Model    string
	Messages []op.Message
	Tools    []op.ToolSpec
	Options  providerRequestOptions
}

func providerRequestFromCanonical(req *ai.ProviderRequest, cfg *op.ModelConfig) (*providerCanonicalRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("canonical request is nil")
	}
	messages := make([]op.Message, 0, len(req.Context.Messages)+1)
	if sys := strings.TrimSpace(req.Context.SystemPrompt); sys != "" {
		messages = append(messages, op.Message{Role: op.RoleSystem, Content: sys})
	}
	for _, msg := range req.Context.Messages {
		converted, err := ai.OpMessageFromCanonical(msg)
		if err != nil {
			return nil, err
		}
		if converted.Role == "" {
			continue
		}
		messages = append(messages, converted)
	}
	tools := make([]op.ToolSpec, 0, len(req.Context.Tools))
	for _, tool := range req.Context.Tools {
		tools = append(tools, op.ToolSpec{
			Name:        strings.TrimSpace(tool.Name),
			Description: strings.TrimSpace(tool.Description),
			InputSchema: tool.Parameters,
		})
	}
	toolChoice := ""
	if len(req.Config.ToolChoice) > 0 {
		_ = json.Unmarshal(req.Config.ToolChoice, &toolChoice)
		toolChoice = strings.TrimSpace(toolChoice)
	}
	return &providerCanonicalRequest{
		Model:    providerModelName(strings.TrimSpace(req.Config.Model), cfg),
		Messages: messages,
		Tools:    tools,
		Options: providerRequestOptions{
			MaxTokens:        req.Config.MaxTokens,
			Temperature:      req.Config.Temperature,
			ReasoningEffort:  strings.TrimSpace(req.Config.ReasoningEffort),
			ReasoningEnabled: req.Config.ReasoningEnabled,
			ReasoningSummary: strings.TrimSpace(req.Config.ReasoningSummary),
			ToolChoice:       toolChoice,
		},
	}, nil
}

func prepareCanonicalReplayForProvider(req *ai.ProviderRequest, cfg *op.ModelConfig, defaultAPI string) *ai.ProviderRequest {
	if req == nil {
		return &ai.ProviderRequest{}
	}
	api := strings.TrimSpace(defaultAPI)
	provider := ""
	model := strings.TrimSpace(req.Config.Model)
	if cfg != nil {
		provider = strings.TrimSpace(cfg.Provider)
		if strings.TrimSpace(cfg.API) != "" {
			api = strings.TrimSpace(cfg.API)
		}
		model = providerModelName(model, cfg)
	}
	return ai.PrepareCanonicalReplayForTarget(req, ai.ReplayTarget{
		Provider: provider,
		API:      api,
		Model:    model,
	})
}

func providerModelName(requestModel string, cfg *op.ModelConfig) string {
	if trimmed := strings.TrimSpace(requestModel); trimmed != "" {
		return trimmed
	}
	if cfg != nil {
		return strings.TrimSpace(cfg.Name)
	}
	return ""
}

func providerMessageText(msg op.Message) string {
	if strings.TrimSpace(msg.Content) != "" {
		return msg.Content
	}
	parts := make([]string, 0, len(msg.ContentParts))
	for _, part := range msg.ContentParts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func decodeProviderDataURL(raw string) (string, []byte, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", nil, err
	}
	meta, encoded, ok := strings.Cut(parsed.Opaque, ",")
	if !ok {
		return "", nil, fmt.Errorf("invalid data url")
	}
	mediaType := "application/octet-stream"
	if head, _, ok := strings.Cut(meta, ";"); ok && strings.TrimSpace(head) != "" {
		mediaType = strings.TrimSpace(head)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, err
	}
	return mediaType, data, nil
}

func normalizeProviderSchemaMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	if params, ok := schema.(map[string]any); ok {
		return params
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil
	}
	return params
}

func normalizeProviderSchemaRequired(value any) []string {
	switch required := value.(type) {
	case []string:
		out := make([]string, 0, len(required))
		for _, item := range required {
			if text := strings.TrimSpace(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(required))
		for _, item := range required {
			text, ok := item.(string)
			if ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func parseToolArgumentsJSON(raw string) (map[string]any, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return nil, false
	}
	return args, true
}

func providerToolArgumentsFromRaw(raw json.RawMessage) map[string]any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return nil
	}
	return args
}

func providerToolArgumentsMap(raw string) map[string]any {
	if args, ok := parseToolArgumentsJSON(raw); ok {
		return args
	}
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	return map[string]any{"input": raw}
}

func providerToolArgumentsJSON(arguments map[string]any) string {
	return ai.MarshalToolArgumentsJSON(arguments)
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
