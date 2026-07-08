package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/openai/openai-go/v3/shared/constant"
)

func (p *ResponsesProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-responses")
}

func (p *ResponsesProvider) CompleteCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("canonical request is nil")
	}
	prepared := prepareCanonicalReplayForProvider(req, p.cfg, "openai-responses")
	result, err := p.CompleteResponses(ctx, canonicalRequestToResponses(prepared))
	if err != nil {
		return nil, err
	}
	return ai.ProviderResponseFromResponsesResult(result), nil
}

func (p *ResponsesProvider) StreamCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req == nil {
		return nil, fmt.Errorf("canonical request is nil")
	}
	return streamCanonicalFromResponsesProvider(ctx, req, p.cfg, p)
}

func streamCanonicalFromResponsesProvider(ctx context.Context, req *ai.ProviderRequest, cfg *op.ModelConfig, respProvider ai.ResponsesProvider) (*ai.ProviderEventStream, error) {
	prepared := prepareCanonicalReplayForProvider(req, cfg, "openai-responses")
	respStream, err := respProvider.StreamResponses(ctx, canonicalRequestToResponses(prepared))
	if err != nil {
		return nil, err
	}
	out := ai.NewProviderEventStream(128)
	go func() {
		defer respStream.Close()
		partial := &ai.StreamConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			ProviderState: &ai.ProviderState{
				Provider: strings.TrimSpace(cfg.Provider),
				API:      "openai-responses",
				Model:    strings.TrimSpace(req.Config.Model),
			},
		}
		itemToIndex := make(map[string]int)
		for respStream.Next() {
			event := respStream.Event()
			switch event.Type {
			case "response.created":
				if event.Response != nil {
					partial.ProviderState.Model = firstNonEmptyString(strings.TrimSpace(event.Response.Model), partial.ProviderState.Model)
					partial.ProviderState.ResponseID = strings.TrimSpace(event.Response.ID)
				}
				if !out.Emit(ai.ProviderEvent{
					Type:    ai.EventCanonicalStart,
					Partial: partial,
					Raw:     event.Raw,
				}) {
					return
				}
			case "response.output_item.added":
				item := event.Item
				if item == nil {
					item = responseItemFromEventRaw(event.Raw)
				}
				key := responseEventItemKey(item, event.Raw)
				if item == nil || key == "" {
					continue
				}
				switch strings.TrimSpace(item.Type) {
				case "reasoning":
					index := len(partial.Content)
					itemToIndex[key] = index
					partial.Content = append(partial.Content, ai.StreamContentBlock{
						Type:             ai.BlockThinking,
						EncryptedContent: strings.TrimSpace(item.EncryptedContent),
						Raw:              append(json.RawMessage(nil), item.Raw...),
					})
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalThinkingStart,
						ContentIndex: index,
						Block:        &partial.Content[index],
						Partial:      partial,
						Raw:          event.Raw,
					}) {
						return
					}
				case "message":
					if strings.ToLower(strings.TrimSpace(item.Role)) != "assistant" {
						continue
					}
					index := len(partial.Content)
					itemToIndex[key] = index
					partial.Content = append(partial.Content, ai.StreamContentBlock{
						Type: ai.BlockText,
						Raw:  append(json.RawMessage(nil), item.Raw...),
					})
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalTextStart,
						ContentIndex: index,
						Block:        &partial.Content[index],
						Partial:      partial,
						Raw:          event.Raw,
					}) {
						return
					}
				case "function_call":
					index := len(partial.Content)
					itemToIndex[key] = index
					block := ai.StreamContentBlock{
						Type: ai.BlockToolCall,
						ToolCall: &ai.StreamToolCall{
							ID:           strings.TrimSpace(item.CallID),
							Name:         strings.TrimSpace(item.Name),
							RawArguments: strings.TrimSpace(item.Arguments),
							Raw:          append(json.RawMessage(nil), item.Raw...),
						},
						Raw: append(json.RawMessage(nil), item.Raw...),
					}
					if args, ok := parseToolArgumentsJSON(block.ToolCall.RawArguments); ok {
						block.ToolCall.Arguments = args
					}
					partial.Content = append(partial.Content, block)
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalToolCallStart,
						ContentIndex: index,
						Block:        &partial.Content[index],
						Partial:      partial,
						Raw:          event.Raw,
					}) {
						return
					}
				}
			case "response.output_text.delta", "response.refusal.delta":
				key := responseEventItemKey(nil, event.Raw)
				index, ok := itemToIndex[key]
				if !ok || index < 0 || index >= len(partial.Content) {
					continue
				}
				partial.Content[index].Text += event.Delta
				if !out.Emit(ai.ProviderEvent{
					Type:         ai.EventCanonicalTextDelta,
					ContentIndex: index,
					Delta:        event.Delta,
					Block:        &partial.Content[index],
					Partial:      partial,
					Raw:          event.Raw,
				}) {
					return
				}
			case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
				key := responseEventItemKey(nil, event.Raw)
				index, ok := itemToIndex[key]
				if !ok || index < 0 || index >= len(partial.Content) {
					continue
				}
				partial.Content[index].Text += event.Delta
				if !out.Emit(ai.ProviderEvent{
					Type:         ai.EventCanonicalThinkingDelta,
					ContentIndex: index,
					Delta:        event.Delta,
					Block:        &partial.Content[index],
					Partial:      partial,
					Raw:          event.Raw,
				}) {
					return
				}
			case "response.function_call_arguments.delta":
				key := responseEventItemKey(nil, event.Raw)
				index, ok := itemToIndex[key]
				if !ok || index < 0 || index >= len(partial.Content) {
					continue
				}
				block := &partial.Content[index]
				if block.ToolCall == nil {
					continue
				}
				block.ToolCall.RawArguments += event.Delta
				if args, ok := parseToolArgumentsJSON(block.ToolCall.RawArguments); ok {
					block.ToolCall.Arguments = args
				}
				if !out.Emit(ai.ProviderEvent{
					Type:         ai.EventCanonicalToolCallDelta,
					ContentIndex: index,
					Delta:        event.Delta,
					Block:        block,
					Partial:      partial,
					Raw:          event.Raw,
				}) {
					return
				}
			case "response.function_call_arguments.done":
				key := responseEventItemKey(nil, event.Raw)
				index, ok := itemToIndex[key]
				if !ok || index < 0 || index >= len(partial.Content) {
					continue
				}
				block := &partial.Content[index]
				if block.ToolCall == nil {
					continue
				}
				block.ToolCall.RawArguments = strings.TrimSpace(event.Delta)
				if args, ok := parseToolArgumentsJSON(block.ToolCall.RawArguments); ok {
					block.ToolCall.Arguments = args
				}
			case "response.output_item.done":
				item := event.Item
				if item == nil {
					item = responseItemFromEventRaw(event.Raw)
				}
				key := responseEventItemKey(item, event.Raw)
				index, ok := itemToIndex[key]
				if item == nil || !ok || index < 0 || index >= len(partial.Content) {
					continue
				}
				block := &partial.Content[index]
				switch strings.TrimSpace(item.Type) {
				case "reasoning":
					block.Text = responseReasoningText(item)
					block.ThinkingSignature = strings.TrimSpace(item.ID)
					block.EncryptedContent = firstNonEmptyString(strings.TrimSpace(item.EncryptedContent), block.EncryptedContent)
					block.Raw = append(json.RawMessage(nil), item.Raw...)
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalThinkingEnd,
						ContentIndex: index,
						Content:      block.Text,
						Block:        block,
						Partial:      partial,
						Raw:          event.Raw,
					}) {
						return
					}
				case "message":
					block.Text = responseMessageText(item)
					block.TextSignature = strings.TrimSpace(item.ID)
					block.Raw = append(json.RawMessage(nil), item.Raw...)
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalTextEnd,
						ContentIndex: index,
						Content:      block.Text,
						Block:        block,
						Partial:      partial,
						Raw:          event.Raw,
					}) {
						return
					}
				case "function_call":
					if block.ToolCall == nil {
						block.ToolCall = &ai.StreamToolCall{}
					}
					block.ToolCall.ID = strings.TrimSpace(item.CallID)
					block.ToolCall.Name = strings.TrimSpace(item.Name)
					block.ToolCall.RawArguments = strings.TrimSpace(item.Arguments)
					if args, ok := parseToolArgumentsJSON(block.ToolCall.RawArguments); ok {
						block.ToolCall.Arguments = args
					}
					block.ToolCall.Complete = true
					block.ToolCall.Raw = append(json.RawMessage(nil), item.Raw...)
					block.Raw = append(json.RawMessage(nil), item.Raw...)
					if !out.Emit(ai.ProviderEvent{
						Type:         ai.EventCanonicalToolCallEnd,
						ContentIndex: index,
						Block:        block,
						Partial:      partial,
						Raw:          event.Raw,
					}) {
						return
					}
				}
			case "response.completed":
				if event.Response != nil {
					mergeResponsesCompletedOutputIntoPartial(partial, event.Response)
					finalResponse := providerResponseFromResponsesStreamPartial(partial, event.Response, cfg, req)
					if finalResponse != nil {
						out.Emit(ai.ProviderEvent{
							Type:     ai.EventCanonicalDone,
							Response: finalResponse,
							Raw:      event.Raw,
						})
					}
				}
				out.Close()
				return
			case "response.failed":
				out.Finish(event.Error)
				return
			}
		}
		if err := respStream.Err(); err != nil {
			out.Finish(err)
			return
		}
		out.Close()
	}()
	return out, nil
}

