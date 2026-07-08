//go:build !windows

package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendUniquePathValueAppendsWithoutOverridingExistingEntries(t *testing.T) {
	current := strings.Join([]string{"/usr/bin", "/opt/homebrew/bin"}, string(os.PathListSeparator))
	got := appendUniquePathValue(current, "/Users/me/.openbrain/bin", samePathEntry)
	want := current + string(os.PathListSeparator) + "/Users/me/.openbrain/bin"
	if got != want {
		t.Fatalf("appendUniquePathValue() = %q, want %q", got, want)
	}
	if gotAgain := appendUniquePathValue(got, "/Users/me/.openbrain/bin", samePathEntry); gotAgain != got {
		t.Fatalf("appendUniquePathValue duplicate = %q, want %q", gotAgain, got)
	}
}

func TestEnsureUnixUserPathContainsWritesManagedAppendBlock(t *testing.T) {
	if pathEntriesCaseInsensitive() {
		t.Skip("unix profile test")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("# user config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.zshrc): %v", err)
	}
	binDir := filepath.Join(home, ".openbrain", "bin")
	if err := ensureUnixUserPathContains(home, "/bin/zsh", binDir); err != nil {
		t.Fatalf("ensureUnixUserPathContains(): %v", err)
	}
	for _, name := range []string{".profile", ".zprofile", ".zshrc"} {
		data, err := os.ReadFile(filepath.Join(home, name))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		text := string(data)
		if !strings.Contains(text, openBrainPathStartMarker) || !strings.Contains(text, `export PATH="$PATH:$HOME/.openbrain/bin"`) {
			t.Fatalf("%s missing managed append block:\n%s", name, text)
		}
	}
	if err := ensureUnixUserPathContains(home, "/bin/zsh", binDir); err != nil {
		t.Fatalf("second ensureUnixUserPathContains(): %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".profile"))
	if err != nil {
		t.Fatalf("ReadFile(.profile): %v", err)
	}
	if count := strings.Count(string(data), openBrainPathStartMarker); count != 1 {
		t.Fatalf("managed block count = %d, want 1", count)
	}
}

func TestProjectSystemToolBinsCopiesSystemToolBinFiles(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	baseDir := filepath.Join(homeDir, ".openbrain")
	toolDir := filepath.Join(baseDir, "tools", "rg-search")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(toolDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "TOOL.md"), []byte("---\nname: rg-search\ntags: builtin,system\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(TOOL.md): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(toolDir, "bin"), 0o755); err != nil {
		t.Fatalf("MkdirAll(bin): %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "bin", "rg"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(rg): %v", err)
	}
	if err := ProjectSystemToolBins(baseDir); err != nil {
		t.Fatalf("ProjectSystemToolBins(): %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "bin", "rg")); err != nil {
		t.Fatalf("projected rg missing: %v", err)
	}
	profile, err := os.ReadFile(filepath.Join(homeDir, ".profile"))
	if err != nil {
		t.Fatalf("ReadFile(.profile): %v", err)
	}
	if got := string(profile); !strings.Contains(got, "$HOME/.openbrain/bin") {
		t.Fatalf(".profile missing OpenBrain PATH block:\n%s", got)
	}
}

func TestProjectSystemToolBinsIgnoresMissingBinDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	baseDir := filepath.Join(homeDir, ".openbrain")
	toolDir := filepath.Join(baseDir, "tools", "metadata-only")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(toolDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "TOOL.md"), []byte("---\nname: metadata-only\ntags: system\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(TOOL.md): %v", err)
	}
	if err := ProjectSystemToolBins(baseDir); err != nil {
		t.Fatalf("ProjectSystemToolBins(): %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "bin")); !os.IsNotExist(err) {
		t.Fatalf("bin dir exists or stat failed: %v", err)
	}
}
