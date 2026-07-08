package run

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
)

func TestShouldUpdateRuntimeVersion(t *testing.T) {
	tests := []struct {
		name    string
		current string
		target  string
		want    bool
	}{
		{
			name:    "newer patch updates",
			current: "0.6.3",
			target:  "0.6.4",
			want:    true,
		},
		{
			name:    "older manifest does not downgrade",
			current: "0.6.4",
			target:  "0.6.3",
			want:    false,
		},
		{
			name:    "same version is idle",
			current: "0.6.4",
			target:  "0.6.4",
			want:    false,
		},
		{
			name:    "numeric comparison handles two digit patch",
			current: "0.6.9",
			target:  "0.6.10",
			want:    true,
		},
		{
			name:    "v-prefixed semver is supported",
			current: "v0.6.3",
			target:  "v0.6.4",
			want:    true,
		},
		{
			name:    "release is newer than prerelease",
			current: "0.6.4-beta.1",
			target:  "0.6.4",
			want:    true,
		},
		{
			name:    "prerelease does not replace release",
			current: "0.6.4",
			target:  "0.6.4-beta.1",
			want:    false,
		},
		{
			name:    "build metadata alone does not update",
			current: "0.6.4+build.1",
			target:  "0.6.4+build.2",
			want:    false,
		},
		{
			name:    "custom versions preserve legacy update behavior",
			current: "local-dev",
			target:  "0.6.4",
			want:    true,
		},
		{
			name:    "empty target is ignored",
			current: "0.6.4",
			target:  "",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUpdateRuntimeVersion(tt.current, tt.target)
			if got != tt.want {
				t.Fatalf("shouldUpdateRuntimeVersion(%q, %q) = %v, want %v", tt.current, tt.target, got, tt.want)
			}
		})
	}
}

func TestRuntimeManagedFilesRequireCoreRuntimeAndPackageDirectories(t *testing.T) {
	exe := runtimeExeSuffix()
	if exe == "" && goruntime.GOOS == "windows" {
		t.Fatal("runtimeExeSuffix() returned empty on windows")
	}
	gbrainName := "gbrain" + exe
	if !hasRuntimeManagedFile(filepath.Join("bin", gbrainName), filepath.Join("bin", gbrainName), true) {
		t.Fatalf("runtimeManagedFiles() is missing packaged gbrain binary")
	}
	if !hasRuntimeManagedDir("agents", "agents") {
		t.Fatalf("runtimeManagedFiles() is missing agents directory")
	}
	if !hasRuntimeManagedDir("tools", "tools") {
		t.Fatalf("runtimeManagedFiles() is missing tools directory")
	}
	if !hasRuntimeManagedDir("skills", "skills") {
		t.Fatalf("runtimeManagedFiles() is missing skills directory")
	}
}

func TestApplyRuntimeStageFilesMergesBuiltinPackageDirectoriesAndProjectsSystemToolBins(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("ProjectSystemToolBins writes the user PATH registry entry on Windows")
	}
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	baseDir := filepath.Join(homeDir, ".openbrain")
	extractDir := filepath.Join(t.TempDir(), "extract")
	exe := runtimeExeSuffix()

	writeRuntimeTestFile(t, filepath.Join(extractDir, "bin", "opagent-runtime"+exe), "runtime", 0o755)
	writeRuntimeTestFile(t, filepath.Join(extractDir, "bin", "opagent-bootstrap"+exe), "bootstrap", 0o755)
	writeRuntimeTestFile(t, filepath.Join(extractDir, "bin", "gbrain"+exe), "gbrain", 0o755)
	writeRuntimeTestFile(t, filepath.Join(extractDir, "configs", "config.json"), "{}", 0o644)
	writeRuntimeTestFile(t, filepath.Join(extractDir, "agents", "coder", ".agent", "AGENT.md"), "---\nname: coder\n---\n", 0o644)
	writeRuntimeTestFile(t, filepath.Join(extractDir, "tools", "rg-search", "TOOL.md"), "---\nname: rg-search\ntags: system\n---\n", 0o644)
	writeRuntimeTestFile(t, filepath.Join(extractDir, "tools", "rg-search", "bin", "rg"+exe), "#!/bin/sh\n", 0o755)
	writeRuntimeTestFile(t, filepath.Join(extractDir, "skills", "openbrain-cloud-sync", "SKILL.md"), "---\nname: sync\n---\n", 0o644)

	writeRuntimeTestFile(t, filepath.Join(baseDir, "agents", "custom", ".agent", "AGENT.md"), "custom", 0o644)
	writeRuntimeTestFile(t, filepath.Join(baseDir, "tools", "rg-search", "stale"), "old", 0o644)

	stage := &stagedRuntimeBundle{Version: "0.9.0", ExtractDir: extractDir}
	if err := applyRuntimeStageFiles(runtimeUpdateSettings{baseDir: baseDir}, stage); err != nil {
		t.Fatalf("applyRuntimeStageFiles(): %v", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "agents", "custom", ".agent", "AGENT.md")); err != nil {
		t.Fatalf("custom agent was not preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "agents", "coder", ".agent", "AGENT.md")); err != nil {
		t.Fatalf("builtin coder agent missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "tools", "rg-search", "stale")); !os.IsNotExist(err) {
		t.Fatalf("stale builtin tool file still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "bin", "rg"+exe)); err != nil {
		t.Fatalf("projected rg missing: %v", err)
	}
	if got := strings.TrimSpace(readRuntimeTestFile(t, filepath.Join(baseDir, "run", runtimeLatestVersionFile))); got != "0.9.0" {
		t.Fatalf("latest version = %q, want 0.9.0", got)
	}
}

func hasRuntimeManagedFile(bundlePath string, targetPath string, executable bool) bool {
	for _, file := range runtimeManagedFiles() {
		if !file.Directory && file.BundleRelativePath == bundlePath && file.TargetRelativePath == targetPath && file.Executable == executable {
			return true
		}
	}
	return false
}

func hasRuntimeManagedDir(bundlePath string, targetPath string) bool {
	for _, file := range runtimeManagedFiles() {
		if file.Directory && file.MergeDirectory && file.BundleRelativePath == bundlePath && file.TargetRelativePath == targetPath {
			return true
		}
	}
	return false
}

func writeRuntimeTestFile(t *testing.T, path string, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func readRuntimeTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(data)
}
