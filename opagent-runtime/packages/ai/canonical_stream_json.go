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
	Partial      *StreamConversationMessage   `json:"partial,omitempty"`
	Response     json.RawMessage              `json:"response,omitempty"`
	Error        *canonicalStreamErrorPayload `json:"error,omitempty"`
}

func ParseCanonicalStreamEventJSON(raw []byte) (ProviderEvent, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "[DONE]" {
		return ProviderEvent{}, nil
	}
	var payload canonicalStreamEventPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ProviderEvent{}, fmt.Errorf("invalid canonical stream event")
	}
	if strings.TrimSpace(string(payload.Type)) == "" {
		return ProviderEvent{}, fmt.Errorf("canonical stream event missing type")
	}
	event := ProviderEvent{
		Type:    payload.Type,
		Raw:     append(json.RawMessage(nil), raw...),
		Delta:   payload.Delta,
		Content: payload.Content,
		Block:   CloneStreamContentBlockPtr(payload.Block),
		Partial: CloneStreamConversationMessagePtr(payload.Partial),
	}
	if payload.ContentIndex != nil {
		event.ContentIndex = *payload.ContentIndex
	}
	if len(payload.Response) > 0 {
		resp, err := ParseCanonicalAPIResponseJSON(payload.Response)
		if err != nil {
			return ProviderEvent{}, err
		}
		event.Response = resp
	}
	if payload.Error != nil {
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
	return event, nil
}

func RenderCanonicalStreamEventJSON(event ProviderEvent) json.RawMessage {
	payload := canonicalStreamEventPayload{
		Type:    event.Type,
		Delta:   event.Delta,
		Content: event.Content,
		Block:   CloneStreamContentBlockPtr(event.Block),
		Partial: CloneStreamConversationMessagePtr(event.Partial),
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
	if event.Response != nil {
		payload.Response = RenderCanonicalAPIResponseJSON(event.Response)
	}
	if event.Error != nil {
		retryErr := NormalizeRetryError(event.Error)
		payload.Error = &canonicalStreamErrorPayload{Message: strings.TrimSpace(event.Error.Error())}
		if retryErr != nil {
			payload.Error.Message = firstNonEmptyString(strings.TrimSpace(retryErr.Message), payload.Error.Message)
			payload.Error.Retryable = retryErr.Retryable
			payload.Error.StatusCode = retryErr.StatusCode
			payload.Error.Code = strings.TrimSpace(retryErr.Code)
			payload.Error.RetryAfterMs = retryErr.RetryAfterMs
		}
	}
	data, _ := json.Marshal(payload)
	return data
}
