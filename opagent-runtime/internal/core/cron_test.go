package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

func TestComputeScheduledNextRun(t *testing.T) {
	now := time.Date(2026, 3, 18, 8, 30, 0, 0, time.Local)

	nextEvery, err := computeScheduledNextRun(CronTaskSchedule{Every: "1h"}, now)
	if err != nil {
		t.Fatalf("computeScheduledNextRun(every): %v", err)
	}
	if want := now.Add(time.Hour); !nextEvery.Equal(want) {
		t.Fatalf("every next = %v, want %v", nextEvery, want)
	}

	nextTime, err := computeScheduledNextRun(CronTaskSchedule{Time: "09:00"}, now)
	if err != nil {
		t.Fatalf("computeScheduledNextRun(time): %v", err)
	}
	if want := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local); !nextTime.Equal(want) {
		t.Fatalf("time next = %v, want %v", nextTime, want)
	}

	nextCron, err := computeScheduledNextRun(CronTaskSchedule{Cron: "0 9 * * *"}, now)
	if err != nil {
		t.Fatalf("computeScheduledNextRun(cron): %v", err)
	}
	if want := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local); !nextCron.Equal(want) {
		t.Fatalf("cron next = %v, want %v", nextCron, want)
	}
}

func TestBuildCronTaskCall(t *testing.T) {
	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "host-test", HostName: "host-name", Env: op.EnvLocal})
	task := CronTask{
		ID:      "task-abcd",
		Name:    "Daily check",
		Enabled: true,
		Target: CronTaskTarget{
			Kind:    cronTargetKindAgent,
			AgentID: "agent-abcd",
			CWD:     "/tmp/workspace",
		},
		Payload: CronTaskPayload{
			Kind: cronPayloadKindAgentTurn,
			Data: map[string]any{
				"workspaceID":   "workspace-1",
				"workspacePath": "/tmp/workspace",
				"repoURL":       "https://github.com/example/repo.git",
				"locationKind":  "local",
				"selectedSkillIDs": []any{
					"skill-test",
				},
				"selectedSkillContext": map[string]any{
					"managedKind": "test-managed-kind",
				},
			},
		},
	}

	scheduledAt := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
	runID := cronTaskRunID(task, scheduledAt)
	runIDPrefix := "run-20260318T090000000Z-"
	if !strings.HasPrefix(runID, runIDPrefix) || len(strings.TrimPrefix(runID, runIDPrefix)) != 8 {
		t.Fatalf("runID = %q, want %s plus 8 hex chars", runID, runIDPrefix)
	}
	meta, content, err := buildCronTaskCall(task, scheduledAt)
	if err != nil {
		t.Fatalf("buildCronTaskCall(): %v", err)
	}
	threadIDPrefix := "thread-20260318T090000Z-"
	if got := meta["threadID"]; !strings.HasPrefix(fmt.Sprint(got), threadIDPrefix) || len(strings.TrimPrefix(fmt.Sprint(got), threadIDPrefix)) != 8 {
		t.Fatalf("threadID = %v, want %s plus 8 hex chars", got, threadIDPrefix)
	}
	if got := meta["chatPath"]; got != nil {
		t.Fatalf("chatPath = %v, want nil for thread-only cron run", got)
	}
	if got := meta["taskID"]; got != "task-abcd" {
		t.Fatalf("taskID = %v, want task-abcd", got)
	}
	text, ok := content.(*op.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *op.TextContent", content)
	}
	if !strings.Contains(text.Text, `"workspaceID":"workspace-1"`) {
		t.Fatalf("content text = %q, want JSON sync payload", text.Text)
	}
	if meta["workspaceID"] != "workspace-1" {
		t.Fatalf("workspaceID meta = %v, want workspace-1", meta["workspaceID"])
	}
	if meta["hostID"] != "host-test" {
		t.Fatalf("hostID meta = %v, want host-test", meta["hostID"])
	}
	selected, ok := meta["selectedSkillIDs"].([]string)
	if !ok || len(selected) != 1 || selected[0] != "skill-test" {
		t.Fatalf("selectedSkillIDs meta = %#v", meta["selectedSkillIDs"])
	}
	selectedContext, ok := meta["selectedSkillContext"].(op.Meta)
	if !ok || selectedContext["managedKind"] != "test-managed-kind" {
		t.Fatalf("selectedSkillContext meta = %#v", meta["selectedSkillContext"])
	}
}

