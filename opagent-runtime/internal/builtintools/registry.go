package builtintools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const ServerID = "__systool__"

type NotifyFunc func(context.Context, *op.InfoNotificationParams)

func IsOSToolName(name string) bool {
	switch normalizeName(name) {
	case "shell", "read", "write", "edit":
		return true
	default:
		return false
	}
}

func IsBuiltinName(name string) bool {
	switch normalizeName(name) {
	case "shell", "read", "write", "edit", "agent_task", "message_publish", "message_update", "message_read", "message_subscribe", "message_ack":
		return true
	default:
		return false
	}
}

func OSToolSpec(name string) (*op.ToolSpec, bool) {
	switch normalizeName(name) {
	case "shell":
		return &op.ToolSpec{
			ServerID:    ServerID,
			Name:        "shell",
			Description: ShellToolDescription(),
			InputSchema: objectSchema(
				[]any{"command"},
				map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Shell command/script to execute in non-interactive mode",
					},
					"timeoutSeconds": map[string]any{
						"type":        "integer",
						"description": "Timeout in seconds (0 uses default, negative disables timeout)",
					},
				},
			),
		}, true
	case "read":
		return &op.ToolSpec{
			ServerID:    ServerID,
			Name:        "read",
			Description: "Read file contents from local filesystem. Supports text files and PNG/JPEG/GIF/WEBP images; image files are returned to the model as image content.",
			InputSchema: objectSchema(
				[]any{"path"},
				map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute or relative file path to read",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "Optional 1-indexed starting line for partial reads",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Optional maximum number of lines to read",
					},
				},
			),
		}, true
	case "write":
		return &op.ToolSpec{
			ServerID:    ServerID,
			Name:        "write",
			Description: "Write file contents to local filesystem",
			InputSchema: objectSchema(
				[]any{"path", "content"},
				map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute or relative file path to write",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Content to write into file",
					},
				},
			),
		}, true
	case "edit":
		return &op.ToolSpec{
			ServerID:    ServerID,
			Name:        "edit",
			Description: "Edit file by replacing one unique old text block with new text",
			InputSchema: objectSchema(
				[]any{"path", "oldText", "newText"},
				map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute or relative file path to edit",
					},
					"oldText": map[string]any{
						"type":        "string",
						"description": "Exact old text to replace. Include enough context so the match is unique",
					},
					"newText": map[string]any{
						"type":        "string",
						"description": "Replacement text",
					},
				},
			),
		}, true
	default:
		return nil, false
	}
}

func Execute(ctx context.Context, name string, params any, meta op.Meta, notifier NotifyFunc) (*op.CallToolResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if meta == nil {
		meta = op.Meta{}
	}
	switch normalizeName(name) {
	case "shell":
		var input shellInput
		if err := decodeParams(params, &input); err != nil {
			return nil, err
		}
		return executeShell(ctx, meta, notifier, input)
	case "read":
		var input readInput
		if err := decodeParams(params, &input); err != nil {
			return nil, err
		}
		service, err := newToolServiceFromMeta(meta)
		if err != nil {
			return nil, err
		}
		result, err := service.Read(input)
		if err != nil {
			return nil, err
		}
		return result.callToolResult(meta), nil
	case "write":
		var input writeInput
		if err := decodeParams(params, &input); err != nil {
			return nil, err
		}
		service, err := newToolServiceFromMeta(meta)
		if err != nil {
			return nil, err
		}
		result, err := service.Write(input)
		if err != nil {
			return nil, err
		}
		return result.callToolResult(meta), nil
	case "edit":
		var input editInput
		if err := decodeParams(params, &input); err != nil {
			return nil, err
		}
		service, err := newToolServiceFromMeta(meta)
		if err != nil {
			return nil, err
		}
		result, err := service.Edit(input)
		if err != nil {
			return nil, err
		}
		return result.callToolResult(meta), nil
	default:
		return nil, fmt.Errorf("unknown built-in systool: %s", name)
	}
}

func objectSchema(required []any, properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}

func decodeParams(params any, out any) error {
	if params == nil {
		params = map[string]any{}
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal tool arguments: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parse tool arguments: %w", err)
	}
	return nil
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
