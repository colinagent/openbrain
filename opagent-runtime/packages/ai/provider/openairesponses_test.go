package provider

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/openai/openai-go/v3/responses"
)

func TestCanonicalRequestToResponses_DefaultsToNonStoredRequestShape(t *testing.T) {
	provider := &ResponsesProvider{}
	req := provider.normalizeRequestForProvider(canonicalRequestToResponses(&ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: ai.CanonicalMessagesFromOp([]op.Message{op.NewUserMessage("hello")}),
		},
		Config: ai.GenerationConfig{Model: "gpt-5"},
	}))

	raw, err := marshalResponsesRequestBody(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(raw), `"store":true`) {
		t.Fatalf("expected non-stored request shape, got %s", string(raw))
	}
}

func TestConvertMessagesToResponses_DropsPlainReasoningKeepsToolCallsAndImages(t *testing.T) {
	msgs := []op.Message{
		{
			Role: op.RoleUser,
			ContentParts: []op.ContentPart{
				{Type: "text", Text: "describe"},
				{Type: "image_url", ImageURL: &op.ImageURL{URL: "data:image/png;base64,AAA", Detail: "auto"}},
			},
		},
		{
			Role:             op.RoleAssistant,
			Content:          "Done",
			ReasoningContent: "thinking",
			ToolCalls: []op.MessageToolCall{{
				ID:        "call-1",
				Name:      "shell",
				Arguments: map[string]any{"command": "pwd"},
			}},
		},
		op.NewToolResultMessage("shell", "call-1", "ok"),
	}

	items := canonicalRequestToResponses(&ai.ProviderRequest{Context: ai.ConversationContext{Messages: ai.CanonicalMessagesFromOp(msgs)}}).Input
	types := make([]string, 0, len(items)+2)
	for _, item := range items {
		types = append(types, item.Type)
		for _, part := range item.Content {
			types = append(types, part.Type)
		}
	}
	joined := strings.Join(types, ",")
	if !strings.Contains(joined, "input_image") {
		t.Fatalf("expected input_image in %s", joined)
	}
	if strings.Contains(joined, "reasoning") {
		t.Fatalf("expected plain reasoning replay to be dropped in %s", joined)
	}
	if !strings.Contains(joined, "function_call") {
		t.Fatalf("expected function_call in %s", joined)
	}
	if !strings.Contains(joined, "function_call_output") {
		t.Fatalf("expected function_call_output in %s", joined)
	}
}

func TestCanonicalRequestToResponses_KeepsToolResultImages(t *testing.T) {
	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: []ai.ConversationMessage{{
				Role: ai.RoleCanonicalTool,
				Content: []ai.ContentBlock{{
					Type: ai.BlockToolResult,
					ToolResult: &ai.CanonicalToolResult{
						ToolCallID: "call_img",
						ToolName:   "read",
						OutputText: "Read image file [image/png]",
						OutputContent: []ai.ContentBlock{
							{Type: ai.BlockText, Text: "Read image file [image/png]"},
							{Type: ai.BlockImage, ImageData: "data:image/png;base64,AAA", MimeType: "auto"},
						},
					},
				}},
			}},
		},
	}
	converted := canonicalRequestToResponses(req)
	if len(converted.Input) != 1 {
		t.Fatalf("input len = %d, want 1", len(converted.Input))
	}
	item := converted.Input[0]
	if item.Type != "function_call_output" || len(item.OutputContent) != 2 {
		t.Fatalf("item = %+v", item)
	}
	raw, err := ai.MarshalResponsesInputItemsJSON(converted.Input)
	if err != nil {
		t.Fatalf("MarshalResponsesInputItemsJSON(): %v", err)
	}
	text := string(raw[0])
	if !strings.Contains(text, `"type":"input_image"`) || !strings.Contains(text, "data:image/png;base64,AAA") {
		t.Fatalf("tool image output missing from payload: %s", text)
	}
}

func TestProviderResponseFromResponsesResult_PrefersRawReasoning(t *testing.T) {
	raw := `{
	  "id": "resp_1",
	  "model": "gpt-5.4",
	  "object": "response",
	  "output": [
	    {
	      "id": "reason-1",
	      "type": "reasoning",
	      "content": [{"type":"reasoning_text","text":"raw reasoning"}],
	      "encrypted_content": "enc_1",
	      "summary": [{"type":"summary_text","text":"summary reasoning"}],
	      "status": "completed"
	    },
	    {
	      "id": "fc_1",
	      "type": "function_call",
	      "call_id": "call-1",
	      "name": "shell",
	      "arguments": "{\"command\":\"pwd\"}",
	      "status": "completed"
	    }
	  ],
	  "status": "completed",
	  "usage": {
	    "input_tokens": 10,
	    "input_tokens_details": {"cached_tokens": 0},
	    "output_tokens": 5,
	    "output_tokens_details": {"reasoning_tokens": 2},
	    "total_tokens": 15
	  }
	}`
	var resp responses.Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	converted := ai.ProviderResponseFromResponsesResult(convertResponsesToNativeResult(&resp))
	if converted.Usage.InputTokens != 10 || converted.Usage.CacheReadTokens != 0 || converted.Usage.CacheWriteTokens != 0 {
		t.Fatalf("unexpected usage conversion: %#v", converted.Usage)
	}
	msg, err := ai.OpMessageFromCanonical(converted.Message)
	if err != nil {
		t.Fatalf("OpMessageFromCanonical(): %v", err)
	}
	if msg.ReasoningContent != "raw reasoning" {
		t.Fatalf("expected raw reasoning, got %q", msg.ReasoningContent)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call-1" {
		t.Fatalf("expected tool call to be converted, got %#v", msg.ToolCalls)
	}
}

