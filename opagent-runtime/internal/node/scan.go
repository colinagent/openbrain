package node

import (
	"context"

	"github.com/colinagent/openbrain/opagent-runtime/internal/core"
	"github.com/colinagent/openbrain/opagent-runtime/internal/scan"
)

func RefreshNodes(ctx context.Context, opts scan.ScanOptions) error {
	return core.RefreshNodeCache(ctx, opts)
}
