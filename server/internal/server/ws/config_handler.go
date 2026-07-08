package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/colinagent/openbrain/server/internal/server/protocol"
	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const (
	configPushMaxFileBytes  = 256 * 1024
	configPushMaxBatchBytes = 2 * 1024 * 1024
)

func (h *Handler) handleConfigPush(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.ConfigPushParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}

	if len(p.Files) == 0 {
		return &protocol.ConfigPushResult{Written: 0}, nil
	}

	userDir, err := h.resolveConfigUserDir()
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Failed to resolve config dir: " + err.Error(),
		}
	}
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Failed to create config dir: " + err.Error(),
		}
	}

	seen := make(map[string]struct{}, len(p.Files))
	total := 0
	written := 0
	for _, file := range p.Files {
		name, err := sanitizeConfigFileName(file.Name)
		if err != nil {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeInvalidParams,
				Message: err.Error(),
			}
		}
		if _, ok := seen[name]; ok {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeInvalidParams,
				Message: fmt.Sprintf("Duplicate file name: %s", name),
			}
		}
		seen[name] = struct{}{}

		size := len([]byte(file.Content))
		if size > configPushMaxFileBytes {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeFileTooLarge,
				Message: fmt.Sprintf("File too large: %s (%d bytes)", name, size),
			}
		}
		total += size
		if total > configPushMaxBatchBytes {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeFileTooLarge,
				Message: fmt.Sprintf("Batch too large: %d bytes", total),
			}
		}

		targetPath := filepath.Join(userDir, name)
		if err := writeFileAtomic(targetPath, []byte(file.Content)); err != nil {
			return nil, &protocol.RPCError{
				Code:    protocol.ErrCodeInternal,
				Message: fmt.Sprintf("Failed to write %s: %v", name, err),
			}
		}
		written++
	}

	return &protocol.ConfigPushResult{Written: written}, nil
}

func (h *Handler) resolveConfigUserDir() (string, error) {
	baseDir := ""
	if session := h.server.GetHostSession(); session != nil {
		result, err := session.OpNode(context.Background(), &op.OpNodeParams{OpCode: op.ConfigSystemGet})
		if err == nil && result != nil {
			if jsonContent, ok := result.Content.(*op.JsonContent); ok {
				var payload interface{}
				if err := json.Unmarshal(jsonContent.Raw, &payload); err == nil {
					baseDir = extractBaseDir(payload)
				}
			}
		}
	}

	if strings.TrimSpace(baseDir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(home, ".openbrain")
	}
	return filepath.Join(baseDir, "configs", "user"), nil
}

func extractBaseDir(payload interface{}) string {
	m, ok := payload.(map[string]interface{})
	if !ok {
		return ""
	}
	if v, ok := m["baseDir"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if sys, ok := m["system"].(map[string]interface{}); ok {
		if v, ok := sys["baseDir"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func sanitizeConfigFileName(input string) (string, error) {
	name := strings.TrimSpace(input)
	if name == "" {
		return "", fmt.Errorf("File name is required")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("Invalid file name: %s", input)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		return "", fmt.Errorf("Only .json files are allowed: %s", input)
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._-", r) {
			continue
		}
		return "", fmt.Errorf("Invalid file name: %s", input)
	}
	return name, nil
}

func writeFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()

	// Keep config writes atomic to avoid partial JSON during sync.
	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	tmpPath = ""
	return nil
}
