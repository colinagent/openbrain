package opagent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	protocol "github.com/colinagent/opagent/opagent-protocol/go-sdk/op"
)

func TestProductRuntimeOptionsAuthorizeOnlyManagedServer(t *testing.T) {
	baseDir := t.TempDir()
	serviceRoot := filepath.Join(baseDir, "agents", "opagent-server")
	manifest := filepath.Join(serviceRoot, ".agent", "AGENT.md")
	executable := filepath.Join(serviceRoot, ".agent", "bin", "openbrain-server")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("---\nid: agent-openbrain-server\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("server"), 0o700); err != nil {
		t.Fatal(err)
	}
	options, err := ProductRuntimeOptions(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	node := &protocol.OpNode{
		ID: managedServerAgentID, Kind: string(protocol.NodeKindAgent), URI: protocol.PathToURI(manifest), Cwd: serviceRoot,
		OpCodes: []protocol.OpCode{protocol.SystemStarted},
		Run:     protocol.Run{Daemon: true, Command: []string{executable, "--host", "127.0.0.1", "--port", "19530"}},
	}
	if err := options.SystemServiceAuthorizer(context.Background(), node); err != nil {
		t.Fatalf("managed server rejected: %v", err)
	}
	for name, mutate := range map[string]func(*protocol.OpNode){
		"wrong id":   func(candidate *protocol.OpNode) { candidate.ID = "agent-forged" },
		"wrong host": func(candidate *protocol.OpNode) { candidate.Run.Command[2] = "0.0.0.0" },
		"wrong port": func(candidate *protocol.OpNode) { candidate.Run.Command[4] = "0" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := *node
			candidate.Run.Command = append([]string(nil), node.Run.Command...)
			mutate(&candidate)
			if err := options.SystemServiceAuthorizer(context.Background(), &candidate); err == nil {
				t.Fatal("forged server was authorized")
			}
		})
	}
}

func TestProductRuntimeOptionsResolveSessionFromProductDirectory(t *testing.T) {
	baseDir := t.TempDir()
	authDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte(`{"version":2,"token":"session-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	options, err := ProductRuntimeOptions(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := options.HeaderResolver(context.Background(), map[string]string{"Authorization": "Bearer " + SessionPlaceholder})
	if err != nil {
		t.Fatal(err)
	}
	if got := headers["Authorization"]; got != "Bearer session-secret" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestProductRuntimeOptionsRejectSymlinkSession(t *testing.T) {
	baseDir := t.TempDir()
	authDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(baseDir, "auth-target.json")
	if err := os.WriteFile(target, []byte(`{"version":2,"token":"session-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(authDir, "auth.json")); err != nil {
		t.Fatal(err)
	}
	options, err := ProductRuntimeOptions(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := options.HeaderResolver(context.Background(), map[string]string{"Authorization": "Bearer " + SessionPlaceholder}); err == nil {
		t.Fatal("symlinked session config was accepted")
	}
}
