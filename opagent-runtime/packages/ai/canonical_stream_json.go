package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

type canonicalStreamErrorPayload struct {
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable,omitempty"`
	StatusCode   int    `json:"statusCode,omitempty"`
	Code         string `json:"code,omitempty"`
	RetryAfterMs int64  `json:"retryAfterMs,omitempty"`
}

type canonicalStreamEventPayload struct {
	Type         ProviderEventType            `json:"type"`
	ContentIndex *int                         `json:"contentIndex,omitempty"`
	Delta        string                       `json:"delta,omitempty"`
	Content      string                       `json:"content,omitempty"`
	Block        *StreamContentBlock          `json:"block,omitempty"`
	Response     json.RawMessage              `json:"response,omitempty"`
	Error        *canonicalStreamErrorPayload `json:"error,omitempty"`
}

type canonicalWebsocketAckPayload struct {
	Type         string            `json:"type"`
	TerminalType ProviderEventType `json:"terminalType"`
	RequestID    string            `json:"requestID,omitempty"`
}

const canonicalWebsocketAckType = "canonical.ack"

func ParseCanonicalStreamEventJSON(raw []byte) (ProviderEvent, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ProviderEvent{}, fmt.Errorf("canonical stream event is empty")
	}
	if trimmed == "[DONE]" {
		return ProviderEvent{}, fmt.Errorf("canonical stream requires an explicit terminal event")
	}
	var payload canonicalStreamEventPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ProviderEvent{}, fmt.Errorf("invalid canonical stream event")
	}
	if !isCanonicalStreamEventType(payload.Type) {
		return ProviderEvent{}, fmt.Errorf("unsupported canonical stream event type %q", payload.Type)
	}
	event := ProviderEvent{
		Type:    payload.Type,
		Raw:     append(json.RawMessage(nil), raw...),
		Delta:   payload.Delta,
		Content: payload.Content,
		Block:   CloneStreamContentBlockPtr(payload.Block),
	}
	if payload.ContentIndex != nil {
		event.ContentIndex = *payload.ContentIndex
	}
	if len(payload.Response) > 0 {
		if payload.Type != EventCanonicalDone {
			return ProviderEvent{}, fmt.Errorf("canonical response is only valid on done events")
		}
		resp, err := ParseCanonicalAPIResponseJSON(payload.Response)
		if err != nil {
			return ProviderEvent{}, err
		}
		event.Response = resp
	}
	if payload.Type == EventCanonicalDone && event.Response == nil {
		return ProviderEvent{}, fmt.Errorf("canonical done event missing response")
	}
	if payload.Error != nil {
		if payload.Type != EventCanonicalError {
			return ProviderEvent{}, fmt.Errorf("canonical error payload is only valid on error events")
		}
		msg := strings.TrimSpace(payload.Error.Message)
		if msg == "" {
			msg = "canonical stream error"
		}
		event.Error = &RetryError{
			Retryable:    payload.Error.Retryable,
			StatusCode:   payload.Error.StatusCode,
			Code:         strings.TrimSpace(payload.Error.Code),
			Message:      msg,
			RetryAfterMs: payload.Error.RetryAfterMs,
			Err:          fmt.Errorf("%s", msg),
		}
	}
	if payload.Type == EventCanonicalError && event.Error == nil {
		return ProviderEvent{}, fmt.Errorf("canonical error event missing error")
	}
	return event, nil
}

func MarshalCanonicalStreamEventJSON(event ProviderEvent) ([]byte, error) {
	if !isCanonicalStreamEventType(event.Type) {
		return nil, fmt.Errorf("unsupported canonical stream event type %q", event.Type)
	}
	payload := canonicalStreamEventPayload{
		Type:    event.Type,
		Delta:   event.Delta,
		Content: event.Content,
	}
	switch event.Type {
	case EventCanonicalTextStart,
		EventCanonicalTextDelta,
		EventCanonicalTextEnd,
		EventCanonicalThinkingStart,
		EventCanonicalThinkingDelta,
		EventCanonicalThinkingEnd,
		EventCanonicalToolCallStart,
		EventCanonicalToolCallDelta,
		EventCanonicalToolCallEnd:
		idx := event.ContentIndex
		payload.ContentIndex = &idx
	}
	if event.Type == EventCanonicalToolCallStart || event.Type == EventCanonicalToolCallEnd {
		payload.Block = CloneStreamContentBlockPtr(event.Block)
	}
	if event.Response != nil {
		if event.Type != EventCanonicalDone {
			return nil, fmt.Errorf("canonical response is only valid on done events")
		}
		response, err := MarshalCanonicalAPIResponseJSON(event.Response)
		if err != nil {
			return nil, err
		}
		payload.Response = response
	} else if event.Type == EventCanonicalDone {
		return nil, fmt.Errorf("canonical done event missing response")
	}
	if event.Error != nil {
		if event.Type != EventCanonicalError {
			return nil, fmt.Errorf("canonical error payload is only valid on error events")
		}
		retryErr := NormalizeRetryError(event.Error)
		payload.Error = &canonicalStreamErrorPayload{Message: strings.TrimSpace(event.Error.Error())}
		if retryErr != nil {
			payload.Error.Message = firstNonEmptyString(strings.TrimSpace(retryErr.Message), payload.Error.Message)
			payload.Error.Retryable = retryErr.Retryable
			payload.Error.StatusCode = retryErr.StatusCode
			payload.Error.Code = strings.TrimSpace(retryErr.Code)
			payload.Error.RetryAfterMs = retryErr.RetryAfterMs
		}
	} else if event.Type == EventCanonicalError {
		return nil, fmt.Errorf("canonical error event missing error")
	}
	return json.Marshal(payload)
}

func MarshalCanonicalWebsocketAckJSON(terminalType ProviderEventType, requestID string) ([]byte, error) {
	if terminalType != EventCanonicalDone && terminalType != EventCanonicalError {
		return nil, fmt.Errorf("unsupported canonical terminal event type %q", terminalType)
	}
	return json.Marshal(canonicalWebsocketAckPayload{
		Type:         canonicalWebsocketAckType,
		TerminalType: terminalType,
		RequestID:    strings.TrimSpace(requestID),
	})
}

func DecodeCanonicalWebsocketAckJSON(raw []byte) (ProviderEventType, string, error) {
	var payload canonicalWebsocketAckPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("invalid canonical websocket acknowledgement")
	}
	if strings.TrimSpace(payload.Type) != canonicalWebsocketAckType {
		return "", "", fmt.Errorf("unsupported websocket acknowledgement")
	}
	if payload.TerminalType != EventCanonicalDone && payload.TerminalType != EventCanonicalError {
		return "", "", fmt.Errorf("invalid canonical terminal acknowledgement")
	}
	return payload.TerminalType, strings.TrimSpace(payload.RequestID), nil
}

func isCanonicalStreamEventType(eventType ProviderEventType) bool {
	switch eventType {
	case EventCanonicalStart,
		EventCanonicalTextStart,
		EventCanonicalTextDelta,
		EventCanonicalTextEnd,
		EventCanonicalThinkingStart,
		EventCanonicalThinkingDelta,
		EventCanonicalThinkingEnd,
		EventCanonicalToolCallStart,
		EventCanonicalToolCallDelta,
		EventCanonicalToolCallEnd,
		EventCanonicalDone,
		EventCanonicalError:
		return true
	default:
		return false
	}
}