func mergeResponsesCompletedOutputIntoPartial(partial *ai.StreamConversationMessage, result *ai.ResponsesResult) {
	if partial == nil || result == nil || len(result.Output) == 0 {
		return
	}
	completed := ai.ProviderResponseFromResponsesResult(result)
	for _, block := range completed.Message.Content {
		mergeResponsesCompletedBlockIntoPartial(partial, block)
	}
}

func mergeResponsesCompletedBlockIntoPartial(partial *ai.StreamConversationMessage, block ai.ContentBlock) {
	switch block.Type {
	case ai.BlockText:
		if strings.TrimSpace(block.Text) == "" {
			return
		}
		if index := findResponsesPartialTextBlock(partial, block); index >= 0 {
			partial.Content[index] = ai.StreamContentBlockFromCanonical(block)
			return
		}
	case ai.BlockThinking:
		if strings.TrimSpace(block.Text) == "" && strings.TrimSpace(block.EncryptedContent) == "" && len(block.Raw) == 0 {
			return
		}
		if index := findResponsesPartialThinkingBlock(partial, block); index >= 0 {
			partial.Content[index] = ai.StreamContentBlockFromCanonical(block)
			return
		}
	case ai.BlockToolCall:
		if block.ToolCall == nil || strings.TrimSpace(block.ToolCall.Name) == "" {
			return
		}
		if index := findResponsesPartialToolCallBlock(partial, block); index >= 0 {
			partial.Content[index] = ai.StreamContentBlockFromCanonical(block)
			return
		}
	default:
		return
	}
	partial.Content = append(partial.Content, ai.StreamContentBlockFromCanonical(block))
}

func findResponsesPartialTextBlock(partial *ai.StreamConversationMessage, block ai.ContentBlock) int {
	firstText := -1
	for index, existing := range partial.Content {
		if existing.Type != ai.BlockText {
			continue
		}
		if firstText < 0 {
			firstText = index
		}
		if strings.TrimSpace(block.TextSignature) != "" && strings.TrimSpace(existing.TextSignature) == strings.TrimSpace(block.TextSignature) {
			return index
		}
		if strings.TrimSpace(existing.Text) != "" {
			return index
		}
	}
	return firstText
}

func findResponsesPartialThinkingBlock(partial *ai.StreamConversationMessage, block ai.ContentBlock) int {
	for index, existing := range partial.Content {
		if existing.Type != ai.BlockThinking {
			continue
		}
		if strings.TrimSpace(block.ThinkingSignature) != "" && strings.TrimSpace(existing.ThinkingSignature) == strings.TrimSpace(block.ThinkingSignature) {
			return index
		}
		return index
	}
	return -1
}

func findResponsesPartialToolCallBlock(partial *ai.StreamConversationMessage, block ai.ContentBlock) int {
	if block.ToolCall == nil {
		return -1
	}
	callID := strings.TrimSpace(block.ToolCall.ID)
	name := strings.TrimSpace(block.ToolCall.Name)
	for index, existing := range partial.Content {
		if existing.Type != ai.BlockToolCall || existing.ToolCall == nil {
			continue
		}
		if callID != "" && strings.TrimSpace(existing.ToolCall.ID) == callID {
			return index
		}
		if callID == "" && name != "" && strings.TrimSpace(existing.ToolCall.Name) == name {
			return index
		}
	}
	return -1
}

func annotateResponsesCanonicalResponse(resp *ai.ProviderResponse, result *ai.ResponsesResult, cfg *op.ModelConfig, req *ai.ProviderRequest, partial *ai.StreamConversationMessage) {
	if resp == nil || result == nil {
		return
	}
	if resp.Message.ProviderState == nil {
		resp.Message.ProviderState = &ai.ProviderState{}
	}
	provider := ""
	if cfg != nil {
		provider = strings.TrimSpace(cfg.Provider)
	}
	model := ""
	if req != nil {
		model = strings.TrimSpace(req.Config.Model)
	}
	partialModel := ""
	if partial != nil && partial.ProviderState != nil {
		partialModel = strings.TrimSpace(partial.ProviderState.Model)
	}
	resp.Message.ProviderState.Provider = provider
	resp.Message.ProviderState.API = "openai-responses"
	resp.Message.ProviderState.Model = firstNonEmptyString(strings.TrimSpace(result.Model), resp.Message.ProviderState.Model, partialModel, model)
	resp.Message.ProviderState.ResponseID = strings.TrimSpace(result.ID)
}

