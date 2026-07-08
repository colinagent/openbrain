package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MarshalResponsesInputItemsJSON encodes replay/input items into Responses API
// request-shape JSON. Known item types are always serialized from structured
// fields instead of blindly replaying item.Raw, so stale output-shape payloads
// cannot poison later continuations.
func MarshalResponsesInputItemsJSON(items []ResponseItem) ([]json.RawMessage, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]json.RawMessage, 0, len(items))
	for idx, item := range items {
		raw, err := MarshalResponsesInputItemJSON(item, idx)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			continue
		}
		out = append(out, raw)
	}
	return out, nil
}

// MarshalResponsesInputItemJSON encodes one replay/input item into Responses API
// request-shape JSON.
func MarshalResponsesInputItemJSON(item ResponseItem, idx int) (json.RawMessage, error) {
	switch strings.TrimSpace(item.Type) {
	case "", "message":
		role := strings.ToLower(strings.TrimSpace(item.Role))
		if role == "" {
			role = "user"
		}
		if role == "assistant" {
			payload := map[string]any{
				"role":    "assistant",
				"content": marshalResponsesAssistantInputContentParts(item.Content),
			}
			if id := strings.TrimSpace(item.ID); id != "" {
				payload["id"] = id
			} else {
				payload["id"] = fmt.Sprintf("msg_%d", idx)
			}
			data, _ := json.Marshal(payload)
			return data, nil
		}
		content := marshalResponsesUserInputContentParts(item.Content)
		if len(content) == 0 {
			return nil, nil
		}
		payload := map[string]any{
			"type":    "message",
			"role":    role,
			"content": content,
		}
		if id := strings.TrimSpace(item.ID); id != "" {
			payload["id"] = id
		}
		data, _ := json.Marshal(payload)
		return data, nil
	case "reasoning":
		summary := marshalResponsesSummaryParts(item.Summary)
		if strings.TrimSpace(item.EncryptedContent) != "" {
			payload := map[string]any{
				"type":              "reasoning",
				"encrypted_content": strings.TrimSpace(item.EncryptedContent),
				"summary":           summary,
			}
			if id := strings.TrimSpace(item.ID); id != "" {
				payload["id"] = id
			}
			data, _ := json.Marshal(payload)
			return data, nil
		}
		if len(summary) == 0 {
			if len(item.Raw) > 0 {
				return cloneResponsesRawJSON(item.Raw), nil
			}
			return nil, nil
		}
		payload := map[string]any{
			"type":    "reasoning",
			"summary": summary,
		}
		if id := strings.TrimSpace(item.ID); id != "" {
			payload["id"] = id
		} else {
			payload["id"] = fmt.Sprintf("reasoning_%d", idx)
		}
		data, _ := json.Marshal(payload)
		return data, nil
	case "function_call":
		callID := strings.TrimSpace(item.CallID)
		name := strings.TrimSpace(item.Name)
		if callID == "" || name == "" {
			if len(item.Raw) > 0 {
				return cloneResponsesRawJSON(item.Raw), nil
			}
			return nil, fmt.Errorf("function_call call_id/name is required")
		}
		payload := map[string]any{
			"type":      "function_call",
			"call_id":   callID,
			"name":      name,
			"arguments": strings.TrimSpace(item.Arguments),
		}
		if id := strings.TrimSpace(item.ID); id != "" {
			payload["id"] = id
		}
		data, _ := json.Marshal(payload)
		return data, nil
	case "function_call_output":
		callID := strings.TrimSpace(item.CallID)
		if callID == "" {
			if len(item.Raw) > 0 {
				return cloneResponsesRawJSON(item.Raw), nil
			}
			return nil, fmt.Errorf("function_call_output call_id is required")
		}
		output := any(normalizeResponsesOutputText(item))
		if len(item.OutputContent) > 0 {
			output = marshalResponsesOutputContentParts(item.OutputContent)
		}
		payload := map[string]any{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  output,
		}
		data, _ := json.Marshal(payload)
		return data, nil
	case "compaction":
		encrypted := strings.TrimSpace(item.EncryptedContent)
		if encrypted == "" {
			if len(item.Raw) > 0 {
				return cloneResponsesRawJSON(item.Raw), nil
			}
			return nil, fmt.Errorf("compaction encrypted_content is required")
		}
		data, _ := json.Marshal(map[string]any{
			"type":              "compaction",
			"encrypted_content": encrypted,
		})
		return data, nil
	default:
		if len(item.Raw) > 0 {
			return cloneResponsesRawJSON(item.Raw), nil
		}
		return nil, fmt.Errorf("unsupported responses input item type %q", strings.TrimSpace(item.Type))
	}
}

