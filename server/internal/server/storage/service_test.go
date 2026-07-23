package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

func TestStatusReturnsIdleForRemoteStorage(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".openbrain", "workspaces", "demo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{homeDir: home}
	writeTestIndex(t, svc, workspaceEntry{
		WorkspaceID:   "ws-test",
		LocalName:     "demo",
		Path:          root,
		BackupEnabled: true,
		Storage: &protocol.WorkspaceStorageBinding{
			Enabled:    true,
			Backend:    "git",
			Provider:   providerGitHub,
			RemoteURL:  "https://github.com/example/demo.git",
			SyncPolicy: defaultPolicy(true),
		},
	})

	status, err := svc.Status(protocol.StorageStatusParams{WorkspaceID: "ws-test"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "idle" {
		t.Fatalf("status = %q, want idle", status.Status)
	}
	if status.LastSyncAt != "" || status.LastError != "" {
		t.Fatalf("status should not read legacy sync state: %+v", status)
	}
}

func TestNormalizePolicyPreservesUserInterval(t *testing.T) {
	policy := normalizePolicy(protocol.WorkspaceSyncPolicy{
		AutoSync:      true,
		OnLocalChange: true,
		IntervalSec:   60,
	}, true)
	if policy.IntervalSec != 60 {
		t.Fatalf("IntervalSec = %d, want 60", policy.IntervalSec)
	}
}

func TestNormalizePolicyDefaultsMissingIntervalToFiveMinutes(t *testing.T) {
	policy := normalizePolicy(protocol.WorkspaceSyncPolicy{
		AutoSync: true,
	}, true)
	if policy.IntervalSec != 300 {
		t.Fatalf("IntervalSec = %d, want 300", policy.IntervalSec)
	}
}

func TestUpdatePolicyPersistsUserInterval(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{homeDir: home}
	writeTestIndex(t, svc, workspaceEntry{
		WorkspaceID: "ws-policy",
		LocalName:   "workspace",
		Path:        root,
		Storage: &protocol.WorkspaceStorageBinding{
			Enabled:    true,
			Backend:    "git",
			Provider:   providerGitHub,
			SyncPolicy: defaultPolicy(true),
		},
	})

	status, err := svc.UpdatePolicy(protocol.StorageUpdatePolicyParams{
		WorkspaceID: "ws-policy",
		Policy: protocol.WorkspaceSyncPolicy{
			AutoSync:      true,
			OnLocalChange: true,
			IntervalSec:   60,
			Conflict:      "keep-both",
			DeleteMode:    "trash",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Policy.IntervalSec != 60 {
		t.Fatalf("status IntervalSec = %d, want 60", status.Policy.IntervalSec)
	}
	index, err := svc.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	entry := index.Workspaces[findWorkspaceIndex(index.Workspaces, "ws-policy", "")]
	if entry.SyncPolicy.IntervalSec != 60 {
		t.Fatalf("entry SyncPolicy IntervalSec = %d, want 60", entry.SyncPolicy.IntervalSec)
	}
	if entry.Storage == nil || entry.Storage.SyncPolicy.IntervalSec != 60 {
		t.Fatalf("storage SyncPolicy = %+v, want interval 60", entry.Storage)
	}
}

func TestStatusNormalizesLegacyOpenBrainCloudWorkspace(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".openbrain", "workspaces", "demo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{homeDir: home}
	writeTestIndex(t, svc, workspaceEntry{
		WorkspaceID:   "ws-cloud",
		OrgID:         "cloud",
		LocalName:     "demo",
		Path:          root,
		TemplateID:    "openbrain-cloud",
		BackupEnabled: true,
		Storage:       &protocol.WorkspaceStorageBinding{Enabled: false, Backend: ""},
	})

	status, err := svc.Status(protocol.StorageStatusParams{WorkspaceID: "ws-cloud"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Storage == nil || !status.Storage.Enabled {
		t.Fatalf("storage = %#v, want enabled OpenBrain Cloud binding", status.Storage)
	}
	if status.Storage.Backend != backendOpenBrain || status.Storage.Provider != providerOpenBrain || status.Storage.RemoteID != "ws-cloud" {
		t.Fatalf("storage = %#v, want OpenBrain Cloud binding", status.Storage)
	}
}

func TestCronBindingsReturnsAllGitHubWorkspaces(t *testing.T) {
	home := t.TempDir()
	first := filepath.Join(home, "alpha")
	second := filepath.Join(home, "beta")
	if err := os.MkdirAll(first, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{homeDir: home}
	writeTestIndexFile(t, svc, []workspaceEntry{
		{
			WorkspaceID: "ws-beta",
			OrgID:       "org",
			LocalName:   "Beta",
			Path:        second,
			Repository:  json.RawMessage(`{"remoteURL":"https://github.com/example/beta.git","defaultBranch":"trunk"}`),
			Storage:     &protocol.WorkspaceStorageBinding{Enabled: true, Backend: "git", Provider: providerGitHub, SyncPolicy: defaultPolicy(true)},
		},
		{
			WorkspaceID: "ws-alpha",
			OrgID:       "org",
			LocalName:   "Alpha",
			Path:        first,
			Storage:     &protocol.WorkspaceStorageBinding{Enabled: true, Backend: "git", Provider: providerGitHub, RemoteURL: "https://github.com/example/alpha.git", SyncPolicy: protocol.WorkspaceSyncPolicy{AutoSync: true, IntervalSec: 900}},
		},
		{
			WorkspaceID: "ws-drive",
			LocalName:   "Drive",
			Path:        filepath.Join(home, "drive"),
			Storage:     &protocol.WorkspaceStorageBinding{Enabled: true, Backend: "drive", Provider: "google-drive"},
		},
	})

	bindings, err := svc.CronBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 {
		t.Fatalf("len(bindings) = %d, want 2: %+v", len(bindings), bindings)
	}
	if bindings[0].WorkspaceID != "ws-alpha" || bindings[1].WorkspaceID != "ws-beta" {
		t.Fatalf("bindings sorted by name = %+v", bindings)
	}
	if bindings[1].RepoURL != "https://github.com/example/beta.git" || bindings[1].Branch != "trunk" {
		t.Fatalf("repository binding not used: %+v", bindings[1])
	}
}

func TestCronBindingsRequiresCurrentAuth(t *testing.T) {
	home := t.TempDir()
	svc := &Service{homeDir: home}
	indexPath := svc.indexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	rawIndex := `{"version":2,"accounts":{"user-bob":{"workspaces":[{"workspaceID":"ws-alpha","localName":"Alpha","path":"/tmp/alpha","storage":{"enabled":true,"backend":"git","provider":"github","remoteURL":"https://github.com/example/alpha.git","syncPolicy":{"autoSync":true,"intervalSec":300}}}]}}}`
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	bindings, err := svc.CronBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 0 {
		t.Fatalf("cron should not read index without auth: %+v", bindings)
	}
}

func TestCronBindingsOnlyReadsCurrentAccount(t *testing.T) {
	home := t.TempDir()
	svc := &Service{homeDir: home}
	writeTestAuth(t, home, "user-bob")
	indexPath := svc.indexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	rawIndex := `{
		"version": 3,
		"deployments": {
		  "dep-test": {
		    "organizations": {
		      "org-test": {
		        "accounts": {
			      "user-alice": {
				"workspaces": [{
					"workspaceID": "ws-alice",
					"localName": "Alice",
					"path": "/tmp/alice",
					"storage": {"enabled": true, "backend": "git", "provider": "github", "remoteURL": "https://github.com/example/alice.git", "syncPolicy": {"autoSync": true, "intervalSec": 300}}
				}]
			},
			      "user-bob": {
				"workspaces": [{
					"workspaceID": "ws-bob",
					"localName": "Bob",
					"path": "/tmp/bob",
					"storage": {"enabled": true, "backend": "git", "provider": "github", "remoteURL": "https://github.com/example/bob.git", "syncPolicy": {"autoSync": true, "intervalSec": 300}}
				}]
			      }
		        }
		      }
		    }
		  }
		}
	}`
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	bindings, err := svc.CronBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].WorkspaceID != "ws-bob" {
		t.Fatalf("cron should only read current account bindings: %+v", bindings)
	}
}

func TestCronBindingsScopesSameUIDByDeploymentAndOrganization(t *testing.T) {
	home := t.TempDir()
	svc := &Service{homeDir: home}
	indexPath := svc.indexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	rawIndex := `{
		"version": 3,
		"deployments": {
			"dep-a": {
				"organizations": {
					"org-a": {"accounts": {"user-same": {"workspaces": [{
						"workspaceID": "ws-dep-a-org-a",
						"orgID": "org-a",
						"localName": "A",
						"path": "/tmp/dep-a-org-a",
						"storage": {"enabled": true, "backend": "git", "provider": "github", "remoteURL": "https://github.com/example/a.git", "syncPolicy": {"autoSync": true, "intervalSec": 300}}
					}]}}},
					"org-b": {"accounts": {"user-same": {"workspaces": [{
						"workspaceID": "ws-dep-a-org-b",
						"orgID": "org-b",
						"localName": "B",
						"path": "/tmp/dep-a-org-b",
						"storage": {"enabled": true, "backend": "git", "provider": "github", "remoteURL": "https://github.com/example/b.git", "syncPolicy": {"autoSync": true, "intervalSec": 300}}
					}]}}}
				}
			},
			"dep-b": {
				"organizations": {
					"org-a": {"accounts": {"user-same": {"workspaces": [{
						"workspaceID": "ws-dep-b-org-a",
						"orgID": "org-a",
						"localName": "C",
						"path": "/tmp/dep-b-org-a",
						"storage": {"enabled": true, "backend": "git", "provider": "github", "remoteURL": "https://github.com/example/c.git", "syncPolicy": {"autoSync": true, "intervalSec": 300}}
					}]}}}
				}
			}
		}
	}`
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		deploymentID string
		orgID        string
		want         string
	}{
		{deploymentID: "dep-a", orgID: "org-a", want: "ws-dep-a-org-a"},
		{deploymentID: "dep-a", orgID: "org-b", want: "ws-dep-a-org-b"},
		{deploymentID: "dep-b", orgID: "org-a", want: "ws-dep-b-org-a"},
	} {
		writeTestAuthContext(t, home, "user-same", test.deploymentID, test.orgID)
		bindings, err := svc.CronBindings()
		if err != nil {
			t.Fatal(err)
		}
		if len(bindings) != 1 || bindings[0].WorkspaceID != test.want {
			t.Fatalf("%s/%s should only read %s, got %+v", test.deploymentID, test.orgID, test.want, bindings)
		}
	}
}

func writeTestIndex(t *testing.T, svc *Service, entry workspaceEntry) {
	t.Helper()
	writeTestIndexFile(t, svc, []workspaceEntry{entry})
}

func TestWriteJSONAtomicUsesPrivateFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX file modes")
	}
	target := filepath.Join(t.TempDir(), "workspaces.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONAtomic(target, map[string]any{"version": 3}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("workspace index mode = %o, want 600", got)
	}
}

func writeTestIndexFile(t *testing.T, svc *Service, entries []workspaceEntry) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range entries {
		entries[i].CreatedAt = now
		entries[i].UpdatedAt = now
	}
	writeTestAuth(t, svc.homeDir, "user-bob")
	if err := svc.saveIndex(&workspaceIndexFile{Version: 3, Workspaces: entries}); err != nil {
		t.Fatal(err)
	}
}

func writeTestAuth(t *testing.T, home string, uid string) {
	t.Helper()
	writeTestAuthContext(t, home, uid, "dep-test", "org-test")
}

func writeTestAuthContext(t *testing.T, home string, uid string, deploymentID string, orgID string) {
	t.Helper()
	authDir := filepath.Join(home, ".openbrain", "configs", "user")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"version":2,"gateway":"http://127.0.0.1.invalid","token":"session-token","uid":"` + uid + `","deploymentID":"` + deploymentID + `","orgID":"` + orgID + `","identityID":"idn-test","connectionID":"conn-test","authMethod":"email","authTime":"2026-07-23T00:00:00Z","expiresAt":"2026-07-24T00:00:00Z","updatedAt":1}`
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}
