package ws

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/colinagent/openbrain/server/internal/server/cache"
	"github.com/colinagent/openbrain/server/internal/server/protocol"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"golang.org/x/sync/singleflight"
)

var agentGetGroup singleflight.Group

const unauthorizedRPCMessage = "unauthorized: please sign in first"

func (h *Handler) resolveUIDFromHost() (string, *protocol.RPCError) {
	cfg, ok := cache.Get[op.Config]("config")
	if !ok {
		return "", &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "config not found",
		}
	}
	uid := cfg.User.Profile.UID
	return uid, nil
	// store := h.server.GetSessionStore()
	// if store == nil {
	// 	slog.Warn("ws unauthorized: session store unavailable", "reason", "session_missing_or_invalid")
	// 	return "", &protocol.RPCError{
	// 		Code:    protocol.ErrCodeInternal,
	// 		Message: unauthorizedRPCMessage,
	// 	}
	// }
	// session, err := store.Get()
	// if err != nil {
	// 	slog.Warn("ws unauthorized: failed to read session", "reason", "session_missing_or_invalid", "error", err)
	// 	return "", &protocol.RPCError{
	// 		Code:    protocol.ErrCodeInternal,
	// 		Message: unauthorizedRPCMessage,
	// 	}
	// }
	// if session == nil || session.Auth == nil {
	// 	slog.Warn("ws unauthorized: auth session missing", "reason", "session_missing_or_invalid")
	// 	return "", &protocol.RPCError{
	// 		Code:    protocol.ErrCodeInternal,
	// 		Message: unauthorizedRPCMessage,
	// 	}
	// }
	// uid := strings.TrimSpace(session.Auth.UID)
	// if uid == "" || strings.EqualFold(uid, "anonymous") {
	// 	slog.Warn("ws unauthorized: auth uid missing", "reason", "session_missing_or_invalid")
	// 	return "", &protocol.RPCError{
	// 		Code:    protocol.ErrCodeInternal,
	// 		Message: unauthorizedRPCMessage,
	// 	}
	// }
	// return uid, nil
}

// handleAgentGet handles agent/get requests
// func (h *Handler) handleAgentGet(params json.RawMessage) (interface{}, *protocol.RPCError) {
// 	var p protocol.AgentGetParams
// 	if err := json.Unmarshal(params, &p); err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInvalidParams,
// 			Message: "Invalid params: " + err.Error(),
// 		}
// 	}

// 	agentID, _ := p.Meta["agentID"].(string)
// 	if agentID == "" {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInvalidParams,
// 			Message: "agentID is required in meta",
// 		}
// 	}

// 	session := h.server.GetSession()
// 	if session == nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "OpAgent session not initialized",
// 		}
// 	}
// 	uid, rpcErr := h.resolveUIDFromSession()
// 	if rpcErr != nil {
// 		return nil, rpcErr
// 	}

// 	result, err, _ := agentGetGroup.Do(agentID, func() (interface{}, error) {
// 		meta := op.Meta{
// 			"agentID": agentID,
// 			"uid":     uid,
// 		}
// 		return session.OpAgent(context.Background(), &op.OpAgentParams{
// 			OpCode: op.OpAgentGet,
// 			Meta:   meta,
// 		})
// 	})
// 	if err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Failed to get agent: " + err.Error(),
// 		}
// 	}

// 	opResult, ok := result.(*op.OpAgentResult)
// 	if !ok || opResult == nil || opResult.Content == nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Invalid response from OpAgent",
// 		}
// 	}

// 	// Return the JSON content directly
// 	jsonContent, ok := opResult.Content.(*op.JsonContent)
// 	if !ok {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Unexpected content type from OpAgent",
// 		}
// 	}

// 	var agentRecord interface{}
// 	if err := json.Unmarshal(jsonContent.Raw, &agentRecord); err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Failed to parse agent record: " + err.Error(),
// 		}
// 	}

// 	return agentRecord, nil
// }

func (h *Handler) handleConfigSystemGet(params json.RawMessage) (interface{}, *protocol.RPCError) {
	session := h.server.GetHostSession()
	if session == nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "OpAgent session not initialized",
		}
	}

	result, err := session.OpNode(context.Background(), &op.OpNodeParams{
		OpCode: op.ConfigSystemGet,
	})
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Failed to get config system: " + err.Error(),
		}
	}
	return parseJSONContentResult(result)
}

