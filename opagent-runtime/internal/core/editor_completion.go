package core

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/internal/nodeindex"
	"github.com/colinagent/openbrain/opagent-runtime/internal/scan"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

const (
	defaultEditorCompletionTimeout   = 12 * time.Second
	defaultEditorCompletionMaxTokens = int64(96)
	maxEditorCompletionContextChars  = 12000
	completionAgentID                = "completion"
)

var editorCompletionCancels = struct {
	mu   sync.Mutex
	byID map[string]context.CancelFunc
}{
	byID: make(map[string]context.CancelFunc),
}

func OpEditorCompletionHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	if req == nil || req.Params == nil {
		return nil, fmt.Errorf("params are required")
	}
	var params op.EditorCompletionRequest
	if content, ok := req.Params.Content.(*op.JsonContent); ok && content != nil {
		if err := content.Unmarshal(&params); err != nil {
			return nil, fmt.Errorf("decode editor completion request: %w", err)
		}
	} else if req.Params.Meta != nil {
		raw, err := json.Marshal(req.Params.Meta)
		if err != nil {
			return nil, fmt.Errorf("encode editor completion meta: %w", err)
		}
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("decode editor completion meta: %w", err)
		}
	}
	if strings.TrimSpace(params.RequestID) == "" {
		return nil, fmt.Errorf("requestID is required")
	}

	callCtx, cancel := context.WithTimeout(ctx, defaultEditorCompletionTimeout)
	registerEditorCompletionCancel(params.RequestID, cancel)
	defer unregisterEditorCompletionCancel(params.RequestID)
	defer cancel()

	result, err := executeEditorCompletion(callCtx, params)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal editor completion result: %w", err)
	}
	return &op.OpNodeResult{
		OpCode:  op.OpEditorCompletion,
		Content: &op.JsonContent{Raw: raw},
	}, nil
}

func OpEditorCompletionCancelHandler(req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	if req == nil || req.Params == nil {
		return nil, fmt.Errorf("params are required")
	}
	var params op.EditorCompletionCancelParams
	if content, ok := req.Params.Content.(*op.JsonContent); ok && content != nil {
		if err := content.Unmarshal(&params); err != nil {
			return nil, fmt.Errorf("decode editor completion cancel request: %w", err)
		}
	} else if req.Params.Meta != nil {
		raw, err := json.Marshal(req.Params.Meta)
		if err != nil {
			return nil, fmt.Errorf("encode editor completion cancel meta: %w", err)
		}
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("decode editor completion cancel meta: %w", err)
		}
	}
	if strings.TrimSpace(params.RequestID) == "" {
		return nil, fmt.Errorf("requestID is required")
	}
	cancelEditorCompletion(params.RequestID)
	raw, err := json.Marshal(map[string]bool{"cancelled": true})
	if err != nil {
		return nil, fmt.Errorf("marshal editor completion cancel result: %w", err)
	}
	return &op.OpNodeResult{
		OpCode:  op.OpEditorCompletionCancel,
		Content: &op.JsonContent{Raw: raw},
	}, nil
}

func registerEditorCompletionCancel(requestID string, cancel context.CancelFunc) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || cancel == nil {
		return
	}
	editorCompletionCancels.mu.Lock()
	defer editorCompletionCancels.mu.Unlock()
	if existing := editorCompletionCancels.byID[requestID]; existing != nil {
		existing()
	}
	editorCompletionCancels.byID[requestID] = cancel
}

func unregisterEditorCompletionCancel(requestID string) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return
	}
	editorCompletionCancels.mu.Lock()
	defer editorCompletionCancels.mu.Unlock()
	delete(editorCompletionCancels.byID, requestID)
}

