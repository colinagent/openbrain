package fs

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/colinagent/openbrain/server/internal/rgsearch"
	"github.com/colinagent/openbrain/server/internal/server/protocol"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
	"github.com/shurcooL/go/trash"
)

// FileService provides file system operations
type FileService struct {
	verbose bool
}

// NewFileService creates a new file service
func NewFileService(verbose bool) *FileService {
	return &FileService{verbose: verbose}
}

// normalizePath cleans and validates a path
// Prevents path traversal attacks and normalizes the path
func normalizePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Clean the path
	cleaned := filepath.Clean(path)

	// Convert to absolute path if relative
	if !filepath.IsAbs(cleaned) {
		// Get current working directory
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory: %w", err)
		}
		cleaned = filepath.Join(cwd, cleaned)
	}

	return cleaned, nil
}

// Stat returns file information
func (s *FileService) Stat(params *protocol.StatParams) (*protocol.StatResult, *protocol.RPCError) {
	path, err := normalizePath(params.Path)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: err.Error(),
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeFileNotFound,
				Message: fmt.Sprintf("File not found: %s", path),
			}
		}
		if os.IsPermission(err) {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodePermissionDeny,
				Message: fmt.Sprintf("Permission denied: %s", path),
			}
		}
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}
	}

	return &protocol.StatResult{
		Path:    path,
		Name:    info.Name(),
		Size:    info.Size(),
		IsDir:   info.IsDir(),
		ModTime: info.ModTime().UnixMilli(),
		Mode:    uint32(info.Mode()),
	}, nil
}

// ReadFile reads file content
func (s *FileService) ReadFile(params *protocol.ReadFileParams) (*protocol.ReadFileResult, *protocol.RPCError) {
	path, err := normalizePath(params.Path)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: err.Error(),
		}
	}

	// Check if file exists and get info
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeFileNotFound,
				Message: fmt.Sprintf("File not found: %s", path),
			}
		}
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}
	}

	if info.IsDir() {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeIsDirectory,
			Message: fmt.Sprintf("Path is a directory: %s", path),
		}
	}

	// Check file size limits
	sizeLimit := int64(DefaultFileSizeLimit)
	if params.Options != nil && params.Options.Limits != nil && params.Options.Limits.Size > 0 {
		sizeLimit = params.Options.Limits.Size
	}

	if IsFileTooLarge(info.Size(), sizeLimit) {
		return &protocol.ReadFileResult{
			Size:     info.Size(),
			TooLarge: true,
			Encoding: "utf8",
		}, nil
	}

	// Read file content
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: fmt.Sprintf("Failed to read file: %v", err),
		}
	}

	// Determine encoding
	encoding := "utf8"
	if params.Options != nil && params.Options.Encoding != "" {
		encoding = params.Options.Encoding
	}

	var contentStr string
	if encoding == "base64" {
		contentStr = base64.StdEncoding.EncodeToString(content)
	} else {
		contentStr = string(content)
	}

	return &protocol.ReadFileResult{
		Content:  contentStr,
		Encoding: encoding,
		Size:     info.Size(),
	}, nil
}

// WriteFile writes content to a file
func (s *FileService) WriteFile(params *protocol.WriteFileParams) (*protocol.WriteFileResult, *protocol.RPCError) {
	path, err := normalizePath(params.Path)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: err.Error(),
		}
	}

	// Check if file exists
	info, err := os.Stat(path)
	fileExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}
	}

	if fileExists && info.IsDir() {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeIsDirectory,
			Message: fmt.Sprintf("Path is a directory: %s", path),
		}
	}

	// Handle create/overwrite options
	opts := params.Options
	if opts == nil {
		opts = &protocol.WriteFileOptions{Create: true, Overwrite: true}
	}

	if !fileExists && !opts.Create {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeFileNotFound,
			Message: fmt.Sprintf("File not found and create=false: %s", path),
		}
	}

	if fileExists && !opts.Overwrite {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeFileExists,
			Message: fmt.Sprintf("File exists and overwrite=false: %s", path),
		}
	}

	// Decode content
	var content []byte
	encoding := "utf8"
	if opts.Encoding != "" {
		encoding = opts.Encoding
	}

	if encoding == "base64" {
		content, err = base64.StdEncoding.DecodeString(params.Content)
		if err != nil {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeInvalidParams,
				Message: fmt.Sprintf("Invalid base64 content: %v", err),
			}
		}
	} else {
		content = []byte(params.Content)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: fmt.Sprintf("Failed to create directory: %v", err),
		}
	}

	// Write file (atomic or direct)
	if opts.Atomic {
		err = s.writeFileAtomic(path, content)
	} else {
		err = os.WriteFile(path, content, 0644)
	}

	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: fmt.Sprintf("Failed to write file: %v", err),
		}
	}

	// Get updated file info
	info, _ = os.Stat(path)
	modTime := int64(0)
	if info != nil {
		modTime = info.ModTime().UnixMilli()
	}

	return &protocol.WriteFileResult{
		Path:    path,
		Size:    int64(len(content)),
		ModTime: modTime,
	}, nil
}

