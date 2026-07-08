package op

import "encoding/json"

const ThreadEntryTypeCanonicalMessage = "canonical_message"

type ConversationRole string

const (
	RoleCanonicalSystem     ConversationRole = "system"
	RoleCanonicalDeveloper  ConversationRole = "developer"
	RoleCanonicalUser       ConversationRole = "user"
	RoleCanonicalAssistant  ConversationRole = "assistant"
	RoleCanonicalTool       ConversationRole = "tool_result"
	RoleCanonicalCompaction ConversationRole = "compaction"
)

type ContentBlockType string

const (
	BlockText       ContentBlockType = "text"
	BlockThinking   ContentBlockType = "thinking"
	BlockImage      ContentBlockType = "image"
	BlockToolCall   ContentBlockType = "tool_call"
	BlockToolResult ContentBlockType = "tool_result"
	BlockCompaction ContentBlockType = "compaction"
)

type ContentBlock struct {
	Type                ContentBlockType     `json:"type"`
	Text                string               `json:"text,omitempty"`
	MimeType            string               `json:"mimeType,omitempty"`
	ImageData           string               `json:"imageData,omitempty"`
	TextSignature       string               `json:"textSignature,omitempty"`
	ThinkingReplayField string               `json:"thinkingReplayField,omitempty"`
	ThinkingSignature   string               `json:"thinkingSignature,omitempty"`
	ToolCall            *CanonicalToolCall   `json:"toolCall,omitempty"`
	ToolResult          *CanonicalToolResult `json:"toolResult,omitempty"`
	EncryptedContent    string               `json:"encryptedContent,omitempty"`
	Raw                 json.RawMessage      `json:"raw,omitempty"`
}

type CanonicalToolCall struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Arguments        map[string]any  `json:"arguments,omitempty"`
	RawArguments     string          `json:"rawArguments,omitempty"`
	ThoughtSignature string          `json:"thoughtSignature,omitempty"`
	Raw              json.RawMessage `json:"raw,omitempty"`
}

type CanonicalToolResult struct {
	ToolCallID    string          `json:"toolCallID"`
	ToolName      string          `json:"toolName,omitempty"`
	IsError       bool            `json:"isError,omitempty"`
	OutputText    string          `json:"outputText,omitempty"`
	OutputContent []ContentBlock  `json:"outputContent,omitempty"`
	Raw           json.RawMessage `json:"raw,omitempty"`
}

type ConversationMessage struct {
	Role          ConversationRole  `json:"role"`
	Content       []ContentBlock    `json:"content,omitempty"`
	Timestamp     int64             `json:"timestamp,omitempty"`
	ProviderState *ProviderState    `json:"providerState,omitempty"`
	Usage         *MessageUsage     `json:"usage,omitempty"`
	StopReason    MessageStopReason `json:"stopReason,omitempty"`
	Raw           json.RawMessage   `json:"raw,omitempty"`
}

type ProviderState struct {
	ProviderRef string `json:"providerRef,omitempty"`
	Provider    string `json:"provider,omitempty"`
	API         string `json:"api,omitempty"`
	Model       string `json:"model,omitempty"`
	ResponseID  string `json:"responseID,omitempty"`
}

type ThreadCanonicalMessageEntry struct {
	ThreadEntryBase
	Message ConversationMessage `json:"message"`
}
