package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func addMessageToolSpecs(toolSpecs map[string]*op.ToolSpec, agentMeta *op.AgentMeta) {
	if toolSpecs == nil {
		return
	}
	if shouldExposeBuiltinSystool(agentMeta, messagePublishToolName) {
		toolSpecs[messagePublishToolName] = &op.ToolSpec{
			ServerID:    systoolServerID,
			Name:        messagePublishToolName,
			Description: "Publish an asynchronous human-visible message, request, or status into the OpenBrain message system. Returns immediately; do not wait for the user through this tool.",
			InputSchema: messagePublishInputSchema(),
		}
	}
	if shouldExposeBuiltinSystool(agentMeta, messageUpdateToolName) {
		toolSpecs[messageUpdateToolName] = &op.ToolSpec{
			ServerID:    systoolServerID,
			Name:        messageUpdateToolName,
			Description: "Update one prior message-system message owned by this agent, such as marking it resolved or changing its body/actions.",
			InputSchema: messageUpdateInputSchema(),
		}
	}
	if shouldExposeBuiltinSystool(agentMeta, messageReadToolName) {
		toolSpecs[messageReadToolName] = &op.ToolSpec{
			ServerID:    systoolServerID,
			Name:        messageReadToolName,
			Description: "Read message-system history or pending user replies for a channel/thread. Use pending_only after async user replies are delivered.",
			InputSchema: messageReadInputSchema(),
		}
	}
	if shouldExposeBuiltinSystool(agentMeta, messageSubscribeToolName) {
		toolSpecs[messageSubscribeToolName] = &op.ToolSpec{
			ServerID:    systoolServerID,
			Name:        messageSubscribeToolName,
			Description: "Declare the channel this agent turn is interested in. This records routing intent and returns immediately.",
			InputSchema: messageSubscribeInputSchema(),
		}
	}
	if shouldExposeBuiltinSystool(agentMeta, messageAckToolName) {
		toolSpecs[messageAckToolName] = &op.ToolSpec{
			ServerID:    systoolServerID,
			Name:        messageAckToolName,
			Description: "Mark delivered user replies in a message-system channel as processed by this agent.",
			InputSchema: messageAckInputSchema(),
		}
	}
}

func appendMessageToolGuidance(basePrompt string, toolSpecs map[string]*op.ToolSpec) string {
	if toolSpecs == nil || toolSpecs[messagePublishToolName] == nil {
		return basePrompt
	}
	guidance := "For asynchronous Human <-> Agent conversations, use the `message_publish`, `message_read`, `message_subscribe`, and `message_ack` tools. The message system only routes messages and structured replies; you must interpret actions/questions and perform business work yourself. For request questions, set a stable `title`; user replies return that title as `requestTitle` in selected skill context. `message_publish` never waits for the user."
	trimmed := strings.TrimSpace(basePrompt)
	if strings.Contains(trimmed, guidance) {
		return trimmed
	}
	if trimmed == "" {
		return guidance
	}
	return trimmed + "\n\n" + guidance
}

func (l *AgentLoop) executeMessageHostTool(loop Loop, toolCall op.MessageToolCall, params any) (op.Message, error) {
	toolName := normalizeToolName(toolCall.Name)
	switch toolName {
	case messagePublishToolName:
		result, err := l.executeMessagePublish(loop, params)
		if err != nil {
			return op.Message{}, err
		}
		return op.NewToolResultMessage(toolName, strings.TrimSpace(toolCall.ID), marshalToolResultJSON(result)), nil
	case messageUpdateToolName:
		result, err := l.executeMessageUpdate(params)
		if err != nil {
			return op.Message{}, err
		}
		return op.NewToolResultMessage(toolName, strings.TrimSpace(toolCall.ID), marshalToolResultJSON(result)), nil
	case messageReadToolName:
		result, err := l.executeMessageRead(loop, params)
		if err != nil {
			return op.Message{}, err
		}
		return op.NewToolResultMessage(toolName, strings.TrimSpace(toolCall.ID), marshalToolResultJSON(result)), nil
	case messageSubscribeToolName:
		result, err := l.executeMessageSubscribe(loop, params)
		if err != nil {
			return op.Message{}, err
		}
		return op.NewToolResultMessage(toolName, strings.TrimSpace(toolCall.ID), marshalToolResultJSON(result)), nil
	case messageAckToolName:
		result, err := l.executeMessageAck(loop, params)
		if err != nil {
			return op.Message{}, err
		}
		return op.NewToolResultMessage(toolName, strings.TrimSpace(toolCall.ID), marshalToolResultJSON(result)), nil
	default:
		return op.Message{}, fmt.Errorf("unknown message host tool: %s", toolCall.Name)
	}
}

