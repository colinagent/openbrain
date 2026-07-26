package docx

import "time"

const (
	SchemaVersion = 1
	wordNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
)

type RunSpan struct {
	ID         string `json:"run_id"`
	Start      int    `json:"start"`
	End        int    `json:"end"`
	Text       string `json:"text"`
	Anchorable bool   `json:"anchorable"`

	rawStart  int
	rawEnd    int
	textStart int
	textEnd   int
}

type Block struct {
	ID          string    `json:"block_id"`
	Part        string    `json:"part"`
	Kind        string    `json:"kind"`
	Text        string    `json:"text"`
	ContextHash string    `json:"context_hash"`
	Runs        []RunSpan `json:"runs"`
}

type Inspection struct {
	Version          int     `json:"version"`
	InputSHA256      string  `json:"input_sha256"`
	Blocks           []Block `json:"blocks"`
	ExistingComments int     `json:"existing_comments"`
}

type CommentPlan struct {
	Version     int              `json:"version"`
	InputSHA256 string           `json:"input_sha256"`
	Comments    []CommentRequest `json:"comments"`
}

type CommentRequest struct {
	FindingID   string    `json:"finding_id"`
	BlockID     string    `json:"block_id"`
	ExactQuote  string    `json:"exact_quote"`
	Start       int       `json:"start"`
	End         int       `json:"end"`
	ContextHash string    `json:"context_hash"`
	Body        string    `json:"body"`
	Author      string    `json:"author,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

type AuditItem struct {
	FindingID string `json:"finding_id"`
	BlockID   string `json:"block_id"`
	CommentID int    `json:"comment_id"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
	Status    string `json:"status"`
}

type Audit struct {
	Version      int         `json:"version"`
	Operation    string      `json:"operation"`
	Status       string      `json:"status"`
	InputSHA256  string      `json:"input_sha256"`
	OutputSHA256 string      `json:"output_sha256"`
	OutputName   string      `json:"output_name"`
	Items        []AuditItem `json:"items"`
}

type Validation struct {
	Version      int      `json:"version"`
	Valid        bool     `json:"valid"`
	Errors       []string `json:"errors"`
	Warnings     []string `json:"warnings"`
	CommentCount int      `json:"comment_count"`
	InputSHA256  string   `json:"input_sha256"`
}
