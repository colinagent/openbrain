package config

import (
	"log/slog"
	"path/filepath"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
)

func Parse(configDir string) error {
	resolvedConfigDir, err := common.ResolveAbsolutePath("", configDir)
	if err != nil {
		return err
	}
	configDir = resolvedConfigDir

	parseSecrets(configDir)
	parseSystemConfig(configDir)
	parseUserConfig(configDir)

	return nil
}

func parseSecrets(configDir string) {
	secretsPath := filepath.Join(configDir, "secrets.env")
	values, err := common.LoadSecretsFile(secretsPath)
	if err != nil {
		slog.Error("load secrets env", "path", secretsPath, "error", err)
		return
	}
	SetSecrets(values)
}

func parseUserConfig(configDir string) {
	// Canonical loader is LoadLocalUserProfile(), which also populates GetUserConfig().
	if _, err := LoadLocalUserProfile(); err != nil {
		slog.Error("load local user config", "error", err)
		return
	}
}

func parseSystemConfig(configDir string) {

	cfg := &op.SystemConfig{}
	// upsert to system.json
	sysDir := filepath.Join(configDir, "system")
	sysCfg := filepath.Join(sysDir, "system.json")

	if sysCfg, ok, err := readOptionalJSON[op.SystemConfig](sysCfg); err != nil {
		slog.Error("read system config", "error", err)
		return
	} else if ok {
		cfg = &sysCfg
	}

	// base Dir
	baseDir, err := common.ExpandHome(CmdBaseDir)
	if err != nil {
		slog.Warn("failed to resolve base dir", "error", err)
		return
	}
	slog.Info("resolved base dir", "baseDir", baseDir)
	cfg.BaseDir = baseDir

	// env
	cfg.Env = CmdEnv

	// host info (hostID, hostName, ips) — use resolved baseDir so hostID is set before SetSystem
	hostInfo := GetHostInfo(sysDir)
	cfg.HostID = hostInfo.HostID
	cfg.HostName = hostInfo.HostName
	cfg.Ips = hostInfo.Ips

	SetSystem(cfg)
}

func applyDefaults(cfg *op.SystemConfig) {
	if cfg.BaseDir == "" {
		cfg.BaseDir = DefaultBaseDir
	}

	// if cfg.LogDir == "" {
	// 	cfg.LogDir = DefaultLogDir
	// }
	// if cfg.Env == "" {
	// 	cfg.Env = DefaultEnv
	// }
	// if cfg.ObjectStore.Type == "" {
	// 	cfg.ObjectStore.Type = "fs"
	// }
	// if cfg.ObjectStore.FS.BaseDir == "" {
	// 	cfg.ObjectStore.FS.BaseDir = filepath.Join(cfg.BaseDir, "objects")
	// }
}

func resolvePath(baseDir string, path string) (string, error) {
	resolved, err := common.ResolveAbsolutePath(baseDir, path)
	if err != nil {
		return "", err
	}
	return resolved, nil
}
