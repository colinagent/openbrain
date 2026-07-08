package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"google.golang.org/genai"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type GeminiProvider struct {
	client *genai.Client
	cfg    *op.ModelConfig
}

func NewGeminiProvider(ctx context.Context, cfg *op.ModelConfig) (*GeminiProvider, error) {
	return NewGeminiProviderWithHeaders(ctx, cfg, nil)
}

func NewGeminiProviderWithHeaders(ctx context.Context, cfg *op.ModelConfig, headers http.Header) (*GeminiProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("model config is nil")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("model config: apiKey is required")
	}
	if headers == nil {
		headers = http.Header{}
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  strings.TrimSpace(cfg.APIKey),
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: strings.TrimSpace(cfg.BaseURL),
			Headers: headers,
		},
	})
	if err != nil {
		return nil, err
	}
	return &GeminiProvider{client: client, cfg: cfg}, nil
}

func (p *GeminiProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("gemini-native")
}

func (p *GeminiProvider) CompleteCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("gemini client is not initialized")
	}
	params, err := providerRequestFromCanonical(req, p.cfg)
	if err != nil {
		return nil, err
	}
	modelName := providerModelName(params.Model, p.cfg)
	if modelName == "" {
		return nil, fmt.Errorf("model name is empty")
	}
	contents, cfg, err := buildGeminiRequest(req, p.cfg)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Models.GenerateContent(ctx, modelName, contents, cfg)
	if err != nil {
		return nil, err
	}
	return providerResponseFromGemini(resp), nil
}

