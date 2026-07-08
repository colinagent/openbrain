package agentctx

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ChatFileMeta is the canonical metadata parsed from chat markdown frontmatter.
// It is used only for projection/index maintenance. Runtime session truth lives
// in session JSONL, not markdown.
type ChatFileMeta struct {
	ThreadID  string
	Protocol  string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

const ChatProtocolV2 = "chat-v2"

// ReadChatFileMeta extracts metadata from YAML frontmatter (first --- block).
func ReadChatFileMeta(path string) (*ChatFileMeta, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	const maxFrontmatterLines = 64
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 8*1024), 1024*1024)

	meta := &ChatFileMeta{}
	started := false
	inFrontmatter := false
	for lineNo := 0; lineNo < maxFrontmatterLines && scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if !inFrontmatter {
			if line == "" {
				continue
			}
			if line != "---" {
				return nil, fmt.Errorf("invalid frontmatter: %s", line)
			}
			started = true
			inFrontmatter = true
			continue
		}
		if line == "---" || line == "..." {
			if strings.TrimSpace(meta.ThreadID) == "" {
				return nil, fmt.Errorf("invalid frontmatter: missing thread")
			}
			return meta, nil
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(key))
		value = strings.TrimSpace(value)

		switch key {
		case "thread":
			threadTarget := parseFrontmatterString(value)
			if strings.HasPrefix(threadTarget, "thread-") {
				meta.ThreadID = strings.TrimSpace(threadTarget)
			}
		case "protocol":
			meta.Protocol = parseFrontmatterString(value)
		case "title":
			meta.Title = parseFrontmatterString(value)
		case "created_at", "createdat":
			t, err := parseFrontmatterTime(value)
			if err != nil {
				return nil, fmt.Errorf("invalid frontmatter created_at: %w", err)
			}
			meta.CreatedAt = t
		case "updated_at", "updatedat":
			t, err := parseFrontmatterTime(value)
			if err != nil {
				return nil, fmt.Errorf("invalid frontmatter updated_at: %w", err)
			}
			meta.UpdatedAt = t
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !started {
		return nil, fmt.Errorf("frontmatter not found")
	}
	if inFrontmatter {
		return nil, fmt.Errorf("invalid frontmatter: missing closing marker")
	}
	return nil, fmt.Errorf("invalid frontmatter")
}

func parseFrontmatterString(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(v); err == nil {
		return unquoted
	}
	return strings.Trim(v, "\"'")
}

func parseFrontmatterTime(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
		return time.Unix(ts, 0).UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", v)
}
