package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func OpAgentHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	if req == nil || req.Params == nil {
		return nil, fmt.Errorf("agent request params are required")
	}
	switch req.Params.OpCode {
	case op.OpThreadInterrupted:
		return OpThreadInterruptedHandler(ctx, req)
	case op.OpThreadCompact:
		return OpThreadCompactHandler(ctx, req)
	case op.OpThreadSteer:
		return OpThreadSteerHandler(ctx, req)
	case op.OpThreadFollowUp:
		return OpThreadFollowUpHandler(ctx, req)
	case op.OpThreadFollowUpPromote:
		return OpThreadFollowUpPromoteHandler(ctx, req)
	case op.OpThreadQueueGet:
		return OpThreadQueueGetHandler(ctx, req)
	case op.OpThreadQueueRemove:
		return OpThreadQueueRemoveHandler(ctx, req)
	case op.OpThreadActiveList:
		return OpThreadActiveListHandler(ctx, req)
	case op.OpMessageList:
		return OpMessageListHandler(req)
	case op.OpMessageRead:
		return OpMessageReadHandler(req)
	case op.OpMessageReply:
		return OpMessageReplyHandler(req)
	case op.OpMessageAck:
		return OpMessageAckHandler(req)
	case op.OpMessageArchive:
		return OpMessageArchiveHandler(req)
	default:
		return nil, fmt.Errorf("unknown agent op code: %s", req.Params.OpCode)
	}
}

func OpThreadInterruptedHandler(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	threadMeta, err := threadControlThreadMeta(req)
	if err != nil {
		return nil, err
	}
	threadID := strings.TrimSpace(threadMeta.ThreadID)
	snapshot, err := clearQueuedMessagesInThread(*threadMeta)
	if err != nil {
		return nil, err
	}
	runtime, runtimeErr := getRuntimeLoopByThreadID(threadID)
	if runtimeErr != nil {
		if !errors.Is(runtimeErr, errRuntimeLoopNotRunning) {
			return nil, runtimeErr
		}
		return threadControlAckResult(req.Params.Meta, op.OpThreadInterrupted, threadID, snapshot, nil)
	}
	if runtime.cancel != nil {
		runtime.cancel()
	}
	unregisterRuntimeLoop(threadID, runtime)
	return threadControlAckResult(req.Params.Meta, op.OpThreadInterrupted, threadID, snapshot, nil)
}

func OpThreadCompactHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	threadMeta, err := threadControlThreadMeta(req)
	if err != nil {
		return nil, err
	}
	threadID := strings.TrimSpace(threadMeta.ThreadID)
	if runtime, runtimeErr := getRuntimeLoopByThreadID(threadID); runtimeErr == nil && runtime != nil && runtime.cancel != nil {
		runtime.cancel()
	}
	if err := compactSessionContext(ctx, *threadMeta, req.Params.Meta); err != nil {
		return nil, err
	}
	return threadControlAckResult(req.Params.Meta, op.OpThreadCompact, threadID, op.ThreadQueueSnapshot{}, nil)
}

func OpThreadSteerHandler(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	threadMeta, err := threadControlThreadMeta(req)
	if err != nil {
		return nil, err
	}
	queueMeta := queueThreadMetaForRequest(*threadMeta, req.Params.Meta)
	msg, err := decodeThreadControlUserMessage(req.Params.Content)
	if err != nil {
		return nil, err
	}
	_, snapshot, err := appendQueuedMessageToThread(
		queueMeta,
		op.ThreadQueueKindSteering,
		msg,
		metaString(req.Params.Meta, "agentName"),
		metaString(req.Params.Meta, "modelKey"),
		metaString(req.Params.Meta, "thinkingLevel"),
		metaPositiveInt64(req.Params.Meta, "contextWindow"),
		metaServiceTier(req.Params.Meta),
		selectedSkillIDsFromMeta(req.Params.Meta),
		selectedSkillContextFromMeta(req.Params.Meta),
		metaBool(req.Params.Meta, "planTurn"),
	)
	if err != nil {
		return nil, err
	}
	return threadControlAckResult(req.Params.Meta, op.OpThreadSteer, threadMeta.ThreadID, snapshot, nil)
}

func OpThreadFollowUpHandler(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	threadMeta, err := threadControlThreadMeta(req)
	if err != nil {
		return nil, err
	}
	queueMeta := queueThreadMetaForRequest(*threadMeta, req.Params.Meta)
	msg, err := decodeThreadControlUserMessage(req.Params.Content)
	if err != nil {
		return nil, err
	}
	_, snapshot, err := appendQueuedMessageToThread(
		queueMeta,
		op.ThreadQueueKindFollowUp,
		msg,
		metaString(req.Params.Meta, "agentName"),
		metaString(req.Params.Meta, "modelKey"),
		metaString(req.Params.Meta, "thinkingLevel"),
		metaPositiveInt64(req.Params.Meta, "contextWindow"),
		metaServiceTier(req.Params.Meta),
		selectedSkillIDsFromMeta(req.Params.Meta),
		selectedSkillContextFromMeta(req.Params.Meta),
		metaBool(req.Params.Meta, "planTurn"),
	)
	if err != nil {
		return nil, err
	}
	return threadControlAckResult(req.Params.Meta, op.OpThreadFollowUp, threadMeta.ThreadID, snapshot, nil)
}

