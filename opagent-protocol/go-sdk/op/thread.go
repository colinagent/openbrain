package op

import "encoding/json"

type ThreadHeader struct {
	Type              string `json:"type"`
	Version           int    `json:"version"`
	ID                string `json:"id"`
	Timestamp         string `json:"timestamp"`
	AgentID           string `json:"agentID"`
	CWD               string `json:"cwd"`
	ChatPath          string `json:"chatPath,omitempty"`
	FileID            string `json:"fileID,omitempty"`
	Title             string `json:"title"`
	ParentThreadID    string `json:"parentThreadID,omitempty"`
	PlanPath          string `json:"planPath,omitempty"`
	ExecutionPlanPath string `json:"executionPlanPath,omitempty"`
}

type ThreadEntryBase struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	ParentID  *string `json:"parentId"`
	Timestamp string  `json:"timestamp"`
}

const ThreadEntryTypeMetaUpdate = "thread_meta_update"

type ThreadMetaUpdateEntry struct {
	ThreadEntryBase
	Title             string `json:"title,omitempty"`
	ChatPath          string `json:"chatPath,omitempty"`
	FileID            string `json:"fileID,omitempty"`
	PlanPath          string `json:"planPath,omitempty"`
	ExecutionPlanPath string `json:"executionPlanPath,omitempty"`
}

type ThreadCompactionEntry struct {
	ThreadEntryBase
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"firstKeptEntryId"`
	TokensBefore     int64  `json:"tokensBefore,omitempty"`
}

type ThreadCreateParams struct {
	AgentID        string `json:"agentID"`
	CWD            string `json:"cwd,omitempty"`
	ChatPath       string `json:"chatPath,omitempty"`
	FileID         string `json:"fileID,omitempty"`
	Title          string `json:"title"`
	ParentThreadID string `json:"parentThreadID,omitempty"`
}

type ThreadCreateResult struct {
	ThreadID       string `json:"threadID"`
	FileID         string `json:"fileID,omitempty"`
	Title          string `json:"title"`
	CWD            string `json:"cwd,omitempty"`
	Path           string `json:"path,omitempty"`
	ChatPath       string `json:"chatPath,omitempty"`
	ThreadFilePath string `json:"threadFilePath,omitempty"`
}

type ThreadMetaQuery struct {
	ThreadID    string                  `json:"threadID,omitempty"`
	FileID      string                  `json:"fileID,omitempty"`
	ChatPath    string                  `json:"chatPath,omitempty"`
	AgentID     string                  `json:"agentID,omitempty"`
	EntryWindow *ThreadEntryWindowQuery `json:"entryWindow,omitempty"`
}

const (
	ThreadEntryWindowModeTail   = "tail"
	ThreadEntryWindowModeBefore = "before"
	ThreadEntryWindowModeAfter  = "after"
)

type ThreadEntryWindowQuery struct {
	Mode     string `json:"mode,omitempty"`
	AnchorID string `json:"anchorId,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type ThreadEntryWindow struct {
	Mode      string `json:"mode,omitempty"`
	AnchorID  string `json:"anchorId,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
	Total     int    `json:"total"`
	HasBefore bool   `json:"hasBefore"`
	HasAfter  bool   `json:"hasAfter"`
}

type ThreadMeta struct {
	ThreadID          string `json:"threadID"`
	FileID            string `json:"fileID,omitempty"`
	AgentID           string `json:"agentID"`
	CWD               string `json:"cwd"`
	Path              string `json:"path,omitempty"`
	ChatPath          string `json:"chatPath,omitempty"`
	ThreadFilePath    string `json:"threadFilePath,omitempty"`
	Title             string `json:"title"`
	ParentThreadID    string `json:"parentThreadID,omitempty"`
	PlanPath          string `json:"planPath,omitempty"`
	ExecutionPlanPath string `json:"executionPlanPath,omitempty"`
}

type ThreadMetaUpdateParams struct {
	ThreadID          string `json:"threadID,omitempty"`
	FileID            string `json:"fileID,omitempty"`
	ChatPath          string `json:"chatPath,omitempty"`
	Title             string `json:"title,omitempty"`
	PlanPath          string `json:"planPath,omitempty"`
	ExecutionPlanPath string `json:"executionPlanPath,omitempty"`
}

type ThreadForkParams struct {
	SourceThreadID    string `json:"sourceThreadID,omitempty"`
	SourceFileID      string `json:"sourceFileID,omitempty"`
	SourceChatPath    string `json:"sourceChatPath,omitempty"`
	AgentID           string `json:"agentID,omitempty"`
	CWD               string `json:"cwd,omitempty"`
	FileID            string `json:"fileID,omitempty"`
	ChatPath          string `json:"chatPath,omitempty"`
	Title             string `json:"title"`
	PlanPath          string `json:"planPath,omitempty"`
	ExecutionPlanPath string `json:"executionPlanPath,omitempty"`
}

type TurnResultToolResult struct {
	ToolName        string         `json:"toolName"`
	ArgumentsObject map[string]any `json:"argumentsObject,omitempty"`
	ResultText      string         `json:"resultText"`
	IsError         bool           `json:"isError,omitempty"`
}

type TurnResultPayload struct {
	ThreadID          string                 `json:"threadID"`
	FileID            string                 `json:"fileID,omitempty"`
	TurnID            string                 `json:"turnID"`
	AgentID           string                 `json:"agentID"`
	Path              string                 `json:"path,omitempty"`
	ChatPath          string                 `json:"chatPath,omitempty"`
	Title             string                 `json:"title"`
	ParentThreadID    string                 `json:"parentThreadID,omitempty"`
	PlanTurn          bool                   `json:"planTurn,omitempty"`
	UserMessage       Message                `json:"userMessage"`
	AssistantText     string                 `json:"assistantText,omitempty"`
	ReasoningText     string                 `json:"reasoningText,omitempty"`
	ToolResults       []TurnResultToolResult `json:"toolResults,omitempty"`
	CanonicalMessages json.RawMessage        `json:"canonicalMessages,omitempty"`
}
