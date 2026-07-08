package treeimport

import "time"

type EntryKind string

const (
	EntryKindFile EntryKind = "file"
	EntryKindDir  EntryKind = "dir"
)

type TreeImportEntry struct {
	Kind         EntryKind `json:"kind"`
	RelativePath string    `json:"relativePath"`
	Size         *int64    `json:"size,omitempty"`
}

type CreateTreeImportSessionRequest struct {
	TargetDir  string            `json:"targetDir"`
	Entries    []TreeImportEntry `json:"entries"`
	GrantToken string            `json:"grantToken,omitempty"`
}

type CreateTreeImportSessionResponse struct {
	SessionID     string   `json:"sessionId"`
	UploadBaseURL string   `json:"uploadBaseUrl"`
	Conflicts     []string `json:"conflicts"`
}

type CommitTreeImportRequest struct {
	Overwrite bool `json:"overwrite"`
}

type CommitTreeImportResponse struct {
	ImportedFiles int `json:"importedFiles"`
	ImportedDirs  int `json:"importedDirs"`
}

type CancelTreeImportResponse struct {
	Success bool `json:"success"`
}

type sessionEntry struct {
	Kind         EntryKind
	RelativePath string
	Size         int64
}

type sessionRecord struct {
	ID        string
	TargetDir string
	StageDir  string
	Entries   map[string]sessionEntry
	ExpiresAt time.Time
}

type ConflictError struct {
	Paths []string
}

func (e *ConflictError) Error() string {
	return "tree import conflicts detected"
}