func (p *GeminiProvider) StreamCanonical(ctx context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req == nil {
		return nil, fmt.Errorf("canonical request is nil")
	}
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("gemini client is not initialized")
	}
	params, err := providerRequestFromCanonical(req, p.cfg)
	if err != nil {
		return nil, err
	}
	modelName := providerModelName(params.Model, p.cfg)
	if modelName == "" {
		return nil, fmt.Errorf("model name is empty")
	}
	contents, cfg, err := buildGeminiRequest(req, p.cfg)
	if err != nil {
		return nil, err
	}
	stream := p.client.Models.GenerateContentStream(ctx, modelName, contents, cfg)
	out := ai.NewProviderEventStream(128)
	go func() {
		partial := &ai.StreamConversationMessage{
			Role: ai.RoleCanonicalAssistant,
			ProviderState: &ai.ProviderState{
				Provider: strings.TrimSpace(p.cfg.Provider),
				API:      "gemini-native",
				Model:    modelName,
			},
		}
		started := false
		currentIndex := -1
		currentType := ai.ContentBlockType("")
		toolCallCounter := 0
		stopReason := ai.StopReasonStop
		usage := ai.Usage{}
		finishCurrent := func() bool {
			if currentIndex < 0 || currentIndex >= len(partial.Content) {
				return true
			}
			block := &partial.Content[currentIndex]
			switch currentType {
			case ai.BlockText:
				ok := out.Emit(ai.ProviderEvent{
					Type:         ai.EventCanonicalTextEnd,
					ContentIndex: currentIndex,
					Content:      block.Text,
					Block:        block,
					Partial:      partial,
				})
				currentIndex = -1
				currentType = ""
				return ok
			case ai.BlockThinking:
				ok := out.Emit(ai.ProviderEvent{
					Type:         ai.EventCanonicalThinkingEnd,
					ContentIndex: currentIndex,
					Content:      block.Text,
					Block:        block,
					Partial:      partial,
				})
				currentIndex = -1
				currentType = ""
				return ok
			default:
				currentIndex = -1
				currentType = ""
				return true
			}
		}
		for resp, streamErr := range stream {
			if streamErr != nil {
				out.Finish(streamErr)
				return
			}
			if resp == nil {
				continue
			}
			if !started {
				started = true
				if !out.Emit(ai.ProviderEvent{
					Type:    ai.EventCanonicalStart,
					Partial: partial,
				}) {
					return
				}
			}
			if len(resp.Candidates) > 0 && resp.Candidates[0] != nil && resp.Candidates[0].Content != nil {
				for _, part := range resp.Candidates[0].Content.Parts {
					if part == nil {
						continue
					}
					if part.Text != "" {
						nextType := ai.BlockText
						if part.Thought {
							nextType = ai.BlockThinking
						}
						if currentType != nextType {
							if !finishCurrent() {
								return
							}
							currentIndex = len(partial.Content)
							currentType = nextType
							partial.Content = append(partial.Content, ai.StreamContentBlock{Type: nextType})
							startEventType := ai.EventCanonicalTextStart
							if nextType == ai.BlockThinking {
								startEventType = ai.EventCanonicalThinkingStart
							}
							if !out.Emit(ai.ProviderEvent{
								Type:         startEventType,
								ContentIndex: currentIndex,
								Block:        &partial.Content[currentIndex],
								Partial:      partial,
							}) {
								return
							}
						}
						block := &partial.Content[currentIndex]
						block.Text += part.Text
						if len(part.ThoughtSignature) > 0 {
							signature := base64.StdEncoding.EncodeToString(part.ThoughtSignature)
							if nextType == ai.BlockThinking {
								block.ThinkingSignature = signature
							} else {
								block.TextSignature = signature
							}
						}
						deltaType := ai.EventCanonicalTextDelta
						if nextType == ai.BlockThinking {
							deltaType = ai.EventCanonicalThinkingDelta
						}
						if !out.Emit(ai.ProviderEvent{
							Type:         deltaType,
							ContentIndex: currentIndex,
							Delta:        part.Text,
							Block:        block,
							Partial:      partial,
						}) {
							return
						}
					}
					if part.FunctionCall != nil {
						if !finishCurrent() {
							return
						}
						id := strings.TrimSpace(part.FunctionCall.ID)
						if id == "" {
							toolCallCounter += 1
							id = fmt.Sprintf("%s_%d", strings.TrimSpace(part.FunctionCall.Name), toolCallCounter)
						}
						rawArgs := ai.MarshalToolArgumentsJSON(part.FunctionCall.Args)
						index := len(partial.Content)
						block := ai.StreamContentBlock{
							Type: ai.BlockToolCall,
							ToolCall: &ai.StreamToolCall{
								ID:           id,
								Name:         strings.TrimSpace(part.FunctionCall.Name),
								Arguments:    part.FunctionCall.Args,
								RawArguments: rawArgs,
								Complete:     true,
							},
						}
						partial.Content = append(partial.Content, block)
						if !out.Emit(ai.ProviderEvent{
							Type:         ai.EventCanonicalToolCallStart,
							ContentIndex: index,
							Block:        &partial.Content[index],
							Partial:      partial,
						}) {
							return
						}
						if rawArgs != "" {
							if !out.Emit(ai.ProviderEvent{
								Type:         ai.EventCanonicalToolCallDelta,
								ContentIndex: index,
								Delta:        rawArgs,
								Block:        &partial.Content[index],
								Partial:      partial,
							}) {
								return
							}
						}
						if !out.Emit(ai.ProviderEvent{
							Type:         ai.EventCanonicalToolCallEnd,
							ContentIndex: index,
							Block:        &partial.Content[index],
							Partial:      partial,
						}) {
							return
						}
					}
				}
				if finishReason := resp.Candidates[0].FinishReason; finishReason != "" {
					if len(resp.FunctionCalls()) > 0 || finishReason == genai.FinishReasonUnexpectedToolCall {
						stopReason = ai.StopReasonToolUse
					} else if finishReason == genai.FinishReasonMaxTokens {
						stopReason = ai.StopReasonLength
					} else {
						stopReason = ai.StopReasonStop
					}
				}
			}
			if resp.UsageMetadata != nil {
				cachedContentTokens := int64(resp.UsageMetadata.CachedContentTokenCount)
				usage.InputTokens = maxInt64(0, int64(resp.UsageMetadata.PromptTokenCount)-cachedContentTokens)
				usage.OutputTokens = int64(resp.UsageMetadata.CandidatesTokenCount + resp.UsageMetadata.ThoughtsTokenCount)
				usage.CacheReadTokens = cachedContentTokens
				usage.TotalTokens = int64(resp.UsageMetadata.TotalTokenCount)
				if usage.TotalTokens == 0 {
					usage.TotalTokens = usage.ResolvedTotalTokens()
				}
			}
		}
		if !finishCurrent() {
			return
		}
		out.Emit(ai.ProviderEvent{
			Type: ai.EventCanonicalDone,
			Response: &ai.ProviderResponse{
				Message:    ai.FinalizeStreamConversationMessage(partial),
				Usage:      usage,
				StopReason: stopReason,
			},
		})
		out.Close()
	}()
	return out, nil
}

