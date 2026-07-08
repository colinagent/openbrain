package ai

import "strings"

type ReplayTarget struct {
	ProviderRef string
	Provider    string
	API         string
	Model       string
}

const (
	semanticHandoffToolOutputMaxLines = 400
	semanticHandoffToolOutputMaxBytes = 8 * 1024
	semanticHandoffToolOutputTotalMax = 48 * 1024

	semanticHandoffToolOutputOmittedNotice = "[Historical tool output omitted for semantic handoff due to aggregate replay budget.]"
	semanticHandoffToolOutputBudgetNotice  = "[Historical tool output truncated for semantic handoff due to aggregate replay budget.]"
)

// PrepareCanonicalReplayForTarget returns a request-only replay view for the
// target endpoint. Provider replay state is preserved only for the exact
// providerRef/API/model that produced it; cross-target history is downgraded to
// ordinary semantic context so provider-specific thinking/tool protocols cannot
// leak into a different endpoint.
func PrepareCanonicalReplayForTarget(req *ProviderRequest, target ReplayTarget) *ProviderRequest {
	if req == nil {
		return &ProviderRequest{}
	}
	out := cloneProviderRequestForReplay(req)
	target = normalizeReplayTarget(target)
	if target.API == "" {
		return out
	}
	out.Context.Messages = prepareCanonicalMessagesForTarget(out.Context.Messages, target)
	return out
}

func prepareCanonicalMessagesForTarget(messages []ConversationMessage, target ReplayTarget) []ConversationMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]ConversationMessage, 0, len(messages))
	toolCallExactReplay := make(map[string]bool)
	remainingToolOutputBytes := semanticHandoffToolOutputTotalMax
	for _, msg := range messages {
		switch msg.Role {
		case RoleCanonicalAssistant:
			if isReplaySkippedCanonicalAssistant(msg) {
				continue
			}
			sameReplayTarget := replayIdentityMatches(msg.ProviderState, target)
			recordToolCallReplayTargets(toolCallExactReplay, msg, sameReplayTarget)
			if sameReplayTarget {
				out = append(out, prepareExactAssistantReplay(msg, target))
				continue
			}
			if semantic, ok := semanticAssistantHandoffMessage(msg); ok {
				out = append(out, semantic)
			}
		case RoleCanonicalTool:
			out = append(out, prepareToolReplayMessages(msg, toolCallExactReplay, &remainingToolOutputBytes)...)
		case RoleCanonicalSystem, RoleCanonicalDeveloper, RoleCanonicalUser:
			if semantic, ok := semanticNonAssistantReplayMessage(msg); ok {
				out = append(out, semantic)
			}
		case RoleCanonicalCompaction:
			if replayIdentityMatches(msg.ProviderState, target) {
				out = append(out, prepareExactNonAssistantReplay(msg))
				continue
			}
			if semantic, ok := semanticCompactionHandoffMessage(msg); ok {
				out = append(out, semantic)
			}
		default:
			if semantic, ok := semanticNonAssistantReplayMessage(msg); ok {
				out = append(out, semantic)
			}
		}
	}
	return out
}

func prepareExactAssistantReplay(msg ConversationMessage, target ReplayTarget) ConversationMessage {
	for blockIndex := range msg.Content {
		block := &msg.Content[blockIndex]
		switch block.Type {
		case BlockThinking:
			sanitizeThinkingBlockForTarget(block, target.API, true)
		case BlockToolCall:
			if block.ToolCall != nil {
				block.ToolCall.Raw = nil
			}
		}
	}
	return msg
}

func prepareExactNonAssistantReplay(msg ConversationMessage) ConversationMessage {
	return msg
}

func recordToolCallReplayTargets(targets map[string]bool, msg ConversationMessage, sameReplayTarget bool) {
	if targets == nil {
		return
	}
	for _, block := range msg.Content {
		if block.Type != BlockToolCall || block.ToolCall == nil {
			continue
		}
		callID := strings.TrimSpace(block.ToolCall.ID)
		if callID == "" {
			continue
		}
		targets[callID] = sameReplayTarget
	}
}

