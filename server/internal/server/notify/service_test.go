package notify

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/sse"
)

func waitSSEMessage(t *testing.T, ch <-chan *sse.Event) *op.GeneralContent {
	t.Helper()
	select {
	case event := <-ch:
		if event == nil || event.Message == nil {
			t.Fatal("received nil SSE message")
		}
		return event.Message
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting SSE message")
		return nil
	}
}

func textOf(t *testing.T, c op.Content) string {
	t.Helper()
	text, ok := c.(*op.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", c)
	}
	return text.Text
}

func TestNotifyErrorSendsErrorEvent(t *testing.T) {
	manager := sse.NewManager()
	svc := NewService(manager)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := manager.Register("t2", ctx)
	defer manager.Unregister("t2", conn)

	meta := op.Meta{"threadID": "t2"}
	svc.NotifyError(meta, &op.TextContent{Text: "boom"})

	msg := waitSSEMessage(t, conn.SSEChan)
	if got, _ := msg.Meta["type"].(string); got != "error" {
		t.Fatalf("expected type=error, got %q", got)
	}
	if got := textOf(t, msg.Content); got != "boom" {
		t.Fatalf("expected content boom, got %q", got)
	}
}

func mustNotifyJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	return raw
}
