package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
	"github.com/rs/xid"
)

const (
	cronTaskStoreVersion  = 1
	cronStateVersion      = 1
	cronHistoryVersion    = 1
	cronHistoryLimit      = 99
	cronTaskStoreFileName = "jobs.json"
	cronStateFileName     = "state.json"
	cronStaleAfter        = 2 * time.Hour
	cronPollInterval      = 30 * time.Second

	cronTargetKindAgent      = "agent"
	cronPayloadKindAgentTurn = "agentTurn"
	cronManagedKindCloudSync = "openbrain-cloud-sync"
)

const (
	cronOpList    op.OpCode = "cron/list"
	cronOpGet     op.OpCode = "cron/get"
	cronOpAdd     op.OpCode = "cron/add"
	cronOpUpsert  op.OpCode = "cron/upsert"
	cronOpUpdate  op.OpCode = "cron/update"
	cronOpRemove  op.OpCode = "cron/remove"
	cronOpRun     op.OpCode = "cron/run"
	cronOpHistory op.OpCode = "cron/history"
)

type CronTask struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Enabled     bool             `json:"enabled"`
	Schedule    CronTaskSchedule `json:"schedule"`
	Target      CronTaskTarget   `json:"target"`
	Payload     CronTaskPayload  `json:"payload"`
	CreatedAtMs int64            `json:"createdAtMs,omitempty"`
	UpdatedAtMs int64            `json:"updatedAtMs,omitempty"`
}

