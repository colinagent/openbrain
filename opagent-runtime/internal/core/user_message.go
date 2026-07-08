package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func buildUserMessage(content op.Content) (op.Message, error) {
	msg, err := op.DecodeUserMessageContent(content)
	if err != nil {
		return op.Message{}, err
	}
	if !userMessageHasBody(msg) {
		return op.Message{}, fmt.Errorf("user message requires content or content_parts")
	}
	return msg, nil
}

func contentFromUserMessage(msg op.Message) (op.Content, error) {
	if msg.Role != op.RoleUser {
		return nil, fmt.Errorf("contentFromUserMessage requires user role, got %q", msg.Role)
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal user message: %w", err)
	}
	return op.NewJsonContentRaw(raw), nil
}

func userMessageHasBody(msg op.Message) bool {
	if strings.TrimSpace(msg.Content) != "" {
		return true
	}
	for _, part := range msg.ContentParts {
		if strings.TrimSpace(part.Text) != "" {
			return true
		}
	}
	return false
}
