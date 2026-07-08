package core

import (
	"context"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

func TestCompactCanonicalMessages_ClearsRetainedAssistantUsageAfterCheckpoint(t *testing.T) {
	messages := []ai.ConversationMessage{
		{
			Role: ai.RoleCanonicalUser,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "old question",
			}},
		},
		{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: strings.Repeat("old-answer-", 40),
			}},
			Usage: &op.MessageUsage{TotalTokens: 1800},
		},
		{
			Role: ai.RoleCanonicalUser,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: strings.Repeat("recent-user-", 24),
			}},
		},
		{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: strings.Repeat("recent-assistant-", 24),
			}},
			Usage: &op.MessageUsage{TotalTokens: 2400},
		},
		{
			Role: ai.RoleCanonicalUser,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: strings.Repeat("latest-user-", 24),
			}},
		},
	}

	before := estimateCanonicalContextTokens(messages, "")
	keepRecentTokens := estimateCanonicalMessageTokens(messages[3]) + estimateCanonicalMessageTokens(messages[4]) + 1
	compacted, err := compactCanonicalMessages(context.Background(), messages, keepRecentTokens, func(context.Context, string) (string, error) {
		return "recent context summary", nil
	})
	if err != nil {
		t.Fatalf("compactCanonicalMessages(): %v", err)
	}
	if len(compacted) < 3 {
		t.Fatalf("len(compacted) = %d, want >= 3", len(compacted))
	}
	if compacted[0].Role != ai.RoleCanonicalSystem {
		t.Fatalf("summary role = %q, want system", compacted[0].Role)
	}
	if got := compacted[0].Content[0].Text; !strings.Contains(got, "Context checkpoint summary:\nrecent context summary") {
		t.Fatalf("summary text = %q, want checkpoint summary", got)
	}

	retainedAssistantUsages := 0
	for _, msg := range compacted[1:] {
		if msg.Role != ai.RoleCanonicalAssistant {
			continue
		}
		retainedAssistantUsages++
		if msg.Usage != nil {
			t.Fatalf("retained assistant usage = %+v, want nil after compaction", msg.Usage)
		}
	}
	if retainedAssistantUsages == 0 {
		t.Fatal("expected compacted history to retain at least one assistant message")
	}

	after := estimateCanonicalContextTokens(compacted, "")
	if after >= before {
		t.Fatalf("tokensAfter = %d, want < tokensBefore = %d", after, before)
	}
}

func TestPruneCanonicalMessagesForSummaryClearsToolResultImages(t *testing.T) {
	imageData := "data:image/png;base64," + strings.Repeat("A", 4096)
	messages := []ai.ConversationMessage{{
		Role: ai.RoleCanonicalTool,
		Content: []ai.ContentBlock{{
			Type: ai.BlockToolResult,
			ToolResult: &ai.CanonicalToolResult{
				ToolName: "read",
				OutputContent: []ai.ContentBlock{{
					Type:      ai.BlockImage,
					ImageData: imageData,
					MimeType:  "image/png",
				}},
			},
		}},
	}}

	pruned := pruneCanonicalMessagesForSummary(messages)
	result := pruned[0].Content[0].ToolResult
	if result == nil {
		t.Fatal("missing tool result")
	}
	if result.OutputText != "[Old tool result content cleared]" {
		t.Fatalf("OutputText = %q, want old content placeholder", result.OutputText)
	}
	if len(result.OutputContent) != 0 {
		t.Fatalf("OutputContent len = %d, want cleared", len(result.OutputContent))
	}
	input := buildCanonicalSummarizationInput(pruned)
	if strings.Contains(input, imageData) {
		t.Fatalf("summary input unexpectedly contains image data")
	}
	if !strings.Contains(input, "[Old tool result content cleared]") {
		t.Fatalf("summary input missing cleared-output marker: %q", input)
	}
}

func TestBuildCanonicalContextUsage_UnknownImmediatelyAfterCompaction(t *testing.T) {
	messages := []ai.ConversationMessage{
		{
			Role: ai.RoleCanonicalSystem,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "Context checkpoint summary:\nolder context",
			}},
		},
		{
			Role: ai.RoleCanonicalUser,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "next question",
			}},
		},
	}

	usage := buildCanonicalContextUsage(messages, "", 1_000_000)
	if usage.Known {
		t.Fatalf("usage.Known = true, want false")
	}
	if usage.Tokens != 0 {
		t.Fatalf("usage.Tokens = %d, want 0 while unknown", usage.Tokens)
	}
	if usage.ContextWindow != 1_000_000 {
		t.Fatalf("usage.ContextWindow = %d, want 1000000", usage.ContextWindow)
	}
}

func TestAgentLoopContextUsageUsesSelectedContextWindow(t *testing.T) {
	loop := &AgentLoop{
		Model:         &ModelClient{config: &op.ModelConfig{ContextWindow: 1_000_000}},
		ContextWindow: 300_000,
		canonicalHistory: []ai.ConversationMessage{{
			Role: ai.RoleCanonicalUser,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "hello",
			}},
		}},
	}

	usage := loop.contextUsageAfter()
	if usage.ContextWindow != 300_000 {
		t.Fatalf("usage.ContextWindow = %d, want 300000", usage.ContextWindow)
	}
}

func TestBuildCanonicalContextUsage_UsesPostCompactionAssistantUsage(t *testing.T) {
	messages := []ai.ConversationMessage{
		{
			Role: ai.RoleCanonicalSystem,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "Context checkpoint summary:\nolder context",
			}},
		},
		{
			Role: ai.RoleCanonicalUser,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "next question",
			}},
		},
		{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "answer",
			}},
			Usage: &op.MessageUsage{TotalTokens: 25_000},
		},
	}

	usage := buildCanonicalContextUsage(messages, "", 1_000_000)
	if !usage.Known {
		t.Fatalf("usage.Known = false, want true")
	}
	if usage.Tokens != 25_000 {
		t.Fatalf("usage.Tokens = %d, want 25000", usage.Tokens)
	}
	if usage.PercentMilli != 2500 {
		t.Fatalf("usage.PercentMilli = %d, want 2500", usage.PercentMilli)
	}
}

func TestBuildCanonicalContextUsage_AddsTrailingEstimatedTokens(t *testing.T) {
	trailing := ai.ConversationMessage{
		Role: ai.RoleCanonicalUser,
		Content: []ai.ContentBlock{{
			Type: ai.BlockText,
			Text: strings.Repeat("tail-", 20),
		}},
	}
	messages := []ai.ConversationMessage{
		{
			Role: ai.RoleCanonicalAssistant,
			Content: []ai.ContentBlock{{
				Type: ai.BlockText,
				Text: "answer",
			}},
			Usage: &op.MessageUsage{TotalTokens: 1_000},
		},
		trailing,
	}

	usage := buildCanonicalContextUsage(messages, "", 10_000)
	want := int64(1_000 + estimateCanonicalMessageTokens(trailing))
	if usage.Tokens != want {
		t.Fatalf("usage.Tokens = %d, want %d", usage.Tokens, want)
	}
}
