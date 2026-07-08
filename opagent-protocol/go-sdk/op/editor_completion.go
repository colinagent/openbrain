package op

type EditorCompletionBlock struct {
	Text     string `json:"text,omitempty"`
	Start    int64  `json:"start,omitempty"`
	End      int64  `json:"end,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Language string `json:"language,omitempty"`
}

type EditorCompletionRequest struct {
	RequestID       string                 `json:"requestID"`
	AgentID         string                 `json:"agentID,omitempty"`
	ModelKey        string                 `json:"modelKey,omitempty"`
	ThinkingLevel   string                 `json:"thinkingLevel,omitempty"`
	EditorKind      string                 `json:"editorKind,omitempty"`
	LanguageID      string                 `json:"languageId,omitempty"`
	DocumentPath    string                 `json:"documentPath,omitempty"`
	CursorOffset    int64                  `json:"cursorOffset"`
	Prefix          string                 `json:"prefix,omitempty"`
	Suffix          string                 `json:"suffix,omitempty"`
	CurrentBlock    *EditorCompletionBlock `json:"currentBlock,omitempty"`
	PreviousBlock   *EditorCompletionBlock `json:"previousBlock,omitempty"`
	NextBlock       *EditorCompletionBlock `json:"nextBlock,omitempty"`
	MaxOutputTokens int64                  `json:"maxOutputTokens,omitempty"`
	Meta            Meta                   `json:"_meta,omitempty"`
}

type EditorCompletionResult struct {
	RequestID   string `json:"requestID"`
	InsertText  string `json:"insertText"`
	ReplaceFrom int64  `json:"replaceFrom"`
	ReplaceTo   int64  `json:"replaceTo"`
	StopReason  string `json:"stopReason,omitempty"`
	ModelKey    string `json:"modelKey,omitempty"`
}

type EditorCompletionCancelParams struct {
	RequestID string `json:"requestID"`
}
