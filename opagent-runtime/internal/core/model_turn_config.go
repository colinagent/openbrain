package core

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func metaPositiveInt64(meta op.Meta, key string) int64 {
	if meta == nil {
		return 0
	}
	value, ok := meta[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return int64(typed)
		}
	case int64:
		if typed > 0 {
			return typed
		}
	case float64:
		const maxInt64AsFloat = float64(1<<63 - 1)
		if typed > 0 && typed <= maxInt64AsFloat && typed == float64(int64(typed)) {
			return int64(typed)
		}
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil && parsed > 0 {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func effectiveContextWindowForMeta(meta op.Meta, modelContextWindow int64) int64 {
	requested := metaPositiveInt64(meta, "contextWindow")
	if requested <= 0 {
		return modelContextWindow
	}
	if modelContextWindow > 0 && requested > modelContextWindow {
		return modelContextWindow
	}
	return requested
}
