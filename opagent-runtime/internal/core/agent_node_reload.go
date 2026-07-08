package core

import (
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/internal/scan"
)

func refreshFileBackedAgentNode(node op.OpNode) op.OpNode {
	if strings.TrimSpace(node.Kind) != string(op.NodeKindAgent) {
		return node
	}
	agentFile := strings.TrimSpace(op.URIToPath(node.URI))
	if agentFile == "" || filepath.Base(agentFile) != "AGENT.md" {
		return node
	}
	agentDir := strings.TrimSpace(node.Cwd)
	if agentDir == "" {
		agentDir = agentRootFromURI(node.URI)
	}
	if agentDir == "" {
		return node
	}

	system := config.GetSystem()
	if system == nil {
		return node
	}
	baseDir := ""
	uid := strings.TrimSpace(node.UID)
	baseDir = strings.TrimSpace(system.BaseDir)
	if baseDir == "" {
		return node
	}
	if uid == "" {
		uid = op.LocalUser
	}
	scanner := scan.NewScanner(uid, agentDir)
	if baseDir != "" {
		scanner.WithRefBaseDir(baseDir).WithNodeIndexBaseDir(baseDir)
	}
	nodes := scanner.ScanAgents(agentDir, 0)
	if err := scanner.Err(); err != nil {
		slog.Warn("failed to refresh file-backed agent node", "agentID", node.ID, "agentDir", agentDir, "error", err)
		return node
	}
	for _, candidate := range nodes {
		if candidate == nil || strings.TrimSpace(candidate.ID) != strings.TrimSpace(node.ID) {
			continue
		}
		cache.SetValue(candidate.ID, cache.PrefixNode, *candidate, cache.NoExpiration)
		return *candidate
	}
	return node
}