func TestResponsesFailureError_PreservesGatewayStringErrorDetail(t *testing.T) {
	err := ai.ParseResponsesFailureErrorJSON(`{"type":"response.failed","error":"response failed without details","request_id":"req_x"}`)
	retryErr, ok := ai.AsRetryError(err)
	if !ok || retryErr == nil {
		t.Fatalf("expected retry error, got %T %v", err, err)
	}
	if !retryErr.Retryable {
		t.Fatalf("Retryable = false, want true: %#v", retryErr)
	}
	if !strings.Contains(retryErr.Error(), "response failed without details") {
		t.Fatalf("error = %q, want preserved gateway detail", retryErr.Error())
	}
	if !strings.Contains(retryErr.Error(), "req_x") {
		t.Fatalf("error = %q, want request id", retryErr.Error())
	}
}

func TestProviderResponseFromResponsesResult_FallsBackToSummaryReasoning(t *testing.T) {
	raw := `{
	  "id": "resp_2",
	  "model": "gpt-5.4",
	  "object": "response",
	  "output": [
	    {
	      "id": "msg-1",
	      "type": "message",
	      "role": "assistant",
	      "status": "completed",
	      "content": [{"type":"output_text","text":"hello","annotations":[]}]
	    },
	    {
	      "id": "reason-1",
	      "type": "reasoning",
	      "summary": [{"type":"summary_text","text":"summary only"}],
	      "encrypted_content": "enc_2",
	      "status": "completed"
	    }
	  ],
	  "status": "completed",
	  "usage": {
	    "input_tokens": 4,
	    "input_tokens_details": {"cached_tokens": 0},
	    "output_tokens": 2,
	    "output_tokens_details": {"reasoning_tokens": 0},
	    "total_tokens": 6
	  }
	}`
	var resp responses.Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	converted := ai.ProviderResponseFromResponsesResult(convertResponsesToNativeResult(&resp))
	if converted.Usage.InputTokens != 4 || converted.Usage.CacheReadTokens != 0 || converted.Usage.CacheWriteTokens != 0 {
		t.Fatalf("unexpected usage conversion: %#v", converted.Usage)
	}
	msg, err := ai.OpMessageFromCanonical(converted.Message)
	if err != nil {
		t.Fatalf("OpMessageFromCanonical(): %v", err)
	}
	if msg.Content != "hello" {
		t.Fatalf("expected content hello, got %q", msg.Content)
	}
	if msg.ReasoningContent != "summary only" {
		t.Fatalf("expected summary fallback, got %q", msg.ReasoningContent)
	}
}

func TestConvertResponsesToNativeResult_SplitsCachedInputTokens(t *testing.T) {
	raw := `{
	  "id": "resp_cached",
	  "model": "gpt-5.4",
	  "object": "response",
	  "output": [
	    {
	      "id": "msg-1",
	      "type": "message",
	      "role": "assistant",
	      "status": "completed",
	      "content": [{"type":"output_text","text":"hello","annotations":[]}]
	    }
	  ],
	  "status": "completed",
	  "usage": {
	    "input_tokens": 12,
	    "input_tokens_details": {"cached_tokens": 7},
	    "output_tokens": 2,
	    "output_tokens_details": {"reasoning_tokens": 0},
	    "total_tokens": 14
	  }
	}`
	var resp responses.Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	converted := convertResponsesToNativeResult(&resp)
	if converted.Usage.InputTokens != 5 || converted.Usage.CacheReadTokens != 7 || converted.Usage.CacheWriteTokens != 0 || converted.Usage.TotalTokens != 14 {
		t.Fatalf("unexpected cached usage conversion: %#v", converted.Usage)
	}
}

func TestMarshalResponsesRequestBody_ReplaysAssistantMessageContent(t *testing.T) {
	raw, err := marshalResponsesRequestBody(&ai.ResponsesRequest{
		Model: "gpt-5.4",
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"1"}]}`)),
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"assistant","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":"Hi."}],"status":"completed","id":"msg_123"}`)),
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"ls /tmp"}]}`)),
		},
	})
	if err != nil {
		t.Fatalf("marshalResponsesRequestBody: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"role":"assistant"`) {
		t.Fatalf("expected assistant message in %s", text)
	}
	if !strings.Contains(text, `"type":"output_text"`) || !strings.Contains(text, `"text":"Hi."`) {
		t.Fatalf("expected assistant content to be preserved in %s", text)
	}
}

