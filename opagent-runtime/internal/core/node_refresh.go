package core

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/internal/scan"
)

var nodeRefreshHTTPToolProbeTimeout = 5 * time.Second

// RefreshNodeCache rescans the configured baseDir and replaces the cached node set.
// The cache is updated only after the full scan completes so renames and deletions
// do not leave stale nodes behind.
func RefreshNodeCache(ctx context.Context, opts scan.ScanOptions) error {
	s := scan.NewScanner(opts.UID, opts.BaseDir).WithNodeIndexBaseDir(opts.BaseDir)

	// Tools first so agent reference resolution can reuse discovered tool nodes.
	s.ScanTools(filepath.Join(opts.BaseDir, "tools"), 0)
	// Skills before agents so @skills/<slug> resolves from the in-memory scan set.
	s.ScanSkills(filepath.Join(opts.BaseDir, "skills"), 0)
	s.ScanAgents(filepath.Join(opts.BaseDir, "agents"), 0)
	if err := s.Err(); err != nil {
		return err
	}

	discoveredNodes := s.Nodes()
	toolNodes := filterNodesByKind(discoveredNodes, op.NodeKindTools)
	skillNodes := filterNodesByKind(discoveredNodes, op.NodeKindSkill)
	agentNodes := filterNodesByKind(discoveredNodes, op.NodeKindAgent)

	filteredNodes := make([]*op.OpNode, 0, len(discoveredNodes))

	for _, node := range toolNodes {
		toolsMeta, ok := node.Meta.(*op.ToolsMeta)
		if !ok || toolsMeta == nil {
			continue
		}
		if !node.Run.HasEndpoint() {
			filteredNodes = append(filteredNodes, node)
			continue
		}
		if node.Run.URL != "" {
			toolSpecs, err := probeHTTPToolSpecsForRefresh(ctx, node)
			if err != nil {
				slog.Warn("http tool server probe skipped", "error", err, "nodeID", node.ID)
				continue
			}
			toolsMeta.Tools = toolSpecs
			filteredNodes = append(filteredNodes, node)
			continue
		}
		conn, err := CreateConnection(ctx, node)
		if err != nil {
			slog.Error("failed to create tool connection", "error", err, "nodeID", node.ID)
			continue
		}
		toolSpecs, err := conn.ListToolSpecs()
		if err != nil {
			slog.Error("failed to list tool uses", "error", err, "nodeID", node.ID)
			continue
		}
		toolsMeta.Tools = toolSpecs

		filteredNodes = append(filteredNodes, node)
	}

	filteredNodes = append(filteredNodes, skillNodes...)
	filteredNodes = append(filteredNodes, agentNodes...)

	existingNodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)
	for _, existing := range existingNodes {
		if key := existing.ID; key != "" {
			cache.Delete(key, cache.PrefixNode)
		}
	}

	for _, node := range filteredNodes {
		cache.SetValue(node.ID, cache.PrefixNode, *node, cache.NoExpiration)
	}
	if err := reconcileCronNodes(filteredNodesToValues(filteredNodes)); err != nil {
		slog.Warn("failed to reconcile cron nodes", "error", err)
	}

	slog.Info("nodes refreshed",
		"total", len(filteredNodes),
		"baseDir", opts.BaseDir,
		"uid", opts.UID,
	)
	return nil
}

func probeHTTPToolSpecsForRefresh(ctx context.Context, node *op.OpNode) ([]*op.ToolSpec, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, nodeRefreshHTTPToolProbeTimeout)
	defer cancel()

	conn, err := createConnection(probeCtx, node, createConnectionOptions{
		httpClient:              newRunHeaderHTTPClientWithTimeout(node.Run.Header, nodeRefreshHTTPToolProbeTimeout),
		skipCache:               true,
		suppressConnectErrorLog: true,
	})
	if err != nil {
		return nil, err
	}
	defer conn.ForceClose()

	return conn.ListToolSpecsContext(probeCtx)
}

func filterNodesByKind(nodes []*op.OpNode, kind op.NodeKind) []*op.OpNode {
	if len(nodes) == 0 {
		return nil
	}
	filtered := make([]*op.OpNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil || node.Kind != string(kind) {
			continue
		}
		filtered = append(filtered, node)
	}
	return filtered
}

func filteredNodesToValues(nodes []*op.OpNode) []op.OpNode {
	if len(nodes) == 0 {
		return nil
	}
	values := make([]op.OpNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		values = append(values, *node)
	}
	return values
}