func buildGeminiRequest(req *ai.ProviderRequest, cfg *op.ModelConfig) ([]*genai.Content, *genai.GenerateContentConfig, error) {
	params, err := providerRequestFromCanonical(req, cfg)
	if err != nil {
		return nil, nil, err
	}
	contents := make([]*genai.Content, 0, len(params.Messages))
	systemText := make([]string, 0)
	for _, msg := range params.Messages {
		switch msg.Role {
		case op.RoleSystem, op.RoleDeveloper:
			if text := providerMessageText(msg); strings.TrimSpace(text) != "" {
				systemText = append(systemText, text)
			}
		case op.RoleUser:
			content, err := geminiContentFromUser(msg)
			if err != nil {
				return nil, nil, err
			}
			if content != nil {
				contents = append(contents, content)
			}
		case op.RoleAssistant:
			if content := geminiContentFromAssistant(msg); content != nil {
				contents = append(contents, content)
			}
		case op.RoleTool:
			if content := geminiContentFromTool(msg); content != nil {
				contents = append(contents, content)
			}
			if content, err := geminiContentFromToolImages(msg); err != nil {
				return nil, nil, err
			} else if content != nil {
				contents = append(contents, content)
			}
		}
	}

	reqCfg := &genai.GenerateContentConfig{}
	if len(systemText) > 0 {
		reqCfg.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: strings.Join(systemText, "\n\n")}},
		}
	}
	if cfg != nil && cfg.MaxOutputTokens > 0 {
		reqCfg.MaxOutputTokens = int32(cfg.MaxOutputTokens)
	}
	if params.Options.MaxTokens != nil && *params.Options.MaxTokens > 0 {
		reqCfg.MaxOutputTokens = int32(*params.Options.MaxTokens)
	}
	if params.Options.Temperature != nil {
		temp := float32(*params.Options.Temperature)
		reqCfg.Temperature = &temp
	}
	if tools := convertToolSpecsToGemini(params.Tools); len(tools) > 0 {
		reqCfg.Tools = tools
		reqCfg.ToolConfig = &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeAuto,
			},
		}
	}
	return contents, reqCfg, nil
}

func geminiContentFromUser(msg op.Message) (*genai.Content, error) {
	if len(msg.ContentParts) == 0 {
		if strings.TrimSpace(msg.Content) == "" {
			return nil, nil
		}
		return genai.NewContentFromText(msg.Content, genai.RoleUser), nil
	}
	parts := make([]*genai.Part, 0, len(msg.ContentParts))
	for _, part := range msg.ContentParts {
		switch part.Type {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, &genai.Part{Text: part.Text})
			}
		case "image_url":
			imagePart, err := geminiImagePart(part)
			if err != nil {
				return nil, err
			}
			if imagePart != nil {
				parts = append(parts, imagePart)
			}
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return genai.NewContentFromParts(parts, genai.RoleUser), nil
}

func geminiContentFromAssistant(msg op.Message) *genai.Content {
	parts := make([]*genai.Part, 0, len(msg.ToolCalls)+1)
	if strings.TrimSpace(msg.Content) != "" {
		parts = append(parts, &genai.Part{Text: msg.Content})
	}
	for _, call := range msg.ToolCalls {
		args := ai.CloneToolArguments(call.Arguments)
		parts = append(parts, &genai.Part{
			FunctionCall: &genai.FunctionCall{
				ID:   call.ID,
				Name: call.Name,
				Args: args,
			},
		})
	}
	if len(parts) == 0 {
		return nil
	}
	return genai.NewContentFromParts(parts, genai.RoleModel)
}

