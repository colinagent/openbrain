package node

// func nodeKindFromID(id string) (string, bool) {
// 	return op.NodeIDKind(id)
// }

// func ensureNodeIDForKind(id, kind string) error {
// 	id = strings.TrimSpace(id)
// 	if id == "" {
// 		return fmt.Errorf("node id is required")
// 	}
// 	parsedKind, ok := nodeKindFromID(id)
// 	if !ok {
// 		return fmt.Errorf("invalid node id format: %s", id)
// 	}
// 	expected := op.NormalizeNodeKind(kind)
// 	if parsedKind != expected {
// 		return fmt.Errorf("node id kind mismatch: id=%s expected=%s got=%s", id, expected, parsedKind)
// 	}
// 	return nil
// }

// func ensureNodeKindMatch(node *op.OpNode, expectedKind string) error {
// 	if node == nil {
// 		return fmt.Errorf("node is nil")
// 	}
// 	kind := op.NormalizeNodeKind(node.Kind)
// 	expected := op.NormalizeNodeKind(expectedKind)
// 	if kind != expected {
// 		return fmt.Errorf("node kind mismatch: id=%s expected=%s got=%s", node.ID, expected, kind)
// 	}
// 	return nil
// }

// func listNodesByUIDAndKind(ctx context.Context, uid, kind string) ([]*op.OpNode, error) {
// 	store := nodestore.GetDefault()
// 	if store == nil {
// 		return nil, fmt.Errorf("node store is not initialized")
// 	}
// 	return store.ListByUIDAndKind(ctx, uid, kind)
// }

// func listNodesByUID(ctx context.Context, uid string) ([]*op.OpNode, error) {
// 	store := nodestore.GetDefault()
// 	if store == nil {
// 		return nil, fmt.Errorf("node store is not initialized")
// 	}
// 	return store.ListByUID(ctx, uid)
// }

// func listNodesByKind(ctx context.Context, kind string) ([]*op.OpNode, error) {
// 	store := GetDefault()
// 	if store == nil {
// 		return nil, fmt.Errorf("node store is not initialized")
// 	}
// 	return store.ListByKind(ctx, kind)
// }

// func getNodeByID(ctx context.Context, id string) (*op.OpNode, error) {
// 	store := nodestore.GetDefault()
// 	if store == nil {
// 		return nil, fmt.Errorf("node store is not initialized")
// 	}
// 	return store.GetByID(ctx, strings.TrimSpace(id))
// }
