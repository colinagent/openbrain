package op

type ThreadQueueKind string

const (
	ThreadQueueKindSteering ThreadQueueKind = "steering"
	ThreadQueueKindFollowUp ThreadQueueKind = "follow_up"
)

const (
	ThreadEntryTypeQueueEnqueue = "queue_enqueue"
	ThreadEntryTypeQueueDequeue = "queue_dequeue"
	ThreadEntryTypeQueueRemove  = "queue_remove"
	ThreadEntryTypeQueuePromote = "queue_promote"
)

type ThreadRunStatus string

const (
	ThreadRunIdle    ThreadRunStatus = "idle"
	ThreadRunRunning ThreadRunStatus = "running"
)

type ThreadTailStatus string

const (
	ThreadTailEmpty             ThreadTailStatus = "empty"
	ThreadTailComplete          ThreadTailStatus = "complete"
	ThreadTailNeedsContinuation ThreadTailStatus = "needs_continuation"
)

type ThreadContinuationReason string

const (
	ThreadContinuationNone           ThreadContinuationReason = ""
	ThreadContinuationUserTail       ThreadContinuationReason = "user_tail"
	ThreadContinuationToolResultTail ThreadContinuationReason = "tool_result_tail"
	ThreadContinuationAssistantTool  ThreadContinuationReason = "assistant_tool_use"
	ThreadContinuationAssistantError ThreadContinuationReason = "assistant_error"
	ThreadContinuationAssistantAbort ThreadContinuationReason = "assistant_aborted"
)

type ThreadQueueItem struct {
	ID                   string   `json:"id"`
	Message              Message  `json:"message"`
	AgentID              string   `json:"agentID,omitempty"`
	AgentName            string   `json:"agentName,omitempty"`
	CWD                  string   `json:"cwd,omitempty"`
	ModelKey             string   `json:"modelKey,omitempty"`
	ThinkingLevel        string   `json:"thinkingLevel,omitempty"`
	ContextWindow        int64    `json:"contextWindow,omitempty"`
	ServiceTier          string   `json:"serviceTier,omitempty"`
	SelectedSkillIDs     []string `json:"selectedSkillIDs,omitempty"`
	SelectedSkillContext Meta     `json:"selectedSkillContext,omitempty"`
	PlanTurn             bool     `json:"planTurn,omitempty"`
}

type ThreadQueueSnapshot struct {
	Steering []ThreadQueueItem `json:"steering,omitempty"`
	FollowUp []ThreadQueueItem `json:"followUp,omitempty"`
}

type ThreadQueueEnqueueEntry struct {
	ThreadEntryBase
	QueueKind ThreadQueueKind `json:"queueKind"`
	Item      ThreadQueueItem `json:"item"`
}

type ThreadQueueDequeueEntry struct {
	ThreadEntryBase
	QueueKind ThreadQueueKind `json:"queueKind"`
	ItemID    string          `json:"itemID"`
}

type ThreadQueueRemoveEntry struct {
	ThreadEntryBase
	QueueKind ThreadQueueKind `json:"queueKind"`
	Item      ThreadQueueItem `json:"item"`
}

type ThreadQueuePromoteEntry struct {
	ThreadEntryBase
	ItemID string `json:"itemID"`
}

type ThreadControlAck struct {
	OK             bool                `json:"ok"`
	ThreadID       string              `json:"threadID"`
	OpCode         OpCode              `json:"opcode"`
	QueuedMessages ThreadQueueSnapshot `json:"queuedMessages,omitempty"`
	RemovedItem    *ThreadQueueItem    `json:"removedItem,omitempty"`
}

type ThreadRuntimeInfo struct {
	ThreadID string `json:"threadID"`
	ChatPath string `json:"chatPath,omitempty"`
}

type ThreadActiveList struct {
	Threads []ThreadRuntimeInfo `json:"threads"`
}
