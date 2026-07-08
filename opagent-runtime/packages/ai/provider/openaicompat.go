package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tidwall/gjson"
)

// Provider implements ai.CanonicalProvider for OpenAI-compatible Chat Completions APIs.
type Provider struct {
	client openai.Client
	cfg    *op.ModelConfig
}

type openAICompatToolCall struct {
	ID           string
	Name         string
	Type         string
	Arguments    map[string]any
	RawArguments string
}

var _ ai.CanonicalProvider = (*Provider)(nil)

func NewProvider(cfg *op.ModelConfig) (*Provider, error) {
	return NewProviderWithOptions(cfg)
}

func NewProviderWithOptions(cfg *op.ModelConfig, opts ...option.RequestOption) (*Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("model config is nil")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("model config: apiKey is required")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("model config: baseURL is required")
	}
	clientOpts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
	}
	clientOpts = append(clientOpts, opts...)
	client := openai.NewClient(clientOpts...)
	return &Provider{client: client, cfg: cfg}, nil
}

func (p *Provider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-completions")
}

func (p *Provider) CompleteCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	params, err := p.buildRequest(req)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty model response")
	}
	choice := resp.Choices[0]
	content := choice.Message.Content
	taggedReasoning := ""
	if p.cfg != nil && p.cfg.Reasoning {
		content, taggedReasoning, _ = sanitizeTaggedAssistantContent(choice.Message.Content)
	}
	reasoningReplayField, reasoning := extractReasoningWithReplayField(choice.Message.RawJSON(), "")
	if reasoning == "" {
		field, extracted := extractReasoningWithReplayField(choice.RawJSON(), "message.")
		if reasoningReplayField == "" {
			reasoningReplayField = field
		}
		reasoning = extracted
	}
	if reasoning == "" {
		reasoning = strings.TrimSpace(taggedReasoning)
		reasoningReplayField = ""
	}
	reasoningSignature := extractReasoningSignature(choice.Message.RawJSON())
	if reasoningSignature == "" {
		reasoningSignature = extractStringFromJSON(choice.RawJSON(), "message.reasoning_signature")
	}
	stopReason := mapStopReason(choice.FinishReason)
	response := ai.ProviderResponseFromOpMessage(op.Message{
		Role:                 op.RoleAssistant,
		Content:              content,
		ReasoningContent:     reasoning,
		ReasoningReplayField: reasoningReplayField,
		ReasoningSignature:   reasoningSignature,
		ToolCalls:            opToolCallsFromCompat(convertOpenAIToolCalls(choice.Message.ToolCalls)),
	}, toUsage(resp.Usage), stopReason)
	if response.Message.ProviderState == nil {
		response.Message.ProviderState = &ai.ProviderState{}
	}
	response.Message.ProviderState.Provider = strings.TrimSpace(p.cfg.Provider)
	response.Message.ProviderState.API = "openai-completions"
	response.Message.ProviderState.Model = string(params.Model)
	return response, nil
}

