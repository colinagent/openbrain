package core

import (
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestBuildTurnResultPayloadUsesOnlyLastAssistantText(t *testing.T) {
	payload := buildTurnResultPayload(
		"turn-1",
		op.ThreadMeta{
			ThreadID: "thread-test",
			AgentID:  "agent-test",
			ChatPath: "/tmp/chat.md",
			Title:    "Chat",
		},
		op.NewUserMessage("hello"),
		[]op.Message{
			op.NewAssistantMessage("tool preface"),
			op.NewToolResultMessage("shell", "call-1", "tool output"),
			op.NewAssistantMessage("final answer"),
		},
		nil,
		false,
	)

	if payload.AssistantText != "final answer" {
		t.Fatalf("AssistantText = %q, want final answer", payload.AssistantText)
	}
	if len(payload.ToolResults) != 1 || payload.ToolResults[0].ResultText != "tool output" {
		t.Fatalf("ToolResults = %+v, want one preserved tool result", payload.ToolResults)
	}
}

func TestBuildTurnResultPayloadUsesTerminalNoticeForThinkingOnlyAbort(t *testing.T) {
	payload := buildTurnResultPayload(
		"turn-1",
		op.ThreadMeta{
			ThreadID: "thread-test",
			AgentID:  "agent-test",
			ChatPath: "/tmp/chat.md",
			Title:    "Chat",
		},
		op.NewUserMessage("hello"),
		[]op.Message{{
			Role:             op.RoleAssistant,
			ReasoningContent: "thinking before cancel",
			StopReason:       op.StopReasonAborted,
		}},
		nil,
		false,
	)

	if payload.AssistantText != "Turn interrupted before completion." {
		t.Fatalf("AssistantText = %q, want interruption notice", payload.AssistantText)
	}
	if payload.ReasoningText != "thinking before cancel" {
		t.Fatalf("ReasoningText = %q, want thinking preserved", payload.ReasoningText)
	}
}

func TestAgentLoopTurnResultPayloadUsesCurrentTurnAgent(t *testing.T) {
	loop := &AgentLoop{
		Agent:    &Agent{AgentID: "agent-next"},
		Meta:     op.Meta{"threadID": "thread-test", "agentID": "agent-next", "cwd": "/tmp/next", "fileID": "file-test"},
		ThreadID: "thread-test",
		ChatPath: "/tmp/chat.md",
		Workdir:  "/tmp/next",
		threadMeta: op.ThreadMeta{
			ThreadID: "thread-test",
			AgentID:  "agent-original",
			CWD:      "/tmp/original",
			ChatPath: "/tmp/chat.md",
		},
	}

	meta := loop.loopThreadMeta()
	if meta.AgentID != "agent-next" {
		t.Fatalf("loopThreadMeta AgentID = %q, want agent-next", meta.AgentID)
	}
	if meta.CWD != "/tmp/next" {
		t.Fatalf("loopThreadMeta CWD = %q, want /tmp/next", meta.CWD)
	}

	payload := buildTurnResultPayload(
		"turn-1",
		meta,
		op.NewUserMessage("hello"),
		[]op.Message{op.NewAssistantMessage("done")},
		nil,
		false,
	)
	if payload.AgentID != "agent-next" {
		t.Fatalf("payload.AgentID = %q, want agent-next", payload.AgentID)
	}
}
