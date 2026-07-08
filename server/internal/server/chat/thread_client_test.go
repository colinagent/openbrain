package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/chatindex"
)

func TestResolveThreadIDByChatPathRepairsMissingFileIndex(t *testing.T) {
	cwd := t.TempDir()
	chatindex.SetBaseDir(cwd)
	t.Cleanup(func() { chatindex.SetBaseDir("") })
	chatPath := filepath.Join(cwd, ".agent", "chat", "demo.md")
	if err := os.MkdirAll(filepath.Dir(chatPath), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	if err := os.WriteFile(chatPath, []byte(strings.Join([]string{
		"---",
		"thread: thread-demo",
		`title: "Demo"`,
		"---",
		"",
		"body",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write chat markdown: %v", err)
	}
	threadFilePath := filepath.Join(cwd, "threads", "thread-demo.jsonl")
	if err := os.MkdirAll(filepath.Dir(threadFilePath), 0o755); err != nil {
		t.Fatalf("mkdir thread dir: %v", err)
	}
	if err := os.WriteFile(threadFilePath, []byte(`{"type":"thread","version":2,"id":"thread-demo","agentID":"agent-demo","cwd":"","chatPath":"","title":"Demo"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write thread file: %v", err)
	}

	threadID := resolveThreadIDByChatPath(chatPath)
	if threadID != "thread-demo" {
		t.Fatalf("threadID = %q, want thread-demo", threadID)
	}

	records, err := chatindex.ReadFileIndex(cwd)
	if err != nil {
		t.Fatalf("ReadFileIndex: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	if records[0].ThreadID != "thread-demo" {
		t.Fatalf("records[0].ThreadID = %q, want thread-demo", records[0].ThreadID)
	}
	if records[0].Path != chatPath {
		t.Fatalf("records[0].Path = %q, want %q", records[0].Path, chatPath)
	}
	if !strings.HasPrefix(records[0].FileID, "file-") {
		t.Fatalf("records[0].FileID = %q, want file-*", records[0].FileID)
	}
}

func TestWithResolvedThreadMetaRepairsMissingThreadIndex(t *testing.T) {
	cwd := t.TempDir()
	chatindex.SetBaseDir(cwd)
	t.Cleanup(func() { chatindex.SetBaseDir("") })
	chatPath := filepath.Join(cwd, ".agent", "chat", "demo.md")
	if err := os.MkdirAll(filepath.Dir(chatPath), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	agentID := "agent-demo"
	threadFilePath := filepath.Join(cwd, "threads", "thread-demo.jsonl")
	if err := os.MkdirAll(filepath.Dir(threadFilePath), 0o755); err != nil {
		t.Fatalf("mkdir thread dir: %v", err)
	}
	if err := os.WriteFile(threadFilePath, []byte(`{"type":"thread","version":2,"id":"thread-demo","agentID":"agent-demo","cwd":"","chatPath":"","title":"Demo"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write thread file: %v", err)
	}

	if err := chatindex.UpsertFileRecord(cwd, chatindex.FileRecord{
		FileID:   "file-demo",
		AgentID:  agentID,
		ThreadID: "thread-demo",
		Path:     chatPath,
	}); err != nil {
		t.Fatalf("UpsertFileRecord: %v", err)
	}

	service := &Service{}
	resolved := service.withResolvedThreadMeta(&op.ThreadMeta{
		ThreadID:       "thread-demo",
		FileID:         "file-demo",
		AgentID:        agentID,
		CWD:            cwd,
		ThreadFilePath: threadFilePath,
	})

	record, err := chatindex.ResolveThreadRecord(agentID, "thread-demo")
	if err != nil {
		t.Fatalf("ResolveThreadRecord: %v", err)
	}
	if record.FileID != "file-demo" {
		t.Fatalf("record.FileID = %q, want file-demo", record.FileID)
	}
	if record.Path != threadFilePath {
		t.Fatalf("record.Path = %q, want %q", record.Path, threadFilePath)
	}
	if resolved.Path != chatPath || resolved.ChatPath != chatPath {
		t.Fatalf("resolved path = %q / %q, want %q", resolved.Path, resolved.ChatPath, chatPath)
	}
}
