package op

import (
	"encoding/json"
	"fmt"
	"strings"
)

func NewUserMessageParts(parts []ContentPart) Message {
	nextParts := make([]ContentPart, 0, len(parts))
	for _, part := range parts {
		nextParts = append(nextParts, part)
	}
	return Message{
		Role:         RoleUser,
		ContentParts: nextParts,
	}
}

func DecodeUserMessageContent(content Content) (Message, error) {
	switch value := content.(type) {
	case *TextContent:
		if value == nil {
			return Message{}, fmt.Errorf("user content is required")
		}
		return NewUserMessage(value.Text), nil
	case *JsonContent:
		if value == nil || len(value.Raw) == 0 {
			return Message{}, fmt.Errorf("user content payload is required")
		}
		var msg Message
		if err := json.Unmarshal(value.Raw, &msg); err != nil {
			return Message{}, fmt.Errorf("decode user message payload: %w", err)
		}
		if msg.Role == "" {
			msg.Role = RoleUser
		}
		if msg.Role != RoleUser {
			return Message{}, fmt.Errorf("user message payload role must be %q", RoleUser)
		}
		if strings.TrimSpace(msg.Content) == "" && len(msg.ContentParts) == 0 {
			return Message{}, fmt.Errorf("user message requires content or content_parts")
		}
		if err := validateTextOnlyUserContentParts(msg.ContentParts); err != nil {
			return Message{}, err
		}
		return msg, nil
	default:
		return Message{}, fmt.Errorf("unsupported user content type %T", content)
	}
}

func validateTextOnlyUserContentParts(parts []ContentPart) error {
	for _, part := range parts {
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "", "text":
			// ok
		default:
			return fmt.Errorf("user message content_parts only supports text; send images as markdown paths")
		}
	}
	return nil
}

func summaryText(msg Message) string {
	if strings.TrimSpace(msg.Content) != "" {
		return strings.TrimSpace(msg.Content)
	}
	if len(msg.ContentParts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(msg.ContentParts))
	for _, part := range msg.ContentParts {
		typ := strings.ToLower(strings.TrimSpace(part.Type))
		switch typ {
		case "", "text":
			text := strings.TrimSpace(part.Text)
			if text != "" {
				parts = append(parts, text)
			}
		case "image", "image_url":
			if part.ImageURL != nil && strings.TrimSpace(part.ImageURL.URL) != "" {
				parts = append(parts, "[Image]")
			}
		}
	}
	return strings.Join(parts, "\n")
}
