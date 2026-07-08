package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

func setupThreadControlTestSession(t *testing.T, threadID string) *op.ThreadMeta {
	t.Helper()
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")
	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "thread-control.md")
	if _, err := createThreadWithID(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "thread-control",
	}, threadID); err != nil {
		t.Fatalf("createThreadWithID(): %v", err)
	}
	meta, err := getThreadMeta(threadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta(): %v", err)
	}
	return meta
}

func TestRunLoop_InjectsFollowUpAfterAssistantStops(t *testing.T) {
	provider := &scriptedProvider{responses: []*ai.ProviderResponse{
		testProviderResponse("first answer", "", nil, ai.StopReasonStop),
		testProviderResponse("after follow-up", "", nil, ai.StopReasonStop),
	}}

	delivered := false
	loop := &AgentLoop{
		Ctx:   context.Background(),
		Agent: &Agent{},
		Model: &ModelClient{
			config:    &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000},
			Canonical: provider,
		},
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{op.NewUserMessage("start")}),
		Meta:             op.Meta{"threadID": "thread-1"},
	}

	newMessages, err := loop.runLoop(&RunLoopConfig{
		GetFollowUpMessages: func(context.Context) ([]PendingLoopMessage, error) {
			if delivered {
				return nil, nil
			}
			delivered = true
			return []PendingLoopMessage{pendingLoopMessageFromMessage(op.NewUserMessage("queued follow-up"))}, nil
		},
	})
	if err != nil {
		t.Fatalf("runLoop(): %v", err)
	}
	if len(newMessages) != 3 {
		t.Fatalf("len(newMessages) = %d, want 3", len(newMessages))
	}
	if newMessages[0].Role != op.RoleAssistant || newMessages[0].Content != "first answer" {
		t.Fatalf("first message = %+v, want first assistant", newMessages[0])
	}
	if newMessages[1].Role != op.RoleUser || newMessages[1].Content != "queued follow-up" {
		t.Fatalf("second message = %+v, want injected follow-up", newMessages[1])
	}
	if newMessages[2].Role != op.RoleAssistant || newMessages[2].Content != "after follow-up" {
		t.Fatalf("third message = %+v, want second assistant", newMessages[2])
	}
}

func TestAgentLoopContinue_AllowsAssistantTailResume(t *testing.T) {
	provider := &scriptedProvider{responses: []*ai.ProviderResponse{testProviderResponse("continued", "", nil, ai.StopReasonStop)}}

	loop := &AgentLoop{
		Ctx:   context.Background(),
		Agent: &Agent{},
		Model: &ModelClient{
			config:    &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000},
			Canonical: provider,
		},
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{
			op.NewUserMessage("initial"),
			op.NewAssistantMessage("initial answer"),
		}),
		Meta: op.Meta{"threadID": "thread-1"},
	}

	if _, err := loop.agentLoopContinue(&ContinueLoopConfig{
		PendingMessages:         []op.Message{op.NewUserMessage("resume with this")},
		SkipInitialSteeringPoll: true,
	}); err != nil {
		t.Fatalf("agentLoopContinue(): %v", err)
	}
	if len(provider.seen) != 1 {
		t.Fatalf("provider seen = %d, want 1", len(provider.seen))
	}
	continuedContext := provider.seen[0]
	if len(continuedContext) != 3 {
		t.Fatalf("continued context len = %d, want 3", len(continuedContext))
	}
	if continuedContext[2].Role != op.RoleUser || continuedContext[2].Content != "resume with this" {
		t.Fatalf("continued context[2] = %+v, want injected user message", continuedContext[2])
	}
	lastMessages := opMessagesFromCanonicalHistory(loop.canonicalHistory)
	last := lastMessages[len(lastMessages)-1]
	if last.Role != op.RoleAssistant || last.Content != "continued" {
		t.Fatalf("last loop message = %+v, want continued assistant", last)
	}
}

