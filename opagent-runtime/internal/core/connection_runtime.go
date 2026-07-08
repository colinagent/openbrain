package core

import (
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

type ConnectionRuntimeSnapshot struct {
	NodeID       string           `json:"nodeID"`
	Name         string           `json:"name,omitempty"`
	Transport    op.TransportType `json:"transport,omitempty"`
	Daemon       bool             `json:"daemon,omitempty"`
	ConnectedAt  *time.Time       `json:"connectedAt,omitempty"`
	PID          int              `json:"pid,omitempty"`
	StartedAt    *time.Time       `json:"startedAt,omitempty"`
	UptimeSec    int64            `json:"uptimeSec,omitempty"`
	LastActiveAt *time.Time       `json:"lastActiveAt,omitempty"`
	URL          string           `json:"url,omitempty"`
}

type connectionRuntimeState struct {
	mu           sync.RWMutex
	connectedAt  *time.Time
	pid          int
	startedAt    *time.Time
	lastActiveAt *time.Time
}

func newConnectionRuntimeState() *connectionRuntimeState {
	return &connectionRuntimeState{}
}

func (s *connectionRuntimeState) setConnected(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC()
	if s.connectedAt == nil {
		s.connectedAt = &now
	}
}

func (s *connectionRuntimeState) setProcess(pid int, startedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	startedAt = startedAt.UTC()
	s.pid = pid
	s.startedAt = &startedAt
	if s.connectedAt == nil {
		s.connectedAt = &startedAt
	}
}

func (s *connectionRuntimeState) markProtocolActivity(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC()
	s.lastActiveAt = &now
	if s.connectedAt == nil {
		s.connectedAt = &now
	}
}

func (s *connectionRuntimeState) snapshot(now time.Time, conn *Connection) *ConnectionRuntimeSnapshot {
	if conn == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	var connectedAt *time.Time
	if s.connectedAt != nil {
		t := s.connectedAt.UTC()
		connectedAt = &t
	}
	var startedAt *time.Time
	if s.startedAt != nil {
		t := s.startedAt.UTC()
		startedAt = &t
	}
	var lastActiveAt *time.Time
	if s.lastActiveAt != nil {
		t := s.lastActiveAt.UTC()
		lastActiveAt = &t
	}

	out := &ConnectionRuntimeSnapshot{
		NodeID:       strings.TrimSpace(conn.NodeID),
		Name:         strings.TrimSpace(conn.Name),
		Transport:    conn.TransType,
		Daemon:       conn.Daemon,
		ConnectedAt:  connectedAt,
		PID:          s.pid,
		StartedAt:    startedAt,
		LastActiveAt: lastActiveAt,
		URL:          strings.TrimSpace(conn.URL),
	}
	if startedAt != nil {
		uptime := now.UTC().Sub(*startedAt)
		if uptime > 0 {
			out.UptimeSec = int64(uptime / time.Second)
		}
	}
	return out
}

func (conn *Connection) setConnectedAt(now time.Time) {
	if conn == nil || conn.runtime == nil {
		return
	}
	conn.runtime.setConnected(now)
}

func (conn *Connection) setProcessRuntime(pid int, startedAt time.Time) {
	if conn == nil || conn.runtime == nil {
		return
	}
	conn.runtime.setProcess(pid, startedAt)
}

func (conn *Connection) markIncomingProtocolTraffic() {
	if conn == nil || conn.TransType != op.HttpStreamable || conn.runtime == nil {
		return
	}
	conn.runtime.markProtocolActivity(time.Now().UTC())
}

func (conn *Connection) markOutgoingProtocolTraffic() {
	if conn == nil || conn.TransType != op.HttpStreamable || conn.runtime == nil {
		return
	}
	conn.runtime.markProtocolActivity(time.Now().UTC())
}

func (conn *Connection) RuntimeSnapshot(now time.Time) *ConnectionRuntimeSnapshot {
	if conn == nil || conn.runtime == nil {
		return nil
	}
	return conn.runtime.snapshot(now, conn)
}

func resolveConnectionMetadata(node *op.OpNode) (name string, description string) {
	if node == nil {
		return "", ""
	}
	switch meta := node.Meta.(type) {
	case *op.AgentMeta:
		if meta != nil {
			return strings.TrimSpace(meta.Name), strings.TrimSpace(meta.Description)
		}
	case *op.ToolsMeta:
		if meta != nil {
			return strings.TrimSpace(meta.Name), strings.TrimSpace(meta.Description)
		}
	case *op.SkillMeta:
		if meta != nil {
			return strings.TrimSpace(meta.Name), strings.TrimSpace(meta.Description)
		}
	case map[string]any:
		rawName, _ := meta["name"].(string)
		rawDescription, _ := meta["description"].(string)
		return strings.TrimSpace(rawName), strings.TrimSpace(rawDescription)
	}
	return "", ""
}

func ListActiveConnectionSnapshots(now time.Time) []*ConnectionRuntimeSnapshot {
	conns := cache.ListByPrefix[Connection](cache.PrefixConnection)
	if len(conns) == 0 {
		return nil
	}
	out := make([]*ConnectionRuntimeSnapshot, 0, len(conns))
	for _, conn := range conns {
		if conn == nil || conn.Session == nil {
			continue
		}
		snapshot := conn.RuntimeSnapshot(now)
		if snapshot == nil || snapshot.NodeID == "" {
			continue
		}
		out = append(out, snapshot)
	}
	return out
}
