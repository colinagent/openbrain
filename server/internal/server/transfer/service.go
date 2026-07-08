package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	maxBinaryBytes = 512 * 1024 * 1024
)

var safeNameRE = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type Service struct {
	baseDir string
	mu      sync.RWMutex
	records map[string]*Record
}

func NewService(baseDir string) *Service {
	return &Service{
		baseDir: strings.TrimSpace(baseDir),
		records: make(map[string]*Record),
	}
}

func (s *Service) Create(req CreateRequest) (*CreateResponse, error) {
	now := time.Now()
	record, err := s.buildRecord(req, now)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.records[record.TransferID] = record
	s.mu.Unlock()
	return &CreateResponse{
		TransferID:   record.TransferID,
		UploadURL:    "/v1/transfers/" + record.TransferID + "/content",
		DownloadURL:  record.DownloadPath,
		RelativePath: record.RelativePath,
		Status:       record.Status,
	}, nil
}

func (s *Service) CreateWithBasePath(req CreateRequest, basePath string) (*CreateResponse, error) {
	res, err := s.Create(req)
	if err != nil {
		return nil, err
	}
	res.UploadURL = basePath + "/v1/transfers/" + res.TransferID + "/content"
	res.DownloadURL = basePath + "/v1/transfers/" + res.TransferID + "/content"
	return res, nil
}

func (s *Service) buildRecord(req CreateRequest, now time.Time) (*Record, error) {
	purpose := Purpose(strings.TrimSpace(string(req.Purpose)))
	if purpose != PurposeBinary {
		return nil, fmt.Errorf("unsupported transfer purpose")
	}
	fileName, ext, err := sanitizeFileName(req.FileName, req.MIMEType)
	if err != nil {
		return nil, err
	}
	if req.Size <= 0 {
		return nil, fmt.Errorf("size must be greater than zero")
	}
	if req.Size > maxBinaryBytes {
		return nil, fmt.Errorf("file exceeds size limit")
	}
	storagePath, relativePath, err := s.resolveStoragePath(fileName, ext)
	if err != nil {
		return nil, err
	}
	transferID := "tf-" + uuid.NewString()
	return &Record{
		TransferID:   transferID,
		Purpose:      purpose,
		FileName:     fileName,
		MIMEType:     strings.TrimSpace(req.MIMEType),
		Size:         req.Size,
		SHA256:       strings.ToLower(strings.TrimSpace(req.SHA256)),
		Status:       StatusPending,
		StoragePath:  storagePath,
		DownloadPath: "/v1/transfers/" + transferID + "/content",
		RelativePath: relativePath,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (s *Service) resolveStoragePath(fileName, ext string) (string, string, error) {
	storageDir := filepath.Join(s.baseDir, "resources", "transfers", "binary")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return "", "", err
	}
	finalPath, _, err := uniqueFilePath(storageDir, fileName, ext)
	return finalPath, "", err
}

func (s *Service) Get(id string) (*Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[strings.TrimSpace(id)]
	if !ok {
		return nil, false
	}
	copy := *record
	return &copy, true
}

func (s *Service) PutContent(id string, body io.Reader, contentLength int64) (*Record, error) {
	s.mu.Lock()
	record, ok := s.records[strings.TrimSpace(id)]
	if !ok {
		s.mu.Unlock()
		return nil, os.ErrNotExist
	}
	if record.Status != StatusPending {
		s.mu.Unlock()
		return nil, fmt.Errorf("transfer is not pending")
	}
	record.UpdatedAt = time.Now()
	storagePath := record.StoragePath
	expectedSize := record.Size
	expectedSHA := record.SHA256
	s.mu.Unlock()

	if contentLength > 0 && expectedSize != contentLength {
		s.fail(id, "content length mismatch")
		return nil, fmt.Errorf("content length mismatch")
	}
	if err := os.MkdirAll(filepath.Dir(storagePath), 0o755); err != nil {
		s.fail(id, err.Error())
		return nil, err
	}
	tmpPath := storagePath + ".part"
	_ = os.Remove(tmpPath)
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		s.fail(id, err.Error())
		return nil, err
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(f, hasher), io.LimitReader(body, expectedSize+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		s.fail(id, copyErr.Error())
		return nil, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		s.fail(id, closeErr.Error())
		return nil, closeErr
	}
	if written != expectedSize {
		_ = os.Remove(tmpPath)
		s.fail(id, "written size mismatch")
		return nil, fmt.Errorf("written size mismatch")
	}
	if expectedSHA != "" && hex.EncodeToString(hasher.Sum(nil)) != expectedSHA {
		_ = os.Remove(tmpPath)
		s.fail(id, "sha256 mismatch")
		return nil, fmt.Errorf("sha256 mismatch")
	}
	if err := os.Rename(tmpPath, storagePath); err != nil {
		_ = os.Remove(tmpPath)
		s.fail(id, err.Error())
		return nil, err
	}

	s.mu.Lock()
	record = s.records[id]
	record.Status = StatusCompleted
	record.Error = ""
	record.UpdatedAt = time.Now()
	copy := *record
	s.mu.Unlock()
	return &copy, nil
}

func (s *Service) fail(id, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return
	}
	record.Status = StatusFailed
	record.Error = strings.TrimSpace(reason)
	record.UpdatedAt = time.Now()
}

func sanitizeFileName(name, mimeType string) (string, string, error) {
	trimmed := strings.TrimSpace(name)
	trimmed = strings.Trim(filepath.Base(trimmed), ". ")
	trimmed = strings.ReplaceAll(trimmed, string(filepath.Separator), "-")
	if trimmed == "" {
		trimmed = "file"
	}
	ext := strings.ToLower(filepath.Ext(trimmed))
	base := strings.TrimSuffix(trimmed, ext)
	base = safeNameRE.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-._")
	if base == "" {
		base = "file"
	}
	if ext == "" {
		ext = extensionFromMime(mimeType)
	}
	return base + ext, ext, nil
}

func extensionFromMime(mimeType string) string {
	if mimeType == "" {
		return ""
	}
	exts, _ := mime.ExtensionsByType(mimeType)
	if len(exts) == 0 {
		return ""
	}
	return strings.ToLower(exts[0])
}

func uniqueFilePath(dir, fileName, ext string) (string, string, error) {
	name := strings.TrimSuffix(fileName, ext)
	if name == "" {
		name = "file"
	}
	for i := 0; i < 1000; i++ {
		candidateName := name + ext
		if i > 0 {
			candidateName = fmt.Sprintf("%s-%d%s", name, i+1, ext)
		}
		candidatePath := filepath.Join(dir, candidateName)
		if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
			return candidatePath, candidateName, nil
		} else if err != nil {
			return "", "", err
		}
	}
	return "", "", fmt.Errorf("failed to allocate unique file path")
}
