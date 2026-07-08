package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/openai/openai-go/v3"
)

func TestConvertMessagesToOpenAI_skipsReasoningOnlyAssistant(t *testing.T) {
	msgs := []op.Message{
		op.NewUserMessage("hello"),
		{
			Role:             op.RoleAssistant,
			ReasoningContent: "internal reasoning only",
		},
		op.NewAssistantMessage("final answer"),
	}

	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		t.Fatalf("convertMessagesToOpenAI: %v", err)
	}
	if len(converted) != 2 {
		t.Fatalf("expected 2 converted messages (reasoning-only skipped), got %d", len(converted))
	}
}

func TestConvertMessagesToOpenAI_keepsAssistantWithToolCalls(t *testing.T) {
	msgs := []op.Message{
		op.NewAssistantToolCalls([]op.MessageToolCall{
			{
				ID:        "call_1",
				Name:      "shell",
				Arguments: map[string]any{"command": "pwd"},
			},
		}),
	}

	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		t.Fatalf("convertMessagesToOpenAI: %v", err)
	}
	if len(converted) != 1 {
		t.Fatalf("expected 1 converted message, got %d", len(converted))
	}
	if converted[0].OfAssistant == nil {
		t.Fatalf("expected assistant message to be preserved")
	}
}

func TestProviderResponseFromOpenAICompatPreservesRawToolArguments(t *testing.T) {
	resp := providerResponseFromOpenAICompat(
		"",
		"",
		"",
		"",
		[]openAICompatToolCall{{
			ID:           "call_1",
			Name:         "read",
			Type:         "function",
			RawArguments: `{"path":"docs/thread.md"}`,
			Arguments:    map[string]any{"path": "docs/thread.md"},
		}},
		ai.Usage{},
		ai.StopReasonToolUse,
	)

	if len(resp.Message.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(resp.Message.Content))
	}
	block := resp.Message.Content[0]
	if block.Type != ai.BlockToolCall || block.ToolCall == nil {
		t.Fatalf("content[0] = %#v, want tool call", block)
	}
	if block.ToolCall.RawArguments != `{"path":"docs/thread.md"}` {
		t.Fatalf("raw arguments = %q", block.ToolCall.RawArguments)
	}
	if got, _ := block.ToolCall.Arguments["path"].(string); got != "docs/thread.md" {
		t.Fatalf("arguments[path] = %q", got)
	}
}

func TestProviderResponseFromOpenAICompatPartialFinalizesStreamPartial(t *testing.T) {
	resp := providerResponseFromOpenAICompatPartial(&ai.StreamConversationMessage{
		Role: ai.RoleCanonicalAssistant,
		Content: []ai.StreamContentBlock{
			{
				Type: ai.BlockText,
				Text: "streamed text",
			},
			{
				Type: ai.BlockToolCall,
				ToolCall: &ai.StreamToolCall{
					ID:           "call_incomplete",
					Name:         "read",
					RawArguments: `{"path":"`,
					Complete:     false,
				},
			},
			{
				Type: ai.BlockToolCall,
				ToolCall: &ai.StreamToolCall{
					ID:           "call_complete",
					Name:         "write",
					RawArguments: `{"path":"out.md"}`,
					Arguments:    map[string]any{"path": "out.md"},
					Complete:     true,
				},
			},
		},
	}, ai.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}, ai.StopReasonToolUse)

	if resp.StopReason != ai.StopReasonToolUse {
		t.Fatalf("stop reason = %q, want tool_use", resp.StopReason)
	}
	if resp.Usage.TotalTokens != 3 {
		t.Fatalf("usage total = %d, want 3", resp.Usage.TotalTokens)
	}
	if len(resp.Message.Content) != 2 {
		t.Fatalf("content blocks = %d, want text + complete tool call", len(resp.Message.Content))
	}
	if resp.Message.Content[0].Type != ai.BlockText || resp.Message.Content[0].Text != "streamed text" {
		t.Fatalf("content[0] = %#v, want streamed text", resp.Message.Content[0])
	}
	if resp.Message.Content[1].Type != ai.BlockToolCall || resp.Message.Content[1].ToolCall == nil || resp.Message.Content[1].ToolCall.ID != "call_complete" {
		t.Fatalf("content[1] = %#v, want complete tool call", resp.Message.Content[1])
	}
}