func cancelEditorCompletion(requestID string) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return
	}
	editorCompletionCancels.mu.Lock()
	cancel := editorCompletionCancels.byID[requestID]
	delete(editorCompletionCancels.byID, requestID)
	editorCompletionCancels.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func executeEditorCompletion(ctx context.Context, params op.EditorCompletionRequest) (*op.EditorCompletionResult, error) {
	agentNode, err := resolveEditorCompletionAgent(params.AgentID)
	if err != nil {
		return nil, err
	}
	agentPrompt, err := loadSimpleCompletionAgentPrompt(ctx, agentNode, op.Meta{
		"modelKey": strings.TrimSpace(params.ModelKey),
	})
	if err != nil {
		return nil, fmt.Errorf("load completion agent: %w", err)
	}
	modelKey := strings.TrimSpace(params.ModelKey)
	if modelKey == "" {
		return nil, fmt.Errorf("completion model is required")
	}
	model, err := NewModelClient(ctx, modelKey, op.Meta{"modelKey": modelKey})
	if err != nil {
		return nil, err
	}
	canonical := model.canonicalProvider()
	if canonical == nil {
		return nil, fmt.Errorf("model %s has no canonical provider", modelKey)
	}

	maxTokens := params.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = defaultEditorCompletionMaxTokens
	}
	// Models with reasoning may consume tokens for thinking before producing text.
	// Ensure enough headroom so the text output is not starved.
	modelHasReasoning := model.config != nil && model.config.Reasoning
	if modelHasReasoning && maxTokens < 1024 {
		maxTokens = 1024
	}
	prompt := buildEditorCompletionUserPrompt(params)
	temperature := 0.0
	reasoningEffort := ""
	var reasoningEnabled *bool
	if thinkingLevel := strings.TrimSpace(params.ThinkingLevel); thinkingLevel != "" && thinkingLevel != "off" {
		reasoningEffort = resolveProviderReasoningEffort(thinkingLevel, model.config)
	}
	reasoningEnabled = resolveProviderReasoningEnabled(params.ThinkingLevel, model.config)
	resp, err := canonical.CompleteCanonical(ctx, &ai.ProviderRequest{
		Context: ai.ConversationContext{
			SystemPrompt: strings.TrimSpace(agentPrompt),
			Messages: []ai.ConversationMessage{{
				Role: ai.RoleCanonicalUser,
				Content: []ai.ContentBlock{{
					Type: ai.BlockText,
					Text: prompt,
				}},
			}},
		},
		Config: ai.GenerationConfig{
			Model:            strings.TrimSpace(model.config.ID),
			ServiceTier:      serviceTierForModelMeta(model.config, nil),
			MaxTokens:        &maxTokens,
			Temperature:      &temperature,
			ReasoningEffort:  reasoningEffort,
			ReasoningEnabled: reasoningEnabled,
		},
		RequestID: strings.TrimSpace(params.RequestID),
	})
	if err != nil {
		return nil, err
	}
	insertText := sanitizeEditorCompletionText(conversationMessageText(resp.Message), params.Suffix)
	return &op.EditorCompletionResult{
		RequestID:   strings.TrimSpace(params.RequestID),
		InsertText:  insertText,
		ReplaceFrom: params.CursorOffset,
		ReplaceTo:   params.CursorOffset,
		StopReason:  string(resp.StopReason),
		ModelKey:    modelKey,
	}, nil
}

func resolveEditorCompletionAgent(agentID string) (*op.OpNode, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID != "" {
		if node, ok := cache.GetValue[op.OpNode](agentID, cache.PrefixNode); ok {
			node = refreshFileBackedAgentNode(node)
			return &node, nil
		}
		return nil, fmt.Errorf("completion agent not found: %s", agentID)
	}
	nodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)
	for i := range nodes {
		node := nodes[i]
		if node.Kind != string(op.NodeKindAgent) {
			continue
		}
		meta, ok := node.Meta.(*op.AgentMeta)
		if !ok || meta == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(meta.Name), completionAgentID) {
			node = refreshFileBackedAgentNode(node)
			return &node, nil
		}
		if strings.EqualFold(filepath.Base(strings.TrimSpace(node.Cwd)), completionAgentID) {
			node = refreshFileBackedAgentNode(node)
			return &node, nil
		}
	}
	return nil, fmt.Errorf("completion agent not found")
}

