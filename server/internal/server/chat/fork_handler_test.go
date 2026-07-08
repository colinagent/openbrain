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

func TestForkHandler_CreatesChildThread(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cwd := t.TempDir()
	chatindex.SetBaseDir(cwd)
	t.Cleanup(func() { chatindex.SetBaseDir("") })
	var captured op.ThreadForkParams
	body := ChatForkParams{
		SourceThreadID:    "thread-parent",
		CWD:               cwd,
		AgentID:           "agent-test",
		Title:             "Build Release",
		ExecutionPlanPath: filepath.Join(cwd, ".agent", "context", "release.plan.md"),
	}
	raw, _ := json.Marshal(body)

	prev := forkThread
	forkThread = func(_ context.Context, _ *Service, params op.ThreadForkParams) (*op.ThreadMeta, error) {
		captured = params
		return &op.ThreadMeta{
			ThreadID:          "thread-child",
			AgentID:           params.AgentID,
			CWD:               params.CWD,
			ChatPath:          params.ChatPath,
			Title:             params.Title,
			ParentThreadID:    params.SourceThreadID,
			PlanPath:          params.PlanPath,
			ExecutionPlanPath: params.ExecutionPlanPath,
		}, nil
	}
	defer func() {
		forkThread = prev
	}()

	req := httptest.NewRequest("POST", "/v1/thread/fork", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/thread/fork", NewForkHandler(&Service{}).Fork)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var res op.ThreadMeta
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.ThreadID != "thread-child" {
		t.Fatalf("threadID = %q, want thread-child", res.ThreadID)
	}
	if res.ParentThreadID != "thread-parent" {
		t.Fatalf("parentThreadID = %q, want thread-parent", res.ParentThreadID)
	}
	if filepath.Dir(res.ChatPath) != filepath.Join(cwd, ".agent", "chat") {
		t.Fatalf("chatPath should be under cwd/.agent/chat; got %s", res.ChatPath)
	}
	if captured.CWD != cwd {
		t.Fatalf("captured cwd = %q, want %q", captured.CWD, cwd)
	}
	if res.PlanPath != "" {
		t.Fatalf("planPath = %q, want empty", res.PlanPath)
	}
	bodyBytes, err := os.ReadFile(res.ChatPath)
	if err != nil {
		t.Fatalf("ReadFile(chat markdown): %v", err)
	}
	text := string(bodyBytes)
	if strings.Contains(text, "parent_chat:") {
		t.Fatalf("chat markdown = %q, want no parent_chat frontmatter", text)
	}
	if strings.Contains(text, "\nid: ") {
		t.Fatalf("chat markdown = %q, want no markdown id frontmatter", text)
	}
	records, err := chatindex.ReadFileIndex(cwd)
	if err != nil {
		t.Fatalf("ReadFileIndex: %v", err)
	}
	if len(records) != 1 || records[0].FileID != res.FileID || records[0].ThreadID != "thread-child" || records[0].Path != res.ChatPath {
		t.Fatalf("chat index = %+v, want child file record", records)
	}
}

func TestForkHandler_UsesExplicitChatBaseDirAndFileName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agentCwd := t.TempDir()
	chatBaseDir := t.TempDir()
	planPath := filepath.Join(chatBaseDir, ".agent", "context", "website-hero-font-refresh.md")
	var captured op.ThreadForkParams
	body := ChatForkParams{
		SourceThreadID:    "thread-parent",
		CWD:               agentCwd,
		AgentID:           "agent-test",
		Title:             "Website Hero Font Refresh Build",
		ChatBaseDir:       chatBaseDir,
		ChatFileName:      "build-website-hero-font-refresh.md",
		ExecutionPlanPath: planPath,
	}
	raw, _ := json.Marshal(body)

	prev := forkThread
	forkThread = func(_ context.Context, _ *Service, params op.ThreadForkParams) (*op.ThreadMeta, error) {
		captured = params
		return &op.ThreadMeta{
			ThreadID:          "thread-child",
			AgentID:           params.AgentID,
			CWD:               params.CWD,
			ChatPath:          params.ChatPath,
			Title:             params.Title,
			ParentThreadID:    params.SourceThreadID,
			ExecutionPlanPath: params.ExecutionPlanPath,
		}, nil
	}
	defer func() {
		forkThread = prev
	}()

	req := httptest.NewRequest("POST", "/v1/thread/fork", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/thread/fork", NewForkHandler(&Service{}).Fork)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	expectedPath := filepath.Join(chatBaseDir, ".agent", "chat", "build-website-hero-font-refresh.md")
	if captured.ChatPath != expectedPath {
		t.Fatalf("captured chatPath = %q, want %q", captured.ChatPath, expectedPath)
	}
	if captured.CWD != agentCwd {
		t.Fatalf("captured cwd = %q, want %q", captured.CWD, agentCwd)
	}
}

func TestForkHandler_RejectsInvalidExplicitChatFileName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := ChatForkParams{
		SourceThreadID: "thread-parent",
		CWD:            t.TempDir(),
		AgentID:        "agent-test",
		Title:          "Invalid",
		ChatBaseDir:    t.TempDir(),
		ChatFileName:   "../bad.md",
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/thread/fork", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/thread/fork", NewForkHandler(&Service{}).Fork)
	router.ServeHTTP(rr, req)

	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}
