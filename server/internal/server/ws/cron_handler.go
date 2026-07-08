package ws

import (
	"encoding/json"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

func (h *Handler) handleCronList(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CronListParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeInvalidParams,
				Message: "Invalid params: " + err.Error(),
			}
		}
	}
	return h.callHostNode(op.OpCode(protocol.MethodCronList), p)
}

func (h *Handler) handleCronGet(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CronIDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpCode(protocol.MethodCronGet), p)
}

func (h *Handler) handleCronAdd(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CronTaskWriteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpCode(protocol.MethodCronAdd), p)
}

func (h *Handler) handleCronUpsert(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CronTaskUpsertParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpCode(protocol.MethodCronUpsert), p)
}

func (h *Handler) handleCronUpdate(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CronTaskWriteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpCode(protocol.MethodCronUpdate), p)
}

func (h *Handler) handleCronRemove(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CronIDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpCode(protocol.MethodCronRemove), p)
}

func (h *Handler) handleCronRun(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CronIDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpCode(protocol.MethodCronRun), p)
}

func (h *Handler) handleCronHistory(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CronHistoryParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpCode(protocol.MethodCronHistory), p)
}
