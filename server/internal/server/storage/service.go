package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
)

type Service struct {
	homeDir string
}

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
	WorkspaceID     string                            `json:"workspaceID"`
	OrgID           string                            `json:"orgID,omitempty"`
	LocalName       string                            `json:"localName"`
	Path            string                            `json:"path"`
	LocationKind    string                            `json:"locationKind,omitempty"`
	TemplateID      string                            `json:"templateID,omitempty"`
	TemplateVersion int                               `json:"templateVersion,omitempty"`
	BackupEnabled   bool                              `json:"backupEnabled"`
	Repository      json.RawMessage                   `json:"repository,omitempty"`
	Storage         *protocol.WorkspaceStorageBinding `json:"storage,omitempty"`
	SyncPolicy      protocol.WorkspaceSyncPolicy      `json:"syncPolicy,omitempty"`
	CreatedAt       string                            `json:"createdAt"`
	UpdatedAt       string                            `json:"updatedAt"`
}

type WorkspaceCronBinding struct {
	WorkspaceID   string
	OrgID         string
	WorkspacePath string
	LocalName     string
	LocationKind  string
	RepoURL       string
	Branch        string
	Enabled       bool
	IntervalSec   int
}

const (
	providerGitHub    = "github"
	providerOpenBrain = "openbrain"
	backendOpenBrain  = "openbrain-cloud"
)

type repositoryBinding struct {
	RemoteURL     string `json:"remoteURL"`
	DefaultBranch string `json:"defaultBranch"`
}

func NewService() *Service {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = "."
	}
	return &Service{homeDir: home}
}

func (s *Service) Status(params protocol.StorageStatusParams) (*protocol.StorageStatusResult, error) {
	entry, err := s.findWorkspace(params.WorkspaceID, params.Path)
	if err != nil {
		return nil, err
	}
	status := "local"
	message := "This workspace has no remote storage binding."
	if entry.Storage != nil && entry.Storage.Enabled {
		status = "idle"
		message = "Remote storage is connected. Use Sync now to exchange regular files."
	}
	policy := entry.SyncPolicy
	if isZeroPolicy(policy) && entry.Storage != nil {
		policy = entry.Storage.SyncPolicy
	}
	if isZeroPolicy(policy) {
		policy = defaultPolicy(entry.BackupEnabled)
	}
	return &protocol.StorageStatusResult{
		WorkspaceID: entry.WorkspaceID,
		Path:        entry.Path,
		Storage:     entry.Storage,
		Policy:      policy,
		Status:      status,
		Message:     message,
	}, nil
}

func (s *Service) CronBindings() ([]WorkspaceCronBinding, error) {
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	bindings := make([]WorkspaceCronBinding, 0, len(index.Workspaces))
	for _, entry := range index.Workspaces {
		normalizeWorkspaceEntryStorage(&entry)
		binding, ok := workspaceCronBinding(entry)
		if !ok {
			continue
		}
		bindings = append(bindings, binding)
	}
	sort.SliceStable(bindings, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(bindings[i].LocalName))
		right := strings.ToLower(strings.TrimSpace(bindings[j].LocalName))
		if left == right {
			return strings.TrimSpace(bindings[i].WorkspaceID) < strings.TrimSpace(bindings[j].WorkspaceID)
		}
		return left < right
	})
	return bindings, nil
}

func (s *Service) CurrentAccountUID() string {
	uid, err := s.loadAuthUID()
	if err != nil {
		return ""
	}
	return uid
}

func (s *Service) CronBinding(params protocol.StorageStatusParams) (*WorkspaceCronBinding, error) {
	entry, err := s.findWorkspace(params.WorkspaceID, params.Path)
	if err != nil {
		return nil, err
	}
	binding, ok := workspaceCronBinding(*entry)
	if !ok {
		return nil, nil
	}
	return &binding, nil
}

