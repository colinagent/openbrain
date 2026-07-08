package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

type messengerReplyRequest struct {
	op.MessageReplyParams
	ModelKey      string `json:"modelKey,omitempty"`
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
	ContextWindow int64  `json:"contextWindow,omitempty"`
	ServiceTier   string `json:"serviceTier,omitempty"`
}

func (h *Handler) handleMessengerList(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p op.MessageListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, invalidParams(err)
	}
	var out op.MessageListResult
	if err := h.callMessengerOp(op.OpMessageList, p, &out); err != nil {
		return nil, internalRPCError(err)
	}
	return out, nil
}

func (h *Handler) handleMessengerChannel(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p op.MessageReadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, invalidParams(err)
	}
	var out op.MessageReadResult
	if err := h.callMessengerOp(op.OpMessageRead, p, &out); err != nil {
		return nil, internalRPCError(err)
	}
	return out, nil
}

func (h *Handler) handleMessengerReply(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p messengerReplyRequest
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, invalidParams(err)
	}
	var out op.MessageReplyResult
	if err := h.callMessengerOp(op.OpMessageReply, p.MessageReplyParams, &out, messengerReplyRuntimeMeta(p)); err != nil {
		return nil, internalRPCError(err)
	}
	h.dispatchMessengerReplyAsync(out.Dispatch)
	out.Dispatch = nil
	out.Queue = nil
	return out, nil
}

func (h *Handler) handleMessengerMarkRead(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p op.MessageAckParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, invalidParams(err)
	}
	var out op.MessageAckResult
	if err := h.callMessengerOp(op.OpMessageAck, p, &out); err != nil {
		return nil, internalRPCError(err)
	}
	return out, nil
}

func (h *Handler) handleMessengerArchive(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p op.MessageArchiveParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, invalidParams(err)
	}
	var out op.MessageArchiveResult
	if err := h.callMessengerOp(op.OpMessageArchive, p, &out); err != nil {
		return nil, internalRPCError(err)
	}
	return out, nil
}

func messengerReplyRuntimeMeta(input messengerReplyRequest) op.Meta {
	meta := op.Meta{}
	if modelKey := strings.TrimSpace(input.ModelKey); modelKey != "" {
		meta["modelKey"] = modelKey
	}
	if thinkingLevel := strings.TrimSpace(input.ThinkingLevel); thinkingLevel != "" {
		meta["thinkingLevel"] = thinkingLevel
	}
	if input.ContextWindow > 0 {
		meta["contextWindow"] = input.ContextWindow
	}
	if serviceTier := strings.TrimSpace(input.ServiceTier); serviceTier != "" {
		meta["serviceTier"] = serviceTier
	}
	return meta
}

func (h *Handler) callMessengerOp(opcode op.OpCode, input any, out any, metaArgs ...op.Meta) error {
	if h == nil || h.server == nil {
		return fmt.Errorf("server is not initialized")
	}
	session := h.server.GetHostSession()
	if session == nil {
		return fmt.Errorf("host session not initialized")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	meta := op.Meta{}
	if len(metaArgs) > 0 && metaArgs[0] != nil {
		meta = metaArgs[0].Clone()
	}
	res, err := session.OpAgent(context.Background(), &op.OpAgentParams{
		OpCode:  opcode,
		Meta:    meta,
		Content: &op.JsonContent{Raw: raw},
	})
	if err != nil {
		return err
	}
	if res == nil || res.Content == nil {
		return fmt.Errorf("%s returned empty response", opcode)
	}
	jsonContent, ok := res.Content.(*op.JsonContent)
	if !ok {
		return fmt.Errorf("%s returned invalid content: %T", opcode, res.Content)
	}
	if err := json.Unmarshal(jsonContent.Raw, out); err != nil {
		return fmt.Errorf("decode %s response: %w", opcode, err)
	}
	return nil
}

func (h *Handler) dispatchMessengerReplyAsync(dispatch *op.MessageReplyDispatch) {
	if dispatch == nil {
		return
	}
	meta := dispatch.Meta.Clone()
	if meta == nil {
		meta = op.Meta{}
	}
	if strings.TrimSpace(metaString(meta, "threadID")) == "" || strings.TrimSpace(metaString(meta, "agentID")) == "" {
		slog.Warn("messenger reply dispatch missing thread or agent", "meta", meta)
		return
	}
	meta["opcode"] = string(op.OpThreadSubmit)
	go func() {
		if err := h.callThreadSubmit(context.Background(), meta, nil); err != nil {
			slog.Warn("messenger reply dispatch failed", "error", err, "threadID", metaString(meta, "threadID"), "agentID", metaString(meta, "agentID"))
		}
	}()
}

func (h *Handler) callThreadSubmit(ctx context.Context, meta op.Meta, content op.Content) error {
	if h == nil || h.server == nil {
		return fmt.Errorf("server is not initialized")
	}
	session := h.server.GetHostSession()
	if session == nil {
		return fmt.Errorf("host session not initialized")
	}
	_, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpThreadSubmit,
		Meta:    meta,
		Content: content,
	})
	return err
}

func metaString(meta op.Meta, key string) string {
	if meta == nil {
		return ""
	}
	value, ok := meta[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func invalidParams(err error) *protocol.RPCError {
	return &protocol.RPCError{
		Code:    protocol.ErrCodeInvalidParams,
		Message: "Invalid params: " + err.Error(),
	}
}

func internalRPCError(err error) *protocol.RPCError {
	return &protocol.RPCError{
		Code:    protocol.ErrCodeInternal,
		Message: err.Error(),
	}
}
