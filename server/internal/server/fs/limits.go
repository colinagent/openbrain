package fs

// File size limits (reference: VS Code files.ts getLargeFileConfirmationLimit)
const (
	// RemoteFileSizeLimit is the max file size for remote files (10MB)
	// This is stricter to avoid costly file transfers
	RemoteFileSizeLimit = 10 * 1024 * 1024

	// LocalFileSizeLimit is the max file size for local files (1GB)
	LocalFileSizeLimit = 1024 * 1024 * 1024

	// DefaultFileSizeLimit used when mode is unknown
	DefaultFileSizeLimit = RemoteFileSizeLimit

	// StreamBufferSize for streaming large files
	StreamBufferSize = 64 * 1024 // 64KB chunks
)

// ByteSize helpers
const (
	KB = 1024
	MB = KB * 1024
	GB = MB * 1024
)

// IsFileTooLarge checks if file size exceeds the limit
func IsFileTooLarge(size int64, limit int64) bool {
	if limit <= 0 {
		limit = DefaultFileSizeLimit
	}
	return size > limit
}
