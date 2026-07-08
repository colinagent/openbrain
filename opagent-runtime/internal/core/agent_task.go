package core

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/builtintools"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

const (
	agentTaskToolName = "agent_task"
	systoolServerID   = builtintools.ServerID
)

type agentTaskArgs struct {
	SubagentID string `json:"subagent_id"`
	Task       string `json:"task"`
	Context    string `json:"context,omitempty"`
}

type agentTaskResult struct {
	SubagentID   string `json:"subagent_id"`
	SubagentName string `json:"subagent_name,omitempty"`
	ThreadID     string `json:"thread_id"`
	ResultText   string `json:"result_text"`
}

func isBuiltinSystoolSpec(spec *op.ToolSpec) bool {
	if spec == nil {
		return false
	}
	return strings.TrimSpace(spec.ServerID) == systoolServerID
}

func (l *AgentLoop) executeBuiltinSystoolCall(loop Loop, toolCall op.MessageToolCall, params any) (op.Message, error) {
	toolName := normalizeToolName(toolCall.Name)
	if builtintools.IsOSToolName(toolName) {
		result, err := builtintools.Execute(loop.Ctx, toolName, params, loop.Meta, func(_ context.Context, notification *op.InfoNotificationParams) {
			if notification == nil {
				return
			}
			NotifyProgress(op.NotifyMessage, notification.Meta, notification.Content)
		})
		if err != nil {
			return op.Message{}, err
		}
		text, callResult, err := extractToolResultText(result)
		if err != nil {
			return op.Message{}, err
		}
		return toolResultMessageFromCallResult(toolName, strings.TrimSpace(toolCall.ID), text, callResult), nil
	}
	switch toolName {
	case agentTaskToolName:
		return l.executeAgentTaskTool(loop, toolCall, params)
	case messagePublishToolName, messageUpdateToolName, messageReadToolName, messageSubscribeToolName, messageAckToolName:
		return l.executeMessageHostTool(loop, toolCall, params)
	default:
		return op.Message{}, fmt.Errorf("unknown built-in systool: %s", toolCall.Name)
	}
}

func (l *AgentLoop) executeAgentTaskTool(loop Loop, toolCall op.MessageToolCall, params any) (op.Message, error) {
	args, err := parseAgentTaskArgs(params)
	if err != nil {
		return op.Message{}, err
	}
	target, err := l.resolveMountedSubagent(args.SubagentID)
	if err != nil {
		return op.Message{}, err
	}
	if strings.TrimSpace(target.ID) == strings.TrimSpace(l.Agent.AgentID) {
		return op.Message{}, fmt.Errorf("subagent cannot target the current agent: %s", target.ID)
	}

	parentMeta := l.threadMeta
	if strings.TrimSpace(parentMeta.ThreadID) == "" {
		resolved, resolveErr := resolveThreadMetaFromMeta(loop.Meta)
		if resolveErr != nil {
			return op.Message{}, fmt.Errorf("resolve parent session: %w", resolveErr)
		}
		parentMeta = *resolved
	}

	childThreadID := op.GenerateThreadID()
	childChatPath := agentTaskChildChatPath(parentMeta, loop.Workdir, strings.TrimSpace(target.ID), childThreadID)
	childTitle := agentTaskChildTitle(&target, args.Task)
	childCWD := strings.TrimSpace(parentMeta.CWD)
	if childCWD == "" {
		childCWD = strings.TrimSpace(loop.Workdir)
	}
	created, err := createThreadWithID(op.ThreadCreateParams{
		AgentID:        strings.TrimSpace(target.ID),
		CWD:            childCWD,
		ChatPath:       childChatPath,
		Title:          childTitle,
		ParentThreadID: strings.TrimSpace(parentMeta.ThreadID),
	}, childThreadID)
	if err != nil {
		return op.Message{}, fmt.Errorf("create subagent session: %w", err)
	}
	resolvedChildCWD := strings.TrimSpace(created.CWD)
	if resolvedChildCWD == "" {
		resolvedChildCWD = childCWD
	}

	childMeta := op.Meta{
		"agentID":        strings.TrimSpace(target.ID),
		"threadID":       strings.TrimSpace(created.ThreadID),
		"chatPath":       strings.TrimSpace(created.ChatPath),
		"path":           strings.TrimSpace(created.ChatPath),
		"title":          strings.TrimSpace(created.Title),
		"cwd":            resolvedChildCWD,
		"parentAgentID":  strings.TrimSpace(l.Agent.AgentID),
		"parentThreadID": strings.TrimSpace(parentMeta.ThreadID),
		"parentTurnID":   strings.TrimSpace(loop.TurnID),
		"parentChatPath": resolvedThreadPath(parentMeta),
	}
	childMeta, err = resolveAgentTaskModelMeta(childMeta, &target, loop.Meta)
	if err != nil {
		return op.Message{}, err
	}
	content := &op.TextContent{Text: agentTaskPrompt(args, parentContext{
		ParentAgentID:     strings.TrimSpace(l.Agent.AgentID),
		ParentThreadID:    strings.TrimSpace(parentMeta.ThreadID),
		ParentTurnID:      strings.TrimSpace(loop.TurnID),
		ParentChatPath:    resolvedThreadPath(parentMeta),
		ParentCWD:         strings.TrimSpace(parentMeta.CWD),
		ParentWorkdir:     strings.TrimSpace(loop.Workdir),
		ChildThreadID:     strings.TrimSpace(created.ThreadID),
		ChildChatPath:     strings.TrimSpace(created.ChatPath),
		SubagentID:        strings.TrimSpace(target.ID),
		SubagentName:      subagentDisplayName(&target),
		SubagentRoot:      agentRootFromURI(target.URI),
		SubagentHome:      agentHomeFromURI(target.URI),
		SubagentFile:      strings.TrimSpace(op.URIToPath(target.URI)),
		EffectiveChildCWD: resolvedChildCWD,
	})}
	result, err := executeAgentCall(loop.Ctx, &target, childMeta, content, agentCallOptions{})
	if err != nil {
		return op.Message{}, fmt.Errorf("run subagent %s: %w", strings.TrimSpace(target.ID), err)
	}

	payload := agentTaskResult{
		SubagentID:   strings.TrimSpace(target.ID),
		SubagentName: subagentDisplayName(&target),
		ThreadID:     strings.TrimSpace(created.ThreadID),
		ResultText:   resultTextFromOpNodeResult(result),
	}
	return op.NewToolResultMessage(agentTaskToolName, strings.TrimSpace(toolCall.ID), marshalToolResultJSON(payload)), nil
}

