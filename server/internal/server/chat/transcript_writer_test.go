package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/server/internal/server/cache"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func resetTranscriptWriterTestState() {
	transcriptMu.Lock()
	defer transcriptMu.Unlock()
	transcriptTurns = make(map[string]*transcriptTurnState)
}

func newTranscriptTestConfigSource(t *testing.T) (*projectionConfigSource, string) {
	t.Helper()
	source := newProjectionConfigSource(defaultProjectionDisplayMetadata())
	installProjectionConfigLoader(source)
	t.Cleanup(func() {
		cache.Delete(testConfigLoaderOverrideKey)
	})
	return source, t.TempDir()
}

func TestTranscriptWriter_AppendsOnlyUserStepToMarkdown(t *testing.T) {
	resetTranscriptWriterTestState()
	t.Cleanup(resetTranscriptWriterTestState)

	_, baseDir := newTranscriptTestConfigSource(t)
	chatPath := filepath.Join(baseDir, ".agent", "chat", "demo.md")
	meta := op.Meta{
		"threadID": "thread-demo",
		"chatPath": chatPath,
		"agentID":  "agent-demo",
		"title":    "Demo",
		"turnID":   "turn-1",
	}
	meta["stepSeq"] = 1

	if err := appendUserStepToTranscript(meta, op.NewUserMessage("hello user")); err != nil {
		t.Fatalf("appendUserStepToTranscript(): %v", err)
	}

	raw, err := os.ReadFile(chatPath)
	if err != nil {
		t.Fatalf("os.ReadFile(): %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "@user-colin") {
		t.Fatalf("chat markdown missing user marker: %s", text)
	}
	if strings.Contains(text, "![Colin](/avatars/user.png)") {
		t.Fatalf("chat markdown should not use avatar image headers: %s", text)
	}
	if !strings.Contains(text, "@user-colin\n\nhello user") {
		t.Fatalf("chat markdown missing plain user markdown: %s", text)
	}
	if strings.Contains(text, "```user") {
		t.Fatalf("user markdown should not be fenced: %s", text)
	}
}

func TestTranscriptWriter_ToolResultStepDoesNotWriteMarkdown(t *testing.T) {
	resetTranscriptWriterTestState()
	t.Cleanup(resetTranscriptWriterTestState)

	_, baseDir := newTranscriptTestConfigSource(t)
	chatPath := filepath.Join(baseDir, ".agent", "chat", "tool.md")
	meta := op.Meta{
		"threadID":        "thread-tool",
		"chatPath":        chatPath,
		"agentID":         "agent-demo",
		"title":           "Tool",
		"turnID":          "turn-1",
		"stepSeq":         1,
		"type":            "tool_result_step",
		"argumentsObject": map[string]any{"command": "ls"},
	}

	service := &Service{}
	service.HandleHostNotification(&op.InfoNotificationServerRequest{
		Params: &op.InfoNotificationParams{
			OpCode: op.NotifyMessage,
			Meta:   meta,
		},
	})

	if _, err := os.Stat(chatPath); !os.IsNotExist(err) {
		t.Fatalf("tool result step should not create chat markdown, stat err = %v", err)
	}
}

func TestTranscriptWriter_WritesMarkdownUserImagesInline(t *testing.T) {
	resetTranscriptWriterTestState()
	t.Cleanup(resetTranscriptWriterTestState)

	_, baseDir := newTranscriptTestConfigSource(t)
	chatPath := filepath.Join(baseDir, ".agent", "chat", "images.md")
	meta := op.Meta{
		"threadID": "thread-images",
		"chatPath": chatPath,
		"agentID":  "agent-demo",
		"title":    "Images",
		"turnID":   "turn-1",
		"stepSeq":  1,
	}

	msg := op.Message{
		Role:    op.RoleUser,
		Content: "look\n\n![test.png](./assets/test.png)",
	}
	if err := appendUserStepToTranscript(meta, msg); err != nil {
		t.Fatalf("appendUserStepToTranscript(): %v", err)
	}

	raw, err := os.ReadFile(chatPath)
	if err != nil {
		t.Fatalf("os.ReadFile(): %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "look\n\n![test.png](./assets/test.png)") {
		t.Fatalf("missing image markdown: %s", text)
	}
	if strings.Contains(text, "```user") {
		t.Fatalf("user markdown should not be fenced: %s", text)
	}
}

func TestTranscriptWriter_WritesNativeMarkdownImageInline(t *testing.T) {
	resetTranscriptWriterTestState()
	t.Cleanup(resetTranscriptWriterTestState)

	_, baseDir := newTranscriptTestConfigSource(t)
	chatPath := filepath.Join(baseDir, ".agent", "chat", "native-images.md")
	imagePath := filepath.Join(baseDir, ".agent", "assets", "images", "test.png")
	meta := op.Meta{
		"threadID": "thread-native-images",
		"chatPath": chatPath,
		"agentID":  "agent-demo",
		"title":    "Native Images",
		"turnID":   "turn-1",
		"stepSeq":  1,
	}

	msg := op.Message{
		Role:    op.RoleUser,
		Content: "![test.png](" + imagePath + ")\n\nlook",
	}
	if err := appendUserStepToTranscript(meta, msg); err != nil {
		t.Fatalf("appendUserStepToTranscript(): %v", err)
	}

	raw, err := os.ReadFile(chatPath)
	if err != nil {
		t.Fatalf("os.ReadFile(): %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "![test.png]("+imagePath+")\n\nlook") {
		t.Fatalf("missing native image markdown: %s", text)
	}
	if strings.Contains(text, "```user") {
		t.Fatalf("user markdown should not be fenced: %s", text)
	}
}