func OpThreadFollowUpPromoteHandler(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	threadMeta, err := threadControlThreadMeta(req)
	if err != nil {
		return nil, err
	}
	itemID := metaString(req.Params.Meta, "itemID")
	if itemID == "" {
		return nil, fmt.Errorf("itemID is required")
	}
	snapshot, err := promoteQueuedMessageInThread(*threadMeta, itemID)
	if err != nil {
		return nil, err
	}
	return threadControlAckResult(req.Params.Meta, op.OpThreadFollowUpPromote, threadMeta.ThreadID, snapshot, nil)
}

func OpThreadQueueGetHandler(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	threadMeta, err := threadControlThreadMeta(req)
	if err != nil {
		return nil, err
	}
	snapshot, err := getQueuedMessagesSnapshot(threadMetaQuery(*threadMeta))
	if err != nil {
		return nil, err
	}
	return threadControlAckResult(req.Params.Meta, op.OpThreadQueueGet, threadMeta.ThreadID, snapshot, nil)
}

func OpThreadQueueRemoveHandler(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	threadMeta, err := threadControlThreadMeta(req)
	if err != nil {
		return nil, err
	}
	queueKind, err := metaQueueKind(req.Params.Meta)
	if err != nil {
		return nil, err
	}
	itemID := metaString(req.Params.Meta, "itemID")
	if itemID == "" {
		return nil, fmt.Errorf("itemID is required")
	}
	removedItem, snapshot, err := removeQueuedMessageFromThread(*threadMeta, queueKind, itemID)
	if err != nil {
		return nil, err
	}
	return threadControlAckResult(req.Params.Meta, op.OpThreadQueueRemove, threadMeta.ThreadID, snapshot, removedItem)
}

func OpThreadActiveListHandler(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	raw, err := json.Marshal(op.ThreadActiveList{
		Threads: listRuntimeThreads(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal active threads: %w", err)
	}
	meta := op.Meta{}
	if req != nil && req.Params != nil && req.Params.Meta != nil {
		meta = req.Params.Meta.Clone()
	}
	return &op.OpAgentResult{
		OpCode:  op.OpThreadActiveList,
		Meta:    meta,
		Content: &op.JsonContent{Raw: raw},
	}, nil
}

func getRuntimeThread(req *op.OpAgentRequest) (string, *runtimeLoop, error) {
	if req == nil || req.Params == nil || req.Params.Meta == nil {
		return "", nil, fmt.Errorf("meta is required")
	}
	threadID := metaString(req.Params.Meta, "threadID")
	if threadID == "" {
		return "", nil, fmt.Errorf("threadID is required")
	}
	runtime, err := getRuntimeLoopByThreadID(threadID)
	if err != nil {
		return "", nil, err
	}
	return threadID, runtime, nil
}

func threadControlThreadMeta(req *op.OpAgentRequest) (*op.ThreadMeta, error) {
	if req == nil || req.Params == nil || req.Params.Meta == nil {
		return nil, fmt.Errorf("meta is required")
	}
	return resolveThreadMetaFromMeta(req.Params.Meta)
}

func queueThreadMetaForRequest(threadMeta op.ThreadMeta, requestMeta op.Meta) op.ThreadMeta {
	next := threadMeta
	if agentID := normalizeThreadAgentID(metaString(requestMeta, "agentID")); agentID != "" {
		next.AgentID = agentID
	}
	if cwd := strings.TrimSpace(metaString(requestMeta, "cwd")); cwd != "" {
		next.CWD = cwd
	}
	return next
}

func metaBool(meta op.Meta, key string) bool {
	if meta == nil {
		return false
	}
	value, ok := meta[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		return trimmed == "true" || trimmed == "1" || trimmed == "yes"
	default:
		return false
	}
}

func metaQueueKind(meta op.Meta) (op.ThreadQueueKind, error) {
	queueKind := op.ThreadQueueKind(metaString(meta, "queueKind"))
	switch queueKind {
	case op.ThreadQueueKindSteering, op.ThreadQueueKindFollowUp:
		return queueKind, nil
	default:
		return "", fmt.Errorf("queueKind is required")
	}
}

func decodeThreadControlUserMessage(content op.Content) (op.Message, error) {
	if content == nil {
		return op.Message{}, fmt.Errorf("content is required")
	}
	msg, err := buildUserMessage(content)
	if err != nil {
		return op.Message{}, err
	}
	return msg, nil
}

func threadControlAckResult(meta op.Meta, opcode op.OpCode, threadID string, snapshot op.ThreadQueueSnapshot, removedItem *op.ThreadQueueItem) (*op.OpAgentResult, error) {
	raw, err := json.Marshal(op.ThreadControlAck{
		OK:             true,
		ThreadID:       strings.TrimSpace(threadID),
		OpCode:         opcode,
		QueuedMessages: snapshot,
		RemovedItem:    removedItem,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal thread control ack: %w", err)
	}
	return &op.OpAgentResult{
		OpCode:  opcode,
		Meta:    meta.Clone(),
		Content: &op.JsonContent{Raw: raw},
	}, nil
}
