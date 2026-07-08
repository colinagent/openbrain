package ai

import (
	"strings"
	"testing"
)

func TestPrepareCanonicalReplayForTarget_PreservesOpenAICompatReplayFieldForSameReplayTarget(t *testing.T) {
	req := &ProviderRequest{
		Context: ConversationContext{
			Messages: []ConversationMessage{{
				Role: RoleCanonicalAssistant,
				ProviderState: &ProviderState{
					ProviderRef: "p1",
					Provider:    "kimi",
					API:         "openai-completions",
					Model:       "kimi-k2.6",
				},
				Content: []ContentBlock{{
					Type:                BlockThinking,
					Text:                "thinking",
					ThinkingReplayField: "reasoning_text",
					ThinkingSignature:   "sig_123",
				}},
			}},
		},
	}

	prepared := PrepareCanonicalReplayForTarget(req, ReplayTarget{
		ProviderRef: "p1",
		Provider:    "kimi",
		API:         "openai-completions",
		Model:       "kimi-k2.6",
	})
	block := prepared.Context.Messages[0].Content[0]
	if got := block.ThinkingReplayField; got != "reasoning_text" {
		t.Fatalf("thinking replay field = %q, want reasoning_text", got)
	}
	if got := block.ThinkingSignature; got != "sig_123" {
		t.Fatalf("thinking signature = %q, want sig_123", got)
	}
}

func TestPrepareCanonicalReplayForTarget_DropsThinkingOnlyAssistantAcrossTargets(t *testing.T) {
	req := &ProviderRequest{
		Context: ConversationContext{
			Messages: []ConversationMessage{{
				Role: RoleCanonicalAssistant,
				ProviderState: &ProviderState{
					ProviderRef: "p1",
					Provider:    "kimi",
					API:         "openai-completions",
					Model:       "kimi-k2.6",
				},
				Content: []ContentBlock{{
					Type:                BlockThinking,
					Text:                "thinking",
					ThinkingReplayField: "reasoning_content",
					ThinkingSignature:   "sig_123",
				}},
			}},
		},
	}

	prepared := PrepareCanonicalReplayForTarget(req, ReplayTarget{
		ProviderRef: "p2",
		Provider:    "openai",
		API:         "openai-completions",
		Model:       "gpt-5.4",
	})
	if got := len(prepared.Context.Messages); got != 0 {
		t.Fatalf("messages = %d, want thinking-only cross-target assistant dropped", got)
	}
	if got := req.Context.Messages[0].Content[0].ThinkingReplayField; got != "reasoning_content" {
		t.Fatalf("original request mutated thinking replay field = %q", got)
	}
}

func TestPrepareCanonicalReplayForTarget_PreservesSameTargetToolProtocol(t *testing.T) {
	req := &ProviderRequest{
		Context: ConversationContext{
			Messages: []ConversationMessage{
				{
					Role: RoleCanonicalAssistant,
					ProviderState: &ProviderState{
						ProviderRef: "deepseek",
						Provider:    "deepseek",
						API:         "openai-completions",
						Model:       "deepseek-v4-pro",
					},
					Content: []ContentBlock{{
						Type: BlockToolCall,
						ToolCall: &CanonicalToolCall{
							ID:           "call_1",
							Name:         "bash",
							RawArguments: `{"command":"pwd"}`,
						},
					}},
				},
				{
					Role: RoleCanonicalTool,
					Content: []ContentBlock{{
						Type: BlockToolResult,
						ToolResult: &CanonicalToolResult{
							ToolCallID: "call_1",
							ToolName:   "bash",
							OutputText: "ok",
						},
					}},
				},
			},
		},
	}

	prepared := PrepareCanonicalReplayForTarget(req, ReplayTarget{
		ProviderRef: "deepseek",
		Provider:    "deepseek",
		API:         "openai-completions",
		Model:       "deepseek-v4-pro",
	})
	if got := len(prepared.Context.Messages); got != 2 {
		t.Fatalf("messages = %d, want same-target assistant/tool pair", got)
	}
	if got := prepared.Context.Messages[0].Content[0].Type; got != BlockToolCall {
		t.Fatalf("assistant block = %q, want tool call", got)
	}
	if got := prepared.Context.Messages[1].Role; got != RoleCanonicalTool {
		t.Fatalf("second role = %q, want tool result", got)
	}
	if got := prepared.Context.Messages[1].Content[0].Type; got != BlockToolResult {
		t.Fatalf("tool block = %q, want tool result", got)
	}
}

