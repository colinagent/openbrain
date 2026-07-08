package ai

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func ParseResponsesStreamEventJSON(raw []byte) (ResponsesStreamEvent, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "[DONE]" {
		return ResponsesStreamEvent{}, nil
	}
	eventType := responsesStreamEventType(trimmed, "")
	if strings.TrimSpace(eventType) == "" {
		return ResponsesStreamEvent{}, fmt.Errorf("responses stream event missing type")
	}
	event := ResponsesStreamEvent{
		Type: strings.TrimSpace(eventType),
		Raw:  cloneResponsesRawJSON(raw),
	}
	switch event.Type {
	case "response.created", "response.completed", "response.in_progress", "response.incomplete", "response.queued":
		resp, err := parseResponsesStreamEventResponse(raw)
		if err != nil {
			return ResponsesStreamEvent{}, err
		}
		event.Response = resp
	case "response.output_item.added", "response.output_item.done":
		item, err := parseResponsesStreamEventItem(raw)
		if err != nil {
			return ResponsesStreamEvent{}, err
		}
		event.Item = item
	case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.done", "response.reasoning_summary_text.done", "response.function_call_arguments.delta", "response.function_call_arguments.done", "response.refusal.delta", "response.output_text.done":
		event.Delta = parseResponsesStreamEventDelta(raw)
	case "response.failed":
		event.Error = ParseResponsesFailureErrorJSON(trimmed)
	}
	return event, nil
}

func ParseResponsesFailureErrorJSON(raw string) error {
	detail := strings.TrimSpace(parseResponsesFailureDetailJSON(raw))
	code := strings.TrimSpace(parseResponsesFailureCodeJSON(raw))
	if detail == "" {
		detail = "response failed"
	}
	return WrapRetryError(
		fmt.Errorf("%s", detail),
		0,
		code,
		detail,
		0,
	)
}

func responsesStreamEventType(raw string, fallback string) string {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	var payload struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		if strings.TrimSpace(payload.Type) != "" {
			return strings.TrimSpace(payload.Type)
		}
	}
	if typ := strings.TrimSpace(extractPartialJSONStringField([]byte(raw), "type")); typ != "" {
		return typ
	}
	return fallback
}

func parseResponsesFailureDetailJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var payload struct {
		Error     json.RawMessage `json:"error"`
		RequestID string          `json:"request_id"`
		Response  struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			IncompleteDetails struct {
				Reason string `json:"reason"`
			} `json:"incomplete_details"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	appendRequestID := func(detail string) string {
		detail = strings.TrimSpace(detail)
		requestID := strings.TrimSpace(payload.RequestID)
		if detail == "" || requestID == "" || strings.Contains(detail, requestID) {
			return detail
		}
		return fmt.Sprintf("%s (request_id=%s)", detail, requestID)
	}
	if code := strings.TrimSpace(payload.Response.Error.Code); code != "" {
		return appendRequestID(firstNonEmptyResponsesString(
			code+": "+strings.TrimSpace(payload.Response.Error.Message),
			code,
		))
	}
	if msg := strings.TrimSpace(payload.Response.Error.Message); msg != "" {
		return appendRequestID(msg)
	}
	if reason := strings.TrimSpace(payload.Response.IncompleteDetails.Reason); reason != "" {
		return appendRequestID("incomplete: " + reason)
	}
	if len(payload.Error) > 0 {
		var text string
		if err := json.Unmarshal(payload.Error, &text); err == nil {
			return appendRequestID(strings.TrimSpace(text))
		}
		var structured struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(payload.Error, &structured); err == nil {
			return appendRequestID(firstNonEmptyResponsesString(
				strings.TrimSpace(structured.Code)+": "+strings.TrimSpace(structured.Message),
				strings.TrimSpace(structured.Message),
				strings.TrimSpace(structured.Code),
			))
		}
	}
	return ""
}

func parseResponsesFailureCodeJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var payload struct {
		Error    json.RawMessage `json:"error"`
		Response struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	topLevelCode := ""
	if len(payload.Error) > 0 {
		var structured struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(payload.Error, &structured); err == nil {
			topLevelCode = strings.TrimSpace(structured.Code)
		}
	}
	return firstNonEmptyResponsesString(
		strings.TrimSpace(payload.Response.Error.Code),
		topLevelCode,
	)
}

func parseResponsesStreamEventResponse(raw []byte) (*ResponsesResult, error) {
	var envelope struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode responses stream response event: %w", err)
	}
	if len(envelope.Response) == 0 {
		return &ResponsesResult{}, nil
	}
	return ParseResponsesResultJSON(envelope.Response)
}

func parseResponsesStreamEventItem(raw []byte) (*ResponseItem, error) {
	var envelope struct {
		Item json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode responses stream item event: %w", err)
	}
	if len(envelope.Item) == 0 {
		return nil, nil
	}
	item := ParseResponseItemRaw(envelope.Item)
	return &item, nil
}

func parseResponsesStreamEventDelta(raw []byte) string {
	var envelope struct {
		Delta     *string `json:"delta"`
		Text      *string `json:"text"`
		Arguments *string `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil {
		switch {
		case envelope.Delta != nil:
			return *envelope.Delta
		case envelope.Text != nil:
			return *envelope.Text
		case envelope.Arguments != nil:
			return *envelope.Arguments
		default:
			return ""
		}
	}
	if value := extractPartialJSONStringField(raw, "delta"); value != "" {
		return value
	}
	if value := extractPartialJSONStringField(raw, "text"); value != "" {
		return value
	}
	if value := extractPartialJSONStringField(raw, "arguments"); value != "" {
		return value
	}
	return ""
}

func extractPartialJSONStringField(raw []byte, field string) string {
	field = strings.TrimSpace(field)
	if field == "" || len(raw) == 0 {
		return ""
	}
	pattern := strconv.Quote(field)
	text := string(raw)
	searchFrom := 0
	for searchFrom < len(text) {
		idx := strings.Index(text[searchFrom:], pattern)
		if idx < 0 {
			return ""
		}
		idx += searchFrom + len(pattern)
		idx = skipJSONWhitespace(text, idx)
		if idx >= len(text) || text[idx] != ':' {
			searchFrom = idx
			continue
		}
		idx++
		idx = skipJSONWhitespace(text, idx)
		if idx >= len(text) || text[idx] != '"' {
			searchFrom = idx
			continue
		}
		value, ok := decodeJSONStringPrefix(text[idx+1:])
		if ok {
			return value
		}
		searchFrom = idx
	}
	return ""
}

func decodeJSONStringPrefix(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch ch {
		case '"':
			return b.String(), true
		case '\\':
			i++
			if i >= len(raw) {
				return b.String(), true
			}
			switch raw[i] {
			case '"', '\\', '/':
				b.WriteByte(raw[i])
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				if i+4 >= len(raw) {
					return b.String(), true
				}
				codePoint, err := strconv.ParseUint(raw[i+1:i+5], 16, 16)
				if err != nil {
					return b.String(), true
				}
				b.WriteRune(rune(codePoint))
				i += 4
			default:
				return b.String(), true
			}
		default:
			b.WriteByte(ch)
		}
	}
	return b.String(), true
}

func skipJSONWhitespace(raw string, idx int) int {
	for idx < len(raw) {
		switch raw[idx] {
		case ' ', '\t', '\n', '\r':
			idx++
		default:
			return idx
		}
	}
	return idx
}
