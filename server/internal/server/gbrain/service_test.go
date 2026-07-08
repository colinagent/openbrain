package gbrain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestListSourcesMapsGBrainSources(t *testing.T) {
	service := NewServiceWithRunner(t.TempDir(), func(ctx context.Context, args []string) (CommandResult, error) {
		if got, want := strings.Join(args, " "), "sources list --json"; got != want {
			t.Fatalf("unexpected args: got %q want %q", got, want)
		}
		return CommandResult{Stdout: `{"sources":[{"id":"notes","name":"Notes","local_path":"/tmp/notes","federated":true,"page_count":12,"last_sync_at":"2026-06-14T01:02:03Z"}]}`}, nil
	})

	res := service.ListSources(context.Background())
	if !res.Success {
		t.Fatalf("expected success: %+v", res)
	}
	if len(res.Sources) != 1 {
		t.Fatalf("expected one source: %+v", res.Sources)
	}
	source := res.Sources[0]
	if source.SourceID != "notes" || source.WorkspaceID != "notes" || source.BrainID != "personal" || !source.Openable {
		t.Fatalf("unexpected mapped source: %+v", source)
	}
	if source.PageCount == nil || *source.PageCount != 12 {
		t.Fatalf("unexpected page count: %+v", source.PageCount)
	}
}

func TestQueryCallsGBrainStructuredOperation(t *testing.T) {
	calls := [][]string{}
	service := NewServiceWithRunner(t.TempDir(), func(ctx context.Context, args []string) (CommandResult, error) {
		calls = append(calls, append([]string(nil), args...))
		switch strings.Join(args[:min(len(args), 2)], " ") {
		case "call query":
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(args[2]), &payload); err != nil {
				t.Fatalf("query payload is not JSON: %v", err)
			}
			if payload["source_id"] != "notes" || payload["query"] != "packaging" {
				t.Fatalf("unexpected payload: %+v", payload)
			}
			return CommandResult{Stdout: `[
				{"slug":"decisions/release","title":"Release decisions","chunk_text":"Use GitHub Releases by default.","chunk_id":7,"chunk_index":0,"score":0.91,"source_id":"notes"}
			]`}, nil
		case "sources list":
			return CommandResult{Stdout: `{"sources":[{"id":"notes","name":"Notes","local_path":"/tmp/notes","federated":true,"page_count":12,"last_sync_at":null}]}`}, nil
		default:
			t.Fatalf("unexpected args: %v", args)
		}
		return CommandResult{}, nil
	})

	res := service.Query(context.Background(), QueryRequest{
		BrainID:     "personal",
		Scope:       "workspace",
		WorkspaceID: "notes",
		Query:       "packaging",
		Limit:       100,
	})
	if !res.Success {
		t.Fatalf("expected success: %+v", res)
	}
	if len(res.Results) != 1 {
		t.Fatalf("expected one result: %+v", res.Results)
	}
	got := res.Results[0]
	if got.WorkspaceID != "notes" || got.WorkspaceName != "Notes" || got.RelativePath != "decisions/release.md" || got.Score != 0.91 {
		t.Fatalf("unexpected mapped result: %+v", got)
	}
	if !strings.HasSuffix(got.Path, "decisions/release.md") {
		t.Fatalf("expected path to include slug markdown path: %q", got.Path)
	}
	if len(calls) != 2 {
		t.Fatalf("expected query and source list calls, got %+v", calls)
	}
}

func TestQueryUsesAllSourcesForBrainScope(t *testing.T) {
	service := NewServiceWithRunner(t.TempDir(), func(ctx context.Context, args []string) (CommandResult, error) {
		if len(args) >= 3 && args[0] == "call" && args[1] == "query" {
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(args[2]), &payload); err != nil {
				t.Fatal(err)
			}
			if payload["source_id"] != "__all__" {
				t.Fatalf("expected __all__ source, got %+v", payload)
			}
			return CommandResult{Stdout: `[]`}, nil
		}
		if reflect.DeepEqual(args, []string{"sources", "list", "--json"}) {
			return CommandResult{Stdout: `{"sources":[]}`}, nil
		}
		t.Fatalf("unexpected args: %v", args)
		return CommandResult{}, nil
	})
	res := service.Query(context.Background(), QueryRequest{BrainID: "personal", Scope: "brain", Query: "anything"})
	if !res.Success || len(res.Results) != 0 {
		t.Fatalf("unexpected response: %+v", res)
	}
}

func TestQueryUnavailableReturnsEmptyResults(t *testing.T) {
	service := NewServiceWithRunner(t.TempDir(), func(ctx context.Context, args []string) (CommandResult, error) {
		return CommandResult{}, ErrGBrainUnavailable
	})
	res := service.Query(context.Background(), QueryRequest{BrainID: "personal", Scope: "brain", Query: "anything"})
	if res.Success || res.Code != "gbrain_unavailable" || len(res.Results) != 0 {
		t.Fatalf("unexpected unavailable response: %+v", res)
	}
}

func TestQueryRejectsMissingWorkspaceID(t *testing.T) {
	service := NewServiceWithRunner(t.TempDir(), nil)
	res := service.Query(context.Background(), QueryRequest{Scope: "workspace", Query: "anything"})
	if res.Success || res.Code != "invalid_request" {
		t.Fatalf("unexpected response: %+v", res)
	}
}

