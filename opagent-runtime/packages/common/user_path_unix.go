//go:build !windows

package common

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	openBrainPathStartMarker = "# >>> OpenBrain managed PATH >>>"
	openBrainPathEndMarker   = "# <<< OpenBrain managed PATH <<<"
)

func EnsureUserPathContains(dir string) error {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return err
	}
	return ensureUnixUserPathContains(home, os.Getenv("SHELL"), dir)
}

func ensureUnixUserPathContains(home string, shellPath string, dir string) error {
	home = filepath.Clean(strings.TrimSpace(home))
	entry := userPathEntryForDir(dir)
	if home == "" || strings.TrimSpace(entry) == "" {
		return nil
	}
	profiles := unixProfilePaths(home, shellPath)
	block := unixPathBlock(entry)
	for _, profile := range profiles {
		if err := upsertManagedBlock(profile, block); err != nil {
			return err
		}
	}
	return nil
}

func unixProfilePaths(home string, shellPath string) []string {
	names := []string{".profile"}
	shellName := filepath.Base(strings.TrimSpace(shellPath))
	switch shellName {
	case "zsh":
		names = append(names, ".zprofile", ".zshrc")
	case "bash":
		names = append(names, ".bash_profile", ".bashrc")
	}
	for _, name := range []string{".zprofile", ".zshrc", ".bash_profile", ".bashrc"} {
		if _, err := os.Stat(filepath.Join(home, name)); err == nil {
			names = append(names, name)
		}
	}
	seen := make(map[string]struct{}, len(names))
	paths := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		paths = append(paths, filepath.Join(home, name))
	}
	return paths
}

func unixPathBlock(entry string) string {
	return openBrainPathStartMarker + "\n" +
		`case ":$PATH:" in` + "\n" +
		`  *":` + entry + `:"*) ;;` + "\n" +
		`  *) export PATH="$PATH:` + entry + `" ;;` + "\n" +
		`esac` + "\n" +
		openBrainPathEndMarker + "\n"
}

func upsertManagedBlock(path string, block string) error {
	currentBytes, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	current := string(currentBytes)
	start := strings.Index(current, openBrainPathStartMarker)
	end := strings.Index(current, openBrainPathEndMarker)
	if start >= 0 && end >= start {
		end += len(openBrainPathEndMarker)
		next := current[:start] + block + current[end:]
		return writeProfile(path, next)
	}
	separator := ""
	if strings.TrimSpace(current) != "" && !strings.HasSuffix(current, "\n") {
		separator = "\n"
	}
	return writeProfile(path, current+separator+block)
}

func writeProfile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func pathEntriesCaseInsensitive() bool {
	return false
}

func homeVariable() string {
	return "$HOME"
}
