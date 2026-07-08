package protocol

import (
	"encoding/json"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

// JSON-RPC 2.0 request/response structures

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"` // string or number
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Notification (no ID, no response expected)
type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Error codes (JSON-RPC 2.0 + custom)
const (
	ErrCodeParse          = -32700 // Invalid JSON
	ErrCodeInvalidRequest = -32600 // Invalid Request
	ErrCodeMethodNotFound = -32601 // Method not found
	ErrCodeInvalidParams  = -32602 // Invalid params
	ErrCodeInternal       = -32603 // Internal error

	// Custom error codes
	ErrCodeFileNotFound      = -32001
	ErrCodeFileExists        = -32002
	ErrCodePermissionDeny    = -32003
	ErrCodeFileTooLarge      = -32004
	ErrCodeIsDirectory       = -32005
	ErrCodeNotDirectory      = -32006
	ErrCodeDirectoryNotEmpty = -32007
)

// File system method names
const (
	MethodFSStat           = "fs/stat"
	MethodFSReadFile       = "fs/readFile"
	MethodFSReadFileStream = "fs/readFileStream"
	MethodFSWriteFile      = "fs/writeFile"
	MethodFSReaddir        = "fs/readdir"
	MethodFSSearch         = "fs/search"
	MethodFSMkdir          = "fs/mkdir"
	MethodFSDelete         = "fs/delete"
	MethodFSRename         = "fs/rename"
	MethodFSCopy           = "fs/copy"
	MethodFSWatch          = "fs/watch"
	MethodFSUnwatch        = "fs/unwatch"
)

// Agent method names
const (
	// MethodAgentGet  = "agent/get"
	MethodAgentScan = string(op.OpAgentScan)
	MethodNodeList  = string(op.OpNodeList)
	// MethodNodesCached = "nodes/cached"
	MethodConfigSystemGet = string(op.ConfigSystemGet)
)

// Session method names
// const (
// 	MethodSessionSet   = "thread/set"
// 	MethodSessionClear = "thread/clear"
// 	MethodSessionGet   = "thread/get"
// )

// Config sync method names
const (
	MethodConfigPush = "config/push"
)

const (
	MethodThreadReviewList     = string(op.OpThreadReviewList)
	MethodThreadReviewResolve  = string(op.OpThreadReviewResolve)
	MethodThreadReviewRollback = string(op.OpThreadReviewRollback)
)

const (
	MethodEditorCompletion       = string(op.OpEditorCompletion)
	MethodEditorCompletionCancel = string(op.OpEditorCompletionCancel)
	MethodEditorRandomID         = "editor/randomID"
)

const (
	MethodCommandExec = "command/exec"
	MethodCommandStop = "command/stop"
)

const (
	MethodArchiveCleanupRun = "archive/cleanup/run"
)

const (
	MethodCronList    = "cron/list"
	MethodCronGet     = "cron/get"
	MethodCronAdd     = "cron/add"
	MethodCronUpsert  = "cron/upsert"
	MethodCronUpdate  = "cron/update"
	MethodCronRemove  = "cron/remove"
	MethodCronRun     = "cron/run"
	MethodCronHistory = "cron/history"
)

const (
	MethodMarketplaceList    = "marketplace/list"
	MethodMarketplaceRefresh = "marketplace/refresh"
	MethodMarketplaceState   = "marketplace/state"
	MethodMarketplaceInstall = "marketplace/install"
	MethodMarketplaceUpdate  = "marketplace/update"
	MethodMarketplaceOrgs    = "marketplace/orgs"
)

const (
	MethodStorageStatus       = "storage/status"
	MethodStorageSyncNow      = "storage/syncNow"
	MethodStorageUpdatePolicy = "storage/updatePolicy"
)

const (
	MethodMessengerList     = "messenger/list"
	MethodMessengerChannel  = "messenger/channel"
	MethodMessengerReply    = "messenger/reply"
	MethodMessengerMarkRead = "messenger/markRead"
	MethodMessengerArchive  = "messenger/archive"
)

// File change event types
const (
	FileChangeCreated = "created"
	FileChangeChanged = "changed"
	FileChangeDeleted = "deleted"
)

// --- Request/Response params ---

// StatParams for fs/stat
type StatParams struct {
	Path string `json:"path"`
}

// StatResult from fs/stat
type StatResult struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime int64  `json:"modTime"` // Unix timestamp ms
	Mode    uint32 `json:"mode"`
}

