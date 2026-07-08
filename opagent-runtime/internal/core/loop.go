package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/packages/agentctx/compaction"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

// Loop is the minimal runtime handle for a running agent loop (tool calls, registry).
// It is built from AgentLoop when running the loop.
type Loop struct {
	Ctx      context.Context
	Meta     op.Meta
	ThreadID string
	TurnID   string
	Workdir  string
}

type AgentLoop struct {
	Ctx                  context.Context    `json:"-"`
	Cancel               context.CancelFunc `json:"-"`
	Agent                *Agent
	Meta                 op.Meta
	ThreadID             string
	TurnID               string
	ChatPath             string
	Workdir              string
	ThinkingLevel        string
	ContextWindow        int64
	PlanTurn             bool
	SelectedSkillIDs     []string
	SelectedSkillContext op.Meta
	Model                *ModelClient
	threadMeta           op.ThreadMeta
	userMessage          op.Message
	canonicalHistory     []ai.ConversationMessage
	stepSeq              int
	tools                []*op.ToolUse
	getSteeringMessages  func(context.Context) ([]PendingLoopMessage, error)
	initialQueuePending  *PendingLoopMessage
	rebuildModel         func(context.Context, op.Meta) (*ModelClient, error)
}

func resolvedThreadPath(meta op.ThreadMeta) string {
	path := strings.TrimSpace(meta.Path)
	if path != "" {
		return path
	}
	return strings.TrimSpace(meta.ChatPath)
}

func applyLoopThreadMeta(meta op.Meta, threadMeta op.ThreadMeta) (op.Meta, string) {
	meta = applyResolvedThreadMetaToMeta(meta, threadMeta)
	if threadCWD := strings.TrimSpace(threadMeta.CWD); threadCWD != "" && strings.TrimSpace(metaString(meta, "cwd")) == "" {
		meta["cwd"] = threadCWD
	}
	thinkingLevel := ""
	if value, ok := meta["thinkingLevel"].(string); ok && strings.TrimSpace(value) != "" {
		thinkingLevel = strings.TrimSpace(value)
	}
	return meta, thinkingLevel
}

func applyAgentModelMeta(meta op.Meta, node *op.OpNode) (op.Meta, error) {
	next := op.Meta{}
	if meta != nil {
		next = meta.Clone()
	}
	if next == nil {
		next = op.Meta{}
	}
	modelKey := strings.TrimSpace(metaString(next, "modelKey"))
	delete(next, "model")
	if modelKey == "" {
		return nil, fmt.Errorf("modelKey is required")
	}
	if _, err := config.GetModelConfig(modelKey); err != nil {
		agentID := ""
		if node != nil {
			agentID = strings.TrimSpace(node.ID)
		}
		publishModelConfigMessage(next,
			"Model needs configuration",
			"This agent turn cannot start because the configured model is not available. Choose an enabled model and retry.\n\nModel: "+modelKey+"\n\nReason: "+err.Error(),
			"model_unavailable",
			op.Meta{
				"agentID":  agentID,
				"modelKey": modelKey,
			},
		)
		return nil, fmt.Errorf("modelKey %q is not available in local models.json: %w", modelKey, err)
	}
	next["modelKey"] = modelKey
	return next, nil
}

func NewAgentLoop(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*AgentLoop, error) {
	userMessage, err := buildUserMessage(content)
	if err != nil {
		return nil, err
	}

	threadID, ok := meta["threadID"].(string)
	if !ok || strings.TrimSpace(threadID) == "" {
		return nil, fmt.Errorf("threadID is required in meta")
	}
	agentID, ok := meta["agentID"].(string)
	if !ok || strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("meta.agentID is required")
	}

	chatContext, err := loadThreadContext(strings.TrimSpace(threadID), strings.TrimSpace(agentID))
	if err != nil {
		return nil, fmt.Errorf("failed to load thread context: %w", err)
	}
	replayableCanonical := ai.NormalizeReplayableCanonicalMessages(chatContext.canonicalMessages)
	if tailStatus, continuationReason := ai.CanonicalMessagesTailState(chatContext.canonicalMessages); tailStatus == op.ThreadTailNeedsContinuation &&
		continuationReason != op.ThreadContinuationAssistantError {
		return nil, fmt.Errorf("thread requires continuation before accepting a new prompt")
	}
	turnID := op.GenerateTurnID()
	meta, thinkingLevel := applyLoopThreadMeta(meta, chatContext.meta)
	meta, err = applyAgentModelMeta(meta, node)
	if err != nil {
		return nil, err
	}
	meta["turnID"] = turnID
	canonicalHistory := append([]ai.ConversationMessage(nil), replayableCanonical.Messages...)
	canonicalHistory = append(canonicalHistory, ai.CanonicalMessagesFromOp([]op.Message{userMessage})...)
	ctx, cancel := context.WithCancel(ctx)

	agent, err := NewAgent(ctx, node, meta)
	if err != nil {
		slog.Error("failed to create agent", "error", err, "agentID", node.ID)
		cancel()
		return nil, err
	}
	model, err := NewModelClient(ctx, "", meta)
	if err != nil {
		slog.Error("failed to create model", "error", err, "agentID", node.ID)
		cancel()
		return nil, err
	}
	contextWindow := effectiveContextWindowForMeta(meta, model.config.ContextWindow)
	return &AgentLoop{
		Ctx:                  ctx,
		Cancel:               cancel,
		Agent:                agent,
		Model:                model,
		Meta:                 meta,
		ThreadID:             strings.TrimSpace(threadID),
		TurnID:               turnID,
		ChatPath:             resolvedThreadPath(chatContext.meta),
		Workdir:              strings.TrimSpace(chatContext.meta.CWD),
		ThinkingLevel:        strings.TrimSpace(thinkingLevel),
		ContextWindow:        contextWindow,
		PlanTurn:             metaBool(meta, "planTurn"),
		SelectedSkillIDs:     append([]string(nil), selectedSkillIDsFromMeta(meta)...),
		SelectedSkillContext: selectedSkillContextFromMeta(meta),
		threadMeta:           chatContext.meta,
		userMessage:          userMessage,
		canonicalHistory:     canonicalHistory,
		stepSeq:              1,
		rebuildModel: func(ctx context.Context, meta op.Meta) (*ModelClient, error) {
			return NewModelClient(ctx, "", meta)
		},
	}, nil
}

func NewContinuationAgentLoop(ctx context.Context, node *op.OpNode, meta op.Meta) (*AgentLoop, error) {
	threadID, ok := meta["threadID"].(string)
	if !ok || strings.TrimSpace(threadID) == "" {
		return nil, fmt.Errorf("threadID is required in meta")
	}
	agentID, ok := meta["agentID"].(string)
	if !ok || strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("meta.agentID is required")
	}

	chatContext, err := loadThreadContext(strings.TrimSpace(threadID), strings.TrimSpace(agentID))
	if err != nil {
		return nil, fmt.Errorf("failed to load thread context: %w", err)
	}
	replayableCanonical := ai.NormalizeReplayableCanonicalMessages(chatContext.canonicalMessages)
	if len(replayableCanonical.Messages) == 0 {
		return nil, fmt.Errorf("cannot continue: no replayable canonical messages in context")
	}
	turnID := op.GenerateTurnID()
	meta, thinkingLevel := applyLoopThreadMeta(meta, chatContext.meta)
	meta, err = applyAgentModelMeta(meta, node)
	if err != nil {
		return nil, err
	}
	meta["turnID"] = turnID

	canonicalHistory := append([]ai.ConversationMessage(nil), replayableCanonical.Messages...)
	ctx, cancel := context.WithCancel(ctx)

	agent, err := NewAgent(ctx, node, meta)
	if err != nil {
		slog.Error("failed to create agent", "error", err, "agentID", node.ID)
		cancel()
		return nil, err
	}
	model, err := NewModelClient(ctx, "", meta)
	if err != nil {
		slog.Error("failed to create model", "error", err, "agentID", node.ID)
		cancel()
		return nil, err
	}
	contextWindow := effectiveContextWindowForMeta(meta, model.config.ContextWindow)
	return &AgentLoop{
		Ctx:                  ctx,
		Cancel:               cancel,
		Agent:                agent,
		Model:                model,
		Meta:                 meta,
		ThreadID:             strings.TrimSpace(threadID),
		TurnID:               turnID,
		ChatPath:             resolvedThreadPath(chatContext.meta),
		Workdir:              strings.TrimSpace(chatContext.meta.CWD),
		ThinkingLevel:        strings.TrimSpace(thinkingLevel),
		ContextWindow:        contextWindow,
		PlanTurn:             metaBool(meta, "planTurn"),
		SelectedSkillIDs:     append([]string(nil), selectedSkillIDsFromMeta(meta)...),
		SelectedSkillContext: selectedSkillContextFromMeta(meta),
		threadMeta:           chatContext.meta,
		canonicalHistory:     canonicalHistory,
		stepSeq:              1,
		rebuildModel: func(ctx context.Context, meta op.Meta) (*ModelClient, error) {
			return NewModelClient(ctx, "", meta)
		},
	}, nil
}