func TestBuildCronTaskCallSanitizesManagedCloudSyncScheduledPayload(t *testing.T) {
	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "host-test", Env: op.EnvLocal})
	staleWorkspace := map[string]any{
		"workspaceID": "ws-old-account",
		"name":        "openbrain",
		"path":        "/Users/example/code/openbrain",
	}
	task := testCronTask(baseDir)
	task.Payload.Text = "Old prompt\n\nWorkspaces JSON:\n[{\"workspaceID\":\"ws-old-account\"}]"
	task.Payload.Data = map[string]any{
		"managedKind":       cronManagedKindCloudSync,
		"workspaces":        []any{staleWorkspace},
		"workspaceSnapshot": []any{staleWorkspace},
		"selectedSkillContext": map[string]any{
			"managedKind":       cronManagedKindCloudSync,
			"workspaces":        []any{staleWorkspace},
			"workspaceSnapshot": []any{staleWorkspace},
		},
	}

	meta, content, err := buildCronTaskCall(task, time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildCronTaskCall(): %v", err)
	}
	text, ok := content.(*op.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *op.TextContent", content)
	}
	if !strings.Contains(text.Text, "helper preflight") {
		t.Fatalf("content text = %q, want helper preflight guidance", text.Text)
	}
	if strings.Contains(text.Text, "ws-old-account") || strings.Contains(text.Text, "Workspaces JSON") {
		t.Fatalf("content text = %q, want stale workspace omitted", text.Text)
	}
	if got := meta["workspaceID"]; got != nil {
		t.Fatalf("workspaceID meta = %v, want nil after scheduled cloud sync sanitization", got)
	}
	rawPayload := metaString(meta, "payloadJSON")
	if strings.Contains(rawPayload, "ws-old-account") {
		t.Fatalf("payloadJSON = %q, want stale workspace omitted", rawPayload)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		t.Fatalf("payloadJSON unmarshal: %v", err)
	}
	if got, ok := payload["workspaces"].([]any); !ok || len(got) != 0 {
		t.Fatalf("payload workspaces = %#v, want empty array", payload["workspaces"])
	}
	if got, ok := payload["workspaceSnapshot"].([]any); !ok || len(got) != 0 {
		t.Fatalf("payload workspaceSnapshot = %#v, want empty array", payload["workspaceSnapshot"])
	}
	selectedContext, ok := meta["selectedSkillContext"].(op.Meta)
	if !ok {
		t.Fatalf("selectedSkillContext meta = %#v, want op.Meta", meta["selectedSkillContext"])
	}
	if got, ok := selectedContext["workspaces"].([]any); !ok || len(got) != 0 {
		t.Fatalf("selectedSkillContext workspaces = %#v, want empty array", selectedContext["workspaces"])
	}
	if got, ok := selectedContext["workspaceSnapshot"].([]any); !ok || len(got) != 0 {
		t.Fatalf("selectedSkillContext workspaceSnapshot = %#v, want empty array", selectedContext["workspaceSnapshot"])
	}
}

func TestBuildCronTaskCallKeepsManagedCloudSyncManualPayload(t *testing.T) {
	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "host-test", Env: op.EnvLocal})
	requestedWorkspace := map[string]any{
		"workspaceID": "ws-current",
		"name":        "note",
		"path":        "/Users/example/note",
	}
	task := testCronTask(baseDir)
	task.Payload.Text = "Manual Sync Now for ws-current"
	task.Payload.Data = map[string]any{
		"managedKind":               cronManagedKindCloudSync,
		"manualRunIncludesDisabled": true,
		"workspaces":                []any{requestedWorkspace},
		"workspaceSnapshot":         []any{requestedWorkspace},
		"requestedWorkspace":        requestedWorkspace,
		"selectedSkillContext": map[string]any{
			"managedKind":       cronManagedKindCloudSync,
			"manual":            true,
			"workspaces":        []any{requestedWorkspace},
			"workspaceSnapshot": []any{requestedWorkspace},
		},
	}

	meta, content, err := buildCronTaskCall(task, time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildCronTaskCall(): %v", err)
	}
	text, ok := content.(*op.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *op.TextContent", content)
	}
	if !strings.Contains(text.Text, "ws-current") {
		t.Fatalf("content text = %q, want manual workspace preserved", text.Text)
	}
	if !strings.Contains(metaString(meta, "payloadJSON"), "ws-current") {
		t.Fatalf("payloadJSON = %q, want manual workspace preserved", metaString(meta, "payloadJSON"))
	}
}

