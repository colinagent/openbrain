package chat

import (
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type toolArgumentsMap map[string]any

func (args toolArgumentsMap) stringValue(key string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	value, ok := args[key].(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

func toolArgumentsFromTurnResult(toolResult op.TurnResultToolResult) (toolArgumentsMap, bool) {
	if len(toolResult.ArgumentsObject) > 0 {
		return toolArgumentsMap(toolResult.ArgumentsObject), true
	}
	return nil, false
}
