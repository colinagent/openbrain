package run

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

func writeTestAgent(t *testing.T, baseDir, name string) {
	t.Helper()
	agentDir := filepath.Join(baseDir, "agents", name, ".agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	content := "---\nname: " + name + "\n---\n"
	if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}
}

func TestReportHeartbeatOnceSendsHostWithoutDirectoryAgents(t *testing.T) {
	baseDir := t.TempDir()
	writeTestAgent(t, baseDir, "coder")

	sysCfg := &op.SystemConfig{
		BaseDir:  baseDir,
		Env:      op.EnvLocal,
		HostID:   "host-a1b2",
		HostName: "host-a1b2-name",
	}
	config.SetSystem(sysCfg)

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var env heartbeatEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Fatalf("decode heartbeat: %v", err)
		}
		if env.Instance.ID != "host-a1b2" {
			t.Fatalf("instance.id = %q, want %q", env.Instance.ID, "host-a1b2")
		}
		if env.Instance.BaseDir != baseDir {
			t.Fatalf("instance.baseDir = %q, want %q", env.Instance.BaseDir, baseDir)
		}
		if env.Connections == nil {
			t.Fatal("connections = nil, want empty runtime connections object")
		}
		if len(env.Connections.Runtime) != 0 {
			t.Fatalf("runtime connections len = %d, want 0 without active connections", len(env.Connections.Runtime))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	sent, err := reportHeartbeatOnce(context.Background(), server.Client(), sysCfg, &op.UserConfig{
		Auth: &op.AuthConfig{
			Gateway: server.URL,
			Token:   "token-123",
		},
	})
	if err != nil {
		t.Fatalf("reportHeartbeatOnce() error = %v", err)
	}
	if !sent {
		t.Fatal("reportHeartbeatOnce() = false, want true")
	}
	if authHeader != "Bearer token-123" {
		t.Fatalf("authorization header = %q, want bearer token", authHeader)
	}
}

func TestReportHeartbeatOnceSkipsWithoutAuth(t *testing.T) {
	baseDir := t.TempDir()
	writeTestAgent(t, baseDir, "coder")

	sysCfg := &op.SystemConfig{
		BaseDir: baseDir,
		Env:     op.EnvLocal,
		HostID:  "host-a1b2",
	}
	config.SetSystem(sysCfg)

	sent, err := reportHeartbeatOnce(context.Background(), &http.Client{Timeout: time.Second}, sysCfg, &op.UserConfig{})
	if err != nil {
		t.Fatalf("reportHeartbeatOnce() error = %v", err)
	}
	if sent {
		t.Fatal("reportHeartbeatOnce() = true, want false")
	}
}

func TestStartHeartbeatReporterSkipsWhenDisabled(t *testing.T) {
	baseDir := t.TempDir()
	writeTestAgent(t, baseDir, "coder")

	enabled := false
	sysCfg := &op.SystemConfig{
		BaseDir:  baseDir,
		Env:      op.EnvLocal,
		HostID:   "host-a1b2",
		HostName: "host-a1b2-name",
		Heartbeat: op.HeartbeatConfig{
			Enabled:  &enabled,
			Interval: "10ms",
		},
	}
	config.SetSystem(sysCfg)

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go StartHeartbeatReporter(ctx)

	time.Sleep(50 * time.Millisecond)
	if got := requests.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}
