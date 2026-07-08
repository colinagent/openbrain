package chat

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

func loadCanonicalMessagesFromThreadFile(threadFilePath string) ([]ai.ConversationMessage, error) {
	trimmedPath := strings.TrimSpace(threadFilePath)
	if trimmedPath == "" {
		return nil, nil
	}
	f, err := os.Open(trimmedPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	messages := make([]ai.ConversationMessage, 0, 16)
	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNo++
		if lineNo == 1 {
			continue
		}
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &typed); err != nil {
			continue
		}
		switch strings.TrimSpace(typed.Type) {
		case op.ThreadEntryTypeCanonicalMessage:
			var entry op.ThreadCanonicalMessageEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			messages = append(messages, entry.Message)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func threadRequiresContinuation(threadFilePath string) (bool, error) {
	canonicalMessages, err := loadCanonicalMessagesFromThreadFile(threadFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if len(canonicalMessages) > 0 {
		return ai.ContinuationRequiredForCanonicalMessages(canonicalMessages), nil
	}
	return false, nil
}
