package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	providerGitHub = "github"
	defaultBranch  = "main"
)

type workspaceIndexFile struct {
	Version    int                               `json:"version"`
	Accounts   map[string]*workspaceIndexAccount `json:"accounts,omitempty"`
	Workspaces []workspaceEntry                  `json:"workspaces,omitempty"`
	ActiveUID  string                            `json:"-"`
}

type workspaceIndexAccount struct {
	Workspaces []workspaceEntry `json:"workspaces"`
}

type workspaceEntry struct {
	WorkspaceID   string                   `json:"workspaceID"`
	OrgID         string                   `json:"orgID,omitempty"`
	LocalName     string                   `json:"localName"`
	Path          string                   `json:"path"`
	Repository    json.RawMessage          `json:"repository,omitempty"`
	Storage       *workspaceStorageBinding `json:"storage,omitempty"`
	SyncPolicy    workspaceSyncPolicy      `json:"syncPolicy,omitempty"`
	BackupEnabled bool                     `json:"backupEnabled"`
}

type workspaceStorageBinding struct {
	Enabled    bool                `json:"enabled"`
	Backend    string              `json:"backend"`
	Provider   string              `json:"provider,omitempty"`
	RemoteURL  string              `json:"remoteURL,omitempty"`
	SyncPolicy workspaceSyncPolicy `json:"syncPolicy"`
}

type workspaceSyncPolicy struct {
	AutoSync    bool `json:"autoSync"`
	IntervalSec int  `json:"intervalSec"`
}

type repositoryBinding struct {
	RemoteURL     string `json:"remoteURL"`
	DefaultBranch string `json:"defaultBranch"`
}

type authConfig struct {
	Gateway string `json:"gateway"`
	Token   string `json:"token"`
	UID     string `json:"uid"`
}

type gitAccessToken struct {
	Username             string `json:"username"`
	AccessToken          string `json:"accessToken"`
	AccessTokenExpiresAt string `json:"accessTokenExpiresAt,omitempty"`
	RemoteURL            string `json:"remoteURL"`
}

type gitTarget struct {
	RemoteURL string `json:"remoteURL,omitempty"`
	Branch    string `json:"branch"`
}

type result struct {
	OK         bool              `json:"ok"`
	Status     string            `json:"status"`
	Code       string            `json:"code,omitempty"`
	Message    string            `json:"message,omitempty"`
	Recovery   string            `json:"recovery,omitempty"`
	Retryable  bool              `json:"retryable,omitempty"`
	Workspaces []workspaceResult `json:"workspaces,omitempty"`
	Workspace  *workspaceResult  `json:"workspace,omitempty"`
	Meta       map[string]string `json:"meta,omitempty"`
}

type workspaceResult struct {
	WorkspaceID   string   `json:"workspaceID"`
	WorkspaceName string   `json:"workspaceName,omitempty"`
	WorkspacePath string   `json:"workspacePath"`
	OrgID         string   `json:"orgID,omitempty"`
	Status        string   `json:"status"`
	Code          string   `json:"code,omitempty"`
	Message       string   `json:"message,omitempty"`
	Recovery      string   `json:"recovery,omitempty"`
	Retryable     bool     `json:"retryable,omitempty"`
	Branch        string   `json:"branch,omitempty"`
	Ahead         bool     `json:"ahead,omitempty"`
	Behind        bool     `json:"behind,omitempty"`
	Dirty         bool     `json:"dirty,omitempty"`
	Unmerged      bool     `json:"unmerged,omitempty"`
	NestedGit     []string `json:"nestedGit,omitempty"`
	Gitlinks      []string `json:"gitlinks,omitempty"`
}

func main() {
	if err := run(); err != nil {
		writeJSON(result{OK: false, Status: "error", Code: "internal_error", Message: sanitize(err.Error(), "")})
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: openbrain-cloud-sync-helper <preflight|sync>")
	}
	switch os.Args[1] {
	case "preflight":
		return runPreflight(os.Args[2:])
	case "sync":
		return runSync(os.Args[2:])
	default:
		return fmt.Errorf("unknown command: %s", os.Args[1])
	}
}

