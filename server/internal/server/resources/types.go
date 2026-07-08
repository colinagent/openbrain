package resources

import "time"

type Target struct {
	Kind string `json:"kind"`
	URI  string `json:"uri,omitempty"`
	URL  string `json:"url,omitempty"`
}

type CreateGrantRequest struct {
	Authority string   `json:"authority"`
	Roots     []string `json:"roots"`
}

type CreateGrantResponse struct {
	GrantToken string `json:"grantToken"`
	ExpiresAt  string `json:"expiresAt"`
}

type InspectRequest struct {
	Target     Target `json:"target"`
	Intent     string `json:"intent"`
	GrantToken string `json:"grantToken,omitempty"`
}

type Meta struct {
	Target           Target `json:"target"`
	Name             string `json:"name"`
	MIMEType         string `json:"mimeType"`
	Size             *int64 `json:"size,omitempty"`
	EntryType        string `json:"entryType,omitempty"`
	EpubPackagePath  string `json:"epubPackagePath,omitempty"`
	Exists           bool   `json:"exists"`
	Renderable       bool   `json:"renderable"`
	Downloadable     bool   `json:"downloadable"`
	CanonicalFileURI string `json:"canonicalFileURI,omitempty"`
	LastModified     string `json:"lastModified,omitempty"`
}

type HandleResponse struct {
	HandleID        string `json:"handleId"`
	URL             string `json:"url"`
	ExpiresAt       string `json:"expiresAt"`
	Intent          string `json:"intent"`
	MIMEType        string `json:"mimeType"`
	Size            *int64 `json:"size,omitempty"`
	EntryType       string `json:"entryType,omitempty"`
	EpubPackagePath string `json:"epubPackagePath,omitempty"`
}

type ImportPurpose string

const (
	ImportPurposeMarkdownImage ImportPurpose = "markdown-image"
	ImportPurposeAttachment    ImportPurpose = "attachment"
)

type CreateImportSessionRequest struct {
	Purpose           ImportPurpose `json:"purpose"`
	TargetDocumentURI string        `json:"targetDocumentURI"`
	FileName          string        `json:"fileName"`
	MIMEType          string        `json:"mimeType"`
	Size              int64         `json:"size"`
	SHA256            string        `json:"sha256,omitempty"`
	GrantToken        string        `json:"grantToken,omitempty"`
}

type CreateImportSessionResponse struct {
	SessionID           string `json:"sessionId"`
	UploadURL           string `json:"uploadUrl"`
	ExpectedDocumentRef string `json:"expectedDocumentRef"`
	ProvisionalTarget   Target `json:"provisionalTarget"`
}

type ImportResult struct {
	DocumentRef  string          `json:"documentRef"`
	Target       Target          `json:"target"`
	RenderHandle *HandleResponse `json:"renderHandle,omitempty"`
}

type handleRecord struct {
	ID              string
	Intent          string
	Target          Target
	Path            string
	MIMEType        string
	Size            *int64
	EntryType       string
	EpubPackagePath string
	ExpiresAt       time.Time
	Inline          bool
}

type importSession struct {
	ID                string
	Purpose           ImportPurpose
	Authority         string
	TargetDocumentURI string
	StoragePath       string
	DocumentRef       string
	Target            Target
	FileName          string
	MIMEType          string
	Size              int64
	SHA256            string
	ExpiresAt         time.Time
}

type grantRecord struct {
	Token     string
	Authority string
	Roots     []string
	ExpiresAt time.Time
}
