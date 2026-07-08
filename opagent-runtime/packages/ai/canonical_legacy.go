package ai

import (
	"encoding/json"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func DefaultCapabilitiesForAPI(api string) ProviderCapabilities {
	switch strings.TrimSpace(api) {
	case "openai-completions":
		return ProviderCapabilities{
			SupportsThinkingBlocks:    true,
			SupportsToolCalls:         true,
			SupportsParallelToolCalls: true,
			SupportsImages:            true,
			SupportsStatelessReplay:   true,
		}
	case "openai-responses":
		return ProviderCapabilities{
			SupportsThinkingBlocks:     true,
			SupportsToolCalls:          true,
			SupportsParallelToolCalls:  true,
			SupportsImages:             true,
			SupportsStatelessReplay:    true,
			SupportsPreviousResponseID: true,
			SupportsCompaction:         true,
			SupportsWebsocketStream:    false,
		}
	case "anthropic-messages":
		return ProviderCapabilities{
			SupportsThinkingBlocks:    true,
			SupportsToolCalls:         true,
			SupportsParallelToolCalls: true,
			SupportsImages:            true,
			SupportsStatelessReplay:   true,
		}
	case "gemini-native":
		return ProviderCapabilities{
			SupportsThinkingBlocks:    true,
			SupportsToolCalls:         true,
			SupportsParallelToolCalls: true,
			SupportsImages:            true,
			SupportsStatelessReplay:   true,
		}
	default:
		return ProviderCapabilities{
			SupportsThinkingBlocks:    true,
			SupportsToolCalls:         true,
			SupportsParallelToolCalls: true,
			SupportsImages:            true,
			SupportsStatelessReplay:   true,
		}
	}
}

func OpMessageFromCanonical(msg ConversationMessage) (op.Message, error) {
	converted, err := opMessageFromCanonical(msg)
	if err != nil {
		return op.Message{}, err
	}
	converted.Timestamp = msg.Timestamp
	converted.StopReason = op.MessageStopReason(msg.StopReason)
	if msg.Usage != nil {
		usage := *msg.Usage
		converted.Usage = &usage
	}
	if msg.ProviderState != nil {
		converted.ResponseID = strings.TrimSpace(msg.ProviderState.ResponseID)
	}
	return converted, nil
}

func responseOutputItemsFromCanonicalAssistantBlocks(blocks []ContentBlock) json.RawMessage {
	if len(blocks) == 0 {
		return nil
	}
	rawItems := make([]json.RawMessage, 0, len(blocks))
	for _, block := range blocks {
		if len(block.Raw) == 0 {
			continue
		}
		rawItems = append(rawItems, append(json.RawMessage(nil), block.Raw...))
	}
	if len(rawItems) == 0 {
		return nil
	}
	data, err := json.Marshal(rawItems)
	if err != nil {
		return nil
	}
	return data
}

func canonicalAssistantContentFromResponseOutputItems(raw json.RawMessage) []ContentBlock {
	if len(raw) == 0 {
		return nil
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(raw, &rawItems); err != nil {
		return nil
	}
	content := make([]ContentBlock, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item := ParseResponseItemRaw(rawItem)
		switch item.Type {
		case "reasoning":
			if strings.TrimSpace(item.EncryptedContent) == "" {
				continue
			}
			content = append(content, ContentBlock{
				Type:              BlockThinking,
				Text:              reasoningTextFromResponseItem(item),
				ThinkingSignature: strings.TrimSpace(item.ID),
				EncryptedContent:  strings.TrimSpace(item.EncryptedContent),
				Raw:               append(json.RawMessage(nil), rawItem...),
			})
		case "message":
			if strings.ToLower(strings.TrimSpace(item.Role)) != "assistant" {
				continue
			}
			text := strings.TrimSpace(strings.Join(ResponseContentTexts(item.Content), "\n"))
			if text == "" {
				continue
			}
			content = append(content, ContentBlock{
				Type:          BlockText,
				Text:          text,
				TextSignature: strings.TrimSpace(item.ID),
				Raw:           append(json.RawMessage(nil), rawItem...),
			})
		case "function_call":
			callID := strings.TrimSpace(item.CallID)
			name := strings.TrimSpace(item.Name)
			if callID == "" || name == "" {
				continue
			}
			block := ContentBlock{
				Type: BlockToolCall,
				ToolCall: &CanonicalToolCall{
					ID:           callID,
					Name:         name,
					RawArguments: strings.TrimSpace(item.Arguments),
					Raw:          append(json.RawMessage(nil), rawItem...),
				},
				Raw: append(json.RawMessage(nil), rawItem...),
			}
			block.ToolCall.Arguments = ParseToolArgumentsObject(block.ToolCall.RawArguments)
			content = append(content, block)
		}
	}
	return content
}

func reasoningTextFromResponseItem(item ResponseItem) string {
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

func canonicalAssistantContentFromResponseItems(items []ResponseItem) []ContentBlock {
	if len(items) == 0 {
		return nil
	}
	rawItems := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		if len(item.Raw) == 0 {
			rawItems = append(rawItems, marshalResponseItemRawForLegacy(item))
			continue
		}
		rawItems = append(rawItems, append(json.RawMessage(nil), item.Raw...))
	}
	data, err := json.Marshal(rawItems)
	if err != nil {
		return nil
	}
	return canonicalAssistantContentFromResponseOutputItems(data)
}

func marshalResponseItemRawForLegacy(item ResponseItem) json.RawMessage {
	payload := map[string]any{
		"type": item.Type,
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
		payload["arguments"] = strings.TrimSpace(item.Arguments)
	}
	if item.EncryptedContent != "" {
		payload["encrypted_content"] = item.EncryptedContent
	}
	if len(item.Content) > 0 {
		content := make([]map[string]any, 0, len(item.Content))
		for _, part := range item.Content {
			entry := map[string]any{"type": firstNonEmptyStringForLegacy(part.Type, "input_text")}
			if part.Text != "" {
				entry["text"] = part.Text
			}
			if part.ImageURL != "" {
				entry["image_url"] = part.ImageURL
			}
			if part.Detail != "" {
				entry["detail"] = part.Detail
			}
			if entry["type"] == "output_text" {
				entry["annotations"] = []any{}
			}
			content = append(content, entry)
		}
		payload["content"] = content
	}
	if len(item.Summary) > 0 {
		summary := make([]map[string]any, 0, len(item.Summary))
		for _, part := range item.Summary {
			summary = append(summary, map[string]any{
				"type": firstNonEmptyStringForLegacy(part.Type, "summary_text"),
				"text": part.Text,
			})
		}
		payload["summary"] = summary
	}
	if item.OutputText != "" {
		payload["output"] = item.OutputText
	}
	data, _ := json.Marshal(payload)
	return data
}

func firstNonEmptyStringForLegacy(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func opMessageFromCanonical(msg ConversationMessage) (op.Message, error) {
	switch msg.Role {
	case RoleCanonicalSystem:
		return op.Message{Role: op.RoleSystem, Content: canonicalText(msg.Content)}, nil
	case RoleCanonicalDeveloper:
		return op.Message{Role: op.RoleDeveloper, Content: canonicalText(msg.Content)}, nil
	case RoleCanonicalUser:
		content, parts := canonicalUserContent(msg.Content)
		return op.Message{Role: op.RoleUser, Content: content, ContentParts: parts}, nil
	case RoleCanonicalAssistant:
		out := op.Message{Role: op.RoleAssistant}
		var toolCalls []op.MessageToolCall
		for _, block := range msg.Content {
			switch block.Type {
			case BlockText:
				if text := strings.TrimSpace(block.Text); text != "" {
					if out.Content != "" {
						out.Content += "\n"
					}
					out.Content += text
				}
			case BlockThinking:
				if text := strings.TrimSpace(block.Text); text != "" {
					if out.ReasoningContent != "" {
						out.ReasoningContent += "\n"
					}
					out.ReasoningContent += text
				}
				if field := strings.TrimSpace(block.ThinkingReplayField); field != "" && out.ReasoningReplayField == "" {
					out.ReasoningReplayField = field
				}
				if sig := strings.TrimSpace(block.ThinkingSignature); sig != "" {
					out.ReasoningSignature = sig
				}
			case BlockToolCall:
				if block.ToolCall == nil {
					continue
				}
				call := op.MessageToolCall{
					ID:        strings.TrimSpace(block.ToolCall.ID),
					Name:      strings.TrimSpace(block.ToolCall.Name),
					Arguments: CloneToolArguments(block.ToolCall.Arguments),
					Type:      "function",
				}
				toolCalls = append(toolCalls, call)
			}
		}
		out.ToolCalls = toolCalls
		return out, nil
	case RoleCanonicalTool:
		result := op.Message{
			Role:    op.RoleTool,
			Name:    "",
			Content: canonicalText(msg.Content),
		}
		for _, block := range msg.Content {
			if block.Type == BlockToolResult && block.ToolResult != nil {
				result.ToolCallID = strings.TrimSpace(block.ToolResult.ToolCallID)
				result.Name = strings.TrimSpace(block.ToolResult.ToolName)
				if text := strings.TrimSpace(block.ToolResult.OutputText); text != "" {
					result.Content = text
				}
				if len(block.ToolResult.OutputContent) > 0 {
					_, result.ContentParts = canonicalUserContent(block.ToolResult.OutputContent)
				}
			}
		}
		return result, nil
	default:
		return op.Message{}, nil
	}
}

func canonicalText(blocks []ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == BlockText || block.Type == BlockThinking || block.Type == BlockCompaction {
			if text := strings.TrimSpace(block.Text); text != "" {
				parts = append(parts, text)
			}
		} else if block.Type == BlockToolResult && block.ToolResult != nil && strings.TrimSpace(block.ToolResult.OutputText) != "" {
			parts = append(parts, strings.TrimSpace(block.ToolResult.OutputText))
		}
	}
	return strings.Join(parts, "\n")
}

func canonicalUserContent(blocks []ContentBlock) (string, []op.ContentPart) {
	parts := make([]op.ContentPart, 0, len(blocks))
	texts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case BlockText:
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			texts = append(texts, text)
			parts = append(parts, op.ContentPart{Type: "text", Text: text})
		case BlockImage:
			if strings.TrimSpace(block.ImageData) == "" {
				continue
			}
			parts = append(parts, op.ContentPart{
				Type: "image_url",
				ImageURL: &op.ImageURL{
					URL:    strings.TrimSpace(block.ImageData),
					Detail: "auto",
				},
			})
		}
	}
	return strings.Join(texts, "\n"), parts
}

