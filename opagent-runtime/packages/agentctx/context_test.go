package agentctx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadChatFileMeta_readsCanonicalThreadFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chat.md")
	content := "---\nthread: thread-1\nprotocol: chat-v2\ntitle: \"demo\"\n---\n\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	meta, err := ReadChatFileMeta(path)
	if err != nil {
		t.Fatalf("ReadChatFileMeta: %v", err)
	}
	if meta.ThreadID != "thread-1" {
		t.Fatalf("expected threadID thread-1, got %q", meta.ThreadID)
	}
	if meta.Protocol != ChatProtocolV2 {
		t.Fatalf("expected protocol %q, got %q", ChatProtocolV2, meta.Protocol)
	}
	if meta.Title != "demo" {
		t.Fatalf("expected title demo, got %q", meta.Title)
	}
}