func TestThreadControlSteerCapturesRequestAgentContext(t *testing.T) {
	threadMeta := setupThreadControlTestSession(t, "thread-turn-agent")
	turnCWD := filepath.Dir(filepath.Dir(filepath.Dir(threadMeta.ChatPath)))
	steerRes, err := OpThreadSteerHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode: op.OpThreadSteer,
			Meta: op.Meta{
				"threadID":      threadMeta.ThreadID,
				"agentID":       "agent-gbrain",
				"agentName":     "gbrain",
				"cwd":           turnCWD,
				"contextWindow": 300000,
				"serviceTier":   "priority",
			},
			Content: &op.TextContent{Text: "use gbrain"},
		},
	})
	if err != nil {
		t.Fatalf("OpThreadSteerHandler(): %v", err)
	}
	jsonContent, ok := steerRes.Content.(*op.JsonContent)
	if !ok {
		t.Fatalf("steer content type = %T, want *op.JsonContent", steerRes.Content)
	}
	var steerAck op.ThreadControlAck
	if err := json.Unmarshal(jsonContent.Raw, &steerAck); err != nil {
		t.Fatalf("decode steer ack: %v", err)
	}
	if len(steerAck.QueuedMessages.Steering) != 1 {
		t.Fatalf("ack queued steering = %+v, want one item", steerAck.QueuedMessages.Steering)
	}
	item := steerAck.QueuedMessages.Steering[0]
	if item.AgentID != "agent-gbrain" || item.AgentName != "gbrain" || item.CWD != turnCWD {
		t.Fatalf("queued item agent context = (%q, %q, %q), want (%q, %q, %q)", item.AgentID, item.AgentName, item.CWD, "agent-gbrain", "gbrain", turnCWD)
	}
	if item.ContextWindow != 300000 || item.ServiceTier != "priority" {
		t.Fatalf("queued item model preference = (%d, %q), want (300000, priority)", item.ContextWindow, item.ServiceTier)
	}
	snapshot, err := getQueuedMessagesSnapshot(op.ThreadMetaQuery{ThreadID: threadMeta.ThreadID})
	if err != nil {
		t.Fatalf("getQueuedMessagesSnapshot(): %v", err)
	}
	if len(snapshot.Steering) != 1 || snapshot.Steering[0].AgentID != "agent-gbrain" || snapshot.Steering[0].AgentName != "gbrain" || snapshot.Steering[0].CWD != turnCWD {
		t.Fatalf("persisted queue = %+v, want request agent context", snapshot.Steering)
	}
	if snapshot.Steering[0].ContextWindow != 300000 || snapshot.Steering[0].ServiceTier != "priority" {
		t.Fatalf("persisted queue model preference = %+v, want contextWindow/serviceTier", snapshot.Steering[0])
	}
}