type stringListFlag []string

func (values *stringListFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *stringListFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	*values = append(*values, trimmed)
	return nil
}

func runPreflight(args []string) error {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	workspaceID := fs.String("workspace-id", "", "workspace id to inspect")
	includeDisabled := fs.Bool("include-disabled", false, "include workspaces whose auto sync policy is disabled")
	var allowNested stringListFlag
	fs.Var(&allowNested, "allow-nested", "nested repo path to allow when gitlink is absent from the parent index")
	if err := fs.Parse(args); err != nil {
		return err
	}
	entries, err := loadEligibleWorkspaces(workspaceFilter{IncludeDisabled: *includeDisabled})
	if err != nil {
		code := classifyError(err)
		writeJSON(result{OK: false, Status: "blocked", Code: code, Message: sanitize(err.Error(), ""), Recovery: recoveryForCode(code), Retryable: retryableCode(code)})
		return nil
	}
	results := make([]workspaceResult, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(*workspaceID) != "" && strings.TrimSpace(entry.WorkspaceID) != strings.TrimSpace(*workspaceID) {
			continue
		}
		results = append(results, preflightWorkspace(entry, allowNested))
	}
	sort.SliceStable(results, func(i, j int) bool {
		return strings.ToLower(results[i].WorkspaceName) < strings.ToLower(results[j].WorkspaceName)
	})
	writeJSON(result{OK: true, Status: "ok", Workspaces: results})
	return nil
}

func runSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	workspaceID := fs.String("workspace-id", "", "workspace id to sync")
	includeDisabled := fs.Bool("include-disabled", false, "allow manual sync for a workspace whose auto sync policy is disabled")
	var allowNested stringListFlag
	fs.Var(&allowNested, "allow-nested", "nested repo path to allow when gitlink is absent from the parent index")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*workspaceID) == "" {
		return errors.New("--workspace-id is required")
	}
	entry, err := findEligibleWorkspace(*workspaceID, workspaceFilter{IncludeDisabled: *includeDisabled})
	if err != nil {
		code := classifyError(err)
		writeJSON(result{OK: false, Status: "blocked", Code: code, Message: sanitize(err.Error(), ""), Recovery: recoveryForCode(code), Retryable: retryableCode(code)})
		return nil
	}
	workspace := syncWorkspace(entry, allowNested)
	if workspace.Status == "synced" || workspace.Status == "clean" {
		writeJSON(result{OK: true, Status: workspace.Status, Workspace: &workspace})
		return nil
	}
	writeJSON(result{OK: false, Status: "blocked", Code: workspace.Code, Message: workspace.Message, Recovery: workspace.Recovery, Retryable: workspace.Retryable, Workspace: &workspace})
	return nil
}

type workspaceFilter struct {
	IncludeDisabled bool
}

func loadEligibleWorkspaces(filter workspaceFilter) ([]workspaceEntry, error) {
	index, err := loadIndex()
	if err != nil {
		return nil, err
	}
	out := make([]workspaceEntry, 0, len(index.Workspaces))
	for _, entry := range index.Workspaces {
		if isEligibleWorkspace(entry, filter) {
			out = append(out, entry)
		}
	}
	return out, nil
}

func findEligibleWorkspace(workspaceID string, filter workspaceFilter) (workspaceEntry, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	entries, err := loadEligibleWorkspaces(filter)
	if err != nil {
		return workspaceEntry{}, err
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.WorkspaceID) == workspaceID {
			return entry, nil
		}
	}
	return workspaceEntry{}, fmt.Errorf("workspace_not_bound_for_account: workspace %s is not bound for the current OpenBrain account", workspaceID)
}

func isEligibleWorkspace(entry workspaceEntry, filter workspaceFilter) bool {
	if entry.Storage == nil || !entry.Storage.Enabled {
		return false
	}
	if strings.TrimSpace(entry.Storage.Backend) != "git" || strings.TrimSpace(entry.Storage.Provider) != providerGitHub {
		return false
	}
	if !filter.IncludeDisabled && !workspaceAutoSyncEnabled(entry) {
		return false
	}
	return strings.TrimSpace(entry.WorkspaceID) != "" && strings.TrimSpace(entry.Path) != ""
}