func TestParseSearchResultsAcceptsToolResultEnvelope(t *testing.T) {
	stdout := `{"content":[{"type":"text","text":"[{\"slug\":\"a\",\"chunk_text\":\"hello\",\"source_id\":\"notes\"}]"}]}`
	results, err := parseSearchResults(stdout)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Slug != "a" || results[0].ChunkText != "hello" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestStatusAcceptsToolResultEnvelope(t *testing.T) {
	service := NewServiceWithRunner(t.TempDir(), func(ctx context.Context, args []string) (CommandResult, error) {
		if got, want := strings.Join(args, " "), "call get_stats {}"; got != want {
			t.Fatalf("unexpected args: got %q want %q", got, want)
		}
		return CommandResult{Stdout: `{"content":[{"type":"text","text":"{\"page_count\":12,\"chunk_count\":34}"}]}`}, nil
	})
	res := service.Status(context.Background())
	if !res.Success {
		t.Fatalf("expected success: %+v", res)
	}
	if res.Status["page_count"] != float64(12) || res.Status["chunk_count"] != float64(34) {
		t.Fatalf("unexpected status: %+v", res.Status)
	}
}

func TestHandlerInvalidRequestUsesBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(NewServiceWithRunner(t.TempDir(), nil))
	router := gin.New()
	router.POST("/v1/openbrain/query", handler.Query)

	req := httptest.NewRequest(http.MethodPost, "/v1/openbrain/query", bytes.NewReader([]byte(`{"scope":"workspace","query":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCloudQueryPassesPublicOwnerUIDAndUsesResultWorkspaceName(t *testing.T) {
	var sawPublicOwner bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/me/brain/workspaces":
			_, _ = w.Write([]byte(`{"workspaces":[]}`))
		case "/v1/me/brain/search":
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["publicOwnerUID"] == "user-alice" {
				sawPublicOwner = true
			}
			_, _ = w.Write([]byte(`{"results":[{"sourceID":"ws-alpha","workspaceID":"ws-alpha","workspaceName":"Alice Research","slug":"notes/a","title":"A","chunkID":1,"chunkText":"hello","score":0.7}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()
	service := newCloudTestService(t, upstream.URL)

	res := service.QueryOpenBrain(context.Background(), QueryRequest{Query: "hello", PublicOwnerUID: "user-alice"})
	if !res.Success || len(res.Results) != 1 {
		t.Fatalf("response = %+v", res)
	}
	if !sawPublicOwner {
		t.Fatalf("publicOwnerUID was not forwarded")
	}
	if res.Results[0].WorkspaceName != "Alice Research" {
		t.Fatalf("WorkspaceName = %q, want API result name", res.Results[0].WorkspaceName)
	}
}

func TestProxyCloudAPIForwardsAuthenticatedRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/brain/public-brains" || r.URL.Query().Get("query") != "Alex" {
			t.Fatalf("unexpected URL %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer session-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"brains":[{"ownerUID":"user-alice","name":"Alice's Brain","username":"alice","activeSourceCount":1,"subscribed":false}]}`))
	}))
	defer upstream.Close()
	service := newCloudTestService(t, upstream.URL)

	status, raw := service.ProxyCloudAPI(context.Background(), http.MethodGet, "/v1/me/brain/public-brains?query=Alex", nil)
	if status != http.StatusOK || !strings.Contains(string(raw), "Alice") {
		t.Fatalf("status/body = %d/%s", status, string(raw))
	}
}

func TestCloudListPublicBrainsForwardsIncludeSelf(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/brain/public-brains" || r.URL.Query().Get("query") != "Open" || r.URL.Query().Get("includeSelf") != "true" {
			t.Fatalf("unexpected URL %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer session-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"brains":[{"ownerUID":"user-bob","name":"OpenBrain","username":"openbrain","activeSourceCount":1,"subscribed":false,"owned":true,"sources":[{"sourceID":"ws-alpha"}]}]}`))
	}))
	defer upstream.Close()

	handler := NewHandler(newCloudTestService(t, upstream.URL))
	router := gin.New()
	router.GET("/v1/openbrain/cloud/public-brains", handler.CloudListPublicBrains)

	req := httptest.NewRequest(http.MethodGet, "/v1/openbrain/cloud/public-brains?query=Open&includeSelf=true", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"owned":true`) || !strings.Contains(rr.Body.String(), `"sourceID":"ws-alpha"`) {
		t.Fatalf("status/body = %d/%s", rr.Code, rr.Body.String())
	}
}

func TestCloudResolvePublicBrainSourcesForwardsOwnerUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/brain/public-brains/user-alice/sources" {
			t.Fatalf("unexpected URL %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer session-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"sources":[{"sourceID":"ws-alpha","workspaceID":"ws-alpha","orgID":"cloud","name":"Alpha"}]}`))
	}))
	defer upstream.Close()

	handler := NewHandler(newCloudTestService(t, upstream.URL))
	router := gin.New()
	router.GET("/v1/openbrain/cloud/public-brains/:ownerUID/sources", handler.CloudResolvePublicBrainSources)

	req := httptest.NewRequest(http.MethodGet, "/v1/openbrain/cloud/public-brains/user-alice/sources", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"sourceID":"ws-alpha"`) {
		t.Fatalf("status/body = %d/%s", rr.Code, rr.Body.String())
	}
}

func TestListCachedOpenBrainSourcesRequiresAuthBeforeReadingWorkspaceIndex(t *testing.T) {
	homeDir := t.TempDir()
	baseDir := filepath.Join(homeDir, ".openbrain")
	indexDir := filepath.Join(baseDir, "index")
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rawIndex := `{
		"version": 1,
		"workspaces": [{
			"workspaceID": "ws-alpha",
			"orgID": "org-alpha",
			"localName": "Alpha Notes",
			"path": "/tmp/alpha",
			"locationKind": "local",
			"templateID": "openbrain-cloud",
			"templateVersion": 1,
			"backupEnabled": true,
			"storage": {"remoteURL": "https://github.com/acme/alpha.git"},
			"createdAt": "2026-06-01T00:00:00Z",
			"updatedAt": "2026-06-01T00:00:00Z"
		}]
	}`
	if err := os.WriteFile(filepath.Join(indexDir, "workspaces.json"), []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(baseDir)

	res := service.ListCachedOpenBrainSources(context.Background())
	if !res.Success || res.Provider != "cloud" || res.Code != "auth_required" || !res.AuthRequired || len(res.Sources) != 0 {
		t.Fatalf("unexpected response: %+v", res)
	}
}

func TestListOpenBrainSourcesWritesCloudSourcesSnapshot(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/brain/workspaces" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer session-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"workspaces":[{
			"id":"ws-alpha",
			"orgID":"cloud",
			"name":"Alpha Cloud",
			"repoURL":"https://github.com/acme/alpha.git",
			"storageRemoteURL":"https://github.com/acme/alpha",
			"disabledQueries":true,
			"publicAccess":true,
			"updatedAt":"2026-06-20T01:02:03Z"
		}]}`))
	}))
	defer upstream.Close()
	service := newCloudTestService(t, upstream.URL)

	res := service.ListOpenBrainSources(context.Background())
	if !res.Success || len(res.Sources) != 1 {
		t.Fatalf("unexpected response: %+v", res)
	}

	cached := service.ListCachedOpenBrainSources(context.Background())
	if !cached.Success || len(cached.Sources) != 1 {
		t.Fatalf("unexpected cached response: %+v", cached)
	}
	source := cached.Sources[0]
	if source.Name != "Alpha Cloud" || !source.DisabledQueries || !source.PublicAccess || source.UpdatedAt != "2026-06-20T01:02:03Z" {
		t.Fatalf("cached source did not preserve cloud fields: %+v", source)
	}
}

