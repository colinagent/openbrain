package core

import (
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func TestRuntimeSnapshotForStdioConnectionIncludesProcessUptime(t *testing.T) {
	startedAt := time.Now().UTC().Add(-95 * time.Second)
	conn := &Connection{
		NodeID:    "local:host-a1b2:agent:file:///tmp/.agent/AGENT.md",
		Name:      "coder",
		TransType: op.Stdio,
		Daemon:    true,
		Session:   &op.ClientSession{},
		runtime:   newConnectionRuntimeState(),
	}
	conn.setConnectedAt(startedAt)
	conn.setProcessRuntime(4242, startedAt)

	snapshot := conn.RuntimeSnapshot(time.Now().UTC())
	if snapshot == nil {
		t.Fatal("RuntimeSnapshot() = nil")
	}
	if snapshot.NodeID != conn.NodeID {
		t.Fatalf("snapshot.NodeID = %q, want %q", snapshot.NodeID, conn.NodeID)
	}
	if snapshot.PID != 4242 {
		t.Fatalf("snapshot.PID = %d, want 4242", snapshot.PID)
	}
	if snapshot.StartedAt == nil {
		t.Fatal("snapshot.StartedAt = nil")
	}
	if snapshot.UptimeSec < 95 {
		t.Fatalf("snapshot.UptimeSec = %d, want >= 95", snapshot.UptimeSec)
	}
	if snapshot.LastActiveAt != nil {
		t.Fatalf("snapshot.LastActiveAt = %v, want nil for stdio runtime", snapshot.LastActiveAt)
	}
}

func TestRuntimeSnapshotForHTTPConnectionTracksProtocolActivity(t *testing.T) {
	connectedAt := time.Now().UTC().Add(-30 * time.Second)
	conn := &Connection{
		NodeID:    "tool-server",
		Name:      "browser",
		TransType: op.HttpStreamable,
		Daemon:    true,
		URL:       "https://example.com/mcp",
		Session:   &op.ClientSession{},
		runtime:   newConnectionRuntimeState(),
	}
	conn.setConnectedAt(connectedAt)

	initial := conn.RuntimeSnapshot(time.Now().UTC())
	if initial == nil {
		t.Fatal("initial RuntimeSnapshot() = nil")
	}
	if initial.LastActiveAt != nil {
		t.Fatalf("initial LastActiveAt = %v, want nil before protocol traffic", initial.LastActiveAt)
	}

	conn.markOutgoingProtocolTraffic()
	afterActivity := conn.RuntimeSnapshot(time.Now().UTC())
	if afterActivity.LastActiveAt == nil {
		t.Fatal("LastActiveAt = nil after protocol traffic")
	}

	stdioConn := &Connection{
		NodeID:    "stdio-server",
		Name:      "coder",
		TransType: op.Stdio,
		Daemon:    true,
		Session:   &op.ClientSession{},
		runtime:   newConnectionRuntimeState(),
	}
	stdioConn.setConnectedAt(time.Now().UTC())
	stdioConn.markOutgoingProtocolTraffic()
	stdioSnapshot := stdioConn.RuntimeSnapshot(time.Now().UTC())
	if stdioSnapshot.LastActiveAt != nil {
		t.Fatalf("stdio LastActiveAt = %v, want nil", stdioSnapshot.LastActiveAt)
	}
}

func TestListActiveConnectionSnapshotsReturnsOnlyLiveConnections(t *testing.T) {
	cache.Flush()
	t.Cleanup(cache.Flush)

	now := time.Now().UTC()
	httpConn := &Connection{
		NodeID:    "http-server",
		Name:      "remote-http",
		TransType: op.HttpStreamable,
		Daemon:    true,
		URL:       "https://example.com/mcp",
		Session:   &op.ClientSession{},
		runtime:   newConnectionRuntimeState(),
	}
	httpConn.setConnectedAt(now.Add(-10 * time.Second))
	httpConn.markOutgoingProtocolTraffic()

	stdioConn := &Connection{
		NodeID:    "stdio-server",
		Name:      "coder",
		TransType: op.Stdio,
		Daemon:    true,
		Session:   &op.ClientSession{},
		runtime:   newConnectionRuntimeState(),
	}
	stdioConn.setConnectedAt(now.Add(-20 * time.Second))
	stdioConn.setProcessRuntime(999, now.Add(-20*time.Second))

	staleConn := &Connection{
		NodeID:    "stale-server",
		Name:      "stale",
		TransType: op.HttpStreamable,
		Daemon:    true,
		runtime:   newConnectionRuntimeState(),
	}

	cache.Set(httpConn.NodeID, cache.PrefixConnection, httpConn, cache.NoExpiration)
	cache.Set(stdioConn.NodeID, cache.PrefixConnection, stdioConn, cache.NoExpiration)
	cache.Set(staleConn.NodeID, cache.PrefixConnection, staleConn, cache.NoExpiration)

	snapshots := ListActiveConnectionSnapshots(time.Now().UTC())
	if len(snapshots) != 2 {
		t.Fatalf("len(snapshots) = %d, want 2", len(snapshots))
	}
}
