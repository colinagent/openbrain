package ws

// handleSessionSet handles session/set requests
// func (h *Handler) handleSessionSet(params json.RawMessage) (interface{}, *protocol.RPCError) {
// 	var p protocol.SessionSetParams
// 	if err := json.Unmarshal(params, &p); err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInvalidParams,
// 			Message: "Invalid params: " + err.Error(),
// 		}
// 	}

// 	store := h.server.GetSessionStore()
// 	if store == nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "session store not initialized",
// 		}
// 	}

// 	result, err := store.Set(p.Auth, p.Profile)
// 	if err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Failed to write session: " + err.Error(),
// 		}
// 	}

// 	return result, nil
// }

// // handleSessionClear handles session/clear requests
// func (h *Handler) handleSessionClear(params json.RawMessage) (interface{}, *protocol.RPCError) {
// 	store := h.server.GetSessionStore()
// 	if store == nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "session store not initialized",
// 		}
// 	}

// 	if err := store.Clear(); err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Failed to clear session: " + err.Error(),
// 		}
// 	}

// 	return map[string]bool{"ok": true}, nil
// }

// // handleSessionGet handles session/get requests
// func (h *Handler) handleSessionGet(_ json.RawMessage) (interface{}, *protocol.RPCError) {
// 	store := h.server.GetSessionStore()
// 	if store == nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "session store not initialized",
// 		}
// 	}

// 	result, err := store.Get()
// 	if err != nil {
// 		return nil, &protocol.RPCError{
// 			Code:    protocol.ErrCodeInternal,
// 			Message: "Failed to read session: " + err.Error(),
// 		}
// 	}

// 	return result, nil
// }