func TestMarshalResponsesRequestBody_PreservesEmptyFunctionCallOutputField(t *testing.T) {
	raw, err := marshalResponsesRequestBody(&ai.ResponsesRequest{
		Model: "gpt-5.4",
		Input: []ai.ResponseItem{{
			Type:       "function_call_output",
			CallID:     "call_123",
			OutputText: "",
			Status:     "completed",
		}},
	})
	if err != nil {
		t.Fatalf("marshalResponsesRequestBody: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"type":"function_call_output"`) {
		t.Fatalf("expected function_call_output in %s", text)
	}
	if !strings.Contains(text, `"output":""`) {
		t.Fatalf("expected empty output field in %s", text)
	}
}

func TestMarshalResponsesRequestBody_SanitizesStaleFunctionCallOutputRaw(t *testing.T) {
	item := ai.ParseResponseItemRaw(json.RawMessage(`{"type":"function_call_output","call_id":"call_123","status":"completed"}`))
	raw, err := marshalResponsesRequestBody(&ai.ResponsesRequest{
		Model: "gpt-5.4",
		Input: []ai.ResponseItem{item},
	})
	if err != nil {
		t.Fatalf("marshalResponsesRequestBody: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"type":"function_call_output"`) {
		t.Fatalf("expected function_call_output in %s", text)
	}
	if !strings.Contains(text, `"output":""`) {
		t.Fatalf("expected sanitized empty output field in %s", text)
	}
}

func TestCanonicalRequestToResponses_NormalizesAssistantBlockReplay(t *testing.T) {
	rawCall := json.RawMessage(`{"type":"function_call","id":"fc_123","call_id":"call_123","name":"bash","arguments":"{\"command\":\"ls /tmp\"}","status":"completed"}`)

	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: []ai.ConversationMessage{
				{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "1",
					}},
				},
				{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{
						{
							Type:          ai.BlockText,
							Text:          "Hi.",
							TextSignature: "msg_123",
						},
						{
							Type: ai.BlockToolCall,
							ToolCall: &ai.CanonicalToolCall{
								ID:           "call_123",
								Name:         "bash",
								RawArguments: `{"command":"ls /tmp"}`,
								Raw:          rawCall,
							},
							Raw: rawCall,
						},
					},
					ProviderState: &ai.ProviderState{
						ResponseID: "resp_123",
					},
				},
				{
					Role: ai.RoleCanonicalTool,
					Content: []ai.ContentBlock{{
						Type: ai.BlockToolResult,
						ToolResult: &ai.CanonicalToolResult{
							ToolCallID: "call_123",
							ToolName:   "bash",
							OutputText: "/tmp\nvar",
						},
					}},
				},
			},
		},
	}

	converted := canonicalRequestToResponses(req)
	if len(converted.Input) != 4 {
		t.Fatalf("len(converted.Input) = %d, want 4", len(converted.Input))
	}
	if converted.Input[1].Type != "" {
		t.Fatalf("assistant replay type = %q, want empty", converted.Input[1].Type)
	}
	if converted.Input[1].Role != "assistant" {
		t.Fatalf("assistant replay role = %q, want assistant", converted.Input[1].Role)
	}
	if converted.Input[1].ID != "msg_123" {
		t.Fatalf("assistant replay id = %q, want msg_123", converted.Input[1].ID)
	}
	if len(converted.Input[1].Content) != 1 || converted.Input[1].Content[0].Type != "output_text" || converted.Input[1].Content[0].Text != "Hi." {
		t.Fatalf("assistant replay content = %#v, want output_text Hi.", converted.Input[1].Content)
	}
	if converted.Input[2].Type != "function_call" {
		t.Fatalf("tool call replay type = %q, want function_call", converted.Input[2].Type)
	}
	if converted.Input[2].ID != "fc_123" {
		t.Fatalf("tool call replay id = %q, want fc_123", converted.Input[2].ID)
	}
	if converted.Input[3].Type != "function_call_output" {
		t.Fatalf("tool result type = %q, want function_call_output", converted.Input[3].Type)
	}
	if converted.Input[3].CallID != "call_123" {
		t.Fatalf("tool result call_id = %q, want call_123", converted.Input[3].CallID)
	}
	if got := string(converted.Input[1].Raw); !strings.Contains(got, `"role":"assistant"`) || !strings.Contains(got, `"text":"Hi."`) || strings.Contains(got, `"status"`) {
		t.Fatalf("assistant replay raw = %s, want normalized assistant input message", got)
	}
}

