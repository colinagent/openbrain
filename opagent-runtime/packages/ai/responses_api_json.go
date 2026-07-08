package ai

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ResponsesAPIRequestPayload is the wire-level JSON payload accepted by the
// OpenAI-compatible Responses HTTP API.
type ResponsesAPIRequestPayload struct {
	Model              string          `json:"model"`
	Instructions       string          `json:"instructions"`
	PreviousResponseID string          `json:"previous_response_id"`
	ServiceTier        string          `json:"service_tier"`
	Input              json.RawMessage `json:"input"`
	Tools              json.RawMessage `json:"tools"`
	Stream             *bool           `json:"stream"`
	MaxOutputTokens    *int64          `json:"max_output_tokens"`
	Temperature        *float64        `json:"temperature"`
	Reasoning          json.RawMessage `json:"reasoning"`
	ToolChoice         json.RawMessage `json:"tool_choice"`
	ParallelToolCalls  *bool           `json:"parallel_tool_calls"`
	Include            []string        `json:"include"`
	PromptCacheKey     string          `json:"prompt_cache_key"`
	Store              *bool           `json:"store"`
	Text               json.RawMessage `json:"text"`
}

// ResponsesWebsocketCreatePayload is the wire-level websocket create envelope
// used by the gateway websocket Responses surface.
type ResponsesWebsocketCreatePayload struct {
	Type     string `json:"type"`
	Generate *bool  `json:"generate"`
	ResponsesAPIRequestPayload
}

func DecodeResponsesAPIRequestJSON(raw []byte) (*ResponsesRequest, bool, error) {
	var payload ResponsesAPIRequestPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false, fmt.Errorf("invalid JSON payload")
	}
	return DecodeResponsesAPIRequestPayload(payload)
}

func DecodeResponsesAPIRequestPayload(req ResponsesAPIRequestPayload) (*ResponsesRequest, bool, error) {
	items, err := decodeResponsesAPIInputItems(req.Input)
	if err != nil {
		return nil, false, err
	}
	tools, err := decodeResponsesAPITools(req.Tools)
	if err != nil {
		return nil, false, err
	}
	textCfg, err := decodeResponsesAPIText(req.Text)
	if err != nil {
		return nil, false, err
	}
	reasoningCfg, err := decodeResponsesAPIReasoning(req.Reasoning)
	if err != nil {
		return nil, false, err
	}
	stream := false
	if req.Stream != nil {
		stream = *req.Stream
	}
	serviceTier := ""
	if strings.TrimSpace(req.ServiceTier) != "" {
		serviceTier = NormalizeServiceTier(req.ServiceTier)
		if serviceTier == "" {
			return nil, false, fmt.Errorf("unsupported service_tier %q", strings.TrimSpace(req.ServiceTier))
		}
	}
	return &ResponsesRequest{
		Model:              strings.TrimSpace(req.Model),
		Instructions:       strings.TrimSpace(req.Instructions),
		PreviousResponseID: strings.TrimSpace(req.PreviousResponseID),
		ServiceTier:        serviceTier,
		Input:              items,
		Tools:              tools,
		ToolChoice:         append(json.RawMessage(nil), req.ToolChoice...),
		ParallelToolCalls:  req.ParallelToolCalls,
		Reasoning:          reasoningCfg,
		Store:              req.Store,
		Stream:             stream,
		Include:            append([]string(nil), req.Include...),
		PromptCacheKey:     strings.TrimSpace(req.PromptCacheKey),
		Text:               textCfg,
		Temperature:        req.Temperature,
		MaxOutputTokens:    req.MaxOutputTokens,
	}, stream, nil
}