func canonicalToolResultContentFromOp(msg op.Message, outputText string) []ContentBlock {
	if len(msg.ContentParts) == 0 {
		return nil
	}
	content := make([]ContentBlock, 0, len(msg.ContentParts))
	for _, part := range msg.ContentParts {
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "", "text":
			if text := strings.TrimSpace(part.Text); text != "" {
				content = append(content, ContentBlock{Type: BlockText, Text: text})
			}
		case "image", "image_url":
			if part.ImageURL != nil && strings.TrimSpace(part.ImageURL.URL) != "" {
				content = append(content, ContentBlock{
					Type:      BlockImage,
					ImageData: strings.TrimSpace(part.ImageURL.URL),
					MimeType:  strings.TrimSpace(part.ImageURL.Detail),
				})
			}
		}
	}
	if len(content) == 0 {
		return nil
	}
	hasText := false
	for _, block := range content {
		if block.Type == BlockText && strings.TrimSpace(block.Text) != "" {
			hasText = true
			break
		}
	}
	if !hasText && strings.TrimSpace(outputText) != "" {
		content = append([]ContentBlock{{Type: BlockText, Text: strings.TrimSpace(outputText)}}, content...)
	}
	return content
}

func CanonicalMessagesFromOp(messages []op.Message) []ConversationMessage {
	return canonicalMessagesFromOp(messages, false)
}

