package md

import "strings"

const (
	opaqueBodyEncoding = "opaque-fenced"
	minFenceWidth      = 3
)

type fencedBlockState struct {
	char  byte
	width int
}

// balanceFencedCodeBlocks closes the last open fenced code block inside a
// single message body so broken model output cannot leak into later messages.
func balanceFencedCodeBlocks(body string) string {
	open := findLastOpenFence(body)
	if open.width == 0 {
		return body
	}
	var b strings.Builder
	b.Grow(len(body) + open.width + 1)
	b.WriteString(body)
	if body != "" && !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat(string(open.char), open.width))
	return b.String()
}

// formatOpaqueBlockBody isolates untrusted body text inside a fenced code block
// so markdown syntax inside tool/result/reasoning payloads never pollutes later blocks.
func formatOpaqueBlockBody(body, infoString string) string {
	width := maxInt(minFenceWidth, maxBacktickRun(body)+1)
	fence := strings.Repeat("`", width)
	open := fence + strings.TrimSpace(infoString)

	var b strings.Builder
	b.Grow(len(body) + len(open) + len(fence) + 2)
	b.WriteString(open)
	b.WriteByte('\n')
	b.WriteString(body)
	if body != "" && !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(fence)
	return b.String()
}

func maybeUnwrapOpaqueBlockBody(content string, attrs map[string]string) string {
	if attrs["encoding"] != opaqueBodyEncoding {
		return content
	}
	return unwrapOpaqueBlockBody(content)
}

func unwrapOpaqueBlockBody(body string) string {
	firstNL := strings.IndexByte(body, '\n')
	if firstNL == -1 {
		return body
	}
	openChar, openWidth, _, ok := parseFenceLine(body[:firstNL])
	if !ok || openChar != '`' {
		return body
	}
	closeStart := strings.LastIndex(body, "\n")
	if closeStart <= firstNL {
		return body
	}
	closeChar, closeWidth, closeRest, ok := parseFenceLine(body[closeStart+1:])
	if !ok || closeChar != openChar || closeWidth < openWidth || strings.TrimSpace(closeRest) != "" {
		return body
	}
	return body[firstNL+1 : closeStart]
}

func findLastOpenFence(body string) fencedBlockState {
	var open fencedBlockState
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		char, width, rest, ok := parseFenceLine(line)
		if !ok {
			continue
		}
		if open.width == 0 {
			open = fencedBlockState{char: char, width: width}
			_ = rest
			continue
		}
		if char == open.char && width >= open.width && strings.TrimSpace(rest) == "" {
			open = fencedBlockState{}
		}
	}
	return open
}

func parseFenceLine(line string) (char byte, width int, rest string, ok bool) {
	trimmed := trimFenceIndent(line)
	if len(trimmed) < minFenceWidth {
		return 0, 0, "", false
	}
	char = trimmed[0]
	if char != '`' && char != '~' {
		return 0, 0, "", false
	}
	for width < len(trimmed) && trimmed[width] == char {
		width++
	}
	if width < minFenceWidth {
		return 0, 0, "", false
	}
	return char, width, trimmed[width:], true
}

func trimFenceIndent(line string) string {
	idx := 0
	for idx < len(line) && idx < 3 && line[idx] == ' ' {
		idx++
	}
	return line[idx:]
}

func maxBacktickRun(text string) int {
	maxRun := 0
	current := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '`' {
			current++
			if current > maxRun {
				maxRun = current
			}
			continue
		}
		current = 0
	}
	return maxRun
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
