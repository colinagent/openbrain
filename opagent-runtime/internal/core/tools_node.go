package core

// func listToolsNodes(ctx context.Context) ([]*op.OpNode, error) {
// 	return listNodesByKind(ctx, nodestore.KindTools)
// }

// func getToolsNode(ctx context.Context, nodeID string) (*op.OpNode, *op.ToolsMeta, error) {
// 	if err := ensureNodeIDForKind(nodeID, nodestore.KindTools); err != nil {
// 		return nil, nil, err
// 	}

// 	node, err := getNodeByID(ctx, nodeID)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	if err := ensureNodeKindMatch(node, nodestore.KindTools); err != nil {
// 		return nil, nil, err
// 	}
// 	meta, ok := nodestore.NodeToolsMeta(node)
// 	if !ok {
// 		return nil, nil, fmt.Errorf("tools meta not found for node: %s", nodeID)
// 	}
// 	return node, meta, nil
// }

// func nodeToolsMeta(node *op.OpNode) (*op.ToolsMeta, bool) {
// 	return nodestore.NodeToolsMeta(node)
// }

// func findToolsNodeByName(ctx context.Context, name string) (*op.OpNode, *op.ToolsMeta, error) {
// 	name = strings.TrimSpace(name)
// 	if name == "" {
// 		return nil, nil, fmt.Errorf("tool server name is required")
// 	}
// 	nodes, err := listToolsNodes(ctx)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	for _, node := range nodes {
// 		meta, ok := nodestore.NodeToolsMeta(node)
// 		if !ok {
// 			continue
// 		}
// 		if strings.EqualFold(strings.TrimSpace(meta.Name), name) {
// 			return node, meta, nil
// 		}
// 	}
// 	return nil, nil, fmt.Errorf("tool server not found by name: %s", name)
// }

// func upsertToolsNodeMeta(ctx context.Context, node *op.OpNode, meta *op.ToolsMeta) error {
// 	if node == nil {
// 		return fmt.Errorf("tools node is nil")
// 	}
// 	if meta == nil {
// 		return fmt.Errorf("tools meta is nil")
// 	}
// 	store := nodestore.GetDefault()
// 	if store == nil {
// 		return fmt.Errorf("node store is not initialized")
// 	}
// 	node.Meta = *meta
// 	return store.Upsert(ctx, node)
// }

// func toToolSpecsFromList(tools []*op.Tool) []op.ToolSpec {
// 	if len(tools) == 0 {
// 		return nil
// 	}
// 	specs := make([]op.ToolSpec, 0, len(tools))
// 	for _, tool := range tools {
// 		if tool == nil || strings.TrimSpace(tool.Name) == "" {
// 			continue
// 		}
// 		specs = append(specs, op.ToolSpec{
// 			Name:        tool.Name,
// 			Description: tool.Description,
// 			InputSchema: tool.InputSchema,
// 		})
// 	}
// 	return specs
// }
