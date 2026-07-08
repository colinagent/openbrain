package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

type ToolCallDelta struct {
	Index       int
	ID          string
	Type        string
	Name        string
	Description string
	Arguments   string
}

type ToolCallState struct {
	buffers map[string]*strings.Builder
	infos   map[string]*ToolCall
	order   []string
}

type toolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Params      any    `json:"params,omitempty"`
}

type ToolCall struct {
	Type string   `json:"type"`
	ID   string   `json:"id"`
	Info toolInfo `json:"info"`
}

func NewToolCallState() *ToolCallState {
	return &ToolCallState{
		buffers: make(map[string]*strings.Builder),
		infos:   make(map[string]*ToolCall),
		order:   make([]string, 0),
	}
}

func (s *ToolCallState) Accumulate(delta ToolCallDelta) {
	key := fmt.Sprintf("tool_%d", delta.Index)
	if _, exists := s.buffers[key]; !exists {
		s.buffers[key] = &strings.Builder{}
		s.infos[key] = &ToolCall{}
		s.order = append(s.order, key)
	}
	info := s.infos[key]
	if delta.ID != "" {
		info.ID = delta.ID
	}
	if delta.Type != "" {
		info.Type = delta.Type
	}
	if delta.Name != "" {
		info.Info.Name = delta.Name
	}
	if delta.Description != "" {
		info.Info.Description = delta.Description
	}
	if delta.Arguments != "" {
		s.buffers[key].WriteString(delta.Arguments)
	}
}

func (s *ToolCallState) Finalize() []ToolCall {
	result := make([]ToolCall, 0, len(s.order))
	for _, key := range s.order {
		info := s.infos[key]
		if info.Info.Params == nil {
			argsStr := s.buffers[key].String()
			if argsStr != "" {
				var parsed map[string]any
				if err := json.Unmarshal([]byte(argsStr), &parsed); err != nil {
					slog.Error("unmarshal tool arguments error", "error", err, "arguments", argsStr)
					continue
				}
				info.Info.Params = parsed
			}
		}
		result = append(result, *info)
	}
	return result
}

func (s *ToolCallState) Reset() {
	s.buffers = make(map[string]*strings.Builder)
	s.infos = make(map[string]*ToolCall)
	s.order = s.order[:0]
}

func (s *ToolCallState) HasPending() bool {
	return len(s.order) > 0
}

func buildAssistantToolCallMsg(calls []ToolCall) op.Message {
	toolCalls := make([]op.MessageToolCall, 0, len(calls))
	for _, tc := range calls {
		argsObj, _ := tc.Info.Params.(map[string]any)
		toolCalls = append(toolCalls, op.MessageToolCall{
			ID:        tc.ID,
			Name:      tc.Info.Name,
			Arguments: opagentCloneToolArguments(argsObj),
			Type:      "function",
		})
	}
	return op.NewAssistantToolCalls(toolCalls)
}

func opagentCloneToolArguments(arguments map[string]any) map[string]any {
	if len(arguments) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(arguments))
	for key, value := range arguments {
		cloned[key] = value
	}
	return cloned
}

func sanitizeToolArgumentsForSchema(arguments any, inputSchema any) any {
	args, ok := arguments.(map[string]any)
	if !ok || len(args) == 0 {
		return arguments
	}
	schema, ok := inputSchema.(map[string]any)
	if !ok {
		return arguments
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		return arguments
	}

	required := make(map[string]bool)
	if rawRequired, ok := schema["required"].([]any); ok {
		for _, item := range rawRequired {
			key, ok := item.(string)
			if ok {
				required[key] = true
			}
		}
	}

	cleaned := opagentCloneToolArguments(args)
	changed := false
	for key, value := range cleaned {
		if required[key] {
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) != "" {
			continue
		}
		property, ok := properties[key].(map[string]any)
		if !ok || !jsonSchemaTypeIncludes(property["type"], "string") {
			continue
		}
		delete(cleaned, key)
		changed = true
	}
	if !changed {
		return arguments
	}
	return cleaned
}

func jsonSchemaTypeIncludes(raw any, expected string) bool {
	switch value := raw.(type) {
	case string:
		return value == expected
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok && text == expected {
				return true
			}
		}
	}
	return false
}

func callToolServerTool(loop *Loop, tc ToolCall, serverID string) (string, *op.CallToolResult, error) {
	parts := strings.SplitN(tc.Info.Name, "__", 2)
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("invalid tool server tool name: %s", tc.Info.Name)
	}
	return callTool(loop, tc, serverID, parts[1])
}

func callSystemTool(loop *Loop, tc ToolCall, serverID string) (string, *op.CallToolResult, error) {
	return callTool(loop, tc, serverID, tc.Info.Name)
}

