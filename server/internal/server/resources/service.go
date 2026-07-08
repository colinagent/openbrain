package resources

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	chatservice "github.com/colinagent/openbrain/server/internal/server/chat"
	"github.com/google/uuid"
)

const handleTTL = 5 * time.Minute
const epubMIME = "application/epub+zip"

var safeNameRE = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
var imageMIMEs = map[string]string{
	".gif":  "image/gif",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}
var resourceMIMEs = map[string]string{
	".css":   "text/css; charset=utf-8",
	".epub":  epubMIME,
	".htm":   "text/html; charset=utf-8",
	".html":  "text/html; charset=utf-8",
	".opf":   "application/oebps-package+xml",
	".xhtml": "application/xhtml+xml",
	".xml":   "application/xml",
}

func isRenderableMIME(mimeType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mimeType))
	return strings.HasPrefix(normalized, "image/") || normalized == "application/pdf" || normalized == epubMIME
}

func isEpubPackageDir(path string, info os.FileInfo) bool {
	return info.IsDir() && strings.EqualFold(filepath.Ext(path), ".epub")
}

type epubContainer struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

func resolveEpubPackagePath(dir string) (string, error) {
	containerPath := filepath.Join(dir, "META-INF", "container.xml")
	raw, err := os.ReadFile(containerPath)
	if err == nil {
		var container epubContainer
		if err := xml.Unmarshal(raw, &container); err != nil {
			return "", fmt.Errorf("invalid EPUB container: %w", err)
		}
		for _, rootfile := range container.Rootfiles {
			fullPath := strings.TrimSpace(rootfile.FullPath)
			if fullPath == "" {
				continue
			}
			candidate, err := resolvePackageChildPath(dir, fullPath)
			if err != nil {
				continue
			}
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return filepath.ToSlash(filepath.Clean(fullPath)), nil
			}
		}
		return "", fmt.Errorf("EPUB container does not reference a readable package document")
	}
	if !os.IsNotExist(err) {
		return "", err
	}

	fallback := filepath.Join(dir, "content.opf")
	if info, err := os.Stat(fallback); err == nil && !info.IsDir() {
		return "content.opf", nil
	}
	return "", fmt.Errorf("EPUB package document not found")
}

func resolvePackageChildPath(root, child string) (string, error) {
	root = filepath.Clean(root)
	trimmed := strings.TrimSpace(child)
	if trimmed == "" {
		return root, nil
	}
	trimmed = strings.TrimLeft(filepath.ToSlash(trimmed), "/")
	cleanChild := filepath.Clean(filepath.FromSlash(trimmed))
	if cleanChild == "." {
		return root, nil
	}
	if strings.HasPrefix(cleanChild, ".."+string(filepath.Separator)) || cleanChild == ".." || filepath.IsAbs(cleanChild) {
		return "", fmt.Errorf("resource path escapes EPUB package")
	}
	candidate := filepath.Clean(filepath.Join(root, cleanChild))
	if candidate != root && !strings.HasPrefix(candidate, root+string(filepath.Separator)) {
		return "", fmt.Errorf("resource path escapes EPUB package")
	}
	return candidate, nil
}

type Service struct {
	baseDir         string
	sessionMeta     chatservice.MetaResolver
	handleMu        sync.RWMutex
	handles         map[string]*handleRecord
	stableHandleIDs map[string]string
	sessionMu       sync.RWMutex
	sessions        map[string]*importSession
	grantMu         sync.RWMutex
	grants          map[string]*grantRecord
}

func NewService(baseDir string, sessionMeta chatservice.MetaResolver) *Service {
	baseDir = strings.TrimSpace(baseDir)
	return &Service{
		baseDir:         baseDir,
		sessionMeta:     sessionMeta,
		handles:         make(map[string]*handleRecord),
		stableHandleIDs: make(map[string]string),
		sessions:        make(map[string]*importSession),
		grants:          make(map[string]*grantRecord),
	}
}

func parseCanonicalFileURI(uri string) (authority string, absolutePath string, err error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", "", err
	}
	if parsed.Scheme != "opfs" {
		return "", "", fmt.Errorf("unsupported uri scheme")
	}
	return parsed.Host, filepath.Clean(parsed.Path), nil
}

func buildCanonicalFileURI(authority, absolutePath string) string {
	return fmt.Sprintf("opfs://%s%s", authority, filepath.ToSlash(filepath.Clean(absolutePath)))
}