func geminiContentFromTool(msg op.Message) *genai.Content {
	if strings.TrimSpace(msg.ToolCallID) == "" || strings.TrimSpace(msg.Name) == "" {
		return nil
	}
	response := map[string]any{"output": msg.Content}
	return &genai.Content{
		Role: genai.RoleUser,
		Parts: []*genai.Part{{
			FunctionResponse: &genai.FunctionResponse{
				ID:       msg.ToolCallID,
				Name:     msg.Name,
				Response: response,
			},
		}},
	}
}

func geminiContentFromToolImages(msg op.Message) (*genai.Content, error) {
	if len(msg.ContentParts) == 0 {
		return nil, nil
	}
	parts := make([]*genai.Part, 0, len(msg.ContentParts)+1)
	for _, part := range msg.ContentParts {
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "image", "image_url":
			imagePart, err := geminiImagePart(part)
			if err != nil {
				return nil, err
			}
			if imagePart != nil {
				parts = append(parts, imagePart)
			}
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	parts = append([]*genai.Part{{Text: "Tool result image:"}}, parts...)
	return genai.NewContentFromParts(parts, genai.RoleUser), nil
}

func geminiImagePart(part op.ContentPart) (*genai.Part, error) {
	if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
		return nil, nil
	}
	rawURL := strings.TrimSpace(part.ImageURL.URL)
	if strings.HasPrefix(rawURL, "data:") {
		mediaType, data, err := decodeProviderDataURL(rawURL)
		if err != nil {
			return nil, err
		}
		return &genai.Part{
			InlineData: &genai.Blob{
				MIMEType: mediaType,
				Data:     data,
			},
		}, nil
	}
	return &genai.Part{
		FileData: &genai.FileData{
			FileURI:  rawURL,
			MIMEType: mime.TypeByExtension(""),
		},
	}, nil
}

func convertToolSpecsToGemini(specs []op.ToolSpec) []*genai.Tool {
	if len(specs) == 0 {
		return nil
	}
	declarations := make([]*genai.FunctionDeclaration, 0, len(specs))
	for _, spec := range specs {
		declarations = append(declarations, &genai.FunctionDeclaration{
			Name:                 spec.Name,
			Description:          spec.Description,
			ParametersJsonSchema: spec.InputSchema,
		})
	}
	return []*genai.Tool{{FunctionDeclarations: declarations}}
}

func providerResponseFromGemini(resp *genai.GenerateContentResponse) *ai.ProviderResponse {
	if resp == nil {
		return nil
	}
	var content strings.Builder
	var reasoning strings.Builder
	toolCalls := make([]op.MessageToolCall, 0)

	if resp.Text() != "" {
		content.WriteString(resp.Text())
	}
	for _, call := range resp.FunctionCalls() {
		toolCalls = append(toolCalls, op.MessageToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: ai.CloneToolArguments(call.Args),
			Type:      "function",
		})
	}
	for _, candidate := range resp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part == nil {
				continue
			}
			if part.Thought {
				reasoning.WriteString(part.Text)
			}
		}
	}

	usage := ai.Usage{}
	if resp.UsageMetadata != nil {
		cachedContentTokens := int64(resp.UsageMetadata.CachedContentTokenCount)
		usage.InputTokens = maxInt64(0, int64(resp.UsageMetadata.PromptTokenCount)-cachedContentTokens)
		usage.OutputTokens = int64(resp.UsageMetadata.CandidatesTokenCount + resp.UsageMetadata.ThoughtsTokenCount)
		usage.CacheReadTokens = cachedContentTokens
		usage.TotalTokens = usage.ResolvedTotalTokens()
	}

	stopReason := ai.StopReasonStop
	if len(toolCalls) > 0 {
		stopReason = ai.StopReasonToolUse
	}

	return ai.ProviderResponseFromOpMessage(op.Message{
		Role:             op.RoleAssistant,
		Content:          content.String(),
		ReasoningContent: reasoning.String(),
		ToolCalls:        toolCalls,
	}, usage, stopReason)
}
