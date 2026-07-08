package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/colinagent/openbrain/server/internal/server/notify"
	"github.com/colinagent/openbrain/server/internal/server/sse"
	"github.com/gin-gonic/gin"
)

func TestStreamHandler_RequiresThreadID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := op.GeneralContent{
		Meta: op.Meta{
			"opcode":        string(op.OpThreadSubmit),
			"chatPath":      "/tmp/chat.md",
			"agentID":       "agent-id",
			"modelKey":      "test:model",
			"turnRequestID": "turn-1",
		},
		Content: &op.TextContent{Text: "hello"},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}

	req := httptest.NewRequest("POST", "/v1/chat/stream", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/chat/stream", NewHandler(sse.NewManager(), NewService(nil)).Stream)
	router.ServeHTTP(rr, req)

	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "threadID is required") {
		t.Fatalf("expected threadID validation error, got %s", rr.Body.String())
	}
}

func TestStreamHandler_RejectsMismatchedThreadTriple(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, _ *op.OpAgentRequest) (*op.OpAgentResult, error) {
			return nil, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req.Params.OpCode != op.OpThreadMetaGet {
				t.Fatalf("received node opcode %s, want %s", req.Params.OpCode, op.OpThreadMetaGet)
			}
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-other",
				AgentID:  "agent-id",
				FileID:   "file-test",
				ChatPath: "/tmp/chat.md",
			})
			if err != nil {
				t.Fatalf("marshal thread meta: %v", err)
			}
			return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
		},
	)
	defer cleanup()

	body := op.GeneralContent{
		Meta: op.Meta{
			"opcode":        string(op.OpThreadSubmit),
			"threadID":      "thread-test",
			"fileID":        "file-test",
			"chatPath":      "/tmp/chat.md",
			"agentID":       "agent-id",
			"modelKey":      "test:model",
			"turnRequestID": "turn-1",
		},
		Content: &op.TextContent{Text: "hello"},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}

	req := httptest.NewRequest("POST", "/v1/chat/stream", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/chat/stream", NewHandler(sse.NewManager(), service).Stream)
	router.ServeHTTP(rr, req)

	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "threadID does not match thread metadata") {
		t.Fatalf("expected thread mismatch error, got %s", rr.Body.String())
	}
}

func TestValidateThreadMeta_AllowsDifferentAgentIDForSameThread(t *testing.T) {
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			t.Fatalf("unexpected OpAgent call: %s", req.Params.OpCode)
			return nil, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req.Params.OpCode != op.OpThreadMetaGet {
				t.Fatalf("received node opcode %s, want %s", req.Params.OpCode, op.OpThreadMetaGet)
			}
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-test",
				AgentID:  "old-agent",
				FileID:   "file-test",
				ChatPath: "/tmp/chat.md",
			})
			if err != nil {
				t.Fatalf("marshal thread meta: %v", err)
			}
			return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
		},
	)
	defer cleanup()

	meta, err := service.validateThreadMeta(context.Background(), op.Meta{
		"threadID": "thread-test",
		"fileID":   "file-test",
		"chatPath": "/tmp/chat.md",
		"agentID":  "new-agent",
	})
	if err != nil {
		t.Fatalf("validateThreadMeta(): %v", err)
	}
	if meta.ThreadID != "thread-test" || meta.ChatPath != "/tmp/chat.md" {
		t.Fatalf("unexpected thread meta: %+v", meta)
	}
}

type streamTestHarness struct {
	service    *Service
	manager    *sse.Manager
	server     *httptest.Server
	cleanup    func()
	baseURL    string
	httpClient *http.Client
}

