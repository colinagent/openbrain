package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/sse"
	"github.com/gin-gonic/gin"
)

func newControlTestService(
	t *testing.T,
	agentHandler func(context.Context, *op.OpAgentRequest) (*op.OpAgentResult, error),
	nodeHandler func(context.Context, *op.OpNodeRequest) (*op.OpNodeResult, error),
) (*Service, func()) {
	t.Helper()

	server := op.NewServer(&op.Implementation{Name: "host", Version: "v0.0.1"}, nil)
	t1, t2 := op.NewInMemoryTransports()
	session, err := server.Connect(context.Background(), t1, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpAgentHandler: agentHandler,
		OpNodeHandler:  nodeHandler,
	})
	clientSession, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}

	service := NewService(nil)
	service.SetHostSession(session)
	return service, func() {
		_ = clientSession.Close()
	}
}

func TestControlHandler_Steer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			if req.Params.OpCode != op.OpThreadSteer {
				t.Fatalf("received opcode %s, want %s", req.Params.OpCode, op.OpThreadSteer)
			}
			raw, err := json.Marshal(op.ThreadControlAck{
				OK:       true,
				ThreadID: "thread-test",
				OpCode:   op.OpThreadSteer,
			})
			if err != nil {
				t.Fatalf("marshal ack: %v", err)
			}
			return &op.OpAgentResult{
				OpCode:  op.OpThreadSteer,
				Content: &op.JsonContent{Raw: raw},
			}, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req.Params.OpCode != op.OpThreadMetaGet {
				t.Fatalf("received node opcode %s, want %s", req.Params.OpCode, op.OpThreadMetaGet)
			}
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-test",
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
			"opcode":   string(op.OpThreadSteer),
			"threadID": "thread-test",
			"fileID":   "file-test",
			"chatPath": "/tmp/chat.md",
			"agentID":  "agent-id",
			"modelKey": "test:model",
		},
		Content: &op.TextContent{
			Text: "interrupt",
		},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/chat/control", NewHandler(sse.NewManager(), service).Control)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var ack op.ThreadControlAck
	if err := json.NewDecoder(rr.Body).Decode(&ack); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !ack.OK || ack.ThreadID != "thread-test" || ack.OpCode != op.OpThreadSteer {
		t.Fatalf("unexpected ack: %+v", ack)
	}
}

func TestControlHandler_SteerPreservesRequestAgentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			if req.Params.OpCode != op.OpThreadSteer {
				t.Fatalf("received opcode %s, want %s", req.Params.OpCode, op.OpThreadSteer)
			}
			if got := req.Params.Meta["agentID"]; got != "agent-next" {
				t.Fatalf("forwarded agentID = %v, want agent-next", got)
			}
			if got := req.Params.Meta["agentName"]; got != "Next Agent" {
				t.Fatalf("forwarded agentName = %v, want Next Agent", got)
			}
			raw, err := json.Marshal(op.ThreadControlAck{
				OK:       true,
				ThreadID: "thread-test",
				OpCode:   op.OpThreadSteer,
			})
			if err != nil {
				t.Fatalf("marshal ack: %v", err)
			}
			return &op.OpAgentResult{
				OpCode:  op.OpThreadSteer,
				Content: &op.JsonContent{Raw: raw},
			}, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req.Params.OpCode != op.OpThreadMetaGet {
				t.Fatalf("received node opcode %s, want %s", req.Params.OpCode, op.OpThreadMetaGet)
			}
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-test",
				AgentID:  "agent-original",
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
			"opcode":    string(op.OpThreadSteer),
			"threadID":  "thread-test",
			"fileID":    "file-test",
			"chatPath":  "/tmp/chat.md",
			"agentID":   "agent-next",
			"agentName": "Next Agent",
			"modelKey":  "test:model",
		},
		Content: &op.TextContent{Text: "interrupt"},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/chat/control", NewHandler(sse.NewManager(), service).Control)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestControlHandler_InterruptedReturnsQueuedMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			raw, err := json.Marshal(op.ThreadControlAck{
				OK:       true,
				ThreadID: "thread-test",
				OpCode:   req.Params.OpCode,
				QueuedMessages: op.ThreadQueueSnapshot{
					FollowUp: []op.ThreadQueueItem{{
						ID:      "queue-1",
						Message: op.NewUserMessage("queued"),
					}},
				},
			})
			if err != nil {
				t.Fatalf("marshal ack: %v", err)
			}
			return &op.OpAgentResult{
				OpCode:  req.Params.OpCode,
				Content: &op.JsonContent{Raw: raw},
			}, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-test",
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
			"opcode":   string(op.OpThreadInterrupted),
			"threadID": "thread-test",
			"fileID":   "file-test",
			"chatPath": "/tmp/chat.md",
			"agentID":  "agent-id",
			"modelKey": "test:model",
		},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/chat/control", NewHandler(sse.NewManager(), service).Control)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var ack op.ThreadControlAck
	if err := json.NewDecoder(rr.Body).Decode(&ack); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(ack.QueuedMessages.FollowUp) != 1 || ack.QueuedMessages.FollowUp[0].Message.Content != "queued" {
		t.Fatalf("unexpected interrupted ack: %+v", ack)
	}
}

