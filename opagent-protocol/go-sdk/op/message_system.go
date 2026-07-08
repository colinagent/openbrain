package op

type MessageSender string

const (
	MessageSenderUser   MessageSender = "user"
	MessageSenderAgent  MessageSender = "agent"
	MessageSenderSystem MessageSender = "system"
)

type MessageKind string

const (
	MessageKindMessage MessageKind = "message"
	MessageKindRequest MessageKind = "request"
	MessageKindStatus  MessageKind = "status"
)

type MessageStatus string

const (
	MessageStatusOpen     MessageStatus = "open"
	MessageStatusResolved MessageStatus = "resolved"
	MessageStatusArchived MessageStatus = "archived"
)

type MessageActionTone string

const (
	MessageActionTonePrimary MessageActionTone = "primary"
	MessageActionToneDanger  MessageActionTone = "danger"
)

type MessageAction struct {
	ID    string            `json:"id"`
	Label string            `json:"label"`
	Tone  MessageActionTone `json:"tone,omitempty"`
}

type MessageQuestionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type MessageQuestion struct {
	ID       string                  `json:"id"`
	Question string                  `json:"question"`
	Options  []MessageQuestionOption `json:"options,omitempty"`
}

type MessageAnswer struct {
	QuestionID string `json:"questionID"`
	OptionID   string `json:"optionID,omitempty"`
	Label      string `json:"label,omitempty"`
	Other      bool   `json:"other,omitempty"`
	Text       string `json:"text,omitempty"`
}

type MessageRecord struct {
	ID               string            `json:"id"`
	ChannelID        string            `json:"channelID"`
	ThreadID         string            `json:"threadID"`
	AgentID          string            `json:"agentID"`
	Sender           MessageSender     `json:"sender"`
	Kind             MessageKind       `json:"kind"`
	Status           MessageStatus     `json:"status"`
	Title            string            `json:"title,omitempty"`
	Body             string            `json:"body"`
	Actions          []MessageAction   `json:"actions,omitempty"`
	Questions        []MessageQuestion `json:"questions,omitempty"`
	ReplyToMessageID string            `json:"replyToMessageID,omitempty"`
	ActionID         string            `json:"actionID,omitempty"`
	Answers          []MessageAnswer   `json:"answers,omitempty"`
	CreatedAt        string            `json:"createdAt"`
	UpdatedAt        string            `json:"updatedAt"`
	Meta             Meta              `json:"meta,omitempty"`
}

type MessageChannelSummary struct {
	ChannelID       string         `json:"channelID"`
	ThreadID        string         `json:"threadID"`
	AgentID         string         `json:"agentID"`
	Title           string         `json:"title,omitempty"`
	LastMessage     *MessageRecord `json:"lastMessage,omitempty"`
	OpenCount       int            `json:"openCount,omitempty"`
	UnreadUserCount int            `json:"unreadUserCount,omitempty"`
	UpdatedAt       string         `json:"updatedAt,omitempty"`
}

const (
	ThreadEntryTypeMessageAppend = "message_append"
	ThreadEntryTypeMessageUpdate = "message_update"
	ThreadEntryTypeMessageAck    = "message_ack"
)

type ThreadMessageAppendEntry struct {
	ThreadEntryBase
	Record  MessageRecord `json:"record"`
	Pending bool          `json:"pending"`
}

type ThreadMessageUpdateEntry struct {
	ThreadEntryBase
	Record MessageRecord `json:"record"`
}

type ThreadMessageAckEntry struct {
	ThreadEntryBase
	MessageID string `json:"messageID"`
	Pending   bool   `json:"pending"`
}