func TestRunContinuation_AllowsAssistantTailResumeWithPersistedQueue(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "resume-queued.md")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "resume-queued",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	if err := appendMessagesToThread(*meta, []op.Message{
		op.NewUserMessage("initial"),
		op.NewAssistantMessage("finished assistant turn"),
	}); err != nil {
		t.Fatalf("appendMessagesToThread: %v", err)
	}
	if _, _, err := appendQueuedMessageToThread(*meta, op.ThreadQueueKindFollowUp, op.NewUserMessage("queued resume"), "", "", "", 0, "", nil, nil, false); err != nil {
		t.Fatalf("appendQueuedMessageToThread: %v", err)
	}

	provider := &completeOnlyProvider{response: testProviderResponse("continued from queue", "", nil, ai.StopReasonStop)}
	loop := &AgentLoop{
		Ctx:              context.Background(),
		Cancel:           func() {},
		Agent:            &Agent{AgentID: agentID, ToolSpecs: map[string]*op.ToolSpec{}},
		Meta:             op.Meta{"threadID": result.ThreadID, "chatPath": chatPath, "agentID": agentID},
		ThreadID:         result.ThreadID,
		ChatPath:         chatPath,
		Workdir:          cwd,
		Model:            &ModelClient{config: &op.ModelConfig{ID: "test", Name: "test", ContextWindow: 1_000_000}, Canonical: provider},
		threadMeta:       *meta,
		canonicalHistory: ai.CanonicalMessagesFromOp([]op.Message{op.NewUserMessage("initial"), op.NewAssistantMessage("finished assistant turn")}),
		stepSeq:          1,
	}

	if _, err := loop.runContinuation(nil); err != nil {
		t.Fatalf("runContinuation(): %v", err)
	}
	if len(provider.seen) != 1 {
		t.Fatalf("provider seen = %d, want 1", len(provider.seen))
	}
	if len(provider.seen[0]) != 3 {
		t.Fatalf("provider seen context len = %d, want 3", len(provider.seen[0]))
	}
	if provider.seen[0][2].Role != op.RoleUser || provider.seen[0][2].Content != "queued resume" {
		t.Fatalf("provider seen[0][2] = %+v, want queued resume user message", provider.seen[0][2])
	}
	sessionCtx, err := loadThreadContext(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("loadThreadContext: %v", err)
	}
	if len(sessionCtx.queuedMessages.FollowUp) != 0 {
		t.Fatalf("queued follow-up after continuation = %+v, want empty", sessionCtx.queuedMessages.FollowUp)
	}
	persisted := opMessagesFromCanonicalHistory(sessionCtx.canonicalMessages)
	if len(persisted) != 4 {
		t.Fatalf("persisted messages len = %d, want 4", len(persisted))
	}
	if persisted[2].Role != op.RoleUser || persisted[2].Content != "queued resume" {
		t.Fatalf("persisted[2] = %+v, want queued resume user message", persisted[2])
	}
	if persisted[3].Role != op.RoleAssistant || persisted[3].Content != "continued from queue" {
		t.Fatalf("persisted[3] = %+v, want continued assistant message", persisted[3])
	}
}

