package core

import (
	"os"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestRequireThreadSubmitCapabilityAcceptsChatAgent(t *testing.T) {
	err := requireThreadSubmitCapability(&op.OpNode{
		ID:      "agent-chat",
		Kind:    string(op.NodeKindAgent),
		OpCodes: []op.OpCode{op.OpPromptGet, op.OpThreadSubmit},
	})
	if err != nil {
		t.Fatalf("requireThreadSubmitCapability(): %v", err)
	}
}

func TestRequireThreadSubmitCapabilityRejectsNonChatAgent(t *testing.T) {
	err := requireThreadSubmitCapability(&op.OpNode{
		ID:      "agent-memory",
		Kind:    string(op.NodeKindAgent),
		OpCodes: []op.OpCode{op.OpPromptGet},
	})
	if err == nil {
		t.Fatal("requireThreadSubmitCapability() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "missing thread/submit opcode") {
		t.Fatalf("error = %q, want missing thread/submit opcode", err.Error())
	}
}

func TestThreadSubmitEndpointQueuedPromptContinuesDeliveredUserMessage(t *testing.T) {
	sourceBytes, err := os.ReadFile("thread_submit.go")
	if err != nil {
		t.Fatalf("read thread_submit.go: %v", err)
	}
	source := string(sourceBytes)
	if !strings.Contains(source, "return executeAgentContinue(ctx, node, callMeta)") {
		t.Fatal("endpoint queued prompt should continue after delivering the queued user message")
	}
	if !strings.Contains(source, "peekNextQueuedMessageForSubmit(snapshot.QueuedMessages)") {
		t.Fatal("needs_continuation submit should prefer queued follow-up before bare continuation")
	}
	if strings.Contains(source, "return executeAgentCall(ctx, node, callMeta, &op.TextContent{Text: pending.Message.Content}") {
		t.Fatal("endpoint queued prompt must not resubmit the delivered user message as a new prompt")
	}
}
