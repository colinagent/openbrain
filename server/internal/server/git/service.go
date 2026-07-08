package gitservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

type runner func(ctx context.Context, dir string, args ...string) (string, error)

type Service struct {
	run runner
}

func NewService() *Service {
	return &Service{run: runGit}
}

func normalizePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory: %w", err)
		}
		cleaned = filepath.Join(cwd, cleaned)
	}
	return cleaned, nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return text, fmt.Errorf("git not found")
		}
		if text != "" {
			return text, fmt.Errorf("%w: %s", err, text)
		}
		return text, err
	}
	return text, nil
}

func (s *Service) Branches(params *protocol.GitBranchesParams) (*protocol.GitBranchesResult, *protocol.RPCError) {
	path, err := normalizePath(params.Path)
	if err != nil {
		return nil, rpcInvalidParams(err)
	}
	repoRoot, repoErr := s.repoRoot(path)
	if repoErr != nil {
		if isNotRepoError(repoErr) {
			return &protocol.GitBranchesResult{IsRepo: false}, nil
		}
		return nil, rpcInternal(repoErr)
	}
	result := &protocol.GitBranchesResult{IsRepo: true, RepoRoot: repoRoot}
	branch, detached, detachedLabel, err := s.currentBranch(repoRoot)
	if err != nil {
		return nil, rpcInternal(err)
	}
	result.CurrentBranch = branch
	result.Detached = detached
	result.DetachedLabel = detachedLabel
	branches, err := s.localBranches(repoRoot, branch)
	if err != nil {
		return nil, rpcInternal(err)
	}
	result.Branches = branches
	dirty, err := s.dirtySummary(repoRoot)
	if err != nil {
		return nil, rpcInternal(err)
	}
	result.Dirty = dirty
	return result, nil
}

func (s *Service) Checkout(params *protocol.GitCheckoutParams) (*protocol.GitCheckoutResult, *protocol.RPCError) {
	path, err := normalizePath(params.Path)
	if err != nil {
		return nil, rpcInvalidParams(err)
	}
	branch := strings.TrimSpace(params.Branch)
	if branch == "" {
		return nil, rpcInvalidParams(fmt.Errorf("branch is required"))
	}
	repoRoot, repoErr := s.repoRoot(path)
	if repoErr != nil {
		if isNotRepoError(repoErr) {
			return nil, rpcInvalidParams(fmt.Errorf("path is not inside a git repository"))
		}
		return nil, rpcInternal(repoErr)
	}
	dirty, err := s.dirtySummary(repoRoot)
	if err != nil {
		return nil, rpcInternal(err)
	}
	if dirty.HasChanges {
		return nil, rpcInvalidParams(fmt.Errorf("working tree has uncommitted changes"))
	}
	if params.Create {
		if err := s.validateNewBranch(repoRoot, branch); err != nil {
			return nil, err
		}
		if _, err := s.runWithTimeout(repoRoot, "checkout", "-b", branch); err != nil {
			return nil, rpcInternal(err)
		}
	} else {
		exists, err := s.branchExists(repoRoot, branch)
		if err != nil {
			return nil, rpcInternal(err)
		}
		if !exists {
			return nil, rpcInvalidParams(fmt.Errorf("local branch not found: %s", branch))
		}
		if _, err := s.runWithTimeout(repoRoot, "checkout", branch); err != nil {
			return nil, rpcInternal(err)
		}
	}
	currentBranch, _, _, err := s.currentBranch(repoRoot)
	if err != nil {
		return nil, rpcInternal(err)
	}
	return &protocol.GitCheckoutResult{RepoRoot: repoRoot, CurrentBranch: currentBranch}, nil
}

func (s *Service) validateNewBranch(repoRoot string, branch string) *protocol.RPCError {
	if _, err := s.runWithTimeout(repoRoot, "check-ref-format", "--branch", branch); err != nil {
		return rpcInvalidParams(fmt.Errorf("invalid branch name: %s", branch))
	}
	exists, err := s.branchExists(repoRoot, branch)
	if err != nil {
		return rpcInternal(err)
	}
	if exists {
		return &protocol.RPCError{Code: protocol.ErrCodeFileExists, Message: fmt.Sprintf("branch already exists: %s", branch)}
	}
	return nil
}

func (s *Service) repoRoot(path string) (string, error) {
	return s.runWithTimeout(path, "rev-parse", "--show-toplevel")
}

func (s *Service) currentBranch(repoRoot string) (branch string, detached bool, detachedLabel string, err error) {
	branch, err = s.runWithTimeout(repoRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		return branch, false, "", nil
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not a symbolic ref") {
		return "", false, "", err
	}
	sha, shaErr := s.runWithTimeout(repoRoot, "rev-parse", "--short", "HEAD")
	if shaErr != nil {
		return "", false, "", shaErr
	}
	return sha, true, sha, nil
}

func (s *Service) localBranches(repoRoot string, currentBranch string) ([]protocol.GitBranch, error) {
	out, err := s.runWithTimeout(repoRoot, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	lines := splitNonEmptyLines(out)
	sort.Strings(lines)
	branches := make([]protocol.GitBranch, 0, len(lines))
	for _, line := range lines {
		if line == currentBranch {
			continue
		}
		branches = append(branches, protocol.GitBranch{Name: line, Current: false})
	}
	if currentBranch != "" {
		current := protocol.GitBranch{Name: currentBranch, Current: true}
		branches = append([]protocol.GitBranch{current}, branches...)
	}
	return branches, nil
}

func (s *Service) branchExists(repoRoot string, branch string) (bool, error) {
	branches, err := s.localBranches(repoRoot, "")
	if err != nil {
		return false, err
	}
	for _, item := range branches {
		if item.Name == branch {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) dirtySummary(repoRoot string) (*protocol.GitDirtySummary, error) {
	statusOut, err := s.runWithTimeout(repoRoot, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	changedFiles := len(splitNonEmptyLines(statusOut))
	diffOut, err := s.runWithTimeout(repoRoot, "diff", "--shortstat", "HEAD", "--")
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "unknown revision") && !strings.Contains(strings.ToLower(err.Error()), "bad revision") {
		return nil, err
	}
	added, deleted := parseShortStat(diffOut)
	return &protocol.GitDirtySummary{
		ChangedFiles: changedFiles,
		AddedLines:   added,
		DeletedLines: deleted,
		HasChanges:   changedFiles > 0,
	}, nil
}

func (s *Service) runWithTimeout(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.run(ctx, dir, args...)
}

var shortStatPattern = regexp.MustCompile(`(\d+)\s+(insertion|insertion\(\+\)|insertions|insertions\(\+\)|deletion|deletion\(-\)|deletions|deletions\(-\))`)

func parseShortStat(value string) (int, int) {
	added := 0
	deleted := 0
	for _, match := range shortStatPattern.FindAllStringSubmatch(strings.TrimSpace(value), -1) {
		count, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		label := match[2]
		if strings.Contains(label, "insert") {
			added = count
			continue
		}
		if strings.Contains(label, "delet") {
			deleted = count
		}
	}
	return added, deleted
}

func splitNonEmptyLines(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, "\n")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		line := strings.TrimSpace(part)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func isNotRepoError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not a git repository") || strings.Contains(message, "needed a single revision")
}

func rpcInvalidParams(err error) *protocol.RPCError {
	return &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: err.Error()}
}

func rpcInternal(err error) *protocol.RPCError {
	return &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: err.Error()}
}
