package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func newMessengerTestHandler(
	t *testing.T,
	agentHandler func(context.Context, *op.OpAgentRequest) (*op.OpAgentResult, error),
) (*Handler, func()) {
	t.Helper()

	server := op.NewServer(&op.Implementation{Name: "host", Version: "v0.0.1"}, nil)
	t1, t2 := op.NewInMemoryTransports()
	session, err := server.Connect(context.Background(), t1, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpAgentHandler: agentHandler,
	})
	clientSession, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}

	wsServer := NewServer("127.0.0.1:0", false)
	wsServer.SetHostSession(session)
	return wsServer.handler, func() {
		_ = clientSession.Close()
	}
}

func TestMessengerReplyPassesModelMetaOutsideMessageContent(t *testing.T) {
	var sawRequest bool
	var requestErr error
	failRequest := func(format string, args ...any) (*op.OpAgentResult, error) {
		requestErr = fmt.Errorf(format, args...)
		return nil, requestErr
	}
	handler, cleanup := newMessengerTestHandler(
		t,
		func(_ context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
			sawRequest = true
			if req.Params.OpCode != op.OpMessageReply {
				return failRequest("opcode = %s, want %s", req.Params.OpCode, op.OpMessageReply)
			}
			if got := metaString(req.Params.Meta, "modelKey"); got != "test:model" {
				return failRequest("modelKey meta = %q, want test:model", got)
			}
			if got := metaString(req.Params.Meta, "thinkingLevel"); got != "high" {
				return failRequest("thinkingLevel meta = %q, want high", got)
			}
			if got := metaString(req.Params.Meta, "serviceTier"); got != "priority" {
				return failRequest("serviceTier meta = %q, want priority", got)
			}
			if got := numericMeta(req.Params.Meta["contextWindow"]); got != 8192 {
				return failRequest("contextWindow meta = %#v, want 8192", req.Params.Meta["contextWindow"])
			}
			content := req.Params.Content.(*op.JsonContent).Raw
			if strings.Contains(string(content), "modelKey") || strings.Contains(string(content), "thinkingLevel") {
				return failRequest("reply content leaked execution meta: %s", string(content))
			}
			var params op.MessageReplyParams
			if err := json.Unmarshal(content, &params); err != nil {
				return failRequest("decode reply params: %v", err)
			}
			if params.ChannelID != "channel-test" || params.ReplyToMessageID != "msg-request" {
				return failRequest("reply params = %+v", params)
			}
			raw, err := json.Marshal(op.MessageReplyResult{
				Record: op.MessageRecord{
					ID:        "msg-reply",
					ChannelID: params.ChannelID,
					Body:      params.Text,
				},
			})
			if err != nil {
				return nil, err
			}
			return &op.OpAgentResult{OpCode: op.OpMessageReply, Content: &op.JsonContent{Raw: raw}}, nil
		},
	)
	defer cleanup()

	_, rpcErr := handler.handleMessengerReply(json.RawMessage(`{
		"channelID":"channel-test",
		"replyToMessageID":"msg-request",
		"text":"Keep independent",
		"modelKey":"test:model",
		"thinkingLevel":"high",
		"contextWindow":8192,
		"serviceTier":"priority"
	}`))
	if rpcErr != nil {
		t.Fatalf("handleMessengerReply(): %v", rpcErr)
	}
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	if !sawRequest {
		t.Fatal("OpAgent request was not called")
	}
}

func numericMeta(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return 0
	}
}
