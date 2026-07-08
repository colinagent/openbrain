package gbrain

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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

const cloudWorkspaceTemplateID = "openbrain-cloud"

var (
	errWorkspacePathConflict            = errors.New("workspace_path_conflict")
	errWorkspacePathOwnedByOtherAccount = errors.New("path_owned_by_other_account")
	errWorkspaceRepoAmbiguous           = errors.New("workspace_repo_ambiguous")
)

const (
	sourceBindingConnected    = "connected"
	sourceBindingNeedsBinding = "needs_binding"

	sourceBindingReasonUnbound   = "unbound"
	sourceBindingReasonMoved     = "moved"
	sourceBindingReasonMismatch  = "mismatch"
	sourceBindingReasonConnected = "connected"

	bindingVerifyCacheWindow = 30 * time.Second
)

type authConfig struct {
	Version        int    `json:"version"`
	BaseURL        string `json:"baseUrl"`
	Gateway        string `json:"gateway"`
	AIGateway      string `json:"aiGateway,omitempty"`
	DefaultOrgID   string `json:"defaultOrgID,omitempty"`
	DefaultOrgName string `json:"defaultOrgName,omitempty"`
	Token          string `json:"token"`
	UID            string `json:"uid"`
	Email          string `json:"email,omitempty"`
	ActiveOrgID    string `json:"activeOrgID,omitempty"`
	ActiveOrgName  string `json:"activeOrgName,omitempty"`
	UpdatedAt      int64  `json:"updatedAt"`
}

type cloudWorkspace struct {
	ID                  string `json:"id"`
	OrgID               string `json:"orgID"`
	Name                string `json:"name"`
	Slug                string `json:"slug"`
	RepoProvider        string `json:"repoProvider"`
	RepoOwner           string `json:"repoOwner"`
	RepoName            string `json:"repoName"`
	RepoURL             string `json:"repoURL"`
	RepoWebURL          string `json:"repoWebURL"`
	RepoExternalID      string `json:"repoExternalID"`
	StorageBackend      string `json:"storageBackend"`
	StorageProvider     string `json:"storageProvider"`
	StorageRemoteURL    string `json:"storageRemoteURL"`
	DefaultBranch       string `json:"defaultBranch"`
	DisabledQueries     bool   `json:"disabledQueries"`
	PublicAccess        bool   `json:"publicAccess"`
	EffectivePermission string `json:"effectivePermission"`
	CanMutateSource     bool   `json:"canMutateSource"`
	PublicOwnerUID      string `json:"publicOwnerUID"`
	Status              string `json:"status"`
	UpdatedAt           string `json:"updatedAt"`
}

type cloudWorkspaceResolveResult struct {
	Resolution          string         `json:"resolution"`
	Workspace           cloudWorkspace `json:"workspace"`
	EffectivePermission string         `json:"effectivePermission"`
	CanMutateSource     bool           `json:"canMutateSource"`
	PublicOwnerUID      string         `json:"publicOwnerUID"`
}

type cloudSearchResult struct {
	SourceID      string      `json:"sourceID"`
	SourceName    string      `json:"sourceName"`
	WorkspaceID   string      `json:"workspaceID"`
	WorkspaceName string      `json:"workspaceName"`
	Path          string      `json:"path"`
	Slug          string      `json:"slug"`
	Title         string      `json:"title"`
	ChunkID       interface{} `json:"chunkID"`
	ChunkIndex    interface{} `json:"chunkIndex"`
	ChunkText     string      `json:"chunkText"`
	Score         float64     `json:"score"`
}

type workspaceIndexFile struct {
	Version          int                               `json:"version"`
	Accounts         map[string]*workspaceIndexAccount `json:"accounts,omitempty"`
	Workspaces       []workspaceIndexEntry             `json:"workspaces,omitempty"`
	HiddenWorkspaces []hiddenWorkspaceEntry            `json:"hiddenWorkspaces,omitempty"`
	ActiveUID        string                            `json:"-"`
}

type workspaceIndexAccount struct {
	Workspaces       []workspaceIndexEntry  `json:"workspaces"`
	HiddenWorkspaces []hiddenWorkspaceEntry `json:"hiddenWorkspaces,omitempty"`
}

type cloudSourcesSnapshotFile struct {
	Version   int      `json:"version"`
	FetchedAt string   `json:"fetchedAt"`
	UID       string   `json:"uid"`
	OrgID     string   `json:"orgID,omitempty"`
	Provider  string   `json:"provider"`
	Sources   []Source `json:"sources"`
}

type workspaceIndexEntry struct {
	WorkspaceID         string                 `json:"workspaceID"`
	OrgID               string                 `json:"orgID,omitempty"`
	LocalName           string                 `json:"localName"`
	Path                string                 `json:"path"`
	LocationKind        string                 `json:"locationKind,omitempty"`
	TemplateID          string                 `json:"templateID,omitempty"`
	TemplateVersion     int                    `json:"templateVersion,omitempty"`
	BackupEnabled       bool                   `json:"backupEnabled"`
	Repository          map[string]interface{} `json:"repository,omitempty"`
	Storage             map[string]interface{} `json:"storage,omitempty"`
	SyncPolicy          map[string]interface{} `json:"syncPolicy,omitempty"`
	CreatedAt           string                 `json:"createdAt"`
	UpdatedAt           string                 `json:"updatedAt"`
	LastVerifiedAt      string                 `json:"lastVerifiedAt,omitempty"`
	LastVerifyReason    string                 `json:"lastVerifyReason,omitempty"`
	EffectivePermission string                 `json:"effectivePermission,omitempty"`
	CanMutateSource     bool                   `json:"canMutateSource,omitempty"`
	PublicOwnerUID      string                 `json:"publicOwnerUID,omitempty"`
	BindingMode         string                 `json:"bindingMode,omitempty"`
}

type hiddenWorkspaceEntry struct {
	WorkspaceID string `json:"workspaceID"`
	OrgID       string `json:"orgID,omitempty"`
	HiddenAt    string `json:"hiddenAt"`
}

type createSourceRequest struct {
	Name            string `json:"name,omitempty"`
	LocalPath       string `json:"localPath,omitempty"`
	Path            string `json:"path,omitempty"`
	SourceID        string `json:"sourceID,omitempty"`
	WorkspaceID     string `json:"workspaceID,omitempty"`
	OrgID           string `json:"orgID,omitempty"`
	Takeover        bool   `json:"takeover,omitempty"`
	CreateRequestID string `json:"createRequestID,omitempty"`
}

type mutationRequest struct {
	SourceID           string `json:"sourceID,omitempty"`
	WorkspaceID        string `json:"workspaceID,omitempty"`
	OrgID              string `json:"orgID,omitempty"`
	Path               string `json:"path,omitempty"`
	DisableQueries     bool   `json:"disableQueries,omitempty"`
	EnableQueries      bool   `json:"enableQueries,omitempty"`
	DisableSync        bool   `json:"disableSync,omitempty"`
	HardDelete         bool   `json:"hardDelete,omitempty"`
	ConfirmWorkspaceID string `json:"confirmWorkspaceID,omitempty"`
	ConfirmName        string `json:"confirmName,omitempty"`
}

type recoveryCandidatesRequest struct {
	SourceID    string   `json:"sourceID,omitempty"`
	WorkspaceID string   `json:"workspaceID,omitempty"`
	OrgID       string   `json:"orgID,omitempty"`
	Paths       []string `json:"paths,omitempty"`
}

type sourceActionResult struct {
	OK              bool   `json:"ok,omitempty"`
	OrgID           string `json:"orgID,omitempty"`
	WorkspaceID     string `json:"workspaceID,omitempty"`
	SourceID        string `json:"sourceID,omitempty"`
	DisabledQueries bool   `json:"disabledQueries,omitempty"`
	EnabledQueries  bool   `json:"enabledQueries,omitempty"`
	DisabledSync    bool   `json:"disabledSync,omitempty"`
	HardDeleted     bool   `json:"hardDeleted,omitempty"`
	SyncJobsRemoved int64  `json:"syncJobsRemoved,omitempty"`
	Status          string `json:"status,omitempty"`
}

type sourceCreateRollbackResult struct {
	OK                bool   `json:"ok,omitempty"`
	OrgID             string `json:"orgID,omitempty"`
	WorkspaceID       string `json:"workspaceID,omitempty"`
	SourceID          string `json:"sourceID,omitempty"`
	RepositoryDeleted bool   `json:"repositoryDeleted,omitempty"`
}

type createWorkspaceResult struct {
	WorkspaceID         string                 `json:"workspaceID"`
	OrgID               string                 `json:"orgID"`
	TemplateID          string                 `json:"templateID"`
	TemplateVersion     int                    `json:"templateVersion"`
	BackupEnabled       bool                   `json:"backupEnabled"`
	DefaultLocalName    string                 `json:"defaultLocalName"`
	Repository          map[string]interface{} `json:"repository,omitempty"`
	Storage             map[string]interface{} `json:"storage,omitempty"`
	Manifest            map[string]interface{} `json:"manifest,omitempty"`
	EffectivePermission string                 `json:"effectivePermission,omitempty"`
	CanMutateSource     bool                   `json:"canMutateSource,omitempty"`
	PublicOwnerUID      string                 `json:"publicOwnerUID,omitempty"`
	BindingMode         string                 `json:"bindingMode,omitempty"`
}

type workspaceTemplateListResult struct {
	Templates []workspaceTemplateView `json:"templates"`
}

type workspaceTemplateView struct {
	TemplateID string                     `json:"templateID"`
	Repository *workspaceRepositoryPolicy `json:"repository,omitempty"`
	Storage    *workspaceStoragePolicy    `json:"storage,omitempty"`
}

type workspaceRepositoryPolicy struct {
	Providers []workspaceProviderOption `json:"providers,omitempty"`
}

type workspaceStoragePolicy struct {
	Providers []workspaceProviderOption `json:"providers,omitempty"`
}

type workspaceProviderOption struct {
	Provider            string                   `json:"provider"`
	Accounts            []workspaceGitHubAccount `json:"accounts,omitempty"`
	CanCreateRepository *bool                    `json:"canCreateRepository,omitempty"`
	CanSyncRepository   *bool                    `json:"canSyncRepository,omitempty"`
}

type workspaceGitHubAccount struct {
	Owner               string `json:"owner"`
	CanCreateRepository *bool  `json:"canCreateRepository,omitempty"`
	CanSyncRepository   *bool  `json:"canSyncRepository,omitempty"`
}

type workspaceStorageConnectionsResult struct {
	Providers []workspaceProviderOption `json:"providers,omitempty"`
}

type workspaceGitToken struct {
	Provider             string `json:"provider,omitempty"`
	Username             string `json:"username,omitempty"`
	AccessToken          string `json:"accessToken,omitempty"`
	AccessTokenExpiresAt string `json:"accessTokenExpiresAt,omitempty"`
	RemoteURL            string `json:"remoteURL,omitempty"`
}

type githubRepoRef struct {
	Owner      string
	Name       string
	RemoteURL  string
	ExternalID string
}

type askpassEnv struct {
	dir string
	env []string
}

type workspacePathOwnerError struct {
	path        string
	ownerUID    string
	workspaceID string
}

func (e *workspacePathOwnerError) Error() string {
	return fmt.Sprintf("%s: %s is already bound to workspace %s by account %s", errWorkspacePathOwnedByOtherAccount, e.path, e.workspaceID, e.ownerUID)
}

func (e *workspacePathOwnerError) Unwrap() error {
	return errWorkspacePathOwnedByOtherAccount
}

