package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type agentCallOptions struct {
	ensureSession  bool
	initialPending *PendingLoopMessage
}

func executeAgentCall(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content, opts agentCallOptions) (*op.OpNodeResult, error) {
	if node == nil {
		return nil, fmt.Errorf("node is nil")
	}
	callMeta := op.Meta{}
	if meta != nil {
		callMeta = meta.Clone()
	}
	var err error
	callMeta, err = applyAgentModelMeta(callMeta, node)
	if err != nil {
		return nil, err
	}
	nodeID := normalizeThreadAgentID(node.ID)
	if strings.TrimSpace(metaString(callMeta, "agentID")) == "" {
		callMeta["agentID"] = nodeID
	}
	if opts.ensureSession {
		if err := ensureAgentCallSession(node, callMeta); err != nil {
			return nil, fmt.Errorf("ensure agent session: %w", err)
		}
	}
	if node.Run.HasEndpoint() {
		if opts.initialPending != nil {
			return nil, fmt.Errorf("queued prompt submission is not supported for endpoint agents")
		}
		conn, err := EnsureConnection(ctx, node)
		if err != nil {
			return nil, fmt.Errorf("failed to get connection: %w", err)
		}
		if conn == nil {
			return nil, fmt.Errorf("connection is nil")
		}
		agentMeta, ok := node.Meta.(*op.AgentMeta)
		if !ok || strings.TrimSpace(agentMeta.Name) == "" {
			return nil, fmt.Errorf("run agent %s missing name", node.ID)
		}
		callRes, err := conn.CallAgent(ctx, strings.TrimSpace(agentMeta.Name), callMeta, content)
		if err != nil {
			return nil, fmt.Errorf("failed to call agent: %w", err)
		}
		return &op.OpNodeResult{
			Content: callRes.Content,
			Meta:    callRes.Meta,
		}, nil
	}
	if node.Kind != string(op.NodeKindAgent) {
		return nil, fmt.Errorf("node is not an agent")
	}

	var agentLoop *AgentLoop
	if opts.initialPending != nil {
		agentLoop, err = NewQueuedPromptAgentLoop(ctx, node, callMeta, *opts.initialPending)
		if err != nil {
			return nil, fmt.Errorf("failed to create queued prompt loop: %w", err)
		}
	} else {
		agentLoop, err = NewAgentLoop(ctx, node, callMeta, content)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent loop: %w", err)
		}
	}
	return agentLoop.run()
}

func executeAgentContinue(ctx context.Context, node *op.OpNode, meta op.Meta) (*op.OpNodeResult, error) {
	if node == nil {
		return nil, fmt.Errorf("node is nil")
	}
	callMeta := op.Meta{}
	if meta != nil {
		callMeta = meta.Clone()
	}
	nodeID := normalizeThreadAgentID(node.ID)
	if strings.TrimSpace(metaString(callMeta, "agentID")) == "" {
		callMeta["agentID"] = nodeID
	}
	if node.Kind != string(op.NodeKindAgent) {
		return nil, fmt.Errorf("node is not an agent")
	}
	agentLoop, err := NewContinuationAgentLoop(ctx, node, callMeta)
	if err != nil {
		return nil, fmt.Errorf("failed to create continuation loop: %w", err)
	}
	return agentLoop.runContinuation(nil)
}

func ensureAgentCallSession(node *op.OpNode, meta op.Meta) error {
	threadID, _ := meta["threadID"].(string)
	chatPath := metaString(meta, "path")
	if chatPath == "" {
		chatPath = metaString(meta, "chatPath")
	}
	if strings.TrimSpace(threadID) == "" {
		return fmt.Errorf("threadID is required")
	}
	nodeID := normalizeThreadAgentID(node.ID)
	threadAgentID := normalizeThreadAgentID(metaString(meta, "agentID"))
	if threadAgentID == "" {
		threadAgentID = nodeID
	}
	if _, err := loadThreadContext(strings.TrimSpace(threadID), threadAgentID); err == nil {
		return nil
	} else if !isThreadNotFound(err) {
		return err
	}

	title := "Scheduled Thread"
	if metaValue, ok := node.Meta.(*op.AgentMeta); ok && metaValue != nil && strings.TrimSpace(metaValue.Name) != "" {
		title = strings.TrimSpace(metaValue.Name) + " Scheduled Thread"
	}
	_, err := createThreadWithID(op.ThreadCreateParams{
		AgentID:  threadAgentID,
		CWD:      metaString(meta, "cwd"),
		ChatPath: strings.TrimSpace(chatPath),
		Title:    title,
	}, strings.TrimSpace(threadID))
	return err
}