func (p *Provider) StreamCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	params, err := p.buildRequest(req)
	if err != nil {
		return nil, err
	}
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: param.NewOpt(true)}

	out := ai.NewProviderEventStream(128)
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	go func() {
		defer stream.Close()

		partial := &ai.StreamConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			ProviderState: &ai.ProviderState{
				Provider: strings.TrimSpace(p.cfg.Provider),
				API:      "openai-completions",
				Model:    string(params.Model),
			},
		}
		var accumulator openai.ChatCompletionAccumulator
		var explicitReasoningBuilder strings.Builder
		reasoningReplayField := ""
		var reasoningSignatureBuilder strings.Builder
		var taggedReasoningBuilder strings.Builder
		toolCallsByIndex := make(map[int]*openAICompatToolCall)
		toolArgsByIndex := make(map[int]*strings.Builder)
		toolBlockByIndex := make(map[int]int)
		filterTaggedReasoning := p.cfg != nil && p.cfg.Reasoning
		contentFilter := newLeakedReasoningStreamFilter()
		sawExplicitReasoning := false
		started := false
		textIndex := -1
		thinkingIndex := -1

		emitStart := func() bool {
			if started {
				return true
			}
			started = true
			return out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalStart, Partial: partial})
		}
		ensureTextBlock := func() (int, bool) {
			if textIndex >= 0 && textIndex < len(partial.Content) {
				return textIndex, true
			}
			if !emitStart() {
				return -1, false
			}
			textIndex = len(partial.Content)
			partial.Content = append(partial.Content, ai.StreamContentBlock{Type: ai.BlockText})
			if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalTextStart, ContentIndex: textIndex, Block: &partial.Content[textIndex], Partial: partial}) {
				return -1, false
			}
			return textIndex, true
		}
		ensureThinkingBlock := func() (int, bool) {
			if thinkingIndex >= 0 && thinkingIndex < len(partial.Content) {
				return thinkingIndex, true
			}
			if !emitStart() {
				return -1, false
			}
			thinkingIndex = len(partial.Content)
			partial.Content = append(partial.Content, ai.StreamContentBlock{Type: ai.BlockThinking})
			if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalThinkingStart, ContentIndex: thinkingIndex, Block: &partial.Content[thinkingIndex], Partial: partial}) {
				return -1, false
			}
			return thinkingIndex, true
		}
		ensureToolBlock := func(index int) (int, bool) {
			if existing, ok := toolBlockByIndex[index]; ok && existing >= 0 && existing < len(partial.Content) {
				return existing, true
			}
			if !emitStart() {
				return -1, false
			}
			blockIndex := len(partial.Content)
			partial.Content = append(partial.Content, ai.StreamContentBlock{Type: ai.BlockToolCall, ToolCall: &ai.StreamToolCall{}})
			toolBlockByIndex[index] = blockIndex
			if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalToolCallStart, ContentIndex: blockIndex, Block: &partial.Content[blockIndex], Partial: partial}) {
				return -1, false
			}
			return blockIndex, true
		}

		for stream.Next() {
			chunk := stream.Current()
			_ = accumulator.AddChunk(chunk)
			for _, choice := range chunk.Choices {
				if delta := choice.Delta.Content; delta != "" {
					if filterTaggedReasoning {
						visibleDelta, taggedReasoningDelta := contentFilter.Consume(delta)
						if visibleDelta != "" {
							index, ok := ensureTextBlock()
							if !ok {
								return
							}
							partial.Content[index].Text += visibleDelta
							if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalTextDelta, ContentIndex: index, Delta: visibleDelta, Block: &partial.Content[index], Partial: partial}) {
								return
							}
						}
						if taggedReasoningDelta != "" {
							taggedReasoningBuilder.WriteString(taggedReasoningDelta)
							if !sawExplicitReasoning {
								index, ok := ensureThinkingBlock()
								if !ok {
									return
								}
								partial.Content[index].Text += taggedReasoningDelta
								if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalThinkingDelta, ContentIndex: index, Delta: taggedReasoningDelta, Block: &partial.Content[index], Partial: partial}) {
									return
								}
							}
						}
					} else {
						index, ok := ensureTextBlock()
						if !ok {
							return
						}
						partial.Content[index].Text += delta
						if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalTextDelta, ContentIndex: index, Delta: delta, Block: &partial.Content[index], Partial: partial}) {
							return
						}
					}
				}
				if field, reasoningDelta, ok := extractReasoningDelta(choice.RawJSON(), "delta."); ok {
					sawExplicitReasoning = true
					if reasoningReplayField == "" {
						reasoningReplayField = field
					}
					explicitReasoningBuilder.WriteString(reasoningDelta)
					index, ok := ensureThinkingBlock()
					if !ok {
						return
					}
					if partial.Content[index].ThinkingReplayField == "" {
						partial.Content[index].ThinkingReplayField = reasoningReplayField
					}
					partial.Content[index].Text += reasoningDelta
					if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalThinkingDelta, ContentIndex: index, Delta: reasoningDelta, Block: &partial.Content[index], Partial: partial}) {
						return
					}
				}
				if sig, ok := extractStringDeltaFromJSON(choice.RawJSON(), "delta.reasoning_signature"); ok {
					reasoningSignatureBuilder.WriteString(sig)
				}
				for _, call := range choice.Delta.ToolCalls {
					index := int(call.Index)
					toolCall := toolCallsByIndex[index]
					if toolCall == nil {
						toolCall = &openAICompatToolCall{Type: "function"}
						toolCallsByIndex[index] = toolCall
					}
					if id := strings.TrimSpace(call.ID); id != "" {
						toolCall.ID = id
					}
					if name := strings.TrimSpace(call.Function.Name); name != "" {
						toolCall.Name = name
					}
					if typ := strings.TrimSpace(call.Type); typ != "" {
						toolCall.Type = typ
					}
					blockIndex, ok := ensureToolBlock(index)
					if !ok {
						return
					}
					block := &partial.Content[blockIndex]
					if block.ToolCall == nil {
						block.ToolCall = &ai.StreamToolCall{}
					}
					block.ToolCall.ID = toolCall.ID
					block.ToolCall.Name = toolCall.Name
					if typ := strings.TrimSpace(toolCall.Type); typ == "" {
						block.ToolCall.Raw = nil
					}
					if delta := call.Function.Arguments; delta != "" {
						builder := toolArgsByIndex[index]
						if builder == nil {
							builder = &strings.Builder{}
							toolArgsByIndex[index] = builder
						}
						builder.WriteString(delta)
						rawArguments := builder.String()
						toolCall.Arguments = providerToolArgumentsMap(builder.String())
						toolCall.RawArguments = rawArguments
						block.ToolCall.RawArguments = rawArguments
						block.ToolCall.Arguments = ai.CloneToolArguments(toolCall.Arguments)
						if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalToolCallDelta, ContentIndex: blockIndex, Delta: delta, Block: block, Partial: partial}) {
							return
						}
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			out.Finish(fmt.Errorf("stream error: %w", err))
			return
		}
		if len(accumulator.Choices) == 0 {
			out.Finish(fmt.Errorf("empty model response"))
			return
		}

		choice := accumulator.Choices[0]
		content := choice.Message.Content
		taggedReasoning := ""
		if filterTaggedReasoning {
			content, taggedReasoning, _ = sanitizeTaggedAssistantContent(choice.Message.Content)
		}
		reasoning := strings.TrimSpace(explicitReasoningBuilder.String())
		if reasoning == "" {
			field, extracted := extractReasoningWithReplayField(choice.Message.RawJSON(), "")
			if reasoningReplayField == "" {
				reasoningReplayField = field
			}
			reasoning = extracted
		}
		if reasoning == "" {
			field, extracted := extractReasoningWithReplayField(choice.RawJSON(), "message.")
			if reasoningReplayField == "" {
				reasoningReplayField = field
			}
			reasoning = extracted
		}
		if reasoning == "" {
			reasoning = strings.TrimSpace(taggedReasoningBuilder.String())
			reasoningReplayField = ""
		}
		if reasoning == "" {
			reasoning = strings.TrimSpace(taggedReasoning)
			reasoningReplayField = ""
		}
		reasoningSignature := strings.TrimSpace(reasoningSignatureBuilder.String())
		if reasoningSignature == "" {
			reasoningSignature = extractReasoningSignature(choice.Message.RawJSON())
		}
		if reasoningSignature == "" {
			reasoningSignature = extractStringFromJSON(choice.RawJSON(), "message.reasoning_signature")
		}
		if textIndex >= 0 && textIndex < len(partial.Content) {
			partial.Content[textIndex].Text = content
			if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalTextEnd, ContentIndex: textIndex, Content: content, Block: &partial.Content[textIndex], Partial: partial}) {
				return
			}
		}
		if thinkingIndex >= 0 && thinkingIndex < len(partial.Content) {
			partial.Content[thinkingIndex].Text = reasoning
			partial.Content[thinkingIndex].ThinkingReplayField = reasoningReplayField
			partial.Content[thinkingIndex].ThinkingSignature = reasoningSignature
			if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalThinkingEnd, ContentIndex: thinkingIndex, Content: reasoning, Block: &partial.Content[thinkingIndex], Partial: partial}) {
				return
			}
		}

		collectedToolCalls := convertOpenAIToolCalls(choice.Message.ToolCalls)
		if len(collectedToolCalls) == 0 && len(toolCallsByIndex) > 0 {
			indexes := make([]int, 0, len(toolCallsByIndex))
			for index := range toolCallsByIndex {
				indexes = append(indexes, index)
			}
			sort.Ints(indexes)
			for _, index := range indexes {
				toolCall := toolCallsByIndex[index]
				if toolCall == nil || strings.TrimSpace(toolCall.Name) == "" {
					continue
				}
				collectedToolCalls = append(collectedToolCalls, *toolCall)
			}
		}
		for index, toolCall := range collectedToolCalls {
			blockIndex, ok := ensureToolBlock(index)
			if !ok {
				return
			}
			block := &partial.Content[blockIndex]
			if block.ToolCall == nil {
				block.ToolCall = &ai.StreamToolCall{}
			}
			block.ToolCall.ID = strings.TrimSpace(toolCall.ID)
			block.ToolCall.Name = strings.TrimSpace(toolCall.Name)
			block.ToolCall.Arguments = ai.CloneToolArguments(toolCall.Arguments)
			block.ToolCall.RawArguments = firstNonEmptyString(
				strings.TrimSpace(toolCall.RawArguments),
				block.ToolCall.RawArguments,
				providerToolArgumentsJSON(toolCall.Arguments),
			)
			block.ToolCall.Complete = true
			if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalToolCallEnd, ContentIndex: blockIndex, Block: block, Partial: partial}) {
				return
			}
		}

		stopReason := mapStopReason(choice.FinishReason)
		if len(collectedToolCalls) > 0 && stopReason == ai.StopReasonStop {
			stopReason = ai.StopReasonToolUse
		}
		usage := toUsage(accumulator.Usage)
		response := providerResponseFromOpenAICompatPartial(partial, usage, stopReason)
		if !out.Emit(ai.ProviderEvent{Type: ai.EventCanonicalDone, Response: response}) {
			return
		}
		out.Close()
	}()
	return out, nil
}