func NewQueuedPromptAgentLoop(ctx context.Context, node *op.OpNode, meta op.Meta, pending PendingLoopMessage) (*AgentLoop, error) {
	threadID, ok := meta["threadID"].(string)
	if !ok || strings.TrimSpace(threadID) == "" {
		return nil, fmt.Errorf("threadID is required in meta")
	}
	agentID, ok := meta["agentID"].(string)
	if !ok || strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("meta.agentID is required")
	}

	chatContext, err := loadThreadContext(strings.TrimSpace(threadID), strings.TrimSpace(agentID))
	if err != nil {
		return nil, fmt.Errorf("failed to load thread context: %w", err)
	}
	replayableCanonical := ai.NormalizeReplayableCanonicalMessages(chatContext.canonicalMessages)
	turnID := op.GenerateTurnID()
	meta, thinkingLevel := applyLoopThreadMeta(meta, chatContext.meta)
	meta, err = applyAgentModelMeta(meta, node)
	if err != nil {
		return nil, err
	}
	meta["turnID"] = turnID

	canonicalHistory := append([]ai.ConversationMessage(nil), replayableCanonical.Messages...)
	canonicalHistory = append(canonicalHistory, ai.CanonicalMessagesFromOp([]op.Message{pending.Message})...)
	ctx, cancel := context.WithCancel(ctx)

	agent, err := NewAgent(ctx, node, meta)
	if err != nil {
		slog.Error("failed to create agent", "error", err, "agentID", node.ID)
		cancel()
		return nil, err
	}
	model, err := NewModelClient(ctx, "", meta)
	if err != nil {
		slog.Error("failed to create model", "error", err, "agentID", node.ID)
		cancel()
		return nil, err
	}
	contextWindow := effectiveContextWindowForMeta(meta, model.config.ContextWindow)
	pendingClone := pending
	pendingClone.SelectedSkillIDs = append([]string(nil), pending.SelectedSkillIDs...)
	pendingClone.SelectedSkillContext = pending.SelectedSkillContext.Clone()
	return &AgentLoop{
		Ctx:                  ctx,
		Cancel:               cancel,
		Agent:                agent,
		Model:                model,
		Meta:                 meta,
		ThreadID:             strings.TrimSpace(threadID),
		TurnID:               turnID,
		ChatPath:             resolvedThreadPath(chatContext.meta),
		Workdir:              strings.TrimSpace(chatContext.meta.CWD),
		ThinkingLevel:        strings.TrimSpace(thinkingLevel),
		ContextWindow:        contextWindow,
		PlanTurn:             metaBool(meta, "planTurn"),
		SelectedSkillIDs:     append([]string(nil), selectedSkillIDsFromMeta(meta)...),
		SelectedSkillContext: selectedSkillContextFromMeta(meta),
		threadMeta:           chatContext.meta,
		userMessage:          pending.Message,
		canonicalHistory:     canonicalHistory,
		stepSeq:              1,
		initialQueuePending:  &pendingClone,
		rebuildModel: func(ctx context.Context, meta op.Meta) (*ModelClient, error) {
			return NewModelClient(ctx, "", meta)
		},
	}, nil
}

func opMessagesFromCanonicalHistory(messages []ai.ConversationMessage) []op.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]op.Message, 0, len(messages))
	for _, msg := range messages {
		converted, err := ai.OpMessageFromCanonical(msg)
		if err != nil || converted.Role == "" {
			continue
		}
		out = append(out, converted)
	}
	return out
}

func (l *AgentLoop) loopThreadMeta() op.ThreadMeta {
	if l == nil {
		return op.ThreadMeta{}
	}
	meta := l.threadMeta
	if strings.TrimSpace(meta.ThreadID) == "" {
		meta.ThreadID = strings.TrimSpace(l.ThreadID)
	}
	if strings.TrimSpace(meta.Path) == "" {
		meta.Path = strings.TrimSpace(l.ChatPath)
	}
	if strings.TrimSpace(meta.ChatPath) == "" {
		meta.ChatPath = strings.TrimSpace(meta.Path)
	}
	if strings.TrimSpace(meta.CWD) == "" {
		meta.CWD = strings.TrimSpace(l.Workdir)
	}
	if turnCWD := strings.TrimSpace(metaString(l.Meta, "cwd")); turnCWD != "" {
		meta.CWD = turnCWD
	}
	if turnAgentID := strings.TrimSpace(metaString(l.Meta, "agentID")); turnAgentID != "" {
		meta.AgentID = turnAgentID
	} else if l.Agent != nil && strings.TrimSpace(l.Agent.AgentID) != "" {
		meta.AgentID = strings.TrimSpace(l.Agent.AgentID)
	}
	if strings.TrimSpace(meta.FileID) == "" {
		meta.FileID = strings.TrimSpace(metaString(l.Meta, "fileID"))
	}
	return meta
}