func semanticAssistantHandoffMessage(msg ConversationMessage) (ConversationMessage, bool) {
	parts := make([]string, 0, len(msg.Content))
	for _, block := range msg.Content {
		switch block.Type {
		case BlockText, BlockCompaction:
			if text := strings.TrimSpace(block.Text); text != "" {
				parts = append(parts, text)
			}
		case BlockToolCall:
			if text := semanticToolCallText(block.ToolCall); text != "" {
				parts = append(parts, text)
			}
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if text == "" {
		return ConversationMessage{}, false
	}
	return ConversationMessage{
		Role:      RoleCanonicalAssistant,
		Timestamp: msg.Timestamp,
		Content: []ContentBlock{{
			Type: BlockText,
			Text: text,
		}},
	}, true
}

func prepareToolReplayMessages(msg ConversationMessage, toolCallExactReplay map[string]bool, remainingToolOutputBytes *int) []ConversationMessage {
	exactBlocks := make([]ContentBlock, 0, len(msg.Content))
	semanticContent := make([]ContentBlock, 0, len(msg.Content))
	for _, block := range msg.Content {
		if block.Type != BlockToolResult || block.ToolResult == nil {
			continue
		}
		callID := strings.TrimSpace(block.ToolResult.ToolCallID)
		if callID != "" && toolCallExactReplay[callID] {
			exactBlocks = append(exactBlocks, block)
			continue
		}
		if text := semanticToolResultText(block.ToolResult, remainingToolOutputBytes); text != "" {
			semanticContent = append(semanticContent, ContentBlock{Type: BlockText, Text: text})
		}
		semanticContent = append(semanticContent, semanticToolResultImages(block.ToolResult)...)
	}

	out := make([]ConversationMessage, 0, 2)
	if len(exactBlocks) > 0 {
		out = append(out, ConversationMessage{
			Role:      RoleCanonicalTool,
			Timestamp: msg.Timestamp,
			Content:   exactBlocks,
		})
	}
	if len(semanticContent) > 0 {
		out = append(out, ConversationMessage{
			Role:      RoleCanonicalUser,
			Timestamp: msg.Timestamp,
			Content:   semanticContent,
		})
	}
	return out
}

func semanticNonAssistantReplayMessage(msg ConversationMessage) (ConversationMessage, bool) {
	out := ConversationMessage{
		Role:      msg.Role,
		Timestamp: msg.Timestamp,
	}
	for _, block := range msg.Content {
		switch block.Type {
		case BlockText:
			if text := strings.TrimSpace(block.Text); text != "" {
				out.Content = append(out.Content, ContentBlock{Type: BlockText, Text: text})
			}
		case BlockImage:
			if strings.TrimSpace(block.ImageData) != "" {
				out.Content = append(out.Content, ContentBlock{
					Type:      BlockImage,
					ImageData: strings.TrimSpace(block.ImageData),
					MimeType:  strings.TrimSpace(block.MimeType),
				})
			}
		case BlockCompaction:
			if text := strings.TrimSpace(block.Text); text != "" {
				out.Content = append(out.Content, ContentBlock{Type: BlockText, Text: text})
			}
		case BlockToolResult:
			if block.ToolResult != nil {
				if text := strings.TrimSpace(block.ToolResult.OutputText); text != "" {
					out.Content = append(out.Content, ContentBlock{Type: BlockText, Text: text})
				}
			}
		}
	}
	if len(out.Content) == 0 {
		return ConversationMessage{}, false
	}
	return out, true
}

func semanticCompactionHandoffMessage(msg ConversationMessage) (ConversationMessage, bool) {
	parts := make([]string, 0, len(msg.Content))
	for _, block := range msg.Content {
		switch block.Type {
		case BlockText, BlockCompaction:
			if text := strings.TrimSpace(block.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if text == "" {
		return ConversationMessage{}, false
	}
	return ConversationMessage{
		Role:      RoleCanonicalSystem,
		Timestamp: msg.Timestamp,
		Content: []ContentBlock{{
			Type: BlockText,
			Text: "Context checkpoint summary:\n" + text,
		}},
	}, true
}

func semanticToolCallText(call *CanonicalToolCall) string {
	if call == nil {
		return ""
	}
	name := strings.TrimSpace(call.Name)
	callID := strings.TrimSpace(call.ID)
	args := strings.TrimSpace(call.RawArguments)
	if args == "" && len(call.Arguments) > 0 {
		args = strings.TrimSpace(MarshalToolArgumentsJSON(call.Arguments))
	}
	parts := make([]string, 0, 3)
	if name != "" {
		parts = append(parts, "Historical tool call: "+name)
	} else {
		parts = append(parts, "Historical tool call")
	}
	if callID != "" {
		parts = append(parts, "Tool call ID: "+callID)
	}
	if args != "" {
		parts = append(parts, "Arguments:\n"+args)
	}
	return strings.Join(parts, "\n")
}

func semanticToolResultText(result *CanonicalToolResult, remainingToolOutputBytes *int) string {
	if result == nil {
		return ""
	}
	header := "Historical tool result"
	if name := strings.TrimSpace(result.ToolName); name != "" {
		header += ": " + name
	}
	if callID := strings.TrimSpace(result.ToolCallID); callID != "" {
		header += " (tool call ID: " + callID + ")"
	}
	if result.IsError {
		header += " [error]"
	}
	output := strings.TrimSpace(result.OutputText)
	if output == "" {
		if toolResultHasImage(result) {
			output = "Image result attached"
		} else {
			output = "No result provided"
		}
	}
	output = TruncateToolOutputForReplayWithLimits(output, semanticHandoffToolOutputMaxLines, semanticHandoffToolOutputMaxBytes)
	output = consumeSemanticHandoffToolOutputBudget(output, remainingToolOutputBytes)
	return header + ":\n" + output
}

func toolResultHasImage(result *CanonicalToolResult) bool {
	if result == nil {
		return false
	}
	for _, part := range result.OutputContent {
		if part.Type == BlockImage && strings.TrimSpace(part.ImageData) != "" {
			return true
		}
	}
	return false
}

func semanticToolResultImages(result *CanonicalToolResult) []ContentBlock {
	if result == nil || len(result.OutputContent) == 0 {
		return nil
	}
	out := make([]ContentBlock, 0, len(result.OutputContent))
	for _, part := range result.OutputContent {
		if part.Type != BlockImage || strings.TrimSpace(part.ImageData) == "" {
			continue
		}
		out = append(out, ContentBlock{
			Type:      BlockImage,
			ImageData: strings.TrimSpace(part.ImageData),
			MimeType:  strings.TrimSpace(part.MimeType),
		})
	}
	return out
}

func consumeSemanticHandoffToolOutputBudget(output string, remainingBytes *int) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	if remainingBytes == nil {
		return output
	}
	if *remainingBytes <= 0 {
		return semanticHandoffToolOutputOmittedNotice
	}
	used := len([]byte(output))
	if used <= *remainingBytes {
		*remainingBytes -= used
		return output
	}
	output = truncateReplayToolOutputStringFromEnd(output, *remainingBytes)
	*remainingBytes = 0
	output = strings.TrimSpace(output)
	if output == "" {
		return semanticHandoffToolOutputOmittedNotice
	}
	return output + "\n" + semanticHandoffToolOutputBudgetNotice
}

func normalizeReplayTarget(target ReplayTarget) ReplayTarget {
	return ReplayTarget{
		ProviderRef: strings.TrimSpace(target.ProviderRef),
		Provider:    strings.ToLower(strings.TrimSpace(target.Provider)),
		API:         strings.TrimSpace(target.API),
		Model:       strings.TrimSpace(target.Model),
	}
}

func replayIdentityMatches(source *ProviderState, target ReplayTarget) bool {
	if source == nil || target.API == "" || target.Model == "" {
		return false
	}
	sourceAPI := strings.TrimSpace(source.API)
	sourceModel := strings.TrimSpace(source.Model)
	if sourceAPI == "" || sourceModel == "" {
		return false
	}
	if sourceRef := strings.TrimSpace(source.ProviderRef); sourceRef != "" && target.ProviderRef != "" {
		return sourceRef == target.ProviderRef && sourceAPI == target.API && sourceModel == target.Model
	}
	sourceProvider := strings.ToLower(strings.TrimSpace(source.Provider))
	if sourceProvider == "" || target.Provider == "" {
		return false
	}
	return sourceProvider == target.Provider && sourceAPI == target.API && sourceModel == target.Model
}

func sanitizeThinkingBlockForTarget(block *ContentBlock, targetAPI string, sameReplayTarget bool) {
	if block == nil || block.Type != BlockThinking {
		return
	}
	switch targetAPI {
	case "openai-completions":
		if sameReplayTarget {
			block.ThinkingReplayField = firstNonEmptyReasoningReplayField(block.ThinkingReplayField)
			return
		}
		block.ThinkingReplayField = ""
		block.ThinkingSignature = ""
	case "anthropic-messages":
		block.ThinkingReplayField = ""
		if !sameReplayTarget {
			block.ThinkingSignature = ""
		}
	case "openai-responses":
		block.ThinkingReplayField = ""
		// Responses replay uses encrypted_content, not legacy reasoning IDs.
		block.ThinkingSignature = ""
	default:
		block.ThinkingReplayField = ""
		block.ThinkingSignature = ""
	}
}

func firstNonEmptyReasoningReplayField(values ...string) string {
	for _, value := range values {
		switch strings.TrimSpace(value) {
		case "reasoning_content", "reasoning", "reasoning_text":
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneProviderRequestForReplay(req *ProviderRequest) *ProviderRequest {
	if req == nil {
		return &ProviderRequest{}
	}
	out := *req
	out.Context.Messages = cloneConversationMessages(req.Context.Messages)
	out.Context.Tools = cloneToolDefinitions(req.Context.Tools)
	out.Config.Include = append([]string(nil), req.Config.Include...)
	if len(req.Config.ToolChoice) > 0 {
		out.Config.ToolChoice = append([]byte(nil), req.Config.ToolChoice...)
	}
	return &out
}

func cloneConversationMessages(messages []ConversationMessage) []ConversationMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		cloned := msg
		if msg.ProviderState != nil {
			providerState := *msg.ProviderState
			cloned.ProviderState = &providerState
		}
		if msg.Usage != nil {
			usage := *msg.Usage
			cloned.Usage = &usage
		}
		if len(msg.Raw) > 0 {
			cloned.Raw = append([]byte(nil), msg.Raw...)
		}
		cloned.Content = cloneContentBlocks(msg.Content)
		out = append(out, cloned)
	}
	return out
}

func cloneContentBlocks(blocks []ContentBlock) []ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		cloned := block
		if len(block.Raw) > 0 {
			cloned.Raw = append([]byte(nil), block.Raw...)
		}
		if block.ToolCall != nil {
			toolCall := *block.ToolCall
			if len(block.ToolCall.Raw) > 0 {
				toolCall.Raw = append([]byte(nil), block.ToolCall.Raw...)
			}
			if len(block.ToolCall.Arguments) > 0 {
				toolCall.Arguments = make(map[string]any, len(block.ToolCall.Arguments))
				for key, value := range block.ToolCall.Arguments {
					toolCall.Arguments[key] = value
				}
			}
			cloned.ToolCall = &toolCall
		}
		if block.ToolResult != nil {
			toolResult := *block.ToolResult
			toolResult.OutputContent = cloneContentBlocks(block.ToolResult.OutputContent)
			if len(block.ToolResult.Raw) > 0 {
				toolResult.Raw = append([]byte(nil), block.ToolResult.Raw...)
			}
			cloned.ToolResult = &toolResult
		}
		out = append(out, cloned)
	}
	return out
}

func cloneToolDefinitions(defs []ToolDefinition) []ToolDefinition {
	if len(defs) == 0 {
		return nil
	}
	out := make([]ToolDefinition, 0, len(defs))
	for _, def := range defs {
		cloned := def
		if len(def.Raw) > 0 {
			cloned.Raw = append([]byte(nil), def.Raw...)
		}
		out = append(out, cloned)
	}
	return out
}
