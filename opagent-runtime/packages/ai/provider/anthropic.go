package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type AnthropicProvider struct {
	client anthropic.Client
	cfg    *op.ModelConfig
}

func NewAnthropicProvider(cfg *op.ModelConfig) (*AnthropicProvider, error) {
	return NewAnthropicProviderWithOptions(cfg)
}

func NewAnthropicProviderWithOptions(cfg *op.ModelConfig, opts ...anthropicoption.RequestOption) (*AnthropicProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("model config is nil")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("model config: apiKey is required")
	}
	clientOpts := []anthropicoption.RequestOption{
		anthropicoption.WithAPIKey(cfg.APIKey),
		anthropicoption.WithBaseURL(strings.TrimSpace(cfg.BaseURL)),
	}
	clientOpts = append(clientOpts, opts...)
	client := anthropic.NewClient(clientOpts...)
	return &AnthropicProvider{client: client, cfg: cfg}, nil
}

func AnthropicRequestOptions(httpClient *http.Client, headers map[string]string) []anthropicoption.RequestOption {
	opts := make([]anthropicoption.RequestOption, 0, len(headers)+1)
	if httpClient != nil {
		opts = append(opts, anthropicoption.WithHTTPClient(httpClient))
	}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		opts = append(opts, anthropicoption.WithHeader(key, value))
	}
	return opts
}

func (p *AnthropicProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("anthropic-messages")
}

func (p *AnthropicProvider) CompleteCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	params, err := p.buildRequest(req)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}
	return providerResponseFromAnthropic(resp), nil
}