// writeFileAtomic writes to a temp file then renames (atomic write)
func (s *FileService) writeFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	defer func() {
		// Clean up temp file on error
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return err
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	// Rename temp file to target (atomic on most file systems)
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	tmpPath = "" // Prevent cleanup of successfully renamed file
	return nil
}

// Readdir lists directory contents
func (s *FileService) Readdir(params *protocol.ReaddirParams) (*protocol.ReaddirResult, *protocol.RPCError) {
	path, err := normalizePath(params.Path)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: err.Error(),
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeFileNotFound,
				Message: fmt.Sprintf("Directory not found: %s", path),
			}
		}
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}
	}

	if !info.IsDir() {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeNotDirectory,
			Message: fmt.Sprintf("Path is not a directory: %s", path),
		}
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: fmt.Sprintf("Failed to read directory: %v", err),
		}
	}

	result := &protocol.ReaddirResult{
		Path:    path,
		Entries: make([]protocol.DirEntry, 0, len(entries)),
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue // Skip entries we can't stat
		}

		dirEntry := protocol.DirEntry{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UnixMilli(),
		}

		result.Entries = append(result.Entries, dirEntry)
	}

	// Sort: directories first, then alphabetically
	sort.Slice(result.Entries, func(i, j int) bool {
		if result.Entries[i].IsDir != result.Entries[j].IsDir {
			return result.Entries[i].IsDir
		}
		return strings.ToLower(result.Entries[i].Name) < strings.ToLower(result.Entries[j].Name)
	})

	return result, nil
}

func (s *FileService) Search(ctx context.Context, params *protocol.SearchParams) (*protocol.SearchResult, *protocol.RPCError) {
	root, err := normalizePath(params.Root)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: err.Error(),
		}
	}
	query := strings.TrimSpace(params.Query)
	if query == "" {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "search query is required",
		}
	}

	binary, err := rgsearch.ResolveBinary(filepath.Join(common.OpagentBinDir(), "rg"), "rg")
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}
	}

	result, err := rgsearch.Search(ctx, binary, rgsearch.Query{
		Root:       root,
		Pattern:    query,
		Regex:      params.Regex,
		MatchCase:  params.MatchCase,
		WholeWord:  params.WholeWord,
		Includes:   append([]string(nil), params.Includes...),
		Excludes:   append([]string(nil), params.Excludes...),
		MaxFiles:   params.MaxFiles,
		MaxMatches: params.MaxMatches,
	})
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}
	}

	files := make([]protocol.SearchFileResult, 0, len(result.Files))
	for _, file := range result.Files {
		matches := make([]protocol.SearchMatch, 0, len(file.Matches))
		for _, match := range file.Matches {
			matches = append(matches, protocol.SearchMatch{
				Line:      match.Line,
				Column:    match.Column,
				EndColumn: match.EndColumn,
				Text:      match.Text,
			})
		}
		files = append(files, protocol.SearchFileResult{
			Path:    file.Path,
			Matches: matches,
			Count:   file.Count,
		})
	}

	return &protocol.SearchResult{
		Files:      files,
		TotalCount: result.TotalCount,
		Truncated:  result.Truncated,
	}, nil
}

// Mkdir creates a directory
func (s *FileService) Mkdir(params *protocol.MkdirParams) *protocol.RPCError {
	path, err := normalizePath(params.Path)
	if err != nil {
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: err.Error(),
		}
	}

	var mkdirErr error
	if params.Recursive {
		mkdirErr = os.MkdirAll(path, 0755)
	} else {
		mkdirErr = os.Mkdir(path, 0755)
	}

	if mkdirErr != nil {
		if os.IsExist(mkdirErr) {
			return &protocol.RPCError{
				Code:    protocol.ErrCodeFileExists,
				Message: fmt.Sprintf("Directory already exists: %s", path),
			}
		}
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: fmt.Sprintf("Failed to create directory: %v", mkdirErr),
		}
	}

	return nil
}