func (s *Service) ListOpenBrainSources(ctx context.Context) ListSourcesResponse {
	auth, err := s.loadAuth()
	if err != nil {
		return ListSourcesResponse{Success: false, Code: "auth_required", Error: "Sign in required to use OpenBrain Cloud.", Provider: "cloud", AuthRequired: true, Sources: []Source{}}
	}
	workspaces, err := s.listCloudWorkspaces(ctx, auth)
	if err != nil {
		if isCloudAuthError(err) {
			return ListSourcesResponse{Success: false, Code: "cloud_unauthorized", Error: "OpenBrain Cloud is not available for this account. Check your organization or cloud access.", Provider: "cloud", Sources: []Source{}}
		}
		return ListSourcesResponse{Success: false, Code: "cloud_unavailable", Error: cleanError(err), Provider: "cloud", Sources: []Source{}}
	}
	index, _ := s.loadWorkspaceIndex(auth, workspaces, true)
	indexByID := map[string]int{}
	indexByRepoKey := map[string][]int{}
	for i, entry := range index.Workspaces {
		if workspaceID := strings.TrimSpace(entry.WorkspaceID); workspaceID != "" {
			indexByID[workspaceID] = i
		}
		for _, key := range workspaceIndexRepoKeys(entry) {
			indexByRepoKey[key] = append(indexByRepoKey[key], i)
		}
	}
	sources := make([]Source, 0, len(workspaces))
	usedIndex := map[int]bool{}
	indexChanged := false
	for _, workspace := range workspaces {
		workspaceID := strings.TrimSpace(workspace.ID)
		if workspaceID == "" || index.isHidden(workspaceID, workspace.OrgID) {
			continue
		}
		indexed := workspaceIndexEntry{}
		indexedPosition := -1
		if indexPosition, ok := indexByID[workspaceID]; ok {
			indexed = index.Workspaces[indexPosition]
			indexedPosition = indexPosition
			usedIndex[indexPosition] = true
		} else if indexPosition, ok := findWorkspaceIndexByRepo(workspace, index.Workspaces, indexByRepoKey, usedIndex); ok {
			indexed = index.Workspaces[indexPosition]
			indexedPosition = indexPosition
			if rekeyWorkspaceIndexEntry(&indexed, workspace) {
				index.Workspaces[indexPosition] = indexed
				indexByID[workspaceID] = indexPosition
				indexChanged = true
			}
			usedIndex[indexPosition] = true
		}
		if strings.TrimSpace(indexed.WorkspaceID) != "" {
			nextIndexed, changed := s.verifyCloudWorkspaceBinding(ctx, workspace, indexed, false)
			indexed = nextIndexed
			if changed {
				if indexedPosition >= 0 && indexedPosition < len(index.Workspaces) {
					index.Workspaces[indexedPosition] = indexed
				}
				indexChanged = true
			}
		}
		source := Source{
			SourceID:            workspaceID,
			WorkspaceID:         workspaceID,
			OrgID:               strings.TrimSpace(workspace.OrgID),
			BrainID:             "personal",
			Name:                cloudWorkspaceName(workspace),
			Path:                strings.TrimSpace(indexed.Path),
			UpdatedAt:           strings.TrimSpace(workspace.UpdatedAt),
			Federated:           true,
			RemoteURL:           stringPtr(firstNonEmpty(workspace.StorageRemoteURL, workspace.RepoURL)),
			Openable:            strings.TrimSpace(indexed.Path) != "",
			LocationKind:        workspaceLocationKind(indexed.LocationKind),
			DisabledQueries:     workspace.DisabledQueries,
			PublicAccess:        workspace.PublicAccess,
			EffectivePermission: cloudWorkspaceEffectivePermission(workspace),
			CanMutateSource:     cloudWorkspaceCanMutate(workspace),
			PublicOwnerUID:      strings.TrimSpace(workspace.PublicOwnerUID),
			BindingMode:         cloudWorkspaceBindingMode(workspace),
		}
		applySourcePermissionDefaults(&source)
		applyBindingState(&source, indexed)
		sources = append(sources, source)
	}
	nextIndexedWorkspaces := make([]workspaceIndexEntry, 0, len(index.Workspaces))
	hiddenAt := time.Now().UTC().Format(time.RFC3339)
	for indexPosition, entry := range index.Workspaces {
		if usedIndex[indexPosition] {
			nextIndexedWorkspaces = append(nextIndexedWorkspaces, index.Workspaces[indexPosition])
			continue
		}
		if index.isHidden(entry.WorkspaceID, entry.OrgID) {
			indexChanged = true
			continue
		}
		resolvedEntry := workspaceIndexEntry{}
		resolvedSource := (*Source)(nil)
		repo, ok := githubRepoRefFromWorkspaceIndexEntry(entry)
		if ok && strings.TrimSpace(entry.BindingMode) == "granted" {
			resolved, err := s.resolveCloudWorkspaceByRepo(ctx, auth, repo)
			if err == nil && cloudWorkspaceResolveFound(resolved) && strings.TrimSpace(resolved.Workspace.ID) == strings.TrimSpace(entry.WorkspaceID) {
				workspace := cloudWorkspaceWithResolveMetadata(resolved.Workspace, resolved)
				nextIndexed, changed := s.verifyCloudWorkspaceBinding(ctx, workspace, entry, false)
				if changed {
					indexChanged = true
				}
				resolvedEntry = nextIndexed
				resolvedSource = sourceFromCloudWorkspace(workspace, nextIndexed)
			}
		}
		if strings.TrimSpace(resolvedEntry.WorkspaceID) != "" {
			nextIndexedWorkspaces = append(nextIndexedWorkspaces, resolvedEntry)
			if resolvedSource != nil {
				sources = append(sources, *resolvedSource)
			}
			continue
		}
		if strings.TrimSpace(entry.WorkspaceID) != "" {
			index.HiddenWorkspaces = appendHidden(index.HiddenWorkspaces, entry.WorkspaceID, entry.OrgID, hiddenAt)
		}
		indexChanged = true
	}
	if indexChanged {
		index.Workspaces = nextIndexedWorkspaces
	}
	if indexChanged {
		_ = s.saveWorkspaceIndex(index)
	}
	sortSourcesByName(sources)
	_ = s.saveCloudSourcesSnapshot(auth, sources)
	return ListSourcesResponse{Success: true, Provider: "cloud", Sources: sources}
}

func (s *Service) ListCachedOpenBrainSources(_ context.Context) ListSourcesResponse {
	auth, err := s.loadAuth()
	if err != nil {
		return ListSourcesResponse{Success: true, Code: "auth_required", Provider: "cloud", AuthRequired: true, Sources: []Source{}}
	}
	if snapshot, err := s.loadCloudSourcesSnapshot(); err == nil && cloudSourcesSnapshotMatchesAuth(snapshot, auth) {
		sources := append([]Source(nil), snapshot.Sources...)
		sortSourcesByName(sources)
		return ListSourcesResponse{Success: true, Provider: "cloud", Sources: sources}
	}
	index, err := s.loadWorkspaceIndex(auth, nil, false)
	if err != nil {
		return ListSourcesResponse{Success: false, Code: "index_error", Error: cleanError(err), Provider: "cloud", Sources: []Source{}}
	}
	sources := make([]Source, 0, len(index.Workspaces))
	for _, entry := range index.Workspaces {
		source := sourceFromIndex(entry)
		if source == nil {
			continue
		}
		sources = append(sources, *source)
	}
	sortSourcesByName(sources)
	return ListSourcesResponse{Success: true, Provider: "cloud", Sources: sources}
}

func (s *Service) QueryOpenBrain(ctx context.Context, req QueryRequest) QueryResponse {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return QueryResponse{Success: false, Code: "invalid_request", Error: "query is required", Provider: "cloud", Results: []QueryResult{}}
	}
	auth, err := s.loadAuth()
	if err != nil {
		return QueryResponse{Success: false, Code: "auth_required", Error: "Sign in required to use OpenBrain Cloud.", Provider: "cloud", AuthRequired: true, Results: []QueryResult{}}
	}
	base := strings.TrimRight(auth.gateway(), "/")
	if base == "" {
		return QueryResponse{Success: false, Code: "cloud_unconfigured", Error: "OpenBrain API URL is not configured.", Provider: "cloud", Results: []QueryResult{}}
	}
	sourceNames := map[string]string{}
	if sources := s.ListOpenBrainSources(ctx); sources.Success {
		for _, source := range sources.Sources {
			sourceNames[firstNonEmpty(source.WorkspaceID, source.SourceID)] = source.Name
		}
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	orgID := firstNonEmpty(req.OrgID, auth.ActiveOrgID, auth.DefaultOrgID)
	endpoint := "/v1/me/brain/search"
	if strings.TrimSpace(req.Scope) == "workspace" {
		if workspaceID == "" {
			return QueryResponse{Success: false, Code: "invalid_request", Error: "workspaceID is required for workspace scope", Provider: "cloud", Results: []QueryResult{}}
		}
		if orgID == "" {
			orgID = "cloud"
		}
		endpoint = "/v1/orgs/" + url.PathEscape(orgID) + "/workspaces/" + url.PathEscape(workspaceID) + "/brain/search"
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	body := map[string]interface{}{
		"query": query,
		"limit": limit,
		"orgID": firstNonEmpty(auth.ActiveOrgID, auth.DefaultOrgID),
	}
	if publicOwnerUID := strings.TrimSpace(req.PublicOwnerUID); publicOwnerUID != "" {
		body["publicOwnerUID"] = publicOwnerUID
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return QueryResponse{Success: false, Code: "cloud_error", Error: err.Error(), Provider: "cloud", Results: []QueryResult{}}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+endpoint, bytes.NewReader(raw))
	if err != nil {
		return QueryResponse{Success: false, Code: "cloud_error", Error: err.Error(), Provider: "cloud", Results: []QueryResult{}}
	}
	setAuthHeaders(httpReq, auth)
	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return QueryResponse{Success: false, Code: "cloud_unavailable", Error: err.Error(), Provider: "cloud", Results: []QueryResult{}}
	}
	defer res.Body.Close()
	var payload struct {
		Results []cloudSearchResult `json:"results"`
		Error   string              `json:"error,omitempty"`
	}
	if err := readCloudJSON(res, &payload); err != nil {
		if isCloudAuthError(err) {
			return QueryResponse{Success: false, Code: "cloud_unauthorized", Error: "OpenBrain Cloud is not available for this account. Check your organization or cloud access.", Provider: "cloud", Results: []QueryResult{}}
		}
		return QueryResponse{Success: false, Code: "cloud_error", Error: err.Error(), Provider: "cloud", Results: []QueryResult{}}
	}
	results := make([]QueryResult, 0, len(payload.Results))
	for _, rawResult := range payload.Results {
		if mapped := mapCloudResult(rawResult, sourceNames); mapped != nil {
			results = append(results, *mapped)
		}
	}
	return QueryResponse{Success: true, Provider: "cloud", Results: results}
}

