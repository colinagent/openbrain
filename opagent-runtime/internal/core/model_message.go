package core

import (
	"log/slog"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func publishModelConfigMessage(meta op.Meta, title string, body string, reason string, fields op.Meta) {
	threadID := strings.TrimSpace(metaString(meta, "threadID"))
	agentID := strings.TrimSpace(metaString(meta, "agentID"))
	if threadID == "" || agentID == "" {
		return
	}
	messageMeta := op.Meta{
		"reason": strings.TrimSpace(reason),
	}
	for key, value := range fields {
		messageMeta[key] = value
	}
	record, err := defaultMessageStore.appendRecord(op.MessageSenderSystem, op.MessageRecord{
		ThreadID: threadID,
		AgentID:  agentID,
		Kind:     op.MessageKindRequest,
		Status:   op.MessageStatusOpen,
		Title:    strings.TrimSpace(title),
		Body:     strings.TrimSpace(body),
		Meta:     messageMeta,
	})
	if err != nil {
		slog.Warn("failed to publish model configuration message", "error", err, "agentID", agentID, "threadID", threadID)
		return
	}
	_ = notifyMessageRecord(meta, record)
}

func publishAgentFrontmatterModelMessage(meta op.Meta, agentID string, modelKey string, detail string) {
	modelKey = strings.TrimSpace(modelKey)
	body := "This agent cannot start because its AGENT.md frontmatter model is not available. Update the agent model field or enable/configure that model in Models settings, then retry.\n\nAGENT.md model: " + modelKey
	if strings.TrimSpace(detail) != "" {
		body += "\n\nReason: " + strings.TrimSpace(detail)
	}
	publishModelConfigMessage(meta, "Agent model needs configuration", body, "agent_frontmatter_model_unavailable", op.Meta{
		"agentID":  strings.TrimSpace(agentID),
		"modelKey": modelKey,
	})
}