func (h *Handler) handleNodeList(params json.RawMessage) (interface{}, *protocol.RPCError) {
	session := h.server.GetHostSession()
	if session == nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "OpAgent session not initialized",
		}
	}

	meta := op.Meta{}
	if len(params) > 0 && string(params) != "null" {
		var raw map[string]any
		if err := json.Unmarshal(params, &raw); err != nil {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeInvalidParams,
				Message: "Invalid params: " + err.Error(),
			}
		}
		if refresh, ok := raw["refresh"].(bool); ok {
			meta["refresh"] = refresh
		}
	}

	result, err := session.OpNode(context.Background(), &op.OpNodeParams{
		OpCode: op.OpNodeList,
		Meta:   meta,
	})
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Failed to list nodes: " + err.Error(),
		}
	}
	payload, rpcErr := parseJSONContentResult(result)
	if rpcErr != nil {
		return nil, rpcErr
	}
	return payload, nil
}

// handleDirAgentScan handles dir/agentscan (and agent/scan) requests.
func (h *Handler) handleAgentScan(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.DirAgentScanParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	if strings.TrimSpace(p.Dir) == "" {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "dir is required",
		}
	}
	session := h.server.GetHostSession()
	if session == nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "OpAgent session not initialized",
		}
	}
	meta := op.Meta{
		"dir": p.Dir,
	}

	result, err := session.OpNode(context.Background(), &op.OpNodeParams{
		OpCode: op.OpAgentScan,
		Meta:   meta,
	})
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Failed to scan nodes: " + err.Error(),
		}
	}
	return parseJSONContentResult(result)
}

// handleNodes handles nodes requests.
// func (h *Handler) handleNodes(params json.RawMessage) (interface{}, *protocol.RPCError) {
// 	var p protocol.NodesParams
// 	if len(params) > 0 {
// 		if err := json.Unmarshal(params, &p); err != nil {
// 			return nil, &protocol.RPCError{
// 				Code:    protocol.ErrCodeInvalidParams,
// 				Message: "Invalid params: " + err.Error(),
// 			}
// 		}
// 	}
// 	session := h.server.GetSession()
// 	if session == nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "OpAgent session not initialized",
// 		}
// 	}
// 	uid, rpcErr := h.resolveUIDFromSession()
// 	if rpcErr != nil {
// 		return nil, rpcErr
// 	}
// 	meta := op.Meta{"uid": uid}
// 	if p.Refresh != nil {
// 		meta["refresh"] = *p.Refresh
// 	}
// 	result, err := session.OpAgent(context.Background(), &op.OpAgentParams{
// 		OpCode: op.OpNodeList,
// 		Meta:   meta,
// 	})
// 	if err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Failed to list nodes: " + err.Error(),
// 		}
// 	}
// 	return parseJSONContentResult(result)
// }

// handleNodesCached handles nodes/cached requests.
// func (h *Handler) handleNodesCached(params json.RawMessage) (interface{}, *protocol.RPCError) {
// 	var p protocol.NodesParams
// 	if len(params) > 0 {
// 		if err := json.Unmarshal(params, &p); err != nil {
// 			return nil, &protocol.RPCError{
// 				Code:    protocol.ErrCodeInvalidParams,
// 				Message: "Invalid params: " + err.Error(),
// 			}
// 		}
// 	}
// 	session := h.server.GetSession()
// 	if session == nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "OpAgent session not initialized",
// 		}
// 	}
// 	uid, rpcErr := h.resolveUIDFromSession()
// 	if rpcErr != nil {
// 		return nil, rpcErr
// 	}
// 	meta := op.Meta{"uid": uid}
// 	result, err := session.OpAgent(context.Background(), &op.OpAgentParams{
// 		OpCode: op.OpNodeCached,
// 		Meta:   meta,
// 	})
// 	if err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Failed to list cached nodes: " + err.Error(),
// 		}
// 	}
// 	return parseJSONContentResult(result)
// }

func parseJSONContentResult(result *op.OpNodeResult) (interface{}, *protocol.RPCError) {

	if result == nil || result.Content == nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Invalid response from OpAgent",
		}
	}
	content := result.Content

	jsonContent, ok := content.(*op.JsonContent)
	if !ok {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Unexpected content type from OpAgent",
		}
	}
	var payload interface{}
	if err := json.Unmarshal(jsonContent.Raw, &payload); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Failed to parse JSON content: " + err.Error(),
		}
	}
	return payload, nil
}