func providerResponseFromResponsesStreamPartial(partial *ai.StreamConversationMessage, result *ai.ResponsesResult, cfg *op.ModelConfig, req *ai.ProviderRequest) *ai.ProviderResponse {
	if partial == nil || result == nil {
		return nil
	}
	if partial.ProviderState == nil {
		partial.ProviderState = &ai.ProviderState{}
	}
	provider := ""
	if cfg != nil {
		provider = strings.TrimSpace(cfg.Provider)
	}
	model := ""
	if req != nil {
		model = strings.TrimSpace(req.Config.Model)
	}
	partial.ProviderState.Provider = provider
	partial.ProviderState.API = "openai-responses"
	partial.ProviderState.Model = firstNonEmptyString(strings.TrimSpace(result.Model), partial.ProviderState.Model, model)
	partial.ProviderState.ResponseID = strings.TrimSpace(result.ID)

	message := ai.FinalizeStreamConversationMessage(partial)
	if !ai.HasSemanticConversationMessage(message) {
		return nil
	}
	stopReason := result.StopReason
	if stopReason == "" {
		stopReason = ai.StopReasonStop
	}
	if stopReason == ai.StopReasonStop && canonicalMessageHasToolCall(message) {
		stopReason = ai.StopReasonToolUse
	}
	return &ai.ProviderResponse{
		Message:    message,
		Usage:      result.Usage,
		StopReason: stopReason,
	}
}

func canonicalMessageHasToolCall(msg ai.ConversationMessage) bool {
	for _, block := range msg.Content {
		if block.Type == ai.BlockToolCall && block.ToolCall != nil {
			return true
		}
	}
	return false
}

func (p *ResponsesProvider) CompleteResponses(ctx context.Context, req *ai.ResponsesRequest) (*ai.ResponsesResult, error) {
	params, err := p.buildNativeRequest(req)
	if err != nil {
		return nil, err
	}
	opts := make([]option.RequestOption, 0, 1)
	if requestID := strings.TrimSpace(req.RequestID); requestID != "" {
		opts = append(opts, option.WithHeader("X-Request-ID", requestID))
	}
	resp, err := p.client.Responses.New(ctx, params, opts...)
	if err != nil {
		return nil, normalizeOpenAIResponsesError(err)
	}
	if resp == nil {
		return nil, fmt.Errorf("empty model response")
	}
	return convertResponsesToNativeResult(resp), nil
}

func (p *ResponsesProvider) StreamResponses(ctx context.Context, req *ai.ResponsesRequest) (*ai.ResponsesEventStream, error) {
	normalizedReq := p.normalizeRequestForProvider(req)
	normalizedReq.Stream = true
	if normalizedReq.Store == nil {
		store := false
		normalizedReq.Store = &store
	}
	params, err := p.buildNativeRequest(normalizedReq)
	if err != nil {
		return nil, err
	}
	opts := make([]option.RequestOption, 0, 1)
	if requestID := strings.TrimSpace(normalizedReq.RequestID); requestID != "" {
		opts = append(opts, option.WithHeader("X-Request-ID", requestID))
	}
	sdkStream := p.client.Responses.NewStreaming(ctx, params, opts...)
	out := ai.NewResponsesEventStream(128)
	go func() {
		defer sdkStream.Close()
		completed := false
		for sdkStream.Next() {
			event, err := responsesSDKStreamEventToEvent(sdkStream.Current())
			if err != nil {
				out.Finish(ai.NormalizeRetryError(fmt.Errorf("decode responses sdk stream event: %w", err)))
				return
			}
			if strings.TrimSpace(event.Type) == "" {
				continue
			}
			if isBlankGatewayResponsesFailure(event) {
				continue
			}
			if strings.TrimSpace(event.Type) == "response.failed" {
				if event.Error != nil {
					out.Finish(event.Error)
				} else {
					out.Close()
				}
				return
			}
			if !out.Emit(event) {
				return
			}
			if strings.TrimSpace(event.Type) == "response.completed" {
				completed = true
				out.Close()
				return
			}
		}
		if err := sdkStream.Err(); err != nil {
			out.Finish(normalizeOpenAIResponsesError(err))
			return
		}
		if completed {
			return
		}
		out.Finish(ai.NormalizeRetryError(fmt.Errorf("responses stream ended before response.completed: unexpected EOF")))
	}()
	return out, nil
}

func responsesSDKStreamEventToEvent(event responses.ResponseStreamEventUnion) (ai.ResponsesStreamEvent, error) {
	raw := []byte(event.RawJSON())
	if strings.TrimSpace(event.Type) == "error" {
		detail := firstNonEmptyString(strings.TrimSpace(event.Message), strings.TrimSpace(event.Code))
		if detail == "" {
			detail = "response failed"
		}
		return ai.ResponsesStreamEvent{
			Type: "response.failed",
			Error: ai.WrapRetryError(
				fmt.Errorf("%s", detail),
				0,
				strings.TrimSpace(event.Code),
				detail,
				0,
			),
			Raw: append(json.RawMessage(nil), raw...),
		}, nil
	}
	return ai.ParseResponsesStreamEventJSON(raw)
}

func normalizeOpenAIResponsesError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) && apiErr != nil {
		retryAfterMs := int64(0)
		if apiErr.Response != nil {
			retryAfterMs = ai.ParseRetryAfterHeaders(apiErr.Response.Header)
		}
		code, message := parseOpenAIResponsesErrorBody([]byte(apiErr.RawJSON()))
		dumpCode, dumpMessage := parseOpenAIResponsesErrorBody(httpResponseDumpBody(apiErr.DumpResponse(true)))
		return ai.WrapRetryError(
			err,
			apiErr.StatusCode,
			firstNonEmptyString(strings.TrimSpace(dumpCode), strings.TrimSpace(code), strings.TrimSpace(apiErr.Code), strings.TrimSpace(apiErr.Type)),
			firstNonEmptyString(strings.TrimSpace(dumpMessage), strings.TrimSpace(message), strings.TrimSpace(apiErr.Message)),
			retryAfterMs,
		)
	}
	return ai.NormalizeRetryError(err)
}