// ReadFileParams for fs/readFile
type ReadFileParams struct {
	Path    string           `json:"path"`
	Options *ReadFileOptions `json:"options,omitempty"`
}

type ReadFileOptions struct {
	Encoding string      `json:"encoding,omitempty"` // "utf8" or "base64", default utf8
	Limits   *FileLimits `json:"limits,omitempty"`
}

type FileLimits struct {
	Size int64 `json:"size"` // max file size in bytes
}

// ReadFileResult from fs/readFile
type ReadFileResult struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"` // "utf8" or "base64"
	Size     int64  `json:"size"`
	TooLarge bool   `json:"tooLarge,omitempty"`
}

// WriteFileParams for fs/writeFile
type WriteFileParams struct {
	Path    string            `json:"path"`
	Content string            `json:"content"`
	Options *WriteFileOptions `json:"options,omitempty"`
}

type WriteFileOptions struct {
	Encoding  string `json:"encoding,omitempty"`  // "utf8" or "base64"
	Create    bool   `json:"create,omitempty"`    // create if not exists
	Overwrite bool   `json:"overwrite,omitempty"` // overwrite if exists
	Atomic    bool   `json:"atomic,omitempty"`    // atomic write (write to temp then rename)
}

// WriteFileResult from fs/writeFile
type WriteFileResult struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
}

// ReaddirParams for fs/readdir
type ReaddirParams struct {
	Path string `json:"path"`
}

// ReaddirResult from fs/readdir
type ReaddirResult struct {
	Path    string     `json:"path"`
	Entries []DirEntry `json:"entries"`
}

type DirEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
}

type SearchParams struct {
	Root       string   `json:"root"`
	Query      string   `json:"query"`
	Regex      bool     `json:"regex,omitempty"`
	MatchCase  bool     `json:"matchCase,omitempty"`
	WholeWord  bool     `json:"wholeWord,omitempty"`
	Includes   []string `json:"includes,omitempty"`
	Excludes   []string `json:"excludes,omitempty"`
	MaxFiles   int      `json:"maxFiles,omitempty"`
	MaxMatches int      `json:"maxMatches,omitempty"`
}

type SearchMatch struct {
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndColumn int    `json:"endColumn"`
	Text      string `json:"text"`
}

type SearchFileResult struct {
	Path    string        `json:"path"`
	Matches []SearchMatch `json:"matches"`
	Count   int           `json:"count"`
}

type SearchResult struct {
	Files      []SearchFileResult `json:"files"`
	TotalCount int                `json:"totalCount"`
	Truncated  bool               `json:"truncated"`
}

type EditorRandomIDResult struct {
	ID string `json:"id"`
}

type CronTaskSchedule struct {
	Cron  string `json:"cron,omitempty"`
	Every string `json:"every,omitempty"`
	Time  string `json:"time,omitempty"`
}

type CronTaskTarget struct {
	Kind    string `json:"kind"`
	AgentID string `json:"agentID"`
	CWD     string `json:"cwd"`
}

type CronTaskPayload struct {
	Kind string                 `json:"kind"`
	Text string                 `json:"text,omitempty"`
	Data map[string]interface{} `json:"data,omitempty"`
}

type CronTask struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Enabled     bool             `json:"enabled"`
	Schedule    CronTaskSchedule `json:"schedule"`
	Target      CronTaskTarget   `json:"target"`
	Payload     CronTaskPayload  `json:"payload"`
	CreatedAtMs int64            `json:"createdAtMs,omitempty"`
	UpdatedAtMs int64            `json:"updatedAtMs,omitempty"`
}