func TestCanonicalRequestToResponses_DropsHistoricalRawReasoningReplay(t *testing.T) {
	rawReasoning := json.RawMessage(`{"type":"reasoning","id":"rs_legacy","summary":[{"type":"summary_text","text":"summary only"}],"encrypted_content":"enc_legacy"}`)
	rawCall := json.RawMessage(`{"type":"function_call","id":"fc_legacy","call_id":"call_legacy","name":"bash","arguments":"{\"command\":\"pwd\"}","status":"completed"}`)

	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: []ai.ConversationMessage{
				{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{
						{Type: ai.BlockThinking, Text: "summary only", EncryptedContent: "enc_legacy", Raw: rawReasoning},
						{
							Type: ai.BlockToolCall,
							ToolCall: &ai.CanonicalToolCall{
								ID:           "call_legacy",
								Name:         "bash",
								RawArguments: `{"command":"pwd"}`,
								Raw:          rawCall,
							},
							Raw: rawCall,
						},
					},
				},
				{
					Role: ai.RoleCanonicalTool,
					Content: []ai.ContentBlock{{
						Type: ai.BlockToolResult,
						ToolResult: &ai.CanonicalToolResult{
							ToolCallID: "call_legacy",
							ToolName:   "bash",
							OutputText: "ok",
						},
					}},
				},
			},
		},
	}

	converted := canonicalRequestToResponses(req)
	if len(converted.Input) != 2 {
		t.Fatalf("len(converted.Input) = %d, want 2", len(converted.Input))
	}
	if converted.Input[0].Type != "function_call" {
		t.Fatalf("first replay item type = %q, want function_call", converted.Input[0].Type)
	}
	if converted.Input[0].CallID != "call_legacy" {
		t.Fatalf("first replay item call_id = %q, want call_legacy", converted.Input[0].CallID)
	}
	if converted.Input[1].Type != "function_call_output" {
		t.Fatalf("second replay item type = %q, want function_call_output", converted.Input[1].Type)
	}
}

func TestCanonicalRequestToResponses_DropsLegacyAssistantThinkingBlocksWithoutEncryptedContent(t *testing.T) {
	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: []ai.ConversationMessage{
				{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{
						{Type: ai.BlockThinking, Text: "legacy summary only"},
						{Type: ai.BlockText, Text: "hello"},
					},
				},
				{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "continue",
					}},
				},
			},
		},
	}

	converted := canonicalRequestToResponses(req)
	if len(converted.Input) != 2 {
		t.Fatalf("len(converted.Input) = %d, want 2", len(converted.Input))
	}
	if converted.Input[0].Role != "assistant" {
		t.Fatalf("first item role = %q, want assistant", converted.Input[0].Role)
	}
	if converted.Input[1].Role != "user" {
		t.Fatalf("second item role = %q, want user", converted.Input[1].Role)
	}
}

func TestCanonicalRequestToResponses_DropsHistoricalEncryptedThinkingReplay(t *testing.T) {
	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: []ai.ConversationMessage{
				{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "hello",
					}},
				},
				{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{
						{
							Type:              ai.BlockThinking,
							Text:              "thinking summary",
							ThinkingSignature: "rs_test_reasoning",
							EncryptedContent:  "enc_reasoning",
						},
						{
							Type: ai.BlockToolCall,
							ToolCall: &ai.CanonicalToolCall{
								ID:           "call_123",
								Name:         "read",
								RawArguments: `{"path":"/tmp/demo.md"}`,
							},
						},
					},
				},
				{
					Role: ai.RoleCanonicalTool,
					Content: []ai.ContentBlock{{
						Type: ai.BlockToolResult,
						ToolResult: &ai.CanonicalToolResult{
							ToolCallID: "call_123",
							ToolName:   "read",
							OutputText: "ok",
						},
					}},
				},
			},
		},
	}

	converted := canonicalRequestToResponses(req)
	if len(converted.Input) != 3 {
		t.Fatalf("len(converted.Input) = %d, want 3", len(converted.Input))
	}
	if converted.Input[0].Role != "user" {
		t.Fatalf("first item role = %q, want user", converted.Input[0].Role)
	}
	if converted.Input[1].Type != "function_call" {
		t.Fatalf("second item type = %q, want function_call", converted.Input[1].Type)
	}
	if converted.Input[1].CallID != "call_123" {
		t.Fatalf("second item call_id = %q, want call_123", converted.Input[1].CallID)
	}
	if converted.Input[2].Type != "function_call_output" {
		t.Fatalf("third item type = %q, want function_call_output", converted.Input[2].Type)
	}
}

