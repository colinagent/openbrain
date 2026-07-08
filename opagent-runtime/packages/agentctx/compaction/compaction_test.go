package compaction

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func boolPtr(b bool) *bool { return &b }

// --- ResolveSettings ---

func TestResolveSettings_Defaults(t *testing.T) {
	enabled, reserve, keepRecent := ResolveSettings(op.CompactionConfig{})
	if !enabled {
		t.Error("expected enabled=true by default")
	}
	if reserve != DefaultReserveTokens {
		t.Errorf("reserve = %d, want %d", reserve, DefaultReserveTokens)
	}
	if keepRecent != DefaultKeepRecentTokens {
		t.Errorf("keepRecent = %d, want %d", keepRecent, DefaultKeepRecentTokens)
	}
}

func TestResolveSettings_ExplicitDisabled(t *testing.T) {
	enabled, _, _ := ResolveSettings(op.CompactionConfig{Enabled: boolPtr(false)})
	if enabled {
		t.Error("expected enabled=false when explicitly disabled")
	}
}

func TestResolveSettings_CustomValues(t *testing.T) {
	enabled, reserve, keepRecent := ResolveSettings(op.CompactionConfig{
		Enabled:          boolPtr(true),
		ReserveTokens:    8000,
		KeepRecentTokens: 10000,
	})
	if !enabled {
		t.Error("expected enabled=true")
	}
	if reserve != 8000 {
		t.Errorf("reserve = %d, want 8000", reserve)
	}
	if keepRecent != 10000 {
		t.Errorf("keepRecent = %d, want 10000", keepRecent)
	}
}

// --- ShouldCompact ---

func TestShouldCompact(t *testing.T) {
	tests := []struct {
		name          string
		estimated     int64
		contextWindow int64
		reserveTokens int64
		want          bool
	}{
		{"zero estimated", 0, 100000, 16384, false},
		{"zero context window", 50000, 0, 16384, false},
		{"within budget", 50000, 100000, 16384, false},
		{"exactly at budget", 83616, 100000, 16384, false},
		{"exceeds budget", 90000, 100000, 16384, true},
		{"negative reserve treated as 0", 100001, 100000, -1, true},
		{"negative estimated", -1, 100000, 16384, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldCompact(tc.estimated, tc.contextWindow, tc.reserveTokens)
			if got != tc.want {
				t.Errorf("ShouldCompact(%d, %d, %d) = %v, want %v",
					tc.estimated, tc.contextWindow, tc.reserveTokens, got, tc.want)
			}
		})
	}
}

// --- FindCutPoint ---

func makeMsg(role op.MessageRole, content string) op.Message {
	return op.Message{Role: role, Content: content}
}

func TestFindCutPoint_EmptyOrTooSmall(t *testing.T) {
	if idx := FindCutPoint(nil, 100); idx != -1 {
		t.Errorf("nil msgs: got %d, want -1", idx)
	}
	if idx := FindCutPoint([]op.Message{makeMsg(op.RoleUser, "hi")}, 100); idx != -1 {
		t.Errorf("single msg: got %d, want -1", idx)
	}
	if idx := FindCutPoint([]op.Message{makeMsg(op.RoleUser, "hi"), makeMsg(op.RoleAssistant, "ok")}, 0); idx != -1 {
		t.Errorf("zero keepRecent: got %d, want -1", idx)
	}
}

func TestFindCutPoint_PrefersUserBoundary(t *testing.T) {
	msgs := []op.Message{
		makeMsg(op.RoleUser, "first question"),
		makeMsg(op.RoleAssistant, "first answer"),
		makeMsg(op.RoleUser, "second question"),
		makeMsg(op.RoleAssistant, "second answer"),
		makeMsg(op.RoleUser, "third question"),
		makeMsg(op.RoleAssistant, strings.Repeat("x", 2000)),
	}
	// ~500 tokens in last msg + ~16 in remaining = ~516 total.
	// keepRecentTokens=510 causes backward scan to stop at "second question" (idx 2).
	idx := FindCutPoint(msgs, 510)
	if idx < 1 {
		t.Fatalf("expected valid cut point, got %d", idx)
	}
	if msgs[idx].Role != op.RoleUser {
		t.Errorf("expected user boundary, got role=%s at idx=%d", msgs[idx].Role, idx)
	}
}

func TestFindCutPoint_NeverCutsAtToolResult(t *testing.T) {
	msgs := []op.Message{
		makeMsg(op.RoleUser, "do something"),
		{Role: op.RoleAssistant, ToolCalls: []op.MessageToolCall{{ID: "c1", Name: "read_file", Arguments: map[string]any{}}}},
		makeMsg(op.RoleTool, strings.Repeat("result data ", 500)),
		makeMsg(op.RoleUser, "thanks"),
		makeMsg(op.RoleAssistant, strings.Repeat("y", 2000)),
	}
	idx := FindCutPoint(msgs, 600)
	if idx >= 0 && msgs[idx].Role == op.RoleTool {
		t.Errorf("cut should never be at tool result, got idx=%d role=%s", idx, msgs[idx].Role)
	}
}

// --- IsOverflowError ---