func TestBuildCronTaskCallPromotesExplicitModelKey(t *testing.T) {
	task := testCronTask(t.TempDir())
	scheduledAt := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)

	meta, content, err := buildCronTaskCall(task, scheduledAt)
	if err != nil {
		t.Fatalf("buildCronTaskCall(): %v", err)
	}
	if got := metaString(meta, "modelKey"); got != "test:model" {
		t.Fatalf("meta modelKey = %q, want test:model", got)
	}
	text, ok := content.(*op.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *op.TextContent", content)
	}
	if text.Text != task.Payload.Text {
		t.Fatalf("content text = %q, want payload text", text.Text)
	}
}

func TestBuildCronTaskCallDoesNotInjectCloudSyncDefaultChatModel(t *testing.T) {
	baseDir := t.TempDir()
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "host-test", Env: op.EnvLocal})
	writeCronTestModelsJSON(t, baseDir, "local-chat:auto-chat", "high")
	if _, err := config.LoadLocalUserProfile(); err != nil {
		t.Fatalf("LoadLocalUserProfile(): %v", err)
	}

	task := testCronTask(baseDir)
	task.Payload.Data = map[string]any{
		"managedKind": cronManagedKindCloudSync,
	}
	scheduledAt := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)

	meta, _, err := buildCronTaskCall(task, scheduledAt)
	if err != nil {
		t.Fatalf("buildCronTaskCall(): %v", err)
	}
	if got := metaString(meta, "modelKey"); got != "" {
		t.Fatalf("meta modelKey = %q, want empty without explicit payload.data.modelKey", got)
	}
	if got := metaString(meta, "thinkingLevel"); got != "" {
		t.Fatalf("meta thinkingLevel = %q, want empty without explicit payload.data.thinkingLevel", got)
	}
	if strings.Contains(metaString(meta, "payloadJSON"), "local-chat:auto-chat") || strings.Contains(metaString(meta, "payloadJSON"), `"thinkingLevel":"high"`) {
		t.Fatalf("payloadJSON = %q, want no injected default chat model", metaString(meta, "payloadJSON"))
	}
}

func TestEnsureAgentCallSessionCreatesFixedThread(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, agentBaseDir, agentFilePath := createTestAgent(t, baseDir, "agents/a")

	node := &op.OpNode{
		ID:   agentID,
		URI:  op.PathToURI(agentFilePath),
		Cwd:  agentBaseDir,
		Kind: string(op.NodeKindAgent),
		Meta: &op.AgentMeta{Name: "Agent A"},
	}
	meta := op.Meta{
		"threadID": "thread-task-agent-session",
	}

	if err := ensureAgentCallSession(node, meta); err != nil {
		t.Fatalf("ensureAgentCallSession(): %v", err)
	}
	threadMeta, err := getThreadMeta("thread-task-agent-session", node.ID)
	if err != nil {
		t.Fatalf("getThreadMeta(): %v", err)
	}
	if threadMeta.ThreadID != "thread-task-agent-session" {
		t.Fatalf("threadID = %q, want %q", threadMeta.ThreadID, "thread-task-agent-session")
	}
	wantCWD := filepath.Join(baseDir, "workspace")
	if threadMeta.CWD != wantCWD {
		t.Fatalf("cwd = %q, want %q", threadMeta.CWD, wantCWD)
	}
	if threadMeta.ChatPath == "" {
		t.Fatalf("chatPath = %q, want projection path", threadMeta.ChatPath)
	}
	if _, err := os.Stat(threadMeta.ChatPath); err != nil {
		t.Fatalf("projection stat: %v", err)
	}
}