func TestControlHandler_Promote(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			if req.Params.OpCode != op.OpThreadFollowUpPromote {
				t.Fatalf("received opcode %s, want %s", req.Params.OpCode, op.OpThreadFollowUpPromote)
			}
			raw, err := json.Marshal(op.ThreadControlAck{
				OK:       true,
				ThreadID: "thread-test",
				OpCode:   op.OpThreadFollowUpPromote,
			})
			if err != nil {
				t.Fatalf("marshal ack: %v", err)
			}
			return &op.OpAgentResult{
				OpCode:  op.OpThreadFollowUpPromote,
				Content: &op.JsonContent{Raw: raw},
			}, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-test",
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
			"opcode":   string(op.OpThreadFollowUpPromote),
			"threadID": "thread-test",
			"fileID":   "file-test",
			"chatPath": "/tmp/chat.md",
			"agentID":  "agent-id",
			"itemID":   "queue-1",
		},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/chat/control", NewHandler(sse.NewManager(), service).Control)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestControlHandler_SteerKeepsMarkdownImagePathTextBeforeRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chatDir := t.TempDir()

	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			if req.Params.OpCode != op.OpThreadSteer {
				t.Fatalf("received opcode %s, want %s", req.Params.OpCode, op.OpThreadSteer)
			}
			jsonContent, ok := req.Params.Content.(*op.JsonContent)
			if !ok {
				t.Fatalf("content type = %T, want *op.JsonContent", req.Params.Content)
			}
			var msg op.Message
			if err := json.Unmarshal(jsonContent.Raw, &msg); err != nil {
				t.Fatalf("decode normalized content: %v", err)
			}
			if msg.Content != "![image-1]("+filepath.Join(chatDir, ".agent", "assets", "images", "image-1.png")+")\n\nlook" {
				t.Fatalf("content = %q", msg.Content)
			}
			raw, err := json.Marshal(op.ThreadControlAck{
				OK:       true,
				ThreadID: "thread-test",
				OpCode:   op.OpThreadSteer,
			})
			if err != nil {
				t.Fatalf("marshal ack: %v", err)
			}
			return &op.OpAgentResult{
				OpCode:  op.OpThreadSteer,
				Content: &op.JsonContent{Raw: raw},
			}, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-test",
				AgentID:  "agent-id",
				FileID:   "file-test",
				ChatPath: filepath.Join(chatDir, "chat.md"),
				Path:     filepath.Join(chatDir, "chat.md"),
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
			"opcode":   string(op.OpThreadSteer),
			"threadID": "thread-test",
			"fileID":   "file-test",
			"chatPath": filepath.Join(chatDir, "chat.md"),
			"agentID":  "agent-id",
			"modelKey": "test:model",
		},
		Content: &op.TextContent{Text: "![image-1](" + filepath.Join(chatDir, ".agent", "assets", "images", "image-1.png") + ")\n\nlook"},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/chat/control", NewHandler(sse.NewManager(), service).Control)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestControlHandler_QueueGetAndRemove(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			switch req.Params.OpCode {
			case op.OpThreadQueueGet:
				raw, err := json.Marshal(op.ThreadControlAck{
					OK:       true,
					ThreadID: "thread-test",
					OpCode:   op.OpThreadQueueGet,
					QueuedMessages: op.ThreadQueueSnapshot{
						FollowUp: []op.ThreadQueueItem{{
							ID:      "queue-1",
							Message: op.NewUserMessage("queued"),
						}},
					},
				})
				if err != nil {
					t.Fatalf("marshal queue get ack: %v", err)
				}
				return &op.OpAgentResult{
					OpCode:  op.OpThreadQueueGet,
					Content: &op.JsonContent{Raw: raw},
				}, nil
			case op.OpThreadQueueRemove:
				if got := req.Params.Meta["itemID"]; got != "queue-1" {
					t.Fatalf("received itemID %v, want queue-1", got)
				}
				raw, err := json.Marshal(op.ThreadControlAck{
					OK:       true,
					ThreadID: "thread-test",
					OpCode:   op.OpThreadQueueRemove,
					RemovedItem: &op.ThreadQueueItem{
						ID:      "queue-1",
						Message: op.NewUserMessage("queued"),
					},
				})
				if err != nil {
					t.Fatalf("marshal queue remove ack: %v", err)
				}
				return &op.OpAgentResult{
					OpCode:  op.OpThreadQueueRemove,
					Content: &op.JsonContent{Raw: raw},
				}, nil
			default:
				t.Fatalf("received opcode %s, want queue get/remove", req.Params.OpCode)
				return nil, nil
			}
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-test",
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

	router := gin.New()
	router.POST("/v1/chat/control", NewHandler(sse.NewManager(), service).Control)

	getBody := op.GeneralContent{
		Meta: op.Meta{
			"opcode":   string(op.OpThreadQueueGet),
			"threadID": "thread-test",
			"fileID":   "file-test",
			"chatPath": "/tmp/chat.md",
			"agentID":  "agent-id",
			"modelKey": "test:model",
		},
	}
	getRaw, _ := json.Marshal(getBody)
	getReq := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader(getRaw))
	getReq.Header.Set("Content-Type", "application/json")
	getRes := httptest.NewRecorder()
	router.ServeHTTP(getRes, getReq)
	if getRes.Code != 200 {
		t.Fatalf("queue get expected 200, got %d body=%s", getRes.Code, getRes.Body.String())
	}

	removeBody := op.GeneralContent{
		Meta: op.Meta{
			"opcode":    string(op.OpThreadQueueRemove),
			"threadID":  "thread-test",
			"fileID":    "file-test",
			"chatPath":  "/tmp/chat.md",
			"agentID":   "agent-id",
			"queueKind": string(op.ThreadQueueKindFollowUp),
			"itemID":    "queue-1",
		},
	}
	removeRaw, _ := json.Marshal(removeBody)
	removeReq := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader(removeRaw))
	removeReq.Header.Set("Content-Type", "application/json")
	removeRes := httptest.NewRecorder()
	router.ServeHTTP(removeRes, removeReq)
	if removeRes.Code != 200 {
		t.Fatalf("queue remove expected 200, got %d body=%s", removeRes.Code, removeRes.Body.String())
	}
}