func (t *CronTask) UnmarshalJSON(data []byte) error {
	type taskAlias CronTask
	wire := struct {
		taskAlias
		Enabled *bool `json:"enabled"`
	}{
		taskAlias: taskAlias{Enabled: true},
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	task := CronTask(wire.taskAlias)
	if wire.Enabled != nil {
		task.Enabled = *wire.Enabled
	}
	*t = task
	return nil
}

type CronTaskSchedule struct {
	Cron  string `json:"cron,omitempty"`
	Every string `json:"every,omitempty"`
	Time  string `json:"time,omitempty"`
}

type CronTaskTarget struct {
	Kind    string `json:"kind"`
	AgentID string `json:"agentID"`
	CWD     string `json:"cwd"`
}

type CronTaskPayload struct {
	Kind string         `json:"kind"`
	Text string         `json:"text,omitempty"`
	Data map[string]any `json:"data,omitempty"`
}

type CronTaskStoreFile struct {
	Version int        `json:"version"`
	Tasks   []CronTask `json:"tasks"`
}

type CronTaskState struct {
	TaskID            string           `json:"taskID"`
	SpecHash          string           `json:"specHash,omitempty"`
	NextRunAtMs       int64            `json:"nextRunAtMs,omitempty"`
	RunNowAtMs        int64            `json:"runNowAtMs,omitempty"`
	RunNowPayload     *CronTaskPayload `json:"runNowPayload,omitempty"`
	LastRunAtMs       int64            `json:"lastRunAtMs,omitempty"`
	RunningAtMs       int64            `json:"runningAtMs,omitempty"`
	LastError         string           `json:"lastError,omitempty"`
	ConsecutiveErrors int              `json:"consecutiveErrors,omitempty"`
}

type cronStateFile struct {
	Version int             `json:"version"`
	Tasks   []CronTaskState `json:"tasks"`
}

type CronTaskRunHistoryEntry struct {
	RunID         string `json:"runID"`
	TaskID        string `json:"taskID"`
	Trigger       string `json:"trigger"`
	ScheduledAtMs int64  `json:"scheduledAtMs,omitempty"`
	StartedAtMs   int64  `json:"startedAtMs"`
	FinishedAtMs  int64  `json:"finishedAtMs,omitempty"`
	DurationMs    int64  `json:"durationMs,omitempty"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
	ThreadID      string `json:"threadID,omitempty"`
	ChatPath      string `json:"chatPath,omitempty"`
	AgentID       string `json:"agentID,omitempty"`
}

type cronTaskHistoryFile struct {
	Version int                       `json:"version"`
	Runs    []CronTaskRunHistoryEntry `json:"runs"`
}

type CronTaskRecord struct {
	Task  CronTask       `json:"task"`
	State *CronTaskState `json:"state,omitempty"`
}

type CronTaskListResult struct {
	Version int              `json:"version"`
	Tasks   []CronTaskRecord `json:"tasks"`
}

type CronTaskRunResult struct {
	Queued bool           `json:"queued"`
	Task   CronTaskRecord `json:"task"`
}

type CronTaskHistoryResult struct {
	TaskID string                    `json:"taskID"`
	Limit  int                       `json:"limit"`
	Runs   []CronTaskRunHistoryEntry `json:"runs"`
}

type cronTaskIDParams struct {
	ID      string           `json:"id"`
	Payload *CronTaskPayload `json:"payload,omitempty"`
}

type cronTaskHistoryParams struct {
	ID    string `json:"id"`
	Limit int    `json:"limit,omitempty"`
}

type cronTaskWriteParams struct {
	Task CronTask `json:"task"`
}

type cronExecutor func(context.Context, *op.OpNode, op.Meta, op.Content) (*op.OpNodeResult, error)

type cronManager struct {
	ctx        context.Context
	cancel     context.CancelFunc
	now        func() time.Time
	executor   cronExecutor
	tasksPath  string
	statePath  string
	historyDir string

	signalCh chan struct{}

	mu           sync.Mutex
	ready        bool
	tasksLoaded  bool
	tasksModTime time.Time
	tasksSize    int64
	tasks        map[string]CronTask
	states       map[string]*CronTaskState
}

var (
	cronMu      sync.Mutex
	defaultCron *cronManager
)

func StartCron(ctx context.Context, cfg *op.SystemConfig) error {
	if cfg == nil || strings.TrimSpace(cfg.BaseDir) == "" {
		return fmt.Errorf("system baseDir is required")
	}

	cronMu.Lock()
	defer cronMu.Unlock()

	if defaultCron != nil {
		defaultCron.stop()
		defaultCron = nil
	}

	managerCtx := ctx
	if managerCtx == nil {
		managerCtx = context.Background()
	}
	manager, err := newCronManager(managerCtx, strings.TrimSpace(cfg.BaseDir), time.Now, func(callCtx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		return executeAgentCall(callCtx, node, meta, content, agentCallOptions{ensureSession: true})
	})
	if err != nil {
		return err
	}
	defaultCron = manager

	go manager.loop()
	return nil
}

func StopCron() {
	cronMu.Lock()
	defer cronMu.Unlock()
	if defaultCron == nil {
		return
	}
	defaultCron.stop()
	defaultCron = nil
}

func reconcileCronNodes(_ []op.OpNode) error {
	cronMu.Lock()
	manager := defaultCron
	cronMu.Unlock()
	if manager == nil {
		return nil
	}
	return manager.markNodeCacheReadyAndReload()
}

func newCronManager(parent context.Context, baseDir string, now func() time.Time, executor cronExecutor) (*cronManager, error) {
	ctx, cancel := context.WithCancel(parent)
	manager := &cronManager{
		ctx:        ctx,
		cancel:     cancel,
		now:        now,
		executor:   executor,
		tasksPath:  filepath.Join(baseDir, "cron", cronTaskStoreFileName),
		statePath:  filepath.Join(baseDir, "run", "cron", cronStateFileName),
		historyDir: filepath.Join(baseDir, "cron", "history"),
		signalCh:   make(chan struct{}, 1),
		tasks:      make(map[string]CronTask),
		states:     make(map[string]*CronTaskState),
	}
	if err := manager.loadState(); err != nil {
		cancel()
		return nil, err
	}
	manager.mu.Lock()
	if err := manager.loadTaskStoreLocked(); err != nil {
		manager.mu.Unlock()
		cancel()
		return nil, err
	}
	manager.reconcileTasksLocked(manager.now())
	if err := manager.saveStateLocked(); err != nil {
		manager.mu.Unlock()
		cancel()
		return nil, err
	}
	manager.mu.Unlock()
	return manager, nil
}

func (s *cronManager) stop() {
	s.cancel()
}

func (s *cronManager) loop() {
	for {
		if err := s.reloadTasksIfChanged(); err != nil {
			slog.Warn("failed to reload scheduled tasks", "error", err)
		}
		delay := s.nextDelay()
		if delay < 0 {
			select {
			case <-s.ctx.Done():
				return
			case <-s.signalCh:
				continue
			}
		}

		timer := time.NewTimer(delay)
		select {
		case <-s.ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-s.signalCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			continue
		case <-timer.C:
			s.runDue()
		}
	}
}

func (s *cronManager) nextDelay() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.ready {
		return -1
	}

	var nextRunAt int64
	for taskID, state := range s.states {
		if state == nil || state.RunningAtMs > 0 {
			continue
		}
		task, ok := s.tasks[taskID]
		if !ok {
			continue
		}
		candidate := int64(0)
		if state.RunNowAtMs > 0 {
			candidate = state.RunNowAtMs
		} else if task.Enabled && state.NextRunAtMs > 0 {
			candidate = state.NextRunAtMs
		}
		if candidate > 0 && (nextRunAt == 0 || candidate < nextRunAt) {
			nextRunAt = candidate
		}
	}
	if nextRunAt == 0 {
		return cronPollInterval
	}
	delay := time.UnixMilli(nextRunAt).Sub(s.now())
	if delay < 0 {
		return 0
	}
	if delay > cronPollInterval {
		return cronPollInterval
	}
	return delay
}

func (s *cronManager) markNodeCacheReadyAndReload() error {
	s.mu.Lock()
	s.ready = true
	if err := s.loadTaskStoreLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.reconcileTasksLocked(s.now())
	if err := s.saveStateLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	s.signalLoop()
	return nil
}

func (s *cronManager) runDue() {
	for {
		task, scheduledAt, manual, ok, err := s.reserveNextDue()
		if err != nil {
			slog.Warn("failed to reserve scheduled task", "error", err)
			return
		}
		if !ok {
			return
		}

		callErr := s.executeTask(task, scheduledAt)
		s.finishRun(task.ID, scheduledAt, manual, callErr)
	}
}

func (s *cronManager) reserveNextDue() (CronTask, time.Time, bool, bool, error) {
	now := s.now()
	nowMs := now.UnixMilli()

	s.mu.Lock()
	defer s.mu.Unlock()

	type candidate struct {
		task    CronTask
		state   *CronTaskState
		dueAtMs int64
		manual  bool
	}
	var (
		picked *candidate
		dirty  bool
	)
	for taskID, state := range s.states {
		if state == nil || state.RunningAtMs > 0 {
			continue
		}
		task, ok := s.tasks[taskID]
		if !ok {
			continue
		}
		manual := state.RunNowAtMs > 0 && state.RunNowAtMs <= nowMs
		dueAtMs := state.NextRunAtMs
		if manual {
			dueAtMs = state.RunNowAtMs
		}
		if dueAtMs <= 0 || dueAtMs > nowMs {
			continue
		}
		if !task.Enabled && !manual {
			continue
		}
		if err := validateCronTask(task); err != nil {
			state.LastError = err.Error()
			state.NextRunAtMs = 0
			state.RunNowAtMs = 0
			state.RunNowPayload = nil
			dirty = true
			continue
		}
		if manual && state.RunNowPayload != nil {
			task.Payload = *state.RunNowPayload
			if err := validateCronTask(task); err != nil {
				state.LastError = err.Error()
				state.RunNowAtMs = 0
				state.RunNowPayload = nil
				dirty = true
				continue
			}
		}
		if picked == nil || dueAtMs < picked.dueAtMs || (dueAtMs == picked.dueAtMs && taskID < picked.state.TaskID) {
			picked = &candidate{task: task, state: state, dueAtMs: dueAtMs, manual: manual}
		}
	}
	if picked == nil {
		if dirty {
			return CronTask{}, time.Time{}, false, false, s.saveStateLocked()
		}
		return CronTask{}, time.Time{}, false, false, nil
	}

	picked.state.RunningAtMs = nowMs
	picked.state.LastError = ""
	if err := s.recordHistoryStartLocked(picked.task, time.UnixMilli(picked.dueAtMs), picked.manual, nowMs); err != nil {
		slog.Warn("failed to persist cron run history", "error", err, "taskID", picked.task.ID)
	}
	if err := s.saveStateLocked(); err != nil {
		return CronTask{}, time.Time{}, false, false, err
	}
	return picked.task, time.UnixMilli(picked.dueAtMs), picked.manual, true, nil
}

func (s *cronManager) executeTask(task CronTask, scheduledAt time.Time) error {
	if err := validateCronTask(task); err != nil {
		return err
	}

	node, ok := cache.GetValue[op.OpNode](task.Target.AgentID, cache.PrefixNode)
	if !ok {
		return fmt.Errorf("agent node not found: %s", task.Target.AgentID)
	}
	if node.Kind != string(op.NodeKindAgent) {
		return fmt.Errorf("target node is not an agent: %s", task.Target.AgentID)
	}

	meta, content, err := buildCronTaskCall(task, scheduledAt)
	if err != nil {
		return err
	}
	if err := s.ensureCronTaskModel(task, meta); err != nil {
		return err
	}
	_, err = s.executor(s.ctx, &node, meta, content)
	return err
}

func (s *cronManager) ensureCronTaskModel(task CronTask, meta op.Meta) error {
	modelKey := strings.TrimSpace(metaString(meta, "modelKey"))
	managedCloudSync := cronTaskManagedKind(task) == cronManagedKindCloudSync
	if modelKey == "" {
		err := cronTaskModelRequiredError(task, managedCloudSync)
		s.publishCronModelConfigMessage(task, meta, err.Error(), managedCloudSync)
		return err
	}
	if _, err := config.GetModelConfig(modelKey); err != nil {
		wrapped := fmt.Errorf("cron task %s model %q is not available: %w", strings.TrimSpace(task.ID), modelKey, err)
		s.publishCronModelConfigMessage(task, meta, wrapped.Error(), managedCloudSync)
		return wrapped
	}
	return nil
}

func cronTaskModelRequiredError(task CronTask, managedCloudSync bool) error {
	if managedCloudSync {
		return fmt.Errorf("OpenBrain Cloud Sync cron task %s requires payload.data.modelKey; configure a chat model for this job", strings.TrimSpace(task.ID))
	}
	return fmt.Errorf("cron task %s requires payload.data.modelKey; configure a chat model for this job", strings.TrimSpace(task.ID))
}

func (s *cronManager) publishCronModelConfigMessage(task CronTask, meta op.Meta, reason string, managedCloudSync bool) {
	threadID := strings.TrimSpace(metaString(meta, "threadID"))
	agentID := strings.TrimSpace(task.Target.AgentID)
	if agentID == "" {
		agentID = strings.TrimSpace(metaString(meta, "agentID"))
	}
	if err := ensureCronMessageSession(task, meta, threadID, agentID); err != nil {
		slog.Warn("failed to create cron message thread", "error", err, "taskID", task.ID)
		return
	}
	title := "Cron task needs a chat model"
	body := fmt.Sprintf(
		"Cron task %q cannot run because no usable chat model is configured. Set payload.data.modelKey for this job and run it again.",
		cronTaskDisplayName(task),
	)
	if managedCloudSync {
		title = "OpenBrain Cloud Sync needs a chat model"
		body = "OpenBrain Cloud Sync cannot run because this cron job has no usable chat model. Open Cron, set the job Model, then run sync again."
	}
	if strings.TrimSpace(reason) != "" {
		body += "\n\nReason: " + strings.TrimSpace(reason)
	}
	record, err := defaultMessageStore.appendRecord(op.MessageSenderSystem, op.MessageRecord{
		ThreadID: threadID,
		AgentID:  agentID,
		Kind:     op.MessageKindRequest,
		Status:   op.MessageStatusOpen,
		Title:    title,
		Body:     body,
		Meta: op.Meta{
			"taskID":      strings.TrimSpace(task.ID),
			"managedKind": cronTaskManagedKind(task),
			"reason":      "model_required",
		},
	})
	if err != nil {
		slog.Warn("failed to publish cron model configuration message", "error", err, "taskID", task.ID)
		return
	}
	_ = notifyMessageRecord(meta, record)
}

func ensureCronMessageSession(task CronTask, meta op.Meta, threadID string, agentID string) error {
	threadID = strings.TrimSpace(threadID)
	agentID = strings.TrimSpace(agentID)
	if threadID == "" || agentID == "" {
		return fmt.Errorf("threadID and agentID are required")
	}
	if _, err := getThreadMeta(threadID, agentID); err == nil {
		return nil
	} else if !isThreadNotFound(err) {
		return err
	}
	cwd := strings.TrimSpace(task.Target.CWD)
	if cwd == "" {
		cwd = strings.TrimSpace(metaString(meta, "cwd"))
	}
	title := cronTaskDisplayName(task)
	if title == "" {
		title = "Cron Thread"
	}
	_, err := createThreadWithID(op.ThreadCreateParams{
		AgentID: agentID,
		CWD:     cwd,
		Title:   title,
	}, threadID)
	return err
}

func cronTaskDisplayName(task CronTask) string {
	if name := strings.TrimSpace(task.Name); name != "" {
		return name
	}
	return strings.TrimSpace(task.ID)
}

func cronTaskManagedKind(task CronTask) string {
	return cronPayloadManagedKind(task.Payload.Data)
}

func cronPayloadManagedKind(payloadData map[string]any) string {
	if kind := strings.TrimSpace(stringFromMap(payloadData, "managedKind")); kind != "" {
		return kind
	}
	if selectedSkillContext, ok := metaMapFromMap(payloadData, "selectedSkillContext"); ok {
		return strings.TrimSpace(metaString(selectedSkillContext, "managedKind"))
	}
	return ""
}

func shouldSanitizeScheduledCloudSyncPayload(payloadData map[string]any) bool {
	if cronPayloadManagedKind(payloadData) != cronManagedKindCloudSync {
		return false
	}
	if boolFromMap(payloadData, "manual") || boolFromMap(payloadData, "manualRunIncludesDisabled") {
		return false
	}
	if selectedSkillContext, ok := metaMapFromMap(payloadData, "selectedSkillContext"); ok {
		if boolFromMap(map[string]any(selectedSkillContext), "manual") {
			return false
		}
	}
	return true
}

func sanitizeScheduledCloudSyncPayload(payloadData map[string]any) map[string]any {
	next := cloneMap(payloadData)
	emptyWorkspaces := []any{}
	next["workspaces"] = emptyWorkspaces
	next["workspaceSnapshot"] = emptyWorkspaces
	next["requestedWorkspace"] = map[string]any{}
	next["manual"] = false
	next["manualRunIncludesDisabled"] = false

	selectedSkillContext, _ := metaMapFromMap(next, "selectedSkillContext")
	if selectedSkillContext == nil {
		selectedSkillContext = op.Meta{}
	}
	selectedSkillContext["managedKind"] = cronManagedKindCloudSync
	selectedSkillContext["workspaces"] = emptyWorkspaces
	selectedSkillContext["workspaceSnapshot"] = emptyWorkspaces
	selectedSkillContext["requestedWorkspace"] = map[string]any{}
	selectedSkillContext["manual"] = false
	next["selectedSkillContext"] = selectedSkillContext
	return next
}

func scheduledCloudSyncTaskText() string {
	return strings.TrimSpace(`Run OpenBrain Cloud Sync as a scheduled maintenance task.

Before syncing, run the openbrain-cloud-sync helper preflight command. Treat the helper's current auth/account workspace list as the only source of truth. Do not trust any persisted cron payload workspace snapshot; it is audit-only and may be stale after an account switch.

Process only the workspaces returned by helper preflight for the current signed-in account. If preflight returns no workspaces or reports that auth is required, report that no sync was run and do not use old payload data.`)
}

func (s *cronManager) finishRun(taskID string, scheduledAt time.Time, manual bool, callErr error) {
	if strings.TrimSpace(taskID) == "" {
		return
	}
	now := s.now()
	if callErr != nil {
		slog.Warn("scheduled task execution failed", "taskID", taskID, "error", callErr)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.states[taskID]
	if !ok || state == nil {
		return
	}
	startedAtMs := state.RunningAtMs
	state.RunningAtMs = 0
	if manual {
		state.RunNowAtMs = 0
		state.RunNowPayload = nil
	}
	state.LastRunAtMs = now.UnixMilli()

	task, taskExists := s.tasks[taskID]
	if taskExists {
		if err := s.recordHistoryFinishLocked(task, scheduledAt, manual, startedAtMs, now.UnixMilli(), callErr); err != nil {
			slog.Warn("failed to persist cron run history", "error", err, "taskID", taskID)
		}
	}
	if !taskExists || !task.Enabled {
		state.NextRunAtMs = 0
		if err := s.saveStateLocked(); err != nil {
			slog.Warn("failed to persist cron state", "error", err, "taskID", taskID)
		}
		s.signalLoop()
		return
	}

	if manual {
		if callErr != nil {
			state.LastError = callErr.Error()
			state.ConsecutiveErrors++
		} else {
			state.LastError = ""
			state.ConsecutiveErrors = 0
		}
		if state.NextRunAtMs <= now.UnixMilli() {
			nextRunAt, nextErr := computeScheduledNextRun(task.Schedule, now)
			if nextErr != nil {
				state.LastError = nextErr.Error()
				state.NextRunAtMs = 0
			} else {
				state.NextRunAtMs = nextRunAt.UnixMilli()
			}
		}
		if err := s.saveStateLocked(); err != nil {
			slog.Warn("failed to persist cron state", "error", err, "taskID", taskID)
		}
		s.signalLoop()
		return
	}

	nextRunAt, nextErr := advanceScheduledNextRun(task.Schedule, scheduledAt, now)
	if callErr != nil {
		state.LastError = callErr.Error()
		state.ConsecutiveErrors++
		if nextErr == nil {
			backoffUntil := now.Add(cronBackoffDelay(state.ConsecutiveErrors))
			if nextRunAt.Before(backoffUntil) {
				nextRunAt = backoffUntil
			}
		}
	} else {
		state.LastError = ""
		state.ConsecutiveErrors = 0
	}

	if nextErr != nil {
		state.LastError = nextErr.Error()
		state.NextRunAtMs = 0
	} else {
		state.NextRunAtMs = nextRunAt.UnixMilli()
	}

	if err := s.saveStateLocked(); err != nil {
		slog.Warn("failed to persist cron state", "error", err, "taskID", taskID)
	}
	s.signalLoop()
}

func (s *cronManager) listTasks() (*CronTaskListResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadTaskStoreLocked(); err != nil {
		return nil, err
	}
	s.reconcileTasksLocked(s.now())
	if err := s.saveStateLocked(); err != nil {
		return nil, err
	}
	return &CronTaskListResult{
		Version: cronTaskStoreVersion,
		Tasks:   s.taskRecordsLocked(),
	}, nil
}

func (s *cronManager) getTask(id string) (*CronTaskRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadTaskStoreLocked(); err != nil {
		return nil, err
	}
	s.reconcileTasksLocked(s.now())
	if err := s.saveStateLocked(); err != nil {
		return nil, err
	}
	task, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	record := s.taskRecordLocked(task)
	return &record, nil
}

func (s *cronManager) listHistory(id string, limit int) (*CronTaskHistoryResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	limit = normalizeCronHistoryLimit(limit)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadTaskStoreLocked(); err != nil {
		return nil, err
	}
	s.reconcileTasksLocked(s.now())
	if err := s.saveStateLocked(); err != nil {
		return nil, err
	}
	if _, ok := s.tasks[id]; !ok {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	runs, err := s.loadHistoryLocked(id)
	if err != nil {
		return nil, err
	}
	if len(runs) > limit {
		runs = runs[:limit]
	}
	return &CronTaskHistoryResult{
		TaskID: id,
		Limit:  limit,
		Runs:   runs,
	}, nil
}

func (s *cronManager) addTask(task CronTask) (*CronTaskRecord, error) {
	nowMs := s.now().UnixMilli()
	task = normalizeCronTask(task)
	if strings.TrimSpace(task.ID) == "" {
		task.ID = "task-" + xid.New().String()
	}
	if task.CreatedAtMs == 0 {
		task.CreatedAtMs = nowMs
	}
	task.UpdatedAtMs = nowMs
	if err := validateCronTask(task); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadTaskStoreLocked(); err != nil {
		return nil, err
	}
	if _, exists := s.tasks[task.ID]; exists {
		return nil, fmt.Errorf("task already exists: %s", task.ID)
	}
	s.tasks[task.ID] = task
	s.reconcileTasksLocked(s.now())
	if err := s.saveTaskStoreLocked(); err != nil {
		return nil, err
	}
	if err := s.saveStateLocked(); err != nil {
		return nil, err
	}
	record := s.taskRecordLocked(task)
	s.signalLoop()
	return &record, nil
}

func (s *cronManager) upsertTask(task CronTask) (*CronTaskRecord, error) {
	nowMs := s.now().UnixMilli()
	task = normalizeCronTask(task)
	if strings.TrimSpace(task.ID) == "" {
		task.ID = "task-" + xid.New().String()
	}
	task.UpdatedAtMs = nowMs
	if err := validateCronTask(task); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadTaskStoreLocked(); err != nil {
		return nil, err
	}
	existing, exists := s.tasks[task.ID]
	if task.CreatedAtMs == 0 {
		if exists && existing.CreatedAtMs > 0 {
			task.CreatedAtMs = existing.CreatedAtMs
		} else {
			task.CreatedAtMs = nowMs
		}
	}
	s.tasks[task.ID] = task
	s.reconcileTasksLocked(s.now())
	if err := s.saveTaskStoreLocked(); err != nil {
		return nil, err
	}
	if err := s.saveStateLocked(); err != nil {
		return nil, err
	}
	record := s.taskRecordLocked(task)
	s.signalLoop()
	return &record, nil
}

func (s *cronManager) updateTask(task CronTask) (*CronTaskRecord, error) {
	nowMs := s.now().UnixMilli()
	task = normalizeCronTask(task)
	if strings.TrimSpace(task.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	if err := validateCronTask(task); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadTaskStoreLocked(); err != nil {
		return nil, err
	}
	existing, exists := s.tasks[task.ID]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", task.ID)
	}
	if task.CreatedAtMs == 0 {
		task.CreatedAtMs = existing.CreatedAtMs
	}
	task.UpdatedAtMs = nowMs
	s.tasks[task.ID] = task
	s.reconcileTasksLocked(s.now())
	if err := s.saveTaskStoreLocked(); err != nil {
		return nil, err
	}
	if err := s.saveStateLocked(); err != nil {
		return nil, err
	}
	record := s.taskRecordLocked(task)
	s.signalLoop()
	return &record, nil
}

func (s *cronManager) removeTask(id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, fmt.Errorf("id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadTaskStoreLocked(); err != nil {
		return false, err
	}
	if _, exists := s.tasks[id]; !exists {
		return false, fmt.Errorf("task not found: %s", id)
	}
	delete(s.tasks, id)
	delete(s.states, id)
	if err := s.removeHistoryLocked(id); err != nil {
		slog.Warn("failed to remove cron run history", "error", err, "taskID", id)
	}
	if err := s.saveTaskStoreLocked(); err != nil {
		return false, err
	}
	if err := s.saveStateLocked(); err != nil {
		return false, err
	}
	s.signalLoop()
	return true, nil
}

func (s *cronManager) runTaskNow(id string, override *CronTaskPayload) (*CronTaskRunResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadTaskStoreLocked(); err != nil {
		return nil, err
	}
	task, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	if err := validateCronTask(task); err != nil {
		return nil, err
	}
	if override != nil {
		next := task
		next.Payload = normalizeCronTask(CronTask{Payload: *override}).Payload
		if strings.TrimSpace(next.Payload.Kind) == "" {
			next.Payload.Kind = task.Payload.Kind
		}
		if err := validateCronTask(next); err != nil {
			return nil, err
		}
		task = next
	}

	state := s.ensureTaskStateLocked(task)
	specHash, err := cronTaskSpecHash(task)
	if err != nil {
		return nil, err
	}
	state.SpecHash = specHash
	state.RunNowAtMs = s.now().UnixMilli()
	if override != nil {
		payload := task.Payload
		state.RunNowPayload = &payload
	} else {
		state.RunNowPayload = nil
	}
	state.RunningAtMs = 0
	state.LastError = ""
	s.ready = true
	if err := s.saveStateLocked(); err != nil {
		return nil, err
	}
	record := s.taskRecordLocked(task)
	s.signalLoop()
	return &CronTaskRunResult{Queued: true, Task: record}, nil
}

func (s *cronManager) loadTaskStoreLocked() error {
	info, err := os.Stat(s.tasksPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		s.tasks = make(map[string]CronTask)
		s.tasksLoaded = true
		s.tasksModTime = time.Time{}
		s.tasksSize = 0
		return nil
	}

	raw, err := os.ReadFile(s.tasksPath)
	if err != nil {
		return err
	}

	var store CronTaskStoreFile
	if err := common.UnmarshalJSONC(raw, &store); err != nil {
		return err
	}
	if store.Version != 0 && store.Version != cronTaskStoreVersion {
		return fmt.Errorf("unsupported task store version %d", store.Version)
	}

	nextTasks := make(map[string]CronTask, len(store.Tasks))
	for _, entry := range store.Tasks {
		task := normalizeCronTask(entry)
		if strings.TrimSpace(task.ID) == "" {
			return fmt.Errorf("task id is required")
		}
		if _, exists := nextTasks[task.ID]; exists {
			return fmt.Errorf("duplicate task id: %s", task.ID)
		}
		nextTasks[task.ID] = task
	}
	s.tasks = nextTasks
	s.tasksLoaded = true
	s.tasksModTime = info.ModTime()
	s.tasksSize = info.Size()
	return nil
}

func (s *cronManager) loadTaskStoreIfChangedLocked() (bool, error) {
	info, err := os.Stat(s.tasksPath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if os.IsNotExist(err) {
		if s.tasksLoaded && len(s.tasks) == 0 && s.tasksModTime.IsZero() && s.tasksSize == 0 {
			return false, nil
		}
		return true, s.loadTaskStoreLocked()
	}
	if s.tasksLoaded && info.ModTime().Equal(s.tasksModTime) && info.Size() == s.tasksSize {
		return false, nil
	}
	return true, s.loadTaskStoreLocked()
}

func (s *cronManager) saveTaskStoreLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.tasksPath), 0o755); err != nil {
		return err
	}

	tasks := make([]CronTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	payload, err := json.MarshalIndent(CronTaskStoreFile{
		Version: cronTaskStoreVersion,
		Tasks:   tasks,
	}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := s.tasksPath + ".tmp"
	if err := os.WriteFile(tmpPath, append(payload, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.tasksPath); err != nil {
		return err
	}
	info, err := os.Stat(s.tasksPath)
	if err != nil {
		return err
	}
	s.tasksLoaded = true
	s.tasksModTime = info.ModTime()
	s.tasksSize = info.Size()
	return nil
}

func (s *cronManager) loadState() error {
	raw, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var stateFile cronStateFile
	if err := json.Unmarshal(raw, &stateFile); err != nil {
		return err
	}
	if stateFile.Version != 0 && stateFile.Version != cronStateVersion {
		return fmt.Errorf("unsupported cron state version %d", stateFile.Version)
	}

	for _, entry := range stateFile.Tasks {
		item := entry
		if strings.TrimSpace(item.TaskID) == "" {
			continue
		}
		s.states[item.TaskID] = &item
	}
	return nil
}

func (s *cronManager) saveStateLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o755); err != nil {
		return err
	}

	entries := make([]CronTaskState, 0, len(s.states))
	for _, state := range s.states {
		if state == nil || strings.TrimSpace(state.TaskID) == "" {
			continue
		}
		entries = append(entries, *state)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].TaskID < entries[j].TaskID
	})

	payload, err := json.MarshalIndent(cronStateFile{
		Version: cronStateVersion,
		Tasks:   entries,
	}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := s.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, append(payload, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.statePath)
}

func (s *cronManager) taskHistoryPath(taskID string) string {
	return filepath.Join(s.historyDir, cronHistoryFileName(taskID))
}

func cronHistoryFileName(taskID string) string {
	return cronSafePathComponent(taskID) + ".json"
}

func normalizeCronHistoryLimit(limit int) int {
	if limit <= 0 || limit > cronHistoryLimit {
		return cronHistoryLimit
	}
	return limit
}

func (s *cronManager) loadHistoryLocked(taskID string) ([]CronTaskRunHistoryEntry, error) {
	raw, err := os.ReadFile(s.taskHistoryPath(taskID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var history cronTaskHistoryFile
	if err := json.Unmarshal(raw, &history); err != nil {
		return nil, err
	}
	if history.Version != 0 && history.Version != cronHistoryVersion {
		return nil, fmt.Errorf("unsupported cron history version %d", history.Version)
	}
	runs := make([]CronTaskRunHistoryEntry, 0, len(history.Runs))
	for _, entry := range history.Runs {
		if strings.TrimSpace(entry.RunID) == "" {
			continue
		}
		if strings.TrimSpace(entry.TaskID) == "" {
			entry.TaskID = strings.TrimSpace(taskID)
		}
		runs = append(runs, entry)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].StartedAtMs != runs[j].StartedAtMs {
			return runs[i].StartedAtMs > runs[j].StartedAtMs
		}
		if runs[i].FinishedAtMs != runs[j].FinishedAtMs {
			return runs[i].FinishedAtMs > runs[j].FinishedAtMs
		}
		return runs[i].RunID > runs[j].RunID
	})
	if len(runs) > cronHistoryLimit {
		runs = runs[:cronHistoryLimit]
	}
	return runs, nil
}

func (s *cronManager) saveHistoryLocked(taskID string, runs []CronTaskRunHistoryEntry) error {
	if err := os.MkdirAll(s.historyDir, 0o755); err != nil {
		return err
	}
	if len(runs) > cronHistoryLimit {
		runs = runs[:cronHistoryLimit]
	}
	payload, err := json.MarshalIndent(cronTaskHistoryFile{
		Version: cronHistoryVersion,
		Runs:    runs,
	}, "", "  ")
	if err != nil {
		return err
	}
	path := s.taskHistoryPath(taskID)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, append(payload, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *cronManager) removeHistoryLocked(taskID string) error {
	err := os.Remove(s.taskHistoryPath(taskID))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *cronManager) recordHistoryStartLocked(task CronTask, scheduledAt time.Time, manual bool, startedAtMs int64) error {
	entry := CronTaskRunHistoryEntry{
		RunID:         cronTaskRunID(task, scheduledAt),
		TaskID:        strings.TrimSpace(task.ID),
		Trigger:       cronHistoryTrigger(manual),
		ScheduledAtMs: scheduledAt.UnixMilli(),
		StartedAtMs:   startedAtMs,
		Status:        "running",
		ThreadID:      cronTaskThreadID(task, scheduledAt),
		AgentID:       strings.TrimSpace(task.Target.AgentID),
	}
	return s.upsertHistoryEntryLocked(task.ID, entry)
}

func (s *cronManager) recordHistoryFinishLocked(task CronTask, scheduledAt time.Time, manual bool, startedAtMs, finishedAtMs int64, callErr error) error {
	status := "success"
	errorMessage := ""
	if callErr != nil {
		status = "failed"
		errorMessage = callErr.Error()
	}
	return s.recordHistoryFinishedStatusLocked(task, scheduledAt, manual, startedAtMs, finishedAtMs, status, errorMessage)
}

func (s *cronManager) recordHistoryFinishedStatusLocked(task CronTask, scheduledAt time.Time, manual bool, startedAtMs, finishedAtMs int64, status string, errorMessage string) error {
	if startedAtMs <= 0 {
		startedAtMs = finishedAtMs
	}
	durationMs := finishedAtMs - startedAtMs
	if durationMs < 0 {
		durationMs = 0
	}
	entry := CronTaskRunHistoryEntry{
		RunID:         cronTaskRunID(task, scheduledAt),
		TaskID:        strings.TrimSpace(task.ID),
		Trigger:       cronHistoryTrigger(manual),
		ScheduledAtMs: scheduledAt.UnixMilli(),
		StartedAtMs:   startedAtMs,
		FinishedAtMs:  finishedAtMs,
		DurationMs:    durationMs,
		Status:        status,
		Error:         strings.TrimSpace(errorMessage),
		ThreadID:      cronTaskThreadID(task, scheduledAt),
		AgentID:       strings.TrimSpace(task.Target.AgentID),
	}
	return s.upsertHistoryEntryLocked(task.ID, entry)
}

func (s *cronManager) upsertHistoryEntryLocked(taskID string, entry CronTaskRunHistoryEntry) error {
	runs, err := s.loadHistoryLocked(taskID)
	if err != nil {
		return err
	}
	next := make([]CronTaskRunHistoryEntry, 0, len(runs)+1)
	next = append(next, entry)
	for _, existing := range runs {
		if existing.RunID == entry.RunID {
			continue
		}
		next = append(next, existing)
	}
	return s.saveHistoryLocked(taskID, next)
}

func cronHistoryTrigger(manual bool) string {
	if manual {
		return "manual"
	}
	return "scheduled"
}

func (s *cronManager) reconcileTasksLocked(now time.Time) {
	for taskID := range s.states {
		if _, ok := s.tasks[taskID]; !ok {
			delete(s.states, taskID)
		}
	}

	for _, task := range s.tasks {
		state := s.ensureTaskStateLocked(task)
		specHash, err := cronTaskSpecHash(task)
		if err != nil {
			state.LastError = err.Error()
			state.NextRunAtMs = 0
			continue
		}
		specChanged := state.SpecHash != specHash
		state.SpecHash = specHash

		if state.RunningAtMs > 0 && now.Sub(time.UnixMilli(state.RunningAtMs)) > cronStaleAfter {
			manual := state.RunNowAtMs > 0
			scheduledAtMs := state.NextRunAtMs
			if manual {
				scheduledAtMs = state.RunNowAtMs
			}
			if scheduledAtMs <= 0 {
				scheduledAtMs = state.RunningAtMs
			}
			if err := s.recordHistoryFinishedStatusLocked(task, time.UnixMilli(scheduledAtMs), manual, state.RunningAtMs, now.UnixMilli(), "failed", "cron run became stale after runtime restart"); err != nil {
				slog.Warn("failed to persist stale cron run history", "error", err, "taskID", task.ID)
			}
			state.RunningAtMs = 0
		}

		if specChanged {
			state.LastError = ""
			state.ConsecutiveErrors = 0
		}
		if !task.Enabled {
			state.NextRunAtMs = 0
			continue
		}
		if err := validateCronTask(task); err != nil {
			state.LastError = err.Error()
			state.NextRunAtMs = 0
			continue
		}
		if specChanged || state.NextRunAtMs <= 0 {
			nextRunAt, err := computeScheduledNextRun(task.Schedule, now)
			if err != nil {
				state.LastError = err.Error()
				state.NextRunAtMs = 0
			} else {
				state.NextRunAtMs = nextRunAt.UnixMilli()
			}
		}
	}
}

func (s *cronManager) reloadTasksIfChanged() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready {
		return nil
	}
	changed, err := s.loadTaskStoreIfChangedLocked()
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	s.reconcileTasksLocked(s.now())
	return s.saveStateLocked()
}

func (s *cronManager) ensureTaskStateLocked(task CronTask) *CronTaskState {
	state, ok := s.states[task.ID]
	if ok && state != nil {
		return state
	}
	state = &CronTaskState{TaskID: task.ID}
	s.states[task.ID] = state
	return state
}

func (s *cronManager) taskRecordsLocked() []CronTaskRecord {
	tasks := make([]CronTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	records := make([]CronTaskRecord, 0, len(tasks))
	for _, task := range tasks {
		records = append(records, s.taskRecordLocked(task))
	}
	return records
}

func (s *cronManager) taskRecordLocked(task CronTask) CronTaskRecord {
	return CronTaskRecord{
		Task:  task,
		State: cloneCronTaskState(s.states[task.ID]),
	}
}

func (s *cronManager) signalLoop() {
	select {
	case s.signalCh <- struct{}{}:
	default:
	}
}

func cloneCronTaskState(state *CronTaskState) *CronTaskState {
	if state == nil {
		return nil
	}
	cp := *state
	return &cp
}

func normalizeCronTask(task CronTask) CronTask {
	task.ID = strings.TrimSpace(task.ID)
	task.Name = strings.TrimSpace(task.Name)
	task.Description = strings.TrimSpace(task.Description)
	task.Schedule.Cron = strings.TrimSpace(task.Schedule.Cron)
	task.Schedule.Every = strings.TrimSpace(task.Schedule.Every)
	task.Schedule.Time = strings.TrimSpace(task.Schedule.Time)
	task.Target.Kind = strings.TrimSpace(task.Target.Kind)
	task.Target.AgentID = strings.TrimSpace(task.Target.AgentID)
	task.Target.CWD = strings.TrimSpace(task.Target.CWD)
	task.Payload.Kind = strings.TrimSpace(task.Payload.Kind)
	task.Payload.Text = strings.TrimSpace(task.Payload.Text)
	return task
}

func validateCronTask(task CronTask) error {
	task = normalizeCronTask(task)
	if task.ID != "" && !strings.HasPrefix(task.ID, "task-") {
		return fmt.Errorf("task id must start with task-")
	}
	if task.Name == "" {
		return fmt.Errorf("task name is required")
	}
	if err := validateCronTaskSchedule(task.Schedule); err != nil {
		return err
	}
	if task.Schedule.Every != "" {
		duration, err := time.ParseDuration(task.Schedule.Every)
		if err != nil {
			return err
		}
		if duration <= 0 {
			return fmt.Errorf("schedule.every must be greater than zero")
		}
	}
	if _, err := computeScheduledNextRun(task.Schedule, time.Now()); err != nil {
		return err
	}
	if task.Target.Kind != cronTargetKindAgent {
		return fmt.Errorf("target.kind must be %q", cronTargetKindAgent)
	}
	if task.Target.AgentID == "" {
		return fmt.Errorf("target.agentID is required")
	}
	if task.Target.CWD != "" && !filepath.IsAbs(task.Target.CWD) {
		return fmt.Errorf("target.cwd must be an absolute path")
	}
	if task.Payload.Kind != cronPayloadKindAgentTurn {
		return fmt.Errorf("payload.kind must be %q", cronPayloadKindAgentTurn)
	}
	if strings.TrimSpace(task.Payload.Text) == "" && len(task.Payload.Data) == 0 {
		return fmt.Errorf("payload.text or payload.data is required")
	}
	return nil
}

func cronTaskSpecHash(task CronTask) (string, error) {
	payload, err := json.Marshal(struct {
		Enabled  bool             `json:"enabled"`
		Schedule CronTaskSchedule `json:"schedule"`
		Target   CronTaskTarget   `json:"target"`
		Payload  CronTaskPayload  `json:"payload"`
	}{
		Enabled:  task.Enabled,
		Schedule: task.Schedule,
		Target:   task.Target,
		Payload:  task.Payload,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func buildCronTaskCall(task CronTask, scheduledAt time.Time) (op.Meta, op.Content, error) {
	task = normalizeCronTask(task)
	if strings.TrimSpace(task.ID) == "" {
		return nil, nil, fmt.Errorf("task id is required")
	}
	if strings.TrimSpace(task.Target.AgentID) == "" {
		return nil, nil, fmt.Errorf("target.agentID is required")
	}
	if strings.TrimSpace(task.Payload.Text) == "" && len(task.Payload.Data) == 0 {
		return nil, nil, fmt.Errorf("payload.text or payload.data is required")
	}

	runID := cronTaskRunID(task, scheduledAt)
	payloadData := cloneMap(task.Payload.Data)
	if shouldSanitizeScheduledCloudSyncPayload(payloadData) {
		payloadData = sanitizeScheduledCloudSyncPayload(payloadData)
		task.Payload.Text = scheduledCloudSyncTaskText()
	}
	workspaceID := stringFromMap(payloadData, "workspaceID")
	workspacePath := stringFromMap(payloadData, "workspacePath")
	if workspacePath == "" && strings.TrimSpace(task.Target.CWD) != "" {
		workspacePath = strings.TrimSpace(task.Target.CWD)
		payloadData["workspacePath"] = workspacePath
	}
	if workspaceID == "" {
		workspaceID = stringFromMap(payloadData, "sourceID")
	}
	trigger := stringFromMap(payloadData, "trigger")
	if trigger == "" {
		trigger = "scheduled"
		payloadData["trigger"] = trigger
	}
	payloadData["jobID"] = task.ID
	payloadData["taskID"] = task.ID
	payloadData["runID"] = runID
	payloadData["scheduledAt"] = scheduledAt.UTC().Format(time.RFC3339Nano)
	modelKey := strings.TrimSpace(stringFromMap(payloadData, "modelKey"))
	thinkingLevel := strings.TrimSpace(stringFromMap(payloadData, "thinkingLevel"))
	if sys := config.GetSystem(); sys != nil {
		if strings.TrimSpace(sys.HostID) != "" {
			payloadData["hostID"] = strings.TrimSpace(sys.HostID)
		}
		if strings.TrimSpace(sys.HostName) != "" {
			payloadData["hostName"] = strings.TrimSpace(sys.HostName)
		}
	}
	rawPayload, _ := json.Marshal(payloadData)
	meta := op.Meta{
		"agentID":     task.Target.AgentID,
		"threadID":    cronTaskThreadID(task, scheduledAt),
		"taskID":      task.ID,
		"jobID":       task.ID,
		"runID":       runID,
		"trigger":     trigger,
		"scheduledAt": scheduledAt.UTC().Format(time.RFC3339Nano),
		"payloadJSON": string(rawPayload),
	}
	if strings.TrimSpace(task.Target.CWD) != "" {
		meta["cwd"] = strings.TrimSpace(task.Target.CWD)
	}
	if modelKey != "" {
		meta["modelKey"] = modelKey
	}
	if thinkingLevel != "" {
		meta["thinkingLevel"] = thinkingLevel
	}
	if selectedSkillIDs, ok := stringSliceFromMap(payloadData, "selectedSkillIDs"); ok {
		meta["selectedSkillIDs"] = selectedSkillIDs
	}
	if selectedSkillContext, ok := metaMapFromMap(payloadData, "selectedSkillContext"); ok {
		meta["selectedSkillContext"] = selectedSkillContext
	}
	if workspaceID != "" {
		meta["workspaceID"] = workspaceID
	}
	if workspacePath != "" {
		meta["workspacePath"] = workspacePath
	}
	for _, key := range []string{"orgID", "repoURL", "branch", "locationKind", "hostID", "hostName"} {
		if value := stringFromMap(payloadData, key); value != "" {
			meta[key] = value
		}
	}
	contentText := strings.TrimSpace(task.Payload.Text)
	if contentText == "" {
		contentText = string(rawPayload)
	}
	return meta, &op.TextContent{Text: contentText}, nil
}

func cronTaskThreadID(task CronTask, scheduledAt time.Time) string {
	at := scheduledAt.UTC()
	seed := strings.TrimSpace(task.ID) + ":" + at.Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(seed))
	return fmt.Sprintf(
		"thread-%sZ-%s",
		at.Format("20060102T150405"),
		hex.EncodeToString(sum[:4]),
	)
}

func cronTaskRunID(task CronTask, scheduledAt time.Time) string {
	at := scheduledAt.UTC()
	seed := strings.TrimSpace(task.ID) + ":" + at.Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(seed))
	return fmt.Sprintf(
		"run-%s%03dZ-%s",
		at.Format("20060102T150405"),
		at.Nanosecond()/int(time.Millisecond),
		hex.EncodeToString(sum[:4]),
	)
}

func cronSafePathComponent(value string) string {
	safe := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\':
			return '_'
		}
		if r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(value))
	if safe == "" || safe == "." || safe == ".." {
		return "task"
	}
	return safe
}

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func boolFromMap(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func stringSliceFromMap(values map[string]any, key string) ([]string, bool) {
	if values == nil {
		return nil, false
	}
	value, ok := values[key]
	if !ok || value == nil {
		return nil, false
	}
	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out, len(out) > 0
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				out = append(out, text)
			}
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

func metaMapFromMap(values map[string]any, key string) (op.Meta, bool) {
	if values == nil {
		return nil, false
	}
	value, ok := values[key]
	if !ok || value == nil {
		return nil, false
	}
	switch typed := value.(type) {
	case op.Meta:
		return typed.Clone(), len(typed) > 0
	case map[string]any:
		return op.Meta(typed).Clone(), len(typed) > 0
	default:
		return nil, false
	}
}

func getDefaultCron() (*cronManager, error) {
	cronMu.Lock()
	defer cronMu.Unlock()
	if defaultCron == nil {
		return nil, fmt.Errorf("cron not started")
	}
	return defaultCron, nil
}

func cronJSONResult(v any) (*op.OpNodeResult, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
}

func decodeCronJSON[T any](req *op.OpNodeRequest, out *T) error {
	if req == nil || req.Params == nil || req.Params.Content == nil {
		return nil
	}
	jsonContent, ok := req.Params.Content.(*op.JsonContent)
	if !ok || jsonContent == nil {
		return fmt.Errorf("content must be json")
	}
	if len(jsonContent.Raw) == 0 {
		return nil
	}
	return json.Unmarshal(jsonContent.Raw, out)
}

func CronListHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	manager, err := getDefaultCron()
	if err != nil {
		return nil, err
	}
	result, err := manager.listTasks()
	if err != nil {
		return nil, err
	}
	return cronJSONResult(result)
}

func CronGetHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params cronTaskIDParams
	if err := decodeCronJSON(req, &params); err != nil {
		return nil, err
	}
	manager, err := getDefaultCron()
	if err != nil {
		return nil, err
	}
	result, err := manager.getTask(params.ID)
	if err != nil {
		return nil, err
	}
	return cronJSONResult(result)
}

func CronAddHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params cronTaskWriteParams
	if err := decodeCronJSON(req, &params); err != nil {
		return nil, err
	}
	manager, err := getDefaultCron()
	if err != nil {
		return nil, err
	}
	result, err := manager.addTask(params.Task)
	if err != nil {
		return nil, err
	}
	return cronJSONResult(result)
}

func CronUpsertHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params cronTaskWriteParams
	if err := decodeCronJSON(req, &params); err != nil {
		return nil, err
	}
	manager, err := getDefaultCron()
	if err != nil {
		return nil, err
	}
	result, err := manager.upsertTask(params.Task)
	if err != nil {
		return nil, err
	}
	return cronJSONResult(result)
}

func CronUpdateHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params cronTaskWriteParams
	if err := decodeCronJSON(req, &params); err != nil {
		return nil, err
	}
	manager, err := getDefaultCron()
	if err != nil {
		return nil, err
	}
	result, err := manager.updateTask(params.Task)
	if err != nil {
		return nil, err
	}
	return cronJSONResult(result)
}

func CronRemoveHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params cronTaskIDParams
	if err := decodeCronJSON(req, &params); err != nil {
		return nil, err
	}
	manager, err := getDefaultCron()
	if err != nil {
		return nil, err
	}
	removed, err := manager.removeTask(params.ID)
	if err != nil {
		return nil, err
	}
	return cronJSONResult(map[string]any{
		"id":      strings.TrimSpace(params.ID),
		"removed": removed,
	})
}

func CronRunHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params cronTaskIDParams
	if err := decodeCronJSON(req, &params); err != nil {
		return nil, err
	}
	manager, err := getDefaultCron()
	if err != nil {
		return nil, err
	}
	result, err := manager.runTaskNow(params.ID, params.Payload)
	if err != nil {
		return nil, err
	}
	return cronJSONResult(result)
}

func CronHistoryHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params cronTaskHistoryParams
	if err := decodeCronJSON(req, &params); err != nil {
		return nil, err
	}
	manager, err := getDefaultCron()
	if err != nil {
		return nil, err
	}
	result, err := manager.listHistory(params.ID, params.Limit)
	if err != nil {
		return nil, err
	}
	return cronJSONResult(result)
}
