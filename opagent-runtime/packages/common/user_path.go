package common

import (
	"os"
	"path/filepath"
	"strings"
)

func appendUniquePathValue(current string, entry string, equal func(string, string) bool) string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return current
	}
	parts := filepath.SplitList(current)
	for _, part := range parts {
		if equal(part, entry) {
			return current
		}
	}
	if strings.TrimSpace(current) == "" {
		return entry
	}
	return current + string(os.PathListSeparator) + entry
}

func samePathEntry(a string, b string) bool {
	a = strings.TrimSpace(expandPathEntry(a))
	b = strings.TrimSpace(expandPathEntry(b))
	if a == "" || b == "" {
		return a == b
	}
	cleanA, errA := filepath.Abs(a)
	cleanB, errB := filepath.Abs(b)
	if errA == nil {
		a = filepath.Clean(cleanA)
	}
	if errB == nil {
		b = filepath.Clean(cleanB)
	}
	if pathEntriesCaseInsensitive() {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func expandPathEntry(value string) string {
	value = os.ExpandEnv(strings.TrimSpace(value))
	if strings.Contains(value, "%USERPROFILE%") {
		value = strings.ReplaceAll(value, "%USERPROFILE%", os.Getenv("USERPROFILE"))
	}
	return value
}

func userPathEntryForDir(dir string) string {
	dir = filepath.Clean(strings.TrimSpace(dir))
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return dir
	}
	home = filepath.Clean(home)
	rel, err := filepath.Rel(home, dir)
	if err == nil && rel == filepath.Join(".openbrain", "bin") {
		return filepath.Join(homeVariable(), rel)
	}
	return dir
}