func (t *CronTask) UnmarshalJSON(data []byte) error {
	type taskAlias CronTask
	wire := struct {
		taskAlias
		Enabled *bool `json:"enabled"`
	}{
		taskAlias: taskAlias{Enabled: true},
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	task := CronTask(wire.taskAlias)
	if wire.Enabled != nil {
		task.Enabled = *wire.Enabled
	}
	*t = task
	return nil
}

type CronTaskState struct {
	TaskID            string `json:"taskID"`
	SpecHash          string `json:"specHash,omitempty"`
	NextRunAtMs       int64  `json:"nextRunAtMs,omitempty"`
	RunNowAtMs        int64  `json:"runNowAtMs,omitempty"`
	LastRunAtMs       int64  `json:"lastRunAtMs,omitempty"`
	RunningAtMs       int64  `json:"runningAtMs,omitempty"`
	LastError         string `json:"lastError,omitempty"`
	ConsecutiveErrors int    `json:"consecutiveErrors,omitempty"`
}

type CronTaskRecord struct {
	Task  CronTask       `json:"task"`
	State *CronTaskState `json:"state,omitempty"`
}

type CronListParams struct{}

type CronListResult struct {
	Version int              `json:"version"`
	Tasks   []CronTaskRecord `json:"tasks"`
}

type CronIDParams struct {
	ID      string           `json:"id"`
	Payload *CronTaskPayload `json:"payload,omitempty"`
}

type CronHistoryParams struct {
	ID    string `json:"id"`
	Limit int    `json:"limit,omitempty"`
}

type CronTaskWriteParams struct {
	Task CronTask `json:"task"`
}

type CronTaskUpsertParams struct {
	Task CronTask `json:"task"`
}

type CronRemoveResult struct {
	ID      string `json:"id"`
	Removed bool   `json:"removed"`
}

type CronRunResult struct {
	Queued bool           `json:"queued"`
	Task   CronTaskRecord `json:"task"`
}

type CronTaskRunHistoryEntry struct {
	RunID         string `json:"runID"`
	TaskID        string `json:"taskID"`
	Trigger       string `json:"trigger"`
	ScheduledAtMs int64  `json:"scheduledAtMs,omitempty"`
	StartedAtMs   int64  `json:"startedAtMs"`
	FinishedAtMs  int64  `json:"finishedAtMs,omitempty"`
	DurationMs    int64  `json:"durationMs,omitempty"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
	ThreadID      string `json:"threadID,omitempty"`
	ChatPath      string `json:"chatPath,omitempty"`
	AgentID       string `json:"agentID,omitempty"`
}

type CronTaskHistoryResult struct {
	TaskID string                    `json:"taskID"`
	Limit  int                       `json:"limit"`
	Runs   []CronTaskRunHistoryEntry `json:"runs"`
}

// MkdirParams for fs/mkdir
type MkdirParams struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

// DeleteParams for fs/delete
type DeleteParams struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
	UseTrash  bool   `json:"useTrash,omitempty"`
}

// RenameParams for fs/rename
type RenameParams struct {
	OldPath   string `json:"oldPath"`
	NewPath   string `json:"newPath"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

// CopyParams for fs/copy
type CopyParams struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

// WatchParams for fs/watch
type WatchParams struct {
	Path      string   `json:"path"`
	Recursive bool     `json:"recursive,omitempty"`
	Excludes  []string `json:"excludes,omitempty"`
}

// WatchResult from fs/watch
type WatchResult struct {
	WatchID string `json:"watchId"`
}

// UnwatchParams for fs/unwatch
type UnwatchParams struct {
	WatchID string `json:"watchId"`
}

// FileChangeEvent notification params
type FileChangeEvent struct {
	WatchID string       `json:"watchId"`
	Changes []FileChange `json:"changes"`
}

type FileChange struct {
	Type string `json:"type"` // created, changed, deleted
	Path string `json:"path"`
}

type CommandExecParams struct {
	Command       string `json:"command"`
	WorkspaceRoot string `json:"workspaceRoot"`
	TargetPath    string `json:"targetPath,omitempty"`
}

type CommandExecResult struct {
	CommandID     string `json:"commandID"`
	FilePath      string `json:"filePath"`
	WorkspaceRoot string `json:"workspaceRoot"`
	Created       bool   `json:"created"`
}

type CommandStopParams struct {
	CommandID string `json:"commandID"`
}

type CommandStopResult struct {
	OK bool `json:"ok"`
}

type CommandStateEvent struct {
	CommandID string `json:"commandID"`
	FilePath  string `json:"filePath"`
	State     string `json:"state"`
	ExitCode  *int   `json:"exitCode,omitempty"`
	Error     string `json:"error,omitempty"`
}

type ArchiveCleanupParams struct {
	WorkspaceRoots []string `json:"workspaceRoots"`
	OpenFilePaths  []string `json:"openFilePaths"`
}

type ArchiveCleanupResult struct {
	MovedChats            int      `json:"movedChats"`
	MovedPlans            int      `json:"movedPlans"`
	MovedThreads          int      `json:"movedThreads"`
	RewrittenThreads      int      `json:"rewrittenThreads"`
	RewrittenChats        int      `json:"rewrittenChats"`
	RolledIntoMonthlyDirs int      `json:"rolledIntoMonthlyDirs"`
	CompressedArchives    int      `json:"compressedArchives"`
	PrunedArchives        int      `json:"prunedArchives"`
	SkippedOpenFiles      int      `json:"skippedOpenFiles"`
	SkippedActiveThreads  int      `json:"skippedActiveThreads"`
	Errors                []string `json:"errors,omitempty"`
}

type MarketplaceUsageReport struct {
	Agents []string `json:"agents,omitempty"`
	Skills []string `json:"skills,omitempty"`
	Tools  []string `json:"tools,omitempty"`
}

type MarketplaceListParams struct {
	Force bool                   `json:"force,omitempty"`
	OrgID string                 `json:"orgID,omitempty"`
	Usage MarketplaceUsageReport `json:"usage,omitempty"`
}

type MarketplaceItemParams struct {
	Kind  string                 `json:"kind"`
	ID    string                 `json:"id"`
	OrgID string                 `json:"orgID,omitempty"`
	Usage MarketplaceUsageReport `json:"usage,omitempty"`
}

// Helper functions

func NewResponse(id interface{}, result interface{}) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func NewErrorResponse(id interface{}, code int, message string, data interface{}) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

func NewNotification(method string, params interface{}) *Notification {
	return &Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
}

// --- Agent request/response params ---

// AgentGetParams for agent/get
type AgentGetParams struct {
	Meta map[string]interface{} `json:"meta"`
}

// DirNodeScanParams for dir/nodescan
type DirAgentScanParams struct {
	Dir string `json:"dir"`
}

// DirNodeScanResult from dir/nodescan
type DirNodeScanResult struct {
	Items []map[string]interface{} `json:"items"`
}

// NodesParams for nodes
// type NodeListParams struct {
// 	Refresh *bool `json:"refresh,omitempty"`
// }

// NodeListResult from node/list
type NodeListResult struct {
	Nodes []*op.OpNode `json:"nodes"`
}

// AgentsRootsResult from agents/roots
type AgentsRootsResult struct {
	Roots []string `json:"roots"`
}

// --- Session request/response params ---

// AuthConfig mirrors OpAgent's ~/.openbrain/configs/auth.json
type AuthConfig struct {
	Version       int    `json:"version"`
	BaseUrl       string `json:"baseUrl"`
	Gateway       string `json:"gateway"`
	AIGateway     string `json:"aiGateway,omitempty"`
	Token         string `json:"token"`
	UID           string `json:"uid"`
	Email         string `json:"email,omitempty"`
	ActiveOrgID   string `json:"activeOrgID,omitempty"`
	ActiveOrgName string `json:"activeOrgName,omitempty"`
	UpdatedAt     int64  `json:"updatedAt"`
}

// UserProfile mirrors OpAgent's ~/.openbrain/settings/profile.json
type UserProfile struct {
	Version   int    `json:"version"`
	UID       string `json:"uid"`
	Username  string `json:"username"`
	Email     string `json:"email,omitempty"`
	Avatar    string `json:"avatar,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Address   string `json:"address,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}

// SessionSetParams for thread/set
type SessionSetParams struct {
	Auth    *AuthConfig  `json:"auth"`
	Profile *UserProfile `json:"profile,omitempty"`
}

// SessionSetResult from thread/set
type SessionSetResult struct {
	OK          bool   `json:"ok"`
	AuthPath    string `json:"authPath"`
	ProfilePath string `json:"profilePath"`
	WrittenAt   int64  `json:"writtenAt"`
}

// SessionClearParams for thread/clear
type SessionClearParams struct {
}

// SessionGetResult from thread/get
type SessionGetResult struct {
	Auth    *AuthConfig  `json:"auth,omitempty"`
	Profile *UserProfile `json:"profile,omitempty"`
}

// --- Config sync request/response params ---

type ConfigPushFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type ConfigPushParams struct {
	Files []ConfigPushFile `json:"files"`
}

type ConfigPushResult struct {
	Written int `json:"written"`
}