// MarshalResponsesOutputItemsJSON encodes output/result items into stable
// output-shape JSON shared by gateway HTTP/SSE/WS surfaces.
func MarshalResponsesOutputItemsJSON(items []ResponseItem) []json.RawMessage {
	if len(items) == 0 {
		return []json.RawMessage{}
	}
	out := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		out = append(out, MarshalResponsesOutputItemJSON(item))
	}
	return out
}

// MarshalResponsesOutputItemJSON encodes one result/output item into stable
// output-shape JSON.
func MarshalResponsesOutputItemJSON(item ResponseItem) json.RawMessage {
	if !isKnownResponsesItem(item) && len(item.Raw) > 0 {
		return cloneResponsesRawJSON(item.Raw)
	}
	payload := map[string]any{}
	if item.Type != "" {
		payload["type"] = item.Type
	}
	if item.Role != "" {
		payload["role"] = item.Role
	}
	if item.ID != "" {
		payload["id"] = item.ID
	}
	if item.Status != "" {
		payload["status"] = item.Status
	}
	if item.CallID != "" {
		payload["call_id"] = item.CallID
	}
	if item.Name != "" {
		payload["name"] = item.Name
	}
	if item.Arguments != "" {
		payload["arguments"] = item.Arguments
	}
	if item.EncryptedContent != "" {
		payload["encrypted_content"] = item.EncryptedContent
	}
	if len(item.Content) > 0 {
		payload["content"] = marshalResponsesOutputContentParts(item.Content)
	}
	if len(item.Summary) > 0 {
		payload["summary"] = marshalResponsesSummaryParts(item.Summary)
	}
	if item.OutputText != "" {
		payload["output"] = item.OutputText
	} else if len(item.OutputContent) > 0 {
		payload["output"] = marshalResponsesOutputContentParts(item.OutputContent)
	} else if strings.TrimSpace(item.Type) == "function_call_output" {
		payload["output"] = ""
	}
	if len(payload) == 0 && len(item.Raw) > 0 {
		return cloneResponsesRawJSON(item.Raw)
	}
	data, _ := json.Marshal(payload)
	return data
}

func isKnownResponsesItem(item ResponseItem) bool {
	switch strings.TrimSpace(item.Type) {
	case "", "message", "reasoning", "function_call", "function_call_output", "compaction":
		return true
	default:
		return false
	}
}

func normalizeResponsesOutputText(item ResponseItem) string {
	if text := strings.TrimSpace(item.OutputText); text != "" {
		return text
	}
	if len(item.OutputContent) == 0 {
		return ""
	}
	texts := make([]string, 0, len(item.OutputContent))
	for _, part := range item.OutputContent {
		if text := strings.TrimSpace(part.Text); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

func marshalResponsesAssistantInputContentParts(parts []ResponseContentPart) []map[string]any {
	out := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		out = append(out, map[string]any{
			"type":        firstNonEmptyResponsesString(strings.TrimSpace(part.Type), "output_text"),
			"text":        text,
			"annotations": []any{},
		})
	}
	return out
}

func marshalResponsesUserInputContentParts(parts []ResponseContentPart) []map[string]any {
	out := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		partType := strings.TrimSpace(part.Type)
		switch partType {
		case "", "text", "input_text", "output_text":
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			out = append(out, map[string]any{
				"type": "input_text",
				"text": text,
			})
		case "image_url", "input_image":
			imageURL := strings.TrimSpace(part.ImageURL)
			if imageURL == "" {
				continue
			}
			entry := map[string]any{
				"type":      "input_image",
				"image_url": imageURL,
			}
			if detail := strings.TrimSpace(part.Detail); detail != "" {
				entry["detail"] = detail
			}
			out = append(out, entry)
		}
	}
	return out
}

func marshalResponsesOutputContentParts(parts []ResponseContentPart) []map[string]any {
	out := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		entry := map[string]any{
			"type": firstNonEmptyResponsesString(strings.TrimSpace(part.Type), "output_text"),
		}
		if text := strings.TrimSpace(part.Text); text != "" {
			entry["text"] = text
		}
		if imageURL := strings.TrimSpace(part.ImageURL); imageURL != "" {
			entry["image_url"] = imageURL
			if detail := strings.TrimSpace(part.Detail); detail != "" {
				entry["detail"] = detail
			}
		}
		if entry["type"] == "output_text" {
			entry["annotations"] = []any{}
		}
		out = append(out, entry)
	}
	return out
}

func marshalResponsesSummaryParts(parts []ResponseSummaryPart) []map[string]any {
	if len(parts) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		out = append(out, map[string]any{
			"type": firstNonEmptyResponsesString(strings.TrimSpace(part.Type), "summary_text"),
			"text": strings.TrimSpace(part.Text),
		})
	}
	return out
}

func cloneResponsesRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func firstNonEmptyResponsesString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