func (p *Provider) buildRequest(req *ai.ProviderRequest) (openai.ChatCompletionNewParams, error) {
	params, err := providerRequestFromCanonical(req, p.cfg)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	messages, err := convertMessagesToOpenAI(params.Messages)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	modelName := providerModelName(params.Model, p.cfg)
	if modelName == "" {
		return openai.ChatCompletionNewParams{}, fmt.Errorf("model name is empty")
	}

	request := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(modelName),
		Messages: messages,
	}
	if tools := convertToolSpecsForOpenAI(params.Tools); len(tools) > 0 {
		request.Tools = tools
	}
	applyOptions(&request, params.Options, p.cfg)
	return request, nil
}

func applyOptions(req *openai.ChatCompletionNewParams, options providerRequestOptions, cfg *op.ModelConfig) {
	hasTools := len(req.Tools) > 0
	maxTokens := int64(0)
	if options.MaxTokens != nil {
		maxTokens = *options.MaxTokens
	} else if cfg != nil && cfg.MaxOutputTokens > 0 {
		maxTokens = cfg.MaxOutputTokens
	}
	if maxTokens > 0 {
		if useLegacyMaxTokensField(cfg) {
			req.MaxTokens = param.NewOpt(maxTokens)
		} else {
			req.MaxCompletionTokens = param.NewOpt(maxTokens)
		}
	}
	if options.Temperature != nil {
		req.Temperature = param.NewOpt(*options.Temperature)
	}
	if enabled := options.ReasoningEnabled; enabled != nil && cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.ReasoningControl), "toggle") {
		req.SetExtraFields(map[string]any{
			"enable_thinking": *enabled,
		})
	}
	if effort := strings.TrimSpace(options.ReasoningEffort); effort != "" {
		req.ReasoningEffort = shared.ReasoningEffort(effort)
	}
	toolChoice := strings.TrimSpace(options.ToolChoice)
	if toolChoice == "" && hasTools {
		toolChoice = "auto"
	}
	if toolChoice != "" {
		req.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String(toolChoice)}
	}
}

