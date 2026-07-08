package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

func TestCreateThreadStoresUnderStableBaseThreadDir(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, agentBaseDir, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}

	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "hello.md"),
		Title:    "hello",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}

	sessionPath := threadFilePathInRoot(threadStorageRootDir(baseDir), result.ThreadID)
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected thread file at %s: %v", sessionPath, err)
	}
	if _, err := os.Stat(threadFilePathInRoot(filepath.Join(agentBaseDir, "thread"), result.ThreadID)); !os.IsNotExist(err) {
		t.Fatalf("unexpected thread file under agent install thread root, got err=%v", err)
	}
}

func TestGetThreadMetaByThreadIDFindsFlatSessionAfterCacheReset(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "workspace/proj")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "hello.md")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "hello",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}

	defaultThreadStore = &threadStore{
		byThread:  make(map[string]*threadRecord),
		threadMux: make(map[string]*sync.Mutex),
	}

	meta, err := getThreadMeta(result.ThreadID, "")
	if err != nil {
		t.Fatalf("getThreadMeta by threadID: %v", err)
	}
	if meta.ThreadID != result.ThreadID {
		t.Fatalf("meta.ThreadID = %q, want %q", meta.ThreadID, result.ThreadID)
	}
}

func TestGetThreadMetaByThreadIDIgnoresStaleAgentID(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "workspace/proj")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "stale-agent.md"),
		Title:    "stale-agent",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}

	meta, err := getThreadMeta(result.ThreadID, "agent-gbrain")
	if err != nil {
		t.Fatalf("getThreadMeta with stale agentID: %v", err)
	}
	if meta.ThreadID != result.ThreadID {
		t.Fatalf("meta.ThreadID = %q, want %q", meta.ThreadID, result.ThreadID)
	}
	if meta.AgentID != agentID {
		t.Fatalf("meta.AgentID = %q, want legacy header agent %q", meta.AgentID, agentID)
	}
}

func TestGetThreadMetaSurvivesAgentInstallDirRefresh(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, agentBaseDir, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "hello.md")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "hello",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	if err := os.RemoveAll(agentBaseDir); err != nil {
		t.Fatalf("remove agent install dir: %v", err)
	}

	defaultThreadStore = &threadStore{
		byThread:  make(map[string]*threadRecord),
		threadMux: make(map[string]*sync.Mutex),
	}

	meta, err := getThreadMeta(result.ThreadID, "")
	if err != nil {
		t.Fatalf("getThreadMeta after agent refresh: %v", err)
	}
	if meta.ThreadID != result.ThreadID {
		t.Fatalf("meta.ThreadID = %q, want %q", meta.ThreadID, result.ThreadID)
	}
}

func TestGetThreadMetaEvictsMissingCachedThreadFile(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "hello.md")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "hello",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, "")
	if err != nil {
		t.Fatalf("prime getThreadMeta: %v", err)
	}
	if err := os.Remove(meta.ThreadFilePath); err != nil {
		t.Fatalf("remove thread file: %v", err)
	}

	if _, err := getThreadMeta(result.ThreadID, ""); !os.IsNotExist(err) {
		t.Fatalf("getThreadMeta after deleting cached file err = %v, want os.ErrNotExist", err)
	}
}