func normalizeRoots(baseDir string, roots []string) []string {
	set := make(map[string]struct{})
	push := func(path string) {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean != "" && filepath.IsAbs(clean) {
			set[clean] = struct{}{}
		}
	}
	for _, root := range roots {
		push(root)
	}
	push(filepath.Join(baseDir, "resources"))
	out := make([]string, 0, len(set))
	for root := range set {
		out = append(out, root)
	}
	sort.Strings(out)
	return out
}

func isAllowedPath(path string, roots []string) bool {
	clean := filepath.Clean(path)
	for _, root := range roots {
		root = filepath.Clean(root)
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func buildStableHandleKey(authority string, roots []string, target Target, intent string) string {
	return strings.TrimSpace(authority) +
		"\n" + strings.Join(roots, "\n") +
		"\n" + strings.TrimSpace(intent) +
		"\n" + strings.TrimSpace(target.Kind) +
		"\n" + strings.TrimSpace(target.URI) +
		"\n" + strings.TrimSpace(target.URL)
}

func handleResponse(record *handleRecord) *HandleResponse {
	url := "/v1/resources/content/" + record.ID
	if record.EntryType == "directory" {
		url += "/"
	}
	return &HandleResponse{
		HandleID:        record.ID,
		URL:             url,
		ExpiresAt:       "",
		Intent:          record.Intent,
		MIMEType:        record.MIMEType,
		Size:            record.Size,
		EntryType:       record.EntryType,
		EpubPackagePath: record.EpubPackagePath,
	}
}

func (s *Service) CreateGrant(req CreateGrantRequest) (*CreateGrantResponse, error) {
	authority := strings.TrimSpace(req.Authority)
	if authority == "" {
		return nil, fmt.Errorf("authority is required")
	}
	roots := normalizeRoots(s.baseDir, req.Roots)
	if len(roots) == 0 {
		return nil, fmt.Errorf("at least one root is required")
	}
	now := time.Now()
	expiresAt := now.Add(handleTTL)
	token := "rg-" + uuid.NewString()
	record := &grantRecord{Token: token, Authority: authority, Roots: roots, ExpiresAt: expiresAt}
	s.grantMu.Lock()
	s.grants[token] = record
	s.grantMu.Unlock()
	return &CreateGrantResponse{GrantToken: token, ExpiresAt: expiresAt.UTC().Format(time.RFC3339)}, nil
}

func (s *Service) ResolveGrant(token, authority string) ([]string, error) {
	return s.resolveGrant(token, authority)
}

func (s *Service) resolveGrant(token, authority string) ([]string, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil, fmt.Errorf("grant token is required")
	}
	s.grantMu.RLock()
	record, ok := s.grants[trimmed]
	s.grantMu.RUnlock()
	if !ok || time.Now().After(record.ExpiresAt) {
		return nil, fmt.Errorf("grant token is invalid or expired")
	}
	if strings.TrimSpace(authority) != "" && record.Authority != authority {
		return nil, fmt.Errorf("grant token does not match authority")
	}
	return record.Roots, nil
}

func (s *Service) resolveFileTarget(target Target, grantToken string) (string, string, []string, error) {
	if target.Kind != "file" {
		return "", "", nil, fmt.Errorf("unsupported target kind %q", target.Kind)
	}
	authority, path, err := parseCanonicalFileURI(target.URI)
	if err != nil {
		return "", "", nil, err
	}
	if !filepath.IsAbs(path) {
		return "", "", nil, fmt.Errorf("target path must be absolute")
	}
	roots, err := s.resolveGrant(grantToken, authority)
	if err != nil {
		return "", "", nil, err
	}
	if !isAllowedPath(path, roots) {
		return "", "", nil, fmt.Errorf("access denied")
	}
	return authority, path, roots, nil
}

func (s *Service) Inspect(req InspectRequest) (*Meta, error) {
	if req.Target.Kind == "external" || req.Target.Kind == "data" {
		return &Meta{Target: req.Target, Exists: true, Renderable: true, Downloadable: true}, nil
	}
	authority, path, _, err := s.resolveFileTarget(req.Target, req.GrantToken)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Meta{Target: req.Target, Exists: false, Renderable: false, Downloadable: false, CanonicalFileURI: buildCanonicalFileURI(authority, path)}, nil
		}
		return nil, err
	}
	if info.IsDir() {
		if isEpubPackageDir(path, info) {
			packagePath, err := resolveEpubPackagePath(path)
			if err != nil {
				return nil, err
			}
			return &Meta{
				Target:           req.Target,
				Name:             filepath.Base(path),
				MIMEType:         epubMIME,
				EntryType:        "directory",
				EpubPackagePath:  packagePath,
				Exists:           true,
				Renderable:       true,
				Downloadable:     false,
				CanonicalFileURI: buildCanonicalFileURI(authority, path),
				LastModified:     info.ModTime().UTC().Format(time.RFC3339),
			}, nil
		}
		return nil, fmt.Errorf("resource target is a directory")
	}
	size := info.Size()
	mimeType := detectMIME(path)
	return &Meta{
		Target:           req.Target,
		Name:             filepath.Base(path),
		MIMEType:         mimeType,
		Size:             &size,
		EntryType:        "file",
		Exists:           true,
		Renderable:       isRenderableMIME(mimeType),
		Downloadable:     true,
		CanonicalFileURI: buildCanonicalFileURI(authority, path),
		LastModified:     info.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

func (s *Service) CreateHandle(target Target, intent string, grantToken string) (*HandleResponse, error) {
	meta, err := s.Inspect(InspectRequest{Target: target, Intent: intent, GrantToken: grantToken})
	if err != nil {
		return nil, err
	}
	if !meta.Exists {
		return nil, fmt.Errorf("resource does not exist")
	}
	authority, path, roots, err := s.resolveFileTarget(target, grantToken)
	if err != nil {
		return nil, err
	}

	inline := intent == "render" && isRenderableMIME(meta.MIMEType)
	if inline {
		stableKey := buildStableHandleKey(authority, roots, target, intent)
		s.handleMu.RLock()
		existingID, ok := s.stableHandleIDs[stableKey]
		existing := s.handles[existingID]
		s.handleMu.RUnlock()
		if ok && existing != nil {
			return handleResponse(existing), nil
		}

		handleID := "rh-" + uuid.NewString()
		record := &handleRecord{
			ID:              handleID,
			Intent:          intent,
			Target:          target,
			Path:            path,
			MIMEType:        meta.MIMEType,
			Size:            meta.Size,
			EntryType:       meta.EntryType,
			EpubPackagePath: meta.EpubPackagePath,
			Inline:          true,
		}
		s.handleMu.Lock()
		if currentID, ok := s.stableHandleIDs[stableKey]; ok {
			if current := s.handles[currentID]; current != nil {
				s.handleMu.Unlock()
				return handleResponse(current), nil
			}
		}
		s.handles[handleID] = record
		s.stableHandleIDs[stableKey] = handleID
		s.handleMu.Unlock()
		return handleResponse(record), nil
	}

	handleID := "rh-" + uuid.NewString()
	expiresAt := time.Now().Add(handleTTL)
	record := &handleRecord{
		ID:        handleID,
		Intent:    intent,
		Target:    target,
		Path:      path,
		MIMEType:  meta.MIMEType,
		Size:      meta.Size,
		EntryType: meta.EntryType,
		ExpiresAt: expiresAt,
		Inline:    false,
	}
	s.handleMu.Lock()
	s.handles[handleID] = record
	s.handleMu.Unlock()
	return &HandleResponse{
		HandleID:        handleID,
		URL:             "/v1/resources/content/" + handleID,
		ExpiresAt:       expiresAt.UTC().Format(time.RFC3339),
		Intent:          intent,
		MIMEType:        meta.MIMEType,
		Size:            meta.Size,
		EntryType:       meta.EntryType,
		EpubPackagePath: meta.EpubPackagePath,
	}, nil
}

func (s *Service) GetHandle(id string) (*handleRecord, bool) {
	s.handleMu.RLock()
	record, ok := s.handles[strings.TrimSpace(id)]
	s.handleMu.RUnlock()
	if !ok {
		return nil, false
	}
	if !record.ExpiresAt.IsZero() && time.Now().After(record.ExpiresAt) {
		return nil, false
	}
	return record, true
}

func (s *Service) CreateImportSession(req CreateImportSessionRequest) (*CreateImportSessionResponse, error) {
	if req.Size <= 0 {
		return nil, fmt.Errorf("size must be greater than zero")
	}
	authority, targetDocumentPath, _, err := s.resolveFileTarget(Target{Kind: "file", URI: req.TargetDocumentURI}, req.GrantToken)
	if err != nil {
		return nil, err
	}
	storagePath, documentRef, err := s.resolveImportDestination(req.Purpose, targetDocumentPath, req.FileName, req.MIMEType)
	if err != nil {
		return nil, err
	}
	sessionID := "rs-" + uuid.NewString()
	target := Target{Kind: "file", URI: buildCanonicalFileURI(authority, storagePath)}
	session := &importSession{ID: sessionID, Purpose: req.Purpose, Authority: authority, TargetDocumentURI: req.TargetDocumentURI, StoragePath: storagePath, DocumentRef: documentRef, Target: target, FileName: filepath.Base(storagePath), MIMEType: req.MIMEType, Size: req.Size, SHA256: strings.ToLower(strings.TrimSpace(req.SHA256)), ExpiresAt: time.Now().Add(handleTTL)}
	s.sessionMu.Lock()
	s.sessions[sessionID] = session
	s.sessionMu.Unlock()
	return &CreateImportSessionResponse{SessionID: sessionID, UploadURL: "/v1/resources/import-sessions/" + sessionID + "/content", ExpectedDocumentRef: documentRef, ProvisionalTarget: target}, nil
}

func (s *Service) GetImportSession(id string) (*importSession, bool) {
	s.sessionMu.RLock()
	sess, ok := s.sessions[strings.TrimSpace(id)]
	s.sessionMu.RUnlock()
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}

func (s *Service) UploadImportSession(id string, body io.Reader, contentLength int64) (*ImportResult, error) {
	sess, ok := s.GetImportSession(id)
	if !ok {
		return nil, os.ErrNotExist
	}
	if contentLength > 0 && contentLength != sess.Size {
		return nil, fmt.Errorf("content length mismatch")
	}
	if err := os.MkdirAll(filepath.Dir(sess.StoragePath), 0o755); err != nil {
		return nil, err
	}
	tmpPath := sess.StoragePath + ".part"
	_ = os.Remove(tmpPath)
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(f, hasher), io.LimitReader(body, sess.Size+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return nil, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return nil, closeErr
	}
	if written != sess.Size {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("written size mismatch")
	}
	if sess.SHA256 != "" && hex.EncodeToString(hasher.Sum(nil)) != sess.SHA256 {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("sha256 mismatch")
	}
	if err := os.Rename(tmpPath, sess.StoragePath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	grant, err := s.CreateGrant(CreateGrantRequest{Authority: sess.Authority, Roots: []string{filepath.Dir(sess.StoragePath)}})
	if err != nil {
		return nil, err
	}
	handle, err := s.CreateHandle(sess.Target, "render", grant.GrantToken)
	if err != nil {
		return nil, err
	}
	s.sessionMu.Lock()
	delete(s.sessions, id)
	s.sessionMu.Unlock()
	return &ImportResult{DocumentRef: sess.DocumentRef, Target: sess.Target, RenderHandle: handle}, nil
}

func (s *Service) resolveImportDestination(purpose ImportPurpose, targetDocumentPath, rawFileName, mimeType string) (string, string, error) {
	baseDir := filepath.Dir(targetDocumentPath)
	sanitizedName, ext, err := sanitizeFileName(rawFileName, mimeType)
	if err != nil {
		return "", "", err
	}
	var targetDir string
	switch purpose {
	case ImportPurposeMarkdownImage:
		targetDir = filepath.Join(baseDir, "assets")
	case ImportPurposeAttachment:
		targetDir = filepath.Join(baseDir, "files")
	default:
		return "", "", fmt.Errorf("unsupported import purpose")
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", "", err
	}
	storagePath, _, err := uniqueFilePath(targetDir, sanitizedName, ext)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(baseDir, storagePath)
	if err != nil {
		return "", "", err
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return storagePath, rel, nil
}

func sanitizeFileName(rawFileName, mimeType string) (string, string, error) {
	name := strings.TrimSpace(filepath.Base(rawFileName))
	ext := strings.ToLower(filepath.Ext(name))
	base := strings.TrimSuffix(name, ext)
	base = safeNameRE.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-._")
	if base == "" {
		base = "file"
	}
	if ext == "" {
		exts, _ := mime.ExtensionsByType(mimeType)
		if len(exts) > 0 {
			ext = strings.ToLower(exts[0])
		}
	}
	if ext == "" {
		return "", "", fmt.Errorf("file extension is required")
	}
	return base + ext, ext, nil
}

func uniqueFilePath(dir, fileName, ext string) (string, string, error) {
	name := strings.TrimSuffix(fileName, ext)
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

func detectMIME(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if mimeType := imageMIMEs[ext]; mimeType != "" {
		return mimeType
	}
	if mimeType := resourceMIMEs[ext]; mimeType != "" {
		return mimeType
	}
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		return mimeType
	}
	return "application/octet-stream"
}