func useLegacyMaxTokensField(cfg *op.ModelConfig) bool {
	if cfg == nil {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	baseURL := strings.ToLower(strings.TrimSpace(cfg.BaseURL))
	return provider == "mistral" ||
		strings.Contains(baseURL, "mistral.ai") ||
		strings.Contains(baseURL, "chutes.ai")
}

func normalizeReasoningReplayField(field string) string {
	switch strings.TrimSpace(field) {
	case "reasoning_content", "reasoning", "reasoning_text":
		return strings.TrimSpace(field)
	default:
		return ""
	}
}

func reasoningExtraFieldsForReplay(msg op.Message) map[string]any {
	reasoning := strings.TrimSpace(msg.ReasoningContent)
	if reasoning == "" {
		return nil
	}
	field := normalizeReasoningReplayField(msg.ReasoningReplayField)
	if field == "" {
		return nil
	}
	extraFields := map[string]any{field: reasoning}
	if signature := strings.TrimSpace(msg.ReasoningSignature); signature != "" {
		extraFields["reasoning_signature"] = signature
	}
	return extraFields
}

func convertMessagesToOpenAI(msgs []op.Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	converted := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {
		case op.RoleSystem:
			if textParts := convertTextParts(msg.ContentParts); len(textParts) > 0 {
				converted = append(converted, openai.SystemMessage(textParts))
			} else {
				converted = append(converted, openai.SystemMessage(msg.Content))
			}
		case op.RoleDeveloper:
			if textParts := convertTextParts(msg.ContentParts); len(textParts) > 0 {
				converted = append(converted, openai.DeveloperMessage(textParts))
			} else {
				converted = append(converted, openai.DeveloperMessage(msg.Content))
			}
		case op.RoleUser:
			if userParts := convertUserParts(msg.ContentParts); len(userParts) > 0 {
				converted = append(converted, openai.UserMessage(userParts))
			} else {
				converted = append(converted, openai.UserMessage(msg.Content))
			}
		case op.RoleAssistant:
			if strings.TrimSpace(msg.Content) == "" && len(msg.ContentParts) == 0 && len(msg.ToolCalls) == 0 {
				// Guard against malformed history entries that providers reject.
				continue
			}
			assistant := openai.ChatCompletionAssistantMessageParam{
				ToolCalls: convertAssistantToolCalls(msg.ToolCalls),
			}
			content := strings.TrimSpace(msg.Content)
			if content == "" {
				content = joinTextParts(msg.ContentParts)
			}
			if content != "" {
				assistant.Content.OfString = openai.String(content)
			}
			if extraFields := reasoningExtraFieldsForReplay(msg); len(extraFields) > 0 {
				assistant.SetExtraFields(extraFields)
			}
			converted = append(converted, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &assistant,
			})
		case op.RoleTool:
			if textParts := convertTextParts(msg.ContentParts); len(textParts) > 0 {
				converted = append(converted, openai.ToolMessage(textParts, msg.ToolCallID))
			} else {
				converted = append(converted, openai.ToolMessage(msg.Content, msg.ToolCallID))
			}
			if imageParts := convertImageUserParts(msg.ContentParts); len(imageParts) > 0 {
				converted = append(converted, openai.UserMessage(imageParts))
			}
		case op.RoleFunction:
			converted = append(converted, openai.ChatCompletionMessageParamOfFunction(msg.Content, msg.Name))
		default:
			return nil, fmt.Errorf("unsupported message role: %q", msg.Role)
		}
	}
	return converted, nil
}

