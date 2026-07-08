package core

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type reasoningEffortCaptureProvider struct {
	efforts []string
	enabled []*bool
}

func (p *reasoningEffortCaptureProvider) Capabilities() ai.ProviderCapabilities {
	return ai.DefaultCapabilitiesForAPI("openai-completions")
}

func (p *reasoningEffortCaptureProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return nil, fmt.Errorf("unexpected CompleteCanonical call")
}

type reasoningIncludeCaptureProvider struct {
	reqs []*ai.ProviderRequest
}

func (p *reasoningIncludeCaptureProvider) Capabilities() ai.ProviderCapabilities {
	return ai.ProviderCapabilities{
		SupportsThinkingBlocks:     true,
		SupportsToolCalls:          true,
		SupportsParallelToolCalls:  true,
		SupportsImages:             true,
		SupportsStatelessReplay:    true,
		SupportsPreviousResponseID: true,
		SupportsCompaction:         true,
	}
}

func (p *reasoningIncludeCaptureProvider) CompleteCanonical(context.Context, *ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return nil, fmt.Errorf("unexpected CompleteCanonical call")
}

func (p *reasoningIncludeCaptureProvider) StreamCanonical(_ context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req == nil {
		return nil, fmt.Errorf("req is nil")
	}
	cloned := *req
	cloned.Context.Messages = append([]ai.ConversationMessage(nil), req.Context.Messages...)
	cloned.Context.Tools = append([]ai.ToolDefinition(nil), req.Context.Tools...)
	cloned.Config.Include = append([]string(nil), req.Config.Include...)
	p.reqs = append(p.reqs, &cloned)

	stream := ai.NewProviderEventStream(1)
	go func() {
		_ = stream.Emit(ai.ProviderEvent{
			Type: ai.EventCanonicalDone,
			Response: &ai.ProviderResponse{
				Message: ai.ConversationMessage{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "ok",
					}},
				},
				StopReason: ai.StopReasonStop,
			},
		})
		stream.Close()
	}()
	return stream, nil
}

func (p *reasoningEffortCaptureProvider) StreamCanonical(_ context.Context, req *ai.ProviderRequest) (*ai.ProviderEventStream, error) {
	if req == nil {
		return nil, fmt.Errorf("req is nil")
	}
	p.efforts = append(p.efforts, strings.TrimSpace(req.Config.ReasoningEffort))
	p.enabled = append(p.enabled, req.Config.ReasoningEnabled)
	stream := ai.NewProviderEventStream(1)
	go func() {
		_ = stream.Emit(ai.ProviderEvent{
			Type: ai.EventCanonicalDone,
			Response: &ai.ProviderResponse{
				Message: ai.ConversationMessage{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "ok",
					}},
				},
				StopReason: ai.StopReasonStop,
			},
		})
		stream.Close()
	}()
	return stream, nil
}

