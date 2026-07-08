package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/core"
)

const (
	defaultHeartbeatInterval = 30 * time.Second
	defaultHeartbeatTimeout  = 5 * time.Second
)

type heartbeatEnvelope struct {
	SchemaVersion string                 `json:"schemaVersion"`
	SentAt        time.Time              `json:"sentAt"`
	Instance      heartbeatInstance      `json:"instance"`
	Connections   *heartbeatConns        `json:"connections,omitempty"`
	Updater       *op.RuntimeUpdateState `json:"updater,omitempty"`
}

type heartbeatInstance struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname,omitempty"`
	Env      string `json:"env,omitempty"`
	BaseDir  string `json:"baseDir"`
	PID      int    `json:"pid,omitempty"`
}

type heartbeatConns struct {
	Runtime []*core.ConnectionRuntimeSnapshot `json:"runtime,omitempty"`
}

func StartHeartbeatReporter(ctx context.Context) {
	sysCfg := config.GetSystem()
	if sysCfg == nil {
		return
	}
	if sysCfg.Heartbeat.Enabled != nil && !*sysCfg.Heartbeat.Enabled {
		return
	}

	interval := resolveHeartbeatInterval(sysCfg.Heartbeat.Interval)
	client := &http.Client{Timeout: defaultHeartbeatTimeout}

	send := func() {
		currentSys := config.GetSystem()
		if currentSys == nil {
			return
		}
		sent, err := reportHeartbeatOnce(ctx, client, currentSys, config.GetUserConfig())
		if err != nil {
			slog.Warn("heartbeat: report failed", "error", err)
			return
		}
		if sent {
			slog.Debug("heartbeat: reported")
		}
	}

	send()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

func resolveHeartbeatInterval(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultHeartbeatInterval
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		slog.Warn("heartbeat: invalid interval, using default", "interval", raw, "default", defaultHeartbeatInterval.String())
		return defaultHeartbeatInterval
	}
	return interval
}

func reportHeartbeatOnce(
	ctx context.Context,
	client *http.Client,
	sysCfg *op.SystemConfig,
	userCfg *op.UserConfig,
) (bool, error) {
	endpoint, token, ready := resolveHeartbeatTarget(userCfg)
	if !ready {
		return false, nil
	}
	if err := sendHeartbeat(ctx, client, endpoint, token, sysCfg); err != nil {
		return false, err
	}
	return true, nil
}

func resolveHeartbeatTarget(userCfg *op.UserConfig) (endpoint string, token string, ready bool) {
	if userCfg == nil || userCfg.Auth == nil {
		return "", "", false
	}
	gateway := strings.TrimRight(strings.TrimSpace(userCfg.Auth.Gateway), "/")
	token = strings.TrimSpace(userCfg.Auth.Token)
	if gateway == "" || token == "" {
		return "", "", false
	}
	return gateway + "/api/v1/heartbeat/heartbeats", token, true
}

func sendHeartbeat(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	token string,
	sysCfg *op.SystemConfig,
) error {
	if client == nil {
		client = &http.Client{Timeout: defaultHeartbeatTimeout}
	}
	env, err := buildHeartbeatEnvelope(sysCfg)
	if err != nil {
		return err
	}

	raw, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("create heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("heartbeat response %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func buildHeartbeatEnvelope(sysCfg *op.SystemConfig) (*heartbeatEnvelope, error) {
	if sysCfg == nil {
		return nil, fmt.Errorf("system config is nil")
	}
	hostID := strings.TrimSpace(sysCfg.HostID)
	if hostID == "" {
		return nil, fmt.Errorf("system hostID is required")
	}
	baseDir := strings.TrimSpace(sysCfg.BaseDir)
	if baseDir == "" {
		return nil, fmt.Errorf("system baseDir is required")
	}

	runtimeConnections := core.ListActiveConnectionSnapshots(time.Now().UTC())

	hostname := strings.TrimSpace(sysCfg.HostName)
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	return &heartbeatEnvelope{
		SchemaVersion: "v1",
		SentAt:        time.Now().UTC(),
		Instance: heartbeatInstance{
			ID:       hostID,
			Hostname: hostname,
			Env:      strings.TrimSpace(sysCfg.Env),
			BaseDir:  baseDir,
			PID:      os.Getpid(),
		},
		Connections: &heartbeatConns{
			Runtime: runtimeConnections,
		},
		Updater: GetRuntimeUpdateSnapshot(),
	}, nil
}