func newStreamTestHarness(
	t *testing.T,
	agentHandler func(context.Context, *op.OpAgentRequest) (*op.OpAgentResult, error),
	nodeHandler func(context.Context, *op.OpNodeRequest) (*op.OpNodeResult, error),
) *streamTestHarness {
	t.Helper()

	manager := sse.NewManager()
	notifySvc := notify.NewService(manager)
	var service *Service

	server := op.NewServer(&op.Implementation{Name: "host", Version: "v0.0.1"}, &op.ServerOptions{
		InfoNotificationHandler: func(ctx context.Context, req *op.InfoNotificationServerRequest) {
			service.HandleHostNotification(req)
			notifySvc.HandleHostNotification(req)
		},
	})

	t1, t2 := op.NewInMemoryTransports()
	session, err := server.Connect(context.Background(), t1, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}

	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpAgentHandler: func(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			if agentHandler == nil {
				raw, err := json.Marshal(op.ThreadControlAck{
					OK:       true,
					ThreadID: "thread-test",
					OpCode:   req.Params.OpCode,
				})
				if err != nil {
					return nil, err
				}
				return &op.OpAgentResult{
					OpCode:  req.Params.OpCode,
					Content: &op.JsonContent{Raw: raw},
				}, nil
			}
			return agentHandler(ctx, req)
		},
		OpNodeHandler: func(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req.Params.OpCode == op.OpThreadMetaGet {
				raw, err := json.Marshal(op.ThreadMeta{
					ThreadID: "thread-test",
					AgentID:  "agent-id",
					FileID:   "file-test",
					ChatPath: "/tmp/chat.md",
				})
				if err != nil {
					return nil, err
				}
				return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
			}
			return nodeHandler(ctx, req)
		},
	})
	clientSession, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}

	service = NewService(notifySvc)
	service.SetHostSession(session)

	router := gin.New()
	handler := NewHandler(manager, service)
	router.POST("/v1/chat/stream", handler.Stream)
	router.POST("/v1/chat/control", handler.Control)
	httpServer := httptest.NewServer(router)

	return &streamTestHarness{
		service:    service,
		manager:    manager,
		server:     httpServer,
		baseURL:    httpServer.URL,
		httpClient: httpServer.Client(),
		cleanup: func() {
			httpServer.Close()
			_ = clientSession.Close()
		},
	}
}

func (h *streamTestHarness) close() {
	if h.cleanup != nil {
		h.cleanup()
	}
}

func streamBody(t *testing.T, turnRequestID string) io.Reader {
	t.Helper()
	body, err := json.Marshal(op.GeneralContent{
		Meta: op.Meta{
			"opcode":        string(op.OpThreadSubmit),
			"threadID":      "thread-test",
			"fileID":        "file-test",
			"chatPath":      "/tmp/chat.md",
			"agentID":       "agent-id",
			"modelKey":      "test:model",
			"turnRequestID": turnRequestID,
		},
		Content: &op.TextContent{Text: "hello"},
	})
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	return bytes.NewReader(body)
}

type recordedSSEEvent struct {
	ID   string
	Data string
}

func readNextSSEEvent(t *testing.T, reader *bufio.Reader) recordedSSEEvent {
	t.Helper()

	var event recordedSSEEvent
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("ReadString(): %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if event.ID != "" || event.Data != "" {
				return event
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "id:"):
			event.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if event.Data == "" {
				event.Data = data
			} else {
				event.Data += "\n" + data
			}
		}
	}
}