func TestCanonicalRequestToResponses_DropsCrossProviderReasoningReplay(t *testing.T) {
	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: []ai.ConversationMessage{
				{
					Role: ai.RoleCanonicalAssistant,
					ProviderState: &ai.ProviderState{
						API: "anthropic-messages",
					},
					Content: []ai.ContentBlock{
						{
							Type:              ai.BlockThinking,
							Text:              "anthropic reasoning",
							ThinkingSignature: "sig_123",
							EncryptedContent:  "enc_123",
						},
						{
							Type: ai.BlockText,
							Text: "final answer",
						},
					},
				},
			},
		},
	}

	sanitized := ai.PrepareCanonicalReplayForTarget(req, ai.ReplayTarget{API: "openai-responses", Provider: "openai", Model: "gpt-5.4"})
	converted := canonicalRequestToResponses(sanitized)
	if len(converted.Input) != 1 {
		t.Fatalf("len(converted.Input) = %d, want 1", len(converted.Input))
	}
	if converted.Input[0].Role != "assistant" {
		t.Fatalf("converted.Input[0] = %#v, want only assistant text replay", converted.Input[0])
	}
	if len(converted.Input[0].Content) != 1 || converted.Input[0].Content[0].Text != "final answer" {
		t.Fatalf("converted.Input[0].Content = %#v, want final answer text replay", converted.Input[0].Content)
	}
}

func TestCanonicalRequestToResponses_SemanticHandoffConvertsCrossProviderTools(t *testing.T) {
	req := &ai.ProviderRequest{
		Config: ai.GenerationConfig{Model: "gpt-5.5"},
		Context: ai.ConversationContext{
			Messages: []ai.ConversationMessage{
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
	prepared := prepareCanonicalReplayForProvider(req, &op.ModelConfig{
		Provider: "opagent-ai-gateway",
		API:      "openai-responses",
		Name:     "gpt-5.5",
	}, "openai-responses")

	converted := canonicalRequestToResponses(prepared)
	raw, err := json.Marshal(converted.Input)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	text := string(raw)
	if strings.Contains(text, `"function_call"`) || strings.Contains(text, `"function_call_output"`) || strings.Contains(text, `"reasoning"`) {
		t.Fatalf("cross-target Responses replay retained provider protocol items: %s", text)
	}
	if strings.Contains(text, "private reasoning") {
		t.Fatalf("cross-target Responses replay leaked thinking: %s", text)
	}
	if !strings.Contains(text, "Historical tool call: bash") || !strings.Contains(text, "Historical tool result: bash") {
		t.Fatalf("cross-target Responses replay missing semantic tool history: %s", text)
	}
}

func TestCanonicalRequestToResponses_NormalizesMixedLegacyAndNativeContinuationHistory(t *testing.T) {
	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: []ai.ConversationMessage{
				{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "clone the repo",
					}},
				},
				{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{{
						Type: ai.BlockToolCall,
						ToolCall: &ai.CanonicalToolCall{
							ID:           "call_legacy",
							Name:         "bash",
							RawArguments: `{"command":"git clone git@github.com:colinagent/my-gblog.git blog/my-gblog"}`,
						},
					}},
				},
				{
					Role: ai.RoleCanonicalTool,
					Content: []ai.ContentBlock{{
						Type: ai.BlockToolResult,
						ToolResult: &ai.CanonicalToolResult{
							ToolCallID: "call_legacy",
							ToolName:   "bash",
							OutputText: "Cloning into 'blog/my-gblog'...",
						},
					}},
				},
				{
					Role: ai.RoleCanonicalAssistant,
					Content: []ai.ContentBlock{{
						Type:          ai.BlockText,
						Text:          "Repository cloned.",
						TextSignature: "msg_native",
					}},
					ProviderState: &ai.ProviderState{
						ResponseID: "resp_native",
					},
				},
				{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "continue",
					}},
				},
			},
		},
	}

	converted := canonicalRequestToResponses(req)
	if len(converted.Input) != 5 {
		t.Fatalf("len(converted.Input) = %d, want 5", len(converted.Input))
	}
	if converted.Input[0].Role != "user" {
		t.Fatalf("first item role = %q, want user", converted.Input[0].Role)
	}
	if converted.Input[1].Type != "function_call" {
		t.Fatalf("second item type = %q, want function_call", converted.Input[1].Type)
	}
	if converted.Input[1].CallID != "call_legacy" {
		t.Fatalf("second item call_id = %q, want call_legacy", converted.Input[1].CallID)
	}
	if converted.Input[2].Type != "function_call_output" {
		t.Fatalf("third item type = %q, want function_call_output", converted.Input[2].Type)
	}
	if converted.Input[3].Role != "assistant" {
		t.Fatalf("fourth item role = %q, want assistant", converted.Input[3].Role)
	}
	if converted.Input[3].ID != "msg_native" {
		t.Fatalf("fourth item id = %q, want msg_native", converted.Input[3].ID)
	}
	if got := string(converted.Input[3].Raw); !strings.Contains(got, `"role":"assistant"`) || strings.Contains(got, `"status"`) {
		t.Fatalf("fourth item raw = %s, want normalized assistant replay", got)
	}
	if converted.Input[4].Role != "user" {
		t.Fatalf("fifth item role = %q, want user", converted.Input[4].Role)
	}
}