func loadSimpleCompletionAgentPrompt(ctx context.Context, node *op.OpNode, meta op.Meta) (string, error) {
	if node == nil {
		return "", fmt.Errorf("completion agent node is nil")
	}
	if _, ok := node.Meta.(*op.AgentMeta); !ok {
		return "", fmt.Errorf("node %s is not an agent", node.ID)
	}
	conn := &Connection{}
	prompt := ""
	promptIsFinal := false
	if node.Run.HasEndpoint() {
		var err error
		conn, err = EnsureConnection(ctx, node)
		if err != nil {
			return "", err
		}
		prompt, promptIsFinal = loadPromptViaEndpoint(ctx, conn, node, meta)
	}
	if !promptIsFinal {
		loaded, err := scan.LoadPromptByURI(node.URI)
		if err != nil {
			return "", err
		}
		prompt = loaded
	}
	return expandAgentPromptVariables(prompt, node), nil
}

func buildEditorCompletionUserPrompt(params op.EditorCompletionRequest) string {
	var b strings.Builder
	writeField := func(name string, value string) {
		value = trimCompletionContext(value)
		if value == "" {
			return
		}
		b.WriteString("\n<")
		b.WriteString(name)
		b.WriteString(">\n")
		b.WriteString(value)
		b.WriteString("\n</")
		b.WriteString(name)
		b.WriteString(">\n")
	}
	b.WriteString("Continue the document at the cursor using the supplied editor context.\n")
	b.WriteString("Return only the exact text to insert at the cursor.\n")
	b.WriteString("\nMetadata:\n")
	b.WriteString("editorKind: " + strings.TrimSpace(params.EditorKind) + "\n")
	b.WriteString("languageId: " + strings.TrimSpace(params.LanguageID) + "\n")
	b.WriteString("documentName: " + completionDocumentName(params.DocumentPath) + "\n")
	b.WriteString(fmt.Sprintf("cursorOffset: %d\n", params.CursorOffset))
	writeBlock := func(name string, block *op.EditorCompletionBlock) {
		if block == nil {
			return
		}
		writeField(name, block.Text)
	}
	writeBlock("previousBlock", params.PreviousBlock)
	writeBlock("currentBlock", params.CurrentBlock)
	writeBlock("nextBlock", params.NextBlock)
	writeField("prefix", params.Prefix)
	writeField("suffix", params.Suffix)
	return b.String()
}

func completionDocumentName(path string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if trimmed == "" {
		return ""
	}
	base := filepath.Base(trimmed)
	if base == "." || base == "/" {
		return ""
	}
	return base
}

func trimCompletionContext(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if len(value) <= maxEditorCompletionContextChars {
		return value
	}
	return value[len(value)-maxEditorCompletionContextChars:]
}

func conversationMessageText(msg ai.ConversationMessage) string {
	var b strings.Builder
	for _, block := range msg.Content {
		if block.Type == ai.BlockText && block.Text != "" {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

func sanitizeEditorCompletionText(text string, suffix string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.Trim(text, "`")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	suffix = strings.ReplaceAll(suffix, "\r\n", "\n")
	if suffix != "" && strings.HasPrefix(suffix, text) {
		return ""
	}
	return text
}

func CompletionAgentIDForSystem(cfg *op.SystemConfig) string {
	if cfg == nil {
		return ""
	}
	agentPath := filepath.Join(strings.TrimSpace(cfg.BaseDir), "agents", completionAgentID, ".agent", "AGENT.md")
	node := op.BuildNode(op.LocalUser, strings.TrimSpace(cfg.HostID), op.NodeKindAgent, op.PathToURI(agentPath), strings.TrimSpace(cfg.Env), nil, op.Run{}, nil, &op.AgentMeta{})
	node.Cwd = filepath.Dir(filepath.Dir(agentPath))
	if idx, err := nodeindex.Open(strings.TrimSpace(cfg.BaseDir)); err == nil {
		_ = idx.Assign(node)
	}
	return strings.TrimSpace(node.ID)
}

func DefaultCompletionAgentID() string {
	return CompletionAgentIDForSystem(config.GetSystem())
}
