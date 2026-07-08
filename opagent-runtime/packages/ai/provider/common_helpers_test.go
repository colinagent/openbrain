package provider

import "testing"

func TestProviderToolArgumentsMap(t *testing.T) {
	t.Run("json object", func(t *testing.T) {
		got := providerToolArgumentsMap(`{"command":"ls"}`)
		if got["command"] != "ls" {
			t.Fatalf("providerToolArgumentsMap json object = %#v, want command=ls", got)
		}
	})

	t.Run("invalid json falls back to input", func(t *testing.T) {
		raw := `command=ls -la`
		got := providerToolArgumentsMap(raw)
		if got["input"] != raw {
			t.Fatalf("providerToolArgumentsMap invalid json = %#v, want input=%q", got, raw)
		}
	})

	t.Run("empty returns empty object", func(t *testing.T) {
		got := providerToolArgumentsMap("   ")
		if len(got) != 0 {
			t.Fatalf("providerToolArgumentsMap empty = %#v, want empty map", got)
		}
	})
}

func TestNormalizeProviderSchemaMap(t *testing.T) {
	type toolSchema struct {
		Type                 string         `json:"type"`
		Properties           map[string]any `json:"properties"`
		Required             []string       `json:"required"`
		AdditionalProperties bool           `json:"additionalProperties"`
	}

	got := normalizeProviderSchemaMap(toolSchema{
		Type: "object",
		Properties: map[string]any{
			"command": map[string]any{"type": "string"},
		},
		Required:             []string{"command"},
		AdditionalProperties: false,
	})

	if got["type"] != "object" {
		t.Fatalf("type = %#v, want object", got["type"])
	}
	properties, ok := got["properties"].(map[string]any)
	if !ok || len(properties) != 1 {
		t.Fatalf("properties = %#v, want one property map", got["properties"])
	}
	required := normalizeProviderSchemaRequired(got["required"])
	if len(required) != 1 || required[0] != "command" {
		t.Fatalf("required = %#v, want [command]", required)
	}
	if got["additionalProperties"] != false {
		t.Fatalf("additionalProperties = %#v, want false", got["additionalProperties"])
	}
}
