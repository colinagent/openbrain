package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func resetMessageTestState(baseDir string) {
	resetThreadTestState(baseDir)
	defaultMessageStore = &messageStore{}
	drainNotifyChan()
}

func testMessageLoop(t *testing.T, agentID, threadID string) (*AgentLoop, Loop) {
	t.Helper()
	if _, err := getThreadMeta(threadID, agentID); err != nil {
		baseDir, baseErr := threadBaseDir()
		if baseErr != nil {
			t.Fatalf("threadBaseDir(): %v", baseErr)
		}
		cwd := filepath.Join(baseDir, "workspace")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("mkdir workspace: %v", err)
		}
		if _, createErr := createThreadWithID(op.ThreadCreateParams{
			AgentID: agentID,
			CWD:     cwd,
			Title:   "message test",
		}, threadID); createErr != nil {
			t.Fatalf("createThreadWithID(): %v", createErr)
		}
	}
	agent := &Agent{
		AgentID:   agentID,
		ToolSpecs: map[string]*op.ToolSpec{},
	}
	addMessageToolSpecs(agent.ToolSpecs, &op.AgentMeta{})
	l := &AgentLoop{
		Ctx:      context.Background(),
		Agent:    agent,
		Meta:     op.Meta{"threadID": threadID, "agentID": agentID},
		ThreadID: threadID,
	}
	loop := Loop{
		Ctx:      context.Background(),
		Meta:     l.Meta,
		ThreadID: threadID,
	}
	return l, loop
}

