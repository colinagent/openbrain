package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
)

const (
	daemonRetryBaseDelay = 30 * time.Second
	daemonRetryMaxDelay  = 5 * time.Minute
)

type createConnectionOptions struct {
	httpClient              *http.Client
	skipCache               bool
	suppressConnectErrorLog bool
}

type daemonRetryState struct {
	Failures  int
	NextRetry time.Time
}

var daemonRetryByNodeID sync.Map     // map[string]daemonRetryState
var connectionRecoveryLocks sync.Map // map[string]*sync.Mutex

type Connection struct {
	NodeID string `json:"nodeID"`
	Name   string `json:"name"`
	// ConnType    ConnType           `json:"connType"`
	Meta        op.Meta                 `json:"meta"`
	TransType   op.TransportType        `json:"type"`
	Command     []string                `json:"command"`
	OpCodes     []op.OpCode             `json:"opcodes"`
	URL         string                  `json:"url"`
	Description string                  `json:"description"`
	Daemon      bool                    `json:"daemon"`
	Session     *op.ClientSession       `json:"-"`
	Ctx         context.Context         `json:"-"`
	Cancel      context.CancelFunc      `json:"-"`
	runtime     *connectionRuntimeState `json:"-"`
}

func CreateConnection(ctx context.Context, node *op.OpNode) (*Connection, error) {
	return createConnection(ctx, node, createConnectionOptions{})
}