func TestConvertMessagesToOpenAI_keepsUserImageParts(t *testing.T) {
	msgs := []op.Message{{
		Role: op.RoleUser,
		ContentParts: []op.ContentPart{
			{Type: "text", Text: "describe this"},
			{Type: "image_url", ImageURL: &op.ImageURL{URL: "data:image/png;base64,AAA", Detail: "auto"}},
		},
	}}

	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		t.Fatalf("convertMessagesToOpenAI: %v", err)
	}
	if len(converted) != 1 {
		t.Fatalf("expected 1 converted message, got %d", len(converted))
	}
	raw, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"type":"image_url"`) {
		t.Fatalf("expected image_url part, got %s", text)
	}
	if !strings.Contains(text, "data:image/png;base64,AAA") {
		t.Fatalf("expected data url in request, got %s", text)
	}
}

func TestConvertMessagesToOpenAI_keepsToolResultImagesAsUserParts(t *testing.T) {
	msgs := []op.Message{{
		Role:       op.RoleTool,
		Name:       "read",
		ToolCallID: "call_1",
		Content:    "Read image file [image/png]",
		ContentParts: []op.ContentPart{
			{Type: "text", Text: "Read image file [image/png]"},
			{Type: "image_url", ImageURL: &op.ImageURL{URL: "data:image/png;base64,AAA", Detail: "auto"}},
		},
	}}

	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		t.Fatalf("convertMessagesToOpenAI: %v", err)
	}
	if len(converted) != 2 {
		t.Fatalf("converted len = %d, want tool result plus synthetic user image", len(converted))
	}
	raw, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"role":"tool"`) || !strings.Contains(text, `"role":"user"`) || !strings.Contains(text, "data:image/png;base64,AAA") {
		t.Fatalf("expected tool text plus user image payload, got %s", text)
	}
}

func TestConvertMessagesToOpenAI_preservesReasoningSignature(t *testing.T) {
	msgs := []op.Message{
		op.NewUserMessage("hello"),
		{
			Role:                 op.RoleAssistant,
			Content:              "final answer",
			ReasoningContent:     "thinking",
			ReasoningReplayField: "reasoning_content",
			ReasoningSignature:   "sig_123",
		},
	}

	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		t.Fatalf("convertMessagesToOpenAI: %v", err)
	}
	if len(converted) != 2 {
		t.Fatalf("expected 2 converted messages, got %d", len(converted))
	}
	raw, err := json.Marshal(converted[1])
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"reasoning_content":"thinking"`) {
		t.Fatalf("expected reasoning_content in request, got %s", text)
	}
	if !strings.Contains(text, `"reasoning_signature":"sig_123"`) {
		t.Fatalf("expected reasoning_signature in request, got %s", text)
	}
}

func TestConvertMessagesToOpenAI_PreservesReasoningOnAssistantToolCall(t *testing.T) {
	msgs := []op.Message{
		op.NewUserMessage("hello"),
		{
			Role:                 op.RoleAssistant,
			ReasoningContent:     "thinking",
			ReasoningReplayField: "reasoning_content",
			ToolCalls: []op.MessageToolCall{{
				ID:        "call_1",
				Name:      "shell",
				Arguments: map[string]any{"command": "pwd"},
			}},
		},
	}

	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		t.Fatalf("convertMessagesToOpenAI: %v", err)
	}
	if len(converted) != 2 {
		t.Fatalf("expected 2 converted messages, got %d", len(converted))
	}
	raw, err := json.Marshal(converted[1])
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"reasoning_content":"thinking"`) {
		t.Fatalf("expected reasoning_content in assistant tool call request, got %s", text)
	}
	if !strings.Contains(text, `"tool_calls"`) {
		t.Fatalf("expected tool_calls in assistant tool call request, got %s", text)
	}
}

