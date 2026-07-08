package protocol

type WorkspaceSyncPolicy struct {
	AutoSync      bool   `json:"autoSync"`
	OnOpen        bool   `json:"onOpen"`
	OnLocalChange bool   `json:"onLocalChange"`
	IntervalSec   int    `json:"intervalSec"`
	Conflict      string `json:"conflict"`
	DeleteMode    string `json:"deleteMode"`
}

type WorkspaceStorageBinding struct {
	Enabled     bool                `json:"enabled"`
	Backend     string              `json:"backend"`
	Provider    string              `json:"provider,omitempty"`
	Region      string              `json:"region,omitempty"`
	RemoteID    string              `json:"remoteID,omitempty"`
	RemoteName  string              `json:"remoteName,omitempty"`
	RemoteURL   string              `json:"remoteURL,omitempty"`
	ConnectedAs string              `json:"connectedAs,omitempty"`
	SyncPolicy  WorkspaceSyncPolicy `json:"syncPolicy"`
}

type StorageStatusParams struct {
	WorkspaceID   string `json:"workspaceID,omitempty"`
	Path          string `json:"path,omitempty"`
	ModelKey      string `json:"modelKey,omitempty"`
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
	ContextWindow int64  `json:"contextWindow,omitempty"`
	ServiceTier   string `json:"serviceTier,omitempty"`
}

type StorageSyncNowParams struct {
	WorkspaceID   string `json:"workspaceID,omitempty"`
	Path          string `json:"path,omitempty"`
	ModelKey      string `json:"modelKey,omitempty"`
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
	ContextWindow int64  `json:"contextWindow,omitempty"`
	ServiceTier   string `json:"serviceTier,omitempty"`
}

type StorageUpdatePolicyParams struct {
	WorkspaceID   string              `json:"workspaceID,omitempty"`
	Path          string              `json:"path,omitempty"`
	ModelKey      string              `json:"modelKey,omitempty"`
	ThinkingLevel string              `json:"thinkingLevel,omitempty"`
	ContextWindow int64               `json:"contextWindow,omitempty"`
	ServiceTier   string              `json:"serviceTier,omitempty"`
	Policy        WorkspaceSyncPolicy `json:"policy"`
}

type StorageStatusResult struct {
	WorkspaceID string                   `json:"workspaceID"`
	Path        string                   `json:"path"`
	Storage     *WorkspaceStorageBinding `json:"storage,omitempty"`
	Policy      WorkspaceSyncPolicy      `json:"policy"`
	Status      string                   `json:"status"`
	LastSyncAt  string                   `json:"lastSyncAt,omitempty"`
	LastError   string                   `json:"lastError,omitempty"`
	Message     string                   `json:"message,omitempty"`
	Error       string                   `json:"error,omitempty"`
}