func (p *AnthropicProvider) StreamCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req == nil {
		return nil, fmt.Errorf("canonical request is nil")
	}
	streamParams, err := p.buildRequest(req)
	if err != nil {
		return nil, err
	}
	stream := p.client.Messages.NewStreaming(ctx, streamParams)
	out := ai.NewProviderEventStream(128)
	go func() {
		defer stream.Close()
		partial := &ai.StreamConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			ProviderState: &ai.ProviderState{
				Provider: strings.TrimSpace(p.cfg.Provider),
				API:      "anthropic-messages",
				Model:    strings.TrimSpace(streamParams.Model),
			},
		}
		usage := ai.Usage{}
		stopReason := ai.StopReasonStop
		for stream.Next() {
			switch event := stream.Current().AsAny().(type) {
			case anthropic.MessageStartEvent:
				usage.InputTokens = int64(event.Message.Usage.InputTokens)
				usage.OutputTokens = int64(event.Message.Usage.OutputTokens)
				usage.CacheReadTokens = int64(event.Message.Usage.CacheReadInputTokens)
				usage.CacheWriteTokens = int64(event.Message.Usage.CacheCreationInputTokens)
				usage.TotalTokens = usage.ResolvedTotalTokens()
				if !out.Emit(ai.ProviderEvent{
					Type:    ai.EventCanonicalStart,
					Partial: partial,
					Raw:     json.RawMessage(event.RawJSON()),
				}) {
					return
				}
			case anthropic.ContentBlockStartEvent:
				index := int(event.Index)
				switch block := event.ContentBlock.AsAny().(type) {
				case anthropic.TextBlock:
					ensureCanonicalContentIndex(partial, index, ai.BlockText)
					partial.Content[index].Text = block.Text
					partial.Content[index].Raw = json.RawMessage(block.RawJSON())
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalTextStart,
						ContentIndex: index,
						Block:        &partial.Content[index],
						Partial:      partial,
						Raw:          json.RawMessage(event.RawJSON()),
					}) {
						return
					}
				case anthropic.ThinkingBlock:
					ensureCanonicalContentIndex(partial, index, ai.BlockThinking)
					partial.Content[index].Text = block.Thinking
					partial.Content[index].ThinkingSignature = block.Signature
					partial.Content[index].Raw = json.RawMessage(block.RawJSON())
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalThinkingStart,
						ContentIndex: index,
						Block:        &partial.Content[index],
						Partial:      partial,
						Raw:          json.RawMessage(event.RawJSON()),
					}) {
						return
					}
				case anthropic.ToolUseBlock:
					ensureCanonicalContentIndex(partial, index, ai.BlockToolCall)
					partial.Content[index].ToolCall = &ai.StreamToolCall{
						ID:   strings.TrimSpace(block.ID),
						Name: strings.TrimSpace(block.Name),
						Raw:  json.RawMessage(block.RawJSON()),
					}
					if len(block.Input) > 0 {
						var args map[string]any
						if err := json.Unmarshal(block.Input, &args); err == nil && len(args) > 0 {
							partial.Content[index].ToolCall.Arguments = args
						}
					}
					partial.Content[index].Raw = json.RawMessage(block.RawJSON())
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalToolCallStart,
						ContentIndex: index,
						Block:        &partial.Content[index],
						Partial:      partial,
						Raw:          json.RawMessage(event.RawJSON()),
					}) {
						return
					}
				}
			case anthropic.ContentBlockDeltaEvent:
				index := int(event.Index)
				if index < 0 || index >= len(partial.Content) {
					continue
				}
				block := &partial.Content[index]
				switch delta := event.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					block.Text += delta.Text
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalTextDelta,
						ContentIndex: index,
						Delta:        delta.Text,
						Block:        block,
						Partial:      partial,
						Raw:          json.RawMessage(event.RawJSON()),
					}) {
						return
					}
				case anthropic.ThinkingDelta:
					block.Text += delta.Thinking
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalThinkingDelta,
						ContentIndex: index,
						Delta:        delta.Thinking,
						Block:        block,
						Partial:      partial,
						Raw:          json.RawMessage(event.RawJSON()),
					}) {
						return
					}
				case anthropic.InputJSONDelta:
					if block.ToolCall == nil {
						continue
					}
					fragment := string(delta.PartialJSON)
					if fragment == "" {
						continue
					}
					switch {
					case fragment == "{}" && strings.TrimSpace(block.ToolCall.RawArguments) == "":
						// Anthropic often sends an initial empty object marker before the real JSON body.
						continue
					case strings.TrimSpace(block.ToolCall.RawArguments) == "{}" && strings.HasPrefix(strings.TrimLeft(fragment, " \t\r\n"), "{"):
						block.ToolCall.RawArguments = fragment
					default:
						block.ToolCall.RawArguments += fragment
					}
					if args, ok := parseToolArgumentsJSON(block.ToolCall.RawArguments); ok {
						block.ToolCall.Arguments = args
					}
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalToolCallDelta,
						ContentIndex: index,
						Delta:        string(delta.PartialJSON),
						Block:        block,
						Partial:      partial,
						Raw:          json.RawMessage(event.RawJSON()),
					}) {
						return
					}
				case anthropic.SignatureDelta:
					block.ThinkingSignature += delta.Signature
				}
			case anthropic.ContentBlockStopEvent:
				index := int(event.Index)
				if index < 0 || index >= len(partial.Content) {
					continue
				}
				block := &partial.Content[index]
				switch block.Type {
				case ai.BlockText:
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalTextEnd,
						ContentIndex: index,
						Content:      block.Text,
						Block:        block,
						Partial:      partial,
						Raw:          json.RawMessage(event.RawJSON()),
					}) {
						return
					}
				case ai.BlockThinking:
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalThinkingEnd,
						ContentIndex: index,
						Content:      block.Text,
						Block:        block,
						Partial:      partial,
						Raw:          json.RawMessage(event.RawJSON()),
					}) {
						return
					}
				case ai.BlockToolCall:
					if block.ToolCall != nil && strings.TrimSpace(block.ToolCall.RawArguments) == "" && len(block.ToolCall.Arguments) > 0 {
						block.ToolCall.RawArguments = ai.MarshalToolArgumentsJSON(block.ToolCall.Arguments)
					}
					if block.ToolCall != nil {
						block.ToolCall.Complete = true
					}
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalToolCallEnd,
						ContentIndex: index,
						Block:        block,
						Partial:      partial,
						Raw:          json.RawMessage(event.RawJSON()),
					}) {
						return
					}
				}
			case anthropic.MessageDeltaEvent:
				usage.OutputTokens = int64(event.Usage.OutputTokens)
				usage.CacheReadTokens = int64(event.Usage.CacheReadInputTokens)
				usage.CacheWriteTokens = int64(event.Usage.CacheCreationInputTokens)
				usage.TotalTokens = usage.ResolvedTotalTokens()
				switch event.Delta.StopReason {
				case anthropic.StopReasonToolUse:
					stopReason = ai.StopReasonToolUse
				case anthropic.StopReasonMaxTokens:
					stopReason = ai.StopReasonLength
				case anthropic.StopReasonPauseTurn:
					stopReason = ai.StopReasonStop
				case anthropic.StopReasonRefusal:
					stopReason = ai.StopReasonStop
				default:
					stopReason = ai.StopReasonStop
				}
			case anthropic.MessageStopEvent:
				out.Emit(ai.ProviderEvent{
					Type: ai.EventCanonicalDone,
					Response: &ai.ProviderResponse{
						Message:    ai.FinalizeStreamConversationMessage(partial),
						Usage:      usage,
						StopReason: stopReason,
					},
					Raw: json.RawMessage(event.RawJSON()),
				})
				out.Close()
				return
			}
		}
		if err := stream.Err(); err != nil {
			out.Finish(err)
			return
		}
		out.Close()
	}()
	return out, nil
}

