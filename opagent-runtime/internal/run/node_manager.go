package run

import (
	"context"
	"os"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	sharedmarketplace "github.com/colinagent/openbrain/opagent-runtime/marketplace"
)

func reconcileManagedMarketplaceNodes(ctx context.Context, cfg *op.SystemConfig) error {
	if cfg == nil {
		return nil
	}
	baseDir := strings.TrimSpace(cfg.BaseDir)
	if baseDir == "" {
		return nil
	}
	service := sharedmarketplace.NewService(baseDir, sharedmarketplace.Options{
		CatalogURL: resolveMarketplaceCatalogURL(),
	})
	return service.Reconcile(ctx)
}

func resolveMarketplaceCatalogURL() string {
	if value := strings.TrimSpace(os.Getenv("OPAGENT_MARKETPLACE_INDEX_URL")); value != "" {
		return value
	}
	return sharedmarketplace.DefaultCatalogURL
}