func TestPrepareCanonicalReplayForTarget_ConvertsCrossTargetToolProtocolToSemanticText(t *testing.T) {
	req := &ProviderRequest{
		Context: ConversationContext{
			Messages: []ConversationMessage{
				{
					Role: RoleCanonicalUser,
					Content: []ContentBlock{{
						Type: BlockText,
						Text: "inspect repo",
					}},
				},
				{
					Role: RoleCanonicalAssistant,
					ProviderState: &ProviderState{
						ProviderRef: "kimi",
						Provider:    "kimi",
						API:         "openai-completions",
						Model:       "kimi-k2.6",
					},
					Content: []ContentBlock{
						{
							Type:                BlockThinking,
							Text:                "private reasoning",
							ThinkingReplayField: "reasoning_content",
						},
						{
							Type: BlockToolCall,
							ToolCall: &CanonicalToolCall{
								ID:           "call_1",
								Name:         "bash",
								RawArguments: `{"command":"pwd"}`,
							},
						},
					},
				},
				{
					Role: RoleCanonicalTool,
					Content: []ContentBlock{{
						Type: BlockToolResult,
						ToolResult: &CanonicalToolResult{
							ToolCallID: "call_1",
							ToolName:   "bash",
							OutputText: "/Users/example/code/OpAgent",
						},
					}},
				},
				{
					Role: RoleCanonicalAssistant,
					ProviderState: &ProviderState{
						ProviderRef: "kimi",
						Provider:    "kimi",
						API:         "openai-completions",
						Model:       "kimi-k2.6",
					},
					Content: []ContentBlock{{
						Type: BlockText,
						Text: "Repo inspected.",
					}},
				},
			},
		},
	}

	prepared := PrepareCanonicalReplayForTarget(req, ReplayTarget{
		ProviderRef: "deepseek",
		Provider:    "deepseek",
		API:         "openai-completions",
		Model:       "deepseek-v4-pro",
	})
	if got := len(prepared.Context.Messages); got != 4 {
		t.Fatalf("messages = %d, want 4 semantic messages", got)
	}
	if got := prepared.Context.Messages[1].Role; got != RoleCanonicalAssistant {
		t.Fatalf("second role = %q, want assistant", got)
	}
	assistantText := prepared.Context.Messages[1].Content[0].Text
	if strings.Contains(assistantText, "private reasoning") {
		t.Fatalf("semantic assistant leaked thinking: %q", assistantText)
	}
	if !strings.Contains(assistantText, "Historical tool call: bash") || !strings.Contains(assistantText, `{"command":"pwd"}`) {
		t.Fatalf("semantic assistant did not include tool call summary: %q", assistantText)
	}
	if got := prepared.Context.Messages[2].Role; got != RoleCanonicalUser {
		t.Fatalf("third role = %q, want user semantic tool result", got)
	}
	toolResultText := prepared.Context.Messages[2].Content[0].Text
	if !strings.Contains(toolResultText, "Historical tool result: bash") || !strings.Contains(toolResultText, "/Users/example/code/OpAgent") {
		t.Fatalf("semantic tool result text = %q", toolResultText)
	}
	for _, msg := range prepared.Context.Messages {
		if msg.ProviderState != nil {
			t.Fatalf("semantic handoff retained provider state: %#v", msg.ProviderState)
		}
		for _, block := range msg.Content {
			if block.Type == BlockThinking || block.Type == BlockToolCall || block.Type == BlockToolResult {
				t.Fatalf("semantic handoff retained provider protocol block: %#v", block)
			}
		}
	}
}