func (p *AnthropicProvider) buildRequest(req *ai.ProviderRequest) (anthropic.MessageNewParams, error) {
	params, err := providerRequestFromCanonical(req, p.cfg)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	modelName := providerModelName(params.Model, p.cfg)
	if modelName == "" {
		return anthropic.MessageNewParams{}, fmt.Errorf("model name is empty")
	}

	systemPrompt, messages, err := convertMessagesToAnthropic(params.Messages)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	request := anthropic.MessageNewParams{
		Model:     modelName,
		Messages:  messages,
		MaxTokens: anthropicDefaultMaxTokens(params.Options, p.cfg),
	}
	if strings.TrimSpace(systemPrompt) != "" {
		request.System = []anthropic.TextBlockParam{{Text: systemPrompt}}
	}
	if tools := convertToolSpecsToAnthropic(params.Tools); len(tools) > 0 {
		request.Tools = tools
	}
	thinkingEnabled := false
	if supportsAnthropicAdaptiveThinking(modelName) {
		if effort, ok := anthropicAdaptiveEffort(params.Options.ReasoningEffort, modelName); ok {
			request.Thinking = anthropic.ThinkingConfigParamUnion{
				OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
			}
			request.OutputConfig = anthropic.OutputConfigParam{Effort: effort}
			thinkingEnabled = true
			// Adaptive thinking: MaxTokens covers thinking + output combined.
			// The 4096 default is far too small for agentic use where the model
			// needs to generate long tool calls (e.g. write a full file).
			// pi-mono uses min(model.maxTokens, 32000) as the base default.
			// We follow the same approach: ensure at least 32000 for adaptive.
			if request.MaxTokens < 32000 {
				request.MaxTokens = 32000
			}
		}
	} else if budget := anthropicThinkingBudget(params.Options.ReasoningEffort); budget > 0 {
		// Budget-based thinking (older models): MaxTokens must cover
		// thinking budget + actual output. Follow pi-mono's approach:
		// add the thinking budget on top of the base maxTokens.
		request.MaxTokens = request.MaxTokens + budget
		request.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
		thinkingEnabled = true
	}
	if params.Options.Temperature != nil && !thinkingEnabled {
		request.Temperature = anthropic.Float(*params.Options.Temperature)
	}
	if choice, ok := anthropicToolChoice(params.Options.ToolChoice); ok {
		request.ToolChoice = choice
	}
	return request, nil
}

// anthropicThinkingBudget returns the thinking token budget for non-adaptive
// thinking models based on the reasoning effort level.
// The caller is responsible for adding this budget on top of MaxTokens.
func anthropicThinkingBudget(effort string) int64 {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "":
		return 0
	case "minimal", "low":
		return 1024
	case "medium":
		return 2048
	case "high":
		return 8192
	case "xhigh", "max":
		return 16384
	default:
		return 2048
	}
}

func supportsAnthropicAdaptiveThinking(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(modelName, "opus-4-6") ||
		strings.Contains(modelName, "opus-4.6") ||
		strings.Contains(modelName, "sonnet-4-6") ||
		strings.Contains(modelName, "sonnet-4.6")
}

func anthropicAdaptiveEffort(reasoningEffort string, modelName string) (anthropic.OutputConfigEffort, bool) {
	switch strings.ToLower(strings.TrimSpace(reasoningEffort)) {
	case "":
		return "", false
	case "minimal", "low":
		return anthropic.OutputConfigEffortLow, true
	case "medium":
		return anthropic.OutputConfigEffortMedium, true
	case "high":
		return anthropic.OutputConfigEffortHigh, true
	case "xhigh", "max":
		if strings.Contains(strings.ToLower(strings.TrimSpace(modelName)), "opus-4-6") || strings.Contains(strings.ToLower(strings.TrimSpace(modelName)), "opus-4.6") {
			return anthropic.OutputConfigEffortMax, true
		}
		return anthropic.OutputConfigEffortHigh, true
	default:
		return anthropic.OutputConfigEffortHigh, true
	}
}

