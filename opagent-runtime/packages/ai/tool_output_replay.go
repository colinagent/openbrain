package ai

import (
	"fmt"
	"strings"
)

const (
	replayToolOutputMaxLines = 2000
	replayToolOutputMaxBytes = 50 * 1024
)

type replayToolOutputTruncation struct {
	Content         string
	Truncated       bool
	TotalLines      int
	TotalBytes      int
	OutputLines     int
	OutputBytes     int
	TruncatedBy     string
	LastLinePartial bool
}

func truncateToolOutputForReplay(content string) string {
	return TruncateToolOutputForReplayWithLimits(content, replayToolOutputMaxLines, replayToolOutputMaxBytes)
}

func TruncateToolOutputForReplay(content string) string {
	return truncateToolOutputForReplay(content)
}

func TruncateToolOutputForReplayWithLimits(content string, maxLines int, maxBytes int) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if maxLines <= 0 {
		maxLines = 1
	}
	if maxBytes <= 0 {
		maxBytes = 1
	}

	truncation := truncateReplayToolOutputTail(content, maxLines, maxBytes)
	if !truncation.Truncated {
		return content
	}

	if truncation.LastLinePartial && truncation.OutputLines == 1 {
		return truncation.Content + fmt.Sprintf(
			"\n[Historical tool output truncated for replay. Showing last %s of line %d (%s replay limit).]",
			formatReplayToolOutputSize(truncation.OutputBytes),
			truncation.TotalLines,
			formatReplayToolOutputSize(maxBytes),
		)
	}

	startLine := truncation.TotalLines - truncation.OutputLines + 1
	if startLine < 1 {
		startLine = 1
	}
	return truncation.Content + fmt.Sprintf(
		"\n[Historical tool output truncated for replay. Showing lines %d-%d of %d (%s replay limit).]",
		startLine,
		truncation.TotalLines,
		truncation.TotalLines,
		formatReplayToolOutputSize(maxBytes),
	)
}

func truncateReplayToolOutputTail(content string, maxLines int, maxBytes int) replayToolOutputTruncation {
	totalBytes := len([]byte(content))
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	if totalLines <= maxLines && totalBytes <= maxBytes {
		return replayToolOutputTruncation{
			Content:     content,
			Truncated:   false,
			TotalLines:  totalLines,
			TotalBytes:  totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
		}
	}

	outputLines := make([]string, 0, minReplayToolOutputInt(totalLines, maxLines))
	outputBytes := 0
	truncatedBy := "lines"
	lastLinePartial := false

	for i := len(lines) - 1; i >= 0 && len(outputLines) < maxLines; i-- {
		line := lines[i]
		lineBytes := len([]byte(line))
		if len(outputLines) > 0 {
			lineBytes++
		}
		if outputBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			if len(outputLines) == 0 {
				truncatedLine := truncateReplayToolOutputStringFromEnd(line, maxBytes)
				outputLines = append([]string{truncatedLine}, outputLines...)
				outputBytes = len([]byte(truncatedLine))
				lastLinePartial = true
			}
			break
		}

		outputLines = append([]string{line}, outputLines...)
		outputBytes += lineBytes
	}

	if len(outputLines) >= maxLines && outputBytes <= maxBytes {
		truncatedBy = "lines"
	}

	outputContent := strings.Join(outputLines, "\n")
	return replayToolOutputTruncation{
		Content:         outputContent,
		Truncated:       true,
		TotalLines:      totalLines,
		TotalBytes:      totalBytes,
		OutputLines:     len(outputLines),
		OutputBytes:     len([]byte(outputContent)),
		TruncatedBy:     truncatedBy,
		LastLinePartial: lastLinePartial,
	}
}

func truncateReplayToolOutputStringFromEnd(value string, maxBytes int) string {
	buf := []byte(value)
	if len(buf) <= maxBytes {
		return value
	}

	start := len(buf) - maxBytes
	for start < len(buf) && (buf[start]&0xc0) == 0x80 {
		start++
	}
	return string(buf[start:])
}

func formatReplayToolOutputSize(sizeBytes int) string {
	if sizeBytes < 1024 {
		return fmt.Sprintf("%dB", sizeBytes)
	}
	if sizeBytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(sizeBytes)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(sizeBytes)/(1024*1024))
}

func minReplayToolOutputInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