func resolveAgentTaskModelMeta(childMeta op.Meta, target *op.OpNode, parentMeta op.Meta) (op.Meta, error) {
	next := op.Meta{}
	if childMeta != nil {
		next = childMeta.Clone()
	}
	if strings.TrimSpace(metaString(next, "modelKey")) != "" {
		delete(next, "model")
		return next, nil
	}
	agentID := ""
	agentModel := ""
	if target != nil {
		agentID = strings.TrimSpace(target.ID)
		if agentMeta, ok := target.Meta.(*op.AgentMeta); ok && agentMeta != nil {
			agentModel = strings.TrimSpace(agentMeta.Model)
		}
	}
	if agentModel != "" {
		if _, err := config.GetModelConfig(agentModel); err != nil {
			publishAgentFrontmatterModelMessage(next, agentID, agentModel, err.Error())
			return nil, fmt.Errorf("subagent model %q is not available in local models.json: %w", agentModel, err)
		}
		next["modelKey"] = agentModel
		delete(next, "model")
		return next, nil
	}
	if modelKey := strings.TrimSpace(metaString(parentMeta, "modelKey")); modelKey != "" {
		next["modelKey"] = modelKey
		delete(next, "model")
		return next, nil
	}
	delete(next, "model")
	return nil, fmt.Errorf("subagent %s requires an explicit modelKey from AGENT.md or parent turn", strings.TrimSpace(agentID))
}

func parseAgentTaskArgs(params any) (agentTaskArgs, error) {
	var args agentTaskArgs
	raw, err := json.Marshal(params)
	if err != nil {
		return args, fmt.Errorf("marshal agent_task arguments: %w", err)
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return args, fmt.Errorf("parse agent_task arguments: %w", err)
	}
	args.SubagentID = normalizeAgentTaskSubagentID(args.SubagentID)
	args.Task = strings.TrimSpace(args.Task)
	args.Context = strings.TrimSpace(args.Context)
	if args.SubagentID == "" {
		return args, fmt.Errorf("agent_task.subagent_id is required")
	}
	if args.Task == "" {
		return args, fmt.Errorf("agent_task.task is required")
	}
	return args, nil
}

