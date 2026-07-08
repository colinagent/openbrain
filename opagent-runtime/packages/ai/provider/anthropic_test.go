package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

func TestFinalizeStreamConversationMessage_DropsIncompleteToolCalls(t *testing.T) {
	message := ai.FinalizeStreamConversationMessage(&ai.StreamConversationMessage{
		Role: ai.RoleCanonicalAssistant,
		Content: []ai.StreamContentBlock{
			{
				Type: ai.BlockText,
				Text: "partial answer",
			},
			{
				Type: ai.BlockToolCall,
				ToolCall: &ai.StreamToolCall{
					ID:           "call-1",
					Name:         "write",
					RawArguments: `{"path":"/tmp/out.md"`,
					Complete:     false,
				},
			},
			{
				Type: ai.BlockToolCall,
				ToolCall: &ai.StreamToolCall{
					ID:           "call-2",
					Name:         "read",
					RawArguments: `{"path":"/tmp/in.md"}`,
					Complete:     true,
				},
			},
		},
	})

	if len(message.Content) != 2 {
		t.Fatalf("content blocks = %d, want 2", len(message.Content))
	}
	if message.Content[0].Type != ai.BlockText {
		t.Fatalf("first block type = %q, want text", message.Content[0].Type)
	}
	if message.Content[1].Type != ai.BlockToolCall || message.Content[1].ToolCall == nil || message.Content[1].ToolCall.ID != "call-2" {
		t.Fatalf("unexpected remaining tool call: %#v", message.Content[1])
	}
}

func TestAnthropicThinkingBudget(t *testing.T) {
	tests := []struct {
		name   string
		effort string
		want   int64
	}{
		{name: "empty", effort: "", want: 0},
		{name: "low", effort: "low", want: 1024},
		{name: "medium", effort: "medium", want: 2048},
		{name: "high", effort: "high", want: 8192},
		{name: "max", effort: "max", want: 16384},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := anthropicThinkingBudget(tt.effort); got != tt.want {
				t.Fatalf("anthropicThinkingBudget(%q) = %d, want %d", tt.effort, got, tt.want)
			}
		})
	}
}

func TestAnthropicToolChoice(t *testing.T) {
	tests := []struct {
		name       string
		toolChoice string
		wantType   string
		wantOK     bool
	}{
		{name: "empty", toolChoice: "", wantType: "", wantOK: false},
		{name: "auto", toolChoice: "auto", wantType: "auto", wantOK: true},
		{name: "required", toolChoice: "required", wantType: "any", wantOK: true},
		{name: "none", toolChoice: "none", wantType: "none", wantOK: true},
		{name: "unknown", toolChoice: "bash", wantType: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := anthropicToolChoice(tt.toolChoice)
			if ok != tt.wantOK {
				t.Fatalf("anthropicToolChoice(%q) ok = %v, want %v", tt.toolChoice, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("json.Marshal tool choice: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("json.Unmarshal tool choice: %v", err)
			}
			if gotType, _ := decoded["type"].(string); gotType != tt.wantType {
				t.Fatalf("anthropicToolChoice(%q) type = %q, want %q", tt.toolChoice, gotType, tt.wantType)
			}
		})
	}
}

func TestAnthropicBuildRequestUsesAdaptiveThinkingForOpus46(t *testing.T) {
	provider := &AnthropicProvider{
		cfg: &op.ModelConfig{
			Name: "claude-opus-4-6",
		},
	}
	temperature := 0.3

	rawToolChoice, _ := json.Marshal("required")
	req, err := provider.buildRequest(&ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: ai.CanonicalMessagesFromOp([]op.Message{{
				Role:    op.RoleUser,
				Content: "list tmp",
			}}),
			Tools: ai.CanonicalToolsFromOp([]op.ToolSpec{{
				Name:        "bash",
				Description: "run shell command",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string"},
					},
					"required": []any{"command"},
				},
			}}),
		},
		Config: ai.GenerationConfig{
			Model:           "claude-opus-4-6",
			ReasoningEffort: "xhigh",
			Temperature:     &temperature,
			ToolChoice:      rawToolChoice,
		},
	})
	if err != nil {
		t.Fatalf("buildRequest error = %v", err)
	}

	if got := req.Thinking.GetBudgetTokens(); got != nil {
		t.Fatalf("thinking budget = %v, want nil for adaptive thinking", got)
	}
	if got := string(req.OutputConfig.Effort); got != "max" {
		t.Fatalf("output_config.effort = %q, want max", got)
	}
	rawReq, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal request: %v", err)
	}
	var decodedReq map[string]any
	if err := json.Unmarshal(rawReq, &decodedReq); err != nil {
		t.Fatalf("json.Unmarshal request: %v", err)
	}
	thinking, ok := decodedReq["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking = %#v, want object", decodedReq["thinking"])
	}
	if got, _ := thinking["type"].(string); got != "adaptive" {
		t.Fatalf("thinking.type = %q, want adaptive", got)
	}
	if _, ok := decodedReq["temperature"]; ok {
		t.Fatalf("temperature should be omitted when thinking is enabled: %#v", decodedReq["temperature"])
	}
	raw, err := json.Marshal(req.ToolChoice)
	if err != nil {
		t.Fatalf("json.Marshal tool choice: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal tool choice: %v", err)
	}
	if got, _ := decoded["type"].(string); got != "any" {
		t.Fatalf("tool choice type = %q, want any", got)
	}
}