func workspaceCronBinding(entry workspaceEntry) (WorkspaceCronBinding, bool) {
	if entry.Storage == nil || !entry.Storage.Enabled {
		return WorkspaceCronBinding{}, false
	}
	if strings.TrimSpace(entry.Storage.Backend) != "git" || strings.TrimSpace(entry.Storage.Provider) != providerGitHub {
		return WorkspaceCronBinding{}, false
	}
	repo := repositoryBinding{}
	if len(entry.Repository) > 0 {
		_ = json.Unmarshal(entry.Repository, &repo)
	}
	remoteURL := strings.TrimSpace(entry.Storage.RemoteURL)
	if remoteURL == "" {
		remoteURL = strings.TrimSpace(repo.RemoteURL)
	}
	branch := strings.TrimSpace(repo.DefaultBranch)
	if branch == "" {
		branch = "main"
	}
	policy := entry.SyncPolicy
	if isZeroPolicy(policy) && entry.Storage != nil {
		policy = entry.Storage.SyncPolicy
	}
	if isZeroPolicy(policy) {
		policy = defaultPolicy(entry.BackupEnabled)
	}
	return WorkspaceCronBinding{
		WorkspaceID:   strings.TrimSpace(entry.WorkspaceID),
		OrgID:         strings.TrimSpace(entry.OrgID),
		WorkspacePath: strings.TrimSpace(entry.Path),
		LocalName:     strings.TrimSpace(entry.LocalName),
		LocationKind:  workspaceLocationKind(entry.LocationKind),
		RepoURL:       remoteURL,
		Branch:        branch,
		Enabled:       policy.AutoSync,
		IntervalSec:   policy.IntervalSec,
	}, true
}

func workspaceLocationKind(value string) string {
	switch strings.TrimSpace(value) {
	case "remote":
		return "remote"
	default:
		return "local"
	}
}

func (s *Service) UpdatePolicy(params protocol.StorageUpdatePolicyParams) (*protocol.StorageStatusResult, error) {
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	i := findWorkspaceIndex(index.Workspaces, params.WorkspaceID, params.Path)
	if i < 0 {
		return nil, errors.New("workspace not found")
	}
	entry := index.Workspaces[i]
	normalizeWorkspaceEntryStorage(&entry)
	policy := normalizePolicy(params.Policy, entry.BackupEnabled)
	entry.SyncPolicy = policy
	if entry.Storage != nil {
		entry.Storage.SyncPolicy = policy
	}
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	index.Workspaces[i] = entry
	if err := s.saveIndex(index); err != nil {
		return nil, err
	}
	return s.Status(protocol.StorageStatusParams{WorkspaceID: entry.WorkspaceID, Path: entry.Path})
}

func (s *Service) findWorkspace(workspaceID string, path string) (*workspaceEntry, error) {
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	i := findWorkspaceIndex(index.Workspaces, workspaceID, path)
	if i < 0 {
		return nil, errors.New("workspace not found")
	}
	entry := index.Workspaces[i]
	normalizeWorkspaceEntryStorage(&entry)
	return &entry, nil
}

func findWorkspaceIndex(workspaces []workspaceEntry, workspaceID string, path string) int {
	workspaceID = strings.TrimSpace(workspaceID)
	path = cleanPath(path)
	for i, entry := range workspaces {
		if workspaceID != "" && entry.WorkspaceID == workspaceID {
			return i
		}
		if path != "" && cleanPath(entry.Path) == path {
			return i
		}
	}
	return -1
}

func normalizeWorkspaceEntryStorage(entry *workspaceEntry) {
	if entry == nil || strings.TrimSpace(entry.TemplateID) != "openbrain-cloud" {
		return
	}
	if entry.Storage != nil && entry.Storage.Enabled && strings.TrimSpace(entry.Storage.Backend) != "" {
		return
	}
	policy := entry.SyncPolicy
	if isZeroPolicy(policy) {
		policy = defaultPolicy(true)
	}
	entry.Storage = &protocol.WorkspaceStorageBinding{
		Enabled:    true,
		Backend:    "openbrain-cloud",
		Provider:   "openbrain",
		RemoteID:   strings.TrimSpace(entry.WorkspaceID),
		RemoteName: strings.TrimSpace(entry.LocalName),
		SyncPolicy: policy,
	}
	entry.SyncPolicy = policy
}

