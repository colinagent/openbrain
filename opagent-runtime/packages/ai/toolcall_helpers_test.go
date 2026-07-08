package ai

import "testing"

func TestMarshalToolArgumentsJSON(t *testing.T) {
	if got := MarshalToolArgumentsJSON(nil); got != "" {
		t.Fatalf("MarshalToolArgumentsJSON(nil) = %q, want empty", got)
	}
	got := MarshalToolArgumentsJSON(map[string]any{"command": "pwd"})
	if got != `{"command":"pwd"}` {
		t.Fatalf("MarshalToolArgumentsJSON(map) = %q, want compact json", got)
	}
}