func workspaceAutoSyncEnabled(entry workspaceEntry) bool {
	policy := entry.SyncPolicy
	if isZeroPolicy(policy) && entry.Storage != nil {
		policy = entry.Storage.SyncPolicy
	}
	if isZeroPolicy(policy) {
		return entry.BackupEnabled
	}
	return policy.AutoSync
}

func isZeroPolicy(policy workspaceSyncPolicy) bool {
	return !policy.AutoSync && policy.IntervalSec == 0
}

func preflightWorkspace(entry workspaceEntry, allowNested []string) workspaceResult {
	out := baseWorkspaceResult(entry)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := ensureGitRepo(ctx, entry.Path); err != nil {
		out.Status = "blocked"
		out.Code = "not_git_repository"
		out.Message = err.Error()
		return out
	}
	nested, gitlinks := detectNestedGit(entry.Path, allowNested)
	out.NestedGit = nested
	out.Gitlinks = gitlinks
	if len(nested) > 0 || len(gitlinks) > 0 {
		out.Status = "blocked"
		out.Code = "nested_git"
		out.Message = "workspace contains nested git repositories or gitlinks"
		return out
	}
	auth, cleanup, target, err := gitAuth(ctx, entry)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		out.Status = "blocked"
		out.Code = classifyError(err)
		out.Message = sanitize(err.Error(), "")
		out.Recovery = recoveryForCode(out.Code)
		out.Retryable = retryableCode(out.Code)
		return out
	}
	if target.RemoteURL != "" {
		if _, err := runGit(ctx, entry.Path, auth, "fetch", "--quiet", target.RemoteURL, target.Branch); err != nil {
			return blocked(out, classifyGitError(err, "git_fetch_failed"), err)
		}
	}
	status, err := gitStatus(ctx, entry.Path)
	if err != nil {
		out.Status = "blocked"
		out.Code = "git_status_failed"
		out.Message = err.Error()
		return out
	}
	applyStatus(&out, status)
	applyFetchHeadStatus(ctx, entry.Path, &out)
	if out.Unmerged {
		out.Status = "blocked"
		out.Code = "git_conflict"
		out.Message = "workspace has unresolved git conflicts"
		return out
	}
	if out.Dirty || out.Ahead || out.Behind {
		out.Status = "needs_sync"
		return out
	}
	out.Status = "clean"
	return out
}

