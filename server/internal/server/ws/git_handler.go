package ws

import (
	"encoding/json"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

func (h *Handler) handleGitBranches(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.GitBranchesParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Invalid params: " + err.Error()}
	}
	return h.git.Branches(&p)
}

func (h *Handler) handleGitCheckout(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.GitCheckoutParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Invalid params: " + err.Error()}
	}
	return h.git.Checkout(&p)
}