func peekQueuedPendingForLoop(meta op.ThreadMeta, queueKind op.ThreadQueueKind) ([]PendingLoopMessage, error) {
	if strings.TrimSpace(meta.ThreadID) == "" {
		return nil, nil
	}
	snapshot, err := getQueuedMessagesSnapshot(threadMetaQuery(meta))
	if err != nil {
		if isThreadNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	switch queueKind {
	case op.ThreadQueueKindSteering:
		if len(snapshot.Steering) == 0 {
			return nil, nil
		}
		return []PendingLoopMessage{pendingLoopMessageFromQueueItem(snapshot.Steering[0], op.ThreadQueueKindSteering)}, nil
	case op.ThreadQueueKindFollowUp:
		if len(snapshot.FollowUp) == 0 {
			return nil, nil
		}
		return []PendingLoopMessage{pendingLoopMessageFromQueueItem(snapshot.FollowUp[0], op.ThreadQueueKindFollowUp)}, nil
	default:
		return nil, nil
	}
}

func peekNextQueuedPendingForLoop(meta op.ThreadMeta) ([]PendingLoopMessage, error) {
	steering, err := peekQueuedPendingForLoop(meta, op.ThreadQueueKindSteering)
	if err != nil || len(steering) > 0 {
		return steering, err
	}
	return peekQueuedPendingForLoop(meta, op.ThreadQueueKindFollowUp)
}

func (l *AgentLoop) runtimeQueueRunConfig(initialPending []PendingLoopMessage, skipInitialSteeringPoll bool) *RunLoopConfig {
	threadMeta := l.loopThreadMeta()
	return &RunLoopConfig{
		InitialPendingMessages:  append([]PendingLoopMessage(nil), initialPending...),
		SkipInitialSteeringPoll: skipInitialSteeringPoll,
		GetSteeringMessages: func(context.Context) ([]PendingLoopMessage, error) {
			return peekQueuedPendingForLoop(threadMeta, op.ThreadQueueKindSteering)
		},
		GetFollowUpMessages: func(context.Context) ([]PendingLoopMessage, error) {
			return peekQueuedPendingForLoop(threadMeta, op.ThreadQueueKindFollowUp)
		},
	}
}

// agentLoop starts an agent loop with new prompt message(s).
// Loads history from chat file if meta["chatFile"] is set; then appends user prompt and runs runLoop.
func (l *AgentLoop) run() (*op.OpNodeResult, error) {
	runtime := newRuntimeLoop(l.ThreadID, l.ChatPath, l.Cancel)
	if err := registerRuntimeLoop(runtime); err != nil {
		return nil, err
	}
	defer unregisterRuntimeLoop(l.ThreadID, runtime)

	if l.initialQueuePending != nil && hasPendingLoopQueueSource(*l.initialQueuePending) {
		if err := l.persistPendingLoopMessages([]PendingLoopMessage{*l.initialQueuePending}); err != nil {
			return nil, fmt.Errorf("append queued user message to thread: %w", err)
		}
		emitQueuedMessageDelivered(l.Meta, string(l.initialQueuePending.QueueKind), l.initialQueuePending.Message, l.initialQueuePending.QueueItemID)
	} else {
		if err := l.persistMessages(l.userMessage); err != nil {
			return nil, fmt.Errorf("append user message to thread: %w", err)
		}
	}
	emitStableMessageStep(l.Meta, "user_step", l.userMessage, l.nextStepSeq())
	canonicalStart := len(l.canonicalHistory)

	msgs, err := l.runLoop(l.runtimeQueueRunConfig(nil, true))
	if err != nil {
		if terminal, ok := l.persistTerminalAssistantForError(err); ok {
			l.emitTurnResultAndStreamEnd(l.userMessage, append(append([]op.Message(nil), msgs...), terminal), canonicalStart)
		}
		return nil, err
	}

	// runLoop returns only *new* messages (assistant + optional tool results / steering),
	// so a normal completion without tool calls will legitimately return exactly 1 message.
	if len(msgs) == 0 {
		err := fmt.Errorf("no messages returned from runLoop")
		if terminal, ok := l.persistTerminalAssistantForError(err); ok {
			l.emitTurnResultAndStreamEnd(l.userMessage, []op.Message{terminal}, canonicalStart)
		}
		return nil, err
	}

	l.emitTurnResultAndStreamEnd(l.userMessage, msgs, canonicalStart)

	return l.opNodeResultFromMessages(msgs)
}

func (l *AgentLoop) emitTurnResultAndStreamEnd(userMessage op.Message, msgs []op.Message, canonicalStart int) {
	if l == nil {
		return
	}
	turnCanonicalMessages := make([]ai.ConversationMessage, 0)
	if userMessage.Role != "" {
		turnCanonicalMessages = append(turnCanonicalMessages, ai.CanonicalMessagesFromOp([]op.Message{userMessage})...)
	}
	if canonicalStart >= 0 && canonicalStart < len(l.canonicalHistory) {
		turnCanonicalMessages = append(turnCanonicalMessages, l.canonicalHistory[canonicalStart:]...)
	}
	emitTurnResultNotification(l.Meta, buildTurnResultPayload(
		l.TurnID,
		l.loopThreadMeta(),
		userMessage,
		msgs,
		turnCanonicalMessages,
		l.PlanTurn,
	))
	emitStreamEndNotification(l.Meta)
}

func (l *AgentLoop) threadMetaForPersistence() (op.ThreadMeta, bool) {
	if l == nil || l.Agent == nil {
		return op.ThreadMeta{}, false
	}
	meta := l.loopThreadMeta()
	if strings.TrimSpace(meta.ThreadID) == "" || strings.TrimSpace(meta.AgentID) == "" {
		return op.ThreadMeta{}, false
	}
	if strings.TrimSpace(meta.Path) == "" {
		meta.Path = strings.TrimSpace(l.ChatPath)
	}
	if strings.TrimSpace(meta.ChatPath) == "" {
		meta.ChatPath = strings.TrimSpace(meta.Path)
	}
	if strings.TrimSpace(meta.CWD) == "" {
		meta.CWD = strings.TrimSpace(l.Workdir)
	}
	return meta, true
}

func (l *AgentLoop) nextStepSeq() int {
	if l == nil {
		return 0
	}
	seq := l.stepSeq
	if seq <= 0 {
		seq = 1
	}
	l.stepSeq = seq + 1
	return seq
}

func (l *AgentLoop) persistMessages(messages ...op.Message) error {
	if len(messages) == 0 {
		return nil
	}
	meta, ok := l.threadMetaForPersistence()
	if !ok {
		// Some unit tests construct AgentLoop without a bound session.
		return nil
	}
	return appendMessagesToThread(meta, messages)
}

func (l *AgentLoop) persistCanonicalMessages(messages ...ai.ConversationMessage) error {
	if len(messages) == 0 {
		return nil
	}
	meta, ok := l.threadMetaForPersistence()
	if !ok {
		return nil
	}
	return appendCanonicalMessagesToThread(meta, messages)
}

func (l *AgentLoop) persistPendingLoopMessages(messages []PendingLoopMessage) error {
	if len(messages) == 0 {
		return nil
	}
	meta, ok := l.threadMetaForPersistence()
	if !ok {
		return nil
	}
	if hasAnyPendingLoopQueueSource(messages) {
		return appendQueueDeliveryToThread(meta, messages)
	}
	return appendMessagesToThread(meta, pendingLoopMessagesRaw(messages))
}

func (l *AgentLoop) persistTerminalAssistantForError(runErr error) (op.Message, bool) {
	if l == nil || runErr == nil {
		return op.Message{}, false
	}
	msg := buildTerminalAssistantErrorMessage(runErr)
	if err := l.persistMessages(msg); err != nil {
		slog.Error("persist terminal assistant error", "error", err, "threadID", l.ThreadID, "turnID", l.TurnID)
		return msg, true
	}
	emitStableMessageStep(l.Meta, "assistant_step", msg, l.nextStepSeq())
	return msg, true
}

func (l *AgentLoop) opNodeResultFromMessages(msgs []op.Message) (*op.OpNodeResult, error) {
	raw, err := json.Marshal(msgs)
	if err != nil {
		return nil, fmt.Errorf("marshal messages: %w", err)
	}

	slog.Info("append messages", "messages", msgs)
	return &op.OpNodeResult{
		OpCode:  op.OpAgentLoopCreate,
		Meta:    l.Meta.Add(op.Meta{"streamEndEmitted": true}),
		Content: op.NewJsonContentRaw(raw),
	}, nil
}

func buildTerminalAssistantErrorMessage(runErr error) op.Message {
	stopReason := op.StopReasonError
	content := "Turn failed before completion."
	if errors.Is(runErr, context.Canceled) {
		stopReason = op.StopReasonAborted
		content = "Turn interrupted before completion."
	} else if runErr != nil {
		if trimmed := strings.TrimSpace(runErr.Error()); trimmed != "" {
			content = trimmed
		}
	}
	return op.Message{
		Role:       op.RoleAssistant,
		Content:    content,
		StopReason: stopReason,
	}
}

func (l *AgentLoop) appendStateMessages(messages ...op.Message) {
	if l == nil || len(messages) == 0 {
		return
	}
	l.canonicalHistory = append(l.canonicalHistory, ai.CanonicalMessagesFromOp(messages)...)
}

func (l *AgentLoop) appendCanonicalStateMessages(messages ...ai.ConversationMessage) {
	if l == nil || len(messages) == 0 {
		return
	}
	l.canonicalHistory = append(l.canonicalHistory, messages...)
}

func (l *AgentLoop) applyPendingLoopMessageState(pending PendingLoopMessage) {
	if l == nil {
		return
	}
	if !hasPendingLoopQueueSource(pending) && len(pending.SelectedSkillIDs) == 0 && len(pending.SelectedSkillContext) == 0 && !pending.PlanTurn {
		return
	}
	l.SelectedSkillIDs = append([]string(nil), pending.SelectedSkillIDs...)
	l.SelectedSkillContext = pending.SelectedSkillContext.Clone()
	l.PlanTurn = pending.PlanTurn
	l.Meta["planTurn"] = pending.PlanTurn
}

func (l *AgentLoop) replaceStateMessages(messages []ai.ConversationMessage) {
	if l == nil {
		return
	}
	l.canonicalHistory = cloneCanonicalMessages(messages)
}

type ContinueLoopConfig struct {
	PendingMessages         []op.Message
	SkipInitialSteeringPoll bool
}

type assistantTurnResult struct {
	message   op.Message
	canonical ai.ConversationMessage
}

// agentLoopContinue continues an agent loop from the current context without adding a new message.
// Aligns with pi-mono assistant-tail resume semantics:
// - if the current context ends in a non-assistant message, continue directly
// - if it ends in an assistant message, the caller must provide explicit pending messages to inject first
func (a *AgentLoop) agentLoopContinue(cfg *ContinueLoopConfig) (*Loop, error) {
	if len(a.canonicalHistory) == 0 {
		return nil, fmt.Errorf("cannot continue: no messages in context")
	}

	last, ok := lastCanonicalAsOpMessage(a.canonicalHistory)
	if !ok {
		return nil, fmt.Errorf("cannot continue: no replayable messages in context")
	}
	var runCfg *RunLoopConfig
	if last.Role == op.RoleAssistant {
		if cfg == nil || len(cfg.PendingMessages) == 0 {
			return nil, fmt.Errorf("cannot continue from message role: assistant")
		}
		runCfg = &RunLoopConfig{
			InitialPendingMessages:  pendingLoopMessagesFromMessages(cfg.PendingMessages),
			SkipInitialSteeringPoll: cfg.SkipInitialSteeringPoll,
		}
	}
	canonicalStart := len(a.canonicalHistory)
	msgs, err := a.runLoop(runCfg)
	if err != nil {
		return nil, err
	}
	if len(msgs) > 0 {
		var emptyUser op.Message
		turnCanonicalMessages := append([]ai.ConversationMessage(nil), a.canonicalHistory[canonicalStart:]...)
		emitTurnResultNotification(a.Meta, buildTurnResultPayload(
			a.TurnID,
			a.loopThreadMeta(),
			emptyUser,
			msgs,
			turnCanonicalMessages,
			a.PlanTurn,
		))
		emitStreamEndNotification(a.Meta)
	}
	return &Loop{Ctx: a.Ctx, Meta: a.Meta, ThreadID: a.ThreadID, TurnID: a.TurnID, Workdir: a.Workdir}, nil
}

func (a *AgentLoop) runContinuation(cfg *ContinueLoopConfig) (*op.OpNodeResult, error) {
	if len(a.canonicalHistory) == 0 {
		err := fmt.Errorf("cannot continue: no messages in context")
		if terminal, ok := a.persistTerminalAssistantForError(err); ok {
			a.emitTurnResultAndStreamEnd(op.Message{}, []op.Message{terminal}, len(a.canonicalHistory))
		}
		return nil, err
	}

	last, ok := lastCanonicalAsOpMessage(a.canonicalHistory)
	if !ok {
		err := fmt.Errorf("cannot continue: no replayable messages in context")
		if terminal, terminalOK := a.persistTerminalAssistantForError(err); terminalOK {
			a.emitTurnResultAndStreamEnd(op.Message{}, []op.Message{terminal}, len(a.canonicalHistory))
		}
		return nil, err
	}

	runtime := newRuntimeLoop(a.ThreadID, a.ChatPath, a.Cancel)
	if err := registerRuntimeLoop(runtime); err != nil {
		if terminal, ok := a.persistTerminalAssistantForError(err); ok {
			a.emitTurnResultAndStreamEnd(op.Message{}, []op.Message{terminal}, len(a.canonicalHistory))
		}
		return nil, err
	}
	defer unregisterRuntimeLoop(a.ThreadID, runtime)
	var runCfg *RunLoopConfig
	if last.Role == op.RoleAssistant {
		if cfg != nil && len(cfg.PendingMessages) > 0 {
			runCfg = a.runtimeQueueRunConfig(
				pendingLoopMessagesFromMessages(cfg.PendingMessages),
				cfg.SkipInitialSteeringPoll,
			)
		} else {
			initialPending, queueErr := peekNextQueuedPendingForLoop(a.loopThreadMeta())
			if queueErr != nil {
				if terminal, ok := a.persistTerminalAssistantForError(queueErr); ok {
					a.emitTurnResultAndStreamEnd(op.Message{}, []op.Message{terminal}, len(a.canonicalHistory))
				}
				return nil, queueErr
			}
			if len(initialPending) == 0 {
				err := fmt.Errorf("cannot continue from message role: assistant")
				if terminal, ok := a.persistTerminalAssistantForError(err); ok {
					a.emitTurnResultAndStreamEnd(op.Message{}, []op.Message{terminal}, len(a.canonicalHistory))
				}
				return nil, err
			}
			runCfg = a.runtimeQueueRunConfig(initialPending, true)
		}
	} else {
		runCfg = a.runtimeQueueRunConfig(nil, false)
	}

	canonicalStart := len(a.canonicalHistory)
	msgs, err := a.runLoop(runCfg)
	if err != nil {
		if terminal, ok := a.persistTerminalAssistantForError(err); ok {
			a.emitTurnResultAndStreamEnd(op.Message{}, append(append([]op.Message(nil), msgs...), terminal), canonicalStart)
		}
		return nil, err
	}
	if len(msgs) == 0 {
		err := fmt.Errorf("no messages returned from runLoop")
		if terminal, ok := a.persistTerminalAssistantForError(err); ok {
			a.emitTurnResultAndStreamEnd(op.Message{}, []op.Message{terminal}, canonicalStart)
		}
		return nil, err
	}
	a.emitTurnResultAndStreamEnd(op.Message{}, msgs, canonicalStart)
	return a.opNodeResultFromMessages(msgs)
}

// RunLoopConfig holds optional callbacks for steering and follow-up messages (pi-mono style).
// If a callback is nil, no messages are injected.
type RunLoopConfig struct {
	GetSteeringMessages     func(context.Context) ([]PendingLoopMessage, error)
	GetFollowUpMessages     func(context.Context) ([]PendingLoopMessage, error)
	InitialPendingMessages  []PendingLoopMessage
	SkipInitialSteeringPoll bool
}

// runLoop implements the same double-loop structure as pi-mono's runLoop:
// - Outer loop: each iteration is one "round"; continues when follow-up messages arrive after the agent would stop.
// - Inner loop: hasMoreToolCalls || len(pendingMessages) > 0; per-round state (hasMoreToolCalls, steeringAfterTools) is reset at the start of each outer iteration.
func (l *AgentLoop) runLoop(cfg *RunLoopConfig) (newMessages []op.Message, err error) {
	ctx := l.Ctx
	newMessages = make([]op.Message, 0)
	prevSteeringGetter := l.getSteeringMessages
	defer func() {
		l.getSteeringMessages = prevSteeringGetter
	}()
	if cfg != nil {
		l.getSteeringMessages = cfg.GetSteeringMessages
	} else {
		l.getSteeringMessages = nil
	}

	getSteering := func() ([]PendingLoopMessage, error) {
		if cfg != nil && cfg.GetSteeringMessages != nil {
			return cfg.GetSteeringMessages(ctx)
		}
		return nil, nil
	}
	getFollowUp := func() ([]PendingLoopMessage, error) {
		if cfg != nil && cfg.GetFollowUpMessages != nil {
			return cfg.GetFollowUpMessages(ctx)
		}
		return nil, nil
	}

	pendingMessages := append([]PendingLoopMessage(nil), cfgOrEmptyPendingMessages(cfg)...)
	if len(pendingMessages) == 0 && !cfgSkipInitialSteeringPoll(cfg) {
		pendingMessages, _ = getSteering()
	}

	for {
		hasMoreToolCalls := true
		var steeringAfterTools []PendingLoopMessage

		for hasMoreToolCalls || len(pendingMessages) > 0 {
			if len(pendingMessages) > 0 {
				rawPending := pendingLoopMessagesRaw(pendingMessages)
				for _, msg := range rawPending {
					newMessages = append(newMessages, msg)
				}
				for _, pending := range pendingMessages {
					l.applyPendingLoopMessageState(pending)
				}
				if err := l.persistPendingLoopMessages(pendingMessages); err != nil {
					return newMessages, err
				}
				l.appendStateMessages(rawPending...)
				for _, pending := range pendingMessages {
					if hasPendingLoopQueueSource(pending) {
						emitQueuedMessageDelivered(l.Meta, string(pending.QueueKind), pending.Message, pending.QueueItemID)
					}
					emitStableMessageStep(l.Meta, "user_step", pending.Message, l.nextStepSeq())
				}
				pendingMessages = nil
			}

			if err := l.maybeCompact(); err != nil {
				return newMessages, err
			}

			assistantResult, stopErr := l.streamAssistantTurnResultWithRetry()
			if stopErr != nil {
				return newMessages, stopErr
			}

			msg := assistantResult.message
			newMessages = append(newMessages, msg)
			l.appendCanonicalStateMessages(assistantResult.canonical)
			if err := l.persistCanonicalMessages(assistantResult.canonical); err != nil {
				return newMessages, err
			}
			emitStableMessageStep(l.Meta, "assistant_step", msg, l.nextStepSeq())

			toolCalls := msg.ToolCalls
			hasMoreToolCalls = len(toolCalls) > 0

			if hasMoreToolCalls {
				loop := Loop{Ctx: l.Ctx, Meta: l.Meta, ThreadID: l.ThreadID, TurnID: l.TurnID, Workdir: l.Workdir}
				toolResults, steering, execErr := l.executeToolCalls(loop, msg)
				if execErr != nil {
					return newMessages, execErr
				}
				steeringAfterTools = steering
				for _, tr := range toolResults {
					newMessages = append(newMessages, tr)
				}
				l.appendStateMessages(toolResults...)
				if err := l.persistMessages(toolResults...); err != nil {
					return newMessages, err
				}
			}

			if len(steeringAfterTools) > 0 {
				pendingMessages = steeringAfterTools
				steeringAfterTools = nil
			} else {
				pendingMessages, _ = getSteering()
			}
		}

		followUp, _ := getFollowUp()
		if len(followUp) > 0 {
			pendingMessages = followUp
			continue
		}
		break
	}

	return newMessages, nil
}

// estimateContextTokens returns the estimated token count for the full LLM
// context using hybrid estimation (pi-mono pattern):
// - If any assistant has real API usage, use it + estimate trailing messages.
// - Otherwise fall back to chars/4 estimate + system prompt estimate.
//
// When API usage is available, the system prompt is already included in the
// reported token count, so we don't double-count it.
func (l *AgentLoop) estimateContextTokens() int64 {
	return estimateCanonicalContextTokens(l.canonicalHistory, l.currentSystemPrompt())
}

func (l *AgentLoop) currentSystemPrompt() string {
	if l == nil || l.Agent == nil {
		return ""
	}
	agentID := strings.TrimSpace(l.Agent.AgentID)
	if l.Agent.PromptIsFinal {
		return appendGBrainQueryScopePrompt(l.Agent.Sysprompt, agentID, l.Meta)
	}
	selectedSkills, _ := resolveSkillNodes(l.SelectedSkillIDs)
	return appendGBrainQueryScopePrompt(
		buildAgentSystemPrompt(l.Agent.Sysprompt, l.Agent.AvailableSkills, selectedSkills, l.SelectedSkillContext),
		agentID,
		l.Meta,
	)
}

func (l *AgentLoop) effectiveContextWindow() int64 {
	if l == nil {
		return 0
	}
	if l.ContextWindow > 0 {
		return l.ContextWindow
	}
	if l.Model == nil || l.Model.config == nil {
		return 0
	}
	return l.Model.config.ContextWindow
}

func (l *AgentLoop) maybeCompact() error {
	if l == nil || l.Model == nil || l.Model.config == nil {
		return nil
	}

	cfg := config.GetConfig()
	if cfg == nil {
		return nil
	}

	enabled, reserveTokens, keepRecentTokens := compaction.ResolveSettings(cfg.Compaction)
	if !enabled {
		return nil
	}

	contextWindow := l.effectiveContextWindow()
	estimatedTokens := l.estimateContextTokens()
	if !compaction.ShouldCompact(estimatedTokens, contextWindow, reserveTokens) {
		return nil
	}

	return l.compactWithSettings(cfg.Compaction, keepRecentTokens)
}

func (l *AgentLoop) forceCompact() error {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil
	}
	_, _, keepRecentTokens := compaction.ResolveSettings(cfg.Compaction)
	return l.compactWithSettings(cfg.Compaction, keepRecentTokens)
}

