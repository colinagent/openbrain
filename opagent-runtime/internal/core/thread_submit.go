package core

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func OpThreadSubmitHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if req == nil || req.Params == nil {
		return nil, fmt.Errorf("params are required")
	}
	if req.Params.Meta == nil {
		return nil, fmt.Errorf("meta is required")
	}

	nodeID := strings.TrimSpace(metaString(req.Params.Meta, "agentID"))
	if nodeID == "" {
		return nil, fmt.Errorf("meta.agentID is required")
	}

	node, err := resolveThreadSubmitAgentNode(nodeID)
	if err != nil {
		return nil, err
	}

	result, err := executeThreadSubmit(ctx, node, req.Params.Meta, req.Params.Content)
	if err != nil {
		slog.Error("failed to execute thread submit", "error", err, "nodeID", nodeID)
		return nil, err
	}
	return result, nil
}

func resolveThreadSubmitAgentNode(nodeID string) (*op.OpNode, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("meta.agentID is required")
	}
	nodeVal, ok := cache.GetValue[op.OpNode](nodeID, cache.PrefixNode)
	if !ok {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}
	nodeVal = refreshFileBackedAgentNode(nodeVal)
	node := &nodeVal
	if err := requireThreadSubmitCapability(node); err != nil {
		return nil, err
	}
	return node, nil
}

func requireThreadSubmitCapability(node *op.OpNode) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}
	if strings.TrimSpace(node.Kind) != string(op.NodeKindAgent) {
		return fmt.Errorf("node is not an agent")
	}
	for _, code := range node.OpCodes {
		if strings.TrimSpace(string(code)) == string(op.OpThreadSubmit) {
			return nil
		}
	}
	return fmt.Errorf("agent %s is not chat-capable: missing %s opcode", strings.TrimSpace(node.ID), op.OpThreadSubmit)
}

func executeThreadSubmit(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
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

	threadID := strings.TrimSpace(metaString(callMeta, "threadID"))
	if threadID == "" {
		return nil, fmt.Errorf("threadID is required")
	}

	threadMeta, err := resolveThreadMetaFromMeta(callMeta)
	if err != nil && nodeID != "" {
		callMeta["agentID"] = nodeID
		threadMeta, err = resolveThreadMetaFromMeta(callMeta)
	}
	if err != nil {
		return nil, fmt.Errorf("load thread thread meta: %w", err)
	}
	callMeta = applyResolvedThreadMetaToMeta(callMeta, *threadMeta)
	snapshot, err := getThreadSnapshot(threadMetaQuery(*threadMeta))
	if err != nil {
		return nil, fmt.Errorf("load thread snapshot: %w", err)
	}
	if snapshot == nil {
		return nil, fmt.Errorf("thread snapshot is nil")
	}
	if snapshot.RunStatus == op.ThreadRunRunning {
		return nil, fmt.Errorf("thread is already running; use thread/steer, thread/follow_up, or thread/interrupted")
	}
	hasContent := content != nil
	switch snapshot.TailStatus {
	case op.ThreadTailEmpty:
		if hasContent {
			return executeAgentCall(ctx, node, callMeta, content, agentCallOptions{})
		}
		return executeThreadSubmitQueuedPrompt(ctx, node, callMeta, threadMeta, snapshot.QueuedMessages)
	case op.ThreadTailComplete:
		if hasContent {
			return executeAgentCall(ctx, node, callMeta, content, agentCallOptions{})
		}
		return executeThreadSubmitQueuedPrompt(ctx, node, callMeta, threadMeta, snapshot.QueuedMessages)
	case op.ThreadTailNeedsContinuation:
		if !hasContent {
			if item, _ := peekNextQueuedMessageForSubmit(snapshot.QueuedMessages); item != nil {
				return executeThreadSubmitQueuedPrompt(ctx, node, callMeta, threadMeta, snapshot.QueuedMessages)
			}
		}
		if hasContent {
			msg, err := decodeThreadControlUserMessage(content)
			if err != nil {
				return nil, err
			}
			queueMeta := queueThreadMetaForRequest(*threadMeta, callMeta)
			if _, _, err := appendQueuedMessageToThread(
				queueMeta,
				op.ThreadQueueKindFollowUp,
				msg,
				metaString(callMeta, "agentName"),
				metaString(callMeta, "modelKey"),
				metaString(callMeta, "thinkingLevel"),
				metaPositiveInt64(callMeta, "contextWindow"),
				metaServiceTier(callMeta),
				selectedSkillIDsFromMeta(callMeta),
				selectedSkillContextFromMeta(callMeta),
				metaBool(callMeta, "planTurn"),
			); err != nil {
				return nil, fmt.Errorf("enqueue follow-up before continuation: %w", err)
			}
		}
		return executeAgentContinue(ctx, node, clearSubmitOnlyMeta(callMeta))
	default:
		if hasContent {
			return executeAgentCall(ctx, node, callMeta, content, agentCallOptions{})
		}
		return nil, fmt.Errorf("no submission content or queued work available for thread")
	}
}

