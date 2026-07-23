package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLoadEligibleWorkspacesSkipsDisabledByDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENBRAIN_HOME", filepath.Join(home, ".openbrain"))
	writeTestAuth(t, "http://127.0.0.1.invalid")
	writeTestIndex(t, []workspaceEntry{
		{
			WorkspaceID:   "enabled",
			OrgID:         "org",
			LocalName:     "Enabled",
			Path:          filepath.Join(home, "enabled"),
			BackupEnabled: true,
			Storage:       &workspaceStorageBinding{Enabled: true, Backend: "git", Provider: providerGitHub},
			SyncPolicy:    workspaceSyncPolicy{AutoSync: true, IntervalSec: 300},
		},
		{
			WorkspaceID:   "disabled",
			OrgID:         "org",
			LocalName:     "Disabled",
			Path:          filepath.Join(home, "disabled"),
			BackupEnabled: true,
			Storage:       &workspaceStorageBinding{Enabled: true, Backend: "git", Provider: providerGitHub, SyncPolicy: workspaceSyncPolicy{AutoSync: false, IntervalSec: 300}},
			SyncPolicy:    workspaceSyncPolicy{AutoSync: false, IntervalSec: 300},
		},
	})

	entries, err := loadEligibleWorkspaces(workspaceFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].WorkspaceID != "enabled" {
		t.Fatalf("entries = %+v, want only enabled workspace", entries)
	}
	entries, err = loadEligibleWorkspaces(workspaceFilter{IncludeDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries with include disabled = %+v, want 2", entries)
	}
}

func TestLoadEligibleWorkspacesOnlyCurrentAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENBRAIN_HOME", filepath.Join(home, ".openbrain"))
	writeTestAuth(t, "http://127.0.0.1.invalid")
	path := filepath.Join(openBrainHome(), "index", "workspaces.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(workspaceIndexFile{
		Version: 3,
		Deployments: map[string]*workspaceIndexDeployment{
			"dep-test": {
				Organizations: map[string]*workspaceIndexOrganization{
					"org": {
						Accounts: map[string]*workspaceIndexAccount{
							"other-user": {Workspaces: []workspaceEntry{{
								WorkspaceID: "other", OrgID: "org", LocalName: "Other", Path: "/tmp/other",
								Storage:    &workspaceStorageBinding{Enabled: true, Backend: "git", Provider: "github", SyncPolicy: workspaceSyncPolicy{AutoSync: true, IntervalSec: 300}},
								SyncPolicy: workspaceSyncPolicy{AutoSync: true, IntervalSec: 300},
							}}},
							"user": {Workspaces: []workspaceEntry{{
								WorkspaceID: "current", OrgID: "org", LocalName: "Current", Path: "/tmp/current",
								Storage:    &workspaceStorageBinding{Enabled: true, Backend: "git", Provider: "github", SyncPolicy: workspaceSyncPolicy{AutoSync: true, IntervalSec: 300}},
								SyncPolicy: workspaceSyncPolicy{AutoSync: true, IntervalSec: 300},
							}}},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := loadEligibleWorkspaces(workspaceFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].WorkspaceID != "current" {
		t.Fatalf("entries = %+v, want current account only", entries)
	}
}

func TestFindEligibleWorkspaceReportsAccountBindingMiss(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENBRAIN_HOME", filepath.Join(home, ".openbrain"))
	writeTestAuth(t, "http://127.0.0.1.invalid")
	writeTestIndex(t, []workspaceEntry{{
		WorkspaceID: "current",
		OrgID:       "org",
		LocalName:   "Current",
		Path:        "/tmp/current",
		Storage:     &workspaceStorageBinding{Enabled: true, Backend: "git", Provider: providerGitHub, SyncPolicy: workspaceSyncPolicy{AutoSync: true, IntervalSec: 300}},
		SyncPolicy:  workspaceSyncPolicy{AutoSync: true, IntervalSec: 300},
	}})

	_, err := findEligibleWorkspace("other", workspaceFilter{})
	if got := classifyError(err); got != "workspace_not_bound_for_account" {
		t.Fatalf("code = %q, err = %v", got, err)
	}
	if retryableCode("workspace_not_bound_for_account") {
		t.Fatal("workspace_not_bound_for_account should not be retryable")
	}
}

func TestPreflightUsesFetchHeadToDetectRemoteChanges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENBRAIN_HOME", filepath.Join(home, ".openbrain"))
	remote := filepath.Join(home, "remote.git")
	localA := filepath.Join(home, "a")
	localB := filepath.Join(home, "b")

	runTestGit(t, home, "init", "--bare", remote)
	runTestGit(t, home, "clone", remote, localA)
	runTestGit(t, localA, "-c", "user.name=OpenBrain", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	runTestGit(t, localA, "push", "origin", "HEAD:main")
	runTestGit(t, home, "clone", remote, localB)
	runTestGit(t, localB, "switch", "main")
	runTestGit(t, localA, "-c", "user.name=OpenBrain", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "remote-change")
	runTestGit(t, localA, "push", "origin", "HEAD:main")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/orgs/org/workspaces/ws/git-token" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"username":    "x-access-token",
			"accessToken": "token",
			"remoteURL":   remote,
		})
	}))
	t.Cleanup(server.Close)
	writeTestAuth(t, server.URL)
	writeTestIndex(t, []workspaceEntry{{
		WorkspaceID:   "ws",
		OrgID:         "org",
		LocalName:     "Workspace",
		Path:          localB,
		Repository:    json.RawMessage(`{"defaultBranch":"main"}`),
		BackupEnabled: true,
		Storage:       &workspaceStorageBinding{Enabled: true, Backend: "git", Provider: providerGitHub, SyncPolicy: workspaceSyncPolicy{AutoSync: true, IntervalSec: 300}},
		SyncPolicy:    workspaceSyncPolicy{AutoSync: true, IntervalSec: 300},
	}})

	result := preflightWorkspace(loadSingleWorkspace(t, "ws"), nil)
	if result.Status != "needs_sync" || !result.Behind {
		t.Fatalf("preflight = %+v, want needs_sync behind", result)
	}
}

