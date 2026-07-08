package chatindex

import (
	"os"
	"path/filepath"
	"testing"
)

func writeChatProjection(t *testing.T, path, threadID, title string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "---\nthread: " + threadID + "\ntitle: \"" + title + "\"\n---\n\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write projection: %v", err)
	}
}

func TestUpsertAndResolveIndexes(t *testing.T) {
	baseDir := t.TempDir()
	SetBaseDir(baseDir)
	t.Cleanup(func() { SetBaseDir("") })
	cwd := filepath.Join(baseDir, "workspace", "demo")
	threadFilePath := filepath.Join(baseDir, "threads", "thread-demo.jsonl")
	if err := os.MkdirAll(filepath.Dir(threadFilePath), 0o755); err != nil {
		t.Fatalf("mkdir thread dir: %v", err)
	}
	if err := os.WriteFile(threadFilePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write thread file: %v", err)
	}

	fileRecord := FileRecord{FileID: "file-demo", AgentID: "agent-demo", ThreadID: "thread-demo", Path: filepath.Join(cwd, ".agent", "chat", "demo.md")}
	if err := UpsertFileRecord(cwd, fileRecord); err != nil {
		t.Fatalf("UpsertFileRecord: %v", err)
	}
	resolvedFile, err := ResolveFileRecord(cwd, "file-demo")
	if err != nil {
		t.Fatalf("ResolveFileRecord: %v", err)
	}
	if resolvedFile.ThreadID != "thread-demo" || resolvedFile.Path != fileRecord.Path {
		t.Fatalf("resolved file = %+v", resolvedFile)
	}

	threadRecord := ThreadRecord{ThreadID: "thread-demo", AgentID: "agent-demo", FileID: "file-demo", Path: threadFilePath}
	if err := UpsertThreadRecordForThreadFile(threadFilePath, threadRecord); err != nil {
		t.Fatalf("UpsertThreadRecordForThreadFile: %v", err)
	}
	indexPath, err := ThreadIndexPath("agent-demo")
	if err != nil {
		t.Fatalf("ThreadIndexPath: %v", err)
	}
	records, err := readThreadIndexAtPath(indexPath)
	if err != nil {
		t.Fatalf("readThreadIndexAtPath: %v", err)
	}
	if len(records) != 1 || records[0].ThreadID != "thread-demo" || records[0].FileID != "file-demo" {
		t.Fatalf("thread records = %+v", records)
	}
}

func TestReconcileFileIndexSkipsDuplicateThreads(t *testing.T) {
	baseDir := t.TempDir()
	SetBaseDir(baseDir)
	t.Cleanup(func() { SetBaseDir("") })
	cwd := filepath.Join(baseDir, "workspace", "demo")
	chatRoot := filepath.Join(cwd, ".agent", "chat")
	writeChatProjection(t, filepath.Join(chatRoot, "demo.md"), "thread-demo", "Demo")
	writeChatProjection(t, filepath.Join(chatRoot, "history", "2026-04-13", "demo-copy.md"), "thread-demo", "Demo")
	writeChatProjection(t, filepath.Join(chatRoot, "other.md"), "thread-other", "Other")
	threadPath := filepath.Join(baseDir, "threads", "thread-other.jsonl")
	if err := os.MkdirAll(filepath.Dir(threadPath), 0o755); err != nil {
		t.Fatalf("mkdir thread dir: %v", err)
	}
	if err := os.WriteFile(threadPath, []byte(`{"type":"thread","version":2,"id":"thread-other","agentID":"agent-demo","cwd":"","chatPath":"","title":"Other"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write thread file: %v", err)
	}

	records, err := ReconcileFileIndex(cwd)
	if err != nil {
		t.Fatalf("ReconcileFileIndex: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	if records[0].ThreadID != "thread-other" {
		t.Fatalf("records[0] = %+v", records[0])
	}
	if records[0].FileID == "" {
		t.Fatal("expected generated fileID")
	}
}

func TestReconcileFileIndexSkipsMarkdownWithoutThreadBody(t *testing.T) {
	baseDir := t.TempDir()
	SetBaseDir(baseDir)
	t.Cleanup(func() { SetBaseDir("") })
	cwd := filepath.Join(baseDir, "workspace", "demo")
	chatRoot := filepath.Join(cwd, ".agent", "chat")
	writeChatProjection(t, filepath.Join(chatRoot, "orphan.md"), "thread-orphan", "Orphan")

	records, err := ReconcileFileIndex(cwd)
	if err != nil {
		t.Fatalf("ReconcileFileIndex: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records len = %d, want 0: %+v", len(records), records)
	}
}

func TestReconcileThreadIndexAtRootWritesThreadIndex(t *testing.T) {
	baseDir := t.TempDir()
	SetBaseDir(baseDir)
	t.Cleanup(func() { SetBaseDir("") })
	threadRoot := filepath.Join(baseDir, "threads")
	if err := os.MkdirAll(threadRoot, 0o755); err != nil {
		t.Fatalf("mkdir thread root: %v", err)
	}
	threadFilePath := filepath.Join(threadRoot, "thread-demo.jsonl")
	if err := os.WriteFile(threadFilePath, []byte(`{"type":"thread","version":2,"id":"thread-demo","agentID":"agent-demo","cwd":"/tmp/workspace","chatPath":"/tmp/workspace/.agent/chat/demo.md","title":"Demo"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write thread file: %v", err)
	}

	records, err := ReconcileThreadIndexAtRoot(threadRoot, []FileRecord{{
		FileID:   "file-demo",
		ThreadID: "thread-demo",
		Path:     "/tmp/workspace/.agent/chat/demo.md",
	}})
	if err != nil {
		t.Fatalf("ReconcileThreadIndexAtRoot: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	if records[0].ThreadID != "thread-demo" || records[0].FileID != "file-demo" || records[0].Path != threadFilePath {
		t.Fatalf("records[0] = %+v", records[0])
	}
	indexRecords, err := ReadThreadIndex("agent-demo")
	if err != nil {
		t.Fatalf("ReadThreadIndex: %v", err)
	}
	if len(indexRecords) != 1 || indexRecords[0] != records[0] {
		t.Fatalf("indexRecords = %+v", indexRecords)
	}
}

func TestReconcileThreadIndexAtRootClearsStaleAgentIndex(t *testing.T) {
	baseDir := t.TempDir()
	SetBaseDir(baseDir)
	t.Cleanup(func() { SetBaseDir("") })
	threadRoot := filepath.Join(baseDir, "threads")
	if err := os.MkdirAll(threadRoot, 0o755); err != nil {
		t.Fatalf("mkdir thread root: %v", err)
	}
	if err := UpsertThreadRecord("agent-stale", ThreadRecord{
		ThreadID: "thread-stale",
		AgentID:  "agent-stale",
		FileID:   "file-stale",
		Path:     filepath.Join(threadRoot, "thread-stale.jsonl"),
	}); err != nil {
		t.Fatalf("UpsertThreadRecord: %v", err)
	}

	records, err := ReconcileThreadIndexAtRoot(threadRoot, nil)
	if err != nil {
		t.Fatalf("ReconcileThreadIndexAtRoot: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records len = %d, want 0", len(records))
	}
	if _, err := ResolveThreadRecord("agent-stale", "thread-stale"); !os.IsNotExist(err) {
		t.Fatalf("ResolveThreadRecord stale err = %v, want os.ErrNotExist", err)
	}
}
