package ws

import (
	"context"
	"encoding/json"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

func (h *Handler) handleArchiveCleanupRun(params json.RawMessage) (interface{}, *protocol.RPCError) {
	if h.archive == nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "archive cleanup service is not initialized",
		}
	}

	var p protocol.ArchiveCleanupParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeInvalidParams,
				Message: "Invalid params: " + err.Error(),
			}
		}
	}

	result, err := h.archive.Run(context.Background(), p)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Archive cleanup failed: " + err.Error(),
		}
	}
	return result, nil
}
