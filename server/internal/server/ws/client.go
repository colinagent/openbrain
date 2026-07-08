package ws

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 10 * 1024 * 1024 // 10MB
)

// Client represents a WebSocket client connection
type Client struct {
	conn   *websocket.Conn
	server *Server
	send   chan []byte
	mu     sync.Mutex
	sendMu sync.Mutex
	closed bool

	// Watch subscriptions for this client
	watches map[string]bool
	watchMu sync.RWMutex
}

// NewClient creates a new WebSocket client
func NewClient(conn *websocket.Conn, server *Server) *Client {
	return &Client{
		conn:    conn,
		server:  server,
		send:    make(chan []byte, 256),
		watches: make(map[string]bool),
	}
}

// ReadPump pumps messages from the WebSocket connection to the handler
func (c *Client) ReadPump() {
	defer func() {
		c.server.unregisterClient(c)
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		if shouldHandleRequestAsync(message) {
			msgCopy := append([]byte(nil), message...)
			go c.handleMessageAsync(msgCopy)
			continue
		}

		response := c.server.GetHandler().HandleMessage(c, message)
		if response != nil && !c.Send(response) {
			c.server.unregisterClient(c)
			return
		}
	}
}

func (c *Client) handleMessageAsync(message []byte) {
	response := c.server.GetHandler().HandleMessage(c, message)
	if response != nil && !c.Send(response) {
		c.server.unregisterClient(c)
	}
}

func shouldHandleRequestAsync(message []byte) bool {
	var req struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(message, &req); err != nil {
		return false
	}
	switch req.Method {
	case protocol.MethodFSStat,
		protocol.MethodFSReadFile,
		protocol.MethodFSReaddir,
		protocol.MethodFSSearch,
		protocol.MethodAgentScan,
		protocol.MethodNodeList,
		protocol.MethodGitBranches,
		protocol.MethodStorageStatus,
		protocol.MethodStorageSyncNow,
		protocol.MethodStorageUpdatePolicy,
		protocol.MethodMessengerReply:
		return true
	case protocol.MethodEditorCompletion:
		return true
	default:
		return false
	}
}

// WritePump pumps messages from the send channel to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				return
			}

			c.mu.Lock()
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteMessage(websocket.TextMessage, message)
			c.mu.Unlock()

			if err != nil {
				return
			}

		case <-ticker.C:
			c.mu.Lock()
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// Send queues a message for this client.
//
// Important: this must stay non-blocking and must not call back into Server.
// Callers may hold server/watcher locks, so cleanup has to happen explicitly
// after Send returns false.
func (c *Client) Send(message []byte) bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if c.closed {
		return false
	}

	select {
	case c.send <- message:
		return true
	default:
		return false
	}
}

// Close closes the client channel and socket exactly once.
//
// Close races with Send during disconnect storms, so channel close is guarded
// by sendMu to avoid send-on-closed-channel panics.
func (c *Client) Close() {
	c.sendMu.Lock()
	if c.closed {
		c.sendMu.Unlock()
		return
	}
	c.closed = true
	close(c.send)
	c.sendMu.Unlock()

	c.mu.Lock()
	_ = c.conn.Close()
	c.mu.Unlock()
}

// AddWatch adds a watch subscription
func (c *Client) AddWatch(watchID string) {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	c.watches[watchID] = true
}

// RemoveWatch removes a watch subscription
func (c *Client) RemoveWatch(watchID string) {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	delete(c.watches, watchID)
}

// HasWatch checks if client has a watch subscription
func (c *Client) HasWatch(watchID string) bool {
	c.watchMu.RLock()
	defer c.watchMu.RUnlock()
	return c.watches[watchID]
}

// GetWatches returns all watch IDs for this client
func (c *Client) GetWatches() []string {
	c.watchMu.RLock()
	defer c.watchMu.RUnlock()
	watches := make([]string, 0, len(c.watches))
	for id := range c.watches {
		watches = append(watches, id)
	}
	return watches
}