func TestControlHandler_QueueGetReturnsEmptySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			if req.Params.OpCode != op.OpThreadQueueGet {
				t.Fatalf("received opcode %s, want %s", req.Params.OpCode, op.OpThreadQueueGet)
			}
			raw, err := json.Marshal(op.ThreadControlAck{
				OK:             true,
				ThreadID:       "thread-missing-runtime",
				OpCode:         op.OpThreadQueueGet,
				QueuedMessages: op.ThreadQueueSnapshot{},
			})
			if err != nil {
				t.Fatalf("marshal queue get ack: %v", err)
			}
			return &op.OpAgentResult{
				OpCode:  op.OpThreadQueueGet,
				Content: &op.JsonContent{Raw: raw},
			}, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-missing-runtime",
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

	router := gin.New()
	router.POST("/v1/chat/control", NewHandler(sse.NewManager(), service).Control)

	body := op.GeneralContent{
		Meta: op.Meta{
			"opcode":   string(op.OpThreadQueueGet),
			"threadID": "thread-missing-runtime",
			"fileID":   "file-test",
			"chatPath": "/tmp/chat.md",
			"agentID":  "agent-id",
			"modelKey": "test:model",
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != 200 {
		t.Fatalf("queue get expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var ack op.ThreadControlAck
	if err := json.NewDecoder(res.Body).Decode(&ack); err != nil {
		t.Fatalf("decode queue get response: %v", err)
	}
	if ack.ThreadID != "thread-missing-runtime" {
		t.Fatalf("queue get threadID = %q, want thread-missing-runtime", ack.ThreadID)
	}
	if len(ack.QueuedMessages.Steering) != 0 || len(ack.QueuedMessages.FollowUp) != 0 {
		t.Fatalf("queue get ack = %+v, want empty queued messages", ack.QueuedMessages)
	}
}

func TestControlHandler_UnsupportedOpcodeDoesNotCallRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			t.Fatalf("unexpected runtime call for unsupported opcode %s", req.Params.OpCode)
			return nil, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-test",
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
			"opcode":   "thread/elicit_reply",
			"threadID": "thread-test",
			"fileID":   "file-test",
			"chatPath": "/tmp/chat.md",
			"agentID":  "agent-id",
		},
		Content: &op.JsonContent{Raw: []byte(`{"requestID":"req-1","answers":[["postgresql"],[]]}`)},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/chat/control", NewHandler(sse.NewManager(), service).Control)
	router.ServeHTTP(rr, req)

	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "unsupported control opcode") {
		t.Fatalf("unexpected response body: %s", rr.Body.String())
	}
}

func TestControlHandler_RequiresSessionIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader([]byte(`{"meta":{"opcode":"thread/steer","threadID":"thread-test","chatPath":"/tmp/chat.md","modelKey":"test:model"}}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/chat/control", NewHandler(sse.NewManager(), NewService(nil)).Control)
	router.ServeHTTP(rr, req)

	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestControlHandler_AllowsDifferentAgentIDForSameThread(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			raw, err := json.Marshal(op.ThreadControlAck{
				OK:       true,
				ThreadID: "thread-test",
				OpCode:   req.Params.OpCode,
			})
			if err != nil {
				t.Fatalf("marshal ack: %v", err)
			}
			return &op.OpAgentResult{
				OpCode:  req.Params.OpCode,
				Content: &op.JsonContent{Raw: raw},
			}, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			raw, err := json.Marshal(op.ThreadMeta{
				ThreadID: "thread-test",
				AgentID:  "different-agent",
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
			"opcode":   string(op.OpThreadSteer),
			"threadID": "thread-test",
			"fileID":   "file-test",
			"chatPath": "/tmp/chat.md",
			"agentID":  "agent-id",
			"modelKey": "test:model",
		},
		Content: &op.TextContent{Text: "interrupt"},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/chat/control", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/chat/control", NewHandler(sse.NewManager(), service).Control)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}