func MarshalResponsesAPIRequestJSON(req *ResponsesRequest) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("responses request is nil")
	}
	body := map[string]any{
		"model": strings.TrimSpace(req.Model),
	}
	if strings.TrimSpace(req.Instructions) != "" {
		body["instructions"] = strings.TrimSpace(req.Instructions)
	}
	if serviceTier, err := ResponsesAPIServiceTier(req.ServiceTier); err != nil {
		return nil, err
	} else if serviceTier != "" {
		body["service_tier"] = serviceTier
	}
	if rawInput, err := marshalResponsesAPIInputJSON(req.Input); err != nil {
		return nil, err
	} else if len(rawInput) > 0 {
		body["input"] = json.RawMessage(rawInput)
	}
	if rawTools, err := marshalResponsesAPIToolsJSON(req.Tools); err != nil {
		return nil, err
	} else if len(rawTools) > 0 {
		body["tools"] = json.RawMessage(rawTools)
	}
	if len(req.ToolChoice) > 0 {
		body["tool_choice"] = json.RawMessage(req.ToolChoice)
	}
	if req.ParallelToolCalls != nil {
		body["parallel_tool_calls"] = *req.ParallelToolCalls
	}
	if req.Reasoning != nil {
		reasoning := map[string]any{}
		if strings.TrimSpace(req.Reasoning.Effort) != "" {
			reasoning["effort"] = strings.TrimSpace(req.Reasoning.Effort)
		}
		if strings.TrimSpace(req.Reasoning.Summary) != "" {
			reasoning["summary"] = strings.TrimSpace(req.Reasoning.Summary)
		}
		if len(reasoning) > 0 {
			body["reasoning"] = reasoning
		}
	}
	if req.Store != nil {
		body["store"] = *req.Store
	}
	body["stream"] = req.Stream
	if len(req.Include) > 0 {
		body["include"] = req.Include
	}
	if strings.TrimSpace(req.PromptCacheKey) != "" {
		body["prompt_cache_key"] = strings.TrimSpace(req.PromptCacheKey)
	}
	if req.Text != nil {
		text := map[string]any{}
		if strings.TrimSpace(req.Text.Verbosity) != "" {
			text["verbosity"] = strings.TrimSpace(req.Text.Verbosity)
		}
		if len(req.Text.FormatRaw) > 0 {
			text["format"] = json.RawMessage(req.Text.FormatRaw)
		}
		if len(text) > 0 {
			body["text"] = text
		}
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.MaxOutputTokens != nil {
		body["max_output_tokens"] = *req.MaxOutputTokens
	}
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		body["previous_response_id"] = strings.TrimSpace(req.PreviousResponseID)
	}
	return json.Marshal(body)
}

func MarshalResponsesWebsocketCreateJSON(req *ResponsesRequest, generate *bool) ([]byte, error) {
	body, err := MarshalResponsesAPIRequestJSON(req)
	if err != nil {
		return nil, err
	}
	payload := make(map[string]json.RawMessage)
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	payload["type"] = json.RawMessage(`"response.create"`)
	if generate != nil {
		if *generate {
			payload["generate"] = json.RawMessage("true")
		} else {
			payload["generate"] = json.RawMessage("false")
		}
	}
	return json.Marshal(payload)
}