func canonicalMessagesFromOp(messages []op.Message, truncateToolOutput bool) []ConversationMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, canonicalMessageFromOp(msg, truncateToolOutput))
	}
	return out
}

func CanonicalToolsFromOp(specs []op.ToolSpec) []ToolDefinition {
	if len(specs) == 0 {
		return nil
	}
	out := make([]ToolDefinition, 0, len(specs))
	for _, spec := range specs {
		out = append(out, ToolDefinition{
			Name:        strings.TrimSpace(spec.Name),
			Description: strings.TrimSpace(spec.Description),
			Parameters:  spec.InputSchema,
		})
	}
	return out
}

func canonicalMessageFromOp(msg op.Message, truncateToolOutput bool) ConversationMessage {
	canonical := ConversationMessage{
		Timestamp:  msg.Timestamp,
		StopReason: StopReason(msg.StopReason),
	}
	if msg.Usage != nil {
		usage := *msg.Usage
		canonical.Usage = &usage
	}
	if strings.TrimSpace(msg.ResponseID) != "" {
		canonical.ProviderState = &ProviderState{
			ResponseID: strings.TrimSpace(msg.ResponseID),
		}
	}
	switch msg.Role {
	case op.RoleSystem:
		canonical.Role = RoleCanonicalSystem
		canonical.Content = []ContentBlock{{Type: BlockText, Text: strings.TrimSpace(msg.Content)}}
	case op.RoleDeveloper:
		canonical.Role = RoleCanonicalDeveloper
		canonical.Content = []ContentBlock{{Type: BlockText, Text: strings.TrimSpace(msg.Content)}}
	case op.RoleUser:
		canonical.Role = RoleCanonicalUser
		content := make([]ContentBlock, 0, len(msg.ContentParts)+1)
		if len(msg.ContentParts) > 0 {
			for _, part := range msg.ContentParts {
				switch strings.TrimSpace(part.Type) {
				case "text", "":
					if text := strings.TrimSpace(part.Text); text != "" {
						content = append(content, ContentBlock{Type: BlockText, Text: text})
					}
				case "image_url", "image":
					if part.ImageURL != nil && strings.TrimSpace(part.ImageURL.URL) != "" {
						content = append(content, ContentBlock{Type: BlockImage, ImageData: strings.TrimSpace(part.ImageURL.URL), MimeType: strings.TrimSpace(part.ImageURL.Detail)})
					}
				}
			}
		} else if text := strings.TrimSpace(msg.Content); text != "" {
			content = append(content, ContentBlock{Type: BlockText, Text: text})
		}
		canonical.Content = content
	case op.RoleAssistant:
		canonical.Role = RoleCanonicalAssistant
		content := make([]ContentBlock, 0, 2+len(msg.ToolCalls))
		if text := strings.TrimSpace(msg.ReasoningContent); text != "" {
			content = append(content, ContentBlock{Type: BlockThinking, Text: text, ThinkingReplayField: strings.TrimSpace(msg.ReasoningReplayField), ThinkingSignature: strings.TrimSpace(msg.ReasoningSignature)})
		}
		if text := strings.TrimSpace(msg.Content); text != "" {
			content = append(content, ContentBlock{Type: BlockText, Text: text})
		}
		for _, call := range msg.ToolCalls {
			block := ContentBlock{
				Type: BlockToolCall,
				ToolCall: &CanonicalToolCall{
					ID:           strings.TrimSpace(call.ID),
					Name:         strings.TrimSpace(call.Name),
					RawArguments: MarshalToolArgumentsJSON(call.Arguments),
					Arguments:    CloneToolArguments(call.Arguments),
				},
			}
			content = append(content, block)
		}
		canonical.Content = content
	case op.RoleTool:
		outputText := msg.Content
		if truncateToolOutput {
			outputText = truncateToolOutputForReplay(outputText)
		}
		outputContent := canonicalToolResultContentFromOp(msg, outputText)
		if truncateToolOutput && len(outputContent) > 0 {
			for i := range outputContent {
				if outputContent[i].Type == BlockText {
					outputContent[i].Text = truncateToolOutputForReplay(outputContent[i].Text)
				}
			}
		}
		canonical.Role = RoleCanonicalTool
		canonical.Content = []ContentBlock{{
			Type: BlockToolResult,
			ToolResult: &CanonicalToolResult{
				ToolCallID:    strings.TrimSpace(msg.ToolCallID),
				ToolName:      strings.TrimSpace(msg.Name),
				OutputText:    outputText,
				OutputContent: outputContent,
			},
		}}
	default:
		canonical.Content = []ContentBlock{}
	}
	return canonical
}