func TestThreadControlHandlers_SupportMarkdownImagesAndInterruptSnapshot(t *testing.T) {
	threadMeta := setupThreadControlTestSession(t, "thread-test")
	interrupted := false
	runtime := newRuntimeLoop("thread-test", threadMeta.ChatPath, func() {
		interrupted = true
	})
	if err := registerRuntimeLoop(runtime); err != nil {
		t.Fatalf("registerRuntimeLoop(): %v", err)
	}
	defer unregisterRuntimeLoop("thread-test", runtime)

	raw, err := json.Marshal(op.Message{
		Role:    op.RoleUser,
		Content: "interrupt\n\n![image-1](/tmp/work/.agent/assets/images/image-1.png)",
	})
	if err != nil {
		t.Fatalf("marshal queued markdown image message: %v", err)
	}

	steerRes, err := OpThreadSteerHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode: op.OpThreadSteer,
			Meta: op.Meta{
				"threadID":             threadMeta.ThreadID,
				"chatPath":             threadMeta.ChatPath,
				"agentID":              threadMeta.AgentID,
				"selectedSkillIDs":     []any{"skill-plan"},
				"selectedSkillContext": map[string]any{"planFilePath": "/tmp/plan.md"},
			},
			Content: &op.JsonContent{Raw: raw},
		},
	})
	if err != nil {
		t.Fatalf("OpThreadSteerHandler(): %v", err)
	}
	if steerRes == nil {
		t.Fatal("OpThreadSteerHandler() returned nil result")
	}
	jsonContent, ok := steerRes.Content.(*op.JsonContent)
	if !ok {
		t.Fatalf("steer content type = %T, want *op.JsonContent", steerRes.Content)
	}
	var steerAck op.ThreadControlAck
	if err := json.Unmarshal(jsonContent.Raw, &steerAck); err != nil {
		t.Fatalf("decode steer ack: %v", err)
	}
	if len(steerAck.QueuedMessages.Steering) != 1 || !strings.Contains(steerAck.QueuedMessages.Steering[0].Message.Content, "![image-1]") {
		t.Fatalf("steer ack queued messages = %+v, want one markdown-image steering item", steerAck.QueuedMessages.Steering)
	}
	queuedSnapshot, err := getQueuedMessagesSnapshot(op.ThreadMetaQuery{
		ThreadID: threadMeta.ThreadID,
		ChatPath: threadMeta.ChatPath,
		AgentID:  threadMeta.AgentID,
	})
	if err != nil {
		t.Fatalf("getQueuedMessagesSnapshot(): %v", err)
	}
	if len(queuedSnapshot.Steering) != 1 || !strings.Contains(queuedSnapshot.Steering[0].Message.Content, "![image-1]") {
		t.Fatalf("queued snapshot = %+v, want one markdown-image steering item", queuedSnapshot)
	}
	if len(queuedSnapshot.Steering[0].SelectedSkillIDs) != 1 || queuedSnapshot.Steering[0].SelectedSkillIDs[0] != "skill-plan" {
		t.Fatalf("queued selectedSkillIDs = %+v, want skill-plan", queuedSnapshot.Steering[0].SelectedSkillIDs)
	}
	if got, _ := queuedSnapshot.Steering[0].SelectedSkillContext["planFilePath"].(string); got != "/tmp/plan.md" {
		t.Fatalf("queued selectedSkillContext = %+v, want planFilePath", queuedSnapshot.Steering[0].SelectedSkillContext)
	}
	followRes, err := OpThreadFollowUpHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode: op.OpThreadFollowUp,
			Meta: op.Meta{
				"threadID": threadMeta.ThreadID,
				"chatPath": threadMeta.ChatPath,
				"agentID":  threadMeta.AgentID,
			},
			Content: &op.TextContent{Text: "afterwards"},
		},
	})
	if err != nil {
		t.Fatalf("OpThreadFollowUpHandler(): %v", err)
	}
	if followRes == nil {
		t.Fatal("OpThreadFollowUpHandler() returned nil result")
	}

	interruptRes, err := OpThreadInterruptedHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode: op.OpThreadInterrupted,
			Meta: op.Meta{
				"threadID": threadMeta.ThreadID,
				"chatPath": threadMeta.ChatPath,
				"agentID":  threadMeta.AgentID,
			},
		},
	})
	if err != nil {
		t.Fatalf("OpThreadInterruptedHandler(): %v", err)
	}
	if !interrupted {
		t.Fatal("runtime cancel was not called")
	}
	jsonContent, ok = interruptRes.Content.(*op.JsonContent)
	if !ok {
		t.Fatalf("interrupt content type = %T, want *op.JsonContent", interruptRes.Content)
	}
	var ack op.ThreadControlAck
	if err := json.Unmarshal(jsonContent.Raw, &ack); err != nil {
		t.Fatalf("decode interrupt ack: %v", err)
	}
	if len(ack.QueuedMessages.FollowUp) != 1 || ack.QueuedMessages.FollowUp[0].Message.Content != "afterwards" {
		t.Fatalf("interrupt queued follow-up = %+v, want afterwards", ack.QueuedMessages.FollowUp)
	}
}