func TestResolveProviderReasoningEffort(t *testing.T) {
	tests := []struct {
		name   string
		level  string
		levels []string
		want   string
	}{
		{
			name:   "xhigh passes through when model advertises xhigh",
			level:  "xhigh",
			levels: []string{"minimal", "low", "medium", "high", "xhigh"},
			want:   "xhigh",
		},
		{
			name:   "max stays max when model advertises max",
			level:  "max",
			levels: []string{"low", "medium", "high", "max"},
			want:   "max",
		},
		{
			name:   "minimal stays minimal when model advertises minimal",
			level:  "minimal",
			levels: []string{"minimal", "low", "medium", "high"},
			want:   "minimal",
		},
		{
			name:   "medium passes through",
			level:  "medium",
			levels: []string{"low", "medium", "high", "xhigh"},
			want:   "medium",
		},
		{
			name:   "high passes through",
			level:  "high",
			levels: []string{"low", "medium", "high", "max"},
			want:   "high",
		},
		{
			name:  "off clears effort",
			level: "off",
			want:  "",
		},
		{
			name:  "missing reasoning levels preserves raw level",
			level: "xhigh",
			want:  "xhigh",
		},
		{
			name:  "toggle reasoning model never returns reasoning effort",
			level: "on",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveProviderReasoningEffort(tc.level, &op.ModelConfig{
				ReasoningControl: func() string {
					if tc.name == "toggle reasoning model never returns reasoning effort" {
						return "toggle"
					}
					return ""
				}(),
				ReasoningLevels: tc.levels,
			})
			if got != tc.want {
				t.Fatalf("resolveProviderReasoningEffort() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveProviderReasoningEnabled(t *testing.T) {
	enabled := resolveProviderReasoningEnabled("on", &op.ModelConfig{ReasoningControl: "toggle"})
	if enabled == nil || !*enabled {
		t.Fatalf("resolveProviderReasoningEnabled(on) = %#v, want true", enabled)
	}

	disabled := resolveProviderReasoningEnabled("off", &op.ModelConfig{ReasoningControl: "toggle"})
	if disabled == nil || *disabled {
		t.Fatalf("resolveProviderReasoningEnabled(off) = %#v, want false", disabled)
	}

	if got := resolveProviderReasoningEnabled("high", &op.ModelConfig{ReasoningControl: "level"}); got != nil {
		t.Fatalf("resolveProviderReasoningEnabled(level model) = %#v, want nil", got)
	}
}

func TestStreamAssistantResponse_MapsThinkingLevelToProviderReasoningEffort(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	tests := []struct {
		name        string
		model       op.ModelConfig
		level       string
		want        string
		wantEnabled *bool
	}{
		{
			name: "gpt5 gateway preserves xhigh",
			model: op.ModelConfig{
				ID:              "gpt-5.4",
				Name:            "gpt-5.4",
				Provider:        "openai",
				API:             "openai-completions",
				APIKey:          "secret",
				BaseURL:         "https://example.com/v1",
				Reasoning:       true,
				ReasoningLevels: []string{"minimal", "low", "medium", "high", "xhigh"},
			},
			level: "xhigh",
			want:  "xhigh",
		},
		{
			name: "gpt5 gateway preserves minimal",
			model: op.ModelConfig{
				ID:              "gpt-5.4",
				Name:            "gpt-5.4",
				Provider:        "openai",
				API:             "openai-completions",
				APIKey:          "secret",
				BaseURL:         "https://example.com/v1",
				Reasoning:       true,
				ReasoningLevels: []string{"minimal", "low", "medium", "high", "xhigh"},
			},
			level: "minimal",
			want:  "minimal",
		},
		{
			name: "claude gateway keeps max",
			model: op.ModelConfig{
				ID:              "claude-opus-4-6",
				Name:            "claude-opus-4-6",
				Provider:        "openai",
				API:             "openai-completions",
				APIKey:          "secret",
				BaseURL:         "https://example.com/v1",
				Reasoning:       true,
				ReasoningLevels: []string{"low", "medium", "high", "max"},
			},
			level: "max",
			want:  "max",
		},
		{
			name: "toggle reasoning model sends boolean enable flag",
			model: op.ModelConfig{
				ID:               "kimi-4.6",
				Name:             "kimi-4.6",
				Provider:         "openai",
				API:              "openai-completions",
				APIKey:           "secret",
				BaseURL:          "https://example.com/v1",
				Reasoning:        true,
				ReasoningControl: "toggle",
			},
			level: "on",
			want:  "",
			wantEnabled: func() *bool {
				value := true
				return &value
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := &reasoningEffortCaptureProvider{}
			loop := &AgentLoop{
				Ctx:              context.Background(),
				Agent:            &Agent{},
				Model:            &ModelClient{config: &tc.model, Canonical: provider},
				canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{op.NewUserMessage("hello")}),
				ThinkingLevel:    tc.level,
				Meta:             op.Meta{},
			}

			msg, err := loop.streamAssistantResponse()
			if err != nil {
				t.Fatalf("streamAssistantResponse(): %v", err)
			}
			if msg.Content != "ok" {
				t.Fatalf("assistant content = %q, want ok", msg.Content)
			}
			if len(provider.efforts) != 1 {
				t.Fatalf("captured efforts = %d, want 1", len(provider.efforts))
			}
			if got := provider.efforts[0]; got != tc.want {
				t.Fatalf("captured effort = %q, want %q", got, tc.want)
			}
			if tc.wantEnabled == nil {
				if provider.enabled[0] != nil {
					t.Fatalf("captured enabled = %#v, want nil", provider.enabled[0])
				}
			} else if provider.enabled[0] == nil || *provider.enabled[0] != *tc.wantEnabled {
				t.Fatalf("captured enabled = %#v, want %#v", provider.enabled[0], tc.wantEnabled)
			}
		})
	}
}

func TestStreamAssistantResponse_OpenAIResponsesIncludesEncryptedReasoning(t *testing.T) {
	drainNotifyChan()
	t.Cleanup(drainNotifyChan)

	provider := &reasoningIncludeCaptureProvider{}
	modelCfg := op.ModelConfig{
		ID:              "gpt-5.4",
		Name:            "gpt-5.4",
		Provider:        "openai",
		API:             "openai-responses",
		APIKey:          "secret",
		BaseURL:         "https://example.com/v1",
		Reasoning:       true,
		ReasoningLevels: []string{"minimal", "low", "medium", "high", "xhigh"},
	}
	loop := &AgentLoop{
		Ctx:              context.Background(),
		Agent:            &Agent{},
		Model:            &ModelClient{config: &modelCfg, Canonical: provider},
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{op.NewUserMessage("hello")}),
		ThinkingLevel:    "max",
		Meta:             op.Meta{},
	}

	msg, err := loop.streamAssistantResponse()
	if err != nil {
		t.Fatalf("streamAssistantResponse(): %v", err)
	}
	if msg.Content != "ok" {
		t.Fatalf("assistant content = %q, want ok", msg.Content)
	}
	if len(provider.reqs) != 1 {
		t.Fatalf("captured requests = %d, want 1", len(provider.reqs))
	}
	req := provider.reqs[0]
	if req.Config.ReasoningSummary != "auto" {
		t.Fatalf("reasoning summary = %q, want auto", req.Config.ReasoningSummary)
	}
	found := false
	for _, include := range req.Config.Include {
		if include == "reasoning.encrypted_content" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected reasoning.encrypted_content in include, got %#v", req.Config.Include)
	}
}
