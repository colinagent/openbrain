package opagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHeartbeatUsesProductBaseDirAndSession(t *testing.T) {
	baseDir := t.TempDir()
	var received heartbeatEnvelope
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/heartbeat/heartbeats" || request.Header.Get("Authorization") != "Bearer session-secret" {
			t.Fatalf("unexpected heartbeat request: %s %q", request.URL.Path, request.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := os.MkdirAll(filepath.Join(baseDir, "configs", "user"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "configs", "system"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "configs", "user", "auth.json"), []byte(`{"version":2,"gateway":"`+server.URL+`","token":"session-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "configs", "system", "host_id"), []byte("host-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewHeartbeatService(baseDir)
	service.client = server.Client()
	service.now = func() time.Time { return time.Unix(123, 0) }
	if err := service.reportOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if received.Instance.ID != "host-test" || received.Instance.BaseDir != baseDir || !received.SentAt.Equal(time.Unix(123, 0)) {
		t.Fatalf("heartbeat = %+v", received)
	}
}

func TestHeartbeatRejectsSymlinkAuthConfig(t *testing.T) {
	baseDir := t.TempDir()
	authPath := filepath.Join(baseDir, "configs", "user", "auth.json")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(baseDir, "auth-target.json")
	if err := os.WriteFile(target, []byte(`{"version":2,"gateway":"https://api.example.com","token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, authPath); err != nil {
		t.Fatal(err)
	}
	if err := NewHeartbeatService(baseDir).reportOnce(context.Background()); err == nil {
		t.Fatal("symlinked heartbeat auth config was accepted")
	}
}