func TestUpdateThreadMetaWritesThreadMetaUpdateEntry(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "hello.md"),
		Title:    "hello",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	if _, err := updateThreadMeta(op.ThreadMetaUpdateParams{
		ThreadID: result.ThreadID,
		Title:    "renamed",
	}); err != nil {
		t.Fatalf("updateThreadMeta: %v", err)
	}

	raw, err := os.ReadFile(result.ThreadFilePath)
	if err != nil {
		t.Fatalf("read thread file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("thread file lines = %d, want 2", len(lines))
	}
	var entry struct {
		Type  string `json:"type"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &entry); err != nil {
		t.Fatalf("decode meta update entry: %v", err)
	}
	if entry.Type != op.ThreadEntryTypeMetaUpdate {
		t.Fatalf("entry type = %q, want %q", entry.Type, op.ThreadEntryTypeMetaUpdate)
	}
	if entry.Title != "renamed" {
		t.Fatalf("entry title = %q, want renamed", entry.Title)
	}
	if strings.Contains(string(raw), "session_info") {
		t.Fatalf("thread file still contains legacy session_info entry: %s", raw)
	}
}

func TestCreateThreadPrunesOldestFlatThreadFilesByMtime(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")
	config.SetSystem(&op.SystemConfig{
		BaseDir: baseDir,
		ThreadStorage: op.ThreadStorageConfig{
			MaxThreads: 10,
		},
	})

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	threadRoot := threadStorageRootDir(baseDir)
	created := make([]*op.ThreadCreateResult, 0, 3)
	for i := 0; i < 3; i++ {
		result, err := createThread(op.ThreadCreateParams{
			AgentID:  agentID,
			CWD:      cwd,
			ChatPath: filepath.Join(cwd, ".agent", "chat", "retention.md"),
			Title:    "retention",
		})
		if err != nil {
			t.Fatalf("createThread(%d): %v", i, err)
		}
		created = append(created, result)
	}

	oldestPath := threadFilePathInRoot(threadRoot, created[0].ThreadID)
	oldestReview := threadReviewRootForFile(oldestPath)
	if err := os.MkdirAll(filepath.Join(oldestReview, "turns", "turn-1"), 0o755); err != nil {
		t.Fatalf("mkdir oldest review: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldestReview, "turns", "turn-1", "manifest.json"), []byte(`{"turnID":"turn-1"}`), 0o644); err != nil {
		t.Fatalf("write oldest review: %v", err)
	}

	for i, result := range created {
		path := threadFilePathInRoot(threadRoot, result.ThreadID)
		modTime := time.Unix(int64(i+1), 0)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
	config.SetSystem(&op.SystemConfig{
		BaseDir: baseDir,
		ThreadStorage: op.ThreadStorageConfig{
			MaxThreads: 2,
		},
	})
	_, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "trigger.md"),
		Title:    "trigger",
	})
	if err != nil {
		t.Fatalf("createThread trigger: %v", err)
	}

	if _, err := os.Stat(oldestPath); !os.IsNotExist(err) {
		t.Fatalf("expected oldest thread pruned, got err=%v", err)
	}
	if _, err := os.Stat(oldestReview); !os.IsNotExist(err) {
		t.Fatalf("expected oldest review pruned, got err=%v", err)
	}
}

func TestGetThreadSnapshotIncludesQueueAndTailState(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "snapshot.md")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "snapshot",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	if err := appendMessagesToThread(*meta, []op.Message{op.NewUserMessage("resume me")}); err != nil {
		t.Fatalf("appendMessagesToThread: %v", err)
	}
	if _, _, err := appendQueuedMessageToThread(*meta, op.ThreadQueueKindFollowUp, op.NewUserMessage("queued"), "", "", "", 0, "", nil, nil, false); err != nil {
		t.Fatalf("appendQueuedMessageToThread: %v", err)
	}
	if _, err := defaultThreadStore.AppendMessageRecord(op.MessageSenderAgent, op.MessageRecord{
		ThreadID: result.ThreadID,
		AgentID:  agentID,
		Kind:     op.MessageKindStatus,
		Body:     "side event",
	}); err != nil {
		t.Fatalf("AppendMessageRecord: %v", err)
	}

	snapshot, err := getThreadSnapshot(op.ThreadMetaQuery{ThreadID: result.ThreadID, AgentID: agentID})
	if err != nil {
		t.Fatalf("getThreadSnapshot: %v", err)
	}
	if snapshot.RunStatus != op.ThreadRunIdle {
		t.Fatalf("snapshot.RunStatus = %q, want %q", snapshot.RunStatus, op.ThreadRunIdle)
	}
	if snapshot.TailStatus != op.ThreadTailNeedsContinuation {
		t.Fatalf("snapshot.TailStatus = %q, want %q", snapshot.TailStatus, op.ThreadTailNeedsContinuation)
	}
	if snapshot.ContinuationReason != op.ThreadContinuationUserTail {
		t.Fatalf("snapshot.ContinuationReason = %q, want %q", snapshot.ContinuationReason, op.ThreadContinuationUserTail)
	}
	if len(snapshot.QueuedMessages.FollowUp) != 1 || snapshot.QueuedMessages.FollowUp[0].Message.Content != "queued" {
		t.Fatalf("snapshot.QueuedMessages = %+v, want one queued follow-up", snapshot.QueuedMessages)
	}
	if len(snapshot.MessageRecords) != 1 || snapshot.MessageRecords[0].Body != "side event" {
		t.Fatalf("snapshot.MessageRecords = %+v, want side event", snapshot.MessageRecords)
	}
	if len(snapshot.ChannelSummaries) != 1 || snapshot.ChannelSummaries[0].OpenCount != 0 {
		t.Fatalf("snapshot.ChannelSummaries = %+v, want channel with no open requests", snapshot.ChannelSummaries)
	}
	if snapshot.EntryWindow.Mode != op.ThreadEntryWindowModeTail || snapshot.EntryWindow.Limit != 400 || snapshot.EntryWindow.Total != 3 {
		t.Fatalf("snapshot.EntryWindow = %+v, want default tail limit 400 over three entries", snapshot.EntryWindow)
	}
	if len(snapshot.Entries) != 3 {
		t.Fatalf("len(snapshot.Entries) = %d, want canonical + queue + message entry", len(snapshot.Entries))
	}
	if snapshot.Entries[0].Type != op.ThreadEntryTypeCanonicalMessage {
		t.Fatalf("snapshot.Entries[0].Type = %q, want %q", snapshot.Entries[0].Type, op.ThreadEntryTypeCanonicalMessage)
	}
	if snapshot.Entries[1].Type != op.ThreadEntryTypeQueueEnqueue {
		t.Fatalf("snapshot.Entries[1].Type = %q, want %q", snapshot.Entries[1].Type, op.ThreadEntryTypeQueueEnqueue)
	}
	if snapshot.Entries[2].Type != op.ThreadEntryTypeMessageAppend {
		t.Fatalf("snapshot.Entries[2].Type = %q, want %q", snapshot.Entries[2].Type, op.ThreadEntryTypeMessageAppend)
	}
	if snapshot.Revision == "" || snapshot.Revision != snapshot.Entries[2].ID {
		t.Fatalf("snapshot.Revision = %q, want last entry id %q", snapshot.Revision, snapshot.Entries[2].ID)
	}
	rawSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var snapshotWire map[string]json.RawMessage
	if err := json.Unmarshal(rawSnapshot, &snapshotWire); err != nil {
		t.Fatalf("unmarshal snapshot wire: %v", err)
	}
	if _, ok := snapshotWire["messages"]; ok {
		t.Fatalf("snapshot wire contains removed messages field: %s", rawSnapshot)
	}
}

func TestGetThreadSnapshotWindowsEntries(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "window.md"),
		Title:    "window",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	for i := 0; i < 6; i++ {
		if err := appendMessagesToThread(*meta, []op.Message{op.NewUserMessage("message")}); err != nil {
			t.Fatalf("appendMessagesToThread %d: %v", i, err)
		}
	}

	all, err := getThreadSnapshot(op.ThreadMetaQuery{
		ThreadID: result.ThreadID,
		AgentID:  agentID,
		EntryWindow: &op.ThreadEntryWindowQuery{
			Mode:  op.ThreadEntryWindowModeTail,
			Limit: 10,
		},
	})
	if err != nil {
		t.Fatalf("getThreadSnapshot all: %v", err)
	}
	if len(all.Entries) != 6 {
		t.Fatalf("len(all.Entries) = %d, want 6", len(all.Entries))
	}

	tail, err := getThreadSnapshot(op.ThreadMetaQuery{
		ThreadID: result.ThreadID,
		AgentID:  agentID,
		EntryWindow: &op.ThreadEntryWindowQuery{
			Mode:  op.ThreadEntryWindowModeTail,
			Limit: 3,
		},
	})
	if err != nil {
		t.Fatalf("getThreadSnapshot tail: %v", err)
	}
	if len(tail.Entries) != 3 || tail.Entries[0].ID != all.Entries[3].ID || tail.Entries[2].ID != all.Entries[5].ID {
		t.Fatalf("tail entries = %+v, want last three entries", tail.Entries)
	}
	if tail.EntryWindow.Mode != op.ThreadEntryWindowModeTail || tail.EntryWindow.Start != 3 || tail.EntryWindow.End != 6 || tail.EntryWindow.Total != 6 || !tail.EntryWindow.HasBefore || tail.EntryWindow.HasAfter {
		t.Fatalf("tail.EntryWindow = %+v", tail.EntryWindow)
	}

	before, err := getThreadSnapshot(op.ThreadMetaQuery{
		ThreadID: result.ThreadID,
		AgentID:  agentID,
		EntryWindow: &op.ThreadEntryWindowQuery{
			Mode:     op.ThreadEntryWindowModeBefore,
			AnchorID: all.Entries[3].ID,
			Limit:    2,
		},
	})
	if err != nil {
		t.Fatalf("getThreadSnapshot before: %v", err)
	}
	if len(before.Entries) != 2 || before.Entries[0].ID != all.Entries[1].ID || before.Entries[1].ID != all.Entries[2].ID {
		t.Fatalf("before entries = %+v, want entries 1 and 2", before.Entries)
	}
	if before.EntryWindow.Start != 1 || before.EntryWindow.End != 3 || before.EntryWindow.Total != 6 || !before.EntryWindow.HasBefore || !before.EntryWindow.HasAfter {
		t.Fatalf("before.EntryWindow = %+v", before.EntryWindow)
	}
	if before.Revision != all.Entries[5].ID {
		t.Fatalf("before.Revision = %q, want full-file last entry %q", before.Revision, all.Entries[5].ID)
	}

	after, err := getThreadSnapshot(op.ThreadMetaQuery{
		ThreadID: result.ThreadID,
		AgentID:  agentID,
		EntryWindow: &op.ThreadEntryWindowQuery{
			Mode:     op.ThreadEntryWindowModeAfter,
			AnchorID: all.Entries[2].ID,
			Limit:    2,
		},
	})
	if err != nil {
		t.Fatalf("getThreadSnapshot after: %v", err)
	}
	if len(after.Entries) != 2 || after.Entries[0].ID != all.Entries[3].ID || after.Entries[1].ID != all.Entries[4].ID {
		t.Fatalf("after entries = %+v, want entries 3 and 4", after.Entries)
	}
	if after.EntryWindow.Start != 3 || after.EntryWindow.End != 5 || after.EntryWindow.Total != 6 || !after.EntryWindow.HasBefore || !after.EntryWindow.HasAfter {
		t.Fatalf("after.EntryWindow = %+v", after.EntryWindow)
	}

	missingAnchor, err := getThreadSnapshot(op.ThreadMetaQuery{
		ThreadID: result.ThreadID,
		AgentID:  agentID,
		EntryWindow: &op.ThreadEntryWindowQuery{
			Mode:     op.ThreadEntryWindowModeBefore,
			AnchorID: "entry-missing",
			Limit:    2,
		},
	})
	if err != nil {
		t.Fatalf("getThreadSnapshot missing anchor: %v", err)
	}
	if missingAnchor.EntryWindow.Mode != op.ThreadEntryWindowModeTail || missingAnchor.EntryWindow.AnchorID != "" {
		t.Fatalf("missingAnchor.EntryWindow = %+v, want tail fallback", missingAnchor.EntryWindow)
	}
	if len(missingAnchor.Entries) != 2 || missingAnchor.Entries[0].ID != all.Entries[4].ID || missingAnchor.Entries[1].ID != all.Entries[5].ID {
		t.Fatalf("missing anchor entries = %+v, want last two entries", missingAnchor.Entries)
	}
}

func TestGetThreadSnapshotWindowKeepsAggregates(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "aggregate.md")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "aggregate",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	if err := appendMessagesToThread(*meta, []op.Message{op.NewUserMessage("resume me")}); err != nil {
		t.Fatalf("appendMessagesToThread: %v", err)
	}
	if _, _, err := appendQueuedMessageToThread(*meta, op.ThreadQueueKindFollowUp, op.NewUserMessage("queued"), "", "", "", 0, "", nil, nil, false); err != nil {
		t.Fatalf("appendQueuedMessageToThread: %v", err)
	}
	if _, err := defaultThreadStore.AppendMessageRecord(op.MessageSenderAgent, op.MessageRecord{
		ThreadID: result.ThreadID,
		AgentID:  agentID,
		Kind:     op.MessageKindStatus,
		Body:     "side event",
	}); err != nil {
		t.Fatalf("AppendMessageRecord: %v", err)
	}

	snapshot, err := getThreadSnapshot(op.ThreadMetaQuery{
		ThreadID: result.ThreadID,
		AgentID:  agentID,
		EntryWindow: &op.ThreadEntryWindowQuery{
			Mode:  op.ThreadEntryWindowModeTail,
			Limit: 1,
		},
	})
	if err != nil {
		t.Fatalf("getThreadSnapshot: %v", err)
	}
	if len(snapshot.Entries) != 1 || snapshot.EntryWindow.Total != 3 {
		t.Fatalf("entries/window = (%d, %+v), want one visible entry out of three", len(snapshot.Entries), snapshot.EntryWindow)
	}
	if snapshot.TailStatus != op.ThreadTailNeedsContinuation || snapshot.ContinuationReason != op.ThreadContinuationUserTail {
		t.Fatalf("tail state = (%q, %q), want user-tail continuation", snapshot.TailStatus, snapshot.ContinuationReason)
	}
	if len(snapshot.QueuedMessages.FollowUp) != 1 || snapshot.QueuedMessages.FollowUp[0].Message.Content != "queued" {
		t.Fatalf("snapshot.QueuedMessages = %+v, want one queued follow-up", snapshot.QueuedMessages)
	}
	if len(snapshot.MessageRecords) != 1 || snapshot.MessageRecords[0].Body != "side event" {
		t.Fatalf("snapshot.MessageRecords = %+v, want side event", snapshot.MessageRecords)
	}
	if len(snapshot.ChannelSummaries) != 1 || snapshot.ChannelSummaries[0].OpenCount != 0 {
		t.Fatalf("snapshot.ChannelSummaries = %+v, want channel with no open requests", snapshot.ChannelSummaries)
	}
}

func TestQueuedMessageCapturesTurnAgentContext(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "queue-agent.md")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "queue-agent",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	turnMeta := *meta
	turnMeta.AgentID = "agent-gbrain"
	turnMeta.CWD = filepath.Join(baseDir, "workspace")
	item, snapshot, err := appendQueuedMessageToThread(turnMeta, op.ThreadQueueKindSteering, op.NewUserMessage("queued"), "gbrain", "test:model", "high", 0, "", nil, nil, false)
	if err != nil {
		t.Fatalf("appendQueuedMessageToThread: %v", err)
	}
	if item == nil {
		t.Fatal("appendQueuedMessageToThread returned nil item")
	}
	if item.AgentID != "agent-gbrain" || item.AgentName != "gbrain" || item.CWD != turnMeta.CWD {
		t.Fatalf("item agent context = (%q, %q, %q), want (%q, %q, %q)", item.AgentID, item.AgentName, item.CWD, "agent-gbrain", "gbrain", turnMeta.CWD)
	}
	if item.ModelKey != "test:model" || item.ThinkingLevel != "high" {
		t.Fatalf("item model context = (%q, %q), want (test:model, high)", item.ModelKey, item.ThinkingLevel)
	}
	if len(snapshot.Steering) != 1 || snapshot.Steering[0].AgentID != "agent-gbrain" || snapshot.Steering[0].AgentName != "gbrain" || snapshot.Steering[0].CWD != turnMeta.CWD {
		t.Fatalf("snapshot.Steering = %+v, want queued turn agent context", snapshot.Steering)
	}
	if snapshot.Steering[0].ModelKey != "test:model" || snapshot.Steering[0].ThinkingLevel != "high" {
		t.Fatalf("snapshot.Steering[0] model context = (%q, %q), want (test:model, high)", snapshot.Steering[0].ModelKey, snapshot.Steering[0].ThinkingLevel)
	}
}

func TestGetThreadSnapshotIncludesContextUsage(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	config.SyncModelCache([]op.ModelConfig{{
		Key:           "test:model",
		ID:            "model",
		Name:          "model",
		Provider:      "test",
		APIKey:        "test-key",
		ContextWindow: 1_000_000,
	}})
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "context.md")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "context",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	if err := appendMessagesToThread(*meta, []op.Message{
		op.NewUserMessage("hello"),
		{
			Role:    op.RoleAssistant,
			Content: "hi",
			Usage:   &op.MessageUsage{TotalTokens: 12_345},
		},
	}); err != nil {
		t.Fatalf("appendMessagesToThread: %v", err)
	}

	snapshot, err := defaultThreadStore.GetThreadSnapshotWithMeta(
		op.ThreadMetaQuery{ThreadID: result.ThreadID, AgentID: agentID},
		op.Meta{"modelKey": "test:model"},
	)
	if err != nil {
		t.Fatalf("GetThreadSnapshotWithMeta: %v", err)
	}
	if !snapshot.ContextUsage.Known {
		t.Fatalf("snapshot.ContextUsage.Known = false, want true")
	}
	if snapshot.ContextUsage.Tokens != 12_345 {
		t.Fatalf("snapshot.ContextUsage.Tokens = %d, want 12345", snapshot.ContextUsage.Tokens)
	}
	if snapshot.ContextUsage.ContextWindow != 1_000_000 {
		t.Fatalf("snapshot.ContextUsage.ContextWindow = %d, want 1000000", snapshot.ContextUsage.ContextWindow)
	}
}

func TestGetThreadSnapshotIncludesCompactionEntriesAfterRewrite(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: filepath.Join(cwd, ".agent", "chat", "compact.md"),
		Title:    "compact",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	if err := appendMessagesToThread(*meta, []op.Message{
		op.NewUserMessage("before compaction"),
		op.NewAssistantMessage("before summary"),
	}); err != nil {
		t.Fatalf("appendMessagesToThread: %v", err)
	}

	compacted := []ai.ConversationMessage{
		{
			Role:    ai.RoleCanonicalSystem,
			Content: []ai.ContentBlock{{Type: ai.BlockText, Text: "Context checkpoint summary:\nshort summary"}},
		},
		{
			Role:    ai.RoleCanonicalUser,
			Content: []ai.ContentBlock{{Type: ai.BlockText, Text: "kept user"}},
		},
	}
	if err := replaceThreadCanonicalMessagesWithCompaction(*meta, compacted, 100); err != nil {
		t.Fatalf("replaceThreadCanonicalMessagesWithCompaction: %v", err)
	}

	snapshot, err := getThreadSnapshot(op.ThreadMetaQuery{ThreadID: result.ThreadID, AgentID: agentID})
	if err != nil {
		t.Fatalf("getThreadSnapshot: %v", err)
	}
	if len(snapshot.Entries) != 2 {
		t.Fatalf("len(snapshot.Entries) = %d, want compaction + kept canonical", len(snapshot.Entries))
	}
	if snapshot.Entries[0].Type != sessionEntryTypeCompaction {
		t.Fatalf("snapshot.Entries[0].Type = %q, want %q", snapshot.Entries[0].Type, sessionEntryTypeCompaction)
	}
	if snapshot.Entries[1].Type != op.ThreadEntryTypeCanonicalMessage {
		t.Fatalf("snapshot.Entries[1].Type = %q, want %q", snapshot.Entries[1].Type, op.ThreadEntryTypeCanonicalMessage)
	}
	if snapshot.Revision != snapshot.Entries[1].ID {
		t.Fatalf("snapshot.Revision = %q, want %q", snapshot.Revision, snapshot.Entries[1].ID)
	}
}

func TestAppendQueueDeliveryAppendsDequeueBeforeCanonicalMessage(t *testing.T) {
	baseDir := t.TempDir()
	resetThreadTestState(baseDir)
	agentID, _, _ := createTestAgent(t, baseDir, "agents/demo")

	cwd := filepath.Join(baseDir, "workspace", "proj")
	if err := os.MkdirAll(filepath.Join(cwd, ".agent", "chat"), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	chatPath := filepath.Join(cwd, ".agent", "chat", "queue-delivery.md")
	result, err := createThread(op.ThreadCreateParams{
		AgentID:  agentID,
		CWD:      cwd,
		ChatPath: chatPath,
		Title:    "queue-delivery",
	})
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	meta, err := getThreadMeta(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("getThreadMeta: %v", err)
	}
	item, _, err := appendQueuedMessageToThread(*meta, op.ThreadQueueKindSteering, op.NewUserMessage("queued user"), "", "", "", 0, "", nil, nil, false)
	if err != nil {
		t.Fatalf("appendQueuedMessageToThread: %v", err)
	}
	if item == nil {
		t.Fatal("appendQueuedMessageToThread returned nil item")
	}
	pending := pendingLoopMessageFromQueueItem(*item, op.ThreadQueueKindSteering)
	if err := appendQueueDeliveryToThread(*meta, []PendingLoopMessage{pending}); err != nil {
		t.Fatalf("appendQueueDeliveryToThread: %v", err)
	}

	sessionCtx, err := loadThreadContext(result.ThreadID, agentID)
	if err != nil {
		t.Fatalf("loadThreadContext: %v", err)
	}
	if len(sessionCtx.queuedMessages.Steering) != 0 || len(sessionCtx.queuedMessages.FollowUp) != 0 {
		t.Fatalf("queued messages after appendQueueDelivery = %+v, want empty", sessionCtx.queuedMessages)
	}
	persisted := opMessagesFromCanonicalForTest(t, sessionCtx.canonicalMessages)
	if len(persisted) != 1 || persisted[0].Role != op.RoleUser || persisted[0].Content != "queued user" {
		t.Fatalf("persisted messages = %+v, want queued user message", persisted)
	}

	raw, err := os.ReadFile(sessionCtx.meta.ThreadFilePath)
	if err != nil {
		t.Fatalf("read thread file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 3 {
		t.Fatalf("session lines len = %d, want at least 3", len(lines))
	}
	if !strings.Contains(lines[len(lines)-2], `"type":"queue_dequeue"`) {
		t.Fatalf("penultimate session line = %s, want queue_dequeue", lines[len(lines)-2])
	}
	if !strings.Contains(lines[len(lines)-1], `"type":"canonical_message"`) {
		t.Fatalf("last session line = %s, want canonical_message", lines[len(lines)-1])
	}
}