func convertUserParts(parts []op.ContentPart) []openai.ChatCompletionContentPartUnionParam {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
	for _, part := range parts {
		typ := strings.ToLower(strings.TrimSpace(part.Type))
		switch typ {
		case "", "text":
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			converted = append(converted, openai.TextContentPart(text))
		case "image", "image_url":
			if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
				continue
			}
			converted = append(converted, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
				URL:    part.ImageURL.URL,
				Detail: strings.TrimSpace(part.ImageURL.Detail),
			}))
		}
	}
	return converted
}

func convertTextParts(parts []op.ContentPart) []openai.ChatCompletionContentPartTextParam {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]openai.ChatCompletionContentPartTextParam, 0, len(parts))
	for _, part := range parts {
		typ := strings.ToLower(strings.TrimSpace(part.Type))
		if typ != "" && typ != "text" {
			continue
		}
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		converted = append(converted, openai.ChatCompletionContentPartTextParam{
			Text: text,
		})
	}
	return converted
}

func convertImageUserParts(parts []op.ContentPart) []openai.ChatCompletionContentPartUnionParam {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
	for _, part := range parts {
		typ := strings.ToLower(strings.TrimSpace(part.Type))
		if typ != "image" && typ != "image_url" {
			continue
		}
		if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
			continue
		}
		converted = append(converted, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
			URL:    part.ImageURL.URL,
			Detail: strings.TrimSpace(part.ImageURL.Detail),
		}))
	}
	return converted
}