func (s *Service) loadIndex() (*workspaceIndexFile, error) {
	uid, _ := s.loadAuthUID()
	raw, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			index := &workspaceIndexFile{Version: 2, Accounts: map[string]*workspaceIndexAccount{}, ActiveUID: uid}
			index.activate(uid)
			return index, nil
		}
		return nil, err
	}
	var index workspaceIndexFile
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, err
	}
	if index.Version != 2 || index.Accounts == nil || strings.TrimSpace(uid) == "" {
		index = workspaceIndexFile{Version: 2, Accounts: map[string]*workspaceIndexAccount{}, ActiveUID: uid}
		index.activate(uid)
		return &index, nil
	}
	index.activate(uid)
	return &index, nil
}

func (s *Service) saveIndex(index *workspaceIndexFile) error {
	if index == nil {
		index = &workspaceIndexFile{Version: 2}
	}
	index.Version = 2
	if strings.TrimSpace(index.ActiveUID) == "" {
		index.ActiveUID, _ = s.loadAuthUID()
	}
	if index.Accounts == nil {
		index.Accounts = map[string]*workspaceIndexAccount{}
	}
	if uid := strings.TrimSpace(index.ActiveUID); uid != "" {
		account := index.ensureAccount(uid)
		account.Workspaces = sortedWorkspaceEntries(index.Workspaces)
	}
	for _, account := range index.Accounts {
		if account == nil {
			continue
		}
		account.Workspaces = sortedWorkspaceEntries(account.Workspaces)
	}
	index.Workspaces = nil
	return writeJSONAtomic(s.indexPath(), index)
}

func (s *Service) loadAuthUID() (string, error) {
	raw, err := os.ReadFile(filepath.Join(s.homeDir, ".openbrain", "configs", "user", "auth.json"))
	if err != nil {
		return "", err
	}
	var auth struct {
		UID   string `json:"uid"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &auth); err != nil {
		return "", err
	}
	if strings.TrimSpace(auth.UID) == "" || strings.TrimSpace(auth.Token) == "" {
		return "", errors.New("auth required")
	}
	return strings.TrimSpace(auth.UID), nil
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
	account := index.ensureAccount(uid)
	index.Workspaces = append([]workspaceEntry(nil), account.Workspaces...)
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

func sortedWorkspaceEntries(entries []workspaceEntry) []workspaceEntry {
	next := append([]workspaceEntry(nil), entries...)
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

func (s *Service) indexPath() string {
	return filepath.Join(s.homeDir, ".openbrain", "index", "workspaces.json")
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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

func isZeroPolicy(policy protocol.WorkspaceSyncPolicy) bool {
	return !policy.AutoSync && !policy.OnOpen && !policy.OnLocalChange && policy.IntervalSec == 0 && policy.Conflict == "" && policy.DeleteMode == ""
}

func normalizePolicy(policy protocol.WorkspaceSyncPolicy, enabled bool) protocol.WorkspaceSyncPolicy {
	if isZeroPolicy(policy) {
		return defaultPolicy(enabled)
	}
	if policy.IntervalSec <= 0 {
		policy.IntervalSec = 300
	}
	if policy.Conflict == "" {
		policy.Conflict = "keep-both"
	}
	if policy.DeleteMode == "" {
		policy.DeleteMode = "trash"
	}
	return policy
}

func defaultPolicy(enabled bool) protocol.WorkspaceSyncPolicy {
	return protocol.WorkspaceSyncPolicy{
		AutoSync:      enabled,
		OnOpen:        false,
		OnLocalChange: false,
		IntervalSec:   300,
		Conflict:      "keep-both",
		DeleteMode:    "trash",
	}
}