func TestAnthropicBuildRequestKeepsBudgetThinkingForLegacyModels(t *testing.T) {
	provider := &AnthropicProvider{
		cfg: &op.ModelConfig{
			Name: "claude-3-7-sonnet-latest",
		},
	}

	req, err := provider.buildRequest(&ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: ai.CanonicalMessagesFromOp([]op.Message{{
				Role:    op.RoleUser,
				Content: "list tmp",
			}}),
		},
		Config: ai.GenerationConfig{
			Model:           "claude-3-7-sonnet-latest",
			ReasoningEffort: "high",
		},
	})
	if err != nil {
		t.Fatalf("buildRequest error = %v", err)
	}

	if got := req.Thinking.GetBudgetTokens(); got == nil || *got != 8192 {
		t.Fatalf("thinking budget = %v, want 8192", got)
	}
	if got := string(req.OutputConfig.Effort); got != "" {
		t.Fatalf("output_config.effort = %q, want empty", got)
	}
	rawReq, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal request: %v", err)
	}
	var decodedReq map[string]any
	if err := json.Unmarshal(rawReq, &decodedReq); err != nil {
		t.Fatalf("json.Unmarshal request: %v", err)
	}
	thinking, ok := decodedReq["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking = %#v, want object", decodedReq["thinking"])
	}
	if got, _ := thinking["type"].(string); got != "enabled" {
		t.Fatalf("thinking.type = %q, want enabled", got)
	}
}

func TestAnthropicAdaptiveEffort(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
		effort    string
		want      string
		wantOK    bool
	}{
		{name: "empty", modelName: "claude-opus-4-6", effort: "", want: "", wantOK: false},
		{name: "low", modelName: "claude-opus-4-6", effort: "low", want: "low", wantOK: true},
		{name: "xhigh opus", modelName: "claude-opus-4-6", effort: "xhigh", want: "max", wantOK: true},
		{name: "xhigh sonnet", modelName: "claude-sonnet-4-6", effort: "xhigh", want: "high", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := anthropicAdaptiveEffort(tt.effort, tt.modelName)
			if ok != tt.wantOK {
				t.Fatalf("anthropicAdaptiveEffort(%q, %q) ok = %v, want %v", tt.effort, tt.modelName, ok, tt.wantOK)
			}
			if string(got) != tt.want {
				t.Fatalf("anthropicAdaptiveEffort(%q, %q) = %q, want %q", tt.effort, tt.modelName, got, tt.want)
			}
		})
	}
}

