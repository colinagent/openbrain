package opagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	protocol "github.com/colinagent/opagent/opagent-protocol/go-sdk/op"
)

func TestRuntimeOptionsUseProductDirectoryAndResolveSession(t *testing.T) {
	baseDir := t.TempDir()
	options, err := RuntimeOptions(Dependencies{
		BaseDir: baseDir,
		SessionToken: func(context.Context) (string, error) {
			return "session-test", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.BaseDir != filepath.Clean(baseDir) {
		t.Fatalf("baseDir = %q", options.BaseDir)
	}
	headers, err := options.HeaderResolver(context.Background(), map[string]string{"Authorization": "Bearer " + SessionPlaceholder})
	if err != nil {
		t.Fatal(err)
	}
	if headers["Authorization"] != "Bearer session-test" {
		t.Fatalf("headers = %#v", headers)
	}
	if err := options.SystemServiceAuthorizer(context.Background(), &protocol.OpNode{}); err == nil {
		t.Fatal("missing product system authorizer did not fail closed")
	}
}

func TestGBrainCloudAuthorizationRequiresManagedIdentityPathAndShape(t *testing.T) {
	baseDir := t.TempDir()
	manifest := filepath.Join(baseDir, "tools", "gbrain-cloud", "TOOL.md")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("---\nid: tools-gbrain-cloud\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options, err := RuntimeOptions(Dependencies{BaseDir: baseDir})
	if err != nil {
		t.Fatal(err)
	}
	valid := &protocol.OpNode{
		ID:   gbrainCloudNodeID,
		Kind: string(protocol.NodeKindTools),
		URI:  protocol.PathToURI(manifest),
		Run: protocol.Run{
			URL: gbrainCloudURL, Daemon: true,
			Header: map[string]string{"Authorization": "Bearer " + SessionPlaceholder},
		},
	}
	if err := options.RemoteNodeAuthorizer(context.Background(), valid); err != nil {
		t.Fatalf("managed node rejected: %v", err)
	}

	cases := map[string]func(*protocol.OpNode){
		"id": func(node *protocol.OpNode) { node.ID = "tools-forged" },
		"path": func(node *protocol.OpNode) {
			node.URI = protocol.PathToURI(filepath.Join(baseDir, "agents", "forged", "TOOL.md"))
		},
		"url":     func(node *protocol.OpNode) { node.Run.URL = "https://example.invalid/mcp" },
		"header":  func(node *protocol.OpNode) { node.Run.Header["Authorization"] = "Bearer user-value" },
		"command": func(node *protocol.OpNode) { node.Run.Command = []string{"helper"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			clone := *valid
			clone.Run = valid.Run
			clone.Run.Header = map[string]string{"Authorization": valid.Run.Header["Authorization"]}
			mutate(&clone)
			if err := options.RemoteNodeAuthorizer(context.Background(), &clone); err == nil {
				t.Fatal("forged remote node was authorized")
			}
		})
	}
}

func TestGBrainCloudAuthorizationRejectsSymlinkManifest(t *testing.T) {
	baseDir := t.TempDir()
	manifest := filepath.Join(baseDir, "tools", "gbrain-cloud", "TOOL.md")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(baseDir, "forged.md")
	if err := os.WriteFile(target, []byte("forged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, manifest); err != nil {
		t.Fatal(err)
	}
	options, err := RuntimeOptions(Dependencies{BaseDir: baseDir})
	if err != nil {
		t.Fatal(err)
	}
	node := &protocol.OpNode{
		ID: gbrainCloudNodeID, Kind: string(protocol.NodeKindTools), URI: protocol.PathToURI(manifest),
		Run: protocol.Run{URL: gbrainCloudURL, Daemon: true, Header: map[string]string{"Authorization": "Bearer " + SessionPlaceholder}},
	}
	if err := options.RemoteNodeAuthorizer(context.Background(), node); err == nil {
		t.Fatal("symlinked GBrain manifest was authorized")
	}
}

func TestSessionResolutionFailsClosed(t *testing.T) {
	resolver := sessionHeaderResolver(func(context.Context) (string, error) {
		return "", errors.New("signed out")
	})
	if _, err := resolver(context.Background(), map[string]string{"Authorization": "Bearer " + SessionPlaceholder}); err == nil {
		t.Fatal("session resolution error was ignored")
	}
}