func (l *AgentLoop) compactWithSettings(compactionCfg op.CompactionConfig, keepRecentTokens int64) error {
	msgsBefore := len(l.canonicalHistory)
	tokensBefore := l.estimateContextTokens()

	summarize := func(ctx context.Context, conversation string) (string, error) {
		model := l.Model
		targetModelID := strings.TrimSpace(compactionCfg.ModelID)
		if targetModelID != "" && (model == nil || model.config == nil || model.config.ID != targetModelID) {
			compactionModel, err := NewModelClient(ctx, targetModelID, l.Meta)
			if err != nil {
				slog.Warn("failed to load compaction model, fallback to active model", "modelID", targetModelID, "error", err)
			} else {
				model = compactionModel
			}
		}
		return l.generateCompactionSummary(ctx, conversation, model)
	}

	compacted, err := compactCanonicalMessages(l.Ctx, l.canonicalHistory, keepRecentTokens, summarize)
	if err != nil {
		return err
	}
	l.replaceStateMessages(compacted)
	if len(compacted) > 0 && isCanonicalContextCheckpoint(compacted[0]) {
		if err := replaceThreadCanonicalMessagesWithCompaction(l.loopThreadMeta(), compacted, tokensBefore); err != nil {
			return err
		}
	}

	tokensAfter := l.estimateContextTokens()
	slog.Info("context compacted",
		"msgsBefore", msgsBefore,
		"msgsAfter", len(l.canonicalHistory),
		"tokensBefore", tokensBefore,
		"tokensAfter", tokensAfter,
	)
	return nil
}

