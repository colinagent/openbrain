package sse

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const (
	defaultExpiration    = 10 * time.Minute
	turnReplayGrace      = 60 * time.Second
	sseChannelBufferSize = 20000
)

var (
	ErrTurnConflict = errors.New("another turn is already running for thread")
	ErrTurnNotFound = errors.New("turn is no longer available for replay")
)

type Event struct {
	ID      int64
	Message *op.GeneralContent
}

// Connection represents a live SSE stream attached to a thread.
type Connection struct {
	ThreadID      string
	TurnRequestID string
	SSEChan       chan *Event
	Ctx           context.Context
	Cancel        context.CancelFunc
	LastActive    time.Time
}

type turnState struct {
	ThreadID      string
	TurnRequestID string
	Connection    *Connection
	Events        []*Event
	NextEventID   int64
	Running       bool
	StartedAt     time.Time
	CompletedAt   time.Time
	Cancel        context.CancelFunc
}

// Manager manages both the foreground SSE connection and the replay buffer for
// the current/recent turn of each thread.
type Manager struct {
	connections map[string]*Connection
	turns       map[string]*turnState
	mu          sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		connections: make(map[string]*Connection),
		turns:       make(map[string]*turnState),
	}
}

func (m *Manager) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pruneExpiredState()
		}
	}
}

func (m *Manager) UpdateLastActive(threadID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conn, ok := m.connections[threadID]
	if !ok || conn == nil {
		return
	}
	conn.LastActive = time.Now()
}

// Register keeps the historical "bare connection" behavior used by tests and
// other best-effort consumers that do not participate in turn replay.
func (m *Manager) Register(threadID string, _ context.Context) *Connection {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registerConnectionLocked(threadID, "")
}

// BeginOrReattachTurn attaches a fresh HTTP SSE connection to a turn. It
// returns whether the caller should start a new background turn execution.
func (m *Manager) BeginOrReattachTurn(
	threadID string,
	turnRequestID string,
	lastEventID int64,
	turnCancel context.CancelFunc,
) (*Connection, []*Event, bool, error) {
	threadID = strings.TrimSpace(threadID)
	turnRequestID = strings.TrimSpace(turnRequestID)
	if threadID == "" || turnRequestID == "" {
		if turnCancel != nil {
			turnCancel()
		}
		return nil, nil, false, ErrTurnNotFound
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if state, ok := m.turns[threadID]; ok && state != nil {
		if state.TurnRequestID == turnRequestID {
			if turnCancel != nil {
				turnCancel()
			}
			conn := m.registerConnectionLocked(threadID, turnRequestID)
			state.Connection = conn
			return conn, replayEventsAfter(state.Events, lastEventID), false, nil
		}
		if state.Running {
			if turnCancel != nil {
				turnCancel()
			}
			return nil, nil, false, ErrTurnConflict
		}
		if lastEventID > 0 {
			if turnCancel != nil {
				turnCancel()
			}
			return nil, nil, false, ErrTurnNotFound
		}
	}

	if lastEventID > 0 {
		if turnCancel != nil {
			turnCancel()
		}
		return nil, nil, false, ErrTurnNotFound
	}

	state := &turnState{
		ThreadID:      threadID,
		TurnRequestID: turnRequestID,
		Events:        make([]*Event, 0, 64),
		NextEventID:   1,
		Running:       true,
		StartedAt:     time.Now(),
		Cancel:        turnCancel,
	}
	conn := m.registerConnectionLocked(threadID, turnRequestID)
	state.Connection = conn
	m.turns[threadID] = state
	return conn, nil, true, nil
}

// Publish appends an event to the replay buffer of the current turn, then
// forwards it to the live SSE connection if there is one.
func (m *Manager) Publish(meta op.Meta, content op.Content) bool {
	threadID, turnRequestID := metaIDs(meta)
	if threadID == "" {
		slog.Error("threadID not found in event meta", "meta", meta)
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	state := m.turns[threadID]
	if state != nil && (turnRequestID == "" || state.TurnRequestID == turnRequestID) {
		msgMeta := meta.Clone()
		if state.TurnRequestID != "" {
			msgMeta["turnRequestID"] = state.TurnRequestID
		}
		event := &Event{
			ID: state.NextEventID,
			Message: &op.GeneralContent{
				Meta:    msgMeta,
				Content: content,
			},
		}
		state.NextEventID += 1
		state.Events = append(state.Events, event)
		if isEndEvent(msgMeta) {
			state.Running = false
			state.CompletedAt = now
		}
		if conn := m.connections[threadID]; conn != nil {
			conn.LastActive = now
			return sendEventNonBlocking(conn, event)
		}
		return true
	}

	conn := m.connections[threadID]
	if conn == nil {
		slog.Warn("no connection found for threadID", "threadID", threadID)
		return false
	}

	conn.LastActive = now
	return sendEventNonBlocking(conn, &Event{
		Message: &op.GeneralContent{
			Meta:    meta.Clone(),
			Content: content,
		},
	})
}

func (m *Manager) Broadcast(msg *op.GeneralContent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for threadID, conn := range m.connections {
		if conn == nil {
			continue
		}
		conn.LastActive = time.Now()
		if !sendEventNonBlocking(conn, &Event{
			Message: &op.GeneralContent{
				Meta:    msg.Meta.Clone(),
				Content: msg.Content,
			},
		}) {
			slog.Warn("connection channel full during broadcast", "threadID", threadID)
		}
	}
}

func (m *Manager) Unregister(threadID string, conn *Connection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unregisterConnectionLocked(threadID, conn)
}

func (m *Manager) CancelTurn(threadID string) bool {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return false
	}

	var cancel context.CancelFunc
	m.mu.RLock()
	if state := m.turns[threadID]; state != nil && state.Running {
		cancel = state.Cancel
	}
	m.mu.RUnlock()

	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (m *Manager) IsTurnComplete(threadID string, turnRequestID string) bool {
	threadID = strings.TrimSpace(threadID)
	turnRequestID = strings.TrimSpace(turnRequestID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	state := m.turns[threadID]
	return state != nil && state.TurnRequestID == turnRequestID && !state.Running
}

func (m *Manager) HasRunningTurn(threadID string, turnRequestID string) bool {
	threadID = strings.TrimSpace(threadID)
	turnRequestID = strings.TrimSpace(turnRequestID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	state := m.turns[threadID]
	return state != nil && state.TurnRequestID == turnRequestID && state.Running
}

func (m *Manager) IsConnected(threadID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.connections[threadID]
	return exists
}

func (m *Manager) GetConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

func (m *Manager) GetAllThreadIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	threadIDs := make([]string, 0, len(m.connections))
	for threadID := range m.connections {
		threadIDs = append(threadIDs, threadID)
	}
	return threadIDs
}

type ConnectionInfo struct {
	ThreadID   string    `json:"thread_id"`
	LastActive time.Time `json:"last_active"`
}

func (m *Manager) GetAllConnections() []ConnectionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]ConnectionInfo, 0, len(m.connections))
	for threadID, conn := range m.connections {
		if conn == nil {
			continue
		}
		out = append(out, ConnectionInfo{
			ThreadID:   threadID,
			LastActive: conn.LastActive,
		})
	}
	return out
}

