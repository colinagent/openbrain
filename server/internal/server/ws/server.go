package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/fs"
	"github.com/colinagent/openbrain/server/internal/server/protocol"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// Version is the server version string (overridden at build time).
var Version = "dev"

// Server represents the WebSocket server
type Server struct {
	addr      string
	verbose   bool
	clients   map[*Client]bool
	mu        sync.RWMutex
	handler   *Handler
	watcher   *fs.Watcher
	sessionMu sync.RWMutex
	session   *op.ServerSession
	// sessionStore *sessionstore.Store
}

// NewServer creates a new WebSocket server
func NewServer(addr string, verbose bool) *Server {
	s := &Server{
		addr:    addr,
		verbose: verbose,
		clients: make(map[*Client]bool),
	}
	s.handler = NewHandler(s, verbose)

	// Initialize watcher
	var err error
	s.watcher, err = fs.NewWatcher(verbose, s.onFileChange)
	if err != nil {
		log.Printf("Warning: Failed to create file watcher: %v", err)
	}

	// Initialize session store
	// store, err := sessionstore.New("")
	// if err != nil {
	// 	log.Printf("Warning: Failed to create session store: %v", err)
	// }
	// s.sessionStore = store

	return s
}

// RegisterHandlers registers WebSocket and utility handlers on router.
func (s *Server) RegisterHandlers(r gin.IRouter) {
	r.GET("/ws", s.handleWebSocket)
	r.GET("/health", s.handleHealth)
	r.GET("/version", s.handleVersion)
}

// Start starts the WebSocket server on its addr using default mux.
func (s *Server) Start() error {
	if s.verbose {
		log.Printf("WebSocket server starting on %s", s.addr)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	s.RegisterHandlers(router)
	return http.ListenAndServe(s.addr, router)
}

// // onFileChange handles file change events from watcher
func (s *Server) onFileChange(subID string, changes []fs.FileChange) {
	// Convert to protocol format
	protoChanges := make([]protocol.FileChange, len(changes))
	for i, c := range changes {
		protoChanges[i] = protocol.FileChange{
			Type: c.Type.String(),
			Path: c.Path,
		}
	}

	event := protocol.FileChangeEvent{
		WatchID: subID,
		Changes: protoChanges,
	}

	notification := protocol.NewNotification("fs/fileChange", event)
	data, _ := json.Marshal(notification)

	// Snapshot the target clients under s.mu, then send after unlock. Sending can
	// trigger cleanup paths and must never run while server state is locked.
	recipients := make([]*Client, 0)
	s.mu.RLock()
	for client := range s.clients {
		if client.HasWatch(subID) {
			recipients = append(recipients, client)
		}
	}
	s.mu.RUnlock()

	for _, client := range recipients {
		if !client.Send(data) {
			s.unregisterClient(client)
		}
	}
}

func (s *Server) handleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := NewClient(conn, s)
	s.registerClient(client)

	if s.verbose {
		log.Printf("Client connected: %s", conn.RemoteAddr())
	}

	go client.ReadPump()
	go client.WritePump()
}

func (s *Server) handleHealth(c *gin.Context) {
	s.mu.RLock()
	clientCount := len(s.clients)
	s.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"clients": clientCount,
		"version": Version,
	})
}

func (s *Server) handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": Version,
	})
}

func (s *Server) registerClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[c] = true
}

func (s *Server) unregisterClient(c *Client) {
	if c == nil {
		return
	}

	s.mu.Lock()
	_, ok := s.clients[c]
	if ok {
		delete(s.clients, c)
	}
	s.mu.Unlock()

	if !ok {
		return
	}

	// Cross-module cleanup must happen outside server.mu. Watcher callbacks can
	// call back into Server, so holding server.mu here would recreate the lock
	// inversion that freezes new WS connections.
	if s.watcher != nil {
		s.watcher.UnwatchClient(c)
	}
	c.Close()
}

// Broadcast sends a message to all connected clients
func (s *Server) Broadcast(message []byte) {
	recipients := make([]*Client, 0)
	s.mu.RLock()
	for client := range s.clients {
		recipients = append(recipients, client)
	}
	s.mu.RUnlock()

	for _, client := range recipients {
		if !client.Send(message) {
			go s.unregisterClient(client)
		}
	}
}

func (s *Server) BroadcastMessengerMessage(record op.MessageRecord) {
	notification := protocol.NewNotification("messenger/message", record)
	data, err := json.Marshal(notification)
	if err != nil {
		return
	}
	s.Broadcast(data)
}

// GetHandler returns the request handler
func (s *Server) GetHandler() *Handler {
	return s.handler
}

// GetWatcher returns the file watcher
func (s *Server) GetWatcher() *fs.Watcher {
	return s.watcher
}

// // SetSession sets the OpAgent host session
func (s *Server) SetHostSession(session *op.ServerSession) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	s.session = session
}

// GetSession returns the OpAgent host session
func (s *Server) GetHostSession() *op.ServerSession {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	return s.session
}

// // GetSessionStore returns the session file store
// func (s *Server) GetSessionStore() *sessionstore.Store {
// 	return s.sessionStore
// }
