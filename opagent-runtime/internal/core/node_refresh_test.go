package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/internal/scan"
)

func TestRefreshNodeCacheSkipsSlowHTTPToolServer(t *testing.T) {
	cache.Flush()
	config.SetSystem(&op.SystemConfig{HostID: "test-host", Env: op.EnvLocal})
	t.Cleanup(func() {
		CloseDaemonConnections()
		cache.Flush()
	})

	previousTimeout := nodeRefreshHTTPToolProbeTimeout
	nodeRefreshHTTPToolProbeTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		nodeRefreshHTTPToolProbeTimeout = previousTimeout
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	t.Cleanup(server.Close)

	baseDir := t.TempDir()
	writeNodeRefreshTestFile(t, filepath.Join(baseDir, "tools", "slow-http", "TOOL.md"), fmt.Sprintf("---\nname: slow-http\nrun:\n  daemon: true\n  url: %q\n---\n", server.URL))
	writeNodeRefreshTestFile(t, filepath.Join(baseDir, "agents", "agent-a", ".agent", "AGENT.md"), "---\nname: agent-a\n---\nagent prompt\n")

	startedAt := time.Now()
	if err := RefreshNodeCache(context.Background(), scan.ScanOptions{UID: "user-test", BaseDir: baseDir}); err != nil {
		t.Fatalf("RefreshNodeCache(): %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 750*time.Millisecond {
		t.Fatalf("RefreshNodeCache() took %s, want slow HTTP tool probe bounded", elapsed)
	}

	nodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)
	if got := countNodeRefreshTestNodesByKind(nodes, op.NodeKindAgent); got != 1 {
		t.Fatalf("agent node count = %d, want 1", got)
	}
	if got := countNodeRefreshTestNodesByKind(nodes, op.NodeKindTools); got != 0 {
		t.Fatalf("tools node count = %d, want 0", got)
	}
}

func writeNodeRefreshTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func countNodeRefreshTestNodesByKind(nodes []op.OpNode, kind op.NodeKind) int {
	count := 0
	for _, node := range nodes {
		if node.Kind == string(kind) {
			count++
		}
	}
	return count
}