func TestListCachedOpenBrainSourcesPrefersSnapshotOverWorkspaceIndex(t *testing.T) {
	service := newCloudTestService(t, "http://127.0.0.1.invalid")
	auth, err := service.loadAuth()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.saveCloudSourcesSnapshot(auth, []Source{{
		SourceID:        "ws-alpha",
		WorkspaceID:     "ws-alpha",
		OrgID:           "cloud",
		Name:            "Snapshot Alpha",
		BrainID:         "personal",
		UpdatedAt:       "2026-06-20T01:02:03Z",
		Federated:       true,
		PublicAccess:    true,
		DisabledQueries: true,
	}}); err != nil {
		t.Fatal(err)
	}
	indexPath := service.workspaceIndexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	rawIndex := `{"version":2,"accounts":{"user-bob":{"workspaces":[{"workspaceID":"ws-alpha","orgID":"cloud","localName":"Index Alpha","path":"/tmp/alpha","createdAt":"2026-06-01T00:00:00Z","updatedAt":"2026-06-01T00:00:00Z"}]}}}`
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	res := service.ListCachedOpenBrainSources(context.Background())
	if !res.Success || len(res.Sources) != 1 {
		t.Fatalf("unexpected response: %+v", res)
	}
	if res.Sources[0].Name != "Snapshot Alpha" || !res.Sources[0].PublicAccess || !res.Sources[0].DisabledQueries {
		t.Fatalf("cached sources should prefer full snapshot, got %+v", res.Sources[0])
	}
}