func ParseResponsesResultJSON(raw []byte) (*ResponsesResult, error) {
	var payload struct {
		ID     string            `json:"id"`
		Model  string            `json:"model"`
		Status string            `json:"status"`
		Output []json.RawMessage `json:"output"`
		Usage  struct {
			InputTokens        int64 `json:"input_tokens"`
			OutputTokens       int64 `json:"output_tokens"`
			TotalTokens        int64 `json:"total_tokens"`
			InputTokensDetails struct {
				CachedTokens     int64 `json:"cached_tokens"`
				CacheWriteTokens int64 `json:"cache_write_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	items := make([]ResponseItem, 0, len(payload.Output))
	for _, rawItem := range payload.Output {
		items = append(items, ParseResponseItemRaw(rawItem))
	}
	stopReason := StopReasonStop
	for _, item := range items {
		if item.Type == "function_call" || item.Type == "custom_tool_call" {
			stopReason = StopReasonToolUse
			break
		}
	}
	if stopReason != StopReasonToolUse && strings.EqualFold(strings.TrimSpace(payload.Status), "incomplete") {
		stopReason = StopReasonLength
	}
	return &ResponsesResult{
		ID:     strings.TrimSpace(payload.ID),
		Model:  strings.TrimSpace(payload.Model),
		Status: strings.TrimSpace(payload.Status),
		Output: items,
		Usage: Usage{
			InputTokens:      maxInt64(0, payload.Usage.InputTokens-payload.Usage.InputTokensDetails.CachedTokens-payload.Usage.InputTokensDetails.CacheWriteTokens),
			OutputTokens:     payload.Usage.OutputTokens,
			CacheReadTokens:  payload.Usage.InputTokensDetails.CachedTokens,
			CacheWriteTokens: payload.Usage.InputTokensDetails.CacheWriteTokens,
			TotalTokens:      firstNonZeroInt64(payload.Usage.TotalTokens, payload.Usage.InputTokens+payload.Usage.OutputTokens),
		},
		StopReason: stopReason,
	}, nil
}

func RenderResponsesAPIResponseJSON(result *ResponsesResult) json.RawMessage {
	if result == nil {
		result = &ResponsesResult{}
	}
	payload := map[string]any{
		"id":         strings.TrimSpace(result.ID),
		"object":     "response",
		"created_at": time.Now().Unix(),
		"model":      strings.TrimSpace(result.Model),
		"output":     MarshalResponsesOutputItemsJSON(result.Output),
		"status":     firstNonEmptyResponsesString(strings.TrimSpace(result.Status), "completed"),
		"usage": map[string]any{
			"input_tokens":  result.Usage.PromptTokens(),
			"output_tokens": result.Usage.OutputTokens,
			"total_tokens":  result.Usage.ResolvedTotalTokens(),
		},
	}
	if result.Usage.CacheReadTokens > 0 || result.Usage.CacheWriteTokens > 0 {
		inputDetails := map[string]any{}
		if result.Usage.CacheReadTokens > 0 {
			inputDetails["cached_tokens"] = result.Usage.CacheReadTokens
		}
		if result.Usage.CacheWriteTokens > 0 {
			inputDetails["cache_write_tokens"] = result.Usage.CacheWriteTokens
		}
		payload["usage"].(map[string]any)["input_tokens_details"] = inputDetails
	}
	data, _ := json.Marshal(payload)
	return data
}

func NewSyntheticResponsesCreatedEvent(responseID, model string) ResponsesStreamEvent {
	return ResponsesStreamEvent{
		Type: "response.created",
		Response: &ResponsesResult{
			ID:     strings.TrimSpace(responseID),
			Model:  strings.TrimSpace(model),
			Status: "in_progress",
		},
	}
}

func NewSyntheticResponsesCompletedEvent(result *ResponsesResult) ResponsesStreamEvent {
	return ResponsesStreamEvent{Type: "response.completed", Response: result}
}

func NewSyntheticResponsesFailureEvent(err error) ResponsesStreamEvent {
	return ResponsesStreamEvent{Type: "response.failed", Error: err}
}

func RenderResponsesAPIStreamEventJSON(event ResponsesStreamEvent, requestID string) json.RawMessage {
	if strings.TrimSpace(event.Type) == "response.failed" {
		return renderResponsesAPIFailureEventJSON(event, requestID)
	}
	if len(event.Raw) > 0 {
		return cloneResponsesRawJSON(event.Raw)
	}
	return renderResponsesAPIStreamEventFallbackJSON(event)
}

func renderResponsesAPIFailureEventJSON(event ResponsesStreamEvent, requestID string) json.RawMessage {
	payload := map[string]any{
		"type":       firstNonEmptyResponsesString(strings.TrimSpace(event.Type), "response.failed"),
		"request_id": strings.TrimSpace(requestID),
	}
	if event.Error != nil && strings.TrimSpace(event.Error.Error()) != "" {
		payload["error"] = strings.TrimSpace(event.Error.Error())
	}
	if len(event.Raw) > 0 {
		var raw map[string]any
		if err := json.Unmarshal(event.Raw, &raw); err == nil {
			for key, value := range raw {
				payload[key] = value
			}
		}
	}
	if _, ok := payload["error"]; !ok {
		payload["error"] = "response failed without details"
	}
	data, _ := json.Marshal(payload)
	return data
}

func renderResponsesAPIStreamEventFallbackJSON(event ResponsesStreamEvent) json.RawMessage {
	switch strings.TrimSpace(event.Type) {
	case "response.output_text.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		data, _ := json.Marshal(map[string]any{"type": event.Type, "delta": event.Delta})
		return data
	case "response.output_item.done":
		item := ResponseItem{}
		if event.Item != nil {
			item = *event.Item
		}
		data, _ := json.Marshal(map[string]any{"type": event.Type, "item": json.RawMessage(MarshalResponsesOutputItemJSON(item))})
		return data
	case "response.completed", "response.created":
		resp := &ResponsesResult{}
		if event.Response != nil {
			resp = event.Response
		}
		data, _ := json.Marshal(map[string]any{"type": event.Type, "response": json.RawMessage(RenderResponsesAPIResponseJSON(resp))})
		return data
	default:
		data, _ := json.Marshal(map[string]any{"type": event.Type})
		return data
	}
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

func decodeResponsesAPIInputItems(raw json.RawMessage) ([]ResponseItem, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("input is required")
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		item := ResponseItem{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{
				Type: "input_text",
				Text: strings.TrimSpace(asString),
			}},
		}
		item.Raw, _ = MarshalResponsesInputItemJSON(item, 0)
		return []ResponseItem{item}, nil
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(raw, &rawItems); err != nil {
		return nil, fmt.Errorf("unsupported responses input")
	}
	items := make([]ResponseItem, 0, len(rawItems))
	for _, rawItem := range rawItems {
		items = append(items, ParseResponseItemRaw(rawItem))
	}
	return items, nil
}

func decodeResponsesAPITools(raw json.RawMessage) ([]ResponseTool, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var rawTools []json.RawMessage
	if err := json.Unmarshal(raw, &rawTools); err != nil {
		return nil, fmt.Errorf("invalid responses tools")
	}
	out := make([]ResponseTool, 0, len(rawTools))
	for _, rawTool := range rawTools {
		var parsed struct {
			Type        string `json:"type"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Parameters  any    `json:"parameters"`
			Strict      *bool  `json:"strict"`
			Function    *struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Parameters  any    `json:"parameters"`
			} `json:"function"`
		}
		if err := json.Unmarshal(rawTool, &parsed); err != nil {
			return nil, fmt.Errorf("invalid response tool")
		}
		tool := ResponseTool{Raw: append(json.RawMessage(nil), rawTool...)}
		if parsed.Function != nil {
			tool.Type = "function"
			tool.Name = strings.TrimSpace(parsed.Function.Name)
			tool.Description = strings.TrimSpace(parsed.Function.Description)
			tool.Parameters = parsed.Function.Parameters
		} else {
			tool.Type = strings.TrimSpace(parsed.Type)
			tool.Name = strings.TrimSpace(parsed.Name)
			tool.Description = strings.TrimSpace(parsed.Description)
			tool.Parameters = parsed.Parameters
			tool.Strict = parsed.Strict
		}
		out = append(out, tool)
	}
	return out, nil
}