func anthropicToolChoice(toolChoice string) (anthropic.ToolChoiceUnionParam, bool) {
	switch strings.ToLower(strings.TrimSpace(toolChoice)) {
	case "":
		return anthropic.ToolChoiceUnionParam{}, false
	case "auto":
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}, true
	case "required":
		return anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{},
		}, true
	case "none":
		choice := anthropic.NewToolChoiceNoneParam()
		return anthropic.ToolChoiceUnionParam{
			OfNone: &choice,
		}, true
	default:
		return anthropic.ToolChoiceUnionParam{}, false
	}
}

func anthropicDefaultMaxTokens(options providerRequestOptions, cfg *op.ModelConfig) int64 {
	if options.MaxTokens != nil && *options.MaxTokens > 0 {
		return *options.MaxTokens
	}
	if cfg != nil && cfg.MaxOutputTokens > 0 {
		return cfg.MaxOutputTokens
	}
	return 4096
}

func convertMessagesToAnthropic(msgs []op.Message) (string, []anthropic.MessageParam, error) {
	if len(msgs) == 0 {
		return "", nil, nil
	}

	var systemParts []string
	converted := make([]anthropic.MessageParam, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {
		case op.RoleSystem, op.RoleDeveloper:
			text := providerMessageText(msg)
			if strings.TrimSpace(text) != "" {
				systemParts = append(systemParts, text)
			}
		case op.RoleUser:
			blocks, err := anthropicUserBlocks(msg)
			if err != nil {
				return "", nil, err
			}
			if len(blocks) == 0 {
				continue
			}
			converted = append(converted, anthropic.NewUserMessage(blocks...))
		case op.RoleAssistant:
			blocks, err := anthropicAssistantBlocks(msg)
			if err != nil {
				return "", nil, err
			}
			if len(blocks) == 0 {
				continue
			}
			converted = append(converted, anthropic.NewAssistantMessage(blocks...))
		case op.RoleTool:
			content, err := anthropicToolResultContent(msg)
			if err != nil {
				return "", nil, err
			}
			block := anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: strings.TrimSpace(msg.ToolCallID),
					Content:   content,
				},
			}
			converted = append(converted, anthropic.NewUserMessage(block))
		}
	}
	return strings.Join(systemParts, "\n\n"), converted, nil
}

func anthropicToolResultContent(msg op.Message) ([]anthropic.ToolResultBlockParamContentUnion, error) {
	blocks := make([]anthropic.ToolResultBlockParamContentUnion, 0, len(msg.ContentParts)+1)
	if len(msg.ContentParts) == 0 {
		return []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: msg.Content}},
		}, nil
	}
	for _, part := range msg.ContentParts {
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "", "text":
			if strings.TrimSpace(part.Text) != "" {
				blocks = append(blocks, anthropic.ToolResultBlockParamContentUnion{
					OfText: &anthropic.TextBlockParam{Text: part.Text},
				})
			}
		case "image", "image_url":
			imageBlock, err := anthropicImageBlock(part)
			if err != nil {
				return nil, err
			}
			if imageBlock != nil {
				blocks = append(blocks, anthropic.ToolResultBlockParamContentUnion{OfImage: imageBlock})
			}
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, anthropic.ToolResultBlockParamContentUnion{
			OfText: &anthropic.TextBlockParam{Text: msg.Content},
		})
	}
	return blocks, nil
}

func anthropicUserBlocks(msg op.Message) ([]anthropic.ContentBlockParamUnion, error) {
	if len(msg.ContentParts) == 0 {
		if strings.TrimSpace(msg.Content) == "" {
			return nil, nil
		}
		return []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: msg.Content}},
		}, nil
	}

	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.ContentParts))
	for _, part := range msg.ContentParts {
		switch part.Type {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfText: &anthropic.TextBlockParam{Text: part.Text},
				})
			}
		case "image_url":
			imageBlock, err := anthropicImageBlock(part)
			if err != nil {
				return nil, err
			}
			if imageBlock != nil {
				blocks = append(blocks, anthropic.ContentBlockParamUnion{OfImage: imageBlock})
			}
		}
	}
	return blocks, nil
}

