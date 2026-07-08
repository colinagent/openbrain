package ws

// handleAgentsRoots handles agents/roots requests
// func (h *Handler) handleAgentsRoots(params json.RawMessage) (interface{}, *protocol.RPCError) {
// 	session := h.server.GetSession()
// 	if session == nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "OpAgent session not initialized",
// 		}
// 	}

// 	opResult, err := session.OpAgent(context.Background(), &op.OpAgentParams{
// 		OpCode: op.OpAgentRoots,
// 	})
// 	if err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Failed to get agents roots: " + err.Error(),
// 		}
// 	}

// 	if opResult == nil || opResult.Content == nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Invalid response from OpAgent",
// 		}
// 	}

// 	jsonContent, ok := opResult.Content.(*op.JsonContent)
// 	if !ok {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Unexpected content type from OpAgent",
// 		}
// 	}

// 	var rootsResult protocol.AgentsRootsResult
// 	if err := json.Unmarshal(jsonContent.Raw, &rootsResult); err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Failed to parse roots: " + err.Error(),
// 		}
// 	}

// 	return &rootsResult, nil
// }
