package op

type OpCode string

const (
	// agent
	// Deprecated: thread chat submission should use OpThreadSubmit. Kept as a legacy edge adapter.
	OpAgentCall OpCode = "agent/call"
	// Deprecated: thread chat submission should use OpThreadSubmit. Kept as a legacy edge adapter.
	OpAgentContinue   OpCode = "agent/continue"
	OpAgentLoopCreate OpCode = "agent/loop/create"
	OpPromptGet       OpCode = "prompt/get"
	// OpAgentRoots OpCode = "agents/roots" // list agent roots
	// OpAgentGet   OpCode = "agent/get"
	OpAgentScan OpCode = "agent/scan"

	//node
	// OpNodeScan OpCode = "node/scan"
	OpNodeList OpCode = "node/list"
	// OpNodeCached OpCode = "node/cached"
	// OpNodeCall OpCode = "node/call"

	// OpAgentUpsert OpCode = "agent/upsert"
	// OpLoopCreate  OpCode = "agent/loop/create"

	//host
	SystemStarted OpCode = "system/started"
	// SystemNotify  OpCode = "system/notify"
	// SystemConfigGet OpCode = "system/config/get"
	// HostSecretGet OpCode = "host/secret/get"

	// notify
	NotifyMessage OpCode = "notify/message"

	//config/get
	ConfigGet       OpCode = "config/get"
	ConfigSystemGet OpCode = "config/system/get"

	//thread
	OpThreadCreate           OpCode = "thread/create"
	OpThreadFork             OpCode = "thread/fork"
	OpThreadMetaGet          OpCode = "thread/meta/get"
	OpThreadMetaUpdate       OpCode = "thread/meta/update"
	OpThreadSnapshotGet      OpCode = "thread/snapshot/get"
	OpThreadReviewList       OpCode = "thread/review/list"
	OpThreadReviewResolve    OpCode = "thread/review/resolve"
	OpThreadReviewRollback   OpCode = "thread/review/rollback"
	OpEditorCompletion       OpCode = "editor/completion"
	OpEditorCompletionCancel OpCode = "editor/completion/cancel"
	OpThreadSubmit           OpCode = "thread/submit"
	OpThreadCompact          OpCode = "thread/compact"
	OpThreadInterrupted      OpCode = "thread/interrupted"
	OpThreadSteer            OpCode = "thread/steer"
	OpThreadFollowUp         OpCode = "thread/follow_up"
	OpThreadFollowUpPromote  OpCode = "thread/follow_up/promote"
	OpThreadQueueGet         OpCode = "thread/queue/get"
	OpThreadQueueRemove      OpCode = "thread/queue/remove"
	OpThreadActiveList       OpCode = "thread/active/list"
	OpMessageList            OpCode = "message/list"
	OpMessageRead            OpCode = "message/read"
	OpMessageReply           OpCode = "message/reply"
	OpMessageAck             OpCode = "message/ack"
	OpMessageArchive         OpCode = "message/archive"
	// OpThreadIDGet OpCode = "threadID/get"
	// OpThreadList        OpCode = "thread/list"
	// OpThreadQuery       OpCode = "thread/query"
	// OpThreadDelete      OpCode = "thread/delete"
	// OpThreadIDUpsert    OpCode = "threadID/upsert"
	// OpThreadUpsert      OpCode = "thread/upsert"

	// //user
	// OpUIDList OpCode = "uid/list" // list all UIDs
	// // user profile / user-agent
	// OpUserProfileGet    OpCode = "user/profile/get"
	// OpUserProfileUpsert OpCode = "user/profile/upsert"
	// OpUserAgentList     OpCode = "user/agent/list"
	// OpUserAgentBind     OpCode = "user/agent/bind"
	// OpUserAgentUnbind   OpCode = "user/agent/unbind"

	// //mcp
	// OpMCPToolCall OpCode = "mcp/tool/call"
	// OpToolCall    OpCode = "tool/call"

	// //elicitation
	// OpElicitCreate OpCode = "elicitation/create"
	// OpElicitUpdate OpCode = "elicitation/update"

	// // skill
	// OpSkillUse OpCode = "skill/use"
	// OpSkillGet OpCode = "skill/get"
)
