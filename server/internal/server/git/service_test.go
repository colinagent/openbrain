package gitservice

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

type stubResult struct {
	out string
	err error
}

func newStubService(results map[string]stubResult) *Service {
	return &Service{
		run: func(_ context.Context, _ string, args ...string) (string, error) {
			key := fmt.Sprint(args)
			result, ok := results[key]
			if !ok {
				return "", fmt.Errorf("unexpected git args: %s", key)
			}
			return result.out, result.err
		},
	}
}

func TestBranchesReturnsNonRepo(t *testing.T) {
	svc := newStubService(map[string]stubResult{
		"[rev-parse --show-toplevel]": {err: errors.New("fatal: not a git repository")},
	})
	result, rpcErr := svc.Branches(&protocol.GitBranchesParams{Path: "/tmp/work"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if result.IsRepo {
		t.Fatalf("expected non repo result")
	}
}

func TestBranchesReturnsCurrentFirst(t *testing.T) {
	svc := newStubService(map[string]stubResult{
		"[rev-parse --show-toplevel]":                         {out: "/tmp/repo"},
		"[symbolic-ref --quiet --short HEAD]":                 {out: "main"},
		"[for-each-ref --format=%(refname:short) refs/heads]": {out: "rune\nmain\ncolin\n"},
		"[status --porcelain]":                                {out: ""},
		"[diff --shortstat HEAD --]":                          {out: ""},
	})
	result, rpcErr := svc.Branches(&protocol.GitBranchesParams{Path: "/tmp/repo"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if len(result.Branches) != 3 {
		t.Fatalf("expected 3 branches, got %d", len(result.Branches))
	}
	if result.Branches[0].Name != "main" || !result.Branches[0].Current {
		t.Fatalf("expected current branch first, got %+v", result.Branches[0])
	}
	if result.Branches[1].Name != "colin" || result.Branches[2].Name != "rune" {
		t.Fatalf("expected remaining branches sorted, got %+v", result.Branches)
	}
}

func TestBranchesHandlesDetachedHead(t *testing.T) {
	svc := newStubService(map[string]stubResult{
		"[rev-parse --show-toplevel]":                         {out: "/tmp/repo"},
		"[symbolic-ref --quiet --short HEAD]":                 {err: errors.New("fatal: ref HEAD is not a symbolic ref")},
		"[rev-parse --short HEAD]":                            {out: "abc1234"},
		"[for-each-ref --format=%(refname:short) refs/heads]": {out: "main\nfeature\n"},
		"[status --porcelain]":                                {out: ""},
		"[diff --shortstat HEAD --]":                          {out: ""},
	})
	result, rpcErr := svc.Branches(&protocol.GitBranchesParams{Path: "/tmp/repo"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if !result.Detached || result.DetachedLabel != "abc1234" || result.CurrentBranch != "abc1234" {
		t.Fatalf("expected detached head result, got %+v", result)
	}
}

func TestCheckoutRejectsDirtyRepo(t *testing.T) {
	svc := newStubService(map[string]stubResult{
		"[rev-parse --show-toplevel]": {out: "/tmp/repo"},
		"[status --porcelain]":        {out: " M a.txt\n"},
		"[diff --shortstat HEAD --]":  {out: " 1 file changed, 3 insertions(+), 1 deletion(-)"},
	})
	_, rpcErr := svc.Checkout(&protocol.GitCheckoutParams{Path: "/tmp/repo", Branch: "main"})
	if rpcErr == nil || rpcErr.Code != protocol.ErrCodeInvalidParams {
		t.Fatalf("expected invalid params for dirty repo, got %+v", rpcErr)
	}
}

func TestCheckoutRejectsInvalidOrDuplicateNewBranch(t *testing.T) {
	svc := newStubService(map[string]stubResult{
		"[rev-parse --show-toplevel]":          {out: "/tmp/repo"},
		"[status --porcelain]":                 {out: ""},
		"[diff --shortstat HEAD --]":           {out: ""},
		"[check-ref-format --branch bad name]": {err: errors.New("fatal: invalid branch name")},
	})
	_, rpcErr := svc.Checkout(&protocol.GitCheckoutParams{Path: "/tmp/repo", Branch: "bad name", Create: true})
	if rpcErr == nil || rpcErr.Code != protocol.ErrCodeInvalidParams {
		t.Fatalf("expected invalid params for invalid branch name, got %+v", rpcErr)
	}

	duplicateSvc := newStubService(map[string]stubResult{
		"[rev-parse --show-toplevel]":                         {out: "/tmp/repo"},
		"[status --porcelain]":                                {out: ""},
		"[diff --shortstat HEAD --]":                          {out: ""},
		"[check-ref-format --branch main]":                    {out: "main"},
		"[for-each-ref --format=%(refname:short) refs/heads]": {out: "main\nfeature\n"},
	})
	_, rpcErr = duplicateSvc.Checkout(&protocol.GitCheckoutParams{Path: "/tmp/repo", Branch: "main", Create: true})
	if rpcErr == nil || rpcErr.Code != protocol.ErrCodeFileExists {
		t.Fatalf("expected duplicate branch error, got %+v", rpcErr)
	}
}