func (l *AgentLoop) resolveMountedSubagent(subagentID string) (op.OpNode, error) {
	normalized := normalizeAgentTaskSubagentID(subagentID)
	if l == nil || l.Agent == nil {
		return op.OpNode{}, fmt.Errorf("agent loop is not initialized")
	}
	for _, candidate := range l.Agent.AvailableSubagents {
		if normalizeAgentTaskSubagentID(candidate.ID) == normalized {
			return refreshFileBackedAgentNode(candidate), nil
		}
	}
	return op.OpNode{}, fmt.Errorf("subagent %s is not mounted or is not thread-submit capable", normalized)
}

func normalizeAgentTaskSubagentID(value string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "@"))
}

type parentContext struct {
	ParentAgentID     string
	ParentThreadID    string
	ParentTurnID      string
	ParentChatPath    string
	ParentCWD         string
	ParentWorkdir     string
	ChildThreadID     string
	ChildChatPath     string
	SubagentID        string
	SubagentName      string
	SubagentRoot      string
	SubagentHome      string
	SubagentFile      string
	EffectiveChildCWD string
}

func agentTaskPrompt(args agentTaskArgs, ctx parentContext) string {
	lines := []string{
		"Task:",
		args.Task,
	}
	if args.Context == "" {
		lines = append(lines, "")
	} else {
		lines = append(lines, "", "Context:", args.Context, "")
	}
	lines = append(lines, "Parent thread context:")
	for _, field := range []struct {
		name  string
		value string
	}{
		{"parentAgentID", ctx.ParentAgentID},
		{"parentThreadID", ctx.ParentThreadID},
		{"parentTurnID", ctx.ParentTurnID},
		{"parentChatPath", ctx.ParentChatPath},
		{"parentCWD", ctx.ParentCWD},
		{"parentWorkdir", ctx.ParentWorkdir},
		{"childThreadID", ctx.ChildThreadID},
		{"childChatPath", ctx.ChildChatPath},
		{"subagentID", ctx.SubagentID},
		{"subagentName", ctx.SubagentName},
		{"subagentAgentFile", ctx.SubagentFile},
		{"subagentAgentRoot", ctx.SubagentRoot},
		{"subagentAgentHome", ctx.SubagentHome},
		{"effectiveChildCWD", ctx.EffectiveChildCWD},
	} {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		lines = append(lines, "- "+field.name+": "+field.value)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func agentTaskChildTitle(target *op.OpNode, task string) string {
	name := subagentDisplayName(target)
	if name == "" {
		name = "Subagent"
	}
	task = strings.TrimSpace(task)
	if task == "" {
		return name + " task"
	}
	taskRunes := []rune(task)
	if len(taskRunes) > 60 {
		task = strings.TrimSpace(string(taskRunes[:60]))
	}
	return name + ": " + task
}

func agentTaskChildChatPath(parent op.ThreadMeta, workdir, subagentID, childThreadID string) string {
	parentPath := resolvedThreadPath(parent)
	baseDir := ""
	if parentPath != "" {
		baseDir = filepath.Dir(parentPath)
	}
	if baseDir == "" || baseDir == "." {
		baseDir = filepath.Join(strings.TrimSpace(workdir), ".agent", "chat")
	}
	if baseDir == "" || baseDir == "." {
		baseDir = filepath.Join(strings.TrimSpace(parent.CWD), ".agent", "chat")
	}
	parentID := safePathComponent(parent.ThreadID)
	if parentID == "" {
		parentID = "parent"
	}
	subagent := safePathComponent(subagentID)
	if subagent == "" {
		subagent = "subagent"
	}
	threadID := safePathComponent(childThreadID)
	if threadID == "" {
		threadID = "thread"
	}
	return filepath.Join(baseDir, ".subagents", parentID, subagent, threadID+".md")
}

func safePathComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

func resultTextFromOpNodeResult(result *op.OpNodeResult) string {
	if result == nil || result.Content == nil {
		return ""
	}
	switch content := result.Content.(type) {
	case *op.TextContent:
		return strings.TrimSpace(content.Text)
	case *op.JsonContent:
		if text := assistantTextFromMessagesJSON(content.Raw); text != "" {
			return text
		}
		return strings.TrimSpace(string(content.Raw))
	default:
		raw, err := json.Marshal(content)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(raw))
	}
}

func assistantTextFromMessagesJSON(raw json.RawMessage) string {
	var messages []op.Message
	if err := json.Unmarshal(raw, &messages); err != nil {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != op.RoleAssistant {
			continue
		}
		if text := strings.TrimSpace(msg.Content); text != "" {
			return text
		}
		for _, part := range msg.ContentParts {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				return strings.TrimSpace(part.Text)
			}
		}
	}
	return ""
}
