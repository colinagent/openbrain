package core

import (
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const gbrainAgentID = "agent-gbrain"

func appendGBrainQueryScopePrompt(basePrompt string, agentID string, meta op.Meta) string {
	basePrompt = strings.TrimSpace(basePrompt)
	if strings.TrimSpace(agentID) != gbrainAgentID {
		return basePrompt
	}
	scopePrompt := buildGBrainQueryScopePrompt(meta)
	if scopePrompt == "" {
		return basePrompt
	}
	if basePrompt == "" {
		return scopePrompt
	}
	return basePrompt + "\n\n" + scopePrompt
}

func buildGBrainQueryScopePrompt(meta op.Meta) string {
	scope := metaMap(meta, "gbrainQueryScope")
	if scope == nil {
		return ""
	}
	kind := strings.TrimSpace(metaString(scope, "kind"))
	switch kind {
	case "source":
		sourceID := strings.TrimSpace(metaString(scope, "sourceID"))
		if sourceID == "" {
			return ""
		}
		label := strings.TrimSpace(metaString(scope, "label"))
		if label == "" {
			label = sourceID
		}
		return strings.Join([]string{
			"## OpenBrain GBrain Query Scope",
			"",
			fmt.Sprintf("This turn was started from OpenBrain graph scope %q.", label),
			fmt.Sprintf("Only use GBrain Cloud results from source_id %q.", sourceID),
			fmt.Sprintf("When calling the gbrain-cloud query tool, include source_id %q.", sourceID),
			"Do not use search or unscoped query for scoped retrieval unless the user explicitly asks to broaden the scope.",
		}, "\n")
	default:
		return ""
	}
}

func applyGBrainQueryScopeToToolCall(meta op.Meta, serverID string, toolName string, inputSchema any, params any) (any, error) {
	if !isGBrainCloudSourceScopedTool(serverID, toolName, inputSchema, params) {
		return params, nil
	}
	sourceIDs, scoped := gbrainQueryScopeSourceIDs(meta)
	if !scoped {
		return params, nil
	}
	if len(sourceIDs) == 0 {
		return params, fmt.Errorf("gbrain query scope has no allowed source IDs; refresh the OpenBrain graph before scoped retrieval")
	}
	args, ok := params.(map[string]any)
	if !ok {
		return params, fmt.Errorf("gbrain query scope requires object tool arguments")
	}
	allowed := make(map[string]bool, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		allowed[sourceID] = true
	}
	requested := strings.TrimSpace(metaString(op.Meta(args), "source_id"))
	if requested != "" {
		if !allowed[requested] {
			return params, fmt.Errorf("source_id %q is outside the OpenBrain graph scope; allowed source_id values: %s", requested, strings.Join(sourceIDs, ", "))
		}
		return params, nil
	}
	if len(sourceIDs) == 1 {
		next := opagentCloneToolArguments(args)
		next["source_id"] = sourceIDs[0]
		return next, nil
	}
	return params, fmt.Errorf("OpenBrain graph scope has multiple source IDs; call %s once per allowed source_id: %s", normalizeToolName(toolName), strings.Join(sourceIDs, ", "))
}

func isGBrainCloudSourceScopedTool(serverID string, toolName string, inputSchema any, params any) bool {
	if !strings.Contains(strings.ToLower(strings.TrimSpace(serverID)), "gbrain-cloud") {
		return false
	}
	switch normalizeToolName(toolName) {
	case "query", "search":
		return true
	}
	if toolInputSchemaHasSourceID(inputSchema) {
		return true
	}
	if args, ok := params.(map[string]any); ok {
		_, ok = args["source_id"]
		return ok
	}
	return false
}

func toolInputSchemaHasSourceID(inputSchema any) bool {
	schema, ok := inputSchema.(map[string]any)
	if !ok {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = properties["source_id"]
	return ok
}

func gbrainQueryScopeSourceIDs(meta op.Meta) ([]string, bool) {
	scope := metaMap(meta, "gbrainQueryScope")
	if scope == nil {
		return nil, false
	}
	kind := strings.TrimSpace(metaString(scope, "kind"))
	switch kind {
	case "source":
		sourceID := strings.TrimSpace(metaString(scope, "sourceID"))
		if sourceID == "" {
			return nil, true
		}
		return []string{sourceID}, true
	default:
		return nil, false
	}
}

func metaMap(meta op.Meta, key string) op.Meta {
	if meta == nil {
		return nil
	}
	switch value := meta[key].(type) {
	case op.Meta:
		return value
	case map[string]any:
		return op.Meta(value)
	default:
		return nil
	}
}

func normalizeMetaSlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []op.Meta:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out, true
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out, true
	default:
		return nil, false
	}
}
