package transfer

import "time"

type Purpose string

const (
	PurposeBinary Purpose = "binary"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type CreateRequest struct {
	Purpose  Purpose `json:"purpose"`
	FileName string  `json:"fileName"`
	MIMEType string  `json:"mimeType"`
	Size     int64   `json:"size"`
	SHA256   string  `json:"sha256,omitempty"`
}

type CreateResponse struct {
	TransferID   string `json:"transferId"`
	UploadURL    string `json:"uploadUrl"`
	DownloadURL  string `json:"downloadUrl"`
	RelativePath string `json:"relativePath,omitempty"`
	Status       Status `json:"status"`
}

type Record struct {
	TransferID   string    `json:"transferId"`
	Purpose      Purpose   `json:"purpose"`
	FileName     string    `json:"fileName"`
	MIMEType     string    `json:"mimeType"`
	Size         int64     `json:"size"`
	SHA256       string    `json:"sha256,omitempty"`
	Status       Status    `json:"status"`
	StoragePath  string    `json:"storagePath,omitempty"`
	DownloadPath string    `json:"downloadPath"`
	RelativePath string    `json:"relativePath,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Error        string    `json:"error,omitempty"`
}
