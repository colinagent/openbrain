package ai

import "strings"

func ProviderResponseFromResponsesResult(result *ResponsesResult) *ProviderResponse {
	if result == nil {
		return &ProviderResponse{}
	}
	message := ConversationMessage{
		Role:       RoleCanonicalAssistant,
		Content:    canonicalAssistantContentFromResponseItems(result.Output),
		StopReason: result.StopReason,
		Usage:      MessageUsageFromUsage(result.Usage),
		ProviderState: &ProviderState{
			ResponseID: strings.TrimSpace(result.ID),
		},
	}
	return &ProviderResponse{
		Message:    message,
		Usage:      result.Usage,
		StopReason: result.StopReason,
	}
}