func (l *AgentLoop) generateCompactionSummary(ctx context.Context, conversation string, model *ModelClient) (string, error) {
	if model == nil || model.config == nil {
		return "", fmt.Errorf("compaction model is not initialized")
	}
	if strings.TrimSpace(conversation) == "" {
		return "", nil
	}

	modelName := strings.TrimSpace(model.config.Name)
	if modelName == "" {
		return "", fmt.Errorf("compaction model name is empty")
	}
	canonical := model.canonicalProvider()
	if canonical == nil {
		return "", fmt.Errorf("compaction model provider is not initialized")
	}
	resp, err := canonical.CompleteCanonical(ctx, &ai.ProviderRequest{
		Context: ai.ConversationContext{
			SystemPrompt: compaction.SummarizationSystemPrompt,
			Messages: []ai.ConversationMessage{{
				Role: ai.RoleCanonicalUser,
				Content: []ai.ContentBlock{{
					Type: ai.BlockText,
					Text: conversation,
				}},
			}},
		},
		Config: ai.GenerationConfig{
			Model:       modelName,
			ServiceTier: serviceTierForModelMeta(model.config, l.Meta),
		},
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("empty compaction summary response")
	}
	msg, err := ai.OpMessageFromCanonical(resp.Message)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(msg.Content), nil
}

// streamAssistantResponse produces one assistant message (and runs LLM). Placeholder: caller should integrate real LLM stream.
func (l *AgentLoop) streamAssistantResponse() (op.Message, error) {
	result, err := l.streamAssistantTurnResultWithRetry()
	if err != nil {
		return op.Message{}, err
	}
	return result.message, nil
}

func (l *AgentLoop) streamAssistantTurnResult() (assistantTurnResult, error) {
	if l.Model == nil || l.Model.config == nil {
		return assistantTurnResult{}, fmt.Errorf("model client not initialized")
	}
	modelName := strings.TrimSpace(l.Model.config.Name)
	if modelName == "" {
		return assistantTurnResult{}, fmt.Errorf("model name is empty")
	}
	sysPrompt := strings.TrimSpace(l.currentSystemPrompt())
	canonical := l.Model.canonicalProvider()
	if canonical == nil {
		return assistantTurnResult{}, fmt.Errorf("model provider does not support canonical requests")
	}
	history := l.canonicalHistory

	tools := collectToolSpecs(l.Agent.ToolSpecs)
	req := &ai.ProviderRequest{
		Context: ai.ConversationContext{
			SystemPrompt: sysPrompt,
			Messages:     append([]ai.ConversationMessage(nil), history...),
			Tools:        ai.CanonicalToolsFromOp(tools),
		},
		Config: ai.GenerationConfig{
			Model:          modelName,
			ServiceTier:    serviceTierForModelMeta(l.Model.config, l.Meta),
			PromptCacheKey: loopPromptCacheKey(l),
		},
		RequestID: strings.TrimSpace(metaString(l.Meta, "turnRequestID")),
	}
	if len(tools) > 0 {
		rawChoice, _ := json.Marshal("auto")
		req.Config.ToolChoice = rawChoice
	}
	if effort := resolveProviderReasoningEffort(l.ThinkingLevel, l.Model.config); effort != "" {
		req.Config.ReasoningEffort = effort
		if l.Model != nil && l.Model.config != nil && strings.TrimSpace(l.Model.config.API) == "openai-responses" {
			req.Config.ReasoningSummary = "auto"
			req.Config.Include = append(req.Config.Include, "reasoning.encrypted_content")
		}
	}
	if enabled := resolveProviderReasoningEnabled(l.ThinkingLevel, l.Model.config); enabled != nil {
		req.Config.ReasoningEnabled = enabled
	}
	buildAssistantMessage := func(resp *ai.ProviderResponse) (assistantTurnResult, error) {
		msg, err := ai.OpMessageFromCanonical(resp.Message)
		if err != nil {
			return assistantTurnResult{}, err
		}
		msg.StopReason = op.MessageStopReason(resp.StopReason)
		usage := &op.MessageUsage{
			InputTokens:      resp.Usage.InputTokens,
			OutputTokens:     resp.Usage.OutputTokens,
			CacheReadTokens:  resp.Usage.CacheReadTokens,
			CacheWriteTokens: resp.Usage.CacheWriteTokens,
			TotalTokens:      resp.Usage.ResolvedTotalTokens(),
		}
		canonicalMessage := *ai.CloneConversationMessagePtr(&resp.Message)
		canonicalMessage.Usage = usage
		canonicalMessage.StopReason = resp.StopReason
		msg.Usage = usage
		l.emitLoopTokenUsage(usage, canonicalMessage)
		return assistantTurnResult{
			message:   msg,
			canonical: canonicalMessage,
		}, nil
	}

	var final *ai.ProviderResponse
	var lastPartial *ai.StreamConversationMessage
	stream, err := canonical.StreamCanonical(l.Ctx, req)
	if err != nil {
		if !errors.Is(err, ai.ErrStreamingNotSupported) {
			return assistantTurnResult{}, err
		}
		final, err = canonical.CompleteCanonical(l.Ctx, req)
		if err != nil {
			return assistantTurnResult{}, err
		}
		emitCanonicalMessageReplay(l.Meta, final)
	} else {
		defer stream.Close()
		for stream.Next() {
			event := stream.Event()
			if event.Partial != nil {
				lastPartial = ai.CloneStreamConversationMessagePtr(event.Partial)
			}
			emitCanonicalLifecycleEvent(l.Meta, event)
			if event.Type == ai.EventCanonicalDone {
				final = event.Response
			}
		}
		if err := stream.Err(); err != nil {
			stopReason := ai.StopReasonError
			// Providers (e.g. the gateway websocket provider) may surface
			// context cancellation as a wrapped transport error that no longer
			// satisfies errors.Is(err, context.Canceled). Treat the turn context
			// itself as the source of truth for the aborted semantic.
			if errors.Is(err, context.Canceled) || errors.Is(l.Ctx.Err(), context.Canceled) {
				stopReason = ai.StopReasonAborted
			}
			if fallback, ok := providerResponseFromPartialTerminal(lastPartial, stopReason, err); ok {
				return buildAssistantMessage(fallback)
			}
			return assistantTurnResult{}, fmt.Errorf("stream error: %w", err)
		}
	}
	final = mergeMissingFinalTextFromPartial(final, lastPartial)
	if !ai.HasSemanticCanonicalResponse(final) {
		if fallback, ok := providerResponseFromPartialTerminal(lastPartial, ai.StopReasonStop, nil); ok {
			return buildAssistantMessage(fallback)
		}
	}
	if final == nil {
		return assistantTurnResult{}, fmt.Errorf("empty model response")
	}
	return buildAssistantMessage(final)
}

func mergeMissingFinalTextFromPartial(final *ai.ProviderResponse, partial *ai.StreamConversationMessage) *ai.ProviderResponse {
	if final == nil || partial == nil || canonicalMessageHasVisibleText(final.Message) {
		return final
	}
	partialFinal := ai.FinalizeStreamConversationMessage(partial)
	textBlocks := make([]ai.ContentBlock, 0, len(partialFinal.Content))
	for _, block := range partialFinal.Content {
		if block.Type != ai.BlockText || strings.TrimSpace(block.Text) == "" {
			continue
		}
		cloned := ai.CloneContentBlockPtr(&block)
		if cloned != nil {
			textBlocks = append(textBlocks, *cloned)
		}
	}
	if len(textBlocks) == 0 {
		return final
	}
	merged := ai.CloneProviderResponsePtr(final)
	if merged == nil {
		return final
	}
	if merged.Message.Role == "" {
		merged.Message.Role = partialFinal.Role
		if merged.Message.Role == "" {
			merged.Message.Role = ai.RoleCanonicalAssistant
		}
	}
	if merged.Message.Timestamp == 0 {
		merged.Message.Timestamp = partialFinal.Timestamp
	}
	if merged.Message.ProviderState == nil && partialFinal.ProviderState != nil {
		providerState := *partialFinal.ProviderState
		merged.Message.ProviderState = &providerState
	}
	if merged.StopReason == "" {
		if merged.Message.StopReason != "" {
			merged.StopReason = ai.StopReason(merged.Message.StopReason)
		} else {
			merged.StopReason = ai.StopReasonStop
		}
	}
	insertAt := len(merged.Message.Content)
	for index, block := range merged.Message.Content {
		if block.Type == ai.BlockToolCall {
			insertAt = index
			break
		}
	}
	content := make([]ai.ContentBlock, 0, len(merged.Message.Content)+len(textBlocks))
	content = append(content, merged.Message.Content[:insertAt]...)
	content = append(content, textBlocks...)
	content = append(content, merged.Message.Content[insertAt:]...)
	merged.Message.Content = content
	return merged
}

func canonicalMessageHasVisibleText(msg ai.ConversationMessage) bool {
	for _, block := range msg.Content {
		if block.Type == ai.BlockText && strings.TrimSpace(block.Text) != "" {
			return true
		}
	}
	return false
}

func providerResponseFromPartialTerminal(partial *ai.StreamConversationMessage, stopReason ai.StopReason, streamErr error) (*ai.ProviderResponse, bool) {
	if partial == nil {
		return nil, false
	}
	content := make([]ai.ContentBlock, 0, len(partial.Content))
	includeThinking := stopReason == ai.StopReasonError || stopReason == ai.StopReasonAborted
	for _, block := range partial.Content {
		switch block.Type {
		case ai.BlockText:
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
			finalBlock := ai.ContentBlock{
				Type:          ai.BlockText,
				Text:          block.Text,
				TextSignature: block.TextSignature,
			}
			if len(block.Raw) > 0 {
				finalBlock.Raw = append(json.RawMessage(nil), block.Raw...)
			}
			content = append(content, finalBlock)
		case ai.BlockThinking:
			if !includeThinking || !streamThinkingBlockHasContent(block) {
				continue
			}
			finalBlock := ai.ContentBlock{
				Type:                ai.BlockThinking,
				Text:                block.Text,
				ThinkingReplayField: block.ThinkingReplayField,
				ThinkingSignature:   block.ThinkingSignature,
				EncryptedContent:    block.EncryptedContent,
			}
			if len(block.Raw) > 0 {
				finalBlock.Raw = append(json.RawMessage(nil), block.Raw...)
			}
			content = append(content, finalBlock)
		default:
			continue
		}
	}
	if len(content) == 0 {
		return nil, false
	}
	message := ai.ConversationMessage{
		Role:       ai.RoleCanonicalAssistant,
		Content:    content,
		Timestamp:  partial.Timestamp,
		StopReason: stopReason,
	}
	if partial.ProviderState != nil {
		providerState := *partial.ProviderState
		message.ProviderState = &providerState
	}
	message.Raw = partialTerminalRaw(partial.Raw, streamErr)
	return &ai.ProviderResponse{
		Message:    message,
		StopReason: stopReason,
	}, true
}

func streamThinkingBlockHasContent(block ai.StreamContentBlock) bool {
	return strings.TrimSpace(block.Text) != "" ||
		strings.TrimSpace(block.ThinkingReplayField) != "" ||
		strings.TrimSpace(block.ThinkingSignature) != "" ||
		strings.TrimSpace(block.EncryptedContent) != "" ||
		len(block.Raw) > 0
}

func partialTerminalRaw(partialRaw json.RawMessage, streamErr error) json.RawMessage {
	if streamErr == nil {
		if len(partialRaw) == 0 {
			return nil
		}
		return append(json.RawMessage(nil), partialRaw...)
	}
	errorMessage := strings.TrimSpace(streamErr.Error())
	if errorMessage == "" {
		return nil
	}
	payload := map[string]any{
		"errorMessage": errorMessage,
	}
	if len(partialRaw) > 0 && json.Valid(partialRaw) {
		payload["partialRaw"] = append(json.RawMessage(nil), partialRaw...)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return raw
}

func cloneCanonicalMessages(messages []ai.ConversationMessage) []ai.ConversationMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]ai.ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		cloned := ai.CloneConversationMessagePtr(&msg)
		if cloned == nil {
			continue
		}
		out = append(out, *cloned)
	}
	return out
}

func lastCanonicalAsOpMessage(messages []ai.ConversationMessage) (op.Message, bool) {
	if len(messages) == 0 {
		return op.Message{}, false
	}
	for i := len(messages) - 1; i >= 0; i-- {
		converted, err := ai.OpMessageFromCanonical(messages[i])
		if err != nil || converted.Role == "" {
			continue
		}
		return converted, true
	}
	return op.Message{}, false
}

func estimateCanonicalMessageTokens(msg ai.ConversationMessage) int64 {
	converted, err := ai.OpMessageFromCanonical(msg)
	if err == nil && converted.Role != "" {
		return op.EstimateMessageTokens(converted)
	}
	var chars int
	for _, block := range msg.Content {
		chars += len(block.Text) + len(block.TextSignature) + len(block.ThinkingSignature) + len(block.EncryptedContent)
		if block.ToolCall != nil {
			chars += len(block.ToolCall.ID) + len(block.ToolCall.Name) + len(block.ToolCall.RawArguments)
		}
		if block.ToolResult != nil {
			chars += len(block.ToolResult.ToolCallID) + len(block.ToolResult.ToolName) + len(block.ToolResult.OutputText)
		}
	}
	if chars <= 0 {
		return 0
	}
	return int64((chars + 3) / 4)
}

