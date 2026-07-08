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

func TestCreateHandler_NewChat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cwd := t.TempDir()
	chatindex.SetBaseDir(cwd)
	t.Cleanup(func() { chatindex.SetBaseDir("") })
	body := ChatCreateParams{CWD: cwd, UserInput: "hello world", AgentID: "agent-test"}
	raw, _ := json.Marshal(body)

	prev := createThread
	createThread = func(_ context.Context, _ *Service, params op.ThreadCreateParams) (*op.ThreadCreateResult, error) {
		chatPath := filepath.Join(params.CWD, ".agent", "chat", "hello-world.md")
		return &op.ThreadCreateResult{
			ThreadID: "thread-test",
			FileID:   "file-test",
			CWD:      params.CWD,
			Title:    params.Title,
			Path:     chatPath,
			ChatPath: chatPath,
		}, nil
	}
	defer func() {
		createThread = prev
	}()

	req := httptest.NewRequest("POST", "/v1/thread/create", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	h := NewCreateHandler(nil)
	router.POST("/v1/thread/create", h.Create)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var res ChatCreateResult
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.ThreadID == "" || res.Title == "" || res.ChatPath == "" {
		t.Fatalf("expected threadID, title, chatPath set; got %+v", res)
	}
	if filepath.Dir(res.ChatPath) != filepath.Join(cwd, ".agent", "chat") {
		t.Fatalf("chatFile should be under cwd/.agent/chat; got %s", res.ChatPath)
	}
	bodyBytes, err := os.ReadFile(res.ChatPath)
	if err != nil {
		t.Fatalf("ReadFile(chat markdown): %v", err)
	}
	text := string(bodyBytes)
	if !strings.Contains(text, "thread: thread-test") {
		t.Fatalf("chat markdown = %q, want thread frontmatter", text)
	}
	if strings.Contains(text, "\nid: ") {
		t.Fatalf("chat markdown = %q, want no markdown id frontmatter", text)
	}
	if res.InitialContent != text {
		t.Fatalf("initialContent = %q, want %q", res.InitialContent, text)
	}
	records, err := chatindex.ReadFileIndex(cwd)
	if err != nil {
		t.Fatalf("ReadFileIndex: %v", err)
	}
	if len(records) != 1 || records[0].FileID != res.FileID || records[0].ThreadID != "thread-test" || records[0].Path != res.ChatPath {
		t.Fatalf("chat index = %+v, want file record", records)
	}
}

func TestCreateHandler_NewChatWithEmptyInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cwd := t.TempDir()
	chatindex.SetBaseDir(cwd)
	t.Cleanup(func() { chatindex.SetBaseDir("") })
	body := ChatCreateParams{CWD: cwd, UserInput: "", AgentID: "agent-test"}
	raw, _ := json.Marshal(body)

	prev := createThread
	createThread = func(_ context.Context, _ *Service, params op.ThreadCreateParams) (*op.ThreadCreateResult, error) {
		if params.Title != "Untitled Chat" {
			t.Fatalf("Title = %q, want Untitled Chat", params.Title)
		}
		chatPath := filepath.Join(params.CWD, ".agent", "chat", "untitled-chat.md")
		return &op.ThreadCreateResult{
			ThreadID: "thread-empty",
			FileID:   "file-empty",
			CWD:      params.CWD,
			Title:    params.Title,
			Path:     chatPath,
			ChatPath: chatPath,
		}, nil
	}
	defer func() {
		createThread = prev
	}()

	req := httptest.NewRequest("POST", "/v1/thread/create", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/thread/create", NewCreateHandler(nil).Create)
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var res ChatCreateResult
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.Title != "Untitled Chat" {
		t.Fatalf("Title = %q, want Untitled Chat", res.Title)
	}
	if filepath.Base(res.ChatPath) != "untitled-chat.md" {
		t.Fatalf("ChatPath = %q, want untitled-chat.md basename", res.ChatPath)
	}
	if !strings.Contains(res.InitialContent, "thread: thread-empty") {
		t.Fatalf("initialContent = %q, want thread frontmatter", res.InitialContent)
	}
	records, err := chatindex.ReadFileIndex(cwd)
	if err != nil {
		t.Fatalf("ReadFileIndex: %v", err)
	}
	if len(records) != 1 || records[0].ThreadID != "thread-empty" || records[0].Path != res.ChatPath {
		t.Fatalf("chat index = %+v, want empty-input file record", records)
	}
}

func TestCreateHandler_MissingCWD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := ChatCreateParams{UserInput: "hi", AgentID: "agent-test"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v1/thread/create", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/thread/create", NewCreateHandler(nil).Create)
	router.ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("expected 400 for missing cwd, got %d", rr.Code)
	}
}
