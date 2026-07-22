package remotecontrol

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
	"github.com/colinagent/openbrain/server/internal/rgsearch"
)

const (
	defaultRemoteFilePageSize = 100
	maxRemoteFilePageSize     = 200
	defaultRemoteSearchLimit  = 100
	maxRemoteSearchLimit      = 500
	maxRemoteSearchQueryBytes = 256
	maxRemoteSearchSnippet    = 1_024
	maxRemoteRelativePath     = 4_096
	maxInlinePreviewBytes     = 2 * 1024 * 1024
	maxQuickLookBytes         = 25 * 1024 * 1024
	maxPreviewChunkBytes      = 128 * 1024
	previewHandleTTL          = 5 * time.Minute
	maxPreviewHandles         = 512
	maxPrincipalHandles       = 32
)

var sensitiveDirectoryNames = map[string]struct{}{
	".agent": {}, ".aws": {}, ".git": {}, ".gnupg": {}, ".openbrain": {}, ".ssh": {},
}

var sensitiveFileNames = map[string]struct{}{
	".netrc": {}, ".npmrc": {}, ".pypirc": {}, "application_default_credentials.json": {},
	"authorized_keys": {}, "credentials": {}, "credentials.json": {}, "id_dsa": {},
	"id_ecdsa": {}, "id_ed25519": {}, "id_rsa": {}, "known_hosts": {},
	"service-account.json": {},
}

var sensitiveFileExtensions = map[string]struct{}{
	".cer": {}, ".crt": {}, ".der": {}, ".key": {}, ".mobileprovision": {},
	".p12": {}, ".pem": {}, ".pfx": {},
}

var remoteSearchExcludes = []string{
	".agent/**", "**/.agent/**", ".aws/**", "**/.aws/**", ".git/**", "**/.git/**",
	".gnupg/**", "**/.gnupg/**", ".openbrain/**", "**/.openbrain/**", ".ssh/**", "**/.ssh/**",
	".env", ".env*", "**/.env", "**/.env*", "*.cer", "**/*.cer", "*.crt", "**/*.crt",
	"*.der", "**/*.der", "*.key", "**/*.key", "*.mobileprovision", "**/*.mobileprovision",
	"*.p12", "**/*.p12", "*.pem", "**/*.pem", "*.pfx", "**/*.pfx", ".netrc", "**/.netrc",
	".npmrc", "**/.npmrc", ".pypirc", "**/.pypirc", "credentials", "**/credentials",
	"credentials.json", "**/credentials.json", "application_default_credentials.json",
	"**/application_default_credentials.json", "service-account.json", "**/service-account.json",
	"id_dsa", "**/id_dsa", "id_ecdsa", "**/id_ecdsa", "id_ed25519", "**/id_ed25519",
	"id_rsa", "**/id_rsa", "authorized_keys", "**/authorized_keys", "known_hosts", "**/known_hosts",
}

type remoteFileService struct {
	runtime RuntimeView
	now     func() time.Time

	handleMu sync.Mutex
	handles  map[string]previewHandle
}

