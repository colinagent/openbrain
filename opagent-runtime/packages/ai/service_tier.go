package ai

import (
	"fmt"
	"strings"
)

const (
	ServiceTierPriority = "priority"
	ServiceTierFlex     = "flex"
)

func NormalizeServiceTier(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ServiceTierPriority:
		return ServiceTierPriority
	case ServiceTierFlex:
		return ServiceTierFlex
	default:
		return ""
	}
}

func ResponsesAPIServiceTier(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	switch NormalizeServiceTier(value) {
	case ServiceTierPriority:
		return ServiceTierPriority, nil
	case ServiceTierFlex:
		return ServiceTierFlex, nil
	default:
		return "", fmt.Errorf("unsupported service tier %q", value)
	}
}