func parseOpenAIResponsesErrorBody(raw []byte) (code string, message string) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", ""
	}
	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return "", strings.TrimSpace(plain)
	}
	var payload struct {
		Error   json.RawMessage `json:"error"`
		Code    string          `json:"code"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", trimmed
	}
	var errObj struct {
		Code    string `json:"code"`
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	var errStr string
	if json.Unmarshal(payload.Error, &errObj) == nil && (errObj.Code != "" || errObj.Message != "" || errObj.Type != "") {
		code = firstNonEmptyString(strings.TrimSpace(errObj.Code), strings.TrimSpace(errObj.Type), strings.TrimSpace(payload.Code))
		message = firstNonEmptyString(strings.TrimSpace(errObj.Message), strings.TrimSpace(payload.Message))
	} else if json.Unmarshal(payload.Error, &errStr) == nil && errStr != "" {
		code = firstNonEmptyString(strings.TrimSpace(errStr), strings.TrimSpace(payload.Code))
		message = firstNonEmptyString(strings.TrimSpace(payload.Message), strings.TrimSpace(errStr))
	} else {
		code = strings.TrimSpace(payload.Code)
		message = strings.TrimSpace(payload.Message)
	}
	if message == "" && code == "" {
		return "", trimmed
	}
	if message == "" {
		message = code
	}
	return code, message
}

func httpResponseDumpBody(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	text := string(raw)
	if idx := strings.Index(text, "\r\n\r\n"); idx >= 0 {
		return []byte(text[idx+4:])
	}
	if idx := strings.Index(text, "\n\n"); idx >= 0 {
		return []byte(text[idx+2:])
	}
	return nil
}

func (p *ResponsesProvider) buildNativeRequest(req *ai.ResponsesRequest) (responses.ResponseNewParams, error) {
	normalizedReq := p.normalizeRequestForProvider(req)
	if normalizedReq == nil {
		return responses.ResponseNewParams{}, fmt.Errorf("responses request is nil")
	}
	modelName := strings.TrimSpace(normalizedReq.Model)
	if modelName == "" && p.cfg != nil {
		modelName = strings.TrimSpace(p.cfg.Name)
	}
	if modelName == "" {
		return responses.ResponseNewParams{}, fmt.Errorf("model name is empty")
	}
	normalizedInput := normalizedReq.Input
	input, err := convertNativeResponsesInput(normalizedInput)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(modelName),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
		Store: param.NewOpt(false),
	}
	if strings.TrimSpace(normalizedReq.Instructions) != "" {
		params.Instructions = param.NewOpt(strings.TrimSpace(normalizedReq.Instructions))
	}
	if normalizedReq.MaxOutputTokens != nil {
		params.MaxOutputTokens = param.NewOpt(*normalizedReq.MaxOutputTokens)
	}
	if normalizedReq.Temperature != nil {
		params.Temperature = param.NewOpt(*normalizedReq.Temperature)
	}
	if serviceTier, err := ai.ResponsesAPIServiceTier(normalizedReq.ServiceTier); err != nil {
		return responses.ResponseNewParams{}, err
	} else if serviceTier != "" {
		params.ServiceTier = responses.ResponseNewParamsServiceTier(serviceTier)
	}
	if strings.TrimSpace(normalizedReq.PreviousResponseID) != "" {
		params.PreviousResponseID = param.NewOpt(strings.TrimSpace(normalizedReq.PreviousResponseID))
	}
	if normalizedReq.Store != nil {
		params.Store = param.NewOpt(*normalizedReq.Store)
	}
	if normalizedReq.ParallelToolCalls != nil {
		params.ParallelToolCalls = param.NewOpt(*normalizedReq.ParallelToolCalls)
	}
	if strings.TrimSpace(normalizedReq.PromptCacheKey) != "" {
		params.PromptCacheKey = param.NewOpt(strings.TrimSpace(normalizedReq.PromptCacheKey))
	}
	if normalizedReq.Reasoning != nil {
		params.Reasoning = shared.ReasoningParam{}
		if effort := strings.TrimSpace(normalizedReq.Reasoning.Effort); effort != "" {
			params.Reasoning.Effort = shared.ReasoningEffort(effort)
		}
		if summary := strings.TrimSpace(normalizedReq.Reasoning.Summary); summary != "" {
			params.Reasoning.Summary = shared.ReasoningSummary(summary)
		}
	}
	if len(normalizedReq.Include) > 0 {
		params.Include = make([]responses.ResponseIncludable, 0, len(normalizedReq.Include))
		for _, include := range normalizedReq.Include {
			if trimmed := strings.TrimSpace(include); trimmed != "" {
				params.Include = append(params.Include, responses.ResponseIncludable(trimmed))
			}
		}
	}
	if normalizedReq.Text != nil {
		text := responses.ResponseTextConfigParam{}
		if verbosity := strings.TrimSpace(normalizedReq.Text.Verbosity); verbosity != "" {
			text.Verbosity = responses.ResponseTextConfigVerbosity(verbosity)
		}
		params.Text = text
	}

	if tools, err := convertNativeResponseTools(normalizedReq.Tools); err != nil {
		return responses.ResponseNewParams{}, err
	} else if len(tools) > 0 {
		params.Tools = tools
		if normalizedReq.ParallelToolCalls == nil {
			params.ParallelToolCalls = param.NewOpt(true)
		}
	}
	if toolChoice, ok, err := convertNativeToolChoice(normalizedReq.ToolChoice); err != nil {
		return responses.ResponseNewParams{}, err
	} else if ok {
		params.ToolChoice = toolChoice
	} else if len(params.Tools) > 0 {
		params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
	}
	hasEncryptedReasoningInclude := false
	for _, include := range normalizedReq.Include {
		if strings.TrimSpace(include) == "reasoning.encrypted_content" {
			hasEncryptedReasoningInclude = true
			break
		}
	}
	reasoningEffort := ""
	reasoningSummary := ""
	if normalizedReq.Reasoning != nil {
		reasoningEffort = strings.TrimSpace(normalizedReq.Reasoning.Effort)
		reasoningSummary = strings.TrimSpace(normalizedReq.Reasoning.Summary)
	}
	slog.Info("openai-responses native request built",
		"requestID", strings.TrimSpace(normalizedReq.RequestID),
		"model", modelName,
		"previousResponseID", strings.TrimSpace(normalizedReq.PreviousResponseID),
		"inputItems", len(normalizedInput),
		"inputSummary", summarizeResponseItems(normalizedInput),
		"tools", len(normalizedReq.Tools),
		"serviceTier", strings.TrimSpace(normalizedReq.ServiceTier),
		"reasoningEffort", reasoningEffort,
		"reasoningSummary", reasoningSummary,
		"includeEncryptedReasoning", hasEncryptedReasoningInclude,
	)
	return params, nil
}

const (
	responsesReplayToolOutputMaxLines = 400
	responsesReplayToolOutputMaxBytes = 8 * 1024
	responsesReplayToolOutputTotalMax = 48 * 1024
)

const (
	responsesReplayToolOutputOmittedNotice      = "[Historical tool output omitted for replay due to aggregate replay budget.]"
	responsesReplayToolOutputShortOmittedNotice = "[truncated]"
)

func summarizeResponseItems(items []ai.ResponseItem) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{
			"type": item.Type,
		}
		if strings.TrimSpace(item.Role) != "" {
			entry["role"] = strings.TrimSpace(item.Role)
		}
		if strings.TrimSpace(item.ID) != "" {
			entry["id"] = strings.TrimSpace(item.ID)
		}
		if strings.TrimSpace(item.CallID) != "" {
			entry["callID"] = strings.TrimSpace(item.CallID)
		}
		if strings.TrimSpace(item.Name) != "" {
			entry["name"] = strings.TrimSpace(item.Name)
		}
		if strings.TrimSpace(item.Status) != "" {
			entry["status"] = strings.TrimSpace(item.Status)
		}
		if len(item.Arguments) > 0 {
			entry["argumentsBytes"] = len(item.Arguments)
		}
		if len(item.OutputText) > 0 {
			entry["outputBytes"] = len(item.OutputText)
		}
		if len(item.Content) > 0 {
			entry["contentParts"] = len(item.Content)
		}
		if len(item.Summary) > 0 {
			entry["summaryParts"] = len(item.Summary)
		}
		if strings.TrimSpace(item.EncryptedContent) != "" {
			entry["hasEncryptedContent"] = true
		}
		out = append(out, entry)
	}
	return out
}

func normalizeResponsesReplayInput(items []ai.ResponseItem) []ai.ResponseItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]ai.ResponseItem, len(items))
	copy(out, items)
	remainingToolOutputBytes := responsesReplayToolOutputTotalMax
	for i := len(out) - 1; i >= 0; i-- {
		if strings.TrimSpace(out[i].Type) != "function_call_output" {
			continue
		}
		out[i] = normalizeResponsesReplayToolOutputItem(out[i], &remainingToolOutputBytes)
	}
	return out
}

func normalizeResponsesReplayToolOutputItem(item ai.ResponseItem, remainingBytes *int) ai.ResponseItem {
	normalized := item
	if len(item.OutputContent) > 0 {
		normalized.OutputText = ""
		normalized.OutputContent = normalizeResponsesReplayToolOutputContent(item.OutputContent, remainingBytes)
		return normalized
	}
	output := strings.TrimSpace(item.OutputText)
	if output == "" {
		return normalized
	}

	output = ai.TruncateToolOutputForReplayWithLimits(output, responsesReplayToolOutputMaxLines, responsesReplayToolOutputMaxBytes)
	output = truncateResponsesReplayStringFromEnd(output, responsesReplayToolOutputMaxBytes)
	if remainingBytes == nil {
		normalized.OutputText = output
		normalized.OutputContent = nil
		return normalized
	}

	if *remainingBytes <= 0 {
		normalized.OutputText = ""
		normalized.OutputContent = nil
		return normalized
	}
	if len([]byte(output)) > *remainingBytes {
		switch {
		case len([]byte(responsesReplayToolOutputOmittedNotice)) <= *remainingBytes:
			output = responsesReplayToolOutputOmittedNotice
		case len([]byte(responsesReplayToolOutputShortOmittedNotice)) <= *remainingBytes:
			output = responsesReplayToolOutputShortOmittedNotice
		default:
			output = ai.TruncateToolOutputForReplayWithLimits(output, 1, *remainingBytes)
		}
	}
	if len([]byte(output)) > *remainingBytes {
		output = ""
	}
	if used := len([]byte(output)); used <= *remainingBytes {
		*remainingBytes -= used
	} else {
		*remainingBytes = 0
	}
	normalized.OutputText = output
	normalized.OutputContent = nil
	return normalized
}

func normalizeResponsesReplayToolOutputContent(parts []ai.ResponseContentPart, remainingBytes *int) []ai.ResponseContentPart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]ai.ResponseContentPart, 0, len(parts))
	for _, part := range parts {
		partType := strings.TrimSpace(part.Type)
		switch partType {
		case "", "text", "input_text", "output_text":
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			text = ai.TruncateToolOutputForReplayWithLimits(text, responsesReplayToolOutputMaxLines, responsesReplayToolOutputMaxBytes)
			text = truncateResponsesReplayStringFromEnd(text, responsesReplayToolOutputMaxBytes)
			text = consumeResponsesReplayToolOutputBudget(text, remainingBytes)
			if strings.TrimSpace(text) == "" {
				continue
			}
			out = append(out, ai.ResponseContentPart{Type: firstNonEmptyString(partType, "input_text"), Text: text})
		case "image_url", "input_image":
			if imageURL := strings.TrimSpace(part.ImageURL); imageURL != "" {
				out = append(out, ai.ResponseContentPart{
					Type:     firstNonEmptyString(partType, "input_image"),
					ImageURL: imageURL,
					Detail:   strings.TrimSpace(part.Detail),
				})
			}
		}
	}
	return out
}

func consumeResponsesReplayToolOutputBudget(output string, remainingBytes *int) string {
	output = strings.TrimSpace(output)
	if output == "" || remainingBytes == nil {
		return output
	}
	if *remainingBytes <= 0 {
		return responsesReplayToolOutputOmittedNotice
	}
	if len([]byte(output)) <= *remainingBytes {
		*remainingBytes -= len([]byte(output))
		return output
	}
	switch {
	case len([]byte(responsesReplayToolOutputOmittedNotice)) <= *remainingBytes:
		output = responsesReplayToolOutputOmittedNotice
	case len([]byte(responsesReplayToolOutputShortOmittedNotice)) <= *remainingBytes:
		output = responsesReplayToolOutputShortOmittedNotice
	default:
		output = ai.TruncateToolOutputForReplayWithLimits(output, 1, *remainingBytes)
	}
	if len([]byte(output)) > *remainingBytes {
		return ""
	}
	*remainingBytes -= len([]byte(output))
	return output
}

func truncateResponsesReplayStringFromEnd(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxBytes <= 0 || len([]byte(value)) <= maxBytes {
		return value
	}
	buf := []byte(value)
	start := len(buf) - maxBytes
	for start < len(buf) && (buf[start]&0xc0) == 0x80 {
		start++
	}
	return string(buf[start:])
}

func canonicalRequestToResponses(req *ai.ProviderRequest) *ai.ResponsesRequest {
	if req == nil {
		return &ai.ResponsesRequest{}
	}
	instructionParts := make([]string, 0, 1)
	seenInstructionParts := make(map[string]struct{}, 2)
	appendInstructionPart := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seenInstructionParts[value]; exists {
			return
		}
		seenInstructionParts[value] = struct{}{}
		instructionParts = append(instructionParts, value)
	}
	appendInstructionPart(req.Context.SystemPrompt)

	items := make([]ai.ResponseItem, 0, len(req.Context.Messages))
	for _, msg := range req.Context.Messages {
		if msg.Role == ai.RoleCanonicalAssistant {
			if replayItems := responseItemsFromAssistantMessage(msg); len(replayItems) > 0 {
				items = append(items, replayItems...)
				continue
			}
		}
		switch msg.Role {
		case ai.RoleCanonicalSystem:
			appendInstructionPart(textFromInstructionBlocks(msg.Content))
		case ai.RoleCanonicalDeveloper:
			appendInstructionPart(textFromInstructionBlocks(msg.Content))
		case ai.RoleCanonicalUser:
			items = append(items, ai.ResponseItem{
				Type:    "message",
				Role:    "user",
				Content: blocksToResponseContent(msg.Content),
			})
		case ai.RoleCanonicalAssistant:
			for _, block := range msg.Content {
				switch block.Type {
				case ai.BlockThinking:
					continue
				case ai.BlockText:
					items = append(items, ai.ResponseItem{
						Type:   "message",
						Role:   "assistant",
						ID:     strings.TrimSpace(block.TextSignature),
						Status: "completed",
						Content: []ai.ResponseContentPart{{
							Type: "output_text",
							Text: strings.TrimSpace(block.Text),
						}},
					})
				case ai.BlockToolCall:
					if block.ToolCall == nil {
						continue
					}
					items = append(items, ai.ResponseItem{
						Type:      "function_call",
						CallID:    strings.TrimSpace(block.ToolCall.ID),
						Name:      strings.TrimSpace(block.ToolCall.Name),
						Arguments: firstNonEmptyString(strings.TrimSpace(block.ToolCall.RawArguments), ai.MarshalToolArgumentsJSON(block.ToolCall.Arguments)),
						Status:    "completed",
					})
				}
			}
		case ai.RoleCanonicalTool:
			for _, block := range msg.Content {
				if block.Type != ai.BlockToolResult || block.ToolResult == nil {
					continue
				}
				items = append(items, ai.ResponseItem{
					Type:          "function_call_output",
					CallID:        strings.TrimSpace(block.ToolResult.ToolCallID),
					OutputText:    firstNonEmptyString(strings.TrimSpace(block.ToolResult.OutputText), strings.TrimSpace(block.Text)),
					OutputContent: blocksToResponseContent(block.ToolResult.OutputContent),
					Status:        "completed",
				})
			}
		case ai.RoleCanonicalCompaction:
			for _, block := range msg.Content {
				if block.Type != ai.BlockCompaction {
					continue
				}
				items = append(items, ai.ResponseItem{
					Type:             "compaction",
					EncryptedContent: strings.TrimSpace(block.EncryptedContent),
				})
			}
		}
	}

	toolDefs := make([]ai.ResponseTool, 0, len(req.Context.Tools))
	for _, tool := range req.Context.Tools {
		toolDefs = append(toolDefs, ai.ResponseTool{
			Type:        "function",
			Name:        strings.TrimSpace(tool.Name),
			Description: strings.TrimSpace(tool.Description),
			Parameters:  tool.Parameters,
			Strict:      tool.Strict,
			Raw:         append(json.RawMessage(nil), tool.Raw...),
		})
	}

	result := &ai.ResponsesRequest{
		Model:              strings.TrimSpace(req.Config.Model),
		Instructions:       strings.Join(instructionParts, "\n\n"),
		PreviousResponseID: strings.TrimSpace(req.PreviousResponseID),
		RequestID:          strings.TrimSpace(req.RequestID),
		ServiceTier:        ai.NormalizeServiceTier(req.Config.ServiceTier),
		Input:              items,
		Tools:              toolDefs,
		ParallelToolCalls:  req.Config.ParallelToolCalls,
		Include:            append([]string(nil), req.Config.Include...),
		PromptCacheKey:     strings.TrimSpace(req.Config.PromptCacheKey),
		MaxOutputTokens:    req.Config.MaxTokens,
		Temperature:        req.Config.Temperature,
	}
	if strings.TrimSpace(req.Config.ReasoningEffort) != "" || strings.TrimSpace(req.Config.ReasoningSummary) != "" {
		result.Reasoning = &ai.ResponsesReasoning{
			Effort:  strings.TrimSpace(req.Config.ReasoningEffort),
			Summary: strings.TrimSpace(req.Config.ReasoningSummary),
		}
	}
	if len(req.Config.ToolChoice) > 0 {
		result.ToolChoice = append(json.RawMessage(nil), req.Config.ToolChoice...)
	}
	return result
}

func textFromInstructionBlocks(blocks []ai.ContentBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type != ai.BlockText {
			continue
		}
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n")
}

func responseItemsFromAssistantMessage(msg ai.ConversationMessage) []ai.ResponseItem {
	if msg.Role != ai.RoleCanonicalAssistant || len(msg.Content) == 0 {
		return nil
	}
	items := make([]ai.ResponseItem, 0, len(msg.Content))
	for _, block := range msg.Content {
		if block.Type == ai.BlockThinking {
			continue
		}
		if item, ok := responseItemFromCanonicalAssistantBlock(block); ok {
			items = append(items, item)
			continue
		}
		switch block.Type {
		case ai.BlockText:
			items = append(items, ai.ResponseItem{
				Type:   "message",
				Role:   "assistant",
				ID:     strings.TrimSpace(block.TextSignature),
				Status: "completed",
				Content: []ai.ResponseContentPart{{
					Type: "output_text",
					Text: strings.TrimSpace(block.Text),
				}},
			})
		case ai.BlockToolCall:
			if block.ToolCall == nil {
				continue
			}
			items = append(items, ai.ResponseItem{
				Type:      "function_call",
				CallID:    strings.TrimSpace(block.ToolCall.ID),
				Name:      strings.TrimSpace(block.ToolCall.Name),
				Arguments: firstNonEmptyString(strings.TrimSpace(block.ToolCall.RawArguments), ai.MarshalToolArgumentsJSON(block.ToolCall.Arguments)),
				Status:    "completed",
			})
		}
	}
	return normalizeContinuationResponseItems(items)
}

func responseItemFromCanonicalAssistantBlock(block ai.ContentBlock) (ai.ResponseItem, bool) {
	raw := firstNonEmptyRawJSON(block.Raw, toolCallRawFromBlock(block))
	if len(raw) == 0 {
		return ai.ResponseItem{}, false
	}
	item := ai.ParseResponseItemRaw(raw)
	if item.Type == "reasoning" {
		return ai.ResponseItem{}, false
	}
	normalized, ok := normalizeContinuationResponseItem(item)
	return normalized, ok
}

func normalizeContinuationResponseItems(items []ai.ResponseItem) []ai.ResponseItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]ai.ResponseItem, 0, len(items))
	for _, item := range items {
		normalized, ok := normalizeContinuationResponseItem(item)
		if ok {
			out = append(out, normalized)
		}
	}
	return out
}

func normalizeContinuationResponseItem(item ai.ResponseItem) (ai.ResponseItem, bool) {
	switch item.Type {
	case "message":
		role := strings.ToLower(strings.TrimSpace(item.Role))
		if role != "assistant" || len(item.Content) == 0 {
			return ai.ResponseItem{}, false
		}
		return ai.ResponseItem{
			Role:    "assistant",
			ID:      strings.TrimSpace(item.ID),
			Content: normalizeContinuationMessageContent(item.Content),
			Raw:     marshalContinuationAssistantMessageRaw(item),
		}, true
	case "function_call":
		callID := strings.TrimSpace(item.CallID)
		name := strings.TrimSpace(item.Name)
		if callID == "" || name == "" {
			return ai.ResponseItem{}, false
		}
		return ai.ResponseItem{
			Type:      "function_call",
			ID:        strings.TrimSpace(item.ID),
			CallID:    callID,
			Name:      name,
			Arguments: providerToolArgumentsJSON(providerToolArgumentsMap(strings.TrimSpace(item.Arguments))),
		}, true
	case "reasoning":
		if strings.TrimSpace(item.EncryptedContent) == "" {
			return ai.ResponseItem{}, false
		}
		return ai.ResponseItem{
			Type:             "reasoning",
			EncryptedContent: strings.TrimSpace(item.EncryptedContent),
			Summary:          append([]ai.ResponseSummaryPart(nil), item.Summary...),
		}, true
	default:
		return ai.ResponseItem{}, false
	}
}

func normalizeContinuationMessageContent(parts []ai.ResponseContentPart) []ai.ResponseContentPart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]ai.ResponseContentPart, 0, len(parts))
	for _, part := range parts {
		partType := strings.TrimSpace(part.Type)
		switch partType {
		case "", "output_text":
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			out = append(out, ai.ResponseContentPart{
				Type: "output_text",
				Text: text,
			})
		}
	}
	return out
}

func marshalContinuationAssistantMessageRaw(item ai.ResponseItem) json.RawMessage {
	payload := map[string]any{
		"role": "assistant",
	}
	if id := strings.TrimSpace(item.ID); id != "" {
		payload["id"] = id
	}
	if content := normalizeContinuationMessageContent(item.Content); len(content) > 0 {
		parts := make([]map[string]any, 0, len(content))
		for _, part := range content {
			parts = append(parts, map[string]any{
				"type":        "output_text",
				"text":        part.Text,
				"annotations": []any{},
			})
		}
		payload["content"] = parts
	}
	data, _ := json.Marshal(payload)
	return data
}

func toolCallRawFromBlock(block ai.ContentBlock) json.RawMessage {
	if block.ToolCall == nil || len(block.ToolCall.Raw) == 0 {
		return nil
	}
	return block.ToolCall.Raw
}

func firstNonEmptyRawJSON(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func responseItemFromEventRaw(raw json.RawMessage) *ai.ResponseItem {
	if len(raw) == 0 {
		return nil
	}
	var envelope struct {
		Item json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Item) == 0 {
		return nil
	}
	item := ai.ParseResponseItemRaw(envelope.Item)
	return &item
}

func responseEventItemKey(item *ai.ResponseItem, raw json.RawMessage) string {
	if item != nil {
		if id := strings.TrimSpace(item.ID); id != "" {
			return id
		}
	}
	type eventFields struct {
		ItemID      string `json:"item_id"`
		OutputIndex int64  `json:"output_index"`
	}
	var fields eventFields
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	if strings.TrimSpace(fields.ItemID) != "" {
		return strings.TrimSpace(fields.ItemID)
	}
	if fields.OutputIndex >= 0 {
		return fmt.Sprintf("output:%d", fields.OutputIndex)
	}
	return ""
}

func responseReasoningText(item *ai.ResponseItem) string {
	if item == nil {
		return ""
	}
	parts := make([]string, 0, len(item.Content)+len(item.Summary))
	for _, part := range item.Content {
		if text := strings.TrimSpace(part.Text); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		for _, part := range item.Summary {
			if text := strings.TrimSpace(part.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func responseMessageText(item *ai.ResponseItem) string {
	if item == nil {
		return ""
	}
	parts := make([]string, 0, len(item.Content))
	for _, part := range item.Content {
		if text := strings.TrimSpace(part.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func blocksToResponseContent(blocks []ai.ContentBlock) []ai.ResponseContentPart {
	content := make([]ai.ResponseContentPart, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case ai.BlockText:
			if text := strings.TrimSpace(block.Text); text != "" {
				content = append(content, ai.ResponseContentPart{Type: "input_text", Text: text})
			}
		case ai.BlockImage:
			if strings.TrimSpace(block.ImageData) != "" {
				content = append(content, ai.ResponseContentPart{
					Type:     "input_image",
					ImageURL: strings.TrimSpace(block.ImageData),
					Detail:   firstNonEmptyString(strings.TrimSpace(block.MimeType), "auto"),
				})
			}
		}
	}
	return content
}

func convertNativeResponsesInput(items []ai.ResponseItem) ([]responses.ResponseInputItemUnionParam, error) {
	rawItems, err := ai.MarshalResponsesInputItemsJSON(items)
	if err != nil {
		return nil, err
	}
	if len(rawItems) == 0 {
		return nil, nil
	}
	out := make([]responses.ResponseInputItemUnionParam, 0, len(rawItems))
	for _, rawItem := range rawItems {
		out = append(out, param.Override[responses.ResponseInputItemUnionParam](rawItem))
	}
	return out, nil
}

func convertNativeOutputContent(parts []ai.ResponseContentPart) []responses.ResponseOutputMessageContentUnionParam {
	content := make([]responses.ResponseOutputMessageContentUnionParam, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		content = append(content, responses.ResponseOutputMessageContentUnionParam{
			OfOutputText: &responses.ResponseOutputTextParam{
				Type:        constant.OutputText("output_text"),
				Text:        text,
				Annotations: []responses.ResponseOutputTextAnnotationUnionParam{},
			},
		})
	}
	return content
}

func convertNativeResponseTools(tools []ai.ResponseTool) ([]responses.ToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make([]responses.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		if len(tool.Raw) > 0 {
			var rawTool responses.ToolUnionParam
			if err := json.Unmarshal(tool.Raw, &rawTool); err == nil {
				out = append(out, rawTool)
				continue
			}
		}
		toolType := strings.TrimSpace(tool.Type)
		if toolType == "" {
			toolType = "function"
		}
		if toolType != "function" {
			return nil, fmt.Errorf("unsupported response tool type %q", toolType)
		}
		function := responses.FunctionToolParam{
			Type:       constant.Function("function"),
			Name:       strings.TrimSpace(tool.Name),
			Parameters: normalizeSchema(tool.Parameters),
		}
		if desc := strings.TrimSpace(tool.Description); desc != "" {
			function.Description = param.NewOpt(desc)
		}
		if tool.Strict != nil {
			function.Strict = param.NewOpt(*tool.Strict)
		} else {
			function.Strict = param.NewOpt(false)
		}
		out = append(out, responses.ToolUnionParam{OfFunction: &function})
	}
	return out, nil
}

func convertNativeToolChoice(raw json.RawMessage) (responses.ResponseNewParamsToolChoiceUnion, bool, error) {
	if len(raw) == 0 {
		return responses.ResponseNewParamsToolChoiceUnion{}, false, nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions(strings.TrimSpace(asString))),
		}, true, nil
	}
	var asObject map[string]any
	if err := json.Unmarshal(raw, &asObject); err != nil {
		return responses.ResponseNewParamsToolChoiceUnion{}, false, fmt.Errorf("invalid responses tool_choice")
	}
	switch strings.TrimSpace(fmt.Sprint(asObject["type"])) {
	case "allowed_tools":
		tools, _ := asObject["tools"].([]any)
		allowed := make([]map[string]any, 0, len(tools))
		for _, tool := range tools {
			if typed, ok := tool.(map[string]any); ok {
				allowed = append(allowed, typed)
			}
		}
		return responses.ResponseNewParamsToolChoiceUnion{
			OfAllowedTools: &responses.ToolChoiceAllowedParam{
				Type:  constant.AllowedTools("allowed_tools"),
				Mode:  responses.ToolChoiceAllowedMode(strings.TrimSpace(fmt.Sprint(asObject["mode"]))),
				Tools: allowed,
			},
		}, true, nil
	case "function":
		return responses.ResponseNewParamsToolChoiceUnion{
			OfFunctionTool: &responses.ToolChoiceFunctionParam{
				Type: constant.Function("function"),
				Name: strings.TrimSpace(fmt.Sprint(asObject["name"])),
			},
		}, true, nil
	case "shell":
		choice := responses.NewToolChoiceShellParam()
		return responses.ResponseNewParamsToolChoiceUnion{
			OfSpecificShellToolChoice: &choice,
		}, true, nil
	default:
		return responses.ResponseNewParamsToolChoiceUnion{}, false, fmt.Errorf("unsupported responses tool_choice object type %q", strings.TrimSpace(fmt.Sprint(asObject["type"])))
	}
}

func convertResponsesToNativeResult(resp *responses.Response) *ai.ResponsesResult {
	if resp == nil {
		return &ai.ResponsesResult{}
	}
	output := make([]ai.ResponseItem, 0, len(resp.Output))
	for _, item := range resp.Output {
		parsed := *responseItemFromRawJSON(item.RawJSON())
		switch typed := item.AsAny().(type) {
		case responses.ResponseFunctionToolCall:
			parsed.Type = firstNonEmptyString(parsed.Type, "function_call")
			parsed.CallID = firstNonEmptyString(parsed.CallID, typed.CallID)
			parsed.Name = firstNonEmptyString(parsed.Name, typed.Name)
			parsed.Arguments = firstNonEmptyString(parsed.Arguments, typed.Arguments)
			parsed.Status = firstNonEmptyString(parsed.Status, strings.TrimSpace(string(typed.Status)))
		case responses.ResponseReasoningItem:
			parsed.Type = firstNonEmptyString(parsed.Type, "reasoning")
			parsed.ID = firstNonEmptyString(parsed.ID, typed.ID)
			if len(parsed.Content) == 0 {
				content := make([]ai.ResponseContentPart, 0, len(typed.Content))
				for _, part := range typed.Content {
					if strings.TrimSpace(part.Text) == "" {
						continue
					}
					content = append(content, ai.ResponseContentPart{Type: "reasoning_text", Text: strings.TrimSpace(part.Text)})
				}
				parsed.Content = content
			}
			if len(parsed.Summary) == 0 {
				summary := make([]ai.ResponseSummaryPart, 0, len(typed.Summary))
				for _, part := range typed.Summary {
					if strings.TrimSpace(part.Text) == "" {
						continue
					}
					summary = append(summary, ai.ResponseSummaryPart{Type: "summary_text", Text: strings.TrimSpace(part.Text)})
				}
				parsed.Summary = summary
			}
			if parsed.EncryptedContent == "" {
				parsed.EncryptedContent = strings.TrimSpace(typed.EncryptedContent)
			}
		case responses.ResponseOutputMessage:
			parsed.Type = firstNonEmptyString(parsed.Type, "message")
			parsed.Role = firstNonEmptyString(parsed.Role, strings.TrimSpace(string(typed.Role)))
			parsed.ID = firstNonEmptyString(parsed.ID, typed.ID)
			parsed.Status = firstNonEmptyString(parsed.Status, strings.TrimSpace(string(typed.Status)))
			if len(parsed.Content) == 0 {
				content := make([]ai.ResponseContentPart, 0, len(typed.Content))
				for _, part := range typed.Content {
					if part.Type == "output_text" && strings.TrimSpace(part.Text) != "" {
						content = append(content, ai.ResponseContentPart{Type: "output_text", Text: strings.TrimSpace(part.Text)})
					}
				}
				parsed.Content = content
			}
		}
		output = append(output, parsed)
	}
	stopReason := ai.StopReasonStop
	for _, item := range output {
		if item.Type == "function_call" || item.Type == "custom_tool_call" {
			stopReason = ai.StopReasonToolUse
			break
		}
	}
	if stopReason != ai.StopReasonToolUse {
		switch strings.TrimSpace(string(resp.Status)) {
		case "", "completed":
			stopReason = ai.StopReasonStop
		case "incomplete":
			stopReason = ai.StopReasonLength
		default:
			stopReason = ai.StopReasonError
		}
	}
	return &ai.ResponsesResult{
		ID:     strings.TrimSpace(resp.ID),
		Model:  strings.TrimSpace(resp.Model),
		Status: strings.TrimSpace(string(resp.Status)),
		Output: output,
		Usage: ai.Usage{
			InputTokens:     maxInt64(0, resp.Usage.InputTokens-resp.Usage.InputTokensDetails.CachedTokens),
			OutputTokens:    resp.Usage.OutputTokens,
			CacheReadTokens: resp.Usage.InputTokensDetails.CachedTokens,
			TotalTokens:     firstNonZeroInt64(resp.Usage.TotalTokens, resp.Usage.InputTokens+resp.Usage.OutputTokens),
		},
		StopReason: stopReason,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func responseItemFromRawJSON(raw string) *ai.ResponseItem {
	item := ai.ParseResponseItemRaw(json.RawMessage(raw))
	return &item
}

func marshalResponsesRequestBody(req *ai.ResponsesRequest) ([]byte, error) {
	return ai.MarshalResponsesAPIRequestJSON(req)
}

func parseResponsesResultRaw(raw []byte) (*ai.ResponsesResult, error) {
	return ai.ParseResponsesResultJSON(raw)
}