func TestListCachedOpenBrainSourcesIgnoresSnapshotForDifferentAuth(t *testing.T) {
	service := newCloudTestService(t, "http://127.0.0.1.invalid")
	if err := service.saveCloudSourcesSnapshotFile(cloudSourcesSnapshotFile{
		Version:   1,
		FetchedAt: "2026-06-20T01:02:03Z",
		UID:       "user-other",
		OrgID:     "cloud",
		Provider:  "cloud",
		Sources: []Source{{
			SourceID:    "ws-alpha",
			WorkspaceID: "ws-alpha",
			OrgID:       "cloud",
			Name:        "Other User Alpha",
			BrainID:     "personal",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	indexPath := service.workspaceIndexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	rawIndex := `{"version":2,"accounts":{"user-bob":{"workspaces":[{"workspaceID":"ws-alpha","orgID":"cloud","localName":"Index Alpha","path":"/tmp/alpha","createdAt":"2026-06-01T00:00:00Z","updatedAt":"2026-06-01T00:00:00Z"}]}}}`
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	res := service.ListCachedOpenBrainSources(context.Background())
	if !res.Success || len(res.Sources) != 1 {
		t.Fatalf("unexpected response: %+v", res)
	}
	if res.Sources[0].Name != "Index Alpha" {
		t.Fatalf("mismatched snapshot should fall back to index, got %+v", res.Sources[0])
	}
}

func TestListOpenBrainSourcesRekeysWorkspaceIndexByRepository(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/brain/workspaces" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer session-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"workspaces":[{
			"id":"ws-new",
			"orgID":"cloud",
			"name":"note",
			"repoProvider":"github",
			"repoOwner":"colinagent",
			"repoName":"note",
			"repoURL":"https://github.com/colinagent/note.git",
			"storageRemoteURL":"https://github.com/colinagent/note",
			"repoExternalID":"1270069234",
			"publicAccess":true
		}]}`))
	}))
	defer upstream.Close()
	service := newCloudTestService(t, upstream.URL)
	indexPath := service.workspaceIndexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	repoPath := initGitHubRepo(t, "colinagent", "note")
	rawIndex := `{
		"version": 1,
		"workspaces": [{
			"workspaceID": "ws-old",
			"orgID": "cloud",
			"localName": "note",
			"path": "__REPO_PATH__",
			"locationKind": "local",
			"templateID": "openbrain-cloud",
			"templateVersion": 1,
			"backupEnabled": true,
			"repository": {
				"provider": "github",
				"remoteURL": "https://github.com/colinagent/note.git",
				"owner": "colinagent",
				"name": "note",
				"externalID": "1270069234"
			},
			"storage": {"provider": "github", "remoteURL": "https://github.com/colinagent/note"},
			"createdAt": "2026-06-01T00:00:00Z",
			"updatedAt": "2026-06-01T00:00:00Z"
		}]
	}`
	rawIndex = strings.ReplaceAll(rawIndex, "__REPO_PATH__", repoPath)
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	res := service.ListOpenBrainSources(context.Background())
	if !res.Success || len(res.Sources) != 1 {
		t.Fatalf("unexpected response: %+v", res)
	}
	source := res.Sources[0]
	if source.WorkspaceID != "ws-new" || source.SourceID != "ws-new" {
		t.Fatalf("workspace id was not rekeyed in response: %+v", source)
	}
	if source.Path != repoPath || !source.Openable {
		t.Fatalf("local path was not preserved: %+v", source)
	}
	if !source.PublicAccess {
		t.Fatalf("public access was not forwarded: %+v", source)
	}

	var saved workspaceIndexFile
	rawSaved, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawSaved, &saved); err != nil {
		t.Fatal(err)
	}
	account := saved.Accounts["user-bob"]
	if account == nil || len(account.Workspaces) != 1 || account.Workspaces[0].WorkspaceID != "ws-new" || account.Workspaces[0].Path != repoPath {
		t.Fatalf("unexpected saved index: %+v", saved.Accounts)
	}
}

func TestCloudWorkspaceRepositoryMatchesPrefersExternalID(t *testing.T) {
	workspace := cloudWorkspace{
		RepoProvider:   "github",
		RepoOwner:      "colinagent",
		RepoName:       "openbrain-renamed",
		RepoURL:        "https://github.com/colinagent/openbrain-renamed.git",
		RepoExternalID: "1270069234",
	}

	if !cloudWorkspaceRepositoryMatches(workspace, githubRepoRef{Owner: "colinagent", Name: "openbrain", ExternalID: "1270069234"}) {
		t.Fatalf("expected matching external ID to win over owner/name drift")
	}
	if cloudWorkspaceRepositoryMatches(workspace, githubRepoRef{Owner: "colinagent", Name: "openbrain-renamed", ExternalID: "999"}) {
		t.Fatalf("external ID mismatch must not fall back to owner/name")
	}
	if !cloudWorkspaceRepositoryMatches(workspace, githubRepoRef{Owner: "colinagent", Name: "openbrain-renamed"}) {
		t.Fatalf("expected owner/name fallback when local external ID is missing")
	}
	if !cloudWorkspaceRepositoryMatches(workspace, githubRepoRef{RemoteURL: "git@github.com:colinagent/openbrain-renamed.git"}) {
		t.Fatalf("expected canonical GitHub remote fallback")
	}
}

func TestVerifyCloudWorkspaceBindingStates(t *testing.T) {
	service := NewService(t.TempDir())
	workspace := cloudWorkspace{
		ID:             "ws-openbrain",
		OrgID:          "cloud",
		Name:           "openbrain",
		RepoProvider:   "github",
		RepoOwner:      "colinagent",
		RepoName:       "openbrain",
		RepoURL:        "https://github.com/colinagent/openbrain.git",
		RepoExternalID: "1270069234",
	}

	connectedPath := initGitHubRepo(t, "colinagent", "openbrain")
	connected, changed := service.verifyCloudWorkspaceBinding(context.Background(), workspace, workspaceIndexEntry{
		WorkspaceID: "ws-openbrain",
		OrgID:       "cloud",
		Path:        connectedPath,
	}, true)
	if !changed || connected.LastVerifyReason != sourceBindingReasonConnected {
		t.Fatalf("expected connected verification, got changed=%v entry=%+v", changed, connected)
	}
	source := sourceFromCloudWorkspace(workspace, connected)
	if source.BindingStatus != sourceBindingConnected || !source.Openable {
		t.Fatalf("expected connected source, got %+v", source)
	}

	moved, _ := service.verifyCloudWorkspaceBinding(context.Background(), workspace, workspaceIndexEntry{
		WorkspaceID: "ws-openbrain",
		OrgID:       "cloud",
		Path:        filepath.Join(t.TempDir(), "missing"),
	}, true)
	if moved.LastVerifyReason != sourceBindingReasonMoved {
		t.Fatalf("expected moved reason, got %+v", moved)
	}

	mismatchPath := initGitHubRepo(t, "colinagent", "note")
	mismatch, _ := service.verifyCloudWorkspaceBinding(context.Background(), workspace, workspaceIndexEntry{
		WorkspaceID: "ws-openbrain",
		OrgID:       "cloud",
		Path:        mismatchPath,
	}, true)
	if mismatch.LastVerifyReason != sourceBindingReasonMismatch {
		t.Fatalf("expected mismatch reason, got %+v", mismatch)
	}

	unbound := sourceFromCloudWorkspace(workspace, workspaceIndexEntry{})
	if unbound.BindingStatus != sourceBindingNeedsBinding || unbound.BindingReason != sourceBindingReasonUnbound || unbound.Openable {
		t.Fatalf("expected unbound source, got %+v", unbound)
	}
}

func TestFindWorkspaceIndexByRepoRejectsExternalIDMismatch(t *testing.T) {
	workspace := cloudWorkspace{
		RepoProvider:   "github",
		RepoOwner:      "colinagent",
		RepoName:       "openbrain",
		RepoExternalID: "1270069234",
	}
	entries := []workspaceIndexEntry{{
		WorkspaceID: "ws-old",
		Repository: map[string]interface{}{
			"provider":   "github",
			"owner":      "colinagent",
			"name":       "openbrain",
			"externalID": "999",
		},
	}}
	indexByRepoKey := map[string][]int{}
	for i, entry := range entries {
		for _, key := range workspaceIndexRepoKeys(entry) {
			indexByRepoKey[key] = append(indexByRepoKey[key], i)
		}
	}

	if _, ok := findWorkspaceIndexByRepo(workspace, entries, indexByRepoKey, map[int]bool{}); ok {
		t.Fatalf("external ID mismatch must not rekey by owner/name")
	}
}

func TestUpsertCloudWorkspaceIndexRejectsPathCollision(t *testing.T) {
	service := NewService(t.TempDir())
	indexPath := service.workspaceIndexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	rawIndex := `{
		"version": 2,
		"accounts": {
			"user-bob": {
				"workspaces": [{
			"workspaceID": "ws-alpha",
			"orgID": "cloud",
			"localName": "alpha",
			"path": "/tmp/openbrain-alpha",
			"locationKind": "local",
			"templateID": "openbrain-cloud",
			"templateVersion": 1,
			"backupEnabled": true,
			"createdAt": "2026-06-01T00:00:00Z",
			"updatedAt": "2026-06-01T00:00:00Z"
				}]
			}
		}
	}`
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := service.upsertCloudWorkspaceIndex(authConfig{UID: "user-bob"}, createWorkspaceResult{
		WorkspaceID:      "ws-beta",
		OrgID:            "cloud",
		TemplateID:       cloudWorkspaceTemplateID,
		TemplateVersion:  1,
		BackupEnabled:    true,
		DefaultLocalName: "beta",
		Repository:       map[string]interface{}{"provider": "github", "owner": "colinagent", "name": "beta"},
		Storage:          map[string]interface{}{"provider": "github", "remoteURL": "https://github.com/colinagent/beta.git"},
	}, "/tmp/openbrain-alpha", "beta", false)
	if !errors.Is(err, errWorkspacePathConflict) {
		t.Fatalf("expected path conflict, got %v", err)
	}

	var saved workspaceIndexFile
	rawSaved, readErr := os.ReadFile(indexPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err := json.Unmarshal(rawSaved, &saved); err != nil {
		t.Fatal(err)
	}
	account := saved.Accounts["user-bob"]
	if account == nil || len(account.Workspaces) != 1 || account.Workspaces[0].WorkspaceID != "ws-alpha" {
		t.Fatalf("path collision should not overwrite old binding: %+v", saved.Accounts)
	}
}

func TestUpsertCloudWorkspaceIndexRequiresTakeoverForOtherAccountPath(t *testing.T) {
	service := NewService(t.TempDir())
	indexPath := service.workspaceIndexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	rawIndex := `{
		"version": 2,
		"accounts": {
			"user-alice": {
				"workspaces": [{
					"workspaceID": "ws-alpha",
					"orgID": "cloud",
					"localName": "alpha",
					"path": "/tmp/openbrain-alpha",
					"locationKind": "local",
					"templateID": "openbrain-cloud",
					"templateVersion": 1,
					"backupEnabled": true,
					"createdAt": "2026-06-01T00:00:00Z",
					"updatedAt": "2026-06-01T00:00:00Z"
				}]
			}
		}
	}`
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	workspace := createWorkspaceResult{
		WorkspaceID:      "ws-beta",
		OrgID:            "cloud",
		TemplateID:       cloudWorkspaceTemplateID,
		TemplateVersion:  1,
		BackupEnabled:    true,
		DefaultLocalName: "beta",
		Repository:       map[string]interface{}{"provider": "github", "owner": "colinagent", "name": "beta"},
		Storage:          map[string]interface{}{"provider": "github", "remoteURL": "https://github.com/colinagent/beta.git"},
	}
	_, err := service.upsertCloudWorkspaceIndex(authConfig{UID: "user-bob"}, workspace, "/tmp/openbrain-alpha", "beta", false)
	var ownerErr *workspacePathOwnerError
	if !errors.As(err, &ownerErr) || ownerErr.ownerUID != "user-alice" {
		t.Fatalf("expected other account path owner error, got %v", err)
	}

	entry, err := service.upsertCloudWorkspaceIndex(authConfig{UID: "user-bob"}, workspace, "/tmp/openbrain-alpha", "beta", true)
	if err != nil {
		t.Fatal(err)
	}
	if entry.WorkspaceID != "ws-beta" || entry.Path != "/tmp/openbrain-alpha" {
		t.Fatalf("unexpected takeover entry: %+v", entry)
	}
	var saved workspaceIndexFile
	rawSaved, readErr := os.ReadFile(indexPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err := json.Unmarshal(rawSaved, &saved); err != nil {
		t.Fatal(err)
	}
	if alice := saved.Accounts["user-alice"]; alice == nil || len(alice.Workspaces) != 0 {
		t.Fatalf("takeover should remove old account path binding: %+v", saved.Accounts)
	}
	bob := saved.Accounts["user-bob"]
	if bob == nil || len(bob.Workspaces) != 1 || bob.Workspaces[0].WorkspaceID != "ws-beta" {
		t.Fatalf("takeover should bind current account: %+v", saved.Accounts)
	}
}

func TestCreateOpenBrainSourceBindsExistingWorkspace(t *testing.T) {
	postWorkspaceCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/brain/workspaces/resolve-by-repo":
			if r.URL.Query().Get("owner") != "colinagent" || r.URL.Query().Get("name") != "openbrain" {
				t.Fatalf("unexpected resolve query %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{
				"resolution":"own",
				"effectivePermission":"admin",
				"canMutateSource":true,
				"workspace":{
					"id":"ws-openbrain",
					"orgID":"cloud",
					"name":"openbrain",
					"repoProvider":"github",
					"repoOwner":"colinagent",
					"repoName":"openbrain",
					"repoURL":"https://github.com/colinagent/openbrain.git",
					"repoExternalID":"1270069234",
					"effectivePermission":"admin",
					"canMutateSource":true
				}
			}`))
		case "/v1/workspaces":
			postWorkspaceCalled = true
			w.WriteHeader(http.StatusInternalServerError)
		case "/v1/orgs/cloud/workspaces/ws-openbrain/git-token":
			t.Fatalf("existing workspace bind must not fetch a git token")
		case "/v1/orgs/cloud/workspaces/ws-openbrain/brain/sync":
			t.Fatalf("existing workspace bind must not trigger brain sync")
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	service := newCloudTestService(t, upstream.URL)
	repoPath := initGitHubRepo(t, "colinagent", "openbrain")
	res := service.CreateOpenBrainSource(context.Background(), createSourceRequest{
		WorkspaceID: "ws-openbrain",
		OrgID:       "cloud",
		Name:        "openbrain",
		LocalPath:   repoPath,
	})
	if !res.Success || res.Workspace == nil {
		t.Fatalf("expected bind success, got %+v", res)
	}
	if postWorkspaceCalled {
		t.Fatalf("rebind must not create a new cloud workspace")
	}
	if res.Workspace.Path != repoPath || res.Workspace.BindingStatus != sourceBindingConnected {
		t.Fatalf("unexpected bound workspace: %+v", res.Workspace)
	}
}