type previewHandle struct {
	ID            string
	UID           string
	EnvironmentID string
	ClientID      string
	WorkspaceID   string
	RelativePath  string
	AbsolutePath  string
	Size          int64
	ModifiedNanos int64
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

type fileListInput struct {
	WorkspaceID string `json:"workspaceID"`
	Path        string `json:"path,omitempty"`
	Cursor      string `json:"cursor,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

type fileStatInput struct {
	WorkspaceID string `json:"workspaceID"`
	Path        string `json:"path"`
}

type fileSearchInput struct {
	WorkspaceID string `json:"workspaceID"`
	Path        string `json:"path,omitempty"`
	Query       string `json:"query"`
	Limit       int    `json:"limit,omitempty"`
}

type filePreviewOpenInput struct {
	WorkspaceID string `json:"workspaceID"`
	Path        string `json:"path"`
}

type filePreviewChunkInput struct {
	WorkspaceID string `json:"workspaceID"`
	HandleID    string `json:"handleID"`
	Offset      int64  `json:"offset"`
	Limit       int    `json:"limit,omitempty"`
}

type remoteFileEntry struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
	MIMEType   string `json:"mimeType,omitempty"`
}

type fileSearchMatch struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Snippet string `json:"snippet"`
}

type fileCursor struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
}

type resolvedRemotePath struct {
	workspace workspaceAccess
	root      string
	relative  string
	absolute  string
	info      os.FileInfo
}

func RegisterFileHandlers(dispatcher *Dispatcher, runtime RuntimeView) error {
	if dispatcher == nil || runtime == nil {
		return errors.New("remote file dependencies are required")
	}
	service := &remoteFileService{
		runtime: runtime,
		now:     time.Now,
		handles: make(map[string]previewHandle),
	}
	registrations := []struct {
		operation protocol.Operation
		handler   Handler
	}{
		{protocol.OperationFileList, service.list},
		{protocol.OperationFileStat, service.stat},
		{protocol.OperationFileSearch, service.search},
		{protocol.OperationFilePreviewOpen, service.previewOpen},
		{protocol.OperationFilePreviewChunk, service.previewChunk},
	}
	for _, registration := range registrations {
		if err := dispatcher.Register(registration.operation, protocol.CapabilityFileRead, registration.handler); err != nil {
			return err
		}
	}
	return nil
}

func (s *remoteFileService) list(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input fileListInput
	if err := decodeRemotePayload(raw, &input); err != nil {
		return nil, invalidRemoteFileRequest()
	}
	target, remoteErr := s.resolve(ctx, input.WorkspaceID, input.Path)
	if remoteErr != nil {
		return nil, remoteErr
	}
	if !target.info.IsDir() {
		return nil, remoteError(protocol.ErrorOperationDenied, "file path is not a directory")
	}
	entries, err := os.ReadDir(target.absolute)
	if err != nil {
		return nil, remoteFileSystemError(err)
	}
	result := make([]remoteFileEntry, 0, len(entries))
	for _, entry := range entries {
		relative := entry.Name()
		if target.relative != "" {
			relative = path.Join(target.relative, entry.Name())
		}
		resolved, entryErr := resolveRemotePath(target.workspace, target.root, relative)
		if entryErr != nil {
			// Directory listings neither reveal denied paths nor expose escaped symlinks.
			continue
		}
		result = append(result, makeRemoteFileEntry(resolved))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind == "directory"
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	offset, cursorErr := decodeFileCursor(input.Cursor, target.relative)
	if cursorErr != nil || offset > len(result) {
		return nil, invalidRemoteFileRequest()
	}
	limit := clamp(input.Limit, defaultRemoteFilePageSize, maxRemoteFilePageSize)
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	nextCursor := ""
	if end < len(result) {
		nextCursor, err = encodeFileCursor(fileCursor{Path: target.relative, Offset: end})
		if err != nil {
			return nil, internalRemoteError()
		}
	}
	return marshalRemote(map[string]any{
		"workspaceID": target.workspace.ID,
		"path":        target.relative,
		"entries":     result[offset:end],
		"nextCursor":  nextCursor,
	})
}

func (s *remoteFileService) stat(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input fileStatInput
	if err := decodeRemotePayload(raw, &input); err != nil {
		return nil, invalidRemoteFileRequest()
	}
	target, remoteErr := s.resolve(ctx, input.WorkspaceID, input.Path)
	if remoteErr != nil {
		return nil, remoteErr
	}
	return marshalRemote(map[string]any{
		"workspaceID": target.workspace.ID,
		"entry":       makeRemoteFileEntry(target),
	})
}

func (s *remoteFileService) search(ctx context.Context, _ Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input fileSearchInput
	if err := decodeRemotePayload(raw, &input); err != nil {
		return nil, invalidRemoteFileRequest()
	}
	query := strings.TrimSpace(input.Query)
	if query == "" || len(query) > maxRemoteSearchQueryBytes || !utf8.ValidString(query) {
		return nil, invalidRemoteFileRequest()
	}
	target, remoteErr := s.resolve(ctx, input.WorkspaceID, input.Path)
	if remoteErr != nil {
		return nil, remoteErr
	}
	if !target.info.IsDir() {
		return nil, remoteError(protocol.ErrorOperationDenied, "file search path is not a directory")
	}
	binary, err := rgsearch.ResolveBinary(filepath.Join(common.OpagentBinDir(), "rg"), "rg")
	if err != nil {
		return nil, internalRemoteError()
	}
	limit := clamp(input.Limit, defaultRemoteSearchLimit, maxRemoteSearchLimit)
	searchResult, err := rgsearch.Search(ctx, binary, rgsearch.Query{
		Root: target.absolute, Pattern: query, Regex: false, MatchCase: false,
		Excludes: append([]string(nil), remoteSearchExcludes...), InsensitiveGlobs: true,
		MaxFiles: limit, MaxMatches: limit,
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, &protocol.RemoteError{Code: protocol.ErrorOperationDenied, Message: "file search was canceled"}
		}
		return nil, internalRemoteError()
	}
	matches := make([]fileSearchMatch, 0, limit)
	for _, file := range searchResult.Files {
		relativeToWorkspace, err := filepath.Rel(target.root, file.Path)
		if err != nil {
			continue
		}
		relative := filepath.ToSlash(relativeToWorkspace)
		resolved, entryErr := resolveRemotePath(target.workspace, target.root, relative)
		if entryErr != nil || resolved.info.IsDir() {
			continue
		}
		for _, match := range file.Matches {
			matches = append(matches, fileSearchMatch{
				Path: relative, Name: path.Base(relative), Line: match.Line, Column: match.Column,
				Snippet: truncateUTF8(strings.TrimSpace(match.Text), maxRemoteSearchSnippet),
			})
			if len(matches) == limit {
				break
			}
		}
		if len(matches) == limit {
			break
		}
	}
	return marshalRemote(map[string]any{
		"workspaceID": target.workspace.ID,
		"path":        target.relative,
		"query":       query,
		"results":     matches,
		"truncated":   searchResult.Truncated || len(matches) == limit,
	})
}

func (s *remoteFileService) previewOpen(ctx context.Context, principal Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input filePreviewOpenInput
	if err := decodeRemotePayload(raw, &input); err != nil {
		return nil, invalidRemoteFileRequest()
	}
	target, remoteErr := s.resolve(ctx, input.WorkspaceID, input.Path)
	if remoteErr != nil {
		return nil, remoteErr
	}
	if target.info.IsDir() {
		return nil, remoteError(protocol.ErrorOperationDenied, "directories cannot be previewed")
	}
	if !target.info.Mode().IsRegular() {
		return nil, remoteError(protocol.ErrorOperationDenied, "only regular files can be previewed")
	}
	if target.info.Size() > maxQuickLookBytes {
		return nil, remoteError(protocol.ErrorFileTooLarge, "file exceeds the remote preview size limit")
	}
	file, err := os.Open(target.absolute)
	if err != nil {
		return nil, remoteFileSystemError(err)
	}
	defer file.Close()
	header := make([]byte, 512)
	headerBytes, readErr := file.Read(header)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, internalRemoteError()
	}
	header = header[:headerBytes]
	mimeType := detectRemoteMIME(target.absolute, header)
	inlineText := ""
	inline := false
	if target.info.Size() <= maxInlinePreviewBytes {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, internalRemoteError()
		}
		content, err := io.ReadAll(io.LimitReader(file, maxInlinePreviewBytes+1))
		if err != nil {
			return nil, internalRemoteError()
		}
		if isRemoteText(content, mimeType) {
			inlineText = string(content)
			inline = true
		}
	}
	now := s.now()
	handleID, err := newPreviewHandleID()
	if err != nil {
		return nil, internalRemoteError()
	}
	handle := previewHandle{
		ID: handleID, UID: principal.UID, EnvironmentID: principal.EnvironmentID,
		ClientID: principal.ClientID, WorkspaceID: target.workspace.ID,
		RelativePath: target.relative, AbsolutePath: target.absolute,
		Size: target.info.Size(), ModifiedNanos: target.info.ModTime().UnixNano(),
		CreatedAt: now, ExpiresAt: now.Add(previewHandleTTL),
	}
	s.storeHandle(handle)
	return marshalRemote(map[string]any{
		"workspaceID": target.workspace.ID,
		"handleID":    handle.ID,
		"path":        target.relative,
		"name":        path.Base(target.relative),
		"mimeType":    mimeType,
		"size":        target.info.Size(),
		"modifiedAt":  target.info.ModTime().UTC().Format(time.RFC3339Nano),
		"expiresAt":   handle.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"inline":      inline,
		"inlineText":  inlineText,
	})
}

func (s *remoteFileService) previewChunk(ctx context.Context, principal Principal, raw json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
	var input filePreviewChunkInput
	if err := decodeRemotePayload(raw, &input); err != nil || input.Offset < 0 {
		return nil, invalidRemoteFileRequest()
	}
	handle, ok := s.loadHandle(strings.TrimSpace(input.HandleID), principal, strings.TrimSpace(input.WorkspaceID))
	if !ok {
		return nil, expiredPreviewError()
	}
	target, remoteErr := s.resolve(ctx, handle.WorkspaceID, handle.RelativePath)
	if remoteErr != nil || target.absolute != handle.AbsolutePath || !target.info.Mode().IsRegular() ||
		target.info.Size() != handle.Size || target.info.ModTime().UnixNano() != handle.ModifiedNanos {
		s.deleteHandle(handle.ID)
		return nil, expiredPreviewError()
	}
	if input.Offset > handle.Size {
		return nil, invalidRemoteFileRequest()
	}
	limit := input.Limit
	if limit <= 0 {
		limit = maxPreviewChunkBytes
	}
	if limit > maxPreviewChunkBytes {
		return nil, invalidRemoteFileRequest()
	}
	content := make([]byte, min(int64(limit), handle.Size-input.Offset))
	if len(content) > 0 {
		file, err := os.Open(handle.AbsolutePath)
		if err != nil {
			return nil, expiredPreviewError()
		}
		defer file.Close()
		readBytes, err := file.ReadAt(content, input.Offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, internalRemoteError()
		}
		content = content[:readBytes]
	}
	nextOffset := input.Offset + int64(len(content))
	return marshalRemote(map[string]any{
		"handleID":   handle.ID,
		"offset":     input.Offset,
		"nextOffset": nextOffset,
		"dataBase64": base64.StdEncoding.EncodeToString(content),
		"eof":        nextOffset >= handle.Size,
	})
}

func (s *remoteFileService) resolve(ctx context.Context, workspaceID, relativePath string) (resolvedRemotePath, *protocol.RemoteError) {
	workspace, err := defaultWorkspace(ctx, s.runtime)
	if err != nil {
		return resolvedRemotePath{}, internalRemoteError()
	}
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(workspaceID) != workspace.ID {
		return resolvedRemotePath{}, remoteError(protocol.ErrorWorkspaceNotFound, "workspace was not found")
	}
	root, err := filepath.Abs(workspace.Path)
	if err != nil {
		return resolvedRemotePath{}, internalRemoteError()
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return resolvedRemotePath{}, remoteFileSystemError(err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return resolvedRemotePath{}, internalRemoteError()
	}
	return resolveRemotePath(workspace, root, relativePath)
}

func resolveRemotePath(workspace workspaceAccess, root, relativePath string) (resolvedRemotePath, *protocol.RemoteError) {
	relative, remoteErr := normalizeRemoteRelativePath(relativePath)
	if remoteErr != nil {
		return resolvedRemotePath{}, remoteErr
	}
	candidate := root
	if relative != "" {
		candidate = filepath.Join(root, filepath.FromSlash(relative))
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return resolvedRemotePath{}, remoteFileSystemError(err)
	}
	containedRelative, err := filepath.Rel(root, resolved)
	if err != nil || containedRelative == ".." || strings.HasPrefix(containedRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(containedRelative) {
		return resolvedRemotePath{}, remoteError(protocol.ErrorPathOutsideWorkspace, "file path is outside the workspace")
	}
	resolvedRelative := ""
	if containedRelative != "." {
		resolvedRelative = filepath.ToSlash(containedRelative)
	}
	if isSensitiveRemotePath(resolvedRelative) {
		return resolvedRemotePath{}, remoteError(protocol.ErrorSensitivePathDenied, "sensitive workspace path is not remotely accessible")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return resolvedRemotePath{}, remoteFileSystemError(err)
	}
	return resolvedRemotePath{
		workspace: workspace, root: root, relative: relative, absolute: resolved, info: info,
	}, nil
}

func normalizeRemoteRelativePath(value string) (string, *protocol.RemoteError) {
	if len(value) > maxRemoteRelativePath || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || strings.Contains(value, "\\") {
		return "", remoteError(protocol.ErrorPathOutsideWorkspace, "file path is not a normalized workspace-relative path")
	}
	if containsPercentEscape(value) || path.IsAbs(value) || filepath.IsAbs(value) || filepath.VolumeName(value) != "" || looksLikeWindowsDrivePath(value) {
		return "", remoteError(protocol.ErrorPathOutsideWorkspace, "file path is not a normalized workspace-relative path")
	}
	if value == "" {
		return "", nil
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", remoteError(protocol.ErrorPathOutsideWorkspace, "file path is not a normalized workspace-relative path")
		}
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned != value {
		return "", remoteError(protocol.ErrorPathOutsideWorkspace, "file path is not a normalized workspace-relative path")
	}
	if isSensitiveRemotePath(cleaned) {
		return "", remoteError(protocol.ErrorSensitivePathDenied, "sensitive workspace path is not remotely accessible")
	}
	return cleaned, nil
}

func looksLikeWindowsDrivePath(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	return value[0] >= 'a' && value[0] <= 'z' || value[0] >= 'A' && value[0] <= 'Z'
}

func containsPercentEscape(value string) bool {
	for index := 0; index+2 < len(value); index++ {
		if value[index] == '%' && isHex(value[index+1]) && isHex(value[index+2]) {
			return true
		}
	}
	return false
}

func isHex(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func isSensitiveRemotePath(relative string) bool {
	if relative == "" {
		return false
	}
	segments := strings.Split(relative, "/")
	for _, segment := range segments {
		lower := strings.ToLower(segment)
		if _, denied := sensitiveDirectoryNames[lower]; denied {
			return true
		}
	}
	name := strings.ToLower(segments[len(segments)-1])
	if strings.HasPrefix(name, ".env") {
		return true
	}
	if _, denied := sensitiveFileNames[name]; denied {
		return true
	}
	_, denied := sensitiveFileExtensions[strings.ToLower(path.Ext(name))]
	return denied
}

func makeRemoteFileEntry(target resolvedRemotePath) remoteFileEntry {
	kind := "file"
	if target.info.IsDir() {
		kind = "directory"
	}
	name := path.Base(target.relative)
	if target.relative == "" {
		name = target.workspace.Name
	}
	entry := remoteFileEntry{
		Path: target.relative, Name: name, Kind: kind, Size: target.info.Size(),
		ModifiedAt: target.info.ModTime().UTC().Format(time.RFC3339Nano),
	}
	if kind == "file" {
		entry.MIMEType = mime.TypeByExtension(strings.ToLower(filepath.Ext(target.absolute)))
	}
	return entry
}

func detectRemoteMIME(filePath string, header []byte) string {
	if value := mime.TypeByExtension(strings.ToLower(filepath.Ext(filePath))); value != "" {
		return value
	}
	return http.DetectContentType(header)
}

func isRemoteText(content []byte, mimeType string) bool {
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	return strings.HasPrefix(normalized, "text/") || normalized == "application/json" ||
		normalized == "application/javascript" || normalized == "application/xml" ||
		normalized == "application/x-yaml" || normalized == "application/yaml" || normalized == ""
}

func encodeFileCursor(cursor fileCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeFileCursor(value, expectedPath string) (int, error) {
	if value == "" {
		return 0, nil
	}
	if len(value) > protocol.MaxCursorBytes {
		return 0, errors.New("cursor is too large")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, err
	}
	var cursor fileCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return 0, err
	}
	if cursor.Path != expectedPath || cursor.Offset < 0 {
		return 0, errors.New("cursor does not match file path")
	}
	return cursor.Offset, nil
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func newPreviewHandleID() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "preview-" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func (s *remoteFileService) storeHandle(handle previewHandle) {
	s.handleMu.Lock()
	defer s.handleMu.Unlock()
	s.pruneHandlesLocked(handle.CreatedAt)
	principalHandles := make([]previewHandle, 0)
	for _, current := range s.handles {
		if current.UID == handle.UID && current.EnvironmentID == handle.EnvironmentID && current.ClientID == handle.ClientID {
			principalHandles = append(principalHandles, current)
		}
	}
	sort.Slice(principalHandles, func(i, j int) bool { return principalHandles[i].CreatedAt.Before(principalHandles[j].CreatedAt) })
	for len(principalHandles) >= maxPrincipalHandles {
		delete(s.handles, principalHandles[0].ID)
		principalHandles = principalHandles[1:]
	}
	if len(s.handles) >= maxPreviewHandles {
		oldest := handle
		for _, current := range s.handles {
			if oldest.ID == handle.ID || current.CreatedAt.Before(oldest.CreatedAt) {
				oldest = current
			}
		}
		if oldest.ID != handle.ID {
			delete(s.handles, oldest.ID)
		}
	}
	s.handles[handle.ID] = handle
}

func (s *remoteFileService) loadHandle(id string, principal Principal, workspaceID string) (previewHandle, bool) {
	s.handleMu.Lock()
	defer s.handleMu.Unlock()
	now := s.now()
	s.pruneHandlesLocked(now)
	handle, ok := s.handles[id]
	if !ok || handle.UID != principal.UID || handle.EnvironmentID != principal.EnvironmentID ||
		handle.ClientID != principal.ClientID || handle.WorkspaceID != workspaceID || !now.Before(handle.ExpiresAt) {
		return previewHandle{}, false
	}
	return handle, true
}

func (s *remoteFileService) deleteHandle(id string) {
	s.handleMu.Lock()
	delete(s.handles, id)
	s.handleMu.Unlock()
}

func (s *remoteFileService) pruneHandlesLocked(now time.Time) {
	for id, handle := range s.handles {
		if !now.Before(handle.ExpiresAt) {
			delete(s.handles, id)
		}
	}
}

func invalidRemoteFileRequest() *protocol.RemoteError {
	return remoteError(protocol.ErrorInvalidEnvelope, "remote file request is invalid")
}

func expiredPreviewError() *protocol.RemoteError {
	return remoteError(protocol.ErrorPreviewExpired, "file preview handle is invalid or expired")
}

func remoteFileSystemError(err error) *protocol.RemoteError {
	if os.IsNotExist(err) {
		return remoteError(protocol.ErrorFileNotFound, "workspace file was not found")
	}
	if os.IsPermission(err) {
		return remoteError(protocol.ErrorOperationDenied, "workspace file is not readable")
	}
	return internalRemoteError()
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