func TestMessagePublishHostToolReturnsWithoutWaiting(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	agentID := "agent-demo"
	threadID := "thread-demo"
	l, loop := testMessageLoop(t, agentID, threadID)

	result, err := l.executeMessagePublish(loop, map[string]any{
		"kind":  "request",
		"title": "Need approval",
		"body":  "Continue?",
		"questions": []any{
			map[string]any{
				"id":       "decision",
				"question": "Continue?",
				"options": []any{
					map[string]any{"id": "continue", "label": "Continue"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("executeMessagePublish(): %v", err)
	}
	if result.MessageID == "" || result.ChannelID == "" || result.ThreadID != threadID {
		t.Fatalf("publish result = %+v", result)
	}
	if !result.Delivered {
		t.Fatalf("Delivered = false, want true")
	}

	read, err := defaultMessageStore.read(op.MessageReadParams{ChannelID: result.ChannelID})
	if err != nil {
		t.Fatalf("read message store: %v", err)
	}
	if len(read.Messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(read.Messages))
	}
	msg := read.Messages[0]
	if msg.Sender != op.MessageSenderAgent || msg.Kind != op.MessageKindRequest || msg.Body != "Continue?" {
		t.Fatalf("message = %+v", msg)
	}
}

func TestMessagePublishQuestionsAndReplyAnswers(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	agentID := "agent-demo"
	threadID := "thread-demo"
	l, loop := testMessageLoop(t, agentID, threadID)

	result, err := l.executeMessagePublish(loop, map[string]any{
		"kind":  "request",
		"title": "Need decision",
		"body":  "Choose a path.",
		"questions": []any{
			map[string]any{
				"id":       "nested_git_resolution",
				"question": "What should happen to cblog?",
				"options": []any{
					map[string]any{"id": "keep-independent", "label": "Keep independent - remove the gitlink and ignore cblog"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("executeMessagePublish(): %v", err)
	}
	read, err := defaultMessageStore.read(op.MessageReadParams{ChannelID: result.ChannelID})
	if err != nil {
		t.Fatalf("read message store: %v", err)
	}
	if len(read.Messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(read.Messages))
	}
	published := read.Messages[0]
	if len(published.Questions) != 1 || published.Questions[0].ID != "nested_git_resolution" {
		t.Fatalf("published questions = %+v", published.Questions)
	}

	raw, _ := json.Marshal(op.MessageReplyParams{
		ChannelID:        published.ChannelID,
		ReplyToMessageID: published.ID,
		Answers: []op.MessageAnswer{
			{
				QuestionID: "nested_git_resolution",
				Other:      true,
				Label:      "Other",
				Text:       "Convert it after I finish reviewing cblog.",
			},
		},
	})
	res, err := OpMessageReplyHandler(&op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode:  op.OpMessageReply,
			Meta:    op.Meta{},
			Content: &op.JsonContent{Raw: raw},
		},
	})
	if err != nil {
		t.Fatalf("OpMessageReplyHandler(): %v", err)
	}
	var reply op.MessageReplyResult
	if err := json.Unmarshal(res.Content.(*op.JsonContent).Raw, &reply); err != nil {
		t.Fatalf("decode reply result: %v", err)
	}
	if len(reply.Record.Answers) != 1 || !reply.Record.Answers[0].Other {
		t.Fatalf("reply answers = %+v", reply.Record.Answers)
	}
	if reply.Resolved == nil || reply.Resolved.ID != published.ID || reply.Resolved.Status != op.MessageStatusResolved {
		t.Fatalf("resolved request = %+v, want original request resolved", reply.Resolved)
	}
	if !strings.Contains(reply.Record.Body, "Convert it after I finish reviewing cblog.") {
		t.Fatalf("reply body = %q", reply.Record.Body)
	}
	if reply.Queue == nil || len(reply.Queue.QueuedMessages.FollowUp) != 1 {
		t.Fatalf("queue ack = %+v, want one follow-up", reply.Queue)
	}
	contextAnswers, ok := reply.Queue.QueuedMessages.FollowUp[0].SelectedSkillContext["answers"].([]any)
	if !ok || len(contextAnswers) != 1 {
		t.Fatalf("selected skill context answers = %#v", reply.Queue.QueuedMessages.FollowUp[0].SelectedSkillContext["answers"])
	}
	answer, ok := contextAnswers[0].(map[string]any)
	if !ok || answer["questionID"] != "nested_git_resolution" || answer["other"] != true {
		t.Fatalf("selected skill context answer = %#v", contextAnswers[0])
	}
	if got := reply.Queue.QueuedMessages.FollowUp[0].SelectedSkillContext["requestTitle"]; got != "Need decision" {
		t.Fatalf("selected skill context requestTitle = %#v, want Need decision", got)
	}
}

func TestMessageReplyKnownOptionIncludesRequestTitleContext(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	l, loop := testMessageLoop(t, "agent-demo", "thread-demo")

	result, err := l.executeMessagePublish(loop, map[string]any{
		"kind":  "request",
		"title": "Sync Blocked: note / cblog nested git",
		"body":  "Choose a path.",
		"questions": []any{
			map[string]any{
				"id":       "nested_git_resolution",
				"question": "What should happen to cblog?",
				"options": []any{
					map[string]any{"id": "keep-independent", "label": "Keep independent"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("executeMessagePublish(): %v", err)
	}
	read, err := defaultMessageStore.read(op.MessageReadParams{ChannelID: result.ChannelID})
	if err != nil {
		t.Fatalf("read message store: %v", err)
	}
	published := read.Messages[0]
	raw, _ := json.Marshal(op.MessageReplyParams{
		ChannelID:        published.ChannelID,
		ReplyToMessageID: published.ID,
		Answers: []op.MessageAnswer{
			{QuestionID: "nested_git_resolution", OptionID: "keep-independent", Label: "Keep independent"},
		},
	})
	res, err := OpMessageReplyHandler(&op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode:  op.OpMessageReply,
			Meta:    op.Meta{},
			Content: &op.JsonContent{Raw: raw},
		},
	})
	if err != nil {
		t.Fatalf("OpMessageReplyHandler(): %v", err)
	}
	var reply op.MessageReplyResult
	if err := json.Unmarshal(res.Content.(*op.JsonContent).Raw, &reply); err != nil {
		t.Fatalf("decode reply result: %v", err)
	}
	if reply.Resolved == nil || reply.Resolved.ID != published.ID || reply.Resolved.Title != "Sync Blocked: note / cblog nested git" {
		t.Fatalf("resolved request = %+v", reply.Resolved)
	}
	if reply.Queue == nil || len(reply.Queue.QueuedMessages.FollowUp) != 1 {
		t.Fatalf("queue ack = %+v, want one follow-up", reply.Queue)
	}
	item := reply.Queue.QueuedMessages.FollowUp[0]
	if got := item.SelectedSkillContext["requestTitle"]; got != "Sync Blocked: note / cblog nested git" {
		t.Fatalf("requestTitle = %#v", got)
	}
	contextAnswers, ok := item.SelectedSkillContext["answers"].([]any)
	if !ok || len(contextAnswers) != 1 {
		t.Fatalf("selected skill context answers = %#v", item.SelectedSkillContext["answers"])
	}
	answer, ok := contextAnswers[0].(map[string]any)
	if !ok || answer["questionID"] != "nested_git_resolution" || answer["optionID"] != "keep-independent" {
		t.Fatalf("selected skill context answer = %#v", contextAnswers[0])
	}
	if reply.Dispatch == nil {
		t.Fatal("dispatch = nil, want thread submit dispatch")
	}
	content := reply.Dispatch.Content
	if !strings.Contains(content, "User answered request: Sync Blocked: note / cblog nested git") ||
		!strings.Contains(content, "What should happen to cblog?") ||
		!strings.Contains(content, "keep-independent") ||
		!strings.Contains(content, "Keep independent") ||
		strings.Contains(content, "Message channel:") {
		t.Fatalf("dispatch content = %q", content)
	}
}

func TestMessagePublishRequestRequiresExactlyOneQuestion(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	l, loop := testMessageLoop(t, "agent-demo", "thread-demo")

	_, err := l.executeMessagePublish(loop, map[string]any{
		"kind": "request",
		"body": "Choose a path.",
	})
	if err == nil || !strings.Contains(err.Error(), "requires exactly one question") {
		t.Fatalf("executeMessagePublish() err = %v, want requires exactly one question", err)
	}

	_, err = l.executeMessagePublish(loop, map[string]any{
		"kind": "request",
		"body": "Choose a path.",
		"actions": []any{
			map[string]any{"id": "continue", "label": "Continue"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "does not support actions") {
		t.Fatalf("executeMessagePublish() err = %v, want does not support actions", err)
	}

	_, err = l.executeMessagePublish(loop, map[string]any{
		"kind": "request",
		"body": "Choose a path.",
		"questions": []any{
			map[string]any{
				"id":       "one",
				"question": "First?",
				"options":  []any{map[string]any{"id": "a", "label": "A"}},
			},
			map[string]any{
				"id":       "two",
				"question": "Second?",
				"options":  []any{map[string]any{"id": "b", "label": "B"}},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "exactly one question in v1") {
		t.Fatalf("executeMessagePublish() err = %v, want exactly one question in v1", err)
	}
}

func TestMessagePublishRejectsObsoleteQuestionFields(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	l, loop := testMessageLoop(t, "agent-demo", "thread-demo")

	_, err := l.executeMessagePublish(loop, map[string]any{
		"kind": "request",
		"body": "Choose a path.",
		"questions": []any{
			map[string]any{
				"id":         "decision",
				"header":     "obsolete",
				"allowOther": false,
				"question":   "Pick one.",
				"options": []any{
					map[string]any{
						"id":          "a",
						"label":       "A",
						"description": "obsolete",
						"tone":        "primary",
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("executeMessagePublish() succeeded with obsolete question fields")
	}
}

func TestMessageReplyResolvesRequestAndRejectsDuplicateAnswer(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	l, loop := testMessageLoop(t, "agent-demo", "thread-demo")

	result, err := l.executeMessagePublish(loop, map[string]any{
		"kind": "request",
		"body": "Choose once.",
		"questions": []any{
			map[string]any{
				"id":       "decision",
				"question": "Pick one.",
				"options": []any{
					map[string]any{"id": "a", "label": "A"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("executeMessagePublish(): %v", err)
	}
	read, err := defaultMessageStore.read(op.MessageReadParams{ChannelID: result.ChannelID})
	if err != nil {
		t.Fatalf("read message store: %v", err)
	}
	published := read.Messages[0]
	if _, err := defaultMessageStore.reply(op.MessageReplyParams{
		ChannelID:        result.ChannelID,
		ReplyToMessageID: published.ID,
		Answers: []op.MessageAnswer{
			{QuestionID: "decision", OptionID: "a", Label: "A"},
		},
	}); err != nil {
		t.Fatalf("first reply: %v", err)
	}
	afterReply, err := defaultMessageStore.read(op.MessageReadParams{ChannelID: result.ChannelID})
	if err != nil {
		t.Fatalf("read after reply: %v", err)
	}
	var resolvedOriginal *op.MessageRecord
	for i := range afterReply.Messages {
		if afterReply.Messages[i].ID == published.ID {
			resolvedOriginal = &afterReply.Messages[i]
			break
		}
	}
	if resolvedOriginal == nil || resolvedOriginal.Status != op.MessageStatusResolved {
		t.Fatalf("resolved original = %+v", resolvedOriginal)
	}
	if _, err := defaultMessageStore.reply(op.MessageReplyParams{
		ChannelID:        result.ChannelID,
		ReplyToMessageID: published.ID,
		Answers: []op.MessageAnswer{
			{QuestionID: "decision", OptionID: "a", Label: "A"},
		},
	}); err == nil || !strings.Contains(err.Error(), "request is not open") {
		t.Fatalf("duplicate reply err = %v, want request is not open", err)
	}
}

func TestMessageReplyRoutesByChannelAndAck(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	agentID := "agent-demo"
	threadID := "thread-demo"
	l, loop := testMessageLoop(t, agentID, threadID)

	first, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-a",
		"body":      "A?",
	})
	if err != nil {
		t.Fatalf("publish first: %v", err)
	}
	second, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-b",
		"body":      "B?",
	})
	if err != nil {
		t.Fatalf("publish second: %v", err)
	}
	if first.ChannelID == second.ChannelID {
		t.Fatalf("expected distinct channels")
	}

	reply, err := defaultMessageStore.reply(op.MessageReplyParams{
		ChannelID:        "channel-b",
		ReplyToMessageID: second.MessageID,
		Text:             "Yes B",
		ActionID:         "yes",
	})
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	if reply.ChannelID != "channel-b" || reply.ThreadID != threadID || reply.AgentID != agentID {
		t.Fatalf("reply routed incorrectly: %+v", reply)
	}

	pendingA, err := defaultMessageStore.read(op.MessageReadParams{ChannelID: "channel-a", PendingOnly: true})
	if err != nil {
		t.Fatalf("read pending A: %v", err)
	}
	if len(pendingA.Messages) != 0 {
		t.Fatalf("pending A len = %d, want 0", len(pendingA.Messages))
	}
	pendingB, err := defaultMessageStore.read(op.MessageReadParams{ChannelID: "channel-b", PendingOnly: true})
	if err != nil {
		t.Fatalf("read pending B: %v", err)
	}
	if len(pendingB.Messages) != 1 || pendingB.Messages[0].Body != "Yes B" {
		t.Fatalf("pending B = %+v", pendingB.Messages)
	}

	ack, err := defaultMessageStore.ack(op.MessageAckParams{ChannelID: "channel-b"})
	if err != nil {
		t.Fatalf("ack: %v", err)
	}
	if ack.Acked != 1 {
		t.Fatalf("Acked = %d, want 1", ack.Acked)
	}
	afterAck, err := defaultMessageStore.read(op.MessageReadParams{ChannelID: "channel-b", PendingOnly: true})
	if err != nil {
		t.Fatalf("read after ack: %v", err)
	}
	if len(afterAck.Messages) != 0 {
		t.Fatalf("pending after ack len = %d, want 0", len(afterAck.Messages))
	}
}

func TestMessageReplyRoutesByReplyToThreadInSharedChannel(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	const channelID = "openbrain-cloud-sync"

	olderLoop, olderThread := testMessageLoop(t, "agent-coder", "thread-older")
	olderRequest, err := olderLoop.executeMessagePublish(olderThread, map[string]any{
		"channelID": channelID,
		"kind":      "request",
		"title":     "Sync Blocked: note",
		"body":      "Choose a path.",
		"questions": []any{
			map[string]any{
				"id":       "nested_git_resolution",
				"question": "What should happen to cblog?",
				"options": []any{
					map[string]any{"id": "keep-independent", "label": "Keep independent"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish older request: %v", err)
	}

	newerLoop, newerThread := testMessageLoop(t, "agent-coder", "thread-newer")
	if _, err := newerLoop.executeMessagePublish(newerThread, map[string]any{
		"channelID": channelID,
		"kind":      "message",
		"body":      "New cron run took over the channel tail.",
	}); err != nil {
		t.Fatalf("publish newer message: %v", err)
	}

	raw, _ := json.Marshal(op.MessageReplyParams{
		ChannelID:        channelID,
		ReplyToMessageID: olderRequest.MessageID,
		Answers: []op.MessageAnswer{
			{QuestionID: "nested_git_resolution", OptionID: "keep-independent", Label: "Keep independent"},
		},
	})
	res, err := OpMessageReplyHandler(&op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode:  op.OpMessageReply,
			Meta:    op.Meta{},
			Content: &op.JsonContent{Raw: raw},
		},
	})
	if err != nil {
		t.Fatalf("OpMessageReplyHandler(): %v", err)
	}
	var reply op.MessageReplyResult
	if err := json.Unmarshal(res.Content.(*op.JsonContent).Raw, &reply); err != nil {
		t.Fatalf("decode reply result: %v", err)
	}
	if reply.Record.ThreadID != "thread-older" {
		t.Fatalf("reply threadID = %q, want thread-older", reply.Record.ThreadID)
	}
	if reply.Resolved == nil || reply.Resolved.ID != olderRequest.MessageID || reply.Resolved.Status != op.MessageStatusResolved {
		t.Fatalf("resolved request = %+v, want original older request resolved", reply.Resolved)
	}
	if reply.Queue == nil || len(reply.Queue.QueuedMessages.FollowUp) != 1 {
		t.Fatalf("queue ack = %+v, want one follow-up", reply.Queue)
	}
	if reply.Queue.ThreadID != "thread-older" {
		t.Fatalf("queue ack threadID = %q, want thread-older", reply.Queue.ThreadID)
	}
	if reply.Dispatch == nil || metaString(reply.Dispatch.Meta, "threadID") != "thread-older" {
		t.Fatalf("dispatch meta threadID = %q, want thread-older", metaString(reply.Dispatch.Meta, "threadID"))
	}
	if !strings.Contains(reply.Dispatch.Content, "Sync Blocked: note") {
		t.Fatalf("dispatch content = %q", reply.Dispatch.Content)
	}
}

func TestMessageReadPendingOnlyDefaultsAcrossChannels(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	agentID := "agent-demo"
	threadID := "thread-demo"
	l, loop := testMessageLoop(t, agentID, threadID)

	first, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-a",
		"body":      "A?",
	})
	if err != nil {
		t.Fatalf("publish first: %v", err)
	}
	second, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-b",
		"body":      "B?",
	})
	if err != nil {
		t.Fatalf("publish second: %v", err)
	}
	if _, err := defaultMessageStore.reply(op.MessageReplyParams{
		ChannelID:        second.ChannelID,
		ReplyToMessageID: second.MessageID,
		Text:             "Yes B",
	}); err != nil {
		t.Fatalf("reply: %v", err)
	}

	pending, err := l.executeMessageRead(loop, map[string]any{
		"pendingOnly": true,
	})
	if err != nil {
		t.Fatalf("executeMessageRead pendingOnly: %v", err)
	}
	if pending.ChannelID != "" {
		t.Fatalf("pending.ChannelID = %q, want empty cross-channel read", pending.ChannelID)
	}
	if len(pending.Messages) != 1 {
		t.Fatalf("pending messages len = %d, want 1: %+v", len(pending.Messages), pending.Messages)
	}
	if pending.Messages[0].ChannelID != first.ChannelID && pending.Messages[0].ChannelID != second.ChannelID {
		t.Fatalf("pending message channel = %q, want known channel", pending.Messages[0].ChannelID)
	}
	if pending.Messages[0].ChannelID != "channel-b" || pending.Messages[0].Body != "Yes B" {
		t.Fatalf("pending message = %+v, want channel-b reply", pending.Messages[0])
	}
}

func TestMessageAckByIDDoesNotDefaultToChannel(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	agentID := "agent-demo"
	threadID := "thread-demo"
	l, loop := testMessageLoop(t, agentID, threadID)

	published, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-custom",
		"body":      "Question",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	reply, err := defaultMessageStore.reply(op.MessageReplyParams{
		ChannelID:        published.ChannelID,
		ReplyToMessageID: published.MessageID,
		Text:             "Answer",
	})
	if err != nil {
		t.Fatalf("reply: %v", err)
	}

	ack, err := l.executeMessageAck(loop, map[string]any{
		"messageIDs": []any{reply.ID},
	})
	if err != nil {
		t.Fatalf("executeMessageAck by id: %v", err)
	}
	if ack.Acked != 1 {
		t.Fatalf("Acked = %d, want 1", ack.Acked)
	}
	pending, err := defaultMessageStore.read(op.MessageReadParams{ChannelID: published.ChannelID, PendingOnly: true})
	if err != nil {
		t.Fatalf("read pending after ack: %v", err)
	}
	if len(pending.Messages) != 0 {
		t.Fatalf("pending messages after ack = %+v, want empty", pending.Messages)
	}
}

func TestNewAgentMountsMessageToolsOnlyForThreadSubmitAgents(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)

	chatPath := filepath.Join(baseDir, "agents", "chat", ".agent", "AGENT.md")
	promptPath := filepath.Join(baseDir, "agents", "prompt", ".agent", "AGENT.md")
	writeAgentTestFile(t, chatPath, "---\nname: chat\n---\nChat prompt.\n")
	writeAgentTestFile(t, promptPath, "---\nname: prompt\n---\nPrompt only.\n")
	chatAgent := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(chatPath), op.EnvLocal, nil, op.Run{}, []op.OpCode{op.OpThreadSubmit}, &op.AgentMeta{Name: "chat"})
	promptAgent := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(promptPath), op.EnvLocal, nil, op.Run{}, []op.OpCode{op.OpPromptGet}, &op.AgentMeta{Name: "prompt"})

	chat, err := NewAgent(context.Background(), chatAgent, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(chat): %v", err)
	}
	if chat.ToolSpecs[messagePublishToolName] == nil {
		t.Fatalf("message_publish missing from chat-capable agent")
	}

	prompt, err := NewAgent(context.Background(), promptAgent, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(prompt): %v", err)
	}
	if prompt.ToolSpecs[messagePublishToolName] != nil {
		t.Fatalf("message_publish mounted on prompt-only agent")
	}
}

func TestNewAgentRespectsSystoolAllowlistForMessageTools(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)

	agentPath := filepath.Join(baseDir, "agents", "chat", ".agent", "AGENT.md")
	writeAgentTestFile(t, agentPath, "---\nname: chat\n---\nChat prompt.\n")
	node := op.BuildNode("user", "test-host", op.NodeKindAgent, op.PathToURI(agentPath), op.EnvLocal, nil, op.Run{}, []op.OpCode{op.OpThreadSubmit}, &op.AgentMeta{
		Name:        "chat",
		SysToolMode: op.SystoolModeAllowlist,
		SysTools:    []string{"read"},
	})

	agent, err := NewAgent(context.Background(), node, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(): %v", err)
	}
	if agent.ToolSpecs[messagePublishToolName] != nil {
		t.Fatalf("message_publish mounted without allowlist entry")
	}

	node.Meta = &op.AgentMeta{
		Name:        "chat",
		SysToolMode: op.SystoolModeAllowlist,
		SysTools:    []string{"message_publish", "message_read"},
	}
	agent, err = NewAgent(context.Background(), node, op.Meta{})
	if err != nil {
		t.Fatalf("NewAgent(with message allowlist): %v", err)
	}
	if agent.ToolSpecs[messagePublishToolName] == nil {
		t.Fatalf("message_publish missing with allowlist entry")
	}
	if agent.ToolSpecs[messageReadToolName] == nil {
		t.Fatalf("message_read missing with allowlist entry")
	}
	if agent.ToolSpecs[messageAckToolName] != nil {
		t.Fatalf("message_ack mounted without allowlist entry")
	}
}

func TestOpMessageReplyHandlerReturnsRecord(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	agentID := "agent-demo"
	threadID := "thread-demo"
	l, loop := testMessageLoop(t, agentID, threadID)
	published, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-op",
		"body":      "Question",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	raw, _ := json.Marshal(op.MessageReplyParams{
		ChannelID:        published.ChannelID,
		ReplyToMessageID: published.MessageID,
		Text:             "Answer",
	})
	res, err := OpMessageReplyHandler(&op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode:  op.OpMessageReply,
			Meta:    op.Meta{},
			Content: &op.JsonContent{Raw: raw},
		},
	})
	if err != nil {
		t.Fatalf("OpMessageReplyHandler(): %v", err)
	}
	var result op.MessageReplyResult
	if err := json.Unmarshal(res.Content.(*op.JsonContent).Raw, &result); err != nil {
		t.Fatalf("decode reply result: %v", err)
	}
	record := result.Record
	if record.Sender != op.MessageSenderUser || record.ChannelID != published.ChannelID || record.Body != "Answer" {
		t.Fatalf("reply record = %+v", record)
	}
	if result.Dispatch == nil {
		t.Fatal("dispatch = nil, want thread submit dispatch")
	}
}

func TestOpMessageReplyHandlerQueuesThreadDispatch(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")
	threadID := "thread-demo"
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "message-reply.md")
	if _, err := createThreadWithID(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "message-reply",
	}, threadID); err != nil {
		t.Fatalf("createThreadWithID(): %v", err)
	}
	l, loop := testMessageLoop(t, agentID, threadID)
	published, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-op",
		"kind":      "request",
		"title":     "Approve operation",
		"body":      "Question",
		"questions": []any{
			map[string]any{
				"id":       "approval",
				"question": "Approve this operation?",
				"options": []any{
					map[string]any{"id": "approve", "label": "Approve"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	raw, _ := json.Marshal(op.MessageReplyParams{
		ChannelID:        published.ChannelID,
		ReplyToMessageID: published.MessageID,
		Answers: []op.MessageAnswer{
			{QuestionID: "approval", OptionID: "approve", Label: "Approve"},
		},
	})
	res, err := OpMessageReplyHandler(&op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode:  op.OpMessageReply,
			Meta:    op.Meta{"cwd": cwd, "modelKey": "test:model", "thinkingLevel": "medium", "contextWindow": int64(8192), "serviceTier": "priority"},
			Content: &op.JsonContent{Raw: raw},
		},
	})
	if err != nil {
		t.Fatalf("OpMessageReplyHandler(): %v", err)
	}
	var result op.MessageReplyResult
	if err := json.Unmarshal(res.Content.(*op.JsonContent).Raw, &result); err != nil {
		t.Fatalf("decode reply result: %v", err)
	}
	if result.Record.Sender != op.MessageSenderUser || result.Record.Body != "Approve" {
		t.Fatalf("reply record = %+v", result.Record)
	}
	if result.Resolved == nil || result.Resolved.ID != published.MessageID || result.Resolved.Status != op.MessageStatusResolved {
		t.Fatalf("resolved request = %+v, want original request resolved", result.Resolved)
	}
	if result.Dispatch == nil {
		t.Fatal("dispatch = nil, want thread submit dispatch")
	}
	if result.Dispatch.Opcode != op.OpThreadSubmit {
		t.Fatalf("dispatch opcode = %s, want %s", result.Dispatch.Opcode, op.OpThreadSubmit)
	}
	if got := metaString(result.Dispatch.Meta, "threadID"); got != threadID {
		t.Fatalf("dispatch threadID = %q, want %q", got, threadID)
	}
	if got := metaString(result.Dispatch.Meta, "agentID"); got != agentID {
		t.Fatalf("dispatch agentID = %q, want %q", got, agentID)
	}
	if got := metaString(result.Dispatch.Meta, "channelID"); got != published.ChannelID {
		t.Fatalf("dispatch channelID = %q, want %q", got, published.ChannelID)
	}
	if got := metaString(result.Dispatch.Meta, "modelKey"); got != "test:model" {
		t.Fatalf("dispatch modelKey = %q, want test:model", got)
	}
	if !strings.Contains(result.Dispatch.Content, "User answered request: Approve operation") ||
		!strings.Contains(result.Dispatch.Content, "Approve this operation?") ||
		!strings.Contains(result.Dispatch.Content, "approve") ||
		strings.Contains(result.Dispatch.Content, "Reply to message:") ||
		strings.Contains(result.Dispatch.Content, "Message channel:") {
		t.Fatalf("dispatch content = %q", result.Dispatch.Content)
	}
	if result.Queue == nil || len(result.Queue.QueuedMessages.FollowUp) != 1 {
		t.Fatalf("queue ack = %+v, want one follow-up", result.Queue)
	}
	item := result.Queue.QueuedMessages.FollowUp[0]
	if item.AgentID != agentID || item.CWD != cwd {
		t.Fatalf("queued item context = %+v, want agent %q cwd %q", item, agentID, cwd)
	}
	if item.ModelKey != "test:model" || item.ThinkingLevel != "medium" || item.ContextWindow != 8192 || item.ServiceTier != "priority" {
		t.Fatalf("queued item model context = %+v, want model execution meta", item)
	}
	if item.SelectedSkillContext["messageSystem"] != true ||
		item.SelectedSkillContext["messageID"] != result.Record.ID ||
		item.SelectedSkillContext["requestTitle"] != "Approve operation" ||
		item.SelectedSkillContext["channelID"] != published.ChannelID {
		t.Fatalf("selected skill context = %+v", item.SelectedSkillContext)
	}
	contextAnswers, ok := item.SelectedSkillContext["answers"].([]any)
	if !ok || len(contextAnswers) != 1 {
		t.Fatalf("selected skill context answers = %#v", item.SelectedSkillContext["answers"])
	}
	answer, ok := contextAnswers[0].(map[string]any)
	if !ok || answer["questionID"] != "approval" || answer["optionID"] != "approve" {
		t.Fatalf("selected skill context answer = %#v", contextAnswers[0])
	}
}

func TestMessageListSkipsArchivedChannels(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	agentID := "agent-demo"
	threadID := "thread-demo"
	l, loop := testMessageLoop(t, agentID, threadID)
	published, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-archive",
		"body":      "Archive me",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	before, err := defaultMessageStore.list(op.MessageListParams{})
	if err != nil {
		t.Fatalf("list before archive: %v", err)
	}
	if len(before.Channels) != 1 || before.Channels[0].ChannelID != published.ChannelID {
		t.Fatalf("channels before archive = %+v", before.Channels)
	}
	archived, err := defaultMessageStore.archive(op.MessageArchiveParams{ChannelID: published.ChannelID})
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.Archived != 1 {
		t.Fatalf("archived = %d, want 1", archived.Archived)
	}
	after, err := defaultMessageStore.list(op.MessageListParams{})
	if err != nil {
		t.Fatalf("list after archive: %v", err)
	}
	if len(after.Channels) != 0 || len(after.Messages) != 0 {
		t.Fatalf("list after archive = %+v", after)
	}
}

func TestMessageArchiveAgentPendingRequestsOnly(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	agentID := "agent-demo"
	threadID := "thread-demo"
	l, loop := testMessageLoop(t, agentID, threadID)
	targetPending, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-pending",
		"kind":      "request",
		"body":      "Needs a decision",
		"questions": []any{
			map[string]any{
				"id":       "decision",
				"question": "Needs a decision",
				"options":  []any{map[string]any{"id": "yes", "label": "Yes"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish target pending: %v", err)
	}
	targetMessage, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-message",
		"body":      "Keep message",
	})
	if err != nil {
		t.Fatalf("publish target message: %v", err)
	}
	targetResolved, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-resolved",
		"kind":      "request",
		"body":      "Already handled",
		"questions": []any{
			map[string]any{
				"id":       "decision",
				"question": "Already handled",
				"options":  []any{map[string]any{"id": "yes", "label": "Yes"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish target resolved: %v", err)
	}
	if _, err := defaultMessageStore.update(op.MessageUpdateParams{MessageID: targetResolved.MessageID, Status: op.MessageStatusResolved}); err != nil {
		t.Fatalf("resolve target request: %v", err)
	}
	otherLoop, otherThread := testMessageLoop(t, "agent-other", "thread-other")
	otherPending, err := otherLoop.executeMessagePublish(otherThread, map[string]any{
		"channelID": "channel-other",
		"kind":      "request",
		"body":      "Other agent request",
		"questions": []any{
			map[string]any{
				"id":       "decision",
				"question": "Other agent request",
				"options":  []any{map[string]any{"id": "yes", "label": "Yes"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish other pending: %v", err)
	}

	findChannel := func(channels []op.MessageChannelSummary, channelID string) op.MessageChannelSummary {
		for _, ch := range channels {
			if ch.ChannelID == channelID {
				return ch
			}
		}
		return op.MessageChannelSummary{}
	}
	hasChannel := func(channels []op.MessageChannelSummary, channelID string) bool {
		for _, ch := range channels {
			if ch.ChannelID == channelID {
				return true
			}
		}
		return false
	}
	before, err := defaultMessageStore.list(op.MessageListParams{})
	if err != nil {
		t.Fatalf("list before archive: %v", err)
	}
	if got := findChannel(before.Channels, "channel-message").OpenCount; got != 0 {
		t.Fatalf("channel-message OpenCount before archive = %d, want 0 (plain messages are not pending requests)", got)
	}
	if got := findChannel(before.Channels, "channel-pending").OpenCount; got != 1 {
		t.Fatalf("channel-pending OpenCount before archive = %d, want 1", got)
	}

	archived, err := defaultMessageStore.archive(op.MessageArchiveParams{
		AgentID:             agentID,
		PendingRequestsOnly: true,
	})
	if err != nil {
		t.Fatalf("archive pending requests: %v", err)
	}
	if archived.Archived != 1 {
		t.Fatalf("archived = %d, want 1", archived.Archived)
	}
	after, err := defaultMessageStore.list(op.MessageListParams{})
	if err != nil {
		t.Fatalf("list after archive: %v", err)
	}
	records := map[string]op.MessageRecord{}
	for _, record := range after.Messages {
		records[record.ID] = record
	}
	if _, ok := records[targetPending.MessageID]; ok {
		t.Fatalf("target pending request was not archived")
	}
	for _, want := range []op.MessagePublishResult{targetMessage, targetResolved, otherPending} {
		if _, ok := records[want.MessageID]; !ok {
			t.Fatalf("record %s should remain after pending-request archive; got %+v", want.MessageID, after.Messages)
		}
	}
	if hasChannel(after.Channels, "channel-pending") {
		t.Fatalf("channel-pending should be dropped after archiving its only request; got %+v", after.Channels)
	}
	if got := findChannel(after.Channels, "channel-message").OpenCount; got != 0 {
		t.Fatalf("channel-message OpenCount after archive = %d, want 0", got)
	}
	if got := findChannel(after.Channels, "channel-other").OpenCount; got != 1 {
		t.Fatalf("channel-other OpenCount after archive = %d, want 1 (unaffected by other agent archive)", got)
	}
}

func TestMessageArchiveAgentAllMessages(t *testing.T) {
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	agentID := "agent-demo"
	threadID := "thread-demo"
	l, loop := testMessageLoop(t, agentID, threadID)
	targetPending, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-pending",
		"kind":      "request",
		"body":      "Needs a decision",
		"questions": []any{
			map[string]any{
				"id":       "decision",
				"question": "Needs a decision",
				"options":  []any{map[string]any{"id": "yes", "label": "Yes"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish target pending: %v", err)
	}
	targetMessage, err := l.executeMessagePublish(loop, map[string]any{
		"channelID": "channel-message",
		"body":      "Keep message",
	})
	if err != nil {
		t.Fatalf("publish target message: %v", err)
	}
	otherLoop, otherThread := testMessageLoop(t, "agent-other", "thread-other")
	otherPending, err := otherLoop.executeMessagePublish(otherThread, map[string]any{
		"channelID": "channel-other",
		"kind":      "request",
		"body":      "Other agent request",
		"questions": []any{
			map[string]any{
				"id":       "decision",
				"question": "Other agent request",
				"options":  []any{map[string]any{"id": "yes", "label": "Yes"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("publish other pending: %v", err)
	}

	before, err := defaultMessageStore.list(op.MessageListParams{})
	if err != nil {
		t.Fatalf("list before archive: %v", err)
	}
	targetChannels := 0
	for _, ch := range before.Channels {
		if ch.AgentID == agentID {
			targetChannels++
		}
	}
	if targetChannels != 2 {
		t.Fatalf("target agent channels before archive = %d, want 2", targetChannels)
	}

	archived, err := defaultMessageStore.archive(op.MessageArchiveParams{AgentID: agentID})
	if err != nil {
		t.Fatalf("archive all agent messages: %v", err)
	}
	if archived.Archived != 2 {
		t.Fatalf("archived = %d, want 2", archived.Archived)
	}
	after, err := defaultMessageStore.list(op.MessageListParams{})
	if err != nil {
		t.Fatalf("list after archive: %v", err)
	}
	records := map[string]op.MessageRecord{}
	for _, record := range after.Messages {
		records[record.ID] = record
	}
	for _, removed := range []op.MessagePublishResult{targetPending, targetMessage} {
		if _, ok := records[removed.MessageID]; ok {
			t.Fatalf("record %s should be archived", removed.MessageID)
		}
	}
	if _, ok := records[otherPending.MessageID]; !ok {
		t.Fatalf("other agent record should remain after target agent archive")
	}
	for _, channelID := range []string{"channel-pending", "channel-message"} {
		for _, ch := range after.Channels {
			if ch.ChannelID == channelID {
				t.Fatalf("channel %s should be dropped after full agent archive; got %+v", channelID, after.Channels)
			}
		}
	}
	if len(after.Channels) != 1 || after.Channels[0].ChannelID != "channel-other" {
		t.Fatalf("channels after archive = %+v, want only channel-other", after.Channels)
	}
}