func TestDetectNestedGitReportsNestedRepo(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "cblog")
	if err := os.MkdirAll(filepath.Join(nested, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "init")

	foundNested, gitlinks := detectNestedGit(root, nil)
	if len(foundNested) != 1 || foundNested[0] != "cblog" {
		t.Fatalf("nested = %#v, want cblog", foundNested)
	}
	if len(gitlinks) != 0 {
		t.Fatalf("gitlinks = %#v, want empty", gitlinks)
	}
}

func TestDetectNestedGitAllowNestedSkipsOnDiskRepoButNotGitlink(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "cblog")
	if err := os.MkdirAll(filepath.Join(nested, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "init")
	runTestGit(t, root, "update-index", "--add", "--cacheinfo", "160000", "0123456789abcdef0123456789abcdef01234567", "cblog")

	foundNested, gitlinks := detectNestedGit(root, []string{"cblog"})
	if len(foundNested) != 0 {
		t.Fatalf("nested = %#v, want empty when cblog is allowed", foundNested)
	}
	if len(gitlinks) != 1 || gitlinks[0] != "cblog" {
		t.Fatalf("gitlinks = %#v, want cblog gitlink to remain blocking", gitlinks)
	}
}

func TestClassifyGitErrorDistinguishesPermissionFromExpiredToken(t *testing.T) {
	if got := classifyGitError(os.ErrPermission, "git_push_failed"); got != "git_permission_denied" {
		t.Fatalf("permission error code = %q, want git_permission_denied", got)
	}
	if got := classifyGitError(assertError("remote: Repository not found."), "git_fetch_failed"); got != "git_permission_denied" {
		t.Fatalf("repository missing error code = %q, want git_permission_denied", got)
	}
	if got := retryableCode("git_permission_denied"); got {
		t.Fatal("git_permission_denied should not be retryable")
	}
	if got := classifyGitError(assertError("fatal: Authentication failed for 'https://github.com/example/repo.git'"), "git_push_failed"); got != "git_token_expired" {
		t.Fatalf("auth error code = %q, want git_token_expired", got)
	}
}

func TestWorkspaceGitTokenClassifiesCloudPermissionDenied(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "generic permission required", status: http.StatusForbidden, body: `{"error":"permission required"}`},
		{name: "resource read permission required", status: http.StatusBadRequest, body: `{"error":"resource read permission required"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("OPENBRAIN_HOME", filepath.Join(home, ".openbrain"))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/orgs/org/workspaces/ws/git-token" {
					http.NotFound(w, r)
					return
				}
				http.Error(w, tc.body, tc.status)
			}))
			t.Cleanup(server.Close)
			writeTestAuth(t, server.URL)

			_, err := workspaceGitToken(context.Background(), workspaceEntry{WorkspaceID: "ws", OrgID: "org"})
			if got := classifyError(err); got != "cloud_permission_denied" {
				t.Fatalf("code = %q, err = %v", got, err)
			}
			if retryableCode("cloud_permission_denied") {
				t.Fatal("cloud_permission_denied should not be retryable")
			}
		})
	}
}

type assertError string

func (e assertError) Error() string {
	return string(e)
}

func writeTestIndex(t *testing.T, entries []workspaceEntry) {
	t.Helper()
	path := filepath.Join(openBrainHome(), "index", "workspaces.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(workspaceIndexFile{
		Version: 3,
		Deployments: map[string]*workspaceIndexDeployment{
			"dep-test": {
				Organizations: map[string]*workspaceIndexOrganization{
					"org": {
						Accounts: map[string]*workspaceIndexAccount{
							"user": {Workspaces: entries},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestAuth(t *testing.T, gateway string) {
	t.Helper()
	path := filepath.Join(openBrainHome(), "configs", "user", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(authConfig{
		Version: 2, Gateway: gateway, Token: "session-token", UID: "user",
		DeploymentID: "dep-test", OrgID: "org", IdentityID: "idn-test",
		ConnectionID: "conn-test", AuthMethod: "email",
		AuthTime: "2026-07-23T00:00:00Z", ExpiresAt: "2026-07-24T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func loadSingleWorkspace(t *testing.T, workspaceID string) workspaceEntry {
	t.Helper()
	entry, err := findEligibleWorkspace(workspaceID, workspaceFilter{})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