func joinTextParts(parts []op.ContentPart) string {
	if len(parts) == 0 {
		return ""
	}
	builder := strings.Builder{}
	for _, part := range parts {
		typ := strings.ToLower(strings.TrimSpace(part.Type))
		if typ != "" && typ != "text" {
			continue
		}
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(text)
	}
	return builder.String()
}

func convertAssistantToolCalls(calls []op.MessageToolCall) []openai.ChatCompletionMessageToolCallUnionParam {
	if len(calls) == 0 {
		return nil
	}
	converted := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for _, call := range calls {
		if call.ID == "" || call.Name == "" {
			continue
		}
		paramCall := openai.ChatCompletionMessageFunctionToolCallParam{
			ID: call.ID,
			Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
				Name:      call.Name,
				Arguments: providerToolArgumentsJSON(call.Arguments),
			},
		}
		converted = append(converted, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &paramCall,
		})
	}
	return converted
}

func convertToolSpecsForOpenAI(toolSpecs []op.ToolSpec) []openai.ChatCompletionToolUnionParam {
	if len(toolSpecs) == 0 {
		return nil
	}
	converted := make([]openai.ChatCompletionToolUnionParam, 0, len(toolSpecs))
	for _, spec := range toolSpecs {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		fn := openai.FunctionDefinitionParam{Name: name}
		if desc := strings.TrimSpace(spec.Description); desc != "" {
			fn.Description = openai.String(desc)
		}
		if params := normalizeSchema(spec.InputSchema); len(params) > 0 {
			fn.Parameters = params
		}
		converted = append(converted, openai.ChatCompletionFunctionTool(fn))
	}
	return converted
}