func (s *Service) ProxyCloudAPI(ctx context.Context, method string, endpoint string, rawBody []byte) (int, []byte) {
	auth, err := s.loadAuth()
	if err != nil {
		return http.StatusUnauthorized, cloudAPIErrorBody("auth_required", "Sign in required to use OpenBrain Cloud.")
	}
	base := strings.TrimRight(auth.gateway(), "/")
	if base == "" {
		return http.StatusServiceUnavailable, cloudAPIErrorBody("cloud_unconfigured", "OpenBrain API URL is not configured.")
	}
	endpoint = strings.TrimSpace(endpoint)
	if !strings.HasPrefix(endpoint, "/v1/") {
		return http.StatusBadRequest, cloudAPIErrorBody("invalid_request", "invalid OpenBrain Cloud endpoint")
	}
	var body io.Reader
	if len(rawBody) > 0 {
		body = bytes.NewReader(rawBody)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, base+endpoint, body)
	if err != nil {
		return http.StatusBadRequest, cloudAPIErrorBody("invalid_request", err.Error())
	}
	setAuthHeaders(httpReq, auth)
	if len(rawBody) > 0 {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return http.StatusServiceUnavailable, cloudAPIErrorBody("cloud_unavailable", err.Error())
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return http.StatusBadGateway, cloudAPIErrorBody("cloud_error", err.Error())
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{"ok":true}`)
	}
	return res.StatusCode, raw
}

func cloudAPIErrorBody(code string, message string) []byte {
	raw, _ := json.Marshal(map[string]interface{}{
		"success": false,
		"code":    strings.TrimSpace(code),
		"error":   strings.TrimSpace(message),
	})
	return raw
}

func (s *Service) CreateOpenBrainSource(ctx context.Context, req createSourceRequest) CreateSourceResponse {
	workspacePath := strings.TrimSpace(firstNonEmpty(req.LocalPath, req.Path))
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(strings.TrimRight(workspacePath, string(filepath.Separator)))
	}
	bindWorkspaceID := req.WorkspaceIDOrSourceID()
	createRequestID := strings.TrimSpace(req.CreateRequestID)
	if createRequestID == "" {
		createRequestID = newSourceCreateRequestID()
	}
	if workspacePath == "" {
		return CreateSourceResponse{Success: false, Code: "invalid_request", Error: "Select a workspace directory first.", Provider: "cloud"}
	}
	if name == "" {
		return CreateSourceResponse{Success: false, Code: "invalid_request", Error: "OpenBrain source name is required.", Provider: "cloud"}
	}
	auth, err := s.loadAuth()
	if err != nil {
		return CreateSourceResponse{Success: false, Code: "auth_required", Error: "Sign in required to create an OpenBrain Cloud workspace.", Provider: "cloud", AuthRequired: true}
	}
	if err := assertExistingDirectory(workspacePath); err != nil {
		return CreateSourceResponse{Success: false, Code: "invalid_request", Error: err.Error(), Provider: "cloud"}
	}
	existing, _ := s.findIndexedWorkspaceByPath(auth, workspacePath)
	if bindWorkspaceID == "" && existing.WorkspaceID != "" && strings.TrimSpace(existing.TemplateID) == cloudWorkspaceTemplateID {
		return CreateSourceResponse{Success: true, Provider: "cloud", Workspace: sourceFromIndex(existing)}
	}
	if err := ensureGitAvailable(); err != nil {
		return CreateSourceResponse{Success: false, Code: "git_error", Error: err.Error(), Provider: "cloud", RequestID: createRequestID}
	}
	repo, err := inspectGitHubRepositoryForSourceCreate(ctx, workspacePath)
	if err != nil {
		return CreateSourceResponse{Success: false, Code: "invalid_repository", Error: err.Error(), Provider: "cloud"}
	}
	created := createWorkspaceResult{}
	createdNewCloudSource := false
	if bindWorkspaceID != "" {
		if repo == nil {
			return CreateSourceResponse{Success: false, Code: "invalid_repository", Error: "Selected directory must be a GitHub repository to bind this OpenBrain source.", Provider: "cloud"}
		}
		resolved, err := s.resolveCloudWorkspaceByRepo(ctx, auth, *repo)
		if err != nil {
			return CreateSourceResponse{Success: false, Code: errorCodeFromCloud(err), Error: err.Error(), Provider: "cloud"}
		}
		if !cloudWorkspaceResolveFound(resolved) || strings.TrimSpace(resolved.Workspace.ID) != bindWorkspaceID {
			return CreateSourceResponse{Success: false, Code: "workspace_repo_mismatch", Error: "Selected folder points to a different repository.", Provider: "cloud"}
		}
		if strings.TrimSpace(req.OrgID) != "" && strings.TrimSpace(resolved.Workspace.OrgID) != "" && strings.TrimSpace(req.OrgID) != strings.TrimSpace(resolved.Workspace.OrgID) {
			return CreateSourceResponse{Success: false, Code: "workspace_repo_mismatch", Error: "Selected folder points to a different repository.", Provider: "cloud"}
		}
		created = createWorkspaceResultFromResolvedWorkspace(resolved.Workspace, repo, resolved)
	} else {
		var err error
		created, createdNewCloudSource, err = s.resolveOrCreateCloudWorkspace(ctx, auth, name, repo, createRequestID)
		if err != nil {
			if isCloudAuthError(err) {
				return CreateSourceResponse{Success: false, Code: "cloud_unauthorized", Error: "OpenBrain Cloud is not available for this account. Check your organization or cloud access.", Provider: "cloud"}
			}
			if errors.Is(err, errWorkspaceRepoAmbiguous) {
				return CreateSourceResponse{Success: false, Code: "workspace_repo_ambiguous", Error: strings.TrimPrefix(err.Error(), errWorkspaceRepoAmbiguous.Error()+": "), Provider: "cloud"}
			}
			return CreateSourceResponse{Success: false, Code: "openbrain_error", Error: err.Error(), Provider: "cloud"}
		}
	}
	var gitRollback *gitImportRollback
	deleteCloudRepositoryOnRollback := createdNewCloudSource && repo == nil
	if createdNewCloudSource && repo == nil {
		gitToken, err := s.fetchWorkspaceGitToken(ctx, auth, created)
		if err != nil {
			return s.createSourceFailureAfterCloudCreate(ctx, auth, created, workspacePath, name, createRequestID, deleteCloudRepositoryOnRollback, nil, errorCodeFromCloud(err), err)
		}
		remoteURL := created.remoteURL()
		if gitToken != nil {
			remoteURL = firstNonEmpty(gitToken.RemoteURL, remoteURL)
		}
		remoteURL = stripRemoteUserInfo(remoteURL)
		gitRollback = captureGitImportRollback(ctx, workspacePath)
		if err := importGitWorkspace(ctx, workspacePath, remoteURL, created.defaultBranch(), gitToken, repo != nil); err != nil {
			return s.createSourceFailureAfterCloudCreate(ctx, auth, created, workspacePath, name, createRequestID, deleteCloudRepositoryOnRollback, gitRollback, "git_error", err)
		}
	}
	if createdNewCloudSource {
		if err := s.triggerBrainSync(ctx, auth, created); err != nil {
			return s.createSourceFailureAfterCloudCreate(ctx, auth, created, workspacePath, name, createRequestID, deleteCloudRepositoryOnRollback, gitRollback, errorCodeFromCloud(err), err)
		}
	}
	entry, err := s.upsertCloudWorkspaceIndex(auth, created, workspacePath, name, req.Takeover)
	if err != nil {
		if errors.Is(err, errWorkspacePathConflict) {
			if createdNewCloudSource {
				return s.createSourceFailureAfterCloudCreate(ctx, auth, created, workspacePath, name, createRequestID, deleteCloudRepositoryOnRollback, gitRollback, "workspace_path_conflict", errors.New(strings.TrimPrefix(err.Error(), errWorkspacePathConflict.Error()+": ")))
			}
			return CreateSourceResponse{Success: false, Code: "workspace_path_conflict", Error: strings.TrimPrefix(err.Error(), errWorkspacePathConflict.Error()+": "), Provider: "cloud", RequestID: createRequestID}
		}
		var ownerErr *workspacePathOwnerError
		if errors.As(err, &ownerErr) {
			if createdNewCloudSource {
				return s.createSourceFailureAfterCloudCreate(ctx, auth, created, workspacePath, name, createRequestID, deleteCloudRepositoryOnRollback, gitRollback, "path_owned_by_other_account", err)
			}
			return CreateSourceResponse{
				Success:      false,
				Code:         "path_owned_by_other_account",
				Error:        strings.TrimPrefix(err.Error(), errWorkspacePathOwnedByOtherAccount.Error()+": "),
				Provider:     "cloud",
				PathOwnerUID: ownerErr.ownerUID,
				RequestID:    createRequestID,
			}
		}
		if createdNewCloudSource {
			return s.createSourceFailureAfterCloudCreate(ctx, auth, created, workspacePath, name, createRequestID, deleteCloudRepositoryOnRollback, gitRollback, "index_error", err)
		}
		return CreateSourceResponse{Success: false, Code: "index_error", Error: err.Error(), Provider: "cloud", RequestID: createRequestID}
	}
	return CreateSourceResponse{Success: true, Provider: "cloud", Workspace: sourceFromIndex(entry)}
}

func (s *Service) VerifyOpenBrainSource(ctx context.Context, req mutationRequest) VerifySourceResponse {
	auth, err := s.loadAuth()
	if err != nil {
		return VerifySourceResponse{Success: false, Code: "auth_required", Error: "Sign in required to verify an OpenBrain Cloud workspace.", Provider: "cloud", AuthRequired: true}
	}
	workspaceID := req.WorkspaceIDOrSourceID()
	if workspaceID == "" {
		return VerifySourceResponse{Success: false, Code: "invalid_request", Error: "OpenBrain Cloud workspace identity is missing.", Provider: "cloud"}
	}
	index, err := s.loadWorkspaceIndex(auth, nil, false)
	if err != nil {
		return VerifySourceResponse{Success: false, Code: "index_error", Error: err.Error(), Provider: "cloud"}
	}
	entry, position := findWorkspaceIndexEntry(index.Workspaces, workspaceID, req.OrgID)
	workspace, err := s.findCloudWorkspace(ctx, auth, req.OrgID, workspaceID)
	if err != nil {
		if position < 0 || strings.TrimSpace(entry.BindingMode) != "granted" {
			return VerifySourceResponse{Success: false, Code: errorCodeFromCloud(err), Error: err.Error(), Provider: "cloud"}
		}
		repo, ok := githubRepoRefFromWorkspaceIndexEntry(entry)
		if !ok {
			return VerifySourceResponse{Success: false, Code: "workspace_unbound", Error: "Bind a local folder for this repository.", Provider: "cloud"}
		}
		resolved, resolveErr := s.resolveCloudWorkspaceByRepo(ctx, auth, repo)
		if resolveErr != nil {
			return VerifySourceResponse{Success: false, Code: errorCodeFromCloud(resolveErr), Error: resolveErr.Error(), Provider: "cloud"}
		}
		if !cloudWorkspaceResolveFound(resolved) || strings.TrimSpace(resolved.Workspace.ID) != workspaceID {
			return VerifySourceResponse{Success: false, Code: "workspace_unbound", Error: "Bind a local folder for this repository.", Provider: "cloud"}
		}
		workspace = cloudWorkspaceWithResolveMetadata(resolved.Workspace, resolved)
	}
	if position < 0 || strings.TrimSpace(entry.Path) == "" {
		source := sourceFromCloudWorkspace(workspace, workspaceIndexEntry{})
		return VerifySourceResponse{Success: false, Code: "workspace_unbound", Error: "Bind a local folder for this repository.", Provider: "cloud", Workspace: source}
	}
	entry, changed := s.verifyCloudWorkspaceBinding(ctx, workspace, entry, true)
	if changed {
		index.Workspaces[position] = entry
		if err := s.saveWorkspaceIndex(index); err != nil {
			return VerifySourceResponse{Success: false, Code: "index_error", Error: err.Error(), Provider: "cloud"}
		}
	}
	source := sourceFromCloudWorkspace(workspace, entry)
	if source.BindingStatus != sourceBindingConnected {
		code := "workspace_unbound"
		if source.BindingReason == sourceBindingReasonMoved {
			code = "workspace_path_missing"
		} else if source.BindingReason == sourceBindingReasonMismatch {
			code = "workspace_repo_mismatch"
		}
		return VerifySourceResponse{Success: false, Code: code, Error: bindingReasonMessage(source.BindingReason), Provider: "cloud", Workspace: source}
	}
	s.upsertCloudSourceSnapshot(auth, source)
	return VerifySourceResponse{Success: true, Provider: "cloud", Workspace: source}
}

func (s *Service) ListOpenBrainSourceRecoveryCandidates(ctx context.Context, req recoveryCandidatesRequest) RecoveryCandidatesResponse {
	auth, err := s.loadAuth()
	if err != nil {
		return RecoveryCandidatesResponse{Success: false, Code: "auth_required", Error: "Sign in required to recover an OpenBrain Cloud workspace binding.", Provider: "cloud", AuthRequired: true, Candidates: []RecoveryCandidate{}}
	}
	workspaceID := req.WorkspaceIDOrSourceID()
	if workspaceID == "" {
		return RecoveryCandidatesResponse{Success: false, Code: "invalid_request", Error: "OpenBrain Cloud workspace identity is missing.", Provider: "cloud", Candidates: []RecoveryCandidate{}}
	}
	paths := dedupeCandidatePaths(req.Paths)
	if len(paths) == 0 {
		return RecoveryCandidatesResponse{Success: true, Provider: "cloud", Candidates: []RecoveryCandidate{}}
	}
	index, _ := s.loadWorkspaceIndex(auth, nil, false)
	entry, position := findWorkspaceIndexEntry(index.Workspaces, workspaceID, req.OrgID)
	workspace, err := s.findCloudWorkspace(ctx, auth, req.OrgID, workspaceID)
	if err != nil {
		if position < 0 || strings.TrimSpace(entry.BindingMode) != "granted" {
			return RecoveryCandidatesResponse{Success: false, Code: errorCodeFromCloud(err), Error: err.Error(), Provider: "cloud", Candidates: []RecoveryCandidate{}}
		}
		repo, ok := githubRepoRefFromWorkspaceIndexEntry(entry)
		if !ok {
			return RecoveryCandidatesResponse{Success: true, Provider: "cloud", Candidates: []RecoveryCandidate{}}
		}
		resolved, resolveErr := s.resolveCloudWorkspaceByRepo(ctx, auth, repo)
		if resolveErr != nil {
			return RecoveryCandidatesResponse{Success: false, Code: errorCodeFromCloud(resolveErr), Error: resolveErr.Error(), Provider: "cloud", Candidates: []RecoveryCandidate{}}
		}
		if !cloudWorkspaceResolveFound(resolved) || strings.TrimSpace(resolved.Workspace.ID) != workspaceID {
			return RecoveryCandidatesResponse{Success: true, Provider: "cloud", Candidates: []RecoveryCandidate{}}
		}
		workspace = cloudWorkspaceWithResolveMetadata(resolved.Workspace, resolved)
	}
	candidates := make([]RecoveryCandidate, 0, len(paths))
	for _, candidatePath := range paths {
		repo, err := inspectGitHubRepository(ctx, candidatePath)
		if err != nil || repo == nil {
			continue
		}
		if !cloudWorkspaceBindingRepositoryMatches(workspace, entry, *repo) {
			continue
		}
		candidates = append(candidates, RecoveryCandidate{
			Path: candidatePath,
			Name: filepath.Base(strings.TrimRight(candidatePath, string(filepath.Separator))),
		})
	}
	return RecoveryCandidatesResponse{Success: true, Provider: "cloud", Candidates: candidates}
}

func (s *Service) RemoveOpenBrainSourceFromDevice(_ context.Context, req mutationRequest) MutationResponse {
	auth, err := s.loadAuth()
	if err != nil {
		return MutationResponse{Success: false, Code: "auth_required", Error: "Sign in required to remove an OpenBrain Cloud workspace from this device.", Provider: "cloud", AuthRequired: true}
	}
	if err := s.removeWorkspaceFromDevice(auth, req.WorkspaceIDOrSourceID(), req.OrgID, req.Path, true); err != nil {
		return MutationResponse{Success: false, Code: "index_error", Error: err.Error(), Provider: "cloud"}
	}
	s.removeCloudSourceFromSnapshot(auth, req.OrgID, req.WorkspaceIDOrSourceID())
	return MutationResponse{Success: true, Provider: "cloud"}
}

func (s *Service) ArchiveOpenBrainSource(ctx context.Context, req mutationRequest) MutationResponse {
	auth, err := s.loadAuth()
	if err != nil {
		return MutationResponse{Success: false, Code: "auth_required", Error: "Sign in required to archive an OpenBrain Cloud workspace.", Provider: "cloud", AuthRequired: true}
	}
	workspaceID := req.WorkspaceIDOrSourceID()
	if err := s.archiveWorkspaceBrain(ctx, auth, req.OrgID, workspaceID); err != nil {
		return MutationResponse{Success: false, Code: errorCodeFromCloud(err), Error: err.Error(), Provider: "cloud"}
	}
	if err := s.removeWorkspaceFromDevice(auth, workspaceID, req.OrgID, req.Path, true); err != nil {
		return MutationResponse{Success: false, Code: "index_error", Error: err.Error(), Provider: "cloud"}
	}
	s.removeCloudSourceFromSnapshot(auth, req.OrgID, workspaceID)
	return MutationResponse{Success: true, Provider: "cloud"}
}

func (s *Service) ApplyOpenBrainSourceAction(ctx context.Context, req mutationRequest) MutationResponse {
	auth, err := s.loadAuth()
	if err != nil {
		return MutationResponse{Success: false, Code: "auth_required", Error: "Sign in required to update an OpenBrain Cloud workspace.", Provider: "cloud", AuthRequired: true}
	}
	workspaceID := req.WorkspaceIDOrSourceID()
	result, err := s.applyWorkspaceBrainSourceAction(ctx, auth, req.OrgID, workspaceID, req)
	if err != nil {
		return MutationResponse{Success: false, Code: errorCodeFromCloud(err), Error: err.Error(), Provider: "cloud"}
	}
	if req.HardDelete || result.HardDeleted {
		if err := s.removeWorkspaceFromDevice(auth, workspaceID, req.OrgID, req.Path, true); err != nil {
			return MutationResponse{Success: false, Code: "index_error", Error: err.Error(), Provider: "cloud"}
		}
		s.removeCloudSourceFromSnapshot(auth, firstNonEmpty(result.OrgID, req.OrgID), firstNonEmpty(result.WorkspaceID, result.SourceID, workspaceID))
	} else if req.DisableQueries || req.EnableQueries || result.DisabledQueries || result.EnabledQueries {
		disabled := req.DisableQueries || result.DisabledQueries
		s.updateCloudSourceSnapshotQueriesDisabled(auth, firstNonEmpty(result.OrgID, req.OrgID), firstNonEmpty(result.WorkspaceID, result.SourceID, workspaceID), disabled)
	}
	return MutationResponse{
		Success:         true,
		Provider:        "cloud",
		SourceID:        result.SourceID,
		WorkspaceID:     result.WorkspaceID,
		OrgID:           result.OrgID,
		DisabledQueries: result.DisabledQueries,
		EnabledQueries:  result.EnabledQueries,
		DisabledSync:    result.DisabledSync,
		HardDeleted:     result.HardDeleted,
		SyncJobsRemoved: result.SyncJobsRemoved,
		Status:          result.Status,
	}
}

func (s *Service) loadAuth() (authConfig, error) {
	var auth authConfig
	raw, err := os.ReadFile(filepath.Join(s.baseDir, "configs", "user", "auth.json"))
	if err != nil {
		return auth, err
	}
	if err := json.Unmarshal(raw, &auth); err != nil {
		return auth, err
	}
	if strings.TrimSpace(auth.Token) == "" || strings.TrimSpace(auth.UID) == "" {
		return auth, errors.New("OpenBrain login is required")
	}
	return auth, nil
}

func (a authConfig) gateway() string {
	return firstNonEmpty(os.Getenv("OPENBRAIN_API_URL"), a.Gateway)
}

func (s *Service) listCloudWorkspaces(ctx context.Context, auth authConfig) ([]cloudWorkspace, error) {
	base := strings.TrimRight(auth.gateway(), "/")
	if base == "" {
		return nil, errors.New("OpenBrain API URL is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/me/brain/workspaces", nil)
	if err != nil {
		return nil, err
	}
	setAuthHeaders(req, auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var payload struct {
		Workspaces []cloudWorkspace `json:"workspaces"`
		Error      string           `json:"error,omitempty"`
	}
	if err := readCloudJSON(res, &payload); err != nil {
		return nil, err
	}
	return payload.Workspaces, nil
}

func (s *Service) resolveCloudWorkspaceByRepo(ctx context.Context, auth authConfig, repo githubRepoRef) (cloudWorkspaceResolveResult, error) {
	base := strings.TrimRight(auth.gateway(), "/")
	if base == "" {
		return cloudWorkspaceResolveResult{}, errors.New("OpenBrain API URL is not configured")
	}
	query := url.Values{}
	query.Set("provider", "github")
	query.Set("host", "github.com")
	if strings.TrimSpace(repo.ExternalID) != "" {
		query.Set("externalID", strings.TrimSpace(repo.ExternalID))
	}
	if strings.TrimSpace(repo.Owner) != "" {
		query.Set("owner", strings.TrimSpace(repo.Owner))
	}
	if strings.TrimSpace(repo.Name) != "" {
		query.Set("name", strings.TrimSpace(repo.Name))
	}
	if strings.TrimSpace(repo.RemoteURL) != "" {
		query.Set("remoteURL", strings.TrimSpace(repo.RemoteURL))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/me/brain/workspaces/resolve-by-repo?"+query.Encode(), nil)
	if err != nil {
		return cloudWorkspaceResolveResult{}, err
	}
	setAuthHeaders(req, auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return cloudWorkspaceResolveResult{}, err
	}
	defer res.Body.Close()
	var resolved cloudWorkspaceResolveResult
	if err := readCloudJSON(res, &resolved); err != nil {
		return cloudWorkspaceResolveResult{}, err
	}
	return resolved, nil
}

func cloudWorkspaceResolveFound(resolved cloudWorkspaceResolveResult) bool {
	resolution := strings.TrimSpace(resolved.Resolution)
	return (resolution == "own" || resolution == "granted") && strings.TrimSpace(resolved.Workspace.ID) != ""
}

func cloudWorkspaceWithResolveMetadata(workspace cloudWorkspace, resolved cloudWorkspaceResolveResult) cloudWorkspace {
	workspace.EffectivePermission = firstNonEmpty(resolved.EffectivePermission, workspace.EffectivePermission)
	workspace.CanMutateSource = resolved.CanMutateSource
	workspace.PublicOwnerUID = firstNonEmpty(resolved.PublicOwnerUID, workspace.PublicOwnerUID)
	return workspace
}

func (s *Service) findCloudWorkspace(ctx context.Context, auth authConfig, orgID string, workspaceID string) (cloudWorkspace, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return cloudWorkspace{}, errors.New("OpenBrain Cloud workspace identity is missing")
	}
	workspaces, err := s.listCloudWorkspaces(ctx, auth)
	if err != nil {
		return cloudWorkspace{}, err
	}
	for _, workspace := range workspaces {
		if strings.TrimSpace(workspace.ID) != workspaceID {
			continue
		}
		if strings.TrimSpace(orgID) != "" && strings.TrimSpace(workspace.OrgID) != "" && strings.TrimSpace(workspace.OrgID) != strings.TrimSpace(orgID) {
			continue
		}
		return workspace, nil
	}
	return cloudWorkspace{}, fmt.Errorf("OpenBrain Cloud workspace not found: %s", workspaceID)
}

func (s *Service) resolveOrCreateCloudWorkspace(ctx context.Context, auth authConfig, name string, repo *githubRepoRef, createRequestID string) (createWorkspaceResult, bool, error) {
	if repo != nil {
		resolved, err := s.resolveCloudWorkspaceByRepo(ctx, auth, *repo)
		if err != nil {
			return createWorkspaceResult{}, false, err
		}
		if cloudWorkspaceResolveFound(resolved) {
			return createWorkspaceResultFromResolvedWorkspace(resolved.Workspace, repo, resolved), false, nil
		}
		if strings.TrimSpace(resolved.Resolution) == "ambiguous" {
			return createWorkspaceResult{}, false, fmt.Errorf("%w: multiple readable OpenBrain Cloud workspaces match this repository", errWorkspaceRepoAmbiguous)
		}
	}
	owner := ""
	repositoryName := ""
	if repo != nil {
		owner = repo.Owner
		repositoryName = repo.Name
	}
	if owner == "" {
		owner = s.defaultGitHubRepositoryOwner(ctx, auth)
	}
	if owner == "" {
		return createWorkspaceResult{}, false, errors.New("Connect GitHub before creating an OpenBrain Cloud workspace")
	}
	base := strings.TrimRight(auth.gateway(), "/")
	reqBody, _ := json.Marshal(map[string]interface{}{
		"templateID":      cloudWorkspaceTemplateID,
		"provider":        "github",
		"storageProvider": "github",
		"repositoryOwner": owner,
		"repositoryName":  repositoryName,
		"name":            name,
		"createRequestID": strings.TrimSpace(createRequestID),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/workspaces", bytes.NewReader(reqBody))
	if err != nil {
		return createWorkspaceResult{}, false, err
	}
	setAuthHeaders(req, auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return createWorkspaceResult{}, false, err
	}
	defer res.Body.Close()
	var created createWorkspaceResult
	if err := readCloudJSON(res, &created); err != nil {
		return createWorkspaceResult{}, false, err
	}
	created.BindingMode = "own"
	created.EffectivePermission = "admin"
	created.CanMutateSource = true
	return created, true, nil
}

func (s *Service) defaultGitHubRepositoryOwner(ctx context.Context, auth authConfig) string {
	templates, err := s.listWorkspaceTemplates(ctx, auth)
	if err == nil {
		providers := []workspaceProviderOption{}
		for _, template := range templates.Templates {
			if strings.TrimSpace(template.TemplateID) != cloudWorkspaceTemplateID {
				continue
			}
			if template.Storage != nil {
				providers = append(providers, template.Storage.Providers...)
			}
			if template.Repository != nil {
				providers = append(providers, template.Repository.Providers...)
			}
			break
		}
		for _, provider := range providers {
			if owner := firstUsableGitHubOwner(provider); owner != "" {
				return owner
			}
		}
	}
	connections, err := s.listStorageConnections(ctx, auth)
	if err != nil {
		return ""
	}
	for _, provider := range connections.Providers {
		if owner := firstUsableGitHubOwner(provider); owner != "" {
			return owner
		}
	}
	return ""
}

func (s *Service) listWorkspaceTemplates(ctx context.Context, auth authConfig) (workspaceTemplateListResult, error) {
	base := strings.TrimRight(auth.gateway(), "/")
	if base == "" {
		return workspaceTemplateListResult{}, errors.New("OpenBrain API URL is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/workspace-templates", nil)
	if err != nil {
		return workspaceTemplateListResult{}, err
	}
	setAuthHeaders(req, auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return workspaceTemplateListResult{}, err
	}
	defer res.Body.Close()
	var payload workspaceTemplateListResult
	if err := readCloudJSON(res, &payload); err != nil {
		return workspaceTemplateListResult{}, err
	}
	return payload, nil
}

func (s *Service) listStorageConnections(ctx context.Context, auth authConfig) (workspaceStorageConnectionsResult, error) {
	base := strings.TrimRight(auth.gateway(), "/")
	if base == "" {
		return workspaceStorageConnectionsResult{}, errors.New("OpenBrain API URL is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/storage-connections", nil)
	if err != nil {
		return workspaceStorageConnectionsResult{}, err
	}
	setAuthHeaders(req, auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return workspaceStorageConnectionsResult{}, err
	}
	defer res.Body.Close()
	var payload workspaceStorageConnectionsResult
	if err := readCloudJSON(res, &payload); err != nil {
		return workspaceStorageConnectionsResult{}, err
	}
	return payload, nil
}

func (s *Service) fetchWorkspaceGitToken(ctx context.Context, auth authConfig, workspace createWorkspaceResult) (*workspaceGitToken, error) {
	base := strings.TrimRight(auth.gateway(), "/")
	if base == "" || workspace.OrgID == "" || workspace.WorkspaceID == "" {
		return nil, nil
	}
	endpoint := base + "/v1/orgs/" + url.PathEscape(workspace.OrgID) + "/workspaces/" + url.PathEscape(workspace.WorkspaceID) + "/git-token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}
	setAuthHeaders(req, auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var token workspaceGitToken
	if err := readCloudJSON(res, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *Service) triggerBrainSync(ctx context.Context, auth authConfig, workspace createWorkspaceResult) error {
	base := strings.TrimRight(auth.gateway(), "/")
	if base == "" || workspace.OrgID == "" || workspace.WorkspaceID == "" {
		return nil
	}
	endpoint := base + "/v1/orgs/" + url.PathEscape(workspace.OrgID) + "/workspaces/" + url.PathEscape(workspace.WorkspaceID) + "/brain/sync"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	setAuthHeaders(req, auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var payload map[string]interface{}
	return readCloudJSON(res, &payload)
}

func (s *Service) createSourceFailureAfterCloudCreate(ctx context.Context, auth authConfig, workspace createWorkspaceResult, workspacePath string, name string, requestID string, deleteRepository bool, gitRollback *gitImportRollback, code string, cause error) CreateSourceResponse {
	cleanupAttempted := false
	cleanupErrors := []string{}
	if gitRollback != nil {
		cleanupAttempted = true
		if err := gitRollback.rollback(ctx); err != nil {
			cleanupErrors = append(cleanupErrors, "local git rollback: "+err.Error())
		}
	}
	if strings.TrimSpace(workspace.WorkspaceID) != "" {
		cleanupAttempted = true
		if err := s.removeWorkspaceFromDevice(auth, workspace.WorkspaceID, workspace.OrgID, "", false); err != nil {
			cleanupErrors = append(cleanupErrors, "local binding rollback: "+err.Error())
		}
		s.removeCloudSourceFromSnapshot(auth, workspace.OrgID, workspace.WorkspaceID)
		if err := s.rollbackCloudWorkspaceCreate(ctx, auth, workspace, name, requestID, deleteRepository); err != nil {
			cleanupErrors = append(cleanupErrors, "cloud rollback: "+err.Error())
		}
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	if message == "" {
		message = "OpenBrain source create failed."
	}
	if len(cleanupErrors) > 0 {
		return CreateSourceResponse{
			Success:          false,
			Code:             "source_create_cleanup_failed",
			Error:            message,
			Provider:         "cloud",
			RequestID:        requestID,
			CleanupAttempted: cleanupAttempted,
			CleanupSucceeded: false,
			CleanupError:     strings.Join(cleanupErrors, "; "),
		}
	}
	return CreateSourceResponse{
		Success:          false,
		Code:             firstNonEmpty(code, "openbrain_error"),
		Error:            message,
		Provider:         "cloud",
		RequestID:        requestID,
		CleanupAttempted: cleanupAttempted,
		CleanupSucceeded: cleanupAttempted,
	}
}

func (s *Service) rollbackCloudWorkspaceCreate(ctx context.Context, auth authConfig, workspace createWorkspaceResult, name string, requestID string, deleteRepository bool) error {
	base := strings.TrimRight(auth.gateway(), "/")
	orgID := strings.TrimSpace(workspace.OrgID)
	workspaceID := strings.TrimSpace(workspace.WorkspaceID)
	if base == "" || orgID == "" || workspaceID == "" {
		return errors.New("OpenBrain Cloud workspace identity is missing")
	}
	body, err := json.Marshal(map[string]interface{}{
		"createRequestID":    strings.TrimSpace(requestID),
		"deleteRepository":   deleteRepository,
		"confirmWorkspaceID": workspaceID,
		"confirmName":        firstNonEmpty(name, workspace.DefaultLocalName, workspaceID),
	})
	if err != nil {
		return err
	}
	endpoint := base + "/v1/orgs/" + url.PathEscape(orgID) + "/workspaces/" + url.PathEscape(workspaceID) + "/rollback-create"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	setAuthHeaders(req, auth)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var result sourceCreateRollbackResult
	return readCloudJSON(res, &result)
}

func (s *Service) archiveWorkspaceBrain(ctx context.Context, auth authConfig, orgID string, workspaceID string) error {
	base := strings.TrimRight(auth.gateway(), "/")
	orgID = firstNonEmpty(orgID, auth.ActiveOrgID, auth.DefaultOrgID)
	workspaceID = strings.TrimSpace(workspaceID)
	if base == "" || orgID == "" || workspaceID == "" {
		return errors.New("OpenBrain Cloud workspace identity is missing")
	}
	endpoint := base + "/v1/orgs/" + url.PathEscape(orgID) + "/workspaces/" + url.PathEscape(workspaceID) + "/brain/archive"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	setAuthHeaders(req, auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var payload map[string]interface{}
	return readCloudJSON(res, &payload)
}

func (s *Service) applyWorkspaceBrainSourceAction(ctx context.Context, auth authConfig, orgID string, workspaceID string, action mutationRequest) (sourceActionResult, error) {
	var result sourceActionResult
	base := strings.TrimRight(auth.gateway(), "/")
	orgID = firstNonEmpty(orgID, auth.ActiveOrgID, auth.DefaultOrgID)
	workspaceID = strings.TrimSpace(workspaceID)
	if base == "" || orgID == "" || workspaceID == "" {
		return result, errors.New("OpenBrain Cloud workspace identity is missing")
	}
	body, err := json.Marshal(map[string]interface{}{
		"disableQueries":     action.DisableQueries,
		"enableQueries":      action.EnableQueries,
		"disableSync":        action.DisableSync,
		"hardDelete":         action.HardDelete,
		"confirmWorkspaceID": action.ConfirmWorkspaceID,
		"confirmName":        action.ConfirmName,
	})
	if err != nil {
		return result, err
	}
	endpoint := base + "/v1/orgs/" + url.PathEscape(orgID) + "/workspaces/" + url.PathEscape(workspaceID) + "/brain/source-action"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	setAuthHeaders(req, auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, err
	}
	defer res.Body.Close()
	if err := readCloudJSON(res, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) loadWorkspaceIndex(auth authConfig, visible []cloudWorkspace, visibleKnown bool) (workspaceIndexFile, error) {
	uid := strings.TrimSpace(auth.UID)
	if uid == "" {
		return workspaceIndexFile{Version: 2, Accounts: map[string]*workspaceIndexAccount{}}, nil
	}
	var index workspaceIndexFile
	target := s.workspaceIndexPath()
	raw, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			index = workspaceIndexFile{Version: 2, Accounts: map[string]*workspaceIndexAccount{}, ActiveUID: uid}
			index.activate(uid)
			return index, nil
		}
		return index, err
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return index, err
	}
	if index.Version == 2 && index.Accounts != nil {
		index.ActiveUID = uid
		index.activate(uid)
		return index, nil
	}
	if !visibleKnown {
		index = workspaceIndexFile{Version: 2, Accounts: map[string]*workspaceIndexAccount{}, ActiveUID: uid}
		index.activate(uid)
		return index, nil
	}
	migrated := migrateLegacyWorkspaceIndex(uid, index, visible)
	if err := backupWorkspaceIndexFile(target, raw); err != nil {
		return index, err
	}
	if err := s.saveWorkspaceIndex(migrated); err != nil {
		return index, err
	}
	index = migrated
	return index, nil
}

func (s *Service) saveWorkspaceIndex(index workspaceIndexFile) error {
	index.Version = 2
	if index.Accounts == nil {
		index.Accounts = map[string]*workspaceIndexAccount{}
	}
	if uid := strings.TrimSpace(index.ActiveUID); uid != "" {
		account := index.ensureAccount(uid)
		account.Workspaces = sortedWorkspaceEntries(index.Workspaces)
		account.HiddenWorkspaces = sortedHiddenWorkspaceEntries(index.HiddenWorkspaces)
	}
	for _, account := range index.Accounts {
		if account == nil {
			continue
		}
		account.Workspaces = sortedWorkspaceEntries(account.Workspaces)
		account.HiddenWorkspaces = sortedHiddenWorkspaceEntries(account.HiddenWorkspaces)
	}
	index.Workspaces = nil
	index.HiddenWorkspaces = nil
	raw, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	target := s.workspaceIndexPath()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

func migrateLegacyWorkspaceIndex(uid string, legacy workspaceIndexFile, visible []cloudWorkspace) workspaceIndexFile {
	visibleKeys := map[string]struct{}{}
	for _, workspace := range visible {
		key := workspaceIdentityStorageKey(strings.TrimSpace(workspace.ID), strings.TrimSpace(workspace.OrgID))
		if key != "" {
			visibleKeys[key] = struct{}{}
		}
	}
	account := &workspaceIndexAccount{}
	seen := map[string]struct{}{}
	for _, entry := range legacy.Workspaces {
		key := workspaceIdentityStorageKey(strings.TrimSpace(entry.WorkspaceID), strings.TrimSpace(entry.OrgID))
		if key == "" {
			continue
		}
		if _, ok := visibleKeys[key]; !ok && !legacyWorkspaceVisibleByRepository(entry, visible) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		account.Workspaces = append(account.Workspaces, entry)
	}
	for _, entry := range legacy.HiddenWorkspaces {
		key := workspaceIdentityStorageKey(strings.TrimSpace(entry.WorkspaceID), strings.TrimSpace(entry.OrgID))
		if key == "" {
			continue
		}
		if _, ok := visibleKeys[key]; ok {
			account.HiddenWorkspaces = appendHidden(account.HiddenWorkspaces, entry.WorkspaceID, entry.OrgID, firstNonEmpty(entry.HiddenAt, time.Now().UTC().Format(time.RFC3339)))
		}
	}
	index := workspaceIndexFile{
		Version:   2,
		ActiveUID: uid,
		Accounts: map[string]*workspaceIndexAccount{
			uid: account,
		},
	}
	index.activate(uid)
	return index
}

func legacyWorkspaceVisibleByRepository(entry workspaceIndexEntry, visible []cloudWorkspace) bool {
	for _, workspace := range visible {
		if workspaceIndexRepositoryMatchesCloudWorkspace(entry, workspace) {
			return true
		}
	}
	return false
}

func backupWorkspaceIndexFile(target string, raw []byte) error {
	backup := target + ".bak-" + time.Now().UTC().Format("20060102T150405Z")
	return os.WriteFile(backup, raw, 0o600)
}

func (index *workspaceIndexFile) activate(uid string) {
	uid = strings.TrimSpace(uid)
	index.ActiveUID = uid
	if index.Accounts == nil {
		index.Accounts = map[string]*workspaceIndexAccount{}
	}
	if uid == "" {
		index.Workspaces = nil
		index.HiddenWorkspaces = nil
		return
	}
	account := index.ensureAccount(uid)
	index.Workspaces = append([]workspaceIndexEntry(nil), account.Workspaces...)
	index.HiddenWorkspaces = append([]hiddenWorkspaceEntry(nil), account.HiddenWorkspaces...)
}

func (index *workspaceIndexFile) ensureAccount(uid string) *workspaceIndexAccount {
	uid = strings.TrimSpace(uid)
	if index.Accounts == nil {
		index.Accounts = map[string]*workspaceIndexAccount{}
	}
	account := index.Accounts[uid]
	if account == nil {
		account = &workspaceIndexAccount{}
		index.Accounts[uid] = account
	}
	return account
}

func sortedWorkspaceEntries(entries []workspaceIndexEntry) []workspaceIndexEntry {
	next := append([]workspaceIndexEntry(nil), entries...)
	sort.SliceStable(next, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(next[i].LocalName))
		right := strings.ToLower(strings.TrimSpace(next[j].LocalName))
		if left == right {
			return strings.TrimSpace(next[i].WorkspaceID) < strings.TrimSpace(next[j].WorkspaceID)
		}
		return left < right
	})
	return next
}

func sortedHiddenWorkspaceEntries(entries []hiddenWorkspaceEntry) []hiddenWorkspaceEntry {
	next := append([]hiddenWorkspaceEntry(nil), entries...)
	sort.SliceStable(next, func(i, j int) bool {
		left := workspaceIdentityStorageKey(next[i].WorkspaceID, next[i].OrgID)
		right := workspaceIdentityStorageKey(next[j].WorkspaceID, next[j].OrgID)
		if left == right {
			return strings.TrimSpace(next[i].HiddenAt) < strings.TrimSpace(next[j].HiddenAt)
		}
		return left < right
	})
	return next
}

func workspaceIdentityStorageKey(workspaceID string, orgID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return ""
	}
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return workspaceID
	}
	return orgID + ":" + workspaceID
}

func (s *Service) workspaceIndexPath() string {
	return filepath.Join(s.homeDir(), ".openbrain", "index", "workspaces.json")
}

func (s *Service) cloudSourcesSnapshotPath() string {
	return filepath.Join(s.homeDir(), ".openbrain", "cache", "cloud-sources.json")
}

func cloudSourcesSnapshotOrgID(auth authConfig) string {
	return firstNonEmpty(auth.ActiveOrgID, auth.DefaultOrgID)
}

func cloudSourcesSnapshotMatchesAuth(snapshot cloudSourcesSnapshotFile, auth authConfig) bool {
	return snapshot.Version == 1 &&
		strings.TrimSpace(snapshot.Provider) == "cloud" &&
		strings.TrimSpace(snapshot.UID) != "" &&
		strings.TrimSpace(snapshot.UID) == strings.TrimSpace(auth.UID) &&
		strings.TrimSpace(snapshot.OrgID) == cloudSourcesSnapshotOrgID(auth)
}

func (s *Service) loadCloudSourcesSnapshot() (cloudSourcesSnapshotFile, error) {
	var snapshot cloudSourcesSnapshotFile
	raw, err := os.ReadFile(s.cloudSourcesSnapshotPath())
	if err != nil {
		return snapshot, err
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return snapshot, err
	}
	if snapshot.Sources == nil {
		snapshot.Sources = []Source{}
	}
	return snapshot, nil
}

func (s *Service) saveCloudSourcesSnapshot(auth authConfig, sources []Source) error {
	snapshot := cloudSourcesSnapshotFile{
		Version:   1,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		UID:       strings.TrimSpace(auth.UID),
		OrgID:     cloudSourcesSnapshotOrgID(auth),
		Provider:  "cloud",
		Sources:   append([]Source(nil), sources...),
	}
	sortSourcesByName(snapshot.Sources)
	return s.saveCloudSourcesSnapshotFile(snapshot)
}

func (s *Service) saveCloudSourcesSnapshotFile(snapshot cloudSourcesSnapshotFile) error {
	if snapshot.Version == 0 {
		snapshot.Version = 1
	}
	if strings.TrimSpace(snapshot.Provider) == "" {
		snapshot.Provider = "cloud"
	}
	if snapshot.Sources == nil {
		snapshot.Sources = []Source{}
	}
	sortSourcesByName(snapshot.Sources)
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	target := s.cloudSourcesSnapshotPath()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

func (s *Service) updateCloudSourcesSnapshot(auth authConfig, update func([]Source) []Source) {
	snapshot, err := s.loadCloudSourcesSnapshot()
	if err != nil || !cloudSourcesSnapshotMatchesAuth(snapshot, auth) {
		return
	}
	snapshot.Sources = update(append([]Source(nil), snapshot.Sources...))
	_ = s.saveCloudSourcesSnapshotFile(snapshot)
}

func (s *Service) removeCloudSourceFromSnapshot(auth authConfig, orgID string, workspaceID string) {
	workspaceID = strings.TrimSpace(workspaceID)
	orgID = firstNonEmpty(orgID, cloudSourcesSnapshotOrgID(auth))
	if workspaceID == "" {
		return
	}
	s.updateCloudSourcesSnapshot(auth, func(sources []Source) []Source {
		next := make([]Source, 0, len(sources))
		for _, source := range sources {
			if cloudSourceSnapshotMatches(source, orgID, workspaceID) {
				continue
			}
			next = append(next, source)
		}
		return next
	})
}

func (s *Service) updateCloudSourceSnapshotQueriesDisabled(auth authConfig, orgID string, workspaceID string, disabled bool) {
	workspaceID = strings.TrimSpace(workspaceID)
	orgID = firstNonEmpty(orgID, cloudSourcesSnapshotOrgID(auth))
	if workspaceID == "" {
		return
	}
	s.updateCloudSourcesSnapshot(auth, func(sources []Source) []Source {
		for i := range sources {
			if cloudSourceSnapshotMatches(sources[i], orgID, workspaceID) {
				sources[i].DisabledQueries = disabled
			}
		}
		return sources
	})
}

func (s *Service) updateCloudSourceSnapshotPublicAccess(auth authConfig, orgID string, workspaceID string, public bool) {
	workspaceID = strings.TrimSpace(workspaceID)
	orgID = firstNonEmpty(orgID, cloudSourcesSnapshotOrgID(auth))
	if workspaceID == "" {
		return
	}
	s.updateCloudSourcesSnapshot(auth, func(sources []Source) []Source {
		for i := range sources {
			if cloudSourceSnapshotMatches(sources[i], orgID, workspaceID) {
				sources[i].PublicAccess = public
			}
		}
		return sources
	})
}

func (s *Service) updateCurrentCloudSourceSnapshotPublicAccess(orgID string, workspaceID string, public bool) {
	auth, err := s.loadAuth()
	if err != nil {
		return
	}
	s.updateCloudSourceSnapshotPublicAccess(auth, orgID, workspaceID, public)
}

func (s *Service) upsertCloudSourceSnapshot(auth authConfig, source *Source) {
	if source == nil || strings.TrimSpace(source.WorkspaceID) == "" {
		return
	}
	orgID := firstNonEmpty(source.OrgID, cloudSourcesSnapshotOrgID(auth))
	workspaceID := firstNonEmpty(source.WorkspaceID, source.SourceID)
	s.updateCloudSourcesSnapshot(auth, func(sources []Source) []Source {
		for i := range sources {
			if cloudSourceSnapshotMatches(sources[i], orgID, workspaceID) {
				sources[i] = mergeCloudSourceSnapshot(sources[i], *source)
				return sources
			}
		}
		sources = append(sources, *source)
		return sources
	})
}

func cloudSourceSnapshotMatches(source Source, orgID string, workspaceID string) bool {
	sourceWorkspaceID := firstNonEmpty(source.WorkspaceID, source.SourceID)
	if strings.TrimSpace(sourceWorkspaceID) != strings.TrimSpace(workspaceID) {
		return false
	}
	sourceOrgID := strings.TrimSpace(source.OrgID)
	return orgID == "" || sourceOrgID == "" || sourceOrgID == strings.TrimSpace(orgID)
}

func mergeCloudSourceSnapshot(existing Source, next Source) Source {
	merged := next
	if strings.TrimSpace(merged.Path) == "" {
		merged.Path = existing.Path
	}
	if merged.RemoteURL == nil {
		merged.RemoteURL = existing.RemoteURL
	}
	if strings.TrimSpace(merged.LocationKind) == "" {
		merged.LocationKind = existing.LocationKind
	}
	if strings.TrimSpace(merged.LocalName) == "" {
		merged.LocalName = existing.LocalName
	}
	if strings.TrimSpace(merged.TemplateID) == "" {
		merged.TemplateID = existing.TemplateID
	}
	if merged.TemplateVersion == 0 {
		merged.TemplateVersion = existing.TemplateVersion
	}
	if strings.TrimSpace(merged.DefaultLocalName) == "" {
		merged.DefaultLocalName = existing.DefaultLocalName
	}
	if strings.TrimSpace(merged.LastVerifiedAt) == "" {
		merged.LastVerifiedAt = existing.LastVerifiedAt
	}
	if strings.TrimSpace(merged.LastVerifyReason) == "" {
		merged.LastVerifyReason = existing.LastVerifyReason
	}
	return merged
}

func sortSourcesByName(sources []Source) {
	sort.SliceStable(sources, func(i, j int) bool {
		return strings.ToLower(sources[i].Name) < strings.ToLower(sources[j].Name)
	})
}

func (s *Service) findIndexedWorkspaceByPath(auth authConfig, workspacePath string) (workspaceIndexEntry, error) {
	index, err := s.loadWorkspaceIndex(auth, nil, false)
	if err != nil {
		return workspaceIndexEntry{}, err
	}
	normalized := cleanPath(workspacePath)
	for _, entry := range index.Workspaces {
		if normalized != "" && cleanPath(entry.Path) == normalized {
			return entry, nil
		}
	}
	return workspaceIndexEntry{}, os.ErrNotExist
}

func (s *Service) upsertCloudWorkspaceIndex(auth authConfig, workspace createWorkspaceResult, workspacePath string, name string, takeover bool) (workspaceIndexEntry, error) {
	index, err := s.loadWorkspaceIndex(auth, nil, false)
	if err != nil {
		return workspaceIndexEntry{}, err
	}
	activeUID := strings.TrimSpace(auth.UID)
	now := time.Now().UTC().Format(time.RFC3339)
	localName := firstNonEmpty(name, workspace.DefaultLocalName, filepath.Base(workspacePath), "workspace")
	entry := workspaceIndexEntry{
		WorkspaceID:         workspace.WorkspaceID,
		OrgID:               workspace.OrgID,
		LocalName:           localName,
		Path:                workspacePath,
		LocationKind:        "local",
		TemplateID:          firstNonEmpty(workspace.TemplateID, cloudWorkspaceTemplateID),
		TemplateVersion:     workspace.TemplateVersion,
		BackupEnabled:       workspace.BackupEnabled,
		Repository:          workspace.Repository,
		Storage:             workspace.Storage,
		SyncPolicy:          syncPolicyFromStorage(workspace.Storage),
		CreatedAt:           now,
		UpdatedAt:           now,
		LastVerifiedAt:      now,
		LastVerifyReason:    sourceBindingReasonConnected,
		EffectivePermission: workspace.EffectivePermission,
		CanMutateSource:     workspace.CanMutateSource,
		PublicOwnerUID:      workspace.PublicOwnerUID,
		BindingMode:         workspace.BindingMode,
	}
	if entry.TemplateVersion == 0 {
		entry.TemplateVersion = 1
	}
	if entry.SyncPolicy == nil {
		entry.SyncPolicy = defaultCloudSyncPolicy()
	}
	if entry.Storage == nil {
		entry.Storage = map[string]interface{}{}
	}
	if _, ok := entry.Storage["syncPolicy"]; !ok {
		entry.Storage["syncPolicy"] = entry.SyncPolicy
	}
	entryWorkspaceID := strings.TrimSpace(entry.WorkspaceID)
	entryPath := cleanPath(entry.Path)
	if entryPath != "" {
		for uid, account := range index.Accounts {
			if account == nil {
				continue
			}
			nextAccountEntries := account.Workspaces[:0]
			for _, existing := range account.Workspaces {
				samePath := cleanPath(existing.Path) == entryPath
				if uid == activeUID {
					nextAccountEntries = append(nextAccountEntries, existing)
					continue
				}
				if samePath {
					if !takeover {
						return workspaceIndexEntry{}, &workspacePathOwnerError{
							path:        entry.Path,
							ownerUID:    uid,
							workspaceID: strings.TrimSpace(existing.WorkspaceID),
						}
					}
					continue
				}
				nextAccountEntries = append(nextAccountEntries, existing)
			}
			account.Workspaces = nextAccountEntries
		}
	}
	next := make([]workspaceIndexEntry, 0, len(index.Workspaces)+1)
	for _, existing := range index.Workspaces {
		if strings.TrimSpace(existing.WorkspaceID) == entryWorkspaceID {
			if strings.TrimSpace(existing.CreatedAt) != "" {
				entry.CreatedAt = existing.CreatedAt
			}
			continue
		}
		if entryPath != "" && cleanPath(existing.Path) == entryPath {
			return workspaceIndexEntry{}, fmt.Errorf("%w: %s is already bound to workspace %s", errWorkspacePathConflict, entry.Path, existing.WorkspaceID)
		}
		next = append(next, existing)
	}
	next = append(next, entry)
	index.Workspaces = next
	index.HiddenWorkspaces = filterHidden(index.HiddenWorkspaces, entry.WorkspaceID, entry.OrgID)
	if err := s.saveWorkspaceIndex(index); err != nil {
		return workspaceIndexEntry{}, err
	}
	return entry, nil
}

func (s *Service) removeWorkspaceFromDevice(auth authConfig, workspaceID string, orgID string, workspacePath string, hide bool) error {
	index, err := s.loadWorkspaceIndex(auth, nil, false)
	if err != nil {
		return err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	normalizedPath := cleanPath(workspacePath)
	removed := make([]workspaceIndexEntry, 0, 1)
	next := make([]workspaceIndexEntry, 0, len(index.Workspaces))
	for _, entry := range index.Workspaces {
		matchesID := workspaceID != "" && strings.TrimSpace(entry.WorkspaceID) == workspaceID
		matchesPath := normalizedPath != "" && cleanPath(entry.Path) == normalizedPath
		if matchesID || matchesPath {
			removed = append(removed, entry)
			continue
		}
		next = append(next, entry)
	}
	index.Workspaces = next
	if hide {
		now := time.Now().UTC().Format(time.RFC3339)
		hidden := index.HiddenWorkspaces
		if workspaceID != "" {
			hidden = appendHidden(hidden, workspaceID, orgID, now)
		}
		for _, entry := range removed {
			hidden = appendHidden(hidden, entry.WorkspaceID, entry.OrgID, now)
		}
		index.HiddenWorkspaces = hidden
	}
	return s.saveWorkspaceIndex(index)
}

func importGitWorkspace(ctx context.Context, workspacePath string, remoteURL string, branch string, token *workspaceGitToken, preserveOrigin bool) error {
	if err := assertExistingDirectory(workspacePath); err != nil {
		return err
	}
	branch = firstNonEmpty(branch, "main")
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return errors.New("Git remote URL is missing")
	}
	if _, err := runGit(ctx, workspacePath, nil, "rev-parse", "--is-inside-work-tree"); err != nil {
		if _, err := runGit(ctx, workspacePath, nil, "init"); err != nil {
			return err
		}
		if _, err := runGit(ctx, workspacePath, nil, "checkout", "-B", branch); err != nil {
			return err
		}
	}
	if !preserveOrigin {
		if err := ensureGitRemote(ctx, workspacePath, remoteURL); err != nil {
			return err
		}
	}
	status, _ := runGit(ctx, workspacePath, nil, "status", "--porcelain")
	hasCommits := gitHasCommits(ctx, workspacePath)
	if strings.TrimSpace(status) != "" || !hasCommits {
		if _, err := runGit(ctx, workspacePath, nil, "add", "-A"); err != nil {
			return err
		}
		message := "Sync workspace"
		if !hasCommits {
			message = "Initial workspace import"
		}
		if _, err := runGit(ctx, workspacePath, nil, "-c", "user.name=OpenBrain", "-c", "user.email=openbrain@users.noreply.github.com", "commit", "-m", message); err != nil && !isNothingToCommit(err) {
			return err
		}
	}
	if !gitHasCommits(ctx, workspacePath) {
		if _, err := runGit(ctx, workspacePath, nil, "-c", "user.name=OpenBrain", "-c", "user.email=openbrain@users.noreply.github.com", "commit", "--allow-empty", "-m", "Initial workspace import"); err != nil {
			return err
		}
	}
	askpass, err := createAskpass(token)
	if err != nil {
		return err
	}
	defer askpass.cleanup()
	remoteHasBranch, err := remoteBranchExists(ctx, workspacePath, askpass.env, remoteURL, branch)
	if err != nil {
		return err
	}
	if remoteHasBranch {
		if _, err := runGit(ctx, workspacePath, askpass.env, "pull", "--rebase", "--allow-unrelated-histories", remoteURL, branch); err != nil {
			abortRebaseBestEffort(ctx, workspacePath)
			return err
		}
	}
	pushRemote := "origin"
	if preserveOrigin {
		pushRemote = remoteURL
	}
	if _, err := runGit(ctx, workspacePath, askpass.env, "push", "-u", pushRemote, "HEAD:"+branch); err != nil {
		return err
	}
	return nil
}

type gitImportRollback struct {
	workspacePath string
	hadGit        bool
	hadOrigin     bool
	originURL     string
}

func captureGitImportRollback(ctx context.Context, workspacePath string) *gitImportRollback {
	state := &gitImportRollback{workspacePath: workspacePath}
	if _, err := runGit(ctx, workspacePath, nil, "rev-parse", "--is-inside-work-tree"); err == nil {
		state.hadGit = true
		if origin, err := runGit(ctx, workspacePath, nil, "remote", "get-url", "origin"); err == nil {
			state.hadOrigin = true
			state.originURL = strings.TrimSpace(origin)
		}
	}
	return state
}

func (r *gitImportRollback) rollback(ctx context.Context) error {
	if r == nil || strings.TrimSpace(r.workspacePath) == "" {
		return nil
	}
	abortRebaseBestEffort(ctx, r.workspacePath)
	if !r.hadGit {
		gitDir := filepath.Join(r.workspacePath, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return os.RemoveAll(gitDir)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	currentOrigin, err := runGit(ctx, r.workspacePath, nil, "remote", "get-url", "origin")
	if err != nil {
		return nil
	}
	if !r.hadOrigin {
		_, err = runGit(ctx, r.workspacePath, nil, "remote", "remove", "origin")
		return err
	}
	if strings.TrimSpace(r.originURL) != "" && normalizeGitRemote(currentOrigin) != normalizeGitRemote(r.originURL) {
		_, err = runGit(ctx, r.workspacePath, nil, "remote", "set-url", "origin", r.originURL)
		return err
	}
	return nil
}

func abortRebaseBestEffort(ctx context.Context, workspacePath string) {
	_, _ = runGit(ctx, workspacePath, nil, "rebase", "--abort")
}

func remoteBranchExists(ctx context.Context, workspacePath string, env []string, remoteURL string, branch string) (bool, error) {
	out, err := runGit(ctx, workspacePath, env, "ls-remote", "--heads", remoteURL, branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func ensureGitRemote(ctx context.Context, workspacePath string, remoteURL string) error {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return errors.New("Git remote URL is missing")
	}
	existing, err := runGit(ctx, workspacePath, nil, "remote", "get-url", "origin")
	if err != nil {
		_, err = runGit(ctx, workspacePath, nil, "remote", "add", "origin", remoteURL)
		return err
	}
	if normalizeGitRemote(strings.TrimSpace(existing)) == normalizeGitRemote(remoteURL) {
		return nil
	}
	return fmt.Errorf("selected directory already has a different origin remote: %s", strings.TrimSpace(existing))
}

func inspectGitHubRepository(ctx context.Context, workspacePath string) (*githubRepoRef, error) {
	if err := assertExistingDirectory(workspacePath); err != nil {
		return nil, err
	}
	if _, err := runGit(ctx, workspacePath, nil, "rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, nil
	}
	remote, err := runGit(ctx, workspacePath, nil, "remote", "get-url", "origin")
	if err != nil || strings.TrimSpace(remote) == "" {
		return nil, nil
	}
	owner, name, ok := parseGitHubRemoteURL(strings.TrimSpace(remote))
	if !ok {
		return nil, fmt.Errorf("OpenBrain Cloud only supports GitHub origin repositories. Selected directory origin is: %s", strings.TrimSpace(remote))
	}
	return &githubRepoRef{Owner: owner, Name: name, RemoteURL: strings.TrimSpace(remote)}, nil
}

func inspectGitHubRepositoryForSourceCreate(ctx context.Context, workspacePath string) (*githubRepoRef, error) {
	repo, err := inspectGitHubRepository(ctx, workspacePath)
	if err != nil || repo != nil {
		return repo, err
	}
	if isGitWorkTree(ctx, workspacePath) {
		return nil, errors.New("Selected directory is already a Git repository but does not have a GitHub origin remote. Add a GitHub origin remote or choose a non-Git directory so OpenBrain can initialize one.")
	}
	return nil, nil
}

func isGitWorkTree(ctx context.Context, workspacePath string) bool {
	_, err := runGit(ctx, workspacePath, nil, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

func runGit(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
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

func ensureGitAvailable() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required to create an OpenBrain Cloud source: %w", err)
	}
	return nil
}

func newSourceCreateRequestID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "src-" + fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return "src-" + hex.EncodeToString(raw[:])
}

func gitHasCommits(ctx context.Context, workspacePath string) bool {
	_, err := runGit(ctx, workspacePath, nil, "rev-parse", "--verify", "HEAD")
	return err == nil
}

func createAskpass(token *workspaceGitToken) (*askpassEnv, error) {
	// Disable git credential helpers (e.g. macOS osxkeychain) for OpenBrain-managed
	// git commands. System helpers run before GIT_ASKPASS and may return a token
	// scoped to a different repository, which GitHub reports as "Repository not
	// found" for private repos. An empty value clears the helper list (git 2.31+).
	gitCredentialOverride := []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
	}
	if token == nil || strings.TrimSpace(token.AccessToken) == "" {
		return &askpassEnv{
			env: append([]string{
				"GIT_TERMINAL_PROMPT=0",
				"GCM_INTERACTIVE=never",
			}, gitCredentialOverride...),
		}, nil
	}
	dir, err := os.MkdirTemp("", "openbrain-git-askpass-*")
	if err != nil {
		return nil, err
	}
	username := firstNonEmpty(token.Username, "x-access-token")
	script := filepath.Join(dir, "askpass.sh")
	content := "#!/bin/sh\ncase \"$1\" in\n*Username*) printf '%s\\n' \"$OPENBRAIN_GIT_ASKPASS_USERNAME\" ;;\n*) printf '%s\\n' \"$OPENBRAIN_GIT_ASKPASS_TOKEN\" ;;\nesac\n"
	if runtime.GOOS == "windows" {
		script = filepath.Join(dir, "askpass.cmd")
		content = "@echo off\r\necho %~1 | findstr /I \"Username\" >nul\r\nif %errorlevel%==0 (\r\n  echo %OPENBRAIN_GIT_ASKPASS_USERNAME%\r\n) else (\r\n  echo %OPENBRAIN_GIT_ASKPASS_TOKEN%\r\n)\r\n"
	}
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &askpassEnv{
		dir: dir,
		env: append([]string{
			"GIT_ASKPASS=" + script,
			"GIT_TERMINAL_PROMPT=0",
			"GCM_INTERACTIVE=never",
			"OPENBRAIN_GIT_ASKPASS_USERNAME=" + username,
			"OPENBRAIN_GIT_ASKPASS_TOKEN=" + strings.TrimSpace(token.AccessToken),
		}, gitCredentialOverride...),
	}, nil
}

func (a *askpassEnv) cleanup() {
	if a != nil && a.dir != "" {
		_ = os.RemoveAll(a.dir)
	}
}

func setAuthHeaders(req *http.Request, auth authConfig) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(auth.Token))
	if strings.TrimSpace(auth.UID) != "" {
		req.Header.Set("X-UID", strings.TrimSpace(auth.UID))
	}
}

func readCloudJSON(res *http.Response, out interface{}) error {
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return fmt.Errorf("cloud_auth: OpenBrain request failed: %s", res.Status)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		if message == "" {
			message = res.Status
		}
		return fmt.Errorf("OpenBrain request failed: %s", message)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("OpenBrain request returned non-JSON response")
	}
	return nil
}

func isCloudAuthError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "cloud_auth")
}

func errorCodeFromCloud(err error) string {
	if isCloudAuthError(err) {
		return "cloud_unauthorized"
	}
	return "openbrain_error"
}

func cloudWorkspaceName(workspace cloudWorkspace) string {
	return firstNonEmpty(workspace.Name, workspace.Slug, workspace.ID, "OpenBrain workspace")
}

func cloudWorkspaceEffectivePermission(workspace cloudWorkspace) string {
	permission := strings.ToLower(strings.TrimSpace(workspace.EffectivePermission))
	switch permission {
	case "read", "write", "admin":
		return permission
	default:
		if strings.TrimSpace(workspace.PublicOwnerUID) != "" {
			return "read"
		}
		return "admin"
	}
}

func cloudPermissionRank(permission string) int {
	switch strings.ToLower(strings.TrimSpace(permission)) {
	case "admin":
		return 3
	case "write":
		return 2
	case "read":
		return 1
	default:
		return 0
	}
}

func cloudWorkspaceCanMutate(workspace cloudWorkspace) bool {
	if strings.TrimSpace(workspace.PublicOwnerUID) != "" {
		return false
	}
	if workspace.CanMutateSource {
		return true
	}
	return cloudPermissionRank(cloudWorkspaceEffectivePermission(workspace)) >= cloudPermissionRank("write")
}

func cloudWorkspaceBindingMode(workspace cloudWorkspace) string {
	if cloudWorkspaceCanMutate(workspace) {
		return "own"
	}
	return "granted"
}

func cloudWorkspaceHasRepositoryIdentity(workspace cloudWorkspace) bool {
	return strings.TrimSpace(workspace.RepoExternalID) != "" ||
		(strings.TrimSpace(workspace.RepoOwner) != "" && strings.TrimSpace(workspace.RepoName) != "") ||
		strings.TrimSpace(workspace.RepoURL) != "" ||
		strings.TrimSpace(workspace.RepoWebURL) != "" ||
		strings.TrimSpace(workspace.StorageRemoteURL) != ""
}

func cloudWorkspaceRepositoryMatches(workspace cloudWorkspace, repo githubRepoRef) bool {
	if !strings.EqualFold(strings.TrimSpace(workspace.RepoProvider), "github") {
		return false
	}
	workspaceExternalID := normalizeRepoExternalID(workspace.RepoExternalID)
	repoExternalID := normalizeRepoExternalID(repo.ExternalID)
	if workspaceExternalID != "" && repoExternalID != "" {
		return workspaceExternalID == repoExternalID
	}
	workspaceOwnerName := repoOwnerNameKey(workspace.RepoProvider, workspace.RepoOwner, workspace.RepoName)
	repoOwnerName := repoOwnerNameKey("github", repo.Owner, repo.Name)
	if workspaceOwnerName != "" && repoOwnerName != "" {
		return workspaceOwnerName == repoOwnerName
	}
	return repoKeysIntersect(cloudWorkspaceRepoKeys(workspace), githubRepoRefKeys(repo))
}

func findWorkspaceIndexByRepo(
	workspace cloudWorkspace,
	entries []workspaceIndexEntry,
	indexByRepoKey map[string][]int,
	usedIndex map[int]bool,
) (int, bool) {
	for _, key := range cloudWorkspaceRepoKeys(workspace) {
		for _, indexPosition := range indexByRepoKey[key] {
			if indexPosition >= 0 && indexPosition < len(entries) && !usedIndex[indexPosition] && workspaceIndexRepositoryMatchesCloudWorkspace(entries[indexPosition], workspace) {
				return indexPosition, true
			}
		}
	}
	return 0, false
}

func rekeyWorkspaceIndexEntry(entry *workspaceIndexEntry, workspace cloudWorkspace) bool {
	if entry == nil {
		return false
	}
	changed := false
	workspaceID := strings.TrimSpace(workspace.ID)
	if workspaceID != "" && strings.TrimSpace(entry.WorkspaceID) != workspaceID {
		entry.WorkspaceID = workspaceID
		changed = true
	}
	orgID := strings.TrimSpace(workspace.OrgID)
	if orgID != "" && strings.TrimSpace(entry.OrgID) != orgID {
		entry.OrgID = orgID
		changed = true
	}
	if changed {
		entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return changed
}

func workspaceIndexRepositoryMatchesCloudWorkspace(entry workspaceIndexEntry, workspace cloudWorkspace) bool {
	workspaceExternalID := normalizeRepoExternalID(workspace.RepoExternalID)
	entryExternalID := normalizeRepoExternalID(firstNonEmpty(stringFromMap(entry.Repository, "externalID"), stringFromMap(entry.Storage, "remoteID")))
	if workspaceExternalID != "" && entryExternalID != "" {
		return workspaceExternalID == entryExternalID
	}
	workspaceOwnerName := repoOwnerNameKey(workspace.RepoProvider, workspace.RepoOwner, workspace.RepoName)
	entryOwnerName := repoOwnerNameKey(
		firstNonEmpty(stringFromMap(entry.Repository, "provider"), stringFromMap(entry.Storage, "provider")),
		stringFromMap(entry.Repository, "owner"),
		stringFromMap(entry.Repository, "name"),
	)
	if workspaceOwnerName != "" && entryOwnerName != "" {
		return workspaceOwnerName == entryOwnerName
	}
	return repoKeysIntersect(cloudWorkspaceRepoKeys(workspace), workspaceIndexRepoKeys(entry))
}

func cloudWorkspaceRepoKeys(workspace cloudWorkspace) []string {
	keys := []string{}
	keys = appendRepoExternalIDKey(keys, workspace.RepoExternalID)
	keys = appendRepoOwnerNameKey(keys, workspace.RepoProvider, workspace.RepoOwner, workspace.RepoName)
	keys = appendRepoURLKeys(keys, workspace.RepoURL)
	keys = appendRepoURLKeys(keys, workspace.RepoWebURL)
	keys = appendRepoURLKeys(keys, workspace.StorageRemoteURL)
	return dedupeStrings(keys)
}

func workspaceIndexRepoKeys(entry workspaceIndexEntry) []string {
	keys := []string{}
	keys = appendRepoExternalIDKey(keys, stringFromMap(entry.Repository, "externalID"))
	keys = appendRepoExternalIDKey(keys, stringFromMap(entry.Storage, "remoteID"))
	provider := firstNonEmpty(stringFromMap(entry.Repository, "provider"), stringFromMap(entry.Storage, "provider"))
	keys = appendRepoOwnerNameKey(keys, provider, stringFromMap(entry.Repository, "owner"), stringFromMap(entry.Repository, "name"))
	keys = appendRepoURLKeys(keys, stringFromMap(entry.Repository, "remoteURL"))
	keys = appendRepoURLKeys(keys, stringFromMap(entry.Repository, "webURL"))
	keys = appendRepoURLKeys(keys, stringFromMap(entry.Storage, "remoteURL"))
	return dedupeStrings(keys)
}

func githubRepoRefFromWorkspaceIndexEntry(entry workspaceIndexEntry) (githubRepoRef, bool) {
	provider := firstNonEmpty(stringFromMap(entry.Repository, "provider"), stringFromMap(entry.Storage, "provider"))
	if provider != "" && !strings.EqualFold(provider, "github") {
		return githubRepoRef{}, false
	}
	repo := githubRepoRef{
		Owner:      stringFromMap(entry.Repository, "owner"),
		Name:       stringFromMap(entry.Repository, "name"),
		RemoteURL:  firstNonEmpty(stringFromMap(entry.Repository, "remoteURL"), stringFromMap(entry.Storage, "remoteURL")),
		ExternalID: firstNonEmpty(stringFromMap(entry.Repository, "externalID"), stringFromMap(entry.Storage, "remoteID")),
	}
	if strings.TrimSpace(repo.Owner) == "" || strings.TrimSpace(repo.Name) == "" {
		if owner, name, ok := parseGitHubRemoteURL(repo.RemoteURL); ok {
			repo.Owner = firstNonEmpty(repo.Owner, owner)
			repo.Name = firstNonEmpty(repo.Name, name)
		}
	}
	if strings.TrimSpace(repo.ExternalID) == "" && strings.TrimSpace(repo.Owner) == "" && strings.TrimSpace(repo.Name) == "" && strings.TrimSpace(repo.RemoteURL) == "" {
		return githubRepoRef{}, false
	}
	return repo, true
}

func workspaceIndexRepositoryMatchesRepo(entry workspaceIndexEntry, repo githubRepoRef) bool {
	entryExternalID := normalizeRepoExternalID(firstNonEmpty(stringFromMap(entry.Repository, "externalID"), stringFromMap(entry.Storage, "remoteID")))
	repoExternalID := normalizeRepoExternalID(repo.ExternalID)
	if entryExternalID != "" && repoExternalID != "" {
		return entryExternalID == repoExternalID
	}
	return repoKeysIntersect(workspaceIndexRepoKeys(entry), githubRepoRefKeys(repo))
}

func githubRepoRefKeys(repo githubRepoRef) []string {
	keys := []string{}
	keys = appendRepoExternalIDKey(keys, repo.ExternalID)
	keys = appendRepoOwnerNameKey(keys, "github", repo.Owner, repo.Name)
	keys = appendRepoURLKeys(keys, repo.RemoteURL)
	return dedupeStrings(keys)
}

func normalizeRepoExternalID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func repoOwnerNameKey(provider string, owner string, name string) string {
	keys := appendRepoOwnerNameKey(nil, provider, owner, name)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func repoKeysIntersect(left []string, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, key := range left {
		key = strings.TrimSpace(key)
		if key != "" {
			set[key] = true
		}
	}
	for _, key := range right {
		if set[strings.TrimSpace(key)] {
			return true
		}
	}
	return false
}

func appendRepoExternalIDKey(keys []string, externalID string) []string {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return keys
	}
	return append(keys, "github-external:"+strings.ToLower(externalID))
}

func appendRepoOwnerNameKey(keys []string, provider string, owner string, name string) []string {
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return keys
	}
	provider = strings.ToLower(firstNonEmpty(provider, "github"))
	if provider != "github" {
		return keys
	}
	return append(keys, "github:"+strings.ToLower(owner)+"/"+strings.ToLower(strings.TrimSuffix(name, ".git")))
}

func appendRepoURLKeys(keys []string, remoteURL string) []string {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return keys
	}
	if owner, name, ok := parseGitHubRemoteURL(remoteURL); ok {
		keys = append(keys, "github:"+strings.ToLower(owner)+"/"+strings.ToLower(name))
	}
	if normalized := normalizeGitRemote(remoteURL); normalized != "" {
		keys = append(keys, "remote:"+normalized)
	}
	return keys
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	result := values[:0]
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func createWorkspaceResultFromCloudWorkspace(workspace cloudWorkspace) createWorkspaceResult {
	remoteURL := firstNonEmpty(workspace.RepoURL, workspace.StorageRemoteURL)
	branch := firstNonEmpty(workspace.DefaultBranch, "main")
	return createWorkspaceResult{
		WorkspaceID:         strings.TrimSpace(workspace.ID),
		OrgID:               strings.TrimSpace(workspace.OrgID),
		TemplateID:          cloudWorkspaceTemplateID,
		TemplateVersion:     1,
		BackupEnabled:       true,
		DefaultLocalName:    firstNonEmpty(workspace.RepoName, cloudWorkspaceName(workspace)),
		EffectivePermission: cloudWorkspaceEffectivePermission(workspace),
		CanMutateSource:     cloudWorkspaceCanMutate(workspace),
		PublicOwnerUID:      strings.TrimSpace(workspace.PublicOwnerUID),
		BindingMode:         cloudWorkspaceBindingMode(workspace),
		Repository: map[string]interface{}{
			"enabled":       strings.TrimSpace(workspace.RepoProvider) != "" && remoteURL != "",
			"provider":      strings.TrimSpace(workspace.RepoProvider),
			"remoteURL":     remoteURL,
			"webURL":        strings.TrimSpace(workspace.RepoWebURL),
			"owner":         strings.TrimSpace(workspace.RepoOwner),
			"name":          strings.TrimSpace(workspace.RepoName),
			"defaultBranch": branch,
			"externalID":    strings.TrimSpace(workspace.RepoExternalID),
		},
		Storage: map[string]interface{}{
			"enabled":    true,
			"backend":    firstNonEmpty(workspace.StorageBackend, "git"),
			"provider":   firstNonEmpty(workspace.StorageProvider, workspace.RepoProvider, "github"),
			"remoteURL":  remoteURL,
			"syncPolicy": defaultCloudSyncPolicy(),
		},
		Manifest: map[string]interface{}{},
	}
}

func createWorkspaceResultFromResolvedWorkspace(workspace cloudWorkspace, repo *githubRepoRef, resolved cloudWorkspaceResolveResult) createWorkspaceResult {
	workspace = cloudWorkspaceWithResolveMetadata(workspace, resolved)
	result := createWorkspaceResultFromCloudWorkspace(workspace)
	if repo != nil {
		repository := result.Repository
		if repository == nil {
			repository = map[string]interface{}{}
		}
		repository["enabled"] = true
		repository["provider"] = "github"
		repository["owner"] = firstNonEmpty(stringFromMap(repository, "owner"), repo.Owner)
		repository["name"] = firstNonEmpty(stringFromMap(repository, "name"), repo.Name)
		repository["remoteURL"] = firstNonEmpty(stringFromMap(repository, "remoteURL"), repo.RemoteURL)
		repository["externalID"] = firstNonEmpty(stringFromMap(repository, "externalID"), repo.ExternalID)
		repository["defaultBranch"] = firstNonEmpty(stringFromMap(repository, "defaultBranch"), result.defaultBranch())
		result.Repository = repository
		if result.Storage == nil {
			result.Storage = map[string]interface{}{}
		}
		if strings.TrimSpace(stringFromMap(result.Storage, "provider")) == "" {
			result.Storage["provider"] = "github"
		}
		if strings.TrimSpace(stringFromMap(result.Storage, "backend")) == "" {
			result.Storage["backend"] = "git"
		}
		if strings.TrimSpace(stringFromMap(result.Storage, "remoteURL")) == "" {
			result.Storage["remoteURL"] = firstNonEmpty(repo.RemoteURL, stringFromMap(repository, "remoteURL"))
		}
	}
	result.EffectivePermission = firstNonEmpty(resolved.EffectivePermission, result.EffectivePermission)
	result.CanMutateSource = cloudWorkspaceCanMutate(workspace)
	result.PublicOwnerUID = firstNonEmpty(resolved.PublicOwnerUID, result.PublicOwnerUID)
	result.BindingMode = cloudWorkspaceBindingMode(workspace)
	return result
}

func (w createWorkspaceResult) remoteURL() string {
	if value, ok := w.Storage["remoteURL"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := w.Repository["remoteURL"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func (w createWorkspaceResult) defaultBranch() string {
	if value, ok := w.Repository["defaultBranch"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return "main"
}

func mapCloudResult(raw cloudSearchResult, sourceNames map[string]string) *QueryResult {
	workspaceID := firstNonEmpty(raw.WorkspaceID, raw.SourceID)
	text := strings.TrimSpace(raw.ChunkText)
	if workspaceID == "" || text == "" {
		return nil
	}
	relativePath := firstNonEmpty(raw.Path, raw.Slug)
	chunkID := fmt.Sprint(raw.ChunkID)
	if strings.TrimSpace(chunkID) == "" || chunkID == "<nil>" {
		chunkID = strings.Join([]string{workspaceID, raw.Slug, fmt.Sprint(raw.ChunkIndex)}, ":")
	}
	return &QueryResult{
		ChunkID:       chunkID,
		WorkspaceID:   workspaceID,
		WorkspaceName: firstNonEmpty(raw.WorkspaceName, raw.SourceName, sourceNames[workspaceID], workspaceID),
		Path:          relativePath,
		RelativePath:  relativePath,
		Title:         firstNonEmpty(raw.Title, raw.Slug, relativePath, "OpenBrain note"),
		Text:          text,
		Score:         raw.Score,
	}
}

func sourceFromIndex(entry workspaceIndexEntry) *Source {
	if strings.TrimSpace(entry.WorkspaceID) == "" {
		return nil
	}
	source := &Source{
		SourceID:            entry.WorkspaceID,
		WorkspaceID:         entry.WorkspaceID,
		OrgID:               entry.OrgID,
		BrainID:             "personal",
		Name:                firstNonEmpty(entry.LocalName, filepath.Base(entry.Path), entry.WorkspaceID),
		Path:                entry.Path,
		RemoteURL:           stringPtr(stringFromMap(entry.Storage, "remoteURL")),
		Openable:            strings.TrimSpace(entry.Path) != "",
		LocationKind:        workspaceLocationKind(entry.LocationKind),
		LocalName:           entry.LocalName,
		TemplateID:          entry.TemplateID,
		TemplateVersion:     entry.TemplateVersion,
		BackupEnabled:       entry.BackupEnabled,
		DefaultLocalName:    entry.LocalName,
		EffectivePermission: strings.TrimSpace(entry.EffectivePermission),
		CanMutateSource:     entry.CanMutateSource,
		PublicOwnerUID:      strings.TrimSpace(entry.PublicOwnerUID),
		BindingMode:         strings.TrimSpace(entry.BindingMode),
	}
	applySourcePermissionDefaults(source)
	applyBindingState(source, entry)
	return source
}

func sourceFromCloudWorkspace(workspace cloudWorkspace, entry workspaceIndexEntry) *Source {
	workspaceID := strings.TrimSpace(workspace.ID)
	if workspaceID == "" {
		return nil
	}
	source := &Source{
		SourceID:            workspaceID,
		WorkspaceID:         workspaceID,
		OrgID:               strings.TrimSpace(workspace.OrgID),
		BrainID:             "personal",
		Name:                cloudWorkspaceName(workspace),
		Path:                strings.TrimSpace(entry.Path),
		UpdatedAt:           strings.TrimSpace(workspace.UpdatedAt),
		Federated:           true,
		RemoteURL:           stringPtr(firstNonEmpty(workspace.StorageRemoteURL, workspace.RepoURL)),
		Openable:            strings.TrimSpace(entry.Path) != "",
		LocationKind:        workspaceLocationKind(entry.LocationKind),
		DisabledQueries:     workspace.DisabledQueries,
		PublicAccess:        workspace.PublicAccess,
		EffectivePermission: cloudWorkspaceEffectivePermission(workspace),
		CanMutateSource:     cloudWorkspaceCanMutate(workspace),
		PublicOwnerUID:      strings.TrimSpace(workspace.PublicOwnerUID),
		BindingMode:         cloudWorkspaceBindingMode(workspace),
	}
	applySourcePermissionDefaults(source)
	applyBindingState(source, entry)
	return source
}

func applySourcePermissionDefaults(source *Source) {
	if source == nil {
		return
	}
	if strings.TrimSpace(source.BindingMode) == "" {
		if strings.TrimSpace(source.PublicOwnerUID) != "" || strings.EqualFold(strings.TrimSpace(source.EffectivePermission), "read") {
			source.BindingMode = "granted"
		} else {
			source.BindingMode = "own"
		}
	}
	if strings.TrimSpace(source.EffectivePermission) == "" {
		if source.BindingMode == "granted" {
			source.EffectivePermission = "read"
		} else {
			source.EffectivePermission = "admin"
		}
	}
	if source.BindingMode == "granted" || strings.EqualFold(source.EffectivePermission, "read") {
		source.CanMutateSource = false
	} else if !source.CanMutateSource && cloudPermissionRank(source.EffectivePermission) >= cloudPermissionRank("write") {
		source.CanMutateSource = true
	}
}

func applyBindingState(source *Source, entry workspaceIndexEntry) {
	if source == nil {
		return
	}
	source.LastVerifiedAt = strings.TrimSpace(entry.LastVerifiedAt)
	source.LastVerifyReason = strings.TrimSpace(entry.LastVerifyReason)
	if strings.TrimSpace(entry.Path) == "" {
		source.Openable = false
		source.BindingStatus = sourceBindingNeedsBinding
		source.BindingReason = sourceBindingReasonUnbound
		return
	}
	reason := strings.TrimSpace(entry.LastVerifyReason)
	if reason == "" || reason == sourceBindingReasonConnected {
		source.Openable = true
		source.BindingStatus = sourceBindingConnected
		source.BindingReason = ""
		return
	}
	source.Openable = false
	source.BindingStatus = sourceBindingNeedsBinding
	source.BindingReason = reason
}

func (s *Service) verifyCloudWorkspaceBinding(ctx context.Context, workspace cloudWorkspace, entry workspaceIndexEntry, force bool) (workspaceIndexEntry, bool) {
	if strings.TrimSpace(entry.Path) == "" {
		return entry, false
	}
	if !force && cachedBindingVerificationFresh(entry) {
		return entry, false
	}
	reason := sourceBindingReasonConnected
	if err := assertExistingDirectory(entry.Path); err != nil {
		reason = sourceBindingReasonMoved
	} else {
		repo, err := inspectGitHubRepository(ctx, entry.Path)
		if err != nil || repo == nil || !cloudWorkspaceBindingRepositoryMatches(workspace, entry, *repo) {
			reason = sourceBindingReasonMismatch
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	oldVerifiedAt := strings.TrimSpace(entry.LastVerifiedAt)
	oldReason := strings.TrimSpace(entry.LastVerifyReason)
	if oldVerifiedAt == now && oldReason == reason {
		return entry, false
	}
	entry.LastVerifiedAt = now
	entry.LastVerifyReason = reason
	entry.UpdatedAt = now
	return entry, force || oldVerifiedAt != now || oldReason != reason
}

func cloudWorkspaceBindingRepositoryMatches(workspace cloudWorkspace, entry workspaceIndexEntry, repo githubRepoRef) bool {
	if cloudWorkspaceHasRepositoryIdentity(workspace) {
		return cloudWorkspaceRepositoryMatches(workspace, repo)
	}
	return workspaceIndexRepositoryMatchesRepo(entry, repo)
}

func cachedBindingVerificationFresh(entry workspaceIndexEntry) bool {
	checkedAt := strings.TrimSpace(entry.LastVerifiedAt)
	if checkedAt == "" {
		return false
	}
	if strings.TrimSpace(entry.LastVerifyReason) == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, checkedAt)
	if err != nil {
		return false
	}
	age := time.Since(parsed)
	return age >= 0 && age < bindingVerifyCacheWindow
}

func bindingReasonMessage(reason string) string {
	switch strings.TrimSpace(reason) {
	case sourceBindingReasonMoved:
		return "Local folder moved or missing."
	case sourceBindingReasonMismatch:
		return "Folder points to a different repository."
	case sourceBindingReasonUnbound:
		return "Bind a local folder for this repository."
	default:
		return "OpenBrain source is not bound on this runtime."
	}
}

func findWorkspaceIndexEntry(entries []workspaceIndexEntry, workspaceID string, orgID string) (workspaceIndexEntry, int) {
	workspaceID = strings.TrimSpace(workspaceID)
	orgID = strings.TrimSpace(orgID)
	for i, entry := range entries {
		if strings.TrimSpace(entry.WorkspaceID) != workspaceID {
			continue
		}
		if orgID != "" && strings.TrimSpace(entry.OrgID) != "" && strings.TrimSpace(entry.OrgID) != orgID {
			continue
		}
		return entry, i
	}
	return workspaceIndexEntry{}, -1
}

func defaultCloudSyncPolicy() map[string]interface{} {
	return map[string]interface{}{
		"autoSync":      true,
		"onOpen":        false,
		"onLocalChange": false,
		"intervalSec":   300,
		"conflict":      "keep-both",
		"deleteMode":    "trash",
	}
}

func syncPolicyFromStorage(storage map[string]interface{}) map[string]interface{} {
	if storage == nil {
		return nil
	}
	if policy, ok := storage["syncPolicy"].(map[string]interface{}); ok {
		return policy
	}
	return nil
}

func (index workspaceIndexFile) isHidden(workspaceID string, orgID string) bool {
	for _, entry := range index.HiddenWorkspaces {
		if strings.TrimSpace(entry.WorkspaceID) == strings.TrimSpace(workspaceID) &&
			(strings.TrimSpace(orgID) == "" || strings.TrimSpace(entry.OrgID) == "" || strings.TrimSpace(entry.OrgID) == strings.TrimSpace(orgID)) {
			return true
		}
	}
	return false
}

func appendHidden(entries []hiddenWorkspaceEntry, workspaceID string, orgID string, hiddenAt string) []hiddenWorkspaceEntry {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return entries
	}
	entries = filterHidden(entries, workspaceID, orgID)
	return append(entries, hiddenWorkspaceEntry{WorkspaceID: workspaceID, OrgID: strings.TrimSpace(orgID), HiddenAt: hiddenAt})
}

func filterHidden(entries []hiddenWorkspaceEntry, workspaceID string, orgID string) []hiddenWorkspaceEntry {
	next := entries[:0]
	for _, entry := range entries {
		if strings.TrimSpace(entry.WorkspaceID) == strings.TrimSpace(workspaceID) &&
			(strings.TrimSpace(orgID) == "" || strings.TrimSpace(entry.OrgID) == strings.TrimSpace(orgID)) {
			continue
		}
		next = append(next, entry)
	}
	return next
}

func parseGitHubRemoteURL(remote string) (string, string, bool) {
	value := strings.TrimSpace(remote)
	if value == "" {
		return "", "", false
	}
	if u, err := url.Parse(value); err == nil && (u.Scheme == "https" || u.Scheme == "http" || u.Scheme == "ssh") && strings.EqualFold(u.Host, "github.com") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 {
			return parts[0], strings.TrimSuffix(parts[1], ".git"), true
		}
	}
	if strings.HasPrefix(value, "git@github.com:") {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(value, "git@github.com:"), ".git"), "/")
		if len(parts) == 2 {
			return parts[0], parts[1], true
		}
	}
	return "", "", false
}

func normalizeGitRemote(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(value), "/"), ".git"))
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

func assertExistingDirectory(target string) error {
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("selected workspace directory does not exist: %s", target)
	}
	if !info.IsDir() {
		return fmt.Errorf("selected workspace path is not a directory: %s", target)
	}
	return nil
}

func isNothingToCommit(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "nothing to commit") || strings.Contains(text, "no changes added to commit")
}

func cleanPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return filepath.Clean(value)
	}
	return filepath.Clean(abs)
}

func workspaceLocationKind(value string) string {
	if strings.TrimSpace(value) == "remote" {
		return "remote"
	}
	return "local"
}

func firstUsableGitHubOwner(provider workspaceProviderOption) string {
	if !strings.EqualFold(strings.TrimSpace(provider.Provider), "github") {
		return ""
	}
	if provider.CanCreateRepository != nil && !*provider.CanCreateRepository {
		return ""
	}
	if provider.CanSyncRepository != nil && !*provider.CanSyncRepository {
		return ""
	}
	for _, account := range provider.Accounts {
		owner := strings.TrimSpace(account.Owner)
		if owner == "" {
			continue
		}
		if account.CanCreateRepository != nil && !*account.CanCreateRepository {
			continue
		}
		if account.CanSyncRepository != nil && !*account.CanSyncRepository {
			continue
		}
		return owner
	}
	return ""
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	return &value
}

func stringFromMap(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	if value, ok := values[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func dedupeCandidatePaths(values []string) []string {
	seen := map[string]bool{}
	paths := make([]string, 0, len(values))
	for _, raw := range values {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}
		cleaned := filepath.Clean(candidate)
		if cleaned == "." || seen[cleaned] {
			continue
		}
		seen[cleaned] = true
		paths = append(paths, cleaned)
	}
	return paths
}

func (r mutationRequest) WorkspaceIDOrSourceID() string {
	return firstNonEmpty(r.WorkspaceID, r.SourceID)
}

func (r createSourceRequest) WorkspaceIDOrSourceID() string {
	return firstNonEmpty(r.WorkspaceID, r.SourceID)
}

func (r recoveryCandidatesRequest) WorkspaceIDOrSourceID() string {
	return firstNonEmpty(r.WorkspaceID, r.SourceID)
}