func createConnection(ctx context.Context, node *op.OpNode, opts createConnectionOptions) (*Connection, error) {
	if node == nil {
		return nil, fmt.Errorf("node is nil")
	}
	if !node.Run.HasEndpoint() {
		return nil, fmt.Errorf("node %s has no endpoint", node.ID)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Daemon connections should be long-lived and must not inherit per-request
	// cancellations/deadlines (e.g. thread/submit legacy chat requests). They are cleaned up explicitly on
	// shutdown via CloseDaemonConnections().
	baseCtx := ctx
	if node.Run.Daemon {
		baseCtx = context.Background()
	}
	connCtx, cancel := context.WithCancel(baseCtx)
	name, description := resolveConnectionMetadata(node)
	conn := &Connection{
		NodeID:      node.ID,
		Name:        name,
		OpCodes:     node.OpCodes,
		Command:     node.Run.Command,
		URL:         node.Run.URL,
		Description: description,
		Daemon:      node.Run.Daemon,
		Ctx:         connCtx,
		Cancel:      cancel,
		runtime:     newConnectionRuntimeState(),
	}

	cliOpts := &op.ClientOptions{
		KeepAlive: time.Second * 30,
		InfoNotificationHandler: func(ctx context.Context, req *op.InfoNotificationClientRequest) {
			conn.markIncomingProtocolTraffic()
			NotifyProgress(op.NotifyMessage, req.Params.Meta, req.Params.Content)
		},
		OpAgentHandler: func(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			conn.markIncomingProtocolTraffic()
			return OpAgentHandler(ctx, req)
		},
		OpNodeHandler: func(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			conn.markIncomingProtocolTraffic()
			return OpNodeHandler(ctx, req)
		},
	}
	client := op.NewClient(&op.Implementation{Name: "client", Version: "v1.0.0"}, cliOpts)

	if len(node.Run.Command) > 0 {
		conn.TransType = op.Stdio
		session, pid, startedAt, err := stdioConn(connCtx, node, client)
		if err != nil {
			if !opts.suppressConnectErrorLog {
				slog.Error("connect (stdio)", "command", strings.Join(node.Run.Command, " "), "nodeURI", node.URI, "error", err)
			}
			cancel()
			return nil, fmt.Errorf("connect (stdio) failed: %v", err)
		}
		conn.Session = session
		conn.setConnectedAt(startedAt)
		if pid > 0 {
			conn.setProcessRuntime(pid, startedAt)
		}
	} else if node.Run.URL != "" {
		conn.TransType = op.HttpStreamable
		session, err := httpConn(connCtx, node, client, opts.httpClient)
		if err != nil {
			if !opts.suppressConnectErrorLog {
				slog.Error("connect (httpstream)", "url", node.Run.URL, "error", err)
			}
			cancel()
			return nil, fmt.Errorf("connect (httpstream) failed: %v", err)
		}
		conn.Session = session
		conn.setConnectedAt(time.Now().UTC())
	}

	conn.Session.SetLoggingLevel(connCtx, &op.SetLoggingLevelParams{Level: "info"})

	go func() {
		<-connCtx.Done()
		conn.Close()
	}()

	if !opts.skipCache {
		SetConn(conn)
	}

	return conn, nil
}

func SetConn(conn *Connection) {
	if conn == nil {
		return
	}
	if conn.Daemon {
		cache.Set(conn.NodeID, cache.PrefixConnection, conn, cache.NoExpiration)
	} else {
		cache.Set(conn.NodeID, cache.PrefixConnection, conn, cache.ShortExpiration)
	}
}

func GetConn(nodeID string) *Connection {
	conn := cache.Get[Connection](nodeID, cache.PrefixConnection)
	if conn == nil {
		return nil
	}
	if !conn.Daemon {
		// update expiration time
		cache.Set(conn.NodeID, cache.PrefixConnection, conn, cache.ShortExpiration)
	}
	return conn
}

func EnsureConnection(ctx context.Context, node *op.OpNode) (*Connection, error) {
	if node == nil {
		return nil, fmt.Errorf("node is nil")
	}
	conn := GetConn(node.ID)
	if conn != nil {
		return conn, nil
	}
	if node.Run.Daemon {
		lock := connectionRecoveryMutex(node.ID)
		lock.Lock()
		defer lock.Unlock()
		conn = GetConn(node.ID)
		if conn != nil {
			return conn, nil
		}
	}
	return CreateConnection(ctx, node)
}

func recoverConnection(ctx context.Context, node *op.OpNode, failed *Connection) (*Connection, error) {
	if node == nil {
		return nil, fmt.Errorf("node is nil")
	}
	if strings.TrimSpace(node.ID) == "" {
		return nil, fmt.Errorf("node ID is required")
	}
	lock := connectionRecoveryMutex(node.ID)
	lock.Lock()
	defer lock.Unlock()

	current := GetConn(node.ID)
	if current != nil && current.Session != nil && current != failed {
		return current, nil
	}
	if current != nil {
		current.ForceClose()
	} else if failed != nil {
		failed.ForceClose()
	}
	return CreateConnection(ctx, node)
}

func connectionRecoveryMutex(nodeID string) *sync.Mutex {
	lockValue, _ := connectionRecoveryLocks.LoadOrStore(nodeID, &sync.Mutex{})
	return lockValue.(*sync.Mutex)
}

func nodeRunLogBaseName(node *op.OpNode) string {
	if node == nil {
		return ""
	}
	p := op.URIToPath(node.URI)
	if p == "" {
		return ""
	}
	dir := filepath.Dir(p)
	base := filepath.Base(dir)
	if base == ".agent" || base == ".agents" {
		base = filepath.Base(filepath.Dir(dir))
	}
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

func nodeStderrWriter(node *op.OpNode) io.Writer {
	sys := config.GetSystem()
	baseDir := ""
	if sys != nil {
		baseDir = strings.TrimSpace(sys.BaseDir)
	}
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			baseDir = filepath.Join(home, ".openbrain")
		}
	}
	if baseDir == "" {
		return os.Stderr
	}

	runDir := filepath.Join(baseDir, "run")
	_ = os.MkdirAll(runDir, 0o755)

	name := nodeRunLogBaseName(node)
	if name == "" {
		name = "node"
	}
	logPath := filepath.Join(runDir, name+".log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return os.Stderr
	}

	// NOTE: We intentionally do not close the file handle here. The exec child
	// will inherit the FD, and the daemon stays alive across requests. The number
	// of daemon nodes is small, so this is acceptable.
	return io.MultiWriter(os.Stderr, f)
}

