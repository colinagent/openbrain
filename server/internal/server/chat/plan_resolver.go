package chat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

type planToolCall struct {
	Name string
	Path string
}

func resolveLatestPlanPath(meta *op.ThreadMeta) (string, error) {
	if meta == nil {
		return "", nil
	}
	threadFilePath := strings.TrimSpace(meta.ThreadFilePath)
	cwd := strings.TrimSpace(meta.CWD)
	if threadFilePath == "" || cwd == "" {
		return "", nil
	}

	f, err := os.Open(threadFilePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	toolCalls := make(map[string]planToolCall)
	candidates := make([]string, 0, 4)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNo++
		if lineNo == 1 {
			continue
		}
		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &base); err != nil {
			return "", fmt.Errorf("parse thread entry: %w", err)
		}
		if strings.TrimSpace(base.Type) != op.ThreadEntryTypeCanonicalMessage {
			continue
		}

		var entry op.ThreadCanonicalMessageEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return "", fmt.Errorf("decode canonical thread message entry: %w", err)
		}
		msg := entry.Message

		switch msg.Role {
		case ai.RoleCanonicalAssistant:
			for _, block := range msg.Content {
				if block.Type != ai.BlockToolCall || block.ToolCall == nil {
					continue
				}
				toolName := normalizePlanToolName(block.ToolCall.Name)
				if toolName != "write" && toolName != "edit" {
					continue
				}
				rawPath, ok := extractPlanPathFromToolArguments(block.ToolCall.Arguments)
				if !ok {
					continue
				}
				resolvedPath, ok := resolvePlanCandidatePath(rawPath, cwd)
				if !ok || !isPlanMarkdownPath(resolvedPath, cwd) {
					continue
				}
				toolCallID := strings.TrimSpace(block.ToolCall.ID)
				if toolCallID == "" {
					continue
				}
				toolCalls[toolCallID] = planToolCall{
					Name: toolName,
					Path: resolvedPath,
				}
			}
		case ai.RoleCanonicalTool:
			for _, block := range msg.Content {
				if block.Type != ai.BlockToolResult || block.ToolResult == nil {
					continue
				}
				toolCallID := strings.TrimSpace(block.ToolResult.ToolCallID)
				if toolCallID == "" {
					continue
				}
				call, ok := toolCalls[toolCallID]
				if !ok {
					continue
				}
				if isPlanToolResultError(block.ToolResult.OutputText) {
					continue
				}
				candidates = append(candidates, call.Path)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	seen := make(map[string]struct{}, len(candidates))
	for index := len(candidates) - 1; index >= 0; index -= 1 {
		candidate := filepath.Clean(strings.TrimSpace(candidates[index]))
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if err := verifyPlanFile(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", nil
}

func normalizePlanToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func extractPlanPathFromToolArguments(arguments map[string]any) (string, bool) {
	return toolArgumentsMap(arguments).stringValue("path")
}

func resolvePlanCandidatePath(rawPath string, cwd string) (string, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(rawPath, "@"))
	if trimmed == "" {
		return "", false
	}
	if trimmed == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		return filepath.Clean(home), true
	}
	if strings.HasPrefix(trimmed, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		return filepath.Clean(filepath.Join(home, trimmed[2:])), true
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed), true
	}
	normalizedCwd := strings.TrimSpace(cwd)
	if normalizedCwd == "" {
		return "", false
	}
	return filepath.Clean(filepath.Join(normalizedCwd, trimmed)), true
}

func isPlanMarkdownPath(path string, cwd string) bool {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	cleanCwd := filepath.Clean(strings.TrimSpace(cwd))
	if cleanPath == "" || cleanCwd == "" {
		return false
	}
	if !strings.EqualFold(filepath.Ext(cleanPath), ".md") {
		return false
	}
	planDir := filepath.Join(cleanCwd, ".agent", "context")
	return cleanPath == planDir || strings.HasPrefix(cleanPath, planDir+string(os.PathSeparator))
}

func verifyPlanFile(path string) error {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" {
		return fmt.Errorf("missing plan path")
	}
	info, err := os.Stat(cleanPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("plan path is dir")
	}
	body, err := os.ReadFile(cleanPath)
	if err != nil {
		return err
	}
	content := strings.ReplaceAll(string(body), "\r\n", "\n")
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("plan file is empty")
	}
	if strings.Contains(content, "<!-- openbrain-plan-seed -->") {
		return fmt.Errorf("plan file is seed")
	}
	return verifyPlanTaskSection(content)
}

func verifyPlanTaskSection(content string) error {
	lines := strings.Split(content, "\n")
	taskSectionIndexes := make([]int, 0, 2)
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Tasks" || trimmed == "## 任务" {
			taskSectionIndexes = append(taskSectionIndexes, index)
		}
	}
	if len(taskSectionIndexes) != 1 {
		return fmt.Errorf("invalid plan task sections")
	}
	start := taskSectionIndexes[0] + 1
	end := len(lines)
	for index := start; index < len(lines); index += 1 {
		trimmed := strings.TrimSpace(lines[index])
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
			end = index
			break
		}
	}
	for index := start; index < end; index += 1 {
		if isPlanChecklistLine(lines[index]) {
			return nil
		}
	}
	return fmt.Errorf("missing checklist")
}

func isPlanChecklistLine(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	prefixes := []string{
		"- [ ] ", "- [x] ", "- [X] ",
		"* [ ] ", "* [x] ", "* [X] ",
		"+ [ ] ", "+ [x] ", "+ [X] ",
	}
	for _, prefix := range prefixes {
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		return strings.TrimSpace(trimmed[len(prefix):]) != ""
	}
	return false
}

func isPlanToolResultError(result string) bool {
	lower := strings.ToLower(strings.TrimSpace(result))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "failed") ||
		strings.Contains(lower, "error") ||
		strings.Contains(lower, "not found") ||
		strings.Contains(lower, "invalid")
}
