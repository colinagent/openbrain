package core

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestBuildUserMessageFromJsonContent(t *testing.T) {
	raw, err := json.Marshal(op.Message{
		Role:         op.RoleUser,
		ContentParts: []op.ContentPart{{Type: "text", Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	msg, err := buildUserMessage(op.NewJsonContentRaw(raw))
	if err != nil {
		t.Fatalf("buildUserMessage: %v", err)
	}
	if msg.Role != op.RoleUser {
		t.Fatalf("expected user role, got %q", msg.Role)
	}
	if len(msg.ContentParts) != 1 || strings.TrimSpace(msg.ContentParts[0].Text) != "hello" {
		t.Fatalf("unexpected content parts: %+v", msg.ContentParts)
	}
}

func TestBuildUserMessageRejectsEmptyTextContent(t *testing.T) {
	if _, err := buildUserMessage(&op.TextContent{Text: " \n\t "}); err == nil {
		t.Fatal("expected empty text content to be rejected")
	} else if got := err.Error(); got != "user message requires content or content_parts" {
		t.Fatalf("unexpected error: %q", got)
	}
}