func estimateCanonicalContextTokens(messages []ai.ConversationMessage, systemPrompt string) int64 {
	lastUsageIndex := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if _, ok := validCanonicalAssistantUsage(messages[i]); ok {
			lastUsageIndex = i
			break
		}
	}

	if lastUsageIndex < 0 {
		var total int64
		for _, msg := range messages {
			total += estimateCanonicalMessageTokens(msg)
		}
		if sysPrompt := strings.TrimSpace(systemPrompt); sysPrompt != "" {
			total += int64(len(sysPrompt)+3) / 4
		}
		return total
	}

	total := resolveMessageUsageTotal(messages[lastUsageIndex].Usage)
	for i := lastUsageIndex + 1; i < len(messages); i++ {
		total += estimateCanonicalMessageTokens(messages[i])
	}
	return total
}

func compactCanonicalMessages(
	ctx context.Context,
	messages []ai.ConversationMessage,
	keepRecentTokens int64,
	summarize func(context.Context, string) (string, error),
) ([]ai.ConversationMessage, error) {
	if len(messages) == 0 || summarize == nil {
		return cloneCanonicalMessages(messages), nil
	}

	cutIdx := findCanonicalCutPoint(messages, keepRecentTokens)
	if cutIdx <= 0 || cutIdx >= len(messages) {
		return cloneCanonicalMessages(messages), nil
	}

	conversation := buildCanonicalSummarizationInput(pruneCanonicalMessagesForSummary(messages[:cutIdx]))
	summary, err := summarize(ctx, conversation)
	if err != nil {
		slog.Warn("compaction summarization failed, using fallback", "error", err)
		summary = "Earlier conversation history was truncated because compaction summary is unavailable."
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "Earlier conversation history was truncated because compaction summary is unavailable."
	}

	tail := cloneCanonicalMessages(messages[cutIdx:])
	clearCanonicalAssistantUsage(tail)

	out := make([]ai.ConversationMessage, 0, len(messages)-cutIdx+1)
	out = append(out, ai.ConversationMessage{
		Role: ai.RoleCanonicalSystem,
		Content: []ai.ContentBlock{{
			Type: ai.BlockText,
			Text: "Context checkpoint summary:\n" + summary,
		}},
	})
	out = append(out, tail...)
	return out, nil
}

func clearCanonicalAssistantUsage(messages []ai.ConversationMessage) {
	for i := range messages {
		if messages[i].Role != ai.RoleCanonicalAssistant {
			continue
		}
		messages[i].Usage = nil
	}
}

func findCanonicalCutPoint(messages []ai.ConversationMessage, keepRecentTokens int64) int {
	if len(messages) <= 1 || keepRecentTokens <= 0 {
		return -1
	}
	var accumulated int64
	start := -1
	for i := len(messages) - 1; i >= 0; i-- {
		accumulated += estimateCanonicalMessageTokens(messages[i])
		if accumulated >= keepRecentTokens {
			start = i
			break
		}
	}
	if start < 0 {
		return -1
	}
	for i := start; i < len(messages); i++ {
		if messages[i].Role == ai.RoleCanonicalUser {
			if i == 0 {
				return -1
			}
			return i
		}
	}
	for i := start; i < len(messages); i++ {
		if messages[i].Role == ai.RoleCanonicalAssistant {
			if i == 0 {
				return -1
			}
			return i
		}
	}
	return -1
}

func pruneCanonicalMessagesForSummary(messages []ai.ConversationMessage) []ai.ConversationMessage {
	out := cloneCanonicalMessages(messages)
	for i := range out {
		if out[i].Role != ai.RoleCanonicalTool {
			continue
		}
		for j := range out[i].Content {
			block := &out[i].Content[j]
			if block.Type == ai.BlockToolResult && block.ToolResult != nil && (block.ToolResult.OutputText != "" || len(block.ToolResult.OutputContent) > 0) {
				block.ToolResult.OutputText = "[Old tool result content cleared]"
				block.ToolResult.OutputContent = nil
			}
		}
	}
	return out
}

func buildCanonicalSummarizationInput(messages []ai.ConversationMessage) string {
	conversation := strings.TrimSpace(serializeCanonicalMessagesForSummary(messages))
	var b strings.Builder
	b.WriteString("<conversation>\n")
	b.WriteString(conversation)
	b.WriteString("\n</conversation>\n\n")
	b.WriteString(compaction.SummarizationPrompt)
	return b.String()
}

