package op

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeUserMessageContentRejectsImageContentParts(t *testing.T) {
	raw, err := json.Marshal(Message{
		Role: RoleUser,
		ContentParts: []ContentPart{
			{Type: "text", Text: "look"},
			{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,AAA", Detail: "auto"}},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	_, err = DecodeUserMessageContent(NewJsonContentRaw(raw))
	if err == nil {
		t.Fatal("expected image content_parts to be rejected")
	}
	if !strings.Contains(err.Error(), "markdown paths") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeUserMessageContentAllowsTextContentParts(t *testing.T) {
	raw, err := json.Marshal(Message{
		Role:         RoleUser,
		ContentParts: []ContentPart{{Type: "text", Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	msg, err := DecodeUserMessageContent(NewJsonContentRaw(raw))
	if err != nil {
		t.Fatalf("DecodeUserMessageContent: %v", err)
	}
	if len(msg.ContentParts) != 1 || msg.ContentParts[0].Text != "hello" {
		t.Fatalf("unexpected message: %+v", msg)
	}
}
