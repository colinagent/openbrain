package config

import (
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestNormalizeNodesMapUsesNodeID(t *testing.T) {
	raw := map[string]op.OpNode{
		"agent-c123": {
			UID:  "user-alice",
			Kind: "agent",
			URI:  "file:///tmp/coder/.agent/AGENT.md",
		},
	}

	normalized := normalizeNodesMap(raw, op.EnvCloud, "host-a1b2")
	if len(normalized) != 1 {
		t.Fatalf("normalized size = %d, want 1", len(normalized))
	}

	node, ok := normalized["agent-c123"]
	if !ok {
		t.Fatalf("normalized map missing node id")
	}
	if node.ID != "agent-c123" {
		t.Fatalf("node id = %q, want agent-c123", node.ID)
	}
	if node.HostID != "host-a1b2" {
		t.Fatalf("node hostID = %q, want host-a1b2", node.HostID)
	}
	if node.Kind != "agent" {
		t.Fatalf("node kind = %q, want agent", node.Kind)
	}
}

func TestNormalizeNodesMapDerivesKindFromID(t *testing.T) {
	raw := map[string]op.OpNode{
		"skill-p456": {
			URI: "file:///tmp/plan/SKILL.md",
		},
	}

	normalized := normalizeNodesMap(raw, op.EnvCloud, "host-b9z1")
	node, ok := normalized["skill-p456"]
	if !ok {
		t.Fatalf("normalized map missing node id")
	}
	if node.Kind != "skill" {
		t.Fatalf("node kind = %q, want skill", node.Kind)
	}
}
