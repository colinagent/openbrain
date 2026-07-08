package chat

import (
	"encoding/json"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestNormalizeUserMessageTextContent(t *testing.T) {
	msg, err := normalizeUserMessage(&op.TextContent{Text: "hello"})
	if err != nil {
		t.Fatalf("normalizeUserMessage: %v", err)
	}
	if msg.Role != op.RoleUser {
		t.Fatalf("expected role user, got %q", msg.Role)
	}
	if msg.Content != "hello" {
		t.Fatalf("expected text content, got %q", msg.Content)
	}
}

func TestNormalizeUserMessageRejectsEmptyTextContent(t *testing.T) {
	if _, err := normalizeUserMessage(&op.TextContent{Text: " \n\t "}); err == nil {
		t.Fatal("expected empty text content to be rejected")
	} else if got := err.Error(); got != "user message requires content or content_parts" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestNormalizeUserMessageForThreadPreservesMarkdownImagePathText(t *testing.T) {
	content := &op.TextContent{Text: "![image-1](/tmp/work/.agent/assets/images/image-1.png)\n\nlook"}

	normalized, err := normalizeUserMessageForThread(content, "/tmp/thread.md")
	if err != nil {
		t.Fatalf("normalizeUserMessageForThread: %v", err)
	}
	msg := decodeUserMessageContent(t, normalized)
	if got := msg.Content; got != "![image-1](/tmp/work/.agent/assets/images/image-1.png)\n\nlook" {
		t.Fatalf("content changed: %q", got)
	}
}

func decodeUserMessageContent(t *testing.T, content op.Content) op.Message {
	t.Helper()
	jsonContent, ok := content.(*op.JsonContent)
	if !ok {
		t.Fatalf("expected JsonContent, got %T", content)
	}
	var msg op.Message
	if err := json.Unmarshal(jsonContent.Raw, &msg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return msg
}