func TestPrepareCanonicalReplayForTarget_PreservesToolResultImageWholeDuringSemanticHandoff(t *testing.T) {
	var builder strings.Builder
	for i := 1; i <= 1200; i++ {
		if i > 1 {
			builder.WriteByte('\n')
		}
		builder.WriteString("line-")
		builder.WriteString(strings.Repeat("x", 40))
	}
	builder.WriteString("\nimage-tail-sentinel")
	imageData := "data:image/png;base64," + strings.Repeat("A", 9000)

	req := &ProviderRequest{
		Context: ConversationContext{
			Messages: []ConversationMessage{
				{
					Role: RoleCanonicalAssistant,
					ProviderState: &ProviderState{
						Provider: "anthropic",
						API:      "anthropic-messages",
						Model:    "claude-sonnet-4.5",
					},
					Content: []ContentBlock{{
						Type: BlockToolCall,
						ToolCall: &CanonicalToolCall{
							ID:           "call_img",
							Name:         "read",
							RawArguments: `{"path":"shot.png"}`,
						},
					}},
				},
				{
					Role: RoleCanonicalTool,
					Content: []ContentBlock{{
						Type: BlockToolResult,
						ToolResult: &CanonicalToolResult{
							ToolCallID: "call_img",
							ToolName:   "read",
							OutputText: builder.String(),
							OutputContent: []ContentBlock{
								{Type: BlockText, Text: builder.String()},
								{Type: BlockImage, ImageData: imageData, MimeType: "image/png"},
							},
						},
					}},
				},
			},
		},
	}

	prepared := PrepareCanonicalReplayForTarget(req, ReplayTarget{
		Provider: "openai",
		API:      "openai-responses",
		Model:    "gpt-5.4",
	})
	if len(prepared.Context.Messages) != 2 {
		t.Fatalf("messages = %d, want assistant semantic call + user semantic result", len(prepared.Context.Messages))
	}
	resultMsg := prepared.Context.Messages[1]
	if resultMsg.Role != RoleCanonicalUser {
		t.Fatalf("result role = %q, want user", resultMsg.Role)
	}
	if len(resultMsg.Content) != 2 {
		t.Fatalf("result content len = %d, want text + image: %#v", len(resultMsg.Content), resultMsg.Content)
	}
	text := resultMsg.Content[0].Text
	if !strings.Contains(text, "image-tail-sentinel") {
		t.Fatalf("expected semantic text to keep tail sentinel, got %q", text)
	}
	if !strings.Contains(text, "Historical tool output truncated for replay") {
		t.Fatalf("expected semantic text truncation notice, got %q", text)
	}
	if resultMsg.Content[1].Type != BlockImage || resultMsg.Content[1].ImageData != imageData || resultMsg.Content[1].MimeType != "image/png" {
		t.Fatalf("image content = %#v, want exact image data preserved", resultMsg.Content[1])
	}
}

func TestPrepareCanonicalReplayForTarget_LabelsImageOnlyToolResultAsAttached(t *testing.T) {
	imageData := "data:image/png;base64,AAA"
	req := &ProviderRequest{
		Context: ConversationContext{
			Messages: []ConversationMessage{{
				Role: RoleCanonicalTool,
				Content: []ContentBlock{{
					Type: BlockToolResult,
					ToolResult: &CanonicalToolResult{
						ToolName: "read",
						OutputContent: []ContentBlock{{
							Type:      BlockImage,
							ImageData: imageData,
							MimeType:  "image/png",
						}},
					},
				}},
			}},
		},
	}

	prepared := PrepareCanonicalReplayForTarget(req, ReplayTarget{API: "openai-responses", Model: "gpt-5.4"})
	if len(prepared.Context.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(prepared.Context.Messages))
	}
	content := prepared.Context.Messages[0].Content
	if len(content) != 2 {
		t.Fatalf("content len = %d, want text + image: %#v", len(content), content)
	}
	if !strings.Contains(content[0].Text, "Image result attached") {
		t.Fatalf("semantic text = %q, want image attachment label", content[0].Text)
	}
	if content[1].ImageData != imageData {
		t.Fatalf("image data = %q, want exact original", content[1].ImageData)
	}
}