func TestEnsureAgentCallSessionDoesNotCreateCronChatFile(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, agentBaseDir, agentFilePath := createTestAgent(t, baseDir, "agents/a")

	node := &op.OpNode{
		ID:   agentID,
		URI:  op.PathToURI(agentFilePath),
		Cwd:  agentBaseDir,
		Kind: string(op.NodeKindAgent),
		Meta: &op.AgentMeta{Name: "Agent A"},
	}
	chatDir := filepath.Join(baseDir, "cron", "chats", "task-check")
	newChatPath := filepath.Join(chatDir, "run-20260318T100000000Z-ffffffff.md")
	meta := op.Meta{
		"threadID": "thread-20260318T100000Z-ffffffff",
		"agentID":  agentID,
		"chatPath": newChatPath,
	}
	if err := ensureAgentCallSession(node, meta); err != nil {
		t.Fatalf("ensureAgentCallSession(): %v", err)
	}
	if _, err := os.Stat(newChatPath); err != nil {
		t.Fatalf("cron chat file should be created, stat err=%v", err)
	}
}

func TestCronTaskStoreLoadSaveSeparatesState(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local)

	manager, err := newCronManager(context.Background(), baseDir, func() time.Time { return now }, func(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("newCronManager(): %v", err)
	}
	defer manager.stop()

	if _, err := manager.addTask(testCronTask(baseDir)); err != nil {
		t.Fatalf("addTask(): %v", err)
	}

	taskRaw, err := os.ReadFile(filepath.Join(baseDir, "cron", cronTaskStoreFileName))
	if err != nil {
		t.Fatalf("ReadFile(tasks): %v", err)
	}
	if strings.Contains(string(taskRaw), "nextRunAtMs") || strings.Contains(string(taskRaw), "lastRunAtMs") {
		t.Fatalf("tasks.json contains runtime state: %s", taskRaw)
	}

	stateRaw, err := os.ReadFile(filepath.Join(baseDir, "run", "cron", cronStateFileName))
	if err != nil {
		t.Fatalf("ReadFile(state): %v", err)
	}
	if !strings.Contains(string(stateRaw), "nextRunAtMs") {
		t.Fatalf("state file missing cron state: %s", stateRaw)
	}
}

func TestCronUpsertAddsAndReplacesTask(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local)
	manager, err := newCronManager(context.Background(), baseDir, func() time.Time { return now }, func(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("newCronManager(): %v", err)
	}
	defer manager.stop()

	task := testCronTask(baseDir)
	first, err := manager.upsertTask(task)
	if err != nil {
		t.Fatalf("upsertTask(add): %v", err)
	}
	if first.Task.ID != task.ID {
		t.Fatalf("upsert add id = %q, want %q", first.Task.ID, task.ID)
	}
	if first.Task.CreatedAtMs == 0 {
		t.Fatal("upsert add CreatedAtMs = 0, want timestamp")
	}

	updated := first.Task
	updated.Name = "Updated check"
	updated.Schedule.Every = "30m"
	now = now.Add(time.Minute)
	second, err := manager.upsertTask(updated)
	if err != nil {
		t.Fatalf("upsertTask(replace): %v", err)
	}
	if second.Task.Name != "Updated check" {
		t.Fatalf("upsert replace name = %q, want Updated check", second.Task.Name)
	}
	if second.Task.CreatedAtMs != first.Task.CreatedAtMs {
		t.Fatalf("CreatedAtMs changed: %d != %d", second.Task.CreatedAtMs, first.Task.CreatedAtMs)
	}
	if second.Task.UpdatedAtMs <= first.Task.UpdatedAtMs {
		t.Fatalf("UpdatedAtMs = %d, want > %d", second.Task.UpdatedAtMs, first.Task.UpdatedAtMs)
	}
}

