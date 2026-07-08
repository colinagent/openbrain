package core

import (
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

func metaServiceTier(meta op.Meta) string {
	return config.NormalizeServiceTier(metaString(meta, "serviceTier"))
}

func modelSupportsServiceTier(model *op.ModelConfig, serviceTier string) bool {
	serviceTier = config.NormalizeServiceTier(serviceTier)
	if model == nil || serviceTier == "" {
		return false
	}
	for _, raw := range model.ServiceTiers {
		if config.NormalizeServiceTier(raw) == serviceTier {
			return true
		}
	}
	return false
}

func serviceTierForModelMeta(model *op.ModelConfig, meta op.Meta) string {
	serviceTier := metaServiceTier(meta)
	if serviceTier == "" || !modelSupportsServiceTier(model, serviceTier) {
		return ""
	}
	return serviceTier
}

func serviceTierForMeta(meta op.Meta) string {
	return metaServiceTier(meta)
}