func syncWorkspace(entry workspaceEntry, allowNested []string) workspaceResult {
	out := preflightWorkspace(entry, allowNested)
	if out.Status == "blocked" || out.Status == "clean" {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	env, cleanup, target, err := gitAuth(ctx, entry)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		out.Status = "blocked"
		out.Code = classifyError(err)
		out.Message = sanitize(err.Error(), "")
		out.Recovery = recoveryForCode(out.Code)
		out.Retryable = retryableCode(out.Code)
		return out
	}
	if target.RemoteURL == "" {
		out.Status = "blocked"
		out.Code = "missing_remote"
		out.Message = "workspace git remote URL is missing"
		return out
	}
	status, err := gitStatus(ctx, entry.Path)
	if err != nil {
		out.Status = "blocked"
		out.Code = "git_status_failed"
		out.Message = err.Error()
		return out
	}
	applyStatus(&out, status)
	if out.Unmerged {
		out.Status = "blocked"
		out.Code = "git_conflict"
		out.Message = "workspace has unresolved git conflicts"
		return out
	}
	if out.Dirty {
		if _, err := runGit(ctx, entry.Path, nil, "add", "-A"); err != nil {
			return blocked(out, "git_add_failed", err)
		}
		if _, err := runGit(ctx, entry.Path, nil, "-c", "user.name=OpenBrain", "-c", "user.email=openbrain@users.noreply.github.com", "commit", "-m", "Sync workspace"); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "nothing to commit") {
				return blocked(out, "git_commit_failed", err)
			}
		}
	}
	if _, err := runGit(ctx, entry.Path, env, "pull", "--rebase", target.RemoteURL, target.Branch); err != nil {
		return blocked(out, classifyGitError(err, "git_rebase_failed"), err)
	}
	status, err = gitStatus(ctx, entry.Path)
	if err != nil {
		return blocked(out, "git_status_failed", err)
	}
	applyStatus(&out, status)
	if out.Unmerged {
		out.Status = "blocked"
		out.Code = "git_conflict"
		out.Message = "workspace has unresolved git conflicts after rebase"
		return out
	}
	if out.Dirty {
		if _, err := runGit(ctx, entry.Path, nil, "add", "-A"); err != nil {
			return blocked(out, "git_add_failed", err)
		}
		if _, err := runGit(ctx, entry.Path, nil, "-c", "user.name=OpenBrain", "-c", "user.email=openbrain@users.noreply.github.com", "commit", "-m", "Sync workspace"); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "nothing to commit") {
				return blocked(out, "git_commit_failed", err)
			}
		}
	}
	if _, err := runGit(ctx, entry.Path, env, "push", target.RemoteURL, "HEAD:"+target.Branch); err != nil {
		return blocked(out, classifyGitError(err, "git_push_failed"), err)
	}
	if err := triggerBrainSync(ctx, entry); err != nil {
		return blocked(out, classifyError(err), err)
	}
	out.Status = "synced"
	out.Code = ""
	out.Message = ""
	out.Dirty = false
	out.Ahead = false
	out.Behind = false
	return out
}

func blocked(out workspaceResult, code string, err error) workspaceResult {
	out.Status = "blocked"
	out.Code = code
	out.Message = sanitize(err.Error(), "")
	out.Recovery = recoveryForCode(code)
	out.Retryable = retryableCode(code)
	return out
}

func baseWorkspaceResult(entry workspaceEntry) workspaceResult {
	return workspaceResult{
		WorkspaceID:   strings.TrimSpace(entry.WorkspaceID),
		WorkspaceName: workspaceName(entry),
		WorkspacePath: strings.TrimSpace(entry.Path),
		OrgID:         strings.TrimSpace(entry.OrgID),
		Branch:        gitTargetFor(entry, nil).Branch,
	}
}

func workspaceName(entry workspaceEntry) string {
	if name := strings.TrimSpace(entry.LocalName); name != "" {
		return name
	}
	return filepath.Base(strings.TrimSpace(entry.Path))
}

func ensureGitRepo(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return errors.New("workspace is not a git repository")
	}
	return nil
}

type statusInfo struct {
	Lines []string
}

func gitStatus(ctx context.Context, dir string) (statusInfo, error) {
	out, err := runGit(ctx, dir, nil, "status", "--porcelain=v1", "--branch")
	if err != nil {
		return statusInfo{}, err
	}
	lines := []string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return statusInfo{Lines: lines}, nil
}

func applyStatus(out *workspaceResult, status statusInfo) {
	out.Dirty = false
	out.Ahead = false
	out.Behind = false
	out.Unmerged = false
	for _, line := range status.Lines {
		if strings.HasPrefix(line, "## ") {
			out.Ahead = strings.Contains(line, "ahead ")
			out.Behind = strings.Contains(line, "behind ")
			continue
		}
		out.Dirty = true
		code := line
		if len(code) > 2 {
			code = code[:2]
		}
		if strings.Contains(code, "U") || code == "AA" || code == "DD" {
			out.Unmerged = true
		}
	}
}

func applyFetchHeadStatus(ctx context.Context, dir string, out *workspaceResult) {
	if out == nil {
		return
	}
	if countGitRevisions(ctx, dir, "HEAD..FETCH_HEAD") > 0 {
		out.Behind = true
	}
	if countGitRevisions(ctx, dir, "FETCH_HEAD..HEAD") > 0 {
		out.Ahead = true
	}
}

