package chat

import (
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestToolArgumentsStringValue(t *testing.T) {
	args, ok := toolArgumentsFromTurnResult(testToolResultArgs(map[string]any{
		"path":    " .agent/context/main.md ",
		"command": "pwd",
	}))
	if !ok {
		t.Fatal("toolArgumentsFromTurnResult ok = false, want true")
	}
	if got, ok := args.stringValue("path"); !ok || got != ".agent/context/main.md" {
		t.Fatalf("path = %q, %v; want .agent/context/main.md, true", got, ok)
	}
	if got, ok := args.stringValue("command"); !ok || got != "pwd" {
		t.Fatalf("command = %q, %v; want pwd, true", got, ok)
	}
}

func TestToolArgumentsRejectInvalidOrEmptyValues(t *testing.T) {
	args, ok := toolArgumentsFromTurnResult(testToolResultArgs(map[string]any{"path": "   "}))
	if !ok {
		t.Fatal("toolArgumentsFromTurnResult ok = false, want true for non-empty object")
	}
	if got, ok := args.stringValue("path"); ok || got != "" {
		t.Fatalf("blank path = %q, %v; want empty, false", got, ok)
	}
	if got, ok := args.stringValue("missing"); ok || got != "" {
		t.Fatalf("missing value = %q, %v; want empty, false", got, ok)
	}
}

func testToolResultArgs(args map[string]any) op.TurnResultToolResult {
	return op.TurnResultToolResult{ArgumentsObject: args}
}
