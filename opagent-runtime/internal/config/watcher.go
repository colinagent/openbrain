package config

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const (
	userConfigReloadDebounceDelay = 200 * time.Millisecond
	userConfigReloadRetryDelay    = 100 * time.Millisecond
	userConfigReloadMaxAttempts   = 3
)

var userConfigWatchFiles = []string{"auth.json", "models.json", "nodes.json", "profile.json"}

// StartUserConfigWatcher watches baseDir/configs/user and hot-reloads local user config files.
func StartUserConfigWatcher(ctx context.Context, onReload func(changedPath string)) error {
	sysCfg := GetSystem()
	if sysCfg == nil || sysCfg.BaseDir == "" {
		return nil
	}

	configDir := filepath.Join(sysCfg.BaseDir, "configs", "user")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(configDir); err != nil {
		watcher.Close()
		return err
	}

	go func() {
		defer watcher.Close()
		var (
			reloadTimer  *time.Timer
			reloadTimerC <-chan time.Time
			pendingPath  string
		)
		stopReloadTimer := func() {
			if reloadTimer == nil {
				return
			}
			if !reloadTimer.Stop() {
				select {
				case <-reloadTimer.C:
				default:
				}
			}
		}
		defer stopReloadTimer()

		scheduleReload := func(changedPath string) {
			pendingPath = changedPath
			if reloadTimer == nil {
				reloadTimer = time.NewTimer(userConfigReloadDebounceDelay)
			} else {
				stopReloadTimer()
				reloadTimer.Reset(userConfigReloadDebounceDelay)
			}
			reloadTimerC = reloadTimer.C
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-reloadTimerC:
				reloadTimerC = nil
				reloadedCfg, err := reloadLocalUserProfileWithRetry()
				if err != nil {
					slog.Warn("failed to reload local user config", "path", pendingPath, "error", err)
					continue
				}
				slog.Info("reloaded local user config", "path", pendingPath, "uid", reloadedCfg.UID)
				if onReload != nil {
					onReload(pendingPath)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Warn("user config watcher error", "error", err)
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				mask := fsnotify.Create | fsnotify.Write | fsnotify.Rename | fsnotify.Remove
				if ev.Op&mask == 0 {
					continue
				}
				if !slices.Contains(userConfigWatchFiles, filepath.Base(ev.Name)) {
					continue
				}
				scheduleReload(ev.Name)
			}
		}
	}()
	return nil
}

func reloadLocalUserProfileWithRetry() (*op.UserProfile, error) {
	var (
		cfg *op.UserProfile
		err error
	)
	for attempt := 0; attempt < userConfigReloadMaxAttempts; attempt++ {
		cfg, err = LoadLocalUserProfile()
		if err == nil {
			return cfg, nil
		}
		if attempt == userConfigReloadMaxAttempts-1 {
			break
		}
		time.Sleep(userConfigReloadRetryDelay)
	}
	return nil, err
}