func decodeResponsesAPIText(raw json.RawMessage) (*ResponsesTextConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var parsed struct {
		Verbosity string          `json:"verbosity"`
		Format    json.RawMessage `json:"format"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("invalid responses text config")
	}
	return &ResponsesTextConfig{
		Verbosity: strings.TrimSpace(parsed.Verbosity),
		FormatRaw: append(json.RawMessage(nil), parsed.Format...),
	}, nil
}

func decodeResponsesAPIReasoning(raw json.RawMessage) (*ResponsesReasoning, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var parsed struct {
		Effort  string `json:"effort"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("invalid responses reasoning config")
	}
	return &ResponsesReasoning{
		Effort:  strings.TrimSpace(parsed.Effort),
		Summary: strings.TrimSpace(parsed.Summary),
	}, nil
}

func marshalResponsesAPIInputJSON(items []ResponseItem) (json.RawMessage, error) {
	rawItems, err := MarshalResponsesInputItemsJSON(items)
	if err != nil || len(rawItems) == 0 {
		return nil, err
	}
	data, err := json.Marshal(rawItems)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func marshalResponsesAPIToolsJSON(tools []ResponseTool) (json.RawMessage, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	rawTools := make([]json.RawMessage, 0, len(tools))
	for _, tool := range tools {
		if len(tool.Raw) > 0 {
			rawTools = append(rawTools, cloneResponsesRawJSON(tool.Raw))
			continue
		}
		payload := map[string]any{
			"type":       firstNonEmptyResponsesString(strings.TrimSpace(tool.Type), "function"),
			"name":       strings.TrimSpace(tool.Name),
			"parameters": normalizeResponsesAPISchema(tool.Parameters),
		}
		if strings.TrimSpace(tool.Description) != "" {
			payload["description"] = strings.TrimSpace(tool.Description)
		}
		if tool.Strict != nil {
			payload["strict"] = *tool.Strict
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		rawTools = append(rawTools, raw)
	}
	data, err := json.Marshal(rawTools)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func normalizeResponsesAPISchema(schema any) map[string]any {
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