func executeThreadSubmitQueuedPrompt(
	ctx context.Context,
	node *op.OpNode,
	meta op.Meta,
	threadMeta *op.ThreadMeta,
	queued op.ThreadQueueSnapshot,
) (*op.OpNodeResult, error) {
	if threadMeta == nil {
		return nil, fmt.Errorf("thread meta is required")
	}
	item, queueKind := peekNextQueuedMessageForSubmit(queued)
	if item == nil {
		return nil, fmt.Errorf("no queued work available for thread")
	}
	callMeta := meta.Clone()
	if item.AgentID != "" {
		callMeta["agentID"] = item.AgentID
		if queuedNode, err := resolveThreadSubmitAgentNode(item.AgentID); err == nil {
			node = queuedNode
		} else {
			return nil, err
		}
	}
	if item.CWD != "" {
		callMeta["cwd"] = item.CWD
	}
	if item.AgentName != "" {
		callMeta["agentName"] = item.AgentName
	}
	if item.ModelKey != "" {
		callMeta["modelKey"] = item.ModelKey
	}
	if item.ThinkingLevel != "" {
		callMeta["thinkingLevel"] = item.ThinkingLevel
	}
	if item.ContextWindow > 0 {
		callMeta["contextWindow"] = item.ContextWindow
	}
	if item.ServiceTier != "" {
		callMeta["serviceTier"] = item.ServiceTier
	}
	callMeta["selectedSkillIDs"] = append([]string(nil), item.SelectedSkillIDs...)
	callMeta["selectedSkillContext"] = item.SelectedSkillContext.Clone()
	callMeta["planTurn"] = item.PlanTurn
	pending := pendingLoopMessageFromQueueItem(*item, queueKind)
	slog.Info("thread submit selected queued prompt",
		"threadID", threadMeta.ThreadID,
		"queueKind", queueKind,
		"queueItemID", item.ID,
	)
	if node.Run.HasEndpoint() {
		if err := appendQueueDeliveryToThread(*threadMeta, []PendingLoopMessage{pending}); err != nil {
			return nil, fmt.Errorf("append queued endpoint user message to session: %w", err)
		}
		emitQueuedMessageDelivered(callMeta, string(pending.QueueKind), pending.Message, pending.QueueItemID)
		return executeAgentContinue(ctx, node, callMeta)
	}
	return executeAgentCall(ctx, node, callMeta, nil, agentCallOptions{initialPending: &pending})
}

func peekNextQueuedMessageForSubmit(snapshot op.ThreadQueueSnapshot) (*op.ThreadQueueItem, op.ThreadQueueKind) {
	if len(snapshot.Steering) > 0 {
		item := cloneThreadQueueItem(snapshot.Steering[0])
		return &item, op.ThreadQueueKindSteering
	}
	if len(snapshot.FollowUp) > 0 {
		item := cloneThreadQueueItem(snapshot.FollowUp[0])
		return &item, op.ThreadQueueKindFollowUp
	}
	return nil, ""
}

func clearSubmitOnlyMeta(meta op.Meta) op.Meta {
	cloned := meta.Clone()
	delete(cloned, "selectedSkillIDs")
	delete(cloned, "selectedSkillContext")
	delete(cloned, "planTurn")
	return cloned
}