func TestCronTaskStoreAllowsComments(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local)
	taskPath := filepath.Join(baseDir, "cron", cronTaskStoreFileName)
	if err := os.MkdirAll(filepath.Dir(taskPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	raw := `{
  // Task definitions live here.
  "version": 1,
  "tasks": [
    {
      "id": "task-jsonc",
      "name": "JSONC task",
      "enabled": true,
      /* Use exactly one schedule mode. */
      "schedule": { "every": "1h" },
      "target": {
        "kind": "agent",
        "agentID": "agent-abcd",
        "cwd": "` + filepath.ToSlash(filepath.Join(baseDir, "workspace", "project")) + `"
      },
      "payload": {
        "kind": "agentTurn",
        "text": "Check https://example.com/docs // keep this text"
      }
    }
  ]
}
`
	if err := os.WriteFile(taskPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	manager, err := newCronManager(context.Background(), baseDir, func() time.Time { return now }, func(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("newCronManager(): %v", err)
	}
	defer manager.stop()

	task, ok := manager.tasks["task-jsonc"]
	if !ok {
		t.Fatal("task-jsonc was not loaded")
	}
	if task.Payload.Text != "Check https://example.com/docs // keep this text" {
		t.Fatalf("payload text = %q", task.Payload.Text)
	}
}

func TestCronRunDueExecutesCronTask(t *testing.T) {
	cache.Flush()
	syncCronTestModel()
	baseDir := t.TempDir()
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local)
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal})

	task := testCronTask(baseDir)
	writeTaskStore(t, baseDir, []CronTask{task})

	var (
		called  bool
		gotMeta op.Meta
		gotText string
		gotCWD  string
	)
	manager, err := newCronManager(context.Background(), baseDir, func() time.Time { return now }, func(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		called = true
		gotCWD = node.Cwd
		gotMeta = meta.Clone()
		text, ok := content.(*op.TextContent)
		if !ok {
			t.Fatalf("content type = %T, want *op.TextContent", content)
		}
		gotText = text.Text
		return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("newCronManager(): %v", err)
	}
	defer manager.stop()

	cache.SetValue(task.Target.AgentID, cache.PrefixNode, op.OpNode{
		ID:   task.Target.AgentID,
		URI:  op.PathToURI(filepath.Join(baseDir, "agents", "a", ".agent", "AGENT.md")),
		Cwd:  filepath.Join(baseDir, "agents", "a"),
		Kind: string(op.NodeKindAgent),
		Meta: &op.AgentMeta{Name: "Agent A"},
	}, cache.NoExpiration)

	manager.mu.Lock()
	state := manager.states[task.ID]
	if state == nil {
		manager.mu.Unlock()
		t.Fatalf("cron state missing for %s", task.ID)
	}
	scheduledAt := now.Add(-time.Minute)
	state.NextRunAtMs = scheduledAt.UnixMilli()
	manager.mu.Unlock()

	manager.runDue()

	if !called {
		t.Fatal("executor was not called")
	}
	if gotText != task.Payload.Text {
		t.Fatalf("trigger text = %q, want %q", gotText, task.Payload.Text)
	}
	if gotMeta["threadID"] != cronTaskThreadID(task, scheduledAt) {
		t.Fatalf("threadID = %v, want %q", gotMeta["threadID"], cronTaskThreadID(task, scheduledAt))
	}
	if gotMeta["taskID"] != task.ID {
		t.Fatalf("taskID = %v, want %q", gotMeta["taskID"], task.ID)
	}
	if gotMeta["modelKey"] != "test:model" {
		t.Fatalf("modelKey = %v, want test:model", gotMeta["modelKey"])
	}
	if gotMeta["cwd"] != task.Target.CWD {
		t.Fatalf("cwd meta = %v, want %q", gotMeta["cwd"], task.Target.CWD)
	}
	wantNodeCWD := filepath.Join(baseDir, "agents", "a")
	if gotCWD != wantNodeCWD {
		t.Fatalf("node cwd = %q, want %q", gotCWD, wantNodeCWD)
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.states[task.ID].LastRunAtMs == 0 {
		t.Fatal("LastRunAtMs = 0, want non-zero")
	}
	if manager.states[task.ID].NextRunAtMs <= now.UnixMilli() {
		t.Fatalf("NextRunAtMs = %d, want > %d", manager.states[task.ID].NextRunAtMs, now.UnixMilli())
	}
}

func TestCronRunDueWithoutModelPublishesConfigMessage(t *testing.T) {
	cache.Flush()
	baseDir := t.TempDir()
	resetMessageTestState(baseDir)
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local)
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal})

	task := testCronTask(baseDir)
	task.Payload.Data = nil
	writeTaskStore(t, baseDir, []CronTask{task})

	called := false
	manager, err := newCronManager(context.Background(), baseDir, func() time.Time { return now }, func(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		called = true
		return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("newCronManager(): %v", err)
	}
	defer manager.stop()

	cache.SetValue(task.Target.AgentID, cache.PrefixNode, op.OpNode{
		ID:   task.Target.AgentID,
		URI:  op.PathToURI(filepath.Join(baseDir, "agents", "a", ".agent", "AGENT.md")),
		Cwd:  filepath.Join(baseDir, "agents", "a"),
		Kind: string(op.NodeKindAgent),
		Meta: &op.AgentMeta{Name: "Agent A"},
	}, cache.NoExpiration)

	manager.mu.Lock()
	scheduledAt := now.Add(-time.Minute)
	manager.states[task.ID].NextRunAtMs = scheduledAt.UnixMilli()
	manager.mu.Unlock()

	manager.runDue()
	if called {
		t.Fatal("executor was called, want model preflight to stop the task")
	}

	manager.mu.Lock()
	state := manager.states[task.ID]
	manager.mu.Unlock()
	if state == nil || !strings.Contains(state.LastError, "payload.data.modelKey") {
		t.Fatalf("LastError = %#v, want payload.data.modelKey", state)
	}

	read, err := defaultMessageStore.read(op.MessageReadParams{ThreadID: cronTaskThreadID(task, scheduledAt)})
	if err != nil {
		t.Fatalf("read message store: %v", err)
	}
	if len(read.Messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(read.Messages))
	}
	record := read.Messages[0]
	if record.Sender != op.MessageSenderSystem || record.Status != op.MessageStatusOpen {
		t.Fatalf("message sender/status = %s/%s, want system/open", record.Sender, record.Status)
	}
	if !strings.Contains(record.Body, "payload.data.modelKey") {
		t.Fatalf("message body = %q, want payload.data.modelKey", record.Body)
	}
}

