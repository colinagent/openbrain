package fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/server/internal/rgsearch"
	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

func TestSearchPrefersOpagentManagedRgBinary(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)

	preferred := filepath.Join(home, ".openbrain", "bin", "rg")
	writeSearchScript(t, preferred, "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"match\",\"data\":{\"path\":{\"text\":\"demo.txt\"},\"lines\":{\"text\":\"preferred\\n\"},\"line_number\":1,\"submatches\":[{\"start\":0,\"end\":9}]}}'\n")

	fallbackDir := t.TempDir()
	writeSearchScript(t, filepath.Join(fallbackDir, "rg"), "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"match\",\"data\":{\"path\":{\"text\":\"demo.txt\"},\"lines\":{\"text\":\"fallback\\n\"},\"line_number\":1,\"submatches\":[{\"start\":0,\"end\":8}]}}'\n")
	t.Setenv("PATH", fallbackDir)

	svc := NewFileService(false)
	result, rpcErr := svc.Search(context.Background(), &protocol.SearchParams{
		Root:  root,
		Query: "demo",
	})
	if rpcErr != nil {
		t.Fatalf("Search() rpcErr = %+v", rpcErr)
	}
	if result.TotalCount != 1 {
		t.Fatalf("TotalCount = %d, want 1", result.TotalCount)
	}
	if got := result.Files[0].Matches[0].Text; got != "preferred" {
		t.Fatalf("match text = %q, want preferred", got)
	}
}

func TestSearchReturnsMissingBinaryErrorWhenRgUnavailable(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")

	svc := NewFileService(false)
	_, rpcErr := svc.Search(context.Background(), &protocol.SearchParams{
		Root:  root,
		Query: "demo",
	})
	if rpcErr == nil {
		t.Fatal("Search() rpcErr = nil, want missing binary error")
	}
	if rpcErr.Message == "" || !strings.Contains(rpcErr.Message, rgsearch.BinaryNotFoundMessage) {
		t.Fatalf("rpcErr.Message = %q, want missing binary text", rpcErr.Message)
	}
}

func writeSearchScript(t *testing.T, scriptPath string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
}