func TestCreateOpenBrainSourceBindsGrantedWorkspaceWithoutMutatingCloud(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/brain/workspaces/resolve-by-repo":
			_, _ = w.Write([]byte(`{
				"resolution":"granted",
				"effectivePermission":"read",
				"canMutateSource":false,
				"publicOwnerUID":"user-alice",
				"workspace":{
					"id":"ws-shared",
					"orgID":"cloud",
					"name":"shared research",
					"repoProvider":"github",
					"storageBackend":"git",
					"storageProvider":"github",
					"defaultBranch":"main",
					"effectivePermission":"read",
					"publicOwnerUID":"user-alice"
				}
			}`))
		case "/v1/workspaces":
			t.Fatalf("granted source bind must not create a cloud workspace")
		case "/v1/orgs/cloud/workspaces/ws-shared/git-token":
			t.Fatalf("granted source bind must not fetch a git token")
		case "/v1/orgs/cloud/workspaces/ws-shared/brain/sync":
			t.Fatalf("granted source bind must not trigger brain sync")
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	service := newCloudTestService(t, upstream.URL)
	repoPath := initGitHubRepo(t, "alice", "research")
	res := service.CreateOpenBrainSource(context.Background(), createSourceRequest{
		Name:      "shared research",
		LocalPath: repoPath,
	})
	if !res.Success || res.Workspace == nil {
		t.Fatalf("expected bind success, got %+v", res)
	}
	if res.Workspace.WorkspaceID != "ws-shared" || res.Workspace.BindingMode != "granted" || res.Workspace.CanMutateSource {
		t.Fatalf("unexpected granted workspace: %+v", res.Workspace)
	}
	if res.Workspace.PublicOwnerUID != "user-alice" || res.Workspace.EffectivePermission != "read" {
		t.Fatalf("unexpected granted metadata: %+v", res.Workspace)
	}
}

func TestCreateOpenBrainSourceReportsAmbiguousRepository(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/brain/workspaces/resolve-by-repo":
			_, _ = w.Write([]byte(`{"resolution":"ambiguous"}`))
		case "/v1/workspaces":
			t.Fatalf("ambiguous source must not create a cloud workspace")
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	service := newCloudTestService(t, upstream.URL)
	repoPath := initGitHubRepo(t, "alice", "research")
	res := service.CreateOpenBrainSource(context.Background(), createSourceRequest{
		Name:      "research",
		LocalPath: repoPath,
	})
	if res.Success || res.Code != "workspace_repo_ambiguous" {
		t.Fatalf("response = %+v, want ambiguous repository failure", res)
	}
}