func stdioConn(ctx context.Context, node *op.OpNode, client *op.Client) (session *op.ClientSession, pid int, startedAt time.Time, err error) {
	if node == nil {
		return nil, 0, time.Time{}, fmt.Errorf("node is nil")
	}
	command := node.Run.Command
	if len(command) == 0 {
		return nil, 0, time.Time{}, fmt.Errorf("stdio command is empty")
	}

	cmdPath := command[0]
	cwd := strings.TrimSpace(node.Cwd)
	if cmdPath != "" && !filepath.IsAbs(cmdPath) && cwd != "" {
		cmdPath = filepath.Join(cwd, cmdPath)
		command = append([]string{cmdPath}, command[1:]...)
	}

	if _, err := os.Stat(cmdPath); err != nil {
		return nil, 0, time.Time{}, fmt.Errorf("agent command not found: %s: %w", cmdPath, err)
	}

	cmd := exec.CommandContext(ctx, cmdPath, command[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = common.WithPrependedPath(nil, common.OpagentBinDir())
	if sys := config.GetSystem(); sys != nil && strings.TrimSpace(sys.BaseDir) != "" {
		cmd.Env = append(cmd.Env, "OPENBRAIN_BASE_DIR="+strings.TrimSpace(sys.BaseDir))
	}
	cmd.Stderr = nodeStderrWriter(node)

	transport := &op.CommandTransport{Command: cmd}
	session, err = client.Connect(ctx, op.Transport(transport), nil)
	if err != nil {
		isInitEOF := strings.Contains(err.Error(), `calling "initialize": EOF`)
		if isInitEOF {
			slog.Error(
				"connect (stdio) initialize EOF",
				"command", strings.Join(command, " "),
				"hint", "child process exited before protocol handshake; check daemon pid lock and whether another instance is already running",
			)
		}
		if isInitEOF {
			slog.Warn("connect (stdio)", "command", strings.Join(command, " "), "error", err)
		} else {
			slog.Error("connect (stdio)", "command", strings.Join(command, " "), "error", err)
		}

		return nil, 0, time.Time{}, fmt.Errorf("connect (stdio) failed: %v", err)
	}
	startedAt = time.Now().UTC()
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	return session, pid, startedAt, nil
}

func httpConn(ctx context.Context, node *op.OpNode, client *op.Client, httpClient *http.Client) (session *op.ClientSession, err error) {
	if node == nil {
		return nil, fmt.Errorf("node is nil")
	}
	if httpClient == nil {
		httpClient = newRunHeaderHTTPClient(node.Run.Header)
	}
	transport := &op.StreamableClientTransport{
		Endpoint:   strings.TrimSpace(node.Run.URL),
		HTTPClient: httpClient,
	}
	session, err = client.Connect(ctx, op.Transport(transport), nil)
	if err != nil {
		return nil, fmt.Errorf("connect (httpstream) failed: %v", err)
	}
	return session, nil
}

func (conn *Connection) Close() {
	if conn.Daemon {
		return
	}
	if conn.Session != nil {
		conn.Session.Close()
	}

	cache.Delete(conn.NodeID, cache.PrefixConnection)
}

func (conn *Connection) ForceClose() {
	if conn == nil {
		return
	}
	if conn.Cancel != nil {
		conn.Cancel()
	}
	if conn.Session != nil {
		_ = conn.Session.Close()
		conn.Session = nil
	}
	cache.Delete(conn.NodeID, cache.PrefixConnection)
}

// CloseDaemonConnections force closes all cached daemon connections.
// Used during opagent shutdown to avoid orphan daemon child processes.
func CloseDaemonConnections() int {
	closed := 0
	for _, conn := range cache.ListByPrefix[Connection](cache.PrefixConnection) {
		if !conn.Daemon {
			continue
		}
		conn.ForceClose()
		closed++
	}
	if closed > 0 {
		slog.Info("daemon connections closed", "count", closed)
	}
	return closed
}

func (conn *Connection) CallNode(ctx context.Context, meta op.Meta, content op.Content) (*op.CallNodeResult, error) {
	if conn.Session == nil {
		return nil, fmt.Errorf("connection session is nil")
	}
	if conn.NodeID == "" {
		return nil, fmt.Errorf("connection ID is required")
	}
	callNodeParams := &op.CallNodeParams{
		Meta:    meta,
		Content: content,
	}
	result, err := conn.Session.CallNode(ctx, callNodeParams)
	if err != nil {
		return nil, err
	}
	conn.markOutgoingProtocolTraffic()
	return result, nil
}

func (conn *Connection) CallAgent(ctx context.Context, agentID string, meta op.Meta, content op.Content) (*op.CallAgentResult, error) {
	if conn == nil || conn.Session == nil {
		return nil, fmt.Errorf("connection session is nil")
	}
	if strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("agentID is required")
	}
	result, err := conn.Session.CallAgent(ctx, &op.CallAgentParams{
		AgentID: strings.TrimSpace(agentID),
		Meta:    meta,
		Content: content,
	})
	if err != nil {
		return nil, err
	}
	conn.markOutgoingProtocolTraffic()
	return result, nil
}

func (conn *Connection) OpNode(ctx context.Context, params *op.OpNodeParams) (*op.OpNodeResult, error) {
	if conn == nil || conn.Session == nil {
		return nil, fmt.Errorf("connection session is nil")
	}
	result, err := conn.Session.OpNode(ctx, params)
	if err != nil {
		return nil, err
	}
	conn.markOutgoingProtocolTraffic()
	return result, nil
}

func (conn *Connection) ListToolSpecs() ([]*op.ToolSpec, error) {
	return conn.ListToolSpecsContext(conn.Ctx)
}

func (conn *Connection) ListToolSpecsContext(ctx context.Context) ([]*op.ToolSpec, error) {
	if ctx == nil {
		ctx = conn.Ctx
	}
	toolSpecs := make([]*op.ToolSpec, 0)
	toolResult, err := conn.Session.ListTools(ctx, &op.ListToolsParams{})
	if err != nil {
		return nil, err
	}
	conn.markOutgoingProtocolTraffic()
	for _, tool := range toolResult.Tools {
		if tool.Sampling {
			continue
		}
		toolSpecs = append(toolSpecs, &op.ToolSpec{
			ServerID:    conn.NodeID,
			Name:        tool.Name,
			Description: tool.Description,
			Sampling:    tool.Sampling,
			InputSchema: tool.InputSchema,
		})
	}
	return toolSpecs, nil
}

func (conn *Connection) CallTool(ctx context.Context, params *op.CallToolParams) (*op.CallToolResult, error) {
	if conn == nil || conn.Session == nil {
		return nil, fmt.Errorf("connection session is nil")
	}
	result, err := conn.Session.CallTool(ctx, params)
	if err != nil {
		return nil, err
	}
	conn.markOutgoingProtocolTraffic()
	return result, nil
}

func (conn *Connection) NotifyInfo(ctx context.Context, params *op.InfoNotificationParams) error {
	if conn == nil || conn.Session == nil {
		return fmt.Errorf("connection session is nil")
	}
	if err := conn.Session.NotifyInfo(ctx, params); err != nil {
		return err
	}
	conn.markOutgoingProtocolTraffic()
	return nil
}

// func (conn *Connection) CallAgent(ctx context.Context, agentID string, meta op.Meta, content op.Content) (*op.CallAgentResult, error) {
// 	if conn.Session == nil {
// 		return nil, fmt.Errorf("connection session is nil")
// 	}
// 	if conn.ID == "" {
// 		return nil, fmt.Errorf("connection ID is required")
// 	}
// 	callAgentParams := &op.CallAgentParams{
// 		AgentID: agentID,
// 		Meta:    meta,
// 		Content: content,
// 	}
// 	return conn.Session.CallAgent(ctx, callAgentParams)
// }

// func (conn *Connection) CallTool(ctx context.Context, agentID string, meta op.Meta, content op.Content) (*op.CallAgentResult, error) {
// 	if conn.Session == nil {
// 		return nil, fmt.Errorf("connection session is nil")
// 	}
// 	if conn.ID == "" {
// 		return nil, fmt.Errorf("connection ID is required")
// 	}
// 	callAgentParams := &op.CallAgentParams{
// 		AgentID: agentID,
// 		Meta:    meta,
// 		Content: content,
// 	}
// 	return conn.Session.CallAgent(ctx, callAgentParams)
// }