func serializeCanonicalMessagesForSummary(messages []ai.ConversationMessage) string {
	var b strings.Builder
	for _, msg := range messages {
		switch msg.Role {
		case ai.RoleCanonicalSystem:
			b.WriteString("[System]: ")
			b.WriteString(canonicalSummaryText(msg))
			b.WriteString("\n\n")
		case ai.RoleCanonicalDeveloper:
			b.WriteString("[Developer]: ")
			b.WriteString(canonicalSummaryText(msg))
			b.WriteString("\n\n")
		case ai.RoleCanonicalUser:
			b.WriteString("[User]: ")
			b.WriteString(canonicalSummaryText(msg))
			b.WriteString("\n\n")
		case ai.RoleCanonicalAssistant:
			if text := canonicalSummaryText(msg); text != "" {
				b.WriteString("[Assistant]: ")
				b.WriteString(text)
				b.WriteString("\n\n")
			}
			toolCalls := canonicalToolCallsSummary(msg)
			if len(toolCalls) > 0 {
				b.WriteString("[Assistant Tool Calls]: ")
				b.WriteString(strings.Join(toolCalls, "; "))
				b.WriteString("\n\n")
			}
		case ai.RoleCanonicalTool:
			b.WriteString("[Tool Result")
			if name := canonicalToolResultName(msg); name != "" {
				b.WriteString(": ")
				b.WriteString(name)
			}
			b.WriteString("]: ")
			b.WriteString(canonicalSummaryText(msg))
			b.WriteString("\n\n")
		default:
			raw, _ := json.Marshal(msg)
			b.WriteString("[Message]: ")
			b.Write(raw)
			b.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func canonicalSummaryText(msg ai.ConversationMessage) string {
	parts := make([]string, 0, len(msg.Content))
	for _, block := range msg.Content {
		switch block.Type {
		case ai.BlockText, ai.BlockThinking, ai.BlockCompaction:
			if text := strings.TrimSpace(block.Text); text != "" {
				parts = append(parts, text)
			}
		case ai.BlockImage:
			if strings.TrimSpace(block.ImageData) != "" {
				parts = append(parts, "[Image]")
			}
		case ai.BlockToolResult:
			if block.ToolResult != nil && strings.TrimSpace(block.ToolResult.OutputText) != "" {
				parts = append(parts, strings.TrimSpace(block.ToolResult.OutputText))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func canonicalToolCallsSummary(msg ai.ConversationMessage) []string {
	parts := make([]string, 0, len(msg.Content))
	for _, block := range msg.Content {
		if block.Type != ai.BlockToolCall || block.ToolCall == nil {
			continue
		}
		call := strings.TrimSpace(block.ToolCall.Name)
		args := strings.TrimSpace(block.ToolCall.RawArguments)
		if args == "" && len(block.ToolCall.Arguments) > 0 {
			args = ai.MarshalToolArgumentsJSON(block.ToolCall.Arguments)
		}
		if args != "" {
			call += "(" + args + ")"
		}
		if call != "" {
			parts = append(parts, call)
		}
	}
	return parts
}

func canonicalToolResultName(msg ai.ConversationMessage) string {
	for _, block := range msg.Content {
		if block.Type == ai.BlockToolResult && block.ToolResult != nil {
			return strings.TrimSpace(block.ToolResult.ToolName)
		}
	}
	return ""
}

func resolveMessageUsageTotal(usage *op.MessageUsage) int64 {
	if usage == nil {
		return 0
	}
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.InputTokens + usage.OutputTokens + usage.CacheReadTokens + usage.CacheWriteTokens
}

func (l *AgentLoop) contextUsageAfter(additional ...ai.ConversationMessage) ai.ThreadContextUsage {
	contextWindow := l.effectiveContextWindow()
	if contextWindow <= 0 {
		return ai.ThreadContextUsage{}
	}
	messages := append([]ai.ConversationMessage(nil), l.canonicalHistory...)
	messages = append(messages, additional...)
	return buildCanonicalContextUsage(messages, l.currentSystemPrompt(), contextWindow)
}

func (l *AgentLoop) emitLoopTokenUsage(usage *op.MessageUsage, additional ...ai.ConversationMessage) {
	if usage == nil || (usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0) {
		return
	}
	loopTotalTokens := resolveMessageUsageTotal(usage)
	eventMeta := op.Meta{
		"type":             "tokenUsage",
		"loopInputTokens":  fmt.Sprintf("%d", usage.InputTokens),
		"loopOutputTokens": fmt.Sprintf("%d", usage.OutputTokens),
		"loopTotalTokens":  fmt.Sprintf("%d", loopTotalTokens),
	}
	contextUsage := l.contextUsageAfter(additional...)
	if contextUsage.ContextWindow > 0 {
		eventMeta["contextWindow"] = fmt.Sprintf("%d", contextUsage.ContextWindow)
		eventMeta["contextKnown"] = fmt.Sprintf("%t", contextUsage.Known)
		if contextUsage.Known && contextUsage.Tokens > 0 {
			eventMeta["contextTokens"] = fmt.Sprintf("%d", contextUsage.Tokens)
			eventMeta["contextPercentMilli"] = fmt.Sprintf("%d", contextUsage.PercentMilli)
		}
	}
	NotifyProgress(op.NotifyMessage, l.Meta.Add(eventMeta), &op.TextContent{Text: ""})
}

func emitCanonicalLifecycleEvent(meta op.Meta, event ai.ProviderEvent) {
	eventMeta := meta.Add(op.Meta{
		"type": string(event.Type),
	})
	if event.ContentIndex >= 0 {
		eventMeta["contentIndex"] = event.ContentIndex
	}
	if event.Partial != nil && event.Partial.ProviderState != nil {
		if model := strings.TrimSpace(event.Partial.ProviderState.Model); model != "" {
			eventMeta["model"] = model
		}
	}
	if event.Block != nil && event.Block.ToolCall != nil {
		if id := strings.TrimSpace(event.Block.ToolCall.ID); id != "" {
			eventMeta["id"] = id
		}
		if name := strings.TrimSpace(event.Block.ToolCall.Name); name != "" {
			eventMeta["name"] = normalizeToolName(name)
		}
	}
	if event.Type == ai.EventCanonicalToolCallStart ||
		event.Type == ai.EventCanonicalToolCallDelta ||
		event.Type == ai.EventCanonicalToolCallEnd {
		payload := canonicalToolCallProgressPayload(event)
		raw, err := json.Marshal(payload)
		if err != nil {
			slog.Warn("marshal canonical tool call progress", "error", err)
			NotifyProgress(op.NotifyMessage, eventMeta, &op.TextContent{Text: event.Delta})
			return
		}
		NotifyProgress(op.NotifyMessage, eventMeta, &op.JsonContent{Raw: raw})
		return
	}
	text := ""
	switch event.Type {
	case ai.EventCanonicalTextDelta, ai.EventCanonicalThinkingDelta:
		text = event.Delta
	case ai.EventCanonicalTextEnd, ai.EventCanonicalThinkingEnd:
		text = event.Content
	}
	NotifyProgress(op.NotifyMessage, eventMeta, &op.TextContent{Text: text})
}

type canonicalToolCallProgressState struct {
	ID           string         `json:"id,omitempty"`
	Name         string         `json:"name,omitempty"`
	RawArguments string         `json:"rawArguments,omitempty"`
	Arguments    map[string]any `json:"arguments,omitempty"`
	Complete     bool           `json:"complete,omitempty"`
}

type canonicalToolCallProgressEvent struct {
	Delta    string                         `json:"delta,omitempty"`
	ToolCall canonicalToolCallProgressState `json:"toolCall"`
}

func canonicalToolCallProgressPayload(event ai.ProviderEvent) canonicalToolCallProgressEvent {
	state := canonicalToolCallProgressState{}
	if event.Block != nil && event.Block.ToolCall != nil {
		toolCall := event.Block.ToolCall
		state.ID = strings.TrimSpace(toolCall.ID)
		state.Name = strings.TrimSpace(toolCall.Name)
		state.RawArguments = strings.TrimSpace(toolCall.RawArguments)
		if state.RawArguments == "" && len(toolCall.Arguments) > 0 {
			state.RawArguments = ai.MarshalToolArgumentsJSON(toolCall.Arguments)
		}
		if len(toolCall.Arguments) > 0 {
			state.Arguments = ai.CloneToolArguments(toolCall.Arguments)
		}
		state.Complete = toolCall.Complete || event.Type == ai.EventCanonicalToolCallEnd
	}
	return canonicalToolCallProgressEvent{
		Delta:    event.Delta,
		ToolCall: state,
	}
}

func emitCanonicalMessageReplay(meta op.Meta, resp *ai.ProviderResponse) {
	if resp == nil {
		return
	}
	message := ai.CloneConversationMessagePtr(&resp.Message)
	if message == nil {
		return
	}
	partial := ai.StreamConversationMessageFromCanonical(*message)
	NotifyProgress(op.NotifyMessage, meta.Add(op.Meta{
		"type": string(ai.EventCanonicalStart),
	}), &op.TextContent{Text: ""})
	for index, block := range message.Content {
		streamBlock := ai.StreamContentBlockFromCanonical(block)
		switch block.Type {
		case ai.BlockText:
			emitCanonicalLifecycleEvent(meta, ai.ProviderEvent{
				Type:         ai.EventCanonicalTextStart,
				ContentIndex: index,
				Block:        &streamBlock,
				Partial:      partial,
			})
			if block.Text != "" {
				emitCanonicalLifecycleEvent(meta, ai.ProviderEvent{
					Type:         ai.EventCanonicalTextDelta,
					ContentIndex: index,
					Delta:        block.Text,
					Block:        &streamBlock,
					Partial:      partial,
				})
			}
			emitCanonicalLifecycleEvent(meta, ai.ProviderEvent{
				Type:         ai.EventCanonicalTextEnd,
				ContentIndex: index,
				Content:      block.Text,
				Block:        &streamBlock,
				Partial:      partial,
			})
		case ai.BlockThinking:
			emitCanonicalLifecycleEvent(meta, ai.ProviderEvent{
				Type:         ai.EventCanonicalThinkingStart,
				ContentIndex: index,
				Block:        &streamBlock,
				Partial:      partial,
			})
			if block.Text != "" {
				emitCanonicalLifecycleEvent(meta, ai.ProviderEvent{
					Type:         ai.EventCanonicalThinkingDelta,
					ContentIndex: index,
					Delta:        block.Text,
					Block:        &streamBlock,
					Partial:      partial,
				})
			}
			emitCanonicalLifecycleEvent(meta, ai.ProviderEvent{
				Type:         ai.EventCanonicalThinkingEnd,
				ContentIndex: index,
				Content:      block.Text,
				Block:        &streamBlock,
				Partial:      partial,
			})
		case ai.BlockToolCall:
			emitCanonicalLifecycleEvent(meta, ai.ProviderEvent{
				Type:         ai.EventCanonicalToolCallStart,
				ContentIndex: index,
				Block:        &streamBlock,
				Partial:      partial,
			})
			if block.ToolCall != nil && strings.TrimSpace(block.ToolCall.RawArguments) != "" {
				emitCanonicalLifecycleEvent(meta, ai.ProviderEvent{
					Type:         ai.EventCanonicalToolCallDelta,
					ContentIndex: index,
					Delta:        block.ToolCall.RawArguments,
					Block:        &streamBlock,
					Partial:      partial,
				})
			}
			emitCanonicalLifecycleEvent(meta, ai.ProviderEvent{
				Type:         ai.EventCanonicalToolCallEnd,
				ContentIndex: index,
				Block:        &streamBlock,
				Partial:      partial,
			})
		}
	}
	NotifyProgress(op.NotifyMessage, meta.Add(op.Meta{
		"type": string(ai.EventCanonicalDone),
	}), &op.TextContent{Text: ""})
}

func emitStableMessageStep(meta op.Meta, eventType string, msg op.Message, stepSeq int) {
	raw, err := json.Marshal(msg)
	if err != nil {
		slog.Warn("marshal stable message step", "error", err, "type", eventType)
		return
	}
	NotifyProgress(op.NotifyMessage, meta.Add(op.Meta{
		"type":    eventType,
		"stepSeq": stepSeq,
	}), &op.JsonContent{Raw: raw})
}

func emitToolResultStep(meta op.Meta, msg op.Message, stepSeq int, arguments map[string]any) {
	eventMeta := op.Meta{
		"stepSeq": stepSeq,
	}
	if argsObj := cloneToolArgumentsObject(arguments); len(argsObj) > 0 {
		eventMeta["argumentsObject"] = argsObj
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		slog.Warn("marshal tool result step", "error", err)
		return
	}
	NotifyProgress(op.NotifyMessage, meta.Add(eventMeta).Add(op.Meta{
		"type": "tool_result_step",
	}), &op.JsonContent{Raw: raw})
}

func emitTurnResultNotification(meta op.Meta, payload op.TurnResultPayload) {
	raw, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal turn result payload", "error", err)
		return
	}
	NotifyProgress(op.NotifyMessage, meta.Add(op.Meta{
		"type": "turn_result",
	}), &op.JsonContent{Raw: raw})
}

func emitStreamEndNotification(meta op.Meta) {
	NotifyProgress(op.NotifyMessage, meta.Add(op.Meta{
		"type": "end",
	}), &op.TextContent{Text: ""})
}

func buildTurnResultPayload(
	turnID string,
	threadMeta op.ThreadMeta,
	userMessage op.Message,
	msgs []op.Message,
	canonicalMessages []ai.ConversationMessage,
	planTurn bool,
) op.TurnResultPayload {
	resolvedPath := strings.TrimSpace(threadMeta.Path)
	if resolvedPath == "" {
		resolvedPath = strings.TrimSpace(threadMeta.ChatPath)
	}
	payload := op.TurnResultPayload{
		ThreadID:       strings.TrimSpace(threadMeta.ThreadID),
		FileID:         strings.TrimSpace(threadMeta.FileID),
		TurnID:         strings.TrimSpace(turnID),
		AgentID:        strings.TrimSpace(threadMeta.AgentID),
		Path:           resolvedPath,
		ChatPath:       resolvedPath,
		Title:          strings.TrimSpace(threadMeta.Title),
		ParentThreadID: strings.TrimSpace(threadMeta.ParentThreadID),
		PlanTurn:       planTurn,
		UserMessage:    userMessage,
	}
	if len(canonicalMessages) > 0 {
		if raw, err := json.Marshal(canonicalMessages); err == nil {
			payload.CanonicalMessages = raw
		}
	}

	type toolArgsPayload struct {
		obj map[string]any
	}
	toolArgsByID := make(map[string]toolArgsPayload)
	lastAssistantText := ""
	lastAssistantStopReason := op.MessageStopReason("")
	reasoningTexts := make([]string, 0, 1)

	for _, msg := range msgs {
		switch msg.Role {
		case op.RoleAssistant:
			lastAssistantText = strings.TrimSpace(msg.Content)
			lastAssistantStopReason = msg.StopReason
			if reasoning := strings.TrimSpace(msg.ReasoningContent); reasoning != "" {
				reasoningTexts = append(reasoningTexts, reasoning)
			}
			for _, call := range msg.ToolCalls {
				if id := strings.TrimSpace(call.ID); id != "" {
					toolArgsByID[id] = toolArgsPayload{
						obj: cloneToolArgumentsObject(call.Arguments),
					}
				}
			}
		case op.RoleTool:
			resultText := strings.TrimSpace(msg.Content)
			if resultText == "" {
				continue
			}
			lower := strings.ToLower(resultText)
			argsPayload := toolArgsByID[strings.TrimSpace(msg.ToolCallID)]
			payload.ToolResults = append(payload.ToolResults, op.TurnResultToolResult{
				ToolName:        normalizeToolName(msg.Name),
				ArgumentsObject: argsPayload.obj,
				ResultText:      resultText,
				IsError: strings.Contains(lower, "failed") ||
					strings.Contains(lower, "error") ||
					strings.Contains(lower, "not found") ||
					strings.Contains(lower, "invalid"),
			})
		}
	}

	if lastAssistantText == "" {
		switch lastAssistantStopReason {
		case op.StopReasonAborted:
			lastAssistantText = "Turn interrupted before completion."
		case op.StopReasonError:
			lastAssistantText = "Turn failed before completion."
		}
	}
	payload.AssistantText = lastAssistantText
	payload.ReasoningText = strings.Join(reasoningTexts, "\n\n")
	return payload
}

func emitToolResultActivity(meta op.Meta, result op.Message, status string, arguments map[string]any) {
	_ = meta
	_ = result
	_ = status
	_ = arguments
}

func cloneToolArgumentsObject(arguments map[string]any) map[string]any {
	return ai.CloneToolArguments(arguments)
}

func collectToolSpecs(toolSpecs map[string]*op.ToolSpec) []op.ToolSpec {
	if len(toolSpecs) == 0 {
		return nil
	}
	result := make([]op.ToolSpec, 0, len(toolSpecs))
	for _, spec := range toolSpecs {
		if spec == nil {
			continue
		}
		result = append(result, *spec)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.TrimSpace(result[i].Name) < strings.TrimSpace(result[j].Name)
	})
	return result
}

// executeToolCalls runs tools for the given assistant message, optionally collects steering messages during execution.
func (l *AgentLoop) executeToolCalls(loop Loop, assistant op.Message) (toolResults []op.Message, steering []PendingLoopMessage, err error) {
	if l == nil || l.Agent == nil {
		return nil, nil, fmt.Errorf("agent loop is not initialized")
	}
	if len(assistant.ToolCalls) == 0 {
		return nil, nil, nil
	}

	toolResults = make([]op.Message, 0, len(assistant.ToolCalls))
	for index, toolCall := range assistant.ToolCalls {
		result := l.executeSingleToolCall(loop, toolCall)
		toolResults = append(toolResults, result)

		if l.getSteeringMessages == nil {
			continue
		}
		pending, getErr := l.getSteeringMessages(loop.Ctx)
		if getErr != nil {
			slog.Warn("failed to get steering messages during tool execution", "error", getErr, "toolCallID", toolCall.ID)
			continue
		}
		if len(pending) == 0 {
			continue
		}

		steering = pending
		for _, skipped := range assistant.ToolCalls[index+1:] {
			skippedResult := skippedToolResultMessage(skipped)
			emitToolResultStep(loop.Meta, skippedResult, l.nextStepSeq(), skipped.Arguments)
			toolResults = append(toolResults, skippedResult)
		}
		break
	}

	return toolResults, steering, nil
}

// executeSingleToolCall resolves one tool call against the agent's registered tool specs,
// executes it, and always returns a tool result message. This keeps the assistant
// -> tool_result -> assistant sequence valid for providers like Claude.
func (l *AgentLoop) executeSingleToolCall(loop Loop, toolCall op.MessageToolCall) op.Message {
	toolName := normalizeToolName(toolCall.Name)
	callID := strings.TrimSpace(toolCall.ID)
	if toolName == "" || callID == "" {
		result := errorToolResultMessage(toolCall, fmt.Errorf("assistant tool call missing id or name"))
		emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
		return result
	}

	slog.Info("tool execution started", "toolCallID", callID, "toolName", toolName)

	spec := l.resolveToolSpec(toolName)
	if spec == nil {
		result := errorToolResultMessage(toolCall, fmt.Errorf("tool not found: %s", toolName))
		emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
		slog.Warn("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "not_found")
		return result
	}
	serverID := strings.TrimSpace(spec.ServerID)
	if serverID == "" {
		result := errorToolResultMessage(toolCall, fmt.Errorf("tool %s missing serverID", toolName))
		emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
		slog.Warn("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "missing_server")
		return result
	}

	params, err := parseToolCallArguments(toolCall)
	if err != nil {
		result := errorToolResultMessage(toolCall, err)
		emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
		slog.Warn("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "invalid_arguments", "error", err)
		return result
	}
	params = sanitizeToolArgumentsForSchema(params, spec.InputSchema)
	params, err = applyGBrainQueryScopeToToolCall(loop.Meta, serverID, normalizeToolName(spec.Name), spec.InputSchema, params)
	if err != nil {
		result := errorToolResultMessage(toolCall, err)
		emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
		slog.Warn("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "scope_rejected", "error", err)
		return result
	}

	if isBuiltinSystoolSpec(spec) {
		result, execErr := l.executeBuiltinSystoolCall(loop, toolCall, params)
		if execErr != nil {
			result = errorToolResultMessage(toolCall, execErr)
			emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
			slog.Warn("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "error", "error", execErr)
			return result
		}
		emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
		slog.Info("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "ok")
		return result
	}

	var preparedReview *preparedReviewMutation
	if shouldTrackToolReview(toolName) {
		rawPath, ok := extractTrackedToolPath(params)
		if ok {
			preparedReview, err = prepareReviewMutation(op.ThreadMeta{
				ThreadID: loop.ThreadID,
				AgentID:  metaString(loop.Meta, "agentID"),
				CWD:      loop.Workdir,
				ChatPath: metaString(loop.Meta, "chatPath"),
			}, loop.TurnID, loop.Workdir, rawPath)
			if err != nil {
				result := errorToolResultMessage(toolCall, fmt.Errorf("prepare file review for %s: %w", toolName, err))
				emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
				slog.Warn("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "review_prepare_error", "error", err)
				return result
			}
		}
	}

	text, callResult, execErr := callTool(&loop, ToolCall{
		Type: strings.TrimSpace(toolCall.Type),
		ID:   callID,
		Info: toolInfo{
			Name:   normalizeToolName(spec.Name),
			Params: params,
		},
	}, serverID, normalizeToolName(spec.Name))
	if execErr != nil {
		result := errorToolResultMessage(toolCall, execErr)
		emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
		slog.Warn("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "error", "error", execErr)
		return result
	}
	if preparedReview != nil {
		finalBytes, readErr := os.ReadFile(preparedReview.ResolvedPath)
		if readErr != nil {
			result := errorToolResultMessage(toolCall, fmt.Errorf("read final file for review: %w", readErr))
			emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
			slog.Warn("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "review_read_error", "error", readErr)
			return result
		}
		if _, reviewErr := commitReviewMutation(op.ThreadMeta{
			ThreadID: loop.ThreadID,
			AgentID:  metaString(loop.Meta, "agentID"),
			CWD:      loop.Workdir,
			ChatPath: metaString(loop.Meta, "chatPath"),
		}, preparedReview, string(finalBytes)); reviewErr != nil {
			result := errorToolResultMessage(toolCall, fmt.Errorf("commit file review for %s: %w", toolName, reviewErr))
			emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
			slog.Warn("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "review_commit_error", "error", reviewErr)
			return result
		}
	}

	result := toolResultMessageFromCallResult(toolName, callID, text, callResult)
	emitToolResultStep(loop.Meta, result, l.nextStepSeq(), toolCall.Arguments)
	slog.Info("tool execution finished", "toolCallID", callID, "toolName", toolName, "status", "ok")
	return result
}

func (l *AgentLoop) resolveToolSpec(toolName string) *op.ToolSpec {
	if l == nil || l.Agent == nil || len(l.Agent.ToolSpecs) == 0 {
		return nil
	}
	return l.Agent.ToolSpecs[normalizeToolName(toolName)]
}

func parseToolCallArguments(toolCall op.MessageToolCall) (any, error) {
	if len(toolCall.Arguments) == 0 {
		return map[string]any{}, nil
	}
	return cloneToolArgumentsObject(toolCall.Arguments), nil
}

func errorToolResultMessage(toolCall op.MessageToolCall, err error) op.Message {
	message := "tool execution failed"
	if err != nil {
		message = err.Error()
	}
	return op.NewToolResultMessage(normalizeToolName(toolCall.Name), strings.TrimSpace(toolCall.ID), message)
}

func skippedToolResultMessage(toolCall op.MessageToolCall) op.Message {
	return op.NewToolResultMessage(
		normalizeToolName(toolCall.Name),
		strings.TrimSpace(toolCall.ID),
		"Skipped due to queued user message.",
	)
}

func shouldTrackToolReview(toolName string) bool {
	switch normalizeToolName(toolName) {
	case "write", "edit":
		return true
	default:
		return false
	}
}

func extractTrackedToolPath(params any) (string, bool) {
	args, ok := params.(map[string]any)
	if !ok {
		return "", false
	}
	path, ok := args["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return "", false
	}
	return path, true
}

func metaString(meta op.Meta, key string) string {
	if meta == nil {
		return ""
	}
	value, _ := meta[key].(string)
	return strings.TrimSpace(value)
}

func loopPromptCacheKey(loop *AgentLoop) string {
	if loop == nil {
		return ""
	}
	if key := strings.TrimSpace(loop.ThreadID); key != "" {
		return key
	}
	return metaString(loop.Meta, "threadID")
}

func cfgOrEmptyPendingMessages(cfg *RunLoopConfig) []PendingLoopMessage {
	if cfg == nil || len(cfg.InitialPendingMessages) == 0 {
		return nil
	}
	return cfg.InitialPendingMessages
}

func cfgSkipInitialSteeringPoll(cfg *RunLoopConfig) bool {
	return cfg != nil && cfg.SkipInitialSteeringPoll
}

func emitQueuedMessageDelivered(meta op.Meta, queueKind string, msg op.Message, itemID string) {
	if queueKind != queueKindSteering && queueKind != queueKindFollowUp {
		return
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		slog.Warn("marshal queued message delivery", "error", err, "queueKind", queueKind)
		return
	}
	NotifyProgress(op.NotifyMessage, meta.Add(op.Meta{
		"type":      "queuedMessageDelivered",
		"queueType": queueKind,
		"itemID":    strings.TrimSpace(itemID),
	}), &op.JsonContent{Raw: raw})
}
