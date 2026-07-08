package agentctx

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	mdutil "github.com/colinagent/openbrain/opagent-runtime/packages/agentctx/md"
)

type ChatProjectionFile struct {
	ThreadID       string
	Title          string
	ChatPath       string
	ParentThreadID string
}

func DefaultConversationWorkdir(baseDir string) string {
	trimmedBaseDir := strings.TrimSpace(baseDir)
	if trimmedBaseDir == "" {
		return ""
	}
	return filepath.Join(filepath.Clean(trimmedBaseDir), "workspace")
}

func BuildChatDir(baseDir string) (string, error) {
	trimmedBaseDir := strings.TrimSpace(baseDir)
	if trimmedBaseDir == "" {
		return "", fmt.Errorf("cwd is required")
	}
	chatDir := filepath.Join(filepath.Clean(trimmedBaseDir), ".agent", "chat")
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		return "", err
	}
	return chatDir, nil
}

func BuildUniqueChatPath(cwd, title string) (string, error) {
	chatDir, err := BuildChatDir(cwd)
	if err != nil {
		return "", err
	}
	base := mdutil.SlugifyThreadTitle(title)
	return BuildUniqueChatPathInDir(chatDir, base+".md")
}

func BuildUniqueChatPathForFileName(baseDir, fileName string) (string, error) {
	chatDir, err := BuildChatDir(baseDir)
	if err != nil {
		return "", err
	}
	normalizedFileName, err := NormalizeChatFileName(fileName)
	if err != nil {
		return "", err
	}
	return BuildUniqueChatPathInDir(chatDir, normalizedFileName)
}

func BuildUniqueChatPathInDir(chatDir, fileName string) (string, error) {
	trimmedFileName := strings.TrimSpace(fileName)
	if trimmedFileName == "" {
		return "", fmt.Errorf("chat file name is required")
	}
	ext := filepath.Ext(trimmedFileName)
	base := strings.TrimSuffix(trimmedFileName, ext)
	if base == "" {
		return "", fmt.Errorf("chat file name is invalid")
	}
	candidate := filepath.Join(chatDir, trimmedFileName)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, nil
	} else if err != nil {
		return "", err
	}
	for i := 2; i < 10000; i++ {
		next := filepath.Join(chatDir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if _, err := os.Stat(next); os.IsNotExist(err) {
			return next, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("unable to allocate chat markdown path")
}

func NormalizeChatFileName(fileName string) (string, error) {
	trimmedFileName := strings.TrimSpace(fileName)
	if trimmedFileName == "" {
		return "", fmt.Errorf("chat file name is required")
	}
	if trimmedFileName != filepath.Base(trimmedFileName) || strings.Contains(trimmedFileName, "\\") {
		return "", fmt.Errorf("chat file name must be a base file name")
	}
	if !strings.HasSuffix(strings.ToLower(trimmedFileName), ".md") {
		trimmedFileName += ".md"
	}
	if strings.TrimSuffix(trimmedFileName, filepath.Ext(trimmedFileName)) == "" {
		return "", fmt.Errorf("chat file name is invalid")
	}
	return trimmedFileName, nil
}

func EnsureChatProjectionFile(file ChatProjectionFile) error {
	threadID := strings.TrimSpace(file.ThreadID)
	chatPath := strings.TrimSpace(file.ChatPath)
	title := strings.TrimSpace(file.Title)
	if threadID == "" {
		return fmt.Errorf("threadID is required")
	}
	if chatPath == "" {
		return fmt.Errorf("chatPath is required")
	}
	if err := os.MkdirAll(filepath.Dir(chatPath), 0o755); err != nil {
		return err
	}
	info, err := os.Stat(chatPath)
	if err == nil && info.Size() > 0 {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if title == "" {
		return fmt.Errorf("title is required")
	}
	return os.WriteFile(chatPath, []byte(ChatProjectionFrontmatter(file)), 0o644)
}

func ChatProjectionFrontmatter(file ChatProjectionFile) string {
	var b strings.Builder
	threadID := strings.TrimSpace(file.ThreadID)
	title := strings.TrimSpace(file.Title)
	b.WriteString("---\n")
	b.WriteString("thread: ")
	b.WriteString(threadID)
	b.WriteString("\n")
	b.WriteString("title: ")
	b.WriteString(strconv.Quote(title))
	if parentThreadID := strings.TrimSpace(file.ParentThreadID); parentThreadID != "" {
		b.WriteString("\nparent_thread: ")
		b.WriteString(parentThreadID)
	}
	b.WriteString("\n")
	b.WriteString("---\n\n")
	return b.String()
}