func providerResponseFromOpenAICompatPartial(partial *ai.StreamConversationMessage, usage ai.Usage, stopReason ai.StopReason) *ai.ProviderResponse {
	return &ai.ProviderResponse{
		Message:    ai.FinalizeStreamConversationMessage(partial),
		Usage:      usage,
		StopReason: stopReason,
	}
}

func providerResponseFromOpenAICompat(
	content string,
	reasoning string,
	reasoningReplayField string,
	reasoningSignature string,
	toolCalls []openAICompatToolCall,
	usage ai.Usage,
	stopReason ai.StopReason,
) *ai.ProviderResponse {
	blocks := make([]ai.ContentBlock, 0, 2+len(toolCalls))
	if reasoningText := strings.TrimSpace(reasoning); reasoningText != "" {
		blocks = append(blocks, ai.ContentBlock{
			Type:                ai.BlockThinking,
			Text:                reasoningText,
			ThinkingReplayField: strings.TrimSpace(reasoningReplayField),
			ThinkingSignature:   strings.TrimSpace(reasoningSignature),
		})
	}
	if text := strings.TrimSpace(content); text != "" {
		blocks = append(blocks, ai.ContentBlock{Type: ai.BlockText, Text: text})
	}
	for _, call := range toolCalls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		args := ai.CloneToolArguments(call.Arguments)
		rawArguments := strings.TrimSpace(call.RawArguments)
		if rawArguments == "" {
			rawArguments = providerToolArgumentsJSON(args)
		}
		blocks = append(blocks, ai.ContentBlock{
			Type: ai.BlockToolCall,
			ToolCall: &ai.CanonicalToolCall{
				ID:           strings.TrimSpace(call.ID),
				Name:         name,
				RawArguments: rawArguments,
				Arguments:    args,
			},
		})
	}
	return &ai.ProviderResponse{
		Message: ai.ConversationMessage{
			Role:       ai.RoleCanonicalAssistant,
			Content:    blocks,
			Usage:      ai.MessageUsageFromUsage(usage),
			StopReason: stopReason,
		},
		Usage:      usage,
		StopReason: stopReason,
	}
}

func opToolCallsFromCompat(calls []openAICompatToolCall) []op.MessageToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]op.MessageToolCall, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		out = append(out, op.MessageToolCall{
			ID:        strings.TrimSpace(call.ID),
			Name:      name,
			Arguments: ai.CloneToolArguments(call.Arguments),
			Type:      firstNonEmptyString(strings.TrimSpace(call.Type), "function"),
		})
	}
	return out
}

func convertOpenAIToolCalls(calls []openai.ChatCompletionMessageToolCallUnion) []openAICompatToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]openAICompatToolCall, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		callType := strings.TrimSpace(call.Type)
		if callType == "" {
			callType = "function"
		}
		rawArguments := strings.TrimSpace(call.Function.Arguments)
		out = append(out, openAICompatToolCall{
			ID:           strings.TrimSpace(call.ID),
			Name:         name,
			Arguments:    providerToolArgumentsMap(rawArguments),
			RawArguments: rawArguments,
			Type:         callType,
		})
	}
	return out
}

func mapStopReason(reason string) ai.StopReason {
	switch reason {
	case "stop", "":
		return ai.StopReasonStop
	case "length":
		return ai.StopReasonLength
	case "tool_calls", "function_call":
		return ai.StopReasonToolUse
	default:
		return ai.StopReasonError
	}
}

func toUsage(usage openai.CompletionUsage) ai.Usage {
	cacheReadTokens := usage.PromptTokensDetails.CachedTokens
	cacheWriteTokens := gjson.Get(usage.RawJSON(), "prompt_tokens_details.cache_write_tokens").Int()
	if cacheWriteTokens < 0 {
		cacheWriteTokens = 0
	}
	if cacheReadTokens > 0 && cacheWriteTokens > 0 && cacheReadTokens >= cacheWriteTokens {
		cacheReadTokens -= cacheWriteTokens
	}
	return ai.Usage{
		InputTokens:      maxInt64(0, usage.PromptTokens-cacheReadTokens-cacheWriteTokens),
		OutputTokens:     usage.CompletionTokens,
		CacheReadTokens:  cacheReadTokens,
		CacheWriteTokens: cacheWriteTokens,
		TotalTokens:      firstNonZeroInt64(usage.TotalTokens, usage.PromptTokens+usage.CompletionTokens),
	}
}

