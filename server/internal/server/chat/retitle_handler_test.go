package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/chatindex"
	"github.com/gin-gonic/gin"
)

func writeProjectionForRetitleTest(t *testing.T, path string, title string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir chat dir: %v", err)
	}
	content := strings.Join([]string{
		"---",
		"thread: thread-test",
		`title: "` + title + `"`,
		"---",
		"",
		"body",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write projection: %v", err)
	}
}

func newRetitleTestService(t *testing.T, meta *op.ThreadMeta) (*Service, func()) {
	t.Helper()
	service, cleanup := newControlTestService(
		t,
		func(_ context.Context, _ *op.OpAgentRequest) (*op.OpAgentResult, error) {
			return nil, nil
		},
		func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			switch req.Params.OpCode {
			case op.OpThreadMetaGet:
				raw, err := json.Marshal(meta)
				if err != nil {
					t.Fatalf("marshal meta: %v", err)
				}
				return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
			case op.OpThreadMetaUpdate:
				jsonContent, ok := req.Params.Content.(*op.JsonContent)
				if !ok {
					t.Fatalf("expected JsonContent, got %T", req.Params.Content)
				}
				var params op.ThreadMetaUpdateParams
				if err := json.Unmarshal(jsonContent.Raw, &params); err != nil {
					t.Fatalf("unmarshal update params: %v", err)
				}
				if strings.TrimSpace(params.Title) != "" {
					meta.Title = strings.TrimSpace(params.Title)
				}
				if strings.TrimSpace(params.ChatPath) != "" {
					meta.ChatPath = strings.TrimSpace(params.ChatPath)
				}
				raw, err := json.Marshal(meta)
				if err != nil {
					t.Fatalf("marshal updated meta: %v", err)
				}
				return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
			default:
				t.Fatalf("unexpected node opcode: %s", req.Params.OpCode)
				return nil, nil
			}
		},
	)
	return service, cleanup
}

