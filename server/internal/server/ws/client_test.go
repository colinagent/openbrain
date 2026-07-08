package ws

import (
	"encoding/json"
	"testing"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

func TestShouldHandleMessengerReplyAsync(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  protocol.MethodMessengerReply,
		"params":  map[string]any{"channelID": "channel-1", "text": "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !shouldHandleRequestAsync(raw) {
		t.Fatalf("%s should be handled asynchronously", protocol.MethodMessengerReply)
	}
}
