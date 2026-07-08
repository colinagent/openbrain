package op

type ThreadReviewFileStatus string

const (
	ThreadReviewFilePending    ThreadReviewFileStatus = "pending"
	ThreadReviewFileApproved   ThreadReviewFileStatus = "approved"
	ThreadReviewFileRejected   ThreadReviewFileStatus = "rejected"
	ThreadReviewFileRolledBack ThreadReviewFileStatus = "rolledBack"
)

type ThreadReviewTurnStatus string

const (
	ThreadReviewTurnPending    ThreadReviewTurnStatus = "pending"
	ThreadReviewTurnResolved   ThreadReviewTurnStatus = "resolved"
	ThreadReviewTurnRolledBack ThreadReviewTurnStatus = "rolledBack"
)

type ThreadReviewMergeState string

const (
	ThreadReviewMergeClean      ThreadReviewMergeState = "clean"
	ThreadReviewMergeUserEdited ThreadReviewMergeState = "userEdited"
	ThreadReviewMergeUserUndone ThreadReviewMergeState = "userUndone"
	ThreadReviewMergeConflicted ThreadReviewMergeState = "conflicted"
	ThreadReviewMergeMissing    ThreadReviewMergeState = "missing"
)

type ThreadReviewDecision string

const (
	ThreadReviewDecisionApprove    ThreadReviewDecision = "approve"
	ThreadReviewDecisionReject     ThreadReviewDecision = "reject"
	ThreadReviewDecisionApproveAll ThreadReviewDecision = "approveAll"
	ThreadReviewDecisionRejectAll  ThreadReviewDecision = "rejectAll"
)

type ThreadReviewRollbackScope string

const (
	ThreadReviewRollbackFile ThreadReviewRollbackScope = "file"
	ThreadReviewRollbackTurn ThreadReviewRollbackScope = "turn"
)

type ThreadReviewLineRange struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

type ThreadReviewHunk struct {
	OldStartLine int      `json:"oldStartLine"`
	OldLineCount int      `json:"oldLineCount"`
	NewStartLine int      `json:"newStartLine"`
	NewLineCount int      `json:"newLineCount"`
	RemovedLines []string `json:"removedLines,omitempty"`
	AddedLines   []string `json:"addedLines,omitempty"`
}

type ThreadReviewFile struct {
	Path               string                  `json:"path"`
	Status             ThreadReviewFileStatus  `json:"status"`
	MergeState         ThreadReviewMergeState  `json:"mergeState,omitempty"`
	HasUserEdits       bool                    `json:"hasUserEdits,omitempty"`
	CanUndo            bool                    `json:"canUndo,omitempty"`
	ConflictMessage    string                  `json:"conflictMessage,omitempty"`
	Diff               string                  `json:"diff"`
	BaselineExists     bool                    `json:"baselineExists"`
	FirstChangedLine   int                     `json:"firstChangedLine,omitempty"`
	FirstChangedColumn int                     `json:"firstChangedColumn,omitempty"`
	LineCount          int                     `json:"lineCount,omitempty"`
	ChangedRanges      []ThreadReviewLineRange `json:"changedRanges,omitempty"`
	Hunks              []ThreadReviewHunk      `json:"hunks,omitempty"`
}

type ThreadReviewState struct {
	ThreadID        string                 `json:"threadID"`
	TurnID          string                 `json:"turnID"`
	ChatPath        string                 `json:"chatPath"`
	Status          ThreadReviewTurnStatus `json:"status"`
	CreatedAt       string                 `json:"createdAt"`
	CanReview       bool                   `json:"canReview"`
	CanRollback     bool                   `json:"canRollback"`
	Unresolved      int                    `json:"unresolved"`
	ApprovedCount   int                    `json:"approvedCount"`
	RejectedCount   int                    `json:"rejectedCount"`
	RolledBackCount int                    `json:"rolledBackCount"`
	ConflictCount   int                    `json:"conflictCount,omitempty"`
	Files           []ThreadReviewFile     `json:"files"`
}

type ThreadReviewListParams struct {
	ThreadID string `json:"threadID,omitempty"`
	ChatPath string `json:"chatPath,omitempty"`
}

type ThreadReviewListResult struct {
	Reviews []ThreadReviewState `json:"reviews,omitempty"`
}

type ThreadReviewResolveParams struct {
	ThreadID string               `json:"threadID,omitempty"`
	ChatPath string               `json:"chatPath,omitempty"`
	TurnID   string               `json:"turnID"`
	Decision ThreadReviewDecision `json:"decision"`
	Path     string               `json:"path,omitempty"`
}

type ThreadReviewResolveResult struct {
	Review *ThreadReviewState `json:"review,omitempty"`
}

type ThreadReviewRollbackParams struct {
	ThreadID string                    `json:"threadID,omitempty"`
	ChatPath string                    `json:"chatPath,omitempty"`
	TurnID   string                    `json:"turnID"`
	Scope    ThreadReviewRollbackScope `json:"scope"`
	Path     string                    `json:"path,omitempty"`
}

type ThreadReviewRollbackResult struct {
	Review *ThreadReviewState `json:"review,omitempty"`
}
