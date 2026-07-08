package md

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	defaultThreadTitle    = "Untitled Chat"
	maxThreadTitleLength  = 60
	maxThreadSlugLength   = 64
	minBreakRatioForTitle = 0.6
)

var (
	commandPrefixRE       = regexp.MustCompile(`^/[a-z0-9_-]+\s*`)
	markdownNoiseRE       = regexp.MustCompile("^[#>*`\\-\\[\\]\\(\\)\\s]+")
	whitespaceRE          = regexp.MustCompile(`\s+`)
	unsafeFilenameCharRE  = regexp.MustCompile("[<>:\"/\\\\|?*\\x00-\\x1F\\x7F]")
	windowsReservedNameRE = regexp.MustCompile(`^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$`)
)

func DeriveChatTitle(userText string) string {
	firstLine := extractFirstUserLine(userText)
	if firstLine == "" {
		return defaultThreadTitle
	}
	return smartTruncate(firstLine, maxThreadTitleLength)
}

func SlugifyThreadTitle(title string) string {
	base := strings.ToLower(strings.TrimSpace(title))
	base = strings.Join(strings.Fields(base), "-")
	base = unsafeFilenameCharRE.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-.")
	base = collapseDashes(base)
	for strings.Contains(base, "..") {
		base = strings.ReplaceAll(base, "..", ".")
	}
	base = strings.Trim(base, "-.")
	base = truncateRunes(base, maxThreadSlugLength)
	base = strings.Trim(base, "-.")
	base = protectWindowsReservedName(base)
	if base == "" || base == "." || base == ".." {
		return "untitled-chat"
	}
	return base
}

func collapseDashes(input string) string {
	if input == "" {
		return ""
	}
	return strings.Join(strings.FieldsFunc(input, func(r rune) bool { return r == '-' }), "-")
}

func truncateRunes(input string, maxRunes int) string {
	if maxRunes <= 0 || input == "" {
		return ""
	}
	if utf8.RuneCountInString(input) <= maxRunes {
		return input
	}
	var b strings.Builder
	count := 0
	for _, r := range input {
		if count >= maxRunes {
			break
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}

func protectWindowsReservedName(input string) string {
	normalized := strings.TrimRight(strings.TrimSpace(input), ". ")
	normalized = strings.ToUpper(normalized)
	if !windowsReservedNameRE.MatchString(normalized) {
		return input
	}
	// Avoid forbidden Windows basenames like CON/PRN/AUX/NUL/COM1/LPT1.
	if maxThreadSlugLength <= 1 {
		return ""
	}
	safe := truncateRunes(input, maxThreadSlugLength-1)
	safe = strings.Trim(safe, "-.")
	if safe == "" {
		return ""
	}
	return safe + "-"
}

func extractFirstUserLine(input string) string {
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		if normalized := normalizeCandidate(line); normalized != "" {
			return normalized
		}
	}
	return ""
}

func normalizeCandidate(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	trimmed = commandPrefixRE.ReplaceAllString(trimmed, "")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" || shouldSkipTitleCandidateLine(trimmed) {
		return ""
	}
	trimmed = NormalizeInlineMarkdownLinks(trimmed)
	trimmed = markdownNoiseRE.ReplaceAllString(trimmed, "")
	trimmed = whitespaceRE.ReplaceAllString(trimmed, " ")
	return strings.TrimSpace(trimmed)
}

func shouldSkipTitleCandidateLine(line string) bool {
	return isStandaloneMarkdownImageLine(line) || IsStandaloneMarkdownLinkLine(line)
}

func isStandaloneMarkdownImageLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	match := markdownImageRE.FindStringIndex(trimmed)
	return match != nil && match[0] == 0 && match[1] == len(trimmed)
}

func smartTruncate(input string, maxLen int) string {
	runes := []rune(strings.TrimSpace(input))
	if len(runes) == 0 {
		return ""
	}
	if len(runes) <= maxLen {
		return string(runes)
	}
	cut := string(runes[:maxLen])
	breakpoints := []string{" ", ",", ".", ";", ":", "!", "?", "-", "，", "。", "；", "：", "！", "？"}
	lastBreak := -1
	for _, token := range breakpoints {
		if idx := strings.LastIndex(cut, token); idx > lastBreak {
			lastBreak = idx
		}
	}
	head := strings.TrimSpace(cut)
	if lastBreak >= int(float64(maxLen)*minBreakRatioForTitle) {
		head = strings.TrimSpace(cut[:lastBreak])
	}
	if head == "" {
		head = strings.TrimSpace(cut)
	}
	return head + "..."
}