func (l *AgentLoop) executeMessagePublish(loop Loop, params any) (op.MessagePublishResult, error) {
	var input op.MessagePublishParams
	if err := decodeMessageToolParams(params, &input); err != nil {
		return op.MessagePublishResult{}, err
	}
	if err := validateMessagePublishInput(input); err != nil {
		return op.MessagePublishResult{}, err
	}
	threadID, agentID, channelID := l.resolveMessageAddress(loop, input.ThreadID, input.AgentID, input.ChannelID)
	record, err := defaultMessageStore.appendRecord(op.MessageSenderAgent, op.MessageRecord{
		ChannelID: channelID,
		ThreadID:  threadID,
		AgentID:   agentID,
		Kind:      input.Kind,
		Status:    op.MessageStatusOpen,
		Title:     input.Title,
		Body:      input.Body,
		Actions:   input.Actions,
		Questions: input.Questions,
		Meta:      input.Meta,
	})
	if err != nil {
		return op.MessagePublishResult{}, err
	}
	delivered := notifyMessageRecord(loop.Meta, record)
	return op.MessagePublishResult{
		MessageID: strings.TrimSpace(record.ID),
		ChannelID: strings.TrimSpace(record.ChannelID),
		ThreadID:  strings.TrimSpace(record.ThreadID),
		Delivered: delivered,
	}, nil
}

func (l *AgentLoop) executeMessageUpdate(params any) (op.MessageRecord, error) {
	var input op.MessageUpdateParams
	if err := decodeMessageToolParams(params, &input); err != nil {
		return op.MessageRecord{}, err
	}
	record, err := defaultMessageStore.update(input)
	if err != nil {
		return op.MessageRecord{}, err
	}
	_ = notifyMessageRecord(op.Meta{
		"threadID": record.ThreadID,
		"agentID":  record.AgentID,
	}, record)
	return record, nil
}

func (l *AgentLoop) executeMessageRead(loop Loop, params any) (op.MessageReadResult, error) {
	var input op.MessageReadParams
	if err := decodeMessageToolParams(params, &input); err != nil {
		return op.MessageReadResult{}, err
	}
	threadID, agentID, _ := l.resolveMessageAddress(loop, input.ThreadID, input.AgentID, input.ChannelID)
	input.ThreadID = threadID
	input.AgentID = agentID
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	return defaultMessageStore.read(input)
}

func (l *AgentLoop) executeMessageSubscribe(loop Loop, params any) (op.MessageSubscribeResult, error) {
	var input op.MessageSubscribeParams
	if err := decodeMessageToolParams(params, &input); err != nil {
		return op.MessageSubscribeResult{}, err
	}
	threadID, agentID, channelID := l.resolveMessageAddress(loop, input.ThreadID, input.AgentID, input.ChannelID)
	return op.MessageSubscribeResult{
		ChannelID:  channelID,
		ThreadID:   threadID,
		AgentID:    agentID,
		Subscribed: true,
	}, nil
}

func (l *AgentLoop) executeMessageAck(loop Loop, params any) (op.MessageAckResult, error) {
	var input op.MessageAckParams
	if err := decodeMessageToolParams(params, &input); err != nil {
		return op.MessageAckResult{}, err
	}
	requestedChannelID := strings.TrimSpace(input.ChannelID)
	threadID, agentID, channelID := l.resolveMessageAddress(loop, input.ThreadID, input.AgentID, input.ChannelID)
	input.ThreadID = threadID
	input.AgentID = agentID
	input.ChannelID = requestedChannelID
	if requestedChannelID != "" {
		input.ChannelID = channelID
	}
	return defaultMessageStore.ack(input)
}

func (l *AgentLoop) resolveMessageAddress(loop Loop, threadID, agentID, channelID string) (string, string, string) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		threadID = strings.TrimSpace(loop.ThreadID)
	}
	if threadID == "" && loop.Meta != nil {
		threadID = metaString(loop.Meta, "threadID")
	}
	if threadID == "" && l != nil {
		threadID = strings.TrimSpace(l.ThreadID)
	}

	agentID = strings.TrimSpace(agentID)
	if agentID == "" && l != nil && l.Agent != nil {
		agentID = strings.TrimSpace(l.Agent.AgentID)
	}
	if agentID == "" && loop.Meta != nil {
		agentID = metaString(loop.Meta, "agentID")
	}

	channelID = strings.TrimSpace(channelID)
	if channelID == "" && loop.Meta != nil {
		channelID = metaString(loop.Meta, "channelID")
	}
	if channelID == "" {
		channelID = defaultMessageChannelID(threadID, agentID)
	}
	return threadID, agentID, channelID
}