func countGitRevisions(ctx context.Context, dir string, revisionRange string) int {
	out, err := runGit(ctx, dir, nil, "rev-list", "--count", revisionRange)
	if err != nil {
		return 0
	}
	var count int
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d", &count); err != nil {
		return 0
	}
	return count
}

func detectNestedGit(root string, allowNested []string) ([]string, []string) {
	allowed := map[string]struct{}{}
	for _, path := range allowNested {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		normalized = strings.Trim(normalized, "/")
		if normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}
	nested := []string{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if name == ".git" {
			if path != filepath.Join(root, ".git") {
				if rel, relErr := filepath.Rel(root, filepath.Dir(path)); relErr == nil && rel != "." {
					relSlash := filepath.ToSlash(rel)
					if _, ok := allowed[relSlash]; !ok {
						nested = append(nested, relSlash)
					}
				}
			}
			if d.IsDir() {
				return filepath.SkipDir
			}
		}
		if d.IsDir() && (name == "node_modules" || name == ".cache") {
			return filepath.SkipDir
		}
		return nil
	})
	gitlinks := []string{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := runGit(ctx, root, nil, "ls-files", "--stage"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[0] == "160000" {
				gitlinks = append(gitlinks, fields[len(fields)-1])
			}
		}
	}
	sort.Strings(nested)
	sort.Strings(gitlinks)
	return nested, gitlinks
}

func gitAuth(ctx context.Context, entry workspaceEntry) ([]string, func(), gitTarget, error) {
	token, err := workspaceGitToken(ctx, entry)
	if err != nil {
		target := gitTargetFor(entry, nil)
		return nil, nil, target, err
	}
	target := gitTargetFor(entry, token)
	env, cleanup, err := gitAskpassEnv(token.Username, token.AccessToken)
	return env, cleanup, target, err
}

func workspaceGitToken(ctx context.Context, entry workspaceEntry) (*gitAccessToken, error) {
	auth, err := loadAuth()
	if err != nil {
		return nil, err
	}
	gateway := strings.TrimRight(strings.TrimSpace(auth.Gateway), "/")
	if gateway == "" || strings.TrimSpace(auth.Token) == "" {
		return nil, errors.New("login_required: sign in before syncing git storage")
	}
	if strings.TrimSpace(entry.OrgID) == "" || strings.TrimSpace(entry.WorkspaceID) == "" {
		return nil, errors.New("workspace identity is missing")
	}
	endpoint := gateway + "/v1/orgs/" + url.PathEscape(entry.OrgID) + "/workspaces/" + url.PathEscape(entry.WorkspaceID) + "/git-token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(auth.Token))
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(auth.UID) != "" {
		req.Header.Set("X-UID", strings.TrimSpace(auth.UID))
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 && res.StatusCode < 500 && strings.Contains(strings.ToLower(string(raw)), "permission required") {
		return nil, fmt.Errorf("cloud_permission_denied: permission required for workspace %s", strings.TrimSpace(entry.WorkspaceID))
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("login_required: git token request failed: %s", res.Status)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("git token request failed: %s: %s", res.Status, strings.TrimSpace(string(raw)))
	}
	var parsed gitAccessToken
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return nil, errors.New("git token response did not include an access token")
	}
	if strings.TrimSpace(parsed.Username) == "" {
		parsed.Username = "x-access-token"
	}
	return &parsed, nil
}

func gitTargetFor(entry workspaceEntry, token *gitAccessToken) gitTarget {
	remoteURL := ""
	if token != nil {
		remoteURL = strings.TrimSpace(token.RemoteURL)
	}
	if remoteURL == "" && entry.Storage != nil {
		remoteURL = strings.TrimSpace(entry.Storage.RemoteURL)
	}
	repo := repositoryBinding{}
	if len(entry.Repository) > 0 {
		_ = json.Unmarshal(entry.Repository, &repo)
		if remoteURL == "" {
			remoteURL = strings.TrimSpace(repo.RemoteURL)
		}
	}
	branch := strings.TrimSpace(repo.DefaultBranch)
	if branch == "" {
		branch = defaultBranch
	}
	return gitTarget{RemoteURL: stripRemoteUserInfo(remoteURL), Branch: branch}
}

