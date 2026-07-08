package md

import (
	"strings"
	"unicode/utf8"
)

type parsedMarkdownLink struct {
	label  string
	target string
	length int
}

func IsStandaloneMarkdownLinkLine(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.HasPrefix(trimmed, "![") {
		return false
	}
	link, ok := parseMarkdownLinkPrefix(trimmed)
	return ok && link.length == len(trimmed)
}

func IsStandaloneMarkdownImageLine(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || !strings.HasPrefix(trimmed, "![") {
		return false
	}
	link, ok := parseMarkdownLinkPrefix(trimmed[1:])
	if !ok || link.length+1 > len(trimmed) {
		return false
	}
	rest := strings.TrimSpace(trimmed[1+link.length:])
	return rest == "" || (strings.HasPrefix(rest, "{") && strings.HasSuffix(rest, "}"))
}

func NormalizeInlineMarkdownLinks(text string) string {
	if !strings.Contains(text, "[") {
		return text
	}

	var b strings.Builder
	for index := 0; index < len(text); {
		if text[index] == '!' && index+1 < len(text) && text[index+1] == '[' {
			link, ok := parseMarkdownLinkPrefix(text[index+1:])
			if ok {
				b.WriteString(link.label)
				index += 1 + link.length
				continue
			}
		}
		if text[index] == '[' {
			link, ok := parseMarkdownLinkPrefix(text[index:])
			if ok {
				b.WriteString(link.label)
				index += link.length
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(text[index:])
		if size <= 0 {
			size = 1
		}
		b.WriteString(text[index : index+size])
		index += size
	}
	return b.String()
}

func parseMarkdownLinkPrefix(text string) (parsedMarkdownLink, bool) {
	if len(text) < 4 || text[0] != '[' {
		return parsedMarkdownLink{}, false
	}

	rawLabel, nextIndex, ok := parseMarkdownLinkLabel(text, 1)
	if !ok || nextIndex >= len(text) || text[nextIndex] != '(' {
		return parsedMarkdownLink{}, false
	}

	rawTarget, endIndex, ok := parseMarkdownLinkTarget(text, nextIndex)
	if !ok {
		return parsedMarkdownLink{}, false
	}

	return parsedMarkdownLink{
		label:  unescapeMarkdownLinkText(rawLabel),
		target: unescapeMarkdownLinkText(rawTarget),
		length: endIndex,
	}, true
}

func parseMarkdownLinkLabel(text string, start int) (string, int, bool) {
	for index := start; index < len(text); {
		switch text[index] {
		case '\\':
			_, size := utf8.DecodeRuneInString(text[index+1:])
			if index+1 >= len(text) || size <= 0 {
				return "", 0, false
			}
			index += 1 + size
		case ']':
			return text[start:index], index + 1, true
		case '\n', '\r':
			return "", 0, false
		default:
			_, size := utf8.DecodeRuneInString(text[index:])
			if size <= 0 {
				return "", 0, false
			}
			index += size
		}
	}
	return "", 0, false
}

func parseMarkdownLinkTarget(text string, openParenIndex int) (string, int, bool) {
	if openParenIndex >= len(text) || text[openParenIndex] != '(' {
		return "", 0, false
	}

	depth := 1
	start := openParenIndex + 1
	for index := start; index < len(text); {
		switch text[index] {
		case '\\':
			_, size := utf8.DecodeRuneInString(text[index+1:])
			if index+1 >= len(text) || size <= 0 {
				return "", 0, false
			}
			index += 1 + size
		case '(':
			depth += 1
			index += 1
		case ')':
			depth -= 1
			if depth == 0 {
				return text[start:index], index + 1, true
			}
			index += 1
		case '\n', '\r':
			return "", 0, false
		default:
			_, size := utf8.DecodeRuneInString(text[index:])
			if size <= 0 {
				return "", 0, false
			}
			index += size
		}
	}
	return "", 0, false
}

func unescapeMarkdownLinkText(text string) string {
	if !strings.Contains(text, "\\") {
		return text
	}

	var b strings.Builder
	for index := 0; index < len(text); {
		if text[index] == '\\' && index+1 < len(text) {
			_, size := utf8.DecodeRuneInString(text[index+1:])
			if size > 0 {
				b.WriteString(text[index+1 : index+1+size])
				index += 1 + size
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(text[index:])
		if size <= 0 {
			size = 1
		}
		b.WriteString(text[index : index+size])
		index += size
	}
	return b.String()
}
