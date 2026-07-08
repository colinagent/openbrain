package gbrain

type Source struct {
	SourceID            string  `json:"sourceID"`
	Name                string  `json:"name"`
	Path                string  `json:"path,omitempty"`
	WorkspaceID         string  `json:"workspaceID,omitempty"`
	OrgID               string  `json:"orgID,omitempty"`
	BrainID             string  `json:"brainID,omitempty"`
	UpdatedAt           string  `json:"updatedAt,omitempty"`
	PageCount           *int    `json:"pageCount,omitempty"`
	Federated           bool    `json:"federated"`
	RemoteURL           *string `json:"remoteURL"`
	Openable            bool    `json:"openable"`
	LocationKind        string  `json:"locationKind,omitempty"`
	LocalName           string  `json:"localName,omitempty"`
	TemplateID          string  `json:"templateID,omitempty"`
	TemplateVersion     int     `json:"templateVersion,omitempty"`
	BackupEnabled       bool    `json:"backupEnabled,omitempty"`
	DefaultLocalName    string  `json:"defaultLocalName,omitempty"`
	DisabledQueries     bool    `json:"disabledQueries,omitempty"`
	PublicAccess        bool    `json:"publicAccess,omitempty"`
	EffectivePermission string  `json:"effectivePermission,omitempty"`
	CanMutateSource     bool    `json:"canMutateSource,omitempty"`
	PublicOwnerUID      string  `json:"publicOwnerUID,omitempty"`
	BindingMode         string  `json:"bindingMode,omitempty"`
	BindingStatus       string  `json:"bindingStatus,omitempty"`
	BindingReason       string  `json:"bindingReason,omitempty"`
	LastVerifiedAt      string  `json:"lastVerifiedAt,omitempty"`
	LastVerifyReason    string  `json:"lastVerifyReason,omitempty"`
}

type ListSourcesResponse struct {
	Success      bool     `json:"success"`
	Code         string   `json:"code,omitempty"`
	Error        string   `json:"error,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	AuthRequired bool     `json:"authRequired,omitempty"`
	Sources      []Source `json:"sources"`
}

type QueryRequest struct {
	BrainID        string `json:"brainID,omitempty"`
	Scope          string `json:"scope,omitempty"`
	WorkspaceID    string `json:"workspaceID,omitempty"`
	OrgID          string `json:"orgID,omitempty"`
	PublicOwnerUID string `json:"publicOwnerUID,omitempty"`
	Query          string `json:"query,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

type QueryResult struct {
	ChunkID       string  `json:"chunkID"`
	WorkspaceID   string  `json:"workspaceID"`
	WorkspaceName string  `json:"workspaceName"`
	Path          string  `json:"path,omitempty"`
	RelativePath  string  `json:"relativePath"`
	Title         string  `json:"title"`
	Text          string  `json:"text"`
	Score         float64 `json:"score"`
}

type QueryResponse struct {
	Success      bool          `json:"success"`
	Code         string        `json:"code,omitempty"`
	Error        string        `json:"error,omitempty"`
	Provider     string        `json:"provider,omitempty"`
	AuthRequired bool          `json:"authRequired,omitempty"`
	Results      []QueryResult `json:"results"`
}

type StatusResponse struct {
	Success bool                   `json:"success"`
	Code    string                 `json:"code,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Status  map[string]interface{} `json:"status,omitempty"`
}

type CreateSourceResponse struct {
	Success          bool    `json:"success"`
	Code             string  `json:"code,omitempty"`
	Error            string  `json:"error,omitempty"`
	Provider         string  `json:"provider,omitempty"`
	AuthRequired     bool    `json:"authRequired,omitempty"`
	Workspace        *Source `json:"workspace,omitempty"`
	PathOwnerUID     string  `json:"pathOwnerUID,omitempty"`
	RequestID        string  `json:"requestID,omitempty"`
	CleanupAttempted bool    `json:"cleanupAttempted,omitempty"`
	CleanupSucceeded bool    `json:"cleanupSucceeded,omitempty"`
	CleanupError     string  `json:"cleanupError,omitempty"`
}

type VerifySourceResponse struct {
	Success      bool    `json:"success"`
	Code         string  `json:"code,omitempty"`
	Error        string  `json:"error,omitempty"`
	Provider     string  `json:"provider,omitempty"`
	AuthRequired bool    `json:"authRequired,omitempty"`
	Workspace    *Source `json:"workspace,omitempty"`
}

type RecoveryCandidate struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

type RecoveryCandidatesResponse struct {
	Success      bool                `json:"success"`
	Code         string              `json:"code,omitempty"`
	Error        string              `json:"error,omitempty"`
	Provider     string              `json:"provider,omitempty"`
	AuthRequired bool                `json:"authRequired,omitempty"`
	Candidates   []RecoveryCandidate `json:"candidates"`
}

type MutationResponse struct {
	Success         bool   `json:"success"`
	Code            string `json:"code,omitempty"`
	Error           string `json:"error,omitempty"`
	Provider        string `json:"provider,omitempty"`
	AuthRequired    bool   `json:"authRequired,omitempty"`
	SourceID        string `json:"sourceID,omitempty"`
	WorkspaceID     string `json:"workspaceID,omitempty"`
	OrgID           string `json:"orgID,omitempty"`
	DisabledQueries bool   `json:"disabledQueries,omitempty"`
	EnabledQueries  bool   `json:"enabledQueries,omitempty"`
	DisabledSync    bool   `json:"disabledSync,omitempty"`
	HardDeleted     bool   `json:"hardDeleted,omitempty"`
	SyncJobsRemoved int64  `json:"syncJobsRemoved,omitempty"`
	Status          string `json:"status,omitempty"`
}
