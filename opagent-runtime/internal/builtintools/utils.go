package builtintools

import (
	"regexp"
	"strings"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// cleanANSI removes ANSI escape sequences and trims whitespace
func cleanANSI(input string) string {
	stripped := ansiRegex.ReplaceAllString(input, "")
	return strings.TrimSpace(stripped)
}
