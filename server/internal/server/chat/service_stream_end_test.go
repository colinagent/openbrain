package chat

import (
	"context"
	"testing"
	"time"

	"github.com/colinagent/openbrain/server/internal/server/notify"
	"github.com/colinagent/openbrain/server/internal/server/sse"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestHandleNodeResultSkipsEndWhenRuntimeEmittedStreamEnd(t *testing.T) {
	manager := sse.NewManager()
	service := NewService(notify.NewService(manager))
	conn := manager.Register("thread-test", context.Background())

	err := service.handleNodeResult(op.Meta{"threadID": "thread-test"}, &op.OpNodeResult{
		Meta:    op.Meta{"streamEndEmitted": true},
		Content: &op.TextContent{Text: "ok"},
	})
	if err != nil {
		t.Fatalf("handleNodeResult(): %v", err)
	}

	select {
	case event := <-conn.SSEChan:
		t.Fatalf("unexpected server end event after runtime end marker: %#v", event)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestHandleNodeResultSendsEndWithoutRuntimeEndMarker(t *testing.T) {
	manager := sse.NewManager()
	service := NewService(notify.NewService(manager))
	conn := manager.Register("thread-test", context.Background())

	err := service.handleNodeResult(op.Meta{"threadID": "thread-test"}, &op.OpNodeResult{
		Content: &op.TextContent{Text: "ok"},
	})
	if err != nil {
		t.Fatalf("handleNodeResult(): %v", err)
	}

	select {
	case event := <-conn.SSEChan:
		if event == nil || event.Message == nil {
			t.Fatalf("expected end event, got %#v", event)
		}
		if typ, _ := event.Message.Meta["type"].(string); typ != "end" {
			t.Fatalf("event type = %q, want end", typ)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for fallback server end event")
	}
}