func TestCanonicalRequestToResponses_TruncatesOversizedToolReplayOutput(t *testing.T) {
	var builder strings.Builder
	for i := 1; i <= 2300; i++ {
		if i > 1 {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("line-%04d-%s", i, strings.Repeat("x", 40)))
	}
	builder.WriteString("\nreplay-tail-sentinel")

	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			Messages: ai.CanonicalMessagesFromReplayableOp([]op.Message{
				op.NewToolResultMessage("bash", "call-oversized", builder.String()),
			}),
		},
	}

	converted := canonicalRequestToResponses(req)
	if len(converted.Input) != 1 {
		t.Fatalf("len(converted.Input) = %d, want 1", len(converted.Input))
	}
	if converted.Input[0].Type != "function_call_output" {
		t.Fatalf("item type = %q, want function_call_output", converted.Input[0].Type)
	}
	if !strings.Contains(converted.Input[0].OutputText, "replay-tail-sentinel") {
		t.Fatalf("expected replay output to keep tail sentinel, got %q", converted.Input[0].OutputText)
	}
	if strings.Contains(converted.Input[0].OutputText, "line-0001-") {
		t.Fatalf("expected replay output to drop old head lines, got %q", converted.Input[0].OutputText)
	}
	if !strings.Contains(converted.Input[0].OutputText, "Historical tool output truncated for replay") {
		t.Fatalf("expected replay truncation notice, got %q", converted.Input[0].OutputText)
	}
}

func TestCanonicalRequestToResponses_FoldsSystemAndDeveloperMessagesIntoInstructions(t *testing.T) {
	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			SystemPrompt: "system instructions",
			Messages: []ai.ConversationMessage{
				{
					Role: ai.RoleCanonicalSystem,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "system replayed context",
					}},
				},
				{
					Role: ai.RoleCanonicalDeveloper,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "developer guidance",
					}},
				},
				{
					Role: ai.RoleCanonicalUser,
					Content: []ai.ContentBlock{{
						Type: ai.BlockText,
						Text: "hello",
					}},
				},
			},
		},
	}

	converted := canonicalRequestToResponses(req)
	if converted.Instructions != "system instructions\n\nsystem replayed context\n\ndeveloper guidance" {
		t.Fatalf("instructions = %q, want folded instructions", converted.Instructions)
	}
	if len(converted.Input) != 1 {
		t.Fatalf("len(converted.Input) = %d, want 1", len(converted.Input))
	}
	if converted.Input[0].Role != "user" {
		t.Fatalf("user role = %q, want user", converted.Input[0].Role)
	}
}