func TestCronRunNowExecutesDisabledTask(t *testing.T) {
	cache.Flush()
	syncCronTestModel()
	baseDir := t.TempDir()
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local)
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal})

	task := testCronTask(baseDir)
	task.Enabled = false
	writeTaskStore(t, baseDir, []CronTask{task})

	called := false
	manager, err := newCronManager(context.Background(), baseDir, func() time.Time { return now }, func(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		called = true
		return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("newCronManager(): %v", err)
	}
	defer manager.stop()

	cache.SetValue(task.Target.AgentID, cache.PrefixNode, op.OpNode{
		ID:   task.Target.AgentID,
		URI:  op.PathToURI(filepath.Join(baseDir, "agents", "a", ".agent", "AGENT.md")),
		Cwd:  filepath.Join(baseDir, "agents", "a"),
		Kind: string(op.NodeKindAgent),
		Meta: &op.AgentMeta{Name: "Agent A"},
	}, cache.NoExpiration)

	if _, err := manager.runTaskNow(task.ID, nil); err != nil {
		t.Fatalf("runTaskNow(): %v", err)
	}
	manager.runDue()
	if !called {
		t.Fatal("executor was not called")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	state := manager.states[task.ID]
	if state == nil {
		t.Fatal("cron state missing")
	}
	if state.RunNowAtMs != 0 {
		t.Fatalf("RunNowAtMs = %d, want 0", state.RunNowAtMs)
	}
	if state.NextRunAtMs != 0 {
		t.Fatalf("NextRunAtMs = %d, want 0 for disabled task", state.NextRunAtMs)
	}
}

func TestCronRunNowPayloadOverrideIsOneShot(t *testing.T) {
	cache.Flush()
	syncCronTestModel()
	baseDir := t.TempDir()
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local)
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal})

	task := testCronTask(baseDir)
	writeTaskStore(t, baseDir, []CronTask{task})

	var gotText string
	manager, err := newCronManager(context.Background(), baseDir, func() time.Time { return now }, func(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		text, ok := content.(*op.TextContent)
		if !ok {
			t.Fatalf("content type = %T, want *op.TextContent", content)
		}
		gotText = text.Text
		return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("newCronManager(): %v", err)
	}
	defer manager.stop()

	cache.SetValue(task.Target.AgentID, cache.PrefixNode, op.OpNode{
		ID:   task.Target.AgentID,
		URI:  op.PathToURI(filepath.Join(baseDir, "agents", "a", ".agent", "AGENT.md")),
		Cwd:  filepath.Join(baseDir, "agents", "a"),
		Kind: string(op.NodeKindAgent),
		Meta: &op.AgentMeta{Name: "Agent A"},
	}, cache.NoExpiration)

	override := CronTaskPayload{
		Kind: cronPayloadKindAgentTurn,
		Text: "Manual workspace sync",
		Data: map[string]any{"modelKey": "test:model"},
	}
	if _, err := manager.runTaskNow(task.ID, &override); err != nil {
		t.Fatalf("runTaskNow(): %v", err)
	}
	manager.runDue()
	if gotText != "Manual workspace sync" {
		t.Fatalf("text = %q, want override text", gotText)
	}

	manager.mu.Lock()
	state := manager.states[task.ID]
	persisted := manager.tasks[task.ID]
	manager.mu.Unlock()
	if state == nil || state.RunNowPayload != nil {
		t.Fatalf("RunNowPayload = %#v, want cleared", state)
	}
	if persisted.Payload.Text != task.Payload.Text {
		t.Fatalf("persisted payload text = %q, want %q", persisted.Payload.Text, task.Payload.Text)
	}
}