func stripRemoteUserInfo(remoteURL string) string {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return ""
	}
	parsed, err := url.Parse(remoteURL)
	if err != nil || parsed == nil || parsed.User == nil {
		return remoteURL
	}
	parsed.User = nil
	return parsed.String()
}

func gitAskpassEnv(username string, token string) ([]string, func(), error) {
	// Disable git credential helpers (e.g. macOS osxkeychain) for OpenBrain-managed
	// git commands. System helpers run before GIT_ASKPASS and may return a token
	// scoped to a different repository, which GitHub reports as "Repository not
	// found" for private repos. An empty value clears the helper list (git 2.31+).
	gitCredentialOverride := []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return append([]string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}, gitCredentialOverride...), nil, nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "x-access-token"
	}
	dir, err := os.MkdirTemp("", "openbrain-git-askpass-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	script := filepath.Join(dir, "askpass.sh")
	content := "#!/bin/sh\ncase \"$1\" in\n  *Username*) printf '%s\\n' \"$OPENBRAIN_GIT_ASKPASS_USERNAME\" ;;\n  *) printf '%s\\n' \"$OPENBRAIN_GIT_ASKPASS_TOKEN\" ;;\nesac\n"
	if runtime.GOOS == "windows" {
		script = filepath.Join(dir, "askpass.cmd")
		content = "@echo off\r\necho %~1 | findstr /I \"Username\" >nul\r\nif %errorlevel%==0 (\r\n  echo %OPENBRAIN_GIT_ASKPASS_USERNAME%\r\n) else (\r\n  echo %OPENBRAIN_GIT_ASKPASS_TOKEN%\r\n)\r\n"
	}
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		cleanup()
		return nil, nil, err
	}
	env := append([]string{
		"GIT_ASKPASS=" + script,
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"OPENBRAIN_GIT_ASKPASS_USERNAME=" + username,
		"OPENBRAIN_GIT_ASKPASS_TOKEN=" + token,
	}, gitCredentialOverride...)
	return env, cleanup, nil
}

func triggerBrainSync(ctx context.Context, entry workspaceEntry) error {
	auth, err := loadAuth()
	if err != nil {
		return err
	}
	gateway := strings.TrimRight(strings.TrimSpace(auth.Gateway), "/")
	if gateway == "" || strings.TrimSpace(auth.Token) == "" {
		return errors.New("login_required: sign in before syncing OpenBrain Cloud brain")
	}
	endpoint := gateway + "/v1/orgs/" + url.PathEscape(entry.OrgID) + "/workspaces/" + url.PathEscape(entry.WorkspaceID) + "/brain/sync"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(auth.Token))
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(auth.UID) != "" {
		req.Header.Set("X-UID", strings.TrimSpace(auth.UID))
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return fmt.Errorf("login_required: brain sync request failed: %s", res.Status)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("brain sync request failed: %s: %s", res.Status, strings.TrimSpace(string(raw)))
	}
	return nil
}

func loadIndex() (workspaceIndexFile, error) {
	var index workspaceIndexFile
	auth, err := loadAuth()
	if err != nil {
		return workspaceIndexFile{Version: 2, Accounts: map[string]*workspaceIndexAccount{}}, nil
	}
	raw, err := os.ReadFile(filepath.Join(openBrainHome(), "index", "workspaces.json"))
	if err != nil {
		if os.IsNotExist(err) {
			index = workspaceIndexFile{Version: 2, Accounts: map[string]*workspaceIndexAccount{}}
			index.activate(auth.UID)
			return index, nil
		}
		return index, err
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return index, err
	}
	if index.Version != 2 || index.Accounts == nil {
		index = workspaceIndexFile{Version: 2, Accounts: map[string]*workspaceIndexAccount{}}
		index.activate(auth.UID)
		return index, nil
	}
	index.activate(auth.UID)
	return index, nil
}

