package core

import (
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestServiceTierForMetaPrefersTurnMeta(t *testing.T) {
	if got := serviceTierForMeta(op.Meta{"serviceTier": "priority"}); got != "priority" {
		t.Fatalf("serviceTierForMeta(priority) = %q, want priority", got)
	}
	if got := serviceTierForMeta(op.Meta{"serviceTier": "flex"}); got != "flex" {
		t.Fatalf("serviceTierForMeta(flex) = %q, want flex", got)
	}
}

func TestServiceTierForModelMetaRequiresModelSupport(t *testing.T) {
	meta := op.Meta{"serviceTier": "priority"}
	if got := serviceTierForModelMeta(&op.ModelConfig{}, meta); got != "" {
		t.Fatalf("serviceTierForModelMeta(unsupported) = %q, want empty", got)
	}
	if got := serviceTierForModelMeta(&op.ModelConfig{ServiceTiers: []string{"priority"}}, meta); got != "priority" {
		t.Fatalf("serviceTierForModelMeta(priority) = %q, want priority", got)
	}
}