type MessagePublishParams struct {
	ChannelID string            `json:"channelID,omitempty"`
	ThreadID  string            `json:"threadID,omitempty"`
	AgentID   string            `json:"agentID,omitempty"`
	Kind      MessageKind       `json:"kind,omitempty"`
	Title     string            `json:"title,omitempty"`
	Body      string            `json:"body"`
	Actions   []MessageAction   `json:"actions,omitempty"`
	Questions []MessageQuestion `json:"questions,omitempty"`
	Meta      Meta              `json:"meta,omitempty"`
}

type MessagePublishResult struct {
	MessageID string `json:"messageID"`
	ChannelID string `json:"channelID"`
	ThreadID  string `json:"threadID"`
	Delivered bool   `json:"delivered"`
}

type MessageUpdateParams struct {
	MessageID string            `json:"messageID"`
	Body      *string           `json:"body,omitempty"`
	Title     *string           `json:"title,omitempty"`
	Status    MessageStatus     `json:"status,omitempty"`
	Actions   []MessageAction   `json:"actions,omitempty"`
	Questions []MessageQuestion `json:"questions,omitempty"`
	Meta      Meta              `json:"meta,omitempty"`
}

type MessageReadParams struct {
	ChannelID   string `json:"channelID,omitempty"`
	ThreadID    string `json:"threadID,omitempty"`
	AgentID     string `json:"agentID,omitempty"`
	PendingOnly bool   `json:"pendingOnly,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

type MessageReadResult struct {
	ChannelID string          `json:"channelID,omitempty"`
	ThreadID  string          `json:"threadID,omitempty"`
	AgentID   string          `json:"agentID,omitempty"`
	Messages  []MessageRecord `json:"messages,omitempty"`
}

type MessageSubscribeParams struct {
	ChannelID string `json:"channelID,omitempty"`
	ThreadID  string `json:"threadID,omitempty"`
	AgentID   string `json:"agentID,omitempty"`
}

type MessageSubscribeResult struct {
	ChannelID  string `json:"channelID"`
	ThreadID   string `json:"threadID"`
	AgentID    string `json:"agentID"`
	Subscribed bool   `json:"subscribed"`
}

type MessageAckParams struct {
	ChannelID  string   `json:"channelID,omitempty"`
	ThreadID   string   `json:"threadID,omitempty"`
	AgentID    string   `json:"agentID,omitempty"`
	MessageIDs []string `json:"messageIDs,omitempty"`
}

type MessageAckResult struct {
	ChannelID string `json:"channelID"`
	ThreadID  string `json:"threadID"`
	AgentID   string `json:"agentID"`
	Acked     int    `json:"acked"`
}

type MessageReplyParams struct {
	ChannelID        string          `json:"channelID"`
	ReplyToMessageID string          `json:"replyToMessageID,omitempty"`
	Text             string          `json:"text,omitempty"`
	ActionID         string          `json:"actionID,omitempty"`
	Answers          []MessageAnswer `json:"answers,omitempty"`
}

type MessageReplyDispatch struct {
	Opcode  OpCode `json:"opcode"`
	Meta    Meta   `json:"meta,omitempty"`
	Content string `json:"content"`
}

type MessageReplyResult struct {
	Record   MessageRecord         `json:"record"`
	Resolved *MessageRecord        `json:"resolved,omitempty"`
	Dispatch *MessageReplyDispatch `json:"dispatch,omitempty"`
	Queue    *ThreadControlAck     `json:"queue,omitempty"`
}

type MessageListParams struct {
	ThreadID string `json:"threadID,omitempty"`
	AgentID  string `json:"agentID,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type MessageListResult struct {
	Channels []MessageChannelSummary `json:"channels,omitempty"`
	Messages []MessageRecord         `json:"messages,omitempty"`
}

type MessageArchiveParams struct {
	ChannelID           string `json:"channelID,omitempty"`
	MessageID           string `json:"messageID,omitempty"`
	AgentID             string `json:"agentID,omitempty"`
	PendingRequestsOnly bool   `json:"pendingRequestsOnly,omitempty"`
}

type MessageArchiveResult struct {
	Archived int `json:"archived"`
}