func TestServiceRetitleThreadMigratesAutoNamedChatPath(t *testing.T) {
	cwd := t.TempDir()
	chatindex.SetBaseDir(cwd)
	t.Cleanup(func() { chatindex.SetBaseDir("") })

	oldPath := filepath.Join(cwd, ".agent", "chat", "untitled-chat.md")
	writeProjectionForRetitleTest(t, oldPath, "Untitled Chat")
	threadFilePath := filepath.Join(cwd, "threads", "thread-test.jsonl")
	if err := chatindex.UpsertFileRecord(cwd, chatindex.FileRecord{
		FileID:   "file-test",
		AgentID:  "agent-test",
		ThreadID: "thread-test",
		CWD:      cwd,
		Path:     oldPath,
	}); err != nil {
		t.Fatalf("UpsertFileRecord(old): %v", err)
	}
	if err := chatindex.UpsertThreadRecordForThreadFile(threadFilePath, chatindex.ThreadRecord{
		ThreadID: "thread-test",
		AgentID:  "agent-test",
		FileID:   "file-test",
		CWD:      cwd,
		ChatPath: oldPath,
		Path:     threadFilePath,
		Title:    "Untitled Chat",
	}); err != nil {
		t.Fatalf("UpsertThreadRecordForThreadFile(old): %v", err)
	}

	meta := &op.ThreadMeta{
		ThreadID:       "thread-test",
		FileID:         "file-test",
		AgentID:        "agent-test",
		CWD:            cwd,
		ChatPath:       oldPath,
		ThreadFilePath: threadFilePath,
		Title:          "Untitled Chat",
	}
	service, cleanup := newRetitleTestService(t, meta)
	defer cleanup()

	updated, err := service.RetitleThread(context.Background(), op.ThreadMetaQuery{ChatPath: oldPath}, "Slash command only at line start")
	if err != nil {
		t.Fatalf("RetitleThread(): %v", err)
	}
	if updated.ChatPath == oldPath {
		t.Fatalf("expected chat path to migrate, got %+v", updated)
	}
	if filepath.Base(updated.ChatPath) != "slash-command-only-at-line-start.md" {
		t.Fatalf("ChatPath basename = %q, want slash-command-only-at-line-start.md", filepath.Base(updated.ChatPath))
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old chat path still exists or stat failed: %v", err)
	}
	body, err := os.ReadFile(updated.ChatPath)
	if err != nil {
		t.Fatalf("ReadFile(new projection): %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `title: "Slash command only at line start"`) {
		t.Fatalf("expected updated title frontmatter, got %q", text)
	}
	if !strings.Contains(text, "\nbody\n") {
		t.Fatalf("expected projection body to be preserved, got %q", text)
	}
	fileRecord, err := chatindex.ResolveFileRecord(cwd, "file-test")
	if err != nil {
		t.Fatalf("ResolveFileRecord: %v", err)
	}
	if fileRecord.Path != updated.ChatPath {
		t.Fatalf("file index path = %q, want %q", fileRecord.Path, updated.ChatPath)
	}
	threadRecord, err := chatindex.ResolveThreadRecord("agent-test", "thread-test")
	if err != nil {
		t.Fatalf("ResolveThreadRecord: %v", err)
	}
	if threadRecord.ChatPath != updated.ChatPath || threadRecord.Title != "Slash command only at line start" {
		t.Fatalf("thread index = %+v, want migrated path and title", threadRecord)
	}
}

func TestServiceRetitleThreadMigratesAutoNamedChatPathWithCollision(t *testing.T) {
	cwd := t.TempDir()
	oldPath := filepath.Join(cwd, ".agent", "chat", "untitled-chat.md")
	writeProjectionForRetitleTest(t, oldPath, "Untitled Chat")
	collidingPath := filepath.Join(cwd, ".agent", "chat", "slash-command-only-at-line-start.md")
	writeProjectionForRetitleTest(t, collidingPath, "Existing")

	meta := &op.ThreadMeta{
		ThreadID: "thread-test",
		AgentID:  "agent-test",
		CWD:      cwd,
		ChatPath: oldPath,
		Title:    "Untitled Chat",
	}
	service, cleanup := newRetitleTestService(t, meta)
	defer cleanup()

	updated, err := service.RetitleThread(context.Background(), op.ThreadMetaQuery{ChatPath: oldPath}, "Slash command only at line start")
	if err != nil {
		t.Fatalf("RetitleThread(): %v", err)
	}
	if filepath.Base(updated.ChatPath) != "slash-command-only-at-line-start-2.md" {
		t.Fatalf("ChatPath basename = %q, want collision suffix", filepath.Base(updated.ChatPath))
	}
}

func TestServiceRetitleThreadKeepsExplicitChatPath(t *testing.T) {
	cwd := t.TempDir()
	chatPath := filepath.Join(cwd, ".agent", "chat", "custom-thread.md")
	writeProjectionForRetitleTest(t, chatPath, "Custom Title")

	meta := &op.ThreadMeta{
		ThreadID:       "thread-test",
		AgentID:        "agent-id",
		CWD:            cwd,
		ChatPath:       chatPath,
		ThreadFilePath: filepath.Join(cwd, ".threads", "thread-test.jsonl"),
		Title:          "Custom Title",
	}
	service, cleanup := newRetitleTestService(t, meta)
	defer cleanup()

	updated, err := service.RetitleThread(context.Background(), op.ThreadMetaQuery{ChatPath: chatPath}, "Refined Title")
	if err != nil {
		t.Fatalf("RetitleThread(): %v", err)
	}
	if updated.ChatPath != chatPath {
		t.Fatalf("expected explicit chat path to remain stable, got %+v", updated)
	}
	body, err := os.ReadFile(chatPath)
	if err != nil {
		t.Fatalf("ReadFile(chat projection): %v", err)
	}
	if !strings.Contains(string(body), `title: "Refined Title"`) {
		t.Fatalf("expected projection title update, got %q", string(body))
	}
}

func TestRetitleHandlerReturnsUpdatedMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := retitleThread
	retitleThread = func(_ context.Context, _ *Service, query op.ThreadMetaQuery, title string) (*op.ThreadMeta, error) {
		return &op.ThreadMeta{
			ThreadID: "thread-test",
			ChatPath: "/tmp/retitled.md",
			Title:    title,
			CWD:      "/tmp",
		}, nil
	}
	defer func() {
		retitleThread = prev
	}()

	raw, _ := json.Marshal(ChatRetitleParams{
		ThreadID: "thread-test",
		Title:    "Retitled",
	})
	req := httptest.NewRequest("POST", "/v1/thread/retitle", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/thread/retitle", NewRetitleHandler(&Service{}).Retitle)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var meta op.ThreadMeta
	if err := json.NewDecoder(rr.Body).Decode(&meta); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if meta.ChatPath != "/tmp/retitled.md" || meta.Title != "Retitled" {
		t.Fatalf("unexpected retitle response: %+v", meta)
	}
}