func TestCreateOpenBrainSourceRejectsExistingGitRepositoryWithoutOriginBeforeCloudCreate(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Fatalf("cloud request must not run for invalid local git repository: %s %s", r.Method, r.URL.Path)
	}))
	defer upstream.Close()

	service := newCloudTestService(t, upstream.URL)
	workspacePath := t.TempDir()
	runGBrainTestGit(t, workspacePath, "init")
	if err := os.WriteFile(filepath.Join(workspacePath, "note.md"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := service.CreateOpenBrainSource(context.Background(), createSourceRequest{
		Name:      "workspace",
		LocalPath: workspacePath,
	})
	if res.Success || res.Code != "invalid_repository" {
		t.Fatalf("response = %+v, want invalid_repository", res)
	}
	if requests != 0 {
		t.Fatalf("cloud requests = %d, want none", requests)
	}
	if _, err := runGit(context.Background(), workspacePath, nil, "remote", "get-url", "origin"); err == nil {
		t.Fatal("origin remote was added to an existing git repository without origin")
	}
	if _, err := runGit(context.Background(), workspacePath, nil, "rev-parse", "--verify", "HEAD"); err == nil {
		t.Fatal("commit was created in an existing git repository without origin")
	}
}

func TestCreateOpenBrainSourceRollbackKeepsExistingPathBinding(t *testing.T) {
	var rollbackCalled bool
	var syncCalled bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/brain/workspaces/resolve-by-repo":
			_, _ = w.Write([]byte(`{"resolution":"not_found"}`))
		case "/v1/workspaces":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"workspaceID":      "ws-new",
				"orgID":            "cloud",
				"templateID":       "openbrain-cloud",
				"templateVersion":  1,
				"backupEnabled":    true,
				"defaultLocalName": "research",
				"repository": map[string]interface{}{
					"enabled":       true,
					"provider":      "github",
					"owner":         "alice",
					"name":          "research",
					"remoteURL":     "https://github.com/alice/research.git",
					"defaultBranch": "main",
				},
				"storage": map[string]interface{}{
					"enabled":   true,
					"backend":   "git",
					"provider":  "github",
					"remoteURL": "https://github.com/alice/research.git",
				},
				"effectivePermission": "admin",
				"canMutateSource":     true,
			})
		case "/v1/orgs/cloud/workspaces/ws-new/brain/sync":
			syncCalled = true
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/v1/orgs/cloud/workspaces/ws-new/rollback-create":
			rollbackCalled = true
			var payload struct {
				DeleteRepository bool `json:"deleteRepository"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.DeleteRepository {
				t.Fatal("rollback must not delete a pre-existing repository")
			}
			_, _ = w.Write([]byte(`{"ok":true,"orgID":"cloud","workspaceID":"ws-new","sourceID":"ws-new"}`))
		case "/v1/orgs/cloud/workspaces/ws-new/git-token":
			t.Fatal("existing GitHub repository create must not fetch an import token")
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	service := newCloudTestService(t, upstream.URL)
	repoPath := initGitHubRepo(t, "alice", "research")
	existingEntry := workspaceIndexEntry{
		WorkspaceID:      "ws-existing",
		OrgID:            "cloud",
		LocalName:        "existing local binding",
		Path:             repoPath,
		LocationKind:     "local",
		TemplateID:       "legacy-local",
		TemplateVersion:  1,
		BackupEnabled:    false,
		CreatedAt:        "2026-06-01T00:00:00Z",
		UpdatedAt:        "2026-06-01T00:00:00Z",
		LastVerifiedAt:   "2026-06-01T00:00:00Z",
		LastVerifyReason: sourceBindingReasonConnected,
	}
	if err := service.saveWorkspaceIndex(workspaceIndexFile{
		ActiveUID:  "user-bob",
		Workspaces: []workspaceIndexEntry{existingEntry},
	}); err != nil {
		t.Fatal(err)
	}

	res := service.CreateOpenBrainSource(context.Background(), createSourceRequest{
		Name:      "research",
		LocalPath: repoPath,
	})
	if res.Success || res.Code != "workspace_path_conflict" {
		t.Fatalf("response = %+v, want workspace_path_conflict", res)
	}
	if !syncCalled || !rollbackCalled || !res.CleanupAttempted || !res.CleanupSucceeded || res.CleanupError != "" {
		t.Fatalf("rollback state mismatch: sync=%v rollback=%v response=%+v", syncCalled, rollbackCalled, res)
	}
	index, err := service.loadWorkspaceIndex(authConfig{UID: "user-bob"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	existing, _ := findWorkspaceIndexEntry(index.Workspaces, "ws-existing", "cloud")
	if existing.WorkspaceID != "ws-existing" || existing.Path != repoPath {
		t.Fatalf("existing binding was removed or changed: %+v", index.Workspaces)
	}
	if created, _ := findWorkspaceIndexEntry(index.Workspaces, "ws-new", "cloud"); created.WorkspaceID != "" {
		t.Fatalf("rolled back workspace binding was persisted: %+v", created)
	}
	if len(index.HiddenWorkspaces) != 0 {
		t.Fatalf("rollback should not hide unrelated bindings: %+v", index.HiddenWorkspaces)
	}
}

func TestCreateOpenBrainSourceRollsBackWhenInitialGitImportFails(t *testing.T) {
	root := t.TempDir()
	missingRemote := filepath.Join(root, "missing.git")
	var rollbackCalled bool
	var rollbackDeleteRepository bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/workspaces":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"workspaceID":      "ws-workspace",
				"orgID":            "cloud",
				"templateID":       "openbrain-cloud",
				"templateVersion":  1,
				"backupEnabled":    true,
				"defaultLocalName": "workspace",
				"repository": map[string]interface{}{
					"enabled":       true,
					"provider":      "github",
					"owner":         "colinagent",
					"name":          "workspace",
					"remoteURL":     missingRemote,
					"defaultBranch": "main",
				},
				"storage": map[string]interface{}{
					"enabled":   true,
					"backend":   "git",
					"provider":  "github",
					"remoteURL": missingRemote,
				},
				"effectivePermission": "admin",
				"canMutateSource":     true,
			})
		case "/v1/workspace-templates":
			_, _ = w.Write([]byte(`{"templates":[{"templateID":"openbrain-cloud","repository":{"providers":[{"provider":"github","accounts":[{"owner":"colinagent","canCreateRepository":true,"canSyncRepository":true}]}]}}]}`))
		case "/v1/orgs/cloud/workspaces/ws-workspace/git-token":
			_, _ = w.Write([]byte(`{"provider":"github","username":"x-access-token","accessToken":"token","remoteURL":""}`))
		case "/v1/orgs/cloud/workspaces/ws-workspace/brain/sync":
			t.Fatalf("brain sync must not run when initial git import fails")
		case "/v1/orgs/cloud/workspaces/ws-workspace/rollback-create":
			rollbackCalled = true
			var payload struct {
				CreateRequestID    string `json:"createRequestID"`
				DeleteRepository   bool   `json:"deleteRepository"`
				ConfirmWorkspaceID string `json:"confirmWorkspaceID"`
				ConfirmName        string `json:"confirmName"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.CreateRequestID == "" || payload.ConfirmWorkspaceID != "ws-workspace" || payload.ConfirmName != "workspace" {
				t.Fatalf("rollback payload = %+v", payload)
			}
			rollbackDeleteRepository = payload.DeleteRepository
			_, _ = w.Write([]byte(`{"ok":true,"orgID":"cloud","workspaceID":"ws-workspace","sourceID":"ws-workspace","repositoryDeleted":true}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	service := newCloudTestService(t, upstream.URL)
	workspacePath := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "note.md"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := service.CreateOpenBrainSource(context.Background(), createSourceRequest{
		Name:      "workspace",
		LocalPath: workspacePath,
	})
	if res.Success || res.Code != "git_error" {
		t.Fatalf("response = %+v, want git_error", res)
	}
	if res.Workspace != nil || !res.CleanupAttempted || !res.CleanupSucceeded || res.CleanupError != "" {
		t.Fatalf("response should be failed and cleaned with no workspace, got %+v", res)
	}
	if !rollbackCalled || !rollbackDeleteRepository {
		t.Fatalf("rollback called = %v deleteRepository = %v, want both true", rollbackCalled, rollbackDeleteRepository)
	}
	if _, err := os.Stat(filepath.Join(workspacePath, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".git stat err = %v, want removed", err)
	}

	index, err := service.loadWorkspaceIndex(authConfig{UID: "user-bob"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := findWorkspaceIndexEntry(index.Workspaces, "ws-workspace", "cloud")
	if entry.WorkspaceID != "" {
		t.Fatalf("binding was persisted after git import failure: %+v", entry)
	}
	if len(index.HiddenWorkspaces) != 0 {
		t.Fatalf("hidden workspaces = %+v, want none", index.HiddenWorkspaces)
	}
}

func TestVerifyGrantedWorkspaceUsesIndexRepositoryWhenCloudWorkspaceIsSanitized(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/brain/workspaces":
			_, _ = w.Write([]byte(`{"workspaces":[{
				"id":"ws-shared",
				"orgID":"cloud",
				"name":"shared research",
				"repoProvider":"github",
				"storageBackend":"git",
				"storageProvider":"github",
				"defaultBranch":"main",
				"effectivePermission":"read"
			}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	service := newCloudTestService(t, upstream.URL)
	repoPath := initGitHubRepo(t, "alice", "research")
	indexPath := service.workspaceIndexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	rawIndex := `{
		"version": 2,
		"accounts": {
			"user-bob": {
				"workspaces": [{
			"workspaceID": "ws-shared",
			"orgID": "cloud",
			"localName": "shared research",
			"path": "__REPO_PATH__",
			"locationKind": "local",
			"templateID": "openbrain-cloud",
			"templateVersion": 1,
			"backupEnabled": true,
			"repository": {"provider": "github", "owner": "alice", "name": "research", "remoteURL": "https://github.com/alice/research.git"},
			"storage": {"provider": "github", "remoteURL": "https://github.com/alice/research.git"},
			"effectivePermission": "read",
			"canMutateSource": false,
			"bindingMode": "granted",
			"createdAt": "2026-06-01T00:00:00Z",
			"updatedAt": "2026-06-01T00:00:00Z"
				}]
			}
		}
	}`
	rawIndex = strings.ReplaceAll(rawIndex, "__REPO_PATH__", repoPath)
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	res := service.VerifyOpenBrainSource(context.Background(), mutationRequest{WorkspaceID: "ws-shared", OrgID: "cloud"})
	if !res.Success || res.Workspace == nil || res.Workspace.BindingStatus != sourceBindingConnected {
		t.Fatalf("response = %+v, want connected granted workspace", res)
	}
	if res.Workspace.CanMutateSource || res.Workspace.BindingMode != "granted" || res.Workspace.EffectivePermission != "read" {
		t.Fatalf("workspace permissions = %+v, want granted read-only", res.Workspace)
	}
}

func TestRecoveryCandidatesReturnMatchingPathWithoutWritingIndex(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/brain/workspaces":
			_, _ = w.Write([]byte(`{"workspaces":[{
				"id":"ws-openbrain",
				"orgID":"cloud",
				"name":"openbrain",
				"repoProvider":"github",
				"repoOwner":"colinagent",
				"repoName":"openbrain",
				"repoURL":"https://github.com/colinagent/openbrain.git",
				"repoExternalID":"1270069234"
			}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	service := newCloudTestService(t, upstream.URL)
	indexPath := service.workspaceIndexPath()
	before := []byte(`{"version":1,"workspaces":[]}`)
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, before, 0o644); err != nil {
		t.Fatal(err)
	}

	matchPath := initGitHubRepo(t, "colinagent", "openbrain")
	mismatchPath := initGitHubRepo(t, "colinagent", "note")
	res := service.ListOpenBrainSourceRecoveryCandidates(context.Background(), recoveryCandidatesRequest{
		WorkspaceID: "ws-openbrain",
		OrgID:       "cloud",
		Paths:       []string{mismatchPath, matchPath, matchPath},
	})
	if !res.Success || len(res.Candidates) != 1 || res.Candidates[0].Path != matchPath {
		t.Fatalf("response = %+v, want exactly matching candidate", res)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("recovery candidates must not write index: %s", string(after))
	}
}

func TestRecoveryCandidatesUseGrantedIndexRepositoryWhenCloudWorkspaceIsSanitized(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/brain/workspaces":
			_, _ = w.Write([]byte(`{"workspaces":[{
				"id":"ws-shared",
				"orgID":"cloud",
				"name":"shared research",
				"repoProvider":"github",
				"storageBackend":"git",
				"storageProvider":"github",
				"effectivePermission":"read"
			}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	service := newCloudTestService(t, upstream.URL)
	indexPath := service.workspaceIndexPath()
	rawIndex := `{
		"version": 2,
		"accounts": {
			"user-bob": {
				"workspaces": [{
			"workspaceID": "ws-shared",
			"orgID": "cloud",
			"localName": "shared research",
			"path": "",
			"locationKind": "local",
			"templateID": "openbrain-cloud",
			"templateVersion": 1,
			"backupEnabled": true,
			"repository": {"provider": "github", "owner": "alice", "name": "research", "remoteURL": "https://github.com/alice/research.git"},
			"storage": {"provider": "github", "remoteURL": "https://github.com/alice/research.git"},
			"effectivePermission": "read",
			"canMutateSource": false,
			"bindingMode": "granted",
			"createdAt": "2026-06-01T00:00:00Z",
			"updatedAt": "2026-06-01T00:00:00Z"
				}]
			}
		}
	}`
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	matchPath := initGitHubRepo(t, "alice", "research")
	res := service.ListOpenBrainSourceRecoveryCandidates(context.Background(), recoveryCandidatesRequest{
		WorkspaceID: "ws-shared",
		OrgID:       "cloud",
		Paths:       []string{matchPath},
	})
	if !res.Success || len(res.Candidates) != 1 || res.Candidates[0].Path != matchPath {
		t.Fatalf("response = %+v, want granted matching candidate", res)
	}
}

func TestUpsertCloudWorkspaceIndexUpdatesSameWorkspacePathAndUnhides(t *testing.T) {
	service := NewService(t.TempDir())
	indexPath := service.workspaceIndexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	rawIndex := `{
		"version": 2,
		"accounts": {
			"user-bob": {
				"workspaces": [{
			"workspaceID": "ws-alpha",
			"orgID": "cloud",
			"localName": "alpha",
			"path": "/tmp/openbrain-old",
			"locationKind": "local",
			"templateID": "openbrain-cloud",
			"templateVersion": 1,
			"backupEnabled": true,
			"createdAt": "2026-06-01T00:00:00Z",
			"updatedAt": "2026-06-01T00:00:00Z"
				}],
				"hiddenWorkspaces": [{
			"workspaceID": "ws-alpha",
			"orgID": "cloud",
			"hiddenAt": "2026-06-02T00:00:00Z"
				}]
			}
		}
	}`
	if err := os.WriteFile(indexPath, []byte(rawIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, err := service.upsertCloudWorkspaceIndex(authConfig{UID: "user-bob"}, createWorkspaceResult{
		WorkspaceID:      "ws-alpha",
		OrgID:            "cloud",
		TemplateID:       cloudWorkspaceTemplateID,
		TemplateVersion:  1,
		BackupEnabled:    true,
		DefaultLocalName: "alpha",
		Repository:       map[string]interface{}{"provider": "github", "owner": "colinagent", "name": "alpha"},
		Storage:          map[string]interface{}{"provider": "github", "remoteURL": "https://github.com/colinagent/alpha.git"},
	}, "/tmp/openbrain-new", "alpha", false)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Path != "/tmp/openbrain-new" || entry.CreatedAt != "2026-06-01T00:00:00Z" {
		t.Fatalf("unexpected updated entry: %+v", entry)
	}

	var saved workspaceIndexFile
	rawSaved, readErr := os.ReadFile(indexPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err := json.Unmarshal(rawSaved, &saved); err != nil {
		t.Fatal(err)
	}
	account := saved.Accounts["user-bob"]
	if account == nil || len(account.Workspaces) != 1 || account.Workspaces[0].Path != "/tmp/openbrain-new" {
		t.Fatalf("expected same workspace path update: %+v", saved.Accounts)
	}
	if len(account.HiddenWorkspaces) != 0 {
		t.Fatalf("successful rebind should unhide workspace: %+v", account.HiddenWorkspaces)
	}
}

func TestCloudSourceActionUpdatesCloudSourcesSnapshot(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/orgs/cloud/workspaces/ws-alpha/brain/source-action" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["hardDelete"] == true {
			_, _ = w.Write([]byte(`{"workspaceID":"ws-alpha","orgID":"cloud","hardDeleted":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"workspaceID":"ws-alpha","orgID":"cloud","disabledQueries":true}`))
	}))
	defer upstream.Close()
	service := newCloudTestService(t, upstream.URL)
	auth, err := service.loadAuth()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.saveCloudSourcesSnapshot(auth, []Source{{
		SourceID:    "ws-alpha",
		WorkspaceID: "ws-alpha",
		OrgID:       "cloud",
		Name:        "Alpha",
		BrainID:     "personal",
	}}); err != nil {
		t.Fatal(err)
	}

	disabled := service.ApplyOpenBrainSourceAction(context.Background(), mutationRequest{
		WorkspaceID:    "ws-alpha",
		OrgID:          "cloud",
		DisableQueries: true,
	})
	if !disabled.Success {
		t.Fatalf("disable response = %+v", disabled)
	}
	cached := service.ListCachedOpenBrainSources(context.Background())
	if !cached.Success || len(cached.Sources) != 1 || !cached.Sources[0].DisabledQueries {
		t.Fatalf("snapshot was not updated after disable: %+v", cached)
	}

	deleted := service.ApplyOpenBrainSourceAction(context.Background(), mutationRequest{
		WorkspaceID: "ws-alpha",
		OrgID:       "cloud",
		HardDelete:  true,
	})
	if !deleted.Success {
		t.Fatalf("delete response = %+v", deleted)
	}
	cached = service.ListCachedOpenBrainSources(context.Background())
	if !cached.Success || len(cached.Sources) != 0 {
		t.Fatalf("snapshot source was not removed after delete: %+v", cached)
	}
}

func TestCloudSourcePublicAccessUpdatesCloudSourcesSnapshot(t *testing.T) {
	service := newCloudTestService(t, "http://127.0.0.1.invalid")
	auth, err := service.loadAuth()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.saveCloudSourcesSnapshot(auth, []Source{{
		SourceID:    "ws-alpha",
		WorkspaceID: "ws-alpha",
		OrgID:       "cloud",
		Name:        "Alpha",
		BrainID:     "personal",
	}}); err != nil {
		t.Fatal(err)
	}

	service.updateCurrentCloudSourceSnapshotPublicAccess("cloud", "ws-alpha", true)
	cached := service.ListCachedOpenBrainSources(context.Background())
	if !cached.Success || len(cached.Sources) != 1 || !cached.Sources[0].PublicAccess {
		t.Fatalf("snapshot was not updated after make public: %+v", cached)
	}

	service.updateCurrentCloudSourceSnapshotPublicAccess("cloud", "ws-alpha", false)
	cached = service.ListCachedOpenBrainSources(context.Background())
	if !cached.Success || len(cached.Sources) != 1 || cached.Sources[0].PublicAccess {
		t.Fatalf("snapshot was not updated after revoke public: %+v", cached)
	}
}

func TestListSourcesUnavailableUsesEmptySources(t *testing.T) {
	service := NewServiceWithRunner(t.TempDir(), func(ctx context.Context, args []string) (CommandResult, error) {
		return CommandResult{}, errors.Join(ErrGBrainUnavailable, errors.New("missing binary"))
	})
	res := service.ListSources(context.Background())
	if res.Success || res.Code != "gbrain_unavailable" || len(res.Sources) != 0 {
		t.Fatalf("unexpected response: %+v", res)
	}
}

func newCloudTestService(t *testing.T, gateway string) *Service {
	t.Helper()
	baseDir := t.TempDir()
	authDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"version":1,"gateway":` + strconv.Quote(gateway) + `,"token":"session-token","uid":"user-bob","defaultOrgID":"cloud","updatedAt":1}`
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	return NewService(baseDir)
}

func initGitHubRepo(t *testing.T, owner string, name string) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, string(out))
	}
	remoteURL := "https://github.com/" + owner + "/" + name + ".git"
	cmd = exec.Command("git", "remote", "add", "origin", remoteURL)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add failed: %v\n%s", err, string(out))
	}
	return dir
}

func TestImportGitWorkspaceRebasesRemoteInitialCommitBeforePush(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	seed := filepath.Join(root, "seed")
	local := filepath.Join(root, "local")

	runGBrainTestGit(t, root, "init", "--bare", remote)
	runGBrainTestGit(t, root, "clone", remote, seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGBrainTestGit(t, seed, "add", "README.md")
	runGBrainTestGit(t, seed, "-c", "user.name=OpenBrain", "-c", "user.email=test@example.com", "commit", "-m", "remote initial")
	runGBrainTestGit(t, seed, "push", "origin", "HEAD:main")

	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "local.md"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importGitWorkspace(context.Background(), local, remote, "main", nil, false); err != nil {
		t.Fatalf("importGitWorkspace failed: %v", err)
	}

	tree := runGBrainTestGitOutput(t, local, "ls-tree", "--name-only", "HEAD")
	if !strings.Contains(tree, "README.md") || !strings.Contains(tree, "local.md") {
		t.Fatalf("local tree after import = %q, want remote and local files", tree)
	}
	remoteTree := runGBrainTestGitOutput(t, root, "--git-dir", remote, "ls-tree", "--name-only", "main")
	if !strings.Contains(remoteTree, "README.md") || !strings.Contains(remoteTree, "local.md") {
		t.Fatalf("remote tree after import = %q, want remote and local files", remoteTree)
	}
}

func runGBrainTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = runGBrainTestGitOutput(t, dir, args...)
}

func runGBrainTestGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return string(out)
}
