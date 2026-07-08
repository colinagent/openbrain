package core

// import (
// 	"context"
// 	"log/slog"

// 	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
// )

// var toolsNodeLister func(ctx context.Context, nodeID string) ([]*op.Tool, error)

// func defaultToolsNodeLister(ctx context.Context, nodeID string) ([]*op.Tool, error) {
// 	conn, err := EnsureConnection(ctx, nodeID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return conn.ListTools()
// }

// // ensureToolsNodeTools hydrates the tool list for a tools node via its connection.
// func ensureToolsNodeTools(ctx context.Context, node *op.OpNode, meta *op.ToolsMeta) *op.ToolsMeta {
// 	if node == nil || meta == nil {
// 		return meta
// 	}
// 	if !shouldHydrateToolsNode(meta.Tools) {
// 		return meta
// 	}
// 	lister := toolsNodeLister
// 	if lister == nil {
// 		lister = defaultToolsNodeLister
// 	}
// 	tools, err := lister(ctx, node.ID)
// 	if err != nil {
// 		slog.Debug("failed to list tools for node", "error", err, "nodeID", node.ID)
// 		return meta
// 	}
// 	meta.Tools = toToolSpecsFromList(tools)
// 	if err := upsertToolsNodeMeta(ctx, node, meta); err != nil {
// 		slog.Debug("failed to persist hydrated tools list", "error", err, "nodeID", node.ID)
// 	}
// 	return meta
// }

// func shouldHydrateToolsNode(specs []op.ToolSpec) bool {
// 	if len(specs) == 0 {
// 		return true
// 	}
// 	for _, spec := range specs {
// 		if normalizeSchema(spec.InputSchema) == nil {
// 			return true
// 		}
// 	}
// 	return false
// }
