package core

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

func TestResolveRunHeaderValueUsesOpenBrainSession(t *testing.T) {
	baseDir := t.TempDir()
	writeTestAuthJSON(t, baseDir, "session-token")
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir})
	t.Cleanup(func() { config.SetSystem(nil) })

	got, err := resolveRunHeaderValue("Bearer {openbrain_session}")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Bearer session-token" {
		t.Fatalf("header value = %q, want bearer session token", got)
	}
}

func TestRunHeaderHTTPClientResolvesOpenBrainSessionPerRequest(t *testing.T) {
	baseDir := t.TempDir()
	writeTestAuthJSON(t, baseDir, "token-one")
	config.SetSystem(&op.SystemConfig{BaseDir: baseDir})
	t.Cleanup(func() { config.SetSystem(nil) })

	var (
		mu     sync.Mutex
		values []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		values = append(values, r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newRunHeaderHTTPClient(map[string]string{
		"Authorization": "Bearer {openbrain_session}",
	})
	if client == nil {
		t.Fatal("newRunHeaderHTTPClient() = nil")
	}
	if _, err := client.Get(server.URL); err != nil {
		t.Fatal(err)
	}
	writeTestAuthJSON(t, baseDir, "token-two")
	if _, err := client.Get(server.URL); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(values) != 2 {
		t.Fatalf("header values = %v, want two requests", values)
	}
	if values[0] != "Bearer token-one" || values[1] != "Bearer token-two" {
		t.Fatalf("header values = %v, want per-request session tokens", values)
	}
}

func writeTestAuthJSON(t *testing.T, baseDir string, token string) {
	t.Helper()
	authDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"token":%q}`, token)
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