var openAICompatReasoningFields = []string{"reasoning_content", "reasoning", "reasoning_text"}

func extractReasoningContent(raw string) string {
	_, reasoning := extractReasoningWithReplayField(raw, "")
	return reasoning
}

func extractReasoningWithReplayField(raw string, pathPrefix string) (string, string) {
	pathPrefix = strings.TrimSpace(pathPrefix)
	for _, field := range openAICompatReasoningFields {
		path := field
		if pathPrefix != "" {
			path = pathPrefix + field
		}
		if reasoning := extractReasoningFromJSON(raw, path); reasoning != "" {
			return field, reasoning
		}
		if reasoningFieldExists(raw, path) {
			return field, ""
		}
	}
	return "", ""
}

func extractReasoningDelta(raw string, pathPrefix string) (string, string, bool) {
	pathPrefix = strings.TrimSpace(pathPrefix)
	for _, field := range openAICompatReasoningFields {
		path := field
		if pathPrefix != "" {
			path = pathPrefix + field
		}
		if delta, ok := extractReasoningDeltaFromJSON(raw, path); ok {
			return field, delta, true
		}
	}
	return "", "", false
}

func reasoningFieldExists(raw string, path string) bool {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(path) == "" {
		return false
	}
	return gjson.Get(raw, path).Exists()
}

func extractReasoningSignature(raw string) string {
	return extractStringFromJSON(raw, "reasoning_signature")
}

// extractReasoningDeltaFromJSON reads a reasoning field at path without trimming,
// so streaming deltas (e.g. " are") keep leading spaces and concatenate correctly.
// Returns (value, true) when the field exists, ("", false) when missing.
func extractReasoningDeltaFromJSON(raw string, path string) (string, bool) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(path) == "" {
		return "", false
	}
	result := gjson.Get(raw, path)
	if !result.Exists() {
		return "", false
	}
	if result.Type == gjson.String {
		return result.String(), true
	}
	if result.IsArray() {
		parts := make([]string, 0, len(result.Array()))
		for _, item := range result.Array() {
			switch {
			case item.Type == gjson.String:
				parts = append(parts, item.String())
			case item.IsObject():
				parts = append(parts, item.Get("text").String())
			}
		}
		return strings.Join(parts, ""), true
	}
	return "", false
}

func extractStringDeltaFromJSON(raw string, path string) (string, bool) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(path) == "" {
		return "", false
	}
	result := gjson.Get(raw, path)
	if !result.Exists() || result.Type != gjson.String {
		return "", false
	}
	return result.String(), true
}

func extractReasoningFromJSON(raw string, path string) string {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(path) == "" {
		return ""
	}
	result := gjson.Get(raw, path)
	if !result.Exists() {
		return ""
	}
	if result.Type == gjson.String {
		return strings.TrimSpace(result.String())
	}
	if result.IsArray() {
		parts := make([]string, 0, len(result.Array()))
		for _, item := range result.Array() {
			switch {
			case item.Type == gjson.String:
				if text := strings.TrimSpace(item.String()); text != "" {
					parts = append(parts, text)
				}
			case item.IsObject():
				if text := strings.TrimSpace(item.Get("text").String()); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}
	return ""
}

func extractStringFromJSON(raw string, path string) string {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(path) == "" {
		return ""
	}
	result := gjson.Get(raw, path)
	if !result.Exists() || result.Type != gjson.String {
		return ""
	}
	return strings.TrimSpace(result.String())
}
