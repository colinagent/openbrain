package builtintools

import (
	"fmt"
	"strings"
)

type truncationResult struct {
	Content               string `json:"content"`
	Truncated             bool   `json:"truncated"`
	TruncatedBy           string `json:"truncatedBy,omitempty"`
	TotalLines            int    `json:"totalLines"`
	TotalBytes            int    `json:"totalBytes"`
	OutputLines           int    `json:"outputLines"`
	OutputBytes           int    `json:"outputBytes"`
	LastLinePartial       bool   `json:"lastLinePartial"`
	FirstLineExceedsLimit bool   `json:"firstLineExceedsLimit"`
	MaxLines              int    `json:"maxLines"`
	MaxBytes              int    `json:"maxBytes"`
}

func truncateHead(content string, maxLines int, maxBytes int) truncationResult {
	totalBytes := len([]byte(content))
	lines := stringsSplitLines(content)
	totalLines := len(lines)

	if totalLines <= maxLines && totalBytes <= maxBytes {
		return truncationResult{
			Content:               content,
			Truncated:             false,
			TotalLines:            totalLines,
			TotalBytes:            totalBytes,
			OutputLines:           totalLines,
			OutputBytes:           totalBytes,
			LastLinePartial:       false,
			FirstLineExceedsLimit: false,
			MaxLines:              maxLines,
			MaxBytes:              maxBytes,
		}
	}

	firstLineBytes := len([]byte(lines[0]))
	if firstLineBytes > maxBytes {
		return truncationResult{
			Content:               "",
			Truncated:             true,
			TruncatedBy:           "bytes",
			TotalLines:            totalLines,
			TotalBytes:            totalBytes,
			OutputLines:           0,
			OutputBytes:           0,
			LastLinePartial:       false,
			FirstLineExceedsLimit: true,
			MaxLines:              maxLines,
			MaxBytes:              maxBytes,
		}
	}

	outputLines := make([]string, 0, minInt(totalLines, maxLines))
	outputBytes := 0
	truncatedBy := "lines"

	for i := 0; i < len(lines) && i < maxLines; i++ {
		line := lines[i]
		lineBytes := len([]byte(line))
		if i > 0 {
			lineBytes++
		}
		if outputBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			break
		}
		outputLines = append(outputLines, line)
		outputBytes += lineBytes
	}

	if len(outputLines) >= maxLines && outputBytes <= maxBytes {
		truncatedBy = "lines"
	}

	outputContent := joinLines(outputLines)
	return truncationResult{
		Content:               outputContent,
		Truncated:             true,
		TruncatedBy:           truncatedBy,
		TotalLines:            totalLines,
		TotalBytes:            totalBytes,
		OutputLines:           len(outputLines),
		OutputBytes:           len([]byte(outputContent)),
		LastLinePartial:       false,
		FirstLineExceedsLimit: false,
		MaxLines:              maxLines,
		MaxBytes:              maxBytes,
	}
}

func truncateTail(content string, maxLines int, maxBytes int) truncationResult {
	totalBytes := len([]byte(content))
	lines := stringsSplitLines(content)
	totalLines := len(lines)

	if totalLines <= maxLines && totalBytes <= maxBytes {
		return truncationResult{
			Content:               content,
			Truncated:             false,
			TotalLines:            totalLines,
			TotalBytes:            totalBytes,
			OutputLines:           totalLines,
			OutputBytes:           totalBytes,
			LastLinePartial:       false,
			FirstLineExceedsLimit: false,
			MaxLines:              maxLines,
			MaxBytes:              maxBytes,
		}
	}

	outputLines := make([]string, 0, minInt(totalLines, maxLines))
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
				truncatedLine := truncateStringToBytesFromEnd(line, maxBytes)
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

	outputContent := joinLines(outputLines)
	return truncationResult{
		Content:               outputContent,
		Truncated:             true,
		TruncatedBy:           truncatedBy,
		TotalLines:            totalLines,
		TotalBytes:            totalBytes,
		OutputLines:           len(outputLines),
		OutputBytes:           len([]byte(outputContent)),
		LastLinePartial:       lastLinePartial,
		FirstLineExceedsLimit: false,
		MaxLines:              maxLines,
		MaxBytes:              maxBytes,
	}
}

func truncateStringToBytesFromEnd(value string, maxBytes int) string {
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

func formatSize(sizeBytes int) string {
	if sizeBytes < 1024 {
		return fmt.Sprintf("%dB", sizeBytes)
	}
	if sizeBytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(sizeBytes)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(sizeBytes)/(1024*1024))
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func stringsSplitLines(value string) []string {
	return strings.Split(value, "\n")
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