func callTool(loop *Loop, tc ToolCall, serverID, toolName string) (string, *op.CallToolResult, error) {
	ctx := loop.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var node *op.OpNode
	loadNode := func() (*op.OpNode, error) {
		if node != nil {
			return node, nil
		}
		nodeVal, ok := cache.GetValue[op.OpNode](serverID, cache.PrefixNode)
		if !ok {
			return nil, fmt.Errorf("tool server node not found: %s", serverID)
		}
		node = &nodeVal
		return node, nil
	}

	conn := GetConn(serverID)
	if conn == nil || conn.Session == nil {
		loadedNode, err := loadNode()
		if err != nil {
			return "", nil, err
		}
		conn, err = EnsureConnection(ctx, loadedNode)
		if err != nil {
			return "", nil, fmt.Errorf("tool server %s unavailable: %w", serverID, err)
		}
	}
	meta := loop.Meta.Clone()
	slog.Info("calling tool",
		"toolCallID", tc.ID,
		"toolName", tc.Info.Name,
		"serverID", serverID,
		"tool", toolName)
	res, err := conn.CallTool(ctx, &op.CallToolParams{
		Meta:      meta,
		Name:      toolName,
		Arguments: tc.Info.Params,
	})
	if err != nil && ctx.Err() == nil && isRecoverableToolConnectionError(err) {
		slog.Warn("tool server connection closed; reconnecting",
			"toolCallID", tc.ID,
			"toolName", tc.Info.Name,
			"serverID", serverID,
			"tool", toolName,
			"error", err)
		loadedNode, nodeErr := loadNode()
		if nodeErr != nil {
			return "", nil, fmt.Errorf("%w; cannot reconnect: %v", err, nodeErr)
		}
		conn, reconnectErr := recoverConnection(ctx, loadedNode, conn)
		if reconnectErr != nil {
			return "", nil, fmt.Errorf("tool server %s connection failed: %w; reconnect failed: %v", serverID, err, reconnectErr)
		}
		res, err = conn.CallTool(ctx, &op.CallToolParams{
			Meta:      meta,
			Name:      toolName,
			Arguments: tc.Info.Params,
		})
	}
	if err != nil {
		return "", nil, err
	}
	return extractToolResultText(res)
}

func isRecoverableToolConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, op.ErrSessionMissing) {
		return true
	}
	message := err.Error()
	if errors.Is(err, op.ErrConnectionClosed) {
		return strings.Contains(message, "client is closing")
	}
	return strings.Contains(message, "connection session is nil")
}

func extractToolResultText(res *op.CallToolResult) (string, *op.CallToolResult, error) {
	var result strings.Builder
	hasContent := false
	lastEndsWithNewline := false
	appendSegment := func(segment string) {
		if segment == "" {
			return
		}
		if hasContent {
			if !lastEndsWithNewline && !strings.HasPrefix(segment, "\n") {
				result.WriteString("\n")
			}
		}
		result.WriteString(segment)
		hasContent = true
		lastEndsWithNewline = strings.HasSuffix(segment, "\n")
	}
	for _, content := range res.Content {
		if text, ok := content.(*op.TextContent); ok {
			appendSegment(text.Text)
		}
		if jsonContent, ok := content.(*op.JsonContent); ok {
			appendSegment(string(jsonContent.Raw))
		}
	}
	if res.StructuredContent != nil {
		jsonResult, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return "", nil, err
		}
		appendSegment(string(jsonResult))
	}
	return result.String(), res, nil
}

func toolResultMessageFromCallResult(toolName, callID, text string, res *op.CallToolResult) op.Message {
	resultText := text
	imageParts := toolResultImageParts(res)
	if strings.TrimSpace(resultText) == "" && len(imageParts) > 0 {
		resultText = "(see attached image)"
	}
	msg := op.NewToolResultMessage(toolName, callID, resultText)
	if len(imageParts) == 0 {
		return msg
	}
	parts := make([]op.ContentPart, 0, 1+len(imageParts))
	if strings.TrimSpace(resultText) != "" {
		parts = append(parts, op.ContentPart{Type: "text", Text: resultText})
	}
	parts = append(parts, imageParts...)
	msg.ContentParts = parts
	return msg
}

func toolResultImageParts(res *op.CallToolResult) []op.ContentPart {
	if res == nil || len(res.Content) == 0 {
		return nil
	}
	parts := make([]op.ContentPart, 0, 1)
	for _, content := range res.Content {
		image, ok := content.(*op.ImageContent)
		if !ok || len(image.Data) == 0 {
			continue
		}
		mimeType := strings.TrimSpace(image.MIMEType)
		if mimeType == "" {
			mimeType = "image/png"
		}
		parts = append(parts, op.ContentPart{
			Type: "image_url",
			ImageURL: &op.ImageURL{
				URL:    "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image.Data),
				Detail: "auto",
			},
		})
	}
	return parts
}
