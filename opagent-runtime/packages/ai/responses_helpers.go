package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ParseResponseItemRaw(raw json.RawMessage) ResponseItem {
	item := ResponseItem{Raw: append(json.RawMessage(nil), raw...)}
	if len(raw) == 0 {
		return item
	}
	var envelope struct {
		Type             string          `json:"type"`
		Role             string          `json:"role"`
		ID               string          `json:"id"`
		Status           string          `json:"status"`
		CallID           string          `json:"call_id"`
		Name             string          `json:"name"`
		Arguments        string          `json:"arguments"`
		EncryptedContent string          `json:"encrypted_content"`
		Content          json.RawMessage `json:"content"`
		Summary          json.RawMessage `json:"summary"`
		Output           json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return item
	}
	item.Type = strings.TrimSpace(envelope.Type)
	item.Role = strings.TrimSpace(envelope.Role)
	item.ID = strings.TrimSpace(envelope.ID)
	item.Status = strings.TrimSpace(envelope.Status)
	item.CallID = strings.TrimSpace(envelope.CallID)
	item.Name = strings.TrimSpace(envelope.Name)
	item.Arguments = strings.TrimSpace(envelope.Arguments)
	item.EncryptedContent = strings.TrimSpace(envelope.EncryptedContent)
	item.Content = parseResponseContentParts(envelope.Content)
	item.Summary = parseResponseSummaryParts(envelope.Summary)
	item.OutputText, item.OutputContent = parseResponseOutput(envelope.Output)
	return item
}

func parseResponseContentParts(raw json.RawMessage) []ResponseContentPart {
	if len(raw) == 0 {
		return nil
	}
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}
	out := make([]ResponseContentPart, 0, len(parts))
	for _, part := range parts {
		content := ResponseContentPart{
			Type: strings.TrimSpace(fmt.Sprint(part["type"])),
		}
		if text := strings.TrimSpace(fmt.Sprint(part["text"])); text != "" && text != "<nil>" {
			content.Text = text
		}
		switch value := part["image_url"].(type) {
		case string:
			content.ImageURL = strings.TrimSpace(value)
		case map[string]any:
			if url := strings.TrimSpace(fmt.Sprint(value["url"])); url != "" && url != "<nil>" {
				content.ImageURL = url
			}
			if detail := strings.TrimSpace(fmt.Sprint(value["detail"])); detail != "" && detail != "<nil>" {
				content.Detail = detail
			}
		}
		out = append(out, content)
	}
	return out
}

func parseResponseSummaryParts(raw json.RawMessage) []ResponseSummaryPart {
	if len(raw) == 0 {
		return nil
	}
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}
	out := make([]ResponseSummaryPart, 0, len(parts))
	for _, part := range parts {
		out = append(out, ResponseSummaryPart{
			Type: strings.TrimSpace(fmt.Sprint(part["type"])),
			Text: strings.TrimSpace(fmt.Sprint(part["text"])),
		})
	}
	return out
}

func parseResponseOutput(raw json.RawMessage) (string, []ResponseContentPart) {
	if len(raw) == 0 {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text), nil
	}
	return "", parseResponseContentParts(raw)
}

func ResponseContentTexts(parts []ResponseContentPart) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := strings.TrimSpace(part.Text); text != "" {
			out = append(out, text)
		}
	}
	return out
}