func TestCronReconcileClearsStaleRunning(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local)
	task := testCronTask(baseDir)
	writeTaskStore(t, baseDir, []CronTask{task})

	manager, err := newCronManager(context.Background(), baseDir, func() time.Time { return now }, func(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("newCronManager(): %v", err)
	}
	defer manager.stop()

	manager.mu.Lock()
	manager.states[task.ID].RunningAtMs = now.Add(-cronStaleAfter - time.Minute).UnixMilli()
	manager.mu.Unlock()

	if err := manager.markNodeCacheReadyAndReload(); err != nil {
		t.Fatalf("markNodeCacheReadyAndReload(): %v", err)
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.states[task.ID].RunningAtMs != 0 {
		t.Fatalf("RunningAtMs = %d, want 0", manager.states[task.ID].RunningAtMs)
	}
}

func TestCronHistoryRecordsRunsAndKeepsNewestFirst(t *testing.T) {
	cache.Flush()
	syncCronTestModel()
	baseDir := t.TempDir()
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local)
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir, HostID: "test-host", Env: op.EnvLocal})

	task := testCronTask(baseDir)
	writeTaskStore(t, baseDir, []CronTask{task})

	execCount := 0
	manager, err := newCronManager(context.Background(), baseDir, func() time.Time { return now }, func(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		execCount++
		if execCount == 1 {
			return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
		}
		return nil, context.Canceled
	})
	if err != nil {
		t.Fatalf("newCronManager(): %v", err)
	}
	defer manager.stop()

	cache.SetValue(task.Target.AgentID, cache.PrefixNode, op.OpNode{
		ID:   task.Target.AgentID,
		URI:  op.PathToURI(filepath.Join(baseDir, "agents", "a", ".agent", "AGENT.md")),
		Cwd:  filepath.Join(baseDir, "agents", "a"),
		Kind: string(op.NodeKindAgent),
		Meta: &op.AgentMeta{Name: "Agent A"},
	}, cache.NoExpiration)

	manager.mu.Lock()
	manager.states[task.ID].NextRunAtMs = now.Add(-time.Minute).UnixMilli()
	manager.mu.Unlock()
	manager.runDue()

	now = now.Add(2 * time.Minute)
	manager.mu.Lock()
	manager.states[task.ID].NextRunAtMs = now.Add(-time.Minute).UnixMilli()
	manager.mu.Unlock()
	manager.runDue()

	history, err := manager.listHistory(task.ID, cronHistoryLimit)
	if err != nil {
		t.Fatalf("listHistory(): %v", err)
	}
	if len(history.Runs) != 2 {
		t.Fatalf("history len = %d, want 2", len(history.Runs))
	}
	if history.Runs[0].Status != "failed" || history.Runs[1].Status != "success" {
		t.Fatalf("statuses = %#v, want newest-first failed then success", []string{history.Runs[0].Status, history.Runs[1].Status})
	}
	if history.Runs[0].Trigger != "scheduled" || history.Runs[1].Trigger != "scheduled" {
		t.Fatalf("triggers = %#v, want scheduled", []string{history.Runs[0].Trigger, history.Runs[1].Trigger})
	}
	if history.Runs[0].RunID == history.Runs[1].RunID {
		t.Fatal("run IDs should differ across runs")
	}
	if history.Runs[0].ThreadID == history.Runs[1].ThreadID {
		t.Fatal("thread IDs should differ across runs")
	}
	if history.Runs[0].ChatPath != "" || history.Runs[1].ChatPath != "" {
		t.Fatalf("chat paths = %q, %q; want empty for thread-only cron runs", history.Runs[0].ChatPath, history.Runs[1].ChatPath)
	}
	if history.Runs[0].FinishedAtMs == 0 || history.Runs[1].FinishedAtMs == 0 {
		t.Fatal("finished timestamps must be recorded")
	}
}