func TestAnthropicAssistantBlocks_ReplaysThinkingWithSignature(t *testing.T) {
	blocks, err := anthropicAssistantBlocks(op.Message{
		Role:               op.RoleAssistant,
		ReasoningContent:   "internal reasoning",
		ReasoningSignature: "sig_123",
		Content:            "final answer",
		ToolCalls: []op.MessageToolCall{{
			ID:        "call_1",
			Name:      "bash",
			Arguments: map[string]any{"command": "pwd"},
		}},
	})
	if err != nil {
		t.Fatalf("anthropicAssistantBlocks() error = %v", err)
	}
	if len(blocks) != 3 {
		t.Fatalf("len(blocks) = %d, want 3", len(blocks))
	}
	if blocks[0].OfThinking == nil {
		t.Fatal("blocks[0] = nil, want thinking block")
	}
	if blocks[0].OfThinking.Signature != "sig_123" {
		t.Fatalf("thinking signature = %q, want sig_123", blocks[0].OfThinking.Signature)
	}
	if blocks[0].OfThinking.Thinking != "internal reasoning" {
		t.Fatalf("thinking text = %q, want internal reasoning", blocks[0].OfThinking.Thinking)
	}
	if blocks[1].OfText == nil {
		t.Fatal("blocks[1] = nil, want text block")
	}
	if blocks[1].OfText.Text != "final answer" {
		t.Fatalf("text block = %q, want final answer", blocks[1].OfText.Text)
	}
}

func TestAnthropicAssistantBlocks_DowngradesUnsignedThinkingToPlainText(t *testing.T) {
	blocks, err := anthropicAssistantBlocks(op.Message{
		Role:             op.RoleAssistant,
		ReasoningContent: "plain reasoning without signature",
	})
	if err != nil {
		t.Fatalf("anthropicAssistantBlocks() error = %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].OfText == nil {
		t.Fatal("blocks[0] = nil, want anthropic text block")
	}
	if blocks[0].OfText.Text != "plain reasoning without signature" {
		t.Fatalf("text = %q, want downgraded reasoning text", blocks[0].OfText.Text)
	}
	if blocks[0].OfText.Text == "<thinking>plain reasoning without signature</thinking>" {
		t.Fatal("unsigned thinking should not be wrapped in <thinking> tags")
	}
}

func TestAnthropicToolCallInputJSONDeltaPreservesWhitespaceAcrossFragments(t *testing.T) {
	rawArguments := "{}"
	fragments := []string{
		`{"command":"rg "`,
		`\"Code, think\" /tmp`,
		`/project -n"}`,
	}
	for _, fragment := range fragments {
		switch {
		case rawArguments == "{}" && strings.HasPrefix(strings.TrimLeft(fragment, " \t\r\n"), "{"):
			rawArguments = fragment
		default:
			rawArguments += fragment
		}
	}
	if rawArguments != `{"command":"rg "\"Code, think\" /tmp/project -n"}` {
		t.Fatalf("raw arguments = %q, want whitespace-preserving concatenation", rawArguments)
	}
	if !strings.Contains(rawArguments, `Code, think\" /tmp/project -n`) {
		t.Fatalf("raw arguments = %q, want preserved fragment boundary spaces", rawArguments)
	}
}

func TestConvertToolSpecsToAnthropic_NormalizesStructuredSchema(t *testing.T) {
	type toolSchema struct {
		Type                 string         `json:"type"`
		Properties           map[string]any `json:"properties"`
		Required             []string       `json:"required"`
		AdditionalProperties bool           `json:"additionalProperties"`
	}

	tools := convertToolSpecsToAnthropic([]op.ToolSpec{{
		Name:        "bash",
		Description: "run shell command",
		InputSchema: toolSchema{
			Type: "object",
			Properties: map[string]any{
				"command": map[string]any{"type": "string"},
			},
			Required:             []string{"command"},
			AdditionalProperties: false,
		},
	}})
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	raw, err := json.Marshal(tools[0])
	if err != nil {
		t.Fatalf("json.Marshal tool: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal tool: %v", err)
	}
	inputSchema, ok := decoded["input_schema"].(map[string]any)
	if !ok {
		t.Fatalf("input_schema = %#v, want object", decoded["input_schema"])
	}
	if inputSchema["type"] != "object" {
		t.Fatalf("input_schema.type = %#v, want object", inputSchema["type"])
	}
	required := normalizeProviderSchemaRequired(inputSchema["required"])
	if len(required) != 1 || required[0] != "command" {
		t.Fatalf("input_schema.required = %#v, want [command]", inputSchema["required"])
	}
	if inputSchema["additionalProperties"] != false {
		t.Fatalf("input_schema.additionalProperties = %#v, want false", inputSchema["additionalProperties"])
	}
}