func readSSEChanEvent(t *testing.T, conn *sse.Connection) *sse.Event {
	t.Helper()
	select {
	case event := <-conn.SSEChan:
		if event == nil || event.Message == nil {
			t.Fatal("received empty SSE event")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for SSE channel event")
	}
	return nil
}

func controlBody(t *testing.T) io.Reader {
	t.Helper()
	body, err := json.Marshal(op.GeneralContent{
		Meta: op.Meta{
			"opcode":   string(op.OpThreadInterrupted),
			"threadID": "thread-test",
			"fileID":   "file-test",
			"chatPath": "/tmp/chat.md",
			"agentID":  "agent-id",
			"modelKey": "test:model",
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	return bytes.NewReader(body)
}

func writeThreadFileForStreamTest(t *testing.T, filePath string, entries ...any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir thread dir: %v", err)
	}
	lines := make([][]byte, 0, len(entries))
	for _, entry := range entries {
		raw, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("marshal thread entry: %v", err)
		}
		lines = append(lines, raw)
	}
	if err := os.WriteFile(filePath, append(bytes.Join(lines, []byte("\n")), '\n'), 0o644); err != nil {
		t.Fatalf("write thread file: %v", err)
	}
}

func TestStreamHandler_FinishStreamTurnIfIncompletePublishesErrorAndEnd(t *testing.T) {
	manager := sse.NewManager()
	notifySvc := notify.NewService(manager)
	service := NewService(notifySvc)
	handler := NewHandler(manager, service)

	conn, _, shouldStart, err := manager.BeginOrReattachTurn("thread-test", "turn-1", 0, func() {})
	if err != nil {
		t.Fatalf("BeginOrReattachTurn(): %v", err)
	}
	defer manager.Unregister("thread-test", conn)
	if !shouldStart {
		t.Fatal("shouldStart = false, want true")
	}

	handler.finishStreamTurnIfIncomplete(op.Meta{
		"threadID":      "thread-test",
		"turnRequestID": "turn-1",
	}, errors.New("boom"))

	errorEvent := readSSEChanEvent(t, conn)
	if got, _ := errorEvent.Message.Meta["type"].(string); got != "error" {
		t.Fatalf("first event type = %q, want error", got)
	}
	if text, ok := errorEvent.Message.Content.(*op.TextContent); !ok || text.Text != "boom" {
		t.Fatalf("first event content = %#v, want boom text", errorEvent.Message.Content)
	}

	endEvent := readSSEChanEvent(t, conn)
	if got, _ := endEvent.Message.Meta["type"].(string); got != "end" {
		t.Fatalf("second event type = %q, want end", got)
	}
	if !manager.IsTurnComplete("thread-test", "turn-1") {
		t.Fatal("turn is not complete after fallback end")
	}
}

func TestStreamHandler_ReattachesRunningTurnWithoutReplayingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	releaseTurn := make(chan struct{})
	firstEventSent := make(chan struct{})
	var once sync.Once
	var callCount atomic.Int32

	harness := newStreamTestHarness(t, nil, func(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
		callCount.Add(1)
		if err := req.Session.NotifyInfo(ctx, &op.InfoNotificationParams{
			Meta:    req.Params.Meta.Add(op.Meta{"type": "stream"}),
			Content: &op.TextContent{Text: "hello"},
		}); err != nil {
			return nil, err
		}
		once.Do(func() { close(firstEventSent) })
		<-releaseTurn
		return &op.OpNodeResult{Content: &op.TextContent{Text: "done"}}, nil
	})
	defer harness.close()

	req1, err := http.NewRequest(http.MethodPost, harness.baseURL+"/v1/chat/stream", streamBody(t, "turn-1"))
	if err != nil {
		t.Fatalf("http.NewRequest(): %v", err)
	}
	req1.Header.Set("Content-Type", "application/json")
	res1, err := harness.httpClient.Do(req1)
	if err != nil {
		t.Fatalf("Do(req1): %v", err)
	}
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res1.StatusCode)
	}

	reader1 := bufio.NewReader(res1.Body)
	firstEvent := readNextSSEEvent(t, reader1)
	if firstEvent.ID != "1" || !strings.Contains(firstEvent.Data, "\"type\":\"stream\"") {
		t.Fatalf("unexpected first event: %+v", firstEvent)
	}
	secondBufferedEvent := readNextSSEEvent(t, reader1)
	if secondBufferedEvent.ID != "2" || !strings.Contains(secondBufferedEvent.Data, "\"type\":\"stream\"") {
		t.Fatalf("unexpected duplicate stream event: %+v", secondBufferedEvent)
	}
	<-firstEventSent
	_ = res1.Body.Close()

	req2, err := http.NewRequest(http.MethodPost, harness.baseURL+"/v1/chat/stream", streamBody(t, "turn-1"))
	if err != nil {
		t.Fatalf("http.NewRequest(): %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Last-Event-ID", "2")
	res2, err := harness.httpClient.Do(req2)
	if err != nil {
		t.Fatalf("Do(req2): %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res2.StatusCode, readAllString(t, res2.Body))
	}

	close(releaseTurn)

	reader2 := bufio.NewReader(res2.Body)
	secondEvent := readNextSSEEvent(t, reader2)
	if secondEvent.ID != "3" || !strings.Contains(secondEvent.Data, "\"type\":\"end\"") {
		t.Fatalf("unexpected reattach event: %+v", secondEvent)
	}
	if got := callCount.Load(); got != 1 {
		t.Fatalf("expected one OpNode call, got %d", got)
	}
}

func TestStreamHandler_SubmitsThreadStateWithoutHTTPContinuationPrecheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	threadFilePath := filepath.Join(t.TempDir(), "thread", "thread-test.jsonl")
	writeThreadFileForStreamTest(
		t,
		threadFilePath,
		op.ThreadHeader{
			Type:      "thread",
			Version:   1,
			ID:        "thread-test",
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			AgentID:   "agent-id",
			CWD:       "/tmp",
			ChatPath:  "/tmp/chat.md",
			Title:     "chat",
		},
		op.ThreadCanonicalMessageEntry{
			ThreadEntryBase: op.ThreadEntryBase{
				Type:      op.ThreadEntryTypeCanonicalMessage,
				ID:        "msg-1",
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			},
			Message: ai.ConversationMessage{
				Role: ai.RoleCanonicalUser,
				Content: []ai.ContentBlock{{
					Type: ai.BlockText,
					Text: "hello",
				}},
			},
		},
	)

	service, cleanup := newControlTestService(
		t,
		nil,
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			switch req.Params.OpCode {
			case op.OpThreadMetaGet:
				raw, err := json.Marshal(op.ThreadMeta{
					ThreadID:       "thread-test",
					AgentID:        "agent-id",
					FileID:         "file-test",
					ChatPath:       "/tmp/chat.md",
					ThreadFilePath: threadFilePath,
				})
				if err != nil {
					t.Fatalf("marshal thread meta: %v", err)
				}
				return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
			case op.OpThreadSubmit:
				return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
			default:
				t.Fatalf("unexpected node opcode: %s", req.Params.OpCode)
				return nil, nil
			}
		},
	)
	defer cleanup()

	router := gin.New()
	router.POST("/v1/chat/stream", NewHandler(sse.NewManager(), service).Stream)
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()

	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/chat/stream", streamBody(t, "turn-1"))
	if err != nil {
		t.Fatalf("http.NewRequest(): %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := httpServer.Client().Do(req)
	if err != nil {
		t.Fatalf("Do(req): %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.StatusCode, readAllString(t, res.Body))
	}
}

type keepAliveWriter struct {
	header http.Header
	buf    bytes.Buffer
	mu     sync.Mutex
}

func mustJSONRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	return raw
}

func (w *keepAliveWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *keepAliveWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *keepAliveWriter) WriteHeader(_ int) {}

func (w *keepAliveWriter) Flush() {}

func (w *keepAliveWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func TestStreamHandler_EventLoopWritesKeepAlive(t *testing.T) {
	handler := &Handler{keepAliveInterval: time.Millisecond}
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/stream", nil)
	reqCtx, cancelReq := context.WithCancel(req.Context())
	defer cancelReq()
	req = req.WithContext(reqCtx)

	connCtx, cancelConn := context.WithCancel(context.Background())
	defer cancelConn()
	conn := &sse.Connection{
		ThreadID: "thread-test",
		SSEChan:  make(chan *sse.Event),
		Ctx:      connCtx,
		Cancel:   cancelConn,
	}

	writer := &keepAliveWriter{}
	done := make(chan struct{})
	go func() {
		handler.eventLoop(writer, req, conn)
		close(done)
	}()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(writer.String(), ": keep-alive") {
			cancelReq()
			<-done
			return
		}
		time.Sleep(time.Millisecond)
	}

	cancelReq()
	<-done
	t.Fatal("expected keep-alive frame before request shutdown")
}

func TestStreamHandler_EventLoopKeepAliveRefreshesConnectionActivity(t *testing.T) {
	manager := sse.NewManager()
	conn := manager.Register("thread-test", context.Background())
	defer manager.Unregister("thread-test", conn)
	stale := time.Now().Add(-time.Hour)
	conn.LastActive = stale

	handler := &Handler{
		sseManager:        manager,
		keepAliveInterval: time.Millisecond,
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/stream", nil)
	reqCtx, cancelReq := context.WithCancel(req.Context())
	defer cancelReq()
	req = req.WithContext(reqCtx)

	writer := &keepAliveWriter{}
	done := make(chan struct{})
	go func() {
		handler.eventLoop(writer, req, conn)
		close(done)
	}()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		connections := manager.GetAllConnections()
		if len(connections) == 1 && connections[0].LastActive.After(stale) {
			cancelReq()
			<-done
			return
		}
		time.Sleep(time.Millisecond)
	}

	cancelReq()
	<-done
	t.Fatal("expected keep-alive to refresh SSE connection activity")
}

func TestStreamHandler_RejectsConcurrentDifferentTurnWhileRunning(t *testing.T) {
	gin.SetMode(gin.TestMode)

	releaseTurn := make(chan struct{})
	firstEventSent := make(chan struct{})
	var once sync.Once
	var callCount atomic.Int32

	harness := newStreamTestHarness(t, nil, func(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
		callCount.Add(1)
		if err := req.Session.NotifyInfo(ctx, &op.InfoNotificationParams{
			Meta:    req.Params.Meta.Add(op.Meta{"type": "stream"}),
			Content: &op.TextContent{Text: "hello"},
		}); err != nil {
			return nil, err
		}
		once.Do(func() { close(firstEventSent) })
		<-releaseTurn
		return &op.OpNodeResult{Content: &op.TextContent{Text: "done"}}, nil
	})
	defer harness.close()

	req1, err := http.NewRequest(http.MethodPost, harness.baseURL+"/v1/chat/stream", streamBody(t, "turn-1"))
	if err != nil {
		t.Fatalf("http.NewRequest(): %v", err)
	}
	req1.Header.Set("Content-Type", "application/json")
	res1, err := harness.httpClient.Do(req1)
	if err != nil {
		t.Fatalf("Do(req1): %v", err)
	}
	defer res1.Body.Close()
	<-firstEventSent

	req2, err := http.NewRequest(http.MethodPost, harness.baseURL+"/v1/chat/stream", streamBody(t, "turn-2"))
	if err != nil {
		t.Fatalf("http.NewRequest(): %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	res2, err := harness.httpClient.Do(req2)
	if err != nil {
		t.Fatalf("Do(req2): %v", err)
	}
	defer res2.Body.Close()

	if res2.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", res2.StatusCode, readAllString(t, res2.Body))
	}

	close(releaseTurn)

	if got := callCount.Load(); got != 1 {
		t.Fatalf("expected one OpNode call, got %d", got)
	}
}

func TestControlHandler_InterruptedCancelsRunningTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	turnCanceled := make(chan struct{})
	harness := newStreamTestHarness(t, func(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
		raw, err := json.Marshal(op.ThreadControlAck{
			OK:       true,
			ThreadID: "thread-test",
			OpCode:   req.Params.OpCode,
		})
		if err != nil {
			return nil, err
		}
		return &op.OpAgentResult{
			OpCode:  req.Params.OpCode,
			Content: &op.JsonContent{Raw: raw},
		}, nil
	}, func(ctx context.Context, _ *op.OpNodeRequest) (*op.OpNodeResult, error) {
		<-ctx.Done()
		close(turnCanceled)
		return nil, ctx.Err()
	})
	defer harness.close()

	req1, err := http.NewRequest(http.MethodPost, harness.baseURL+"/v1/chat/stream", streamBody(t, "turn-1"))
	if err != nil {
		t.Fatalf("http.NewRequest(): %v", err)
	}
	req1.Header.Set("Content-Type", "application/json")
	res1, err := harness.httpClient.Do(req1)
	if err != nil {
		t.Fatalf("Do(req1): %v", err)
	}
	defer res1.Body.Close()

	req2, err := http.NewRequest(http.MethodPost, harness.baseURL+"/v1/chat/control", controlBody(t))
	if err != nil {
		t.Fatalf("http.NewRequest(): %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	res2, err := harness.httpClient.Do(req2)
	if err != nil {
		t.Fatalf("Do(req2): %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res2.StatusCode, readAllString(t, res2.Body))
	}

	<-turnCanceled
}

func readAllString(t *testing.T, reader io.Reader) string {
	t.Helper()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll(): %v", err)
	}
	return string(body)
}