func TestIsOverflowError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("something else"), false},
		{fmt.Errorf("context length exceeded"), true},
		{fmt.Errorf("request too long: maximum context window"), true},
		{fmt.Errorf("Too many tokens in input"), true},
		{fmt.Errorf("prompt is too long"), true},
	}
	for _, tc := range tests {
		got := IsOverflowError(tc.err)
		if got != tc.want {
			t.Errorf("IsOverflowError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// --- EstimateContextTokens (hybrid) ---

func TestEstimateContextTokens_NoUsage(t *testing.T) {
	msgs := []op.Message{
		makeMsg(op.RoleUser, "hello"),
		makeMsg(op.RoleAssistant, "world"),
	}
	est := EstimateContextTokens(msgs)
	if est.LastUsageIndex != -1 {
		t.Errorf("expected LastUsageIndex=-1 without usage, got %d", est.LastUsageIndex)
	}
	if est.UsageTokens != 0 {
		t.Errorf("expected UsageTokens=0, got %d", est.UsageTokens)
	}
	// Should match pure estimate.
	pure := op.EstimateMessagesTokens(msgs)
	if est.Tokens != pure {
		t.Errorf("Tokens=%d, want pure estimate %d", est.Tokens, pure)
	}
}

func TestEstimateContextTokens_WithUsage(t *testing.T) {
	msgs := []op.Message{
		makeMsg(op.RoleUser, "hello"),
		{
			Role:    op.RoleAssistant,
			Content: "world",
			Usage:   &op.MessageUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
		},
		makeMsg(op.RoleUser, "follow up question here"),
	}
	est := EstimateContextTokens(msgs)
	if est.LastUsageIndex != 1 {
		t.Errorf("expected LastUsageIndex=1, got %d", est.LastUsageIndex)
	}
	if est.UsageTokens != 15 {
		t.Errorf("expected UsageTokens=15, got %d", est.UsageTokens)
	}
	trailingEstimate := op.EstimateMessageTokens(msgs[2])
	if est.TrailingTokens != trailingEstimate {
		t.Errorf("TrailingTokens=%d, want %d", est.TrailingTokens, trailingEstimate)
	}
	if est.Tokens != 15+trailingEstimate {
		t.Errorf("Tokens=%d, want %d", est.Tokens, 15+trailingEstimate)
	}
}

func TestEstimateContextTokens_MultipleUsages(t *testing.T) {
	msgs := []op.Message{
		makeMsg(op.RoleUser, "q1"),
		{Role: op.RoleAssistant, Content: "a1", Usage: &op.MessageUsage{TotalTokens: 100}},
		makeMsg(op.RoleUser, "q2"),
		{Role: op.RoleAssistant, Content: "a2", Usage: &op.MessageUsage{TotalTokens: 500}},
		makeMsg(op.RoleUser, "q3"),
	}
	est := EstimateContextTokens(msgs)
	// Should use the last assistant with usage (index 3).
	if est.LastUsageIndex != 3 {
		t.Errorf("expected LastUsageIndex=3, got %d", est.LastUsageIndex)
	}
	if est.UsageTokens != 500 {
		t.Errorf("expected UsageTokens=500, got %d", est.UsageTokens)
	}
}

func TestEstimateContextTokens_ZeroTotalTokensIgnored(t *testing.T) {
	msgs := []op.Message{
		makeMsg(op.RoleUser, "hello"),
		{Role: op.RoleAssistant, Content: "world", Usage: &op.MessageUsage{TotalTokens: 0}},
	}
	est := EstimateContextTokens(msgs)
	if est.LastUsageIndex != -1 {
		t.Errorf("zero TotalTokens should be ignored, got LastUsageIndex=%d", est.LastUsageIndex)
	}
}

// --- Compact ---

func TestCompact_EmptyMessages(t *testing.T) {
	result, err := Compact(context.Background(), nil, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil msgs, got %d", len(result))
	}
}

func TestCompact_NoCutPoint(t *testing.T) {
	msgs := []op.Message{makeMsg(op.RoleUser, "hi")}
	result, err := Compact(context.Background(), msgs, 100, func(_ context.Context, _ string) (string, error) {
		return "summary", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 msg when no cut point, got %d", len(result))
	}
}

func TestCompact_ProducesSummary(t *testing.T) {
	msgs := []op.Message{
		makeMsg(op.RoleUser, "first"),
		makeMsg(op.RoleAssistant, "first reply"),
		makeMsg(op.RoleUser, "second"),
		makeMsg(op.RoleAssistant, strings.Repeat("long reply ", 500)),
		makeMsg(op.RoleUser, "third"),
		makeMsg(op.RoleAssistant, strings.Repeat("another long reply ", 500)),
	}

	summaryText := "## Goal\nTest compaction"
	result, err := Compact(context.Background(), msgs, 100, func(_ context.Context, conversation string) (string, error) {
		if conversation == "" {
			t.Error("expected non-empty conversation input")
		}
		return summaryText, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result) < 2 {
		t.Fatalf("expected at least 2 msgs (summary + kept), got %d", len(result))
	}
	if result[0].Role != op.RoleSystem {
		t.Errorf("first msg should be system summary, got role=%s", result[0].Role)
	}
	if !strings.Contains(result[0].Content, summaryText) {
		t.Errorf("summary not found in first message content")
	}
	if len(result) >= len(msgs) {
		t.Errorf("compacted result (%d) should be shorter than original (%d)", len(result), len(msgs))
	}
}

func TestCompact_SummarizeFails_UsesFallback(t *testing.T) {
	msgs := []op.Message{
		makeMsg(op.RoleUser, "first"),
		makeMsg(op.RoleAssistant, strings.Repeat("x", 5000)),
		makeMsg(op.RoleUser, "second"),
		makeMsg(op.RoleAssistant, strings.Repeat("y", 5000)),
	}
	result, err := Compact(context.Background(), msgs, 100, func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("llm unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) < 1 {
		t.Fatal("expected at least 1 message")
	}
	if !strings.Contains(result[0].Content, defaultSummaryFallback) {
		t.Error("expected fallback text in summary")
	}
}