func anthropicAssistantBlocks(msg op.Message) ([]anthropic.ContentBlockParamUnion, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.ToolCalls)+2)
	if reasoning := strings.TrimSpace(msg.ReasoningContent); reasoning != "" {
		if signature := strings.TrimSpace(msg.ReasoningSignature); signature != "" {
			blocks = append(blocks, anthropic.NewThinkingBlock(signature, reasoning))
		} else {
			blocks = append(blocks, anthropic.NewTextBlock(reasoning))
		}
	}
	if text := strings.TrimSpace(msg.Content); text != "" {
		blocks = append(blocks, anthropic.NewTextBlock(text))
	}
	for _, tc := range msg.ToolCalls {
		input := ai.CloneToolArguments(tc.Arguments)
		blocks = append(blocks, anthropic.ContentBlockParamUnion{
			OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    strings.TrimSpace(tc.ID),
				Name:  strings.TrimSpace(tc.Name),
				Input: input,
			},
		})
	}
	return blocks, nil
}

func anthropicImageBlock(part op.ContentPart) (*anthropic.ImageBlockParam, error) {
	if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
		return nil, nil
	}
	rawURL := strings.TrimSpace(part.ImageURL.URL)
	if strings.HasPrefix(rawURL, "data:") {
		mediaType, data, err := decodeProviderDataURL(rawURL)
		if err != nil {
			return nil, err
		}
		return &anthropic.ImageBlockParam{
			Source: anthropic.ImageBlockParamSourceUnion{
				OfBase64: &anthropic.Base64ImageSourceParam{
					Data:      base64.StdEncoding.EncodeToString(data),
					MediaType: anthropic.Base64ImageSourceMediaType(mediaType),
				},
			},
		}, nil
	}
	return &anthropic.ImageBlockParam{
		Source: anthropic.ImageBlockParamSourceUnion{
			OfURL: &anthropic.URLImageSourceParam{URL: rawURL},
		},
	}, nil
}

func convertToolSpecsToAnthropic(specs []op.ToolSpec) []anthropic.ToolUnionParam {
	if len(specs) == 0 {
		return nil
	}
	tools := make([]anthropic.ToolUnionParam, 0, len(specs))
	for _, spec := range specs {
		schema := anthropic.ToolInputSchemaParam{}
		if raw := normalizeProviderSchemaMap(spec.InputSchema); len(raw) > 0 {
			if properties, ok := raw["properties"]; ok {
				schema.Properties = properties
			}
			if required := normalizeProviderSchemaRequired(raw["required"]); len(required) > 0 {
				schema.Required = required
			}
			for key, value := range raw {
				if key == "type" || key == "properties" || key == "required" {
					continue
				}
				if schema.ExtraFields == nil {
					schema.ExtraFields = map[string]any{}
				}
				schema.ExtraFields[key] = value
			}
		}
		tool := anthropic.ToolUnionParamOfTool(schema, spec.Name)
		if desc := strings.TrimSpace(spec.Description); desc != "" {
			tool.OfTool.Description = anthropic.String(desc)
		}
		tools = append(tools, tool)
	}
	return tools
}

func providerResponseFromAnthropic(resp *anthropic.Message) *ai.ProviderResponse {
	if resp == nil {
		return nil
	}
	var content strings.Builder
	var reasoning strings.Builder
	toolCalls := make([]op.MessageToolCall, 0)

	for _, block := range resp.Content {
		switch value := block.AsAny().(type) {
		case anthropic.TextBlock:
			content.WriteString(value.Text)
		case anthropic.ThinkingBlock:
			reasoning.WriteString(value.Thinking)
		case anthropic.ToolUseBlock:
			toolCalls = append(toolCalls, op.MessageToolCall{
				ID:        value.ID,
				Name:      value.Name,
				Arguments: providerToolArgumentsFromRaw(value.Input),
				Type:      "function",
			})
		}
	}

	stopReason := ai.StopReasonStop
	if len(toolCalls) > 0 || resp.StopReason == anthropic.StopReasonToolUse {
		stopReason = ai.StopReasonToolUse
	} else if resp.StopReason == anthropic.StopReasonMaxTokens {
		stopReason = ai.StopReasonLength
	}

	usage := ai.Usage{}
	usage.InputTokens = int64(resp.Usage.InputTokens)
	usage.OutputTokens = int64(resp.Usage.OutputTokens)
	usage.CacheReadTokens = int64(resp.Usage.CacheReadInputTokens)
	usage.CacheWriteTokens = int64(resp.Usage.CacheCreationInputTokens)
	usage.TotalTokens = usage.ResolvedTotalTokens()

	return ai.ProviderResponseFromOpMessage(op.Message{
		Role:             op.RoleAssistant,
		Content:          content.String(),
		ReasoningContent: reasoning.String(),
		ToolCalls:        toolCalls,
	}, usage, stopReason)
}
