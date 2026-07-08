package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func normalizeUserMessage(content op.Content) (op.Message, error) {
	msg, err := op.DecodeUserMessageContent(content)
	if err != nil {
		return op.Message{}, err
	}
	if !userMessageHasBody(msg) {
		return op.Message{}, fmt.Errorf("user message requires content or content_parts")
	}
	return msg, nil
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

func normalizeUserMessageForThread(content op.Content, chatPath string) (op.Content, error) {
	_ = chatPath
	msg, err := normalizeUserMessage(content)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal normalized user message: %w", err)
	}
	return op.NewJsonContentRaw(raw), nil
}