func TestCronHistoryLimitAndDeleteCleanup(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.Local)
	task := testCronTask(baseDir)
	writeTaskStore(t, baseDir, []CronTask{task})
	manager, err := newCronManager(context.Background(), baseDir, func() time.Time { return now }, func(ctx context.Context, node *op.OpNode, meta op.Meta, content op.Content) (*op.OpNodeResult, error) {
		return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("newCronManager(): %v", err)
	}
	defer manager.stop()

	for i := 0; i < 101; i++ {
		scheduledAt := now.Add(time.Duration(i) * time.Minute)
		if err := manager.recordHistoryFinishedStatusLocked(task, scheduledAt, false, scheduledAt.UnixMilli(), scheduledAt.Add(time.Second).UnixMilli(), "success", ""); err != nil {
			t.Fatalf("recordHistoryFinishedStatusLocked(%d): %v", i, err)
		}
	}

	history, err := manager.listHistory(task.ID, 0)
	if err != nil {
		t.Fatalf("listHistory(): %v", err)
	}
	if len(history.Runs) != 99 {
		t.Fatalf("history len = %d, want 99", len(history.Runs))
	}

	raw, err := os.ReadFile(filepath.Join(baseDir, "cron", "history", cronHistoryFileName(task.ID)))
	if err != nil {
		t.Fatalf("ReadFile(history): %v", err)
	}
	if count := strings.Count(string(raw), `"runID"`); count != 99 {
		t.Fatalf("history file run count = %d, want 99", count)
	}

	if removed, err := manager.removeTask(task.ID); err != nil || !removed {
		t.Fatalf("removeTask(): removed=%t err=%v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "cron", "history", cronHistoryFileName(task.ID))); !os.IsNotExist(err) {
		t.Fatalf("history file should be removed, stat err=%v", err)
	}
}

func testCronTask(baseDir string) CronTask {
	return CronTask{
		ID:      "task-check",
		Name:    "Daily check",
		Enabled: true,
		Schedule: CronTaskSchedule{
			Every: "1h",
		},
		Target: CronTaskTarget{
			Kind:    cronTargetKindAgent,
			AgentID: "agent-abcd",
			CWD:     filepath.Join(baseDir, "workspace", "project"),
		},
		Payload: CronTaskPayload{
			Kind: cronPayloadKindAgentTurn,
			Text: "Check the project status.",
			Data: map[string]any{
				"modelKey": "test:model",
			},
		},
	}
}

func syncCronTestModel() {
	config.SyncModelCache([]op.ModelConfig{{
		Key:      "test:model",
		ID:       "model",
		Name:     "model",
		Provider: "test",
		API:      "openai-responses",
		APIKey:   "test-key",
		BaseURL:  "https://api.example.test/v1",
		Enabled:  true,
	}})
}

func writeCronTestModelsJSON(t *testing.T, baseDir string, defaultChatModelKey string, defaultChatThinkingLevel string) {
	t.Helper()
	userDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(user config): %v", err)
	}
	raw := fmt.Sprintf(`{
  "version": 5,
  "defaultModelKey": "local-chat:auto-chat",
  "strategies": {
    "auto": {
      "defaultChatModelID": %q,
      "defaultChatThinkingLevel": %q
    }
  },
  "providers": {
    "local-chat": {
      "api": "openai-responses",
      "baseUrl": "https://api.example.test/v1",
      "apiKey": "test-key",
      "models": [
        { "id": "auto-chat", "enabled": true }
      ]
    }
  }
}
`, defaultChatModelKey, defaultChatThinkingLevel)
	if err := os.WriteFile(filepath.Join(userDir, "models.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
}

func writeTaskStore(t *testing.T, baseDir string, tasks []CronTask) {
	t.Helper()
	path := filepath.Join(baseDir, "cron", cronTaskStoreFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	raw, err := json.MarshalIndent(CronTaskStoreFile{
		Version: cronTaskStoreVersion,
		Tasks:   tasks,
	}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(): %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
}
