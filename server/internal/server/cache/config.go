package cache

import (
	"fmt"
	"strings"

	hostcfg "github.com/colinagent/openbrain/server/internal/server/hostcfg"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type configLoader = func() (*op.Config, error)

const configLoaderOverrideKey = "__server_cache_config_loader_override__"

func loadLatestConfig() (*op.Config, error) {
	if loader, ok := Get[configLoader](configLoaderOverrideKey); ok && loader != nil {
		return loader()
	}

	host := hostcfg.Get()
	if host == nil {
		return nil, fmt.Errorf("host session is nil")
	}
	cfg, err := host.GetConfig()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	return cfg, nil
}

// GetUserID returns the current profile uid.
func GetUserID() (string, error) {
	cfg, err := loadLatestConfig()
	if err != nil {
		return "", err
	}
	if cfg.User == nil || cfg.User.Profile == nil {
		return "", fmt.Errorf("user profile is nil")
	}
	return strings.TrimSpace(cfg.User.Profile.UID), nil
}