func validateMessagePublishInput(input op.MessagePublishParams) error {
	kind := input.Kind
	if kind == "" {
		kind = op.MessageKindMessage
	}
	if kind != op.MessageKindRequest {
		return nil
	}
	if len(input.Actions) > 0 {
		return fmt.Errorf("kind=request does not support actions; use questions[]")
	}
	questions := normalizeMessageQuestions(input.Questions)
	if len(questions) == 0 {
		return fmt.Errorf("kind=request requires exactly one question")
	}
	if len(questions) != 1 {
		return fmt.Errorf("kind=request supports exactly one question in v1")
	}
	return nil
}

func decodeMessageToolParams(params any, out any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal message tool arguments: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("parse message tool arguments: %w", err)
	}
	return nil
}

func notifyMessageRecord(baseMeta op.Meta, record op.MessageRecord) bool {
	raw, err := json.Marshal(record)
	if err != nil {
		return false
	}
	meta := op.Meta{}
	if baseMeta != nil {
		meta = baseMeta.Clone()
	}
	meta["type"] = "message"
	meta["messageID"] = strings.TrimSpace(record.ID)
	meta["channelID"] = strings.TrimSpace(record.ChannelID)
	meta["threadID"] = strings.TrimSpace(record.ThreadID)
	meta["agentID"] = strings.TrimSpace(record.AgentID)
	NotifyProgress(op.NotifyMessage, meta, &op.JsonContent{Raw: raw})
	return true
}

func messagePublishInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"body"},
		"properties": map[string]any{
			"channelID": messageStringSchema("Optional explicit channel id for parallel agent conversations."),
			"threadID":  messageStringSchema("Optional thread id. Defaults to the current thread."),
			"agentID":   messageStringSchema("Optional agent id. Defaults to the current agent."),
			"kind":      messageEnumSchema([]any{"message", "request", "status"}, "Message kind."),
			"title":     messageStringSchema("Optional short title for UI projection and requestTitle follow-up context."),
			"body":      messageStringSchema("Message body visible to the user."),
			"actions":   messageActionsSchema(),
			"questions": messageQuestionsSchema(),
			"meta":      map[string]any{"type": "object", "additionalProperties": true},
		},
	}
}

func messageUpdateInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"messageID"},
		"properties": map[string]any{
			"messageID": messageStringSchema("Message id to update."),
			"body":      messageStringSchema("Replacement body."),
			"title":     messageStringSchema("Replacement title."),
			"status":    messageEnumSchema([]any{"open", "resolved", "archived"}, "Updated message status."),
			"actions":   messageActionsSchema(),
			"questions": messageQuestionsSchema(),
			"meta":      map[string]any{"type": "object", "additionalProperties": true},
		},
	}
}

func messageReadInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"channelID":   messageStringSchema("Optional channel id."),
			"threadID":    messageStringSchema("Optional thread id. Defaults to current thread."),
			"agentID":     messageStringSchema("Optional agent id. Defaults to current agent."),
			"pendingOnly": map[string]any{"type": "boolean", "description": "Only return pending user replies."},
			"limit":       map[string]any{"type": "integer", "minimum": 1, "maximum": 500},
		},
	}
}

func messageSubscribeInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"channelID": messageStringSchema("Optional channel id."),
			"threadID":  messageStringSchema("Optional thread id. Defaults to current thread."),
			"agentID":   messageStringSchema("Optional agent id. Defaults to current agent."),
		},
	}
}

func messageAckInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"channelID":  messageStringSchema("Optional channel id."),
			"threadID":   messageStringSchema("Optional thread id. Defaults to current thread."),
			"agentID":    messageStringSchema("Optional agent id. Defaults to current agent."),
			"messageIDs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
}

func messageStringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func messageEnumSchema(values []any, description string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": description}
}

func messageActionsSchema() map[string]any {
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"id", "label"},
			"properties": map[string]any{
				"id":    messageStringSchema("Stable action id returned as actionID when user clicks it."),
				"label": messageStringSchema("Human-readable action label."),
				"tone":  messageEnumSchema([]any{"primary", "danger"}, "Optional action tone."),
			},
		},
	}
}

func messageQuestionsSchema() map[string]any {
	optionSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"id", "label"},
		"properties": map[string]any{
			"id":    messageStringSchema("Stable option id returned in answers[].optionID when the user chooses it."),
			"label": messageStringSchema("Human-readable option label."),
		},
	}
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"id", "question", "options"},
			"properties": map[string]any{
				"id":       messageStringSchema("Stable question id used to map user answers."),
				"question": messageStringSchema("Single question shown to the user."),
				"options":  map[string]any{"type": "array", "items": optionSchema},
			},
		},
	}
}
