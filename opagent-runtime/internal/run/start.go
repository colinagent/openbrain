package run

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/core"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/objectstore/factory"
	"github.com/colinagent/openbrain/opagent-runtime/internal/node"
	"github.com/colinagent/openbrain/opagent-runtime/internal/pidlock"
	"github.com/colinagent/openbrain/opagent-runtime/internal/scan"
)

func Start() {
	cfg := config.GetSystem()
	if cfg == nil {
		slog.Error("config is not initialized")
		os.Exit(1)
	}

	runDir := filepath.Join(cfg.BaseDir, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		slog.Error("failed to create run directory", "error", err)
		os.Exit(1)
	}
	if _, err := ensureDefaultConversationWorkspace(cfg); err != nil {
		slog.Error("failed to create default conversation workspace", "error", err)
		os.Exit(1)
	}
	markRuntimeProcessStarted(cfg.BaseDir)

	pidMgr := pidlock.New(runDir)
	if err := pidMgr.Acquire(); err != nil {
		slog.Error("failed to acquire pid lock, another instance may be running", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := pidMgr.Release(); err != nil {
			slog.Error("failed to release pid lock", "error", err)
		}
	}()
	if shouldBootstrapLegacyCanonicalWorkspace() {
		if err := ensureCanonicalWorkspaceBootstrap(cfg); err != nil {
			slog.Warn("failed to bootstrap legacy canonical workspace (continuing)", "error", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	memory.NewStorage(ctx)

	objStore, err := factory.NewDefault(ctx)
	if err != nil {
		slog.Error("failed to initialize object store", "error", err)
		os.Exit(1)
	}
	core.SetDefaultObjectStore(objStore)
	if err := core.StartCron(ctx, cfg); err != nil {
		slog.Error("failed to start cron", "error", err)
		os.Exit(1)
	}

	if strings.EqualFold(cfg.Env, op.EnvLocal) {
		if _, err := config.ReloadLocalUserConfig(); err != nil {
			slog.Warn("failed to load local user config", "error", err)
		} else {
			loadModels(ctx)
		}
		if err := config.StartUserConfigWatcher(ctx, func(changedPath string) {
			switch filepath.Base(strings.TrimSpace(changedPath)) {
			case "auth.json", "models.json", "profile.json":
				loadModels(ctx)
			}

			// refreshLocalNodes(ctx, cfg)
		}); err != nil {
			slog.Warn("failed to start local user config watcher", "error", err)
		}
	} else {
		loadModels(ctx)
	}

	if strings.EqualFold(cfg.Env, op.EnvLocal) {
		// scan local nodes
		refreshLocalNodes(ctx, cfg)
	}

	go core.StartNotify(ctx)
	go StartHeartbeatReporter(ctx)
	StartRuntimeUpdater(ctx)

	<-ctx.Done()
	core.StopCron()
	closed := core.CloseDaemonConnections()
	if closed > 0 {
		slog.Info("daemon connections cleaned on shutdown", "count", closed)
	}
	slog.Info("opagent.Run context done, closing bus")
}

// func upsertLocalUserSettings(ctx context.Context, cfg *op.SystemConfig) {
// 	if cfg == nil {
// 		return
// 	}
// 	userCfg := config.GetUserConfig()
// 	if userCfg == nil || userCfg.Profile == nil || strings.TrimSpace(userCfg.Profile.UID) == "" {
// 		return
// 	}
// 	settings := &op.UserSettings{
// 		UID:     strings.TrimSpace(userCfg.Profile.UID),
// 		BaseDir: cfg.BaseDir,
// 	}
// 	if err := core.GetStorage().UpsertUserSettings(ctx, settings); err != nil {
// 		slog.Error("failed to upsert user settings", "error", err)
// 	}
// }

func refreshLocalNodes(ctx context.Context, cfg *op.SystemConfig) {
	if cfg == nil {
		return
	}
	userCfg := config.GetUserConfig()
	uid := ""
	if userCfg != nil && userCfg.Profile != nil {
		uid = strings.TrimSpace(userCfg.Profile.UID)
	}
	err := node.RefreshNodes(ctx, scan.ScanOptions{
		UID:     uid,
		BaseDir: cfg.BaseDir,
	})
	if err != nil {
		slog.Error("failed to refresh nodes", "error", err)
		return
	}

	// opcodes: system/started
	SystemStartedConns(ctx)

}

// opcodes: system/started
func SystemStartedConns(ctx context.Context) {

	nodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)

	systemStartedNodes := make([]op.OpNode, 0, len(nodes))
	for i := range nodes {
		n := nodes[i]
		if !slices.Contains(n.OpCodes, op.SystemStarted) {
			continue
		}
		systemStartedNodes = append(systemStartedNodes, n)
	}
	for i := range systemStartedNodes {
		n := systemStartedNodes[i]
		if _, err := core.CreateConnection(ctx, &n); err != nil {
			slog.Warn("create system started connection", "nodeID", n.ID, "error", err)
			continue
		}
	}
	slog.Info("system started connections created", "count", len(systemStartedNodes))
}