func TestThreadControlHandlers_QueueGetPromoteAndRemove(t *testing.T) {
	threadMeta := setupThreadControlTestSession(t, "thread-test")

	if _, err := OpThreadFollowUpHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode: op.OpThreadFollowUp,
			Meta: op.Meta{
				"threadID": threadMeta.ThreadID,
				"chatPath": threadMeta.ChatPath,
				"agentID":  threadMeta.AgentID,
			},
			Content: &op.TextContent{Text: "queued"},
		},
	}); err != nil {
		t.Fatalf("OpThreadFollowUpHandler(queued): %v", err)
	}
	if _, err := OpThreadFollowUpHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode: op.OpThreadFollowUp,
			Meta: op.Meta{
				"threadID": threadMeta.ThreadID,
				"chatPath": threadMeta.ChatPath,
				"agentID":  threadMeta.AgentID,
			},
			Content: &op.TextContent{Text: "queued-2"},
		},
	}); err != nil {
		t.Fatalf("OpThreadFollowUpHandler(queued-2): %v", err)
	}
	getRes, err := OpThreadQueueGetHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode: op.OpThreadQueueGet,
			Meta: op.Meta{
				"threadID": threadMeta.ThreadID,
				"chatPath": threadMeta.ChatPath,
				"agentID":  threadMeta.AgentID,
			},
		},
	})
	if err != nil {
		t.Fatalf("OpThreadQueueGetHandler(): %v", err)
	}
	getJSON, ok := getRes.Content.(*op.JsonContent)
	if !ok {
		t.Fatalf("queue get content type = %T, want *op.JsonContent", getRes.Content)
	}
	var getAck op.ThreadControlAck
	if err := json.Unmarshal(getJSON.Raw, &getAck); err != nil {
		t.Fatalf("decode queue get ack: %v", err)
	}
	if len(getAck.QueuedMessages.FollowUp) != 2 {
		t.Fatalf("queue get ack = %+v, want two follow-up items", getAck.QueuedMessages.FollowUp)
	}
	promoteRes, err := OpThreadFollowUpPromoteHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode: op.OpThreadFollowUpPromote,
			Meta: op.Meta{
				"threadID": threadMeta.ThreadID,
				"chatPath": threadMeta.ChatPath,
				"agentID":  threadMeta.AgentID,
				"itemID":   getAck.QueuedMessages.FollowUp[1].ID,
			},
		},
	})
	if err != nil {
		t.Fatalf("OpThreadFollowUpPromoteHandler(): %v", err)
	}
	promoteJSON, ok := promoteRes.Content.(*op.JsonContent)
	if !ok {
		t.Fatalf("queue promote content type = %T, want *op.JsonContent", promoteRes.Content)
	}
	var promoteAck op.ThreadControlAck
	if err := json.Unmarshal(promoteJSON.Raw, &promoteAck); err != nil {
		t.Fatalf("decode queue promote ack: %v", err)
	}
	if len(promoteAck.QueuedMessages.Steering) != 1 || promoteAck.QueuedMessages.Steering[0].Message.Content != "queued-2" {
		t.Fatalf("queue promote steering = %+v, want queued-2", promoteAck.QueuedMessages.Steering)
	}
	if len(promoteAck.QueuedMessages.FollowUp) != 1 || promoteAck.QueuedMessages.FollowUp[0].Message.Content != "queued" {
		t.Fatalf("queue promote follow-up = %+v, want queued", promoteAck.QueuedMessages.FollowUp)
	}
	removeRes, err := OpThreadQueueRemoveHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode: op.OpThreadQueueRemove,
			Meta: op.Meta{
				"threadID":  threadMeta.ThreadID,
				"chatPath":  threadMeta.ChatPath,
				"agentID":   threadMeta.AgentID,
				"queueKind": string(op.ThreadQueueKindFollowUp),
				"itemID":    getAck.QueuedMessages.FollowUp[0].ID,
			},
		},
	})
	if err != nil {
		t.Fatalf("OpThreadQueueRemoveHandler(): %v", err)
	}
	removeJSON, ok := removeRes.Content.(*op.JsonContent)
	if !ok {
		t.Fatalf("queue remove content type = %T, want *op.JsonContent", removeRes.Content)
	}
	var removeAck op.ThreadControlAck
	if err := json.Unmarshal(removeJSON.Raw, &removeAck); err != nil {
		t.Fatalf("decode queue remove ack: %v", err)
	}
	if removeAck.RemovedItem == nil || removeAck.RemovedItem.Message.Content != "queued" {
		t.Fatalf("queue remove ack removed item = %+v, want queued", removeAck.RemovedItem)
	}
	if len(removeAck.QueuedMessages.FollowUp) != 0 {
		t.Fatalf("queue remove ack follow-up = %+v, want empty", removeAck.QueuedMessages.FollowUp)
	}
}