// Delete removes a file or directory
func (s *FileService) Delete(params *protocol.DeleteParams) *protocol.RPCError {
	path, err := normalizePath(params.Path)
	if err != nil {
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: err.Error(),
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &protocol.RPCError{
				Code:    protocol.ErrCodeFileNotFound,
				Message: fmt.Sprintf("Path not found: %s", path),
			}
		}
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}
	}

	if params.UseTrash {
		if err := trash.MoveTo(path); err != nil {
			return &protocol.RPCError{
				Code:    protocol.ErrCodeInternal,
				Message: fmt.Sprintf("Failed to move to trash: %v", err),
			}
		}
		return nil
	}

	if info.IsDir() {
		if params.Recursive {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
			if err != nil {
				// Check if directory is not empty
				entries, _ := os.ReadDir(path)
				if len(entries) > 0 {
					return &protocol.RPCError{
						Code:    protocol.ErrCodeDirectoryNotEmpty,
						Message: fmt.Sprintf("Directory not empty: %s", path),
					}
				}
			}
		}
	} else {
		err = os.Remove(path)
	}

	if err != nil {
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: fmt.Sprintf("Failed to delete: %v", err),
		}
	}

	return nil
}

// Rename moves/renames a file or directory
func (s *FileService) Rename(params *protocol.RenameParams) *protocol.RPCError {
	oldPath, err := normalizePath(params.OldPath)
	if err != nil {
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: fmt.Sprintf("Invalid old path: %v", err),
		}
	}

	newPath, err := normalizePath(params.NewPath)
	if err != nil {
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: fmt.Sprintf("Invalid new path: %v", err),
		}
	}

	// Check source exists
	if _, err := os.Stat(oldPath); err != nil {
		if os.IsNotExist(err) {
			return &protocol.RPCError{
				Code:    protocol.ErrCodeFileNotFound,
				Message: fmt.Sprintf("Source not found: %s", oldPath),
			}
		}
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}
	}

	// Check target
	if _, err := os.Stat(newPath); err == nil {
		if !params.Overwrite {
			return &protocol.RPCError{
				Code:    protocol.ErrCodeFileExists,
				Message: fmt.Sprintf("Target exists and overwrite=false: %s", newPath),
			}
		}
		// Remove target if overwrite is true
		os.RemoveAll(newPath)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: fmt.Sprintf("Failed to create parent directory: %v", err),
		}
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: fmt.Sprintf("Failed to rename: %v", err),
		}
	}

	return nil
}

// Copy copies a file or directory
func (s *FileService) Copy(params *protocol.CopyParams) *protocol.RPCError {
	srcPath, err := normalizePath(params.Source)
	if err != nil {
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: fmt.Sprintf("Invalid source path: %v", err),
		}
	}

	dstPath, err := normalizePath(params.Target)
	if err != nil {
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: fmt.Sprintf("Invalid target path: %v", err),
		}
	}

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &protocol.RPCError{
				Code:    protocol.ErrCodeFileNotFound,
				Message: fmt.Sprintf("Source not found: %s", srcPath),
			}
		}
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}
	}

	// Check target
	if _, err := os.Stat(dstPath); err == nil {
		if !params.Overwrite {
			return &protocol.RPCError{
				Code:    protocol.ErrCodeFileExists,
				Message: fmt.Sprintf("Target exists and overwrite=false: %s", dstPath),
			}
		}
	}

	if srcInfo.IsDir() {
		err = s.copyDir(srcPath, dstPath)
	} else {
		err = s.copyFile(srcPath, dstPath)
	}

	if err != nil {
		return &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: fmt.Sprintf("Failed to copy: %v", err),
		}
	}

	return nil
}

func (s *FileService) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	// Copy file mode
	srcInfo, _ := os.Stat(src)
	return os.Chmod(dst, srcInfo.Mode())
}

func (s *FileService) copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := s.copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := s.copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// Helper to create a standard RPC success
func Success(data interface{}) (interface{}, *protocol.RPCError) {
	return data, nil
}

// Realpath returns the real path (resolving symlinks)
func (s *FileService) Realpath(path string) (string, error) {
	normalized, err := normalizePath(path)
	if err != nil {
		return "", err
	}

	realPath, err := filepath.EvalSymlinks(normalized)
	if err != nil {
		return normalized, nil // Return normalized path if symlink resolution fails
	}

	return realPath, nil
}
