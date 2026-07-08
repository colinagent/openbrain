package op

import "testing"

func TestMessageValidate_assistantRequiresContentOrToolCalls(t *testing.T) {
	err := (Message{
		Role:             RoleAssistant,
		ReasoningContent: "thinking only",
	}).Validate()
	if err == nil {
		t.Fatalf("expected validation error for reasoning-only assistant")
	}
}

func TestMessageValidate_assistantWithToolCallsIsValid(t *testing.T) {
	err := (Message{
		Role: RoleAssistant,
		ToolCalls: []MessageToolCall{
			{ID: "call_1", Name: "shell", Arguments: map[string]any{"command": "ls"}},
		},
	}).Validate()
	if err != nil {
		t.Fatalf("expected valid assistant with tool calls, got: %v", err)
	}
}
