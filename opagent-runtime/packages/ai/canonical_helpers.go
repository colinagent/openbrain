package ai

import "strings"

func HasSemanticCanonicalResponse(resp *ProviderResponse) bool {
	if resp == nil {
		return false
	}
	return HasSemanticConversationMessage(resp.Message)
}

func HasSemanticConversationMessage(msg ConversationMessage) bool {
	for _, block := range msg.Content {
		switch block.Type {
		case BlockText:
			if strings.TrimSpace(block.Text) != "" {
				return true
			}
		case BlockImage:
			if strings.TrimSpace(block.ImageData) != "" || strings.TrimSpace(block.MimeType) != "" {
				return true
			}
		case BlockToolCall:
			if block.ToolCall == nil {
				continue
			}
			if strings.TrimSpace(block.ToolCall.ID) != "" || strings.TrimSpace(block.ToolCall.Name) != "" || strings.TrimSpace(block.ToolCall.RawArguments) != "" || len(block.ToolCall.Arguments) > 0 {
				return true
			}
		case BlockCompaction:
			if strings.TrimSpace(block.Text) != "" || len(block.Raw) > 0 {
				return true
			}
		}
	}
	return false
}