func (m *Manager) pruneExpiredState() {
	now := time.Now()
	type connectionToClose struct {
		threadID string
		conn     *Connection
	}

	staleConnections := make([]connectionToClose, 0)

	m.mu.Lock()
	for threadID, conn := range m.connections {
		if conn == nil {
			continue
		}
		if conn.LastActive.IsZero() || now.Sub(conn.LastActive) > defaultExpiration {
			staleConnections = append(staleConnections, connectionToClose{threadID: threadID, conn: conn})
		}
	}
	for _, stale := range staleConnections {
		m.unregisterConnectionLocked(stale.threadID, stale.conn)
	}
	for threadID, state := range m.turns {
		if state == nil {
			delete(m.turns, threadID)
			continue
		}
		if state.Running || state.CompletedAt.IsZero() {
			continue
		}
		if now.Sub(state.CompletedAt) > turnReplayGrace {
			delete(m.turns, threadID)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) registerConnectionLocked(threadID string, turnRequestID string) *Connection {
	if oldConn, exists := m.connections[threadID]; exists && oldConn != nil {
		slog.Info("existing connection for threadID, closing old one", "threadID", threadID)
		oldConn.Cancel()
		if oldConn.SSEChan != nil {
			close(oldConn.SSEChan)
		}
	}

	connCtx, cancel := context.WithCancel(context.Background())
	conn := &Connection{
		ThreadID:      threadID,
		TurnRequestID: strings.TrimSpace(turnRequestID),
		SSEChan:       make(chan *Event, sseChannelBufferSize),
		Ctx:           connCtx,
		Cancel:        cancel,
		LastActive:    time.Now(),
	}
	m.connections[threadID] = conn
	slog.Info("registered SSE connection", "threadID", threadID, "turnRequestID", turnRequestID)
	return conn
}

func (m *Manager) unregisterConnectionLocked(threadID string, conn *Connection) {
	cur, exists := m.connections[threadID]
	if !exists || cur == nil {
		return
	}
	if conn != nil && cur != conn {
		return
	}

	cur.Cancel()
	if cur.SSEChan != nil {
		close(cur.SSEChan)
	}
	delete(m.connections, threadID)

	if state := m.turns[threadID]; state != nil && state.Connection == cur {
		state.Connection = nil
	}

	slog.Info("unregistered SSE connection", "threadID", threadID)
}

func sendEventNonBlocking(conn *Connection, event *Event) bool {
	select {
	case conn.SSEChan <- event:
		return true
	default:
		slog.Warn("connection channel full, dropping message", "threadID", conn.ThreadID)
		return false
	}
}

func replayEventsAfter(events []*Event, lastEventID int64) []*Event {
	if len(events) == 0 {
		return nil
	}
	if lastEventID < 0 {
		lastEventID = 0
	}
	out := make([]*Event, 0, len(events))
	for _, event := range events {
		if event == nil || event.Message == nil {
			continue
		}
		if event.ID <= lastEventID {
			continue
		}
		out = append(out, event)
	}
	return out
}

func metaIDs(meta op.Meta) (threadID string, turnRequestID string) {
	threadID, _ = meta["threadID"].(string)
	turnRequestID, _ = meta["turnRequestID"].(string)
	return strings.TrimSpace(threadID), strings.TrimSpace(turnRequestID)
}

func isEndEvent(meta op.Meta) bool {
	typ, _ := meta["type"].(string)
	return strings.TrimSpace(typ) == "end"
}

func ParseLastEventID(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}