func loadAuth() (authConfig, error) {
	var auth authConfig
	raw, err := os.ReadFile(filepath.Join(openBrainHome(), "configs", "user", "auth.json"))
	if err != nil {
		return auth, errors.New("login_required: sign in before syncing OpenBrain Cloud")
	}
	if err := json.Unmarshal(raw, &auth); err != nil {
		return auth, err
	}
	if strings.TrimSpace(auth.UID) == "" || strings.TrimSpace(auth.Token) == "" {
		return auth, errors.New("login_required: sign in before syncing OpenBrain Cloud")
	}
	return auth, nil
}

func (index *workspaceIndexFile) activate(uid string) {
	uid = strings.TrimSpace(uid)
	index.ActiveUID = uid
	if index.Accounts == nil {
		index.Accounts = map[string]*workspaceIndexAccount{}
	}
	if uid == "" {
		index.Workspaces = nil
		return
	}
	account := index.Accounts[uid]
	if account == nil {
		index.Workspaces = nil
		return
	}
	index.Workspaces = append([]workspaceEntry(nil), account.Workspaces...)
}

func openBrainHome() string {
	if value := strings.TrimSpace(os.Getenv("OPENBRAIN_HOME")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("OP_HOME")); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".openbrain"
	}
	return filepath.Join(home, ".openbrain")
}

func runGit(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	text := strings.TrimSpace(out.String())
	if err != nil {
		if text != "" {
			return text, errors.New(text)
		}
		return text, err
	}
	return text, nil
}

func classifyError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "login_required") {
		return "login_required"
	}
	if strings.Contains(text, "workspace_not_bound_for_account") {
		return "workspace_not_bound_for_account"
	}
	if strings.Contains(text, "cloud_permission_denied") {
		return "cloud_permission_denied"
	}
	if isGitAuthErrorText(text) {
		return "git_token_expired"
	}
	return "error"
}

func classifyGitError(err error, fallback string) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	if isGitPermissionOrRepoErrorText(text) {
		return "git_permission_denied"
	}
	if isGitAuthErrorText(text) {
		return "git_token_expired"
	}
	if strings.Contains(text, "conflict") || strings.Contains(text, "unmerged") {
		return "git_conflict"
	}
	return fallback
}

func isGitAuthErrorText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	patterns := []string{
		"authentication failed",
		"could not read username",
		"could not read password",
		"invalid username or password",
		"support for password authentication was removed",
		"http basic: access denied",
		"403",
		"401",
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func isGitPermissionOrRepoErrorText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	patterns := []string{
		"repository not found",
		"permission denied",
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func retryableCode(code string) bool {
	switch strings.TrimSpace(code) {
	case "git_token_expired":
		return true
	default:
		return false
	}
}

func recoveryForCode(code string) string {
	switch strings.TrimSpace(code) {
	case "login_required":
		return "OpenBrain login is expired or missing. Ask the user to sign in again from the desktop OpenBrain login dialog, then rerun this helper command."
	case "git_token_expired":
		return "The short-lived workspace git token may have expired. Rerun this helper command once; it will request a fresh token before retrying git."
	case "workspace_not_bound_for_account":
		return "This local workspace binding does not belong to the current OpenBrain account. Switch accounts, ask for workspace sharing, remove the local binding, or bind this folder again for the current account."
	case "cloud_permission_denied":
		return "The current OpenBrain account does not have Cloud access to this workspace. Switch accounts, ask for workspace sharing, remove the local binding, or skip this workspace."
	case "git_permission_denied":
		return "The workspace repository is missing or the GitHub App no longer has access. Ask the user to reconnect GitHub or confirm repository access before retrying."
	default:
		return ""
	}
}

func sanitize(text string, secret string) string {
	text = strings.TrimSpace(text)
	if secret != "" {
		text = strings.ReplaceAll(text, secret, "[redacted]")
	}
	return text
}

func writeJSON(value any) {
	raw, _ := json.MarshalIndent(value, "", "  ")
	fmt.Println(string(raw))
}