func TestConvertMessagesToOpenAI_UsesRecordedReasoningReplayField(t *testing.T) {
	msgs := []op.Message{
		op.NewUserMessage("hello"),
		{
			Role:                 op.RoleAssistant,
			Content:              "final answer",
			ReasoningContent:     "thinking",
			ReasoningReplayField: "reasoning_text",
		},
	}

	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		t.Fatalf("convertMessagesToOpenAI: %v", err)
	}
	raw, err := json.Marshal(converted[1])
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"reasoning_text":"thinking"`) {
		t.Fatalf("expected reasoning_text in request, got %s", text)
	}
	if strings.Contains(text, `"reasoning_content":"thinking"`) {
		t.Fatalf("did not expect reasoning_content fallback, got %s", text)
	}
}

func TestConvertMessagesToOpenAI_DoesNotGuessReasoningReplayField(t *testing.T) {
	msgs := []op.Message{
		op.NewUserMessage("hello"),
		{
			Role:             op.RoleAssistant,
			Content:          "final answer",
			ReasoningContent: "thinking",
		},
	}

	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		t.Fatalf("convertMessagesToOpenAI: %v", err)
	}
	raw, err := json.Marshal(converted[1])
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, `"reasoning_content":`) || strings.Contains(text, `"reasoning_text":`) || strings.Contains(text, `"reasoning":`) {
		t.Fatalf("did not expect guessed reasoning replay fields, got %s", text)
	}
}

func TestConvertMessagesToOpenAI_CrossTargetKimiThinkingToolHistoryIsSemanticText(t *testing.T) {
	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: []ai.ConversationMessage{
				{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "inspect repo",
					}},
				},
				{
					Role: ai.RoleCanonicalAssistant,
					ProviderState: &ai.ProviderState{
						ProviderRef: "kimi",
						Provider:    "kimi",
						API:         "openai-completions",
						Model:       "kimi-k2.6",
					},
					Content: []ai.ContentBlock{
						{
							Type:                ai.BlockThinking,
							Text:                "private reasoning",
							ThinkingReplayField: "reasoning_content",
						},
						{
							Type: ai.BlockToolCall,
							ToolCall: &ai.CanonicalToolCall{
								ID:           "call_1",
								Name:         "bash",
								RawArguments: `{"command":"pwd"}`,
							},
						},
					},
				},
				{
					Role: ai.RoleCanonicalTool,
					Content: []ai.ContentBlock{{
						Type: ai.BlockToolResult,
						ToolResult: &ai.CanonicalToolResult{
							ToolCallID: "call_1",
							ToolName:   "bash",
							OutputText: "/Users/example/code/OpAgent",
						},
					}},
				},
			},
		},
	}
	prepared := ai.PrepareCanonicalReplayForTarget(req, ai.ReplayTarget{
		ProviderRef: "deepseek",
		Provider:    "deepseek",
		API:         "openai-completions",
		Model:       "deepseek-v4-pro",
	})
	msgs := make([]op.Message, 0, len(prepared.Context.Messages))
	for _, msg := range prepared.Context.Messages {
		converted, err := ai.OpMessageFromCanonical(msg)
		if err != nil {
			t.Fatalf("OpMessageFromCanonical(): %v", err)
		}
		if converted.Role != "" {
			msgs = append(msgs, converted)
		}
	}
	converted, err := convertMessagesToOpenAI(msgs)
	if err != nil {
		t.Fatalf("convertMessagesToOpenAI(): %v", err)
	}
	raw, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	text := string(raw)
	if strings.Contains(text, `"reasoning_content":`) || strings.Contains(text, `"reasoning_text":`) || strings.Contains(text, `"tool_calls"`) || strings.Contains(text, `"role":"tool"`) {
		t.Fatalf("cross-target replay retained provider protocol fields: %s", text)
	}
	if strings.Contains(text, "private reasoning") {
		t.Fatalf("cross-target replay leaked thinking: %s", text)
	}
	if !strings.Contains(text, "Historical tool call: bash") || !strings.Contains(text, "Historical tool result: bash") {
		t.Fatalf("cross-target replay missing semantic tool history: %s", text)
	}
}

func TestApplyOptions_UsesToggleReasoningBooleanInsteadOfReasoningEffort(t *testing.T) {
	req := openAICompatRequestForTest()
	applyOptions(&req, providerRequestOptions{
		ReasoningEnabled: func() *bool {
			value := true
			return &value
		}(),
	}, &op.ModelConfig{
		Reasoning:        true,
		ReasoningControl: "toggle",
	})

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"enable_thinking":true`) {
		t.Fatalf("expected enable_thinking in request, got %s", text)
	}
	if strings.Contains(text, `"reasoning_effort"`) {
		t.Fatalf("did not expect reasoning_effort in toggle request, got %s", text)
	}
}

func openAICompatRequestForTest() openai.ChatCompletionNewParams {
	return openai.ChatCompletionNewParams{
		Model:    "test-model",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")},
	}
}
