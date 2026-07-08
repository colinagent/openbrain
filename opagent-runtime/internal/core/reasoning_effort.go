package core

import (
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func isToggleReasoningModel(model *op.ModelConfig) bool {
	if model == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(model.ReasoningControl), "toggle")
}

func resolveProviderReasoningEnabled(level string, model *op.ModelConfig) *bool {
	if !isToggleReasoningModel(model) {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(level))
	if normalized == "" || normalized == "off" {
		disabled := false
		return &disabled
	}
	enabled := true
	return &enabled
}

func resolveProviderReasoningEffort(level string, model *op.ModelConfig) string {
	normalized := strings.TrimSpace(level)
	if normalized == "" || strings.EqualFold(normalized, "off") {
		return ""
	}
	if isToggleReasoningModel(model) {
		return ""
	}

	if model == nil || len(model.ReasoningLevels) == 0 {
		return normalized
	}

	available := make(map[string]struct{}, len(model.ReasoningLevels))
	for _, raw := range model.ReasoningLevels {
		level := strings.TrimSpace(raw)
		if level == "" {
			continue
		}
		available[level] = struct{}{}
	}
	if len(available) == 0 {
		return normalized
	}

	return normalized
}
