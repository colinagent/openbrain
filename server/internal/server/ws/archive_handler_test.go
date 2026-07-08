package ws

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

type stubCleanupRunner struct {
	params protocol.ArchiveCleanupParams
	result *protocol.ArchiveCleanupResult
	err    error
	called bool
}

func (s *stubCleanupRunner) Run(_ context.Context, params protocol.ArchiveCleanupParams) (*protocol.ArchiveCleanupResult, error) {
	s.called = true
	s.params = params
	return s.result, s.err
}

func TestHandleArchiveCleanupRun(t *testing.T) {
	handler := NewHandler(NewServer(":0", false), false)
	stub := &stubCleanupRunner{
		result: &protocol.ArchiveCleanupResult{MovedChats: 2},
	}
	handler.archive = stub

	raw, err := json.Marshal(protocol.ArchiveCleanupParams{
		WorkspaceRoots: []string{"/tmp/workspace"},
		OpenFilePaths:  []string{"/tmp/workspace/.agent/chat/a.md"},
	})
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}

	result, rpcErr := handler.handleArchiveCleanupRun(raw)
	if rpcErr != nil {
		t.Fatalf("handleArchiveCleanupRun(): %+v", rpcErr)
	}
	if !stub.called {
		t.Fatal("expected cleanup runner to be called")
	}
	if len(stub.params.WorkspaceRoots) != 1 || stub.params.WorkspaceRoots[0] != "/tmp/workspace" {
		t.Fatalf("unexpected workspace roots: %+v", stub.params.WorkspaceRoots)
	}
	cleanupResult, ok := result.(*protocol.ArchiveCleanupResult)
	if !ok {
		t.Fatalf("result type = %T, want *protocol.ArchiveCleanupResult", result)
	}
	if cleanupResult.MovedChats != 2 {
		t.Fatalf("MovedChats = %d, want 2", cleanupResult.MovedChats)
	}
}