func TestResponsesProviderBuildNativeRequest_KeepsPreviousResponseID(t *testing.T) {
	provider := &ResponsesProvider{
		cfg: &op.ModelConfig{
			ID:       "test",
			Name:     "gpt-5.4",
			Provider: "openai",
			APIKey:   "test-key",
			BaseURL:  "https://api.openai.com/v1",
		},
	}

	params, err := provider.buildNativeRequest(&ai.ResponsesRequest{
		Model:              "gpt-5.4",
		PreviousResponseID: "resp_prev_123",
		Input: []ai.ResponseItem{
			ai.ParseResponseItemRaw(json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`)),
		},
	})
	if err != nil {
		t.Fatalf("buildNativeRequest: %v", err)
	}
	if got := params.PreviousResponseID.Value; strings.TrimSpace(got) != "resp_prev_123" {
		t.Fatalf("previous_response_id = %q, want resp_prev_123", got)
	}
}

func TestResponsesProviderBuildNativeRequest_WritesPriorityServiceTier(t *testing.T) {
	provider := &ResponsesProvider{
		cfg: &op.ModelConfig{
			ID:       "test",
			Name:     "gpt-5.4",
			Provider: "openai",
			APIKey:   "test-key",
			BaseURL:  "https://api.openai.com/v1",
		},
	}

	params, err := provider.buildNativeRequest(&ai.ResponsesRequest{
		Model:       "gpt-5.4",
		ServiceTier: ai.ServiceTierPriority,
	})
	if err != nil {
		t.Fatalf("buildNativeRequest: %v", err)
	}
	if got := params.ServiceTier; got != responses.ResponseNewParamsServiceTierPriority {
		t.Fatalf("service_tier = %q, want priority", got)
	}
}

func TestResponsesProviderNormalizeRequestForProvider_LeavesCodexLikeURLReasoningUntouched(t *testing.T) {
	cfg := &op.ModelConfig{
		ID:       "gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "openai",
		APIKey:   "test-key",
		BaseURL:  "https://api.aicodemirror.com/api/codex/backend-api/codex",
	}
	provider := &ResponsesProvider{cfg: cfg}
	normalized := provider.normalizeRequestForProvider(&ai.ResponsesRequest{
		Model:     "gpt-5.4",
		Reasoning: &ai.ResponsesReasoning{Effort: "minimal"},
	})
	if normalized == nil || normalized.Reasoning == nil {
		t.Fatalf("normalized reasoning = %#v, want non-nil", normalized)
	}
	if got := normalized.Reasoning.Effort; got != "minimal" {
		t.Fatalf("reasoning effort = %q, want minimal", got)
	}
	if got := normalized.Reasoning.Summary; got != "" {
		t.Fatalf("reasoning summary = %q, want empty", got)
	}
}

func TestResponsesProviderNormalizeRequestForProvider_LeavesOpenAIMinimalReasoningUnchanged(t *testing.T) {
	cfg := &op.ModelConfig{
		ID:       "gpt-5.4",
		Name:     "gpt-5.4",
		Provider: "openai",
		APIKey:   "test-key",
		BaseURL:  "https://api.openai.com/v1",
	}
	provider := &ResponsesProvider{cfg: cfg}
	normalized := provider.normalizeRequestForProvider(&ai.ResponsesRequest{
		Model:     "gpt-5.4",
		Reasoning: &ai.ResponsesReasoning{Effort: "minimal", Summary: "auto"},
	})
	if normalized == nil || normalized.Reasoning == nil {
		t.Fatalf("normalized reasoning = %#v, want non-nil", normalized)
	}
	if got := normalized.Reasoning.Effort; got != "minimal" {
		t.Fatalf("reasoning effort = %q, want minimal", got)
	}
}

func TestConvertNativeResponsesInput_EncryptedReasoningAlwaysIncludesSummaryField(t *testing.T) {
	items, err := convertNativeResponsesInput([]ai.ResponseItem{{
		Type:             "reasoning",
		ID:               "rs_test",
		EncryptedContent: "enc_123",
	}})
	if err != nil {
		t.Fatalf("convertNativeResponsesInput(): %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	raw, err := json.Marshal(items[0])
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"summary":[]`) {
		t.Fatalf("reasoning replay item = %s, want summary field", text)
	}
}

func TestConvertNativeResponsesInput_NormalizesAssistantReplayMessage(t *testing.T) {
	input, err := convertNativeResponsesInput([]ai.ResponseItem{{
		Role: "assistant",
		ID:   "msg_123",
		Content: []ai.ResponseContentPart{{
			Type: "output_text",
			Text: "Hello!",
		}},
	}})
	if err != nil {
		t.Fatalf("convertNativeResponsesInput: %v", err)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"role":"assistant"`) || !strings.Contains(text, `"id":"msg_123"`) {
		t.Fatalf("assistant replay payload = %s", text)
	}
	if strings.Contains(text, `"status":"completed"`) {
		t.Fatalf("assistant replay payload unexpectedly contains status: %s", text)
	}
	if strings.Contains(text, `"type":"message"`) {
		t.Fatalf("assistant replay payload unexpectedly contains type=message: %s", text)
	}
}

func TestConvertNativeResponsesInput_PreservesFunctionCallReplayID(t *testing.T) {
	input, err := convertNativeResponsesInput([]ai.ResponseItem{{
		Type:      "function_call",
		ID:        "fc_123",
		CallID:    "call_123",
		Name:      "bash",
		Arguments: `{"command":"pwd"}`,
	}})
	if err != nil {
		t.Fatalf("convertNativeResponsesInput: %v", err)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"type":"function_call"`) || !strings.Contains(text, `"id":"fc_123"`) || !strings.Contains(text, `"call_id":"call_123"`) {
		t.Fatalf("function call replay payload = %s", text)
	}
}

func TestConvertNativeResponsesInput_PreservesEmptyFunctionCallOutput(t *testing.T) {
	input, err := convertNativeResponsesInput([]ai.ResponseItem{{
		Type:       "function_call_output",
		CallID:     "call_empty",
		Status:     "completed",
		OutputText: "",
	}})
	if err != nil {
		t.Fatalf("convertNativeResponsesInput: %v", err)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"type":"function_call_output"`) || !strings.Contains(text, `"call_id":"call_empty"`) {
		t.Fatalf("function_call_output replay payload = %s", text)
	}
	if !strings.Contains(text, `"output":""`) {
		t.Fatalf("expected empty output field to be preserved, got %s", text)
	}
}

func TestConvertNativeResponsesInput_SanitizesStaleFunctionCallOutputRaw(t *testing.T) {
	item := ai.ParseResponseItemRaw(json.RawMessage(`{"type":"function_call_output","call_id":"call_empty","status":"completed"}`))
	input, err := convertNativeResponsesInput([]ai.ResponseItem{item})
	if err != nil {
		t.Fatalf("convertNativeResponsesInput: %v", err)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"type":"function_call_output"`) || !strings.Contains(text, `"call_id":"call_empty"`) {
		t.Fatalf("function_call_output replay payload = %s", text)
	}
	if !strings.Contains(text, `"output":""`) {
		t.Fatalf("expected sanitized empty output field to be preserved, got %s", text)
	}
}

func TestNormalizeResponsesReplayInput_TruncatesCanonicalToolReplayOutput(t *testing.T) {
	var builder strings.Builder
	for i := 1; i <= 1200; i++ {
		if i > 1 {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("line-%04d-%s", i, strings.Repeat("x", 32)))
	}
	builder.WriteString("\nreplay-tail-sentinel")

	normalized := normalizeResponsesReplayInput([]ai.ResponseItem{{
		Type:       "function_call_output",
		CallID:     "call_big",
		OutputText: builder.String(),
	}})
	if len(normalized) != 1 {
		t.Fatalf("len(normalized) = %d, want 1", len(normalized))
	}
	got := normalized[0].OutputText
	if !strings.Contains(got, "replay-tail-sentinel") {
		t.Fatalf("expected replay output to keep tail sentinel, got %q", got)
	}
	if strings.Contains(got, "line-0001-") {
		t.Fatalf("expected replay output to drop old head lines, got %q", got)
	}
	if !strings.Contains(got, "Historical tool output truncated for replay") {
		t.Fatalf("expected replay truncation notice, got %q", got)
	}
	if len([]byte(got)) > responsesReplayToolOutputMaxBytes {
		t.Fatalf("normalized replay output bytes = %d, want <= %d", len([]byte(got)), responsesReplayToolOutputMaxBytes)
	}
}

func TestNormalizeResponsesReplayInput_PreservesToolResultImagesWhenTextIsTruncated(t *testing.T) {
	var builder strings.Builder
	for i := 1; i <= 1200; i++ {
		if i > 1 {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("line-%04d-%s", i, strings.Repeat("x", 32)))
	}
	builder.WriteString("\nimage-tail-sentinel")
	imageURL := "data:image/png;base64," + strings.Repeat("A", responsesReplayToolOutputMaxBytes+64)

	normalized := normalizeResponsesReplayInput([]ai.ResponseItem{{
		Type:   "function_call_output",
		CallID: "call_img",
		OutputContent: []ai.ResponseContentPart{
			{Type: "input_text", Text: builder.String()},
			{Type: "input_image", ImageURL: imageURL, Detail: "auto"},
		},
	}})

	if len(normalized) != 1 {
		t.Fatalf("len(normalized) = %d, want 1", len(normalized))
	}
	content := normalized[0].OutputContent
	if len(content) != 2 {
		t.Fatalf("len(OutputContent) = %d, want 2: %#v", len(content), content)
	}
	text := content[0].Text
	if !strings.Contains(text, "image-tail-sentinel") {
		t.Fatalf("expected replay text to keep tail sentinel, got %q", text)
	}
	if strings.Contains(text, "line-0001-") {
		t.Fatalf("expected replay text to drop old head lines, got %q", text)
	}
	if !strings.Contains(text, "Historical tool output truncated for replay") {
		t.Fatalf("expected replay truncation notice, got %q", text)
	}
	if content[1].Type != "input_image" || content[1].ImageURL != imageURL || content[1].Detail != "auto" {
		t.Fatalf("image content = %#v, want exact image URL preserved", content[1])
	}
}

func TestNormalizeResponsesReplayInput_EnforcesAggregateToolReplayBudget(t *testing.T) {
	itemCount := responsesReplayToolOutputTotalMax/responsesReplayToolOutputMaxBytes + 6
	items := make([]ai.ResponseItem, 0, itemCount)
	for i := 0; i < itemCount; i++ {
		var builder strings.Builder
		for line := 0; line < 220; line++ {
			if line > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(strings.Repeat("z", 48))
		}
		builder.WriteString(fmt.Sprintf("\nmarker-%02d", i))
		items = append(items, ai.ResponseItem{
			Type:       "function_call_output",
			CallID:     fmt.Sprintf("call_%02d", i),
			OutputText: builder.String(),
		})
	}

	normalized := normalizeResponsesReplayInput(items)
	if len(normalized) != len(items) {
		t.Fatalf("len(normalized) = %d, want %d", len(normalized), len(items))
	}
	if !strings.Contains(normalized[len(normalized)-1].OutputText, fmt.Sprintf("marker-%02d", len(normalized)-1)) {
		t.Fatalf("newest replay output = %q, want newest marker", normalized[len(normalized)-1].OutputText)
	}
	if strings.Contains(normalized[0].OutputText, "marker-00") {
		t.Fatalf("oldest replay output unexpectedly kept full marker: %q", normalized[0].OutputText)
	}
	if got := normalized[0].OutputText; got != responsesReplayToolOutputOmittedNotice && got != responsesReplayToolOutputShortOmittedNotice && got != "" {
		t.Fatalf("oldest replay output = %q, want omission notice or empty output", got)
	}

	totalBytes := 0
	for _, item := range normalized {
		totalBytes += len([]byte(item.OutputText))
	}
	if totalBytes > responsesReplayToolOutputTotalMax {
		t.Fatalf("normalized replay output bytes = %d, want <= %d", totalBytes, responsesReplayToolOutputTotalMax)
	}
}