func TestThreadControlHandlers_QueueGetReturnsEmptySnapshotWithoutRuntime(t *testing.T) {
	threadMeta := setupThreadControlTestSession(t, "thread-missing-runtime")
	getRes, err := OpThreadQueueGetHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{
			OpCode: op.OpThreadQueueGet,
			Meta: op.Meta{
				"threadID": threadMeta.ThreadID,
				"chatPath": threadMeta.ChatPath,
				"agentID":  threadMeta.AgentID,
			},
		},
	})
	if err != nil {
		t.Fatalf("OpThreadQueueGetHandler(): %v", err)
	}
	if getRes == nil {
		t.Fatal("OpThreadQueueGetHandler() returned nil result")
	}
	getJSON, ok := getRes.Content.(*op.JsonContent)
	if !ok {
		t.Fatalf("queue get content type = %T, want *op.JsonContent", getRes.Content)
	}
	var getAck op.ThreadControlAck
	if err := json.Unmarshal(getJSON.Raw, &getAck); err != nil {
		t.Fatalf("decode queue get ack: %v", err)
	}
	if getAck.ThreadID != threadMeta.ThreadID {
		t.Fatalf("queue get threadID = %q, want %q", getAck.ThreadID, threadMeta.ThreadID)
	}
	if len(getAck.QueuedMessages.Steering) != 0 || len(getAck.QueuedMessages.FollowUp) != 0 {
		t.Fatalf("queue get ack = %+v, want empty queued messages", getAck.QueuedMessages)
	}
}

func TestOpThreadActiveListHandler(t *testing.T) {
	runtime := newRuntimeLoop("thread-active", "/tmp/chat.md", nil)
	if err := registerRuntimeLoop(runtime); err != nil {
		t.Fatalf("registerRuntimeLoop(): %v", err)
	}
	defer unregisterRuntimeLoop("thread-active", runtime)

	res, err := OpThreadActiveListHandler(context.Background(), &op.OpAgentRequest{
		Params: &op.OpAgentParams{OpCode: op.OpThreadActiveList},
	})
	if err != nil {
		t.Fatalf("OpThreadActiveListHandler(): %v", err)
	}
	jsonContent, ok := res.Content.(*op.JsonContent)
	if !ok {
		t.Fatalf("content type = %T, want *op.JsonContent", res.Content)
	}
	var payload op.ThreadActiveList
	if err := json.Unmarshal(jsonContent.Raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if len(payload.Threads) != 1 {
		t.Fatalf("active threads len = %d, want 1", len(payload.Threads))
	}
	if payload.Threads[0].ThreadID != "thread-active" || payload.Threads[0].ChatPath != "/tmp/chat.md" {
		t.Fatalf("active thread = %+v, want thread-active /tmp/chat.md", payload.Threads[0])
	}
}

func TestDecodeThreadControlUserMessageDoesNotRequireChatPath(t *testing.T) {
	raw, err := json.Marshal(op.Message{
		Role:    op.RoleUser,
		Content: "interrupt\n\n![image-1](/tmp/work/.agent/assets/images/image-1.png)",
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	msg, err := decodeThreadControlUserMessage(&op.JsonContent{Raw: raw})
	if err != nil {
		t.Fatalf("decodeThreadControlUserMessage(): %v", err)
	}
	if msg.Role != op.RoleUser {
		t.Fatalf("role = %q, want user", msg.Role)
	}
	if !strings.Contains(msg.Content, "![image-1](/tmp/work/.agent/assets/images/image-1.png)") {
		t.Fatalf("unexpected content: %+v", msg)
	}
}
