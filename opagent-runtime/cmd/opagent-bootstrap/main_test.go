package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStateDoesNotDowngradeNewerInstalledRuntime(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "run"), 0o755); err != nil {
		t.Fatalf("MkdirAll(run): %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "run", latestVersionFile), []byte("0.8.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(latest): %v", err)
	}
	for _, spec := range managedFiles() {
		target := filepath.Join(baseDir, spec.TargetRelativePath)
		if spec.Directory {
			if err := os.MkdirAll(filepath.Join(target, ".agent"), 0o755); err != nil {
				t.Fatalf("MkdirAll(%s): %v", target, err)
			}
			if err := os.WriteFile(filepath.Join(target, ".agent", "AGENT.md"), []byte("x"), 0o644); err != nil {
				t.Fatalf("WriteFile(%s): %v", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, []byte("x"), 0o755); err != nil {
			t.Fatalf("WriteFile(%s): %v", target, err)
		}
	}

	st := loadState(options{baseDir: baseDir, port: "46190", version: "0.8.0"})
	if st.LatestVersion != "0.8.1" {
		t.Fatalf("LatestVersion = %q, want installed newer version", st.LatestVersion)
	}
	if st.NeedsInstall {
		t.Fatal("NeedsInstall = true, want false for older bundled version")
	}
	if st.NeedsUpdate {
		t.Fatal("NeedsUpdate = true, want false for older bundled version")
	}
	if !st.NeedsStart {
		t.Fatal("NeedsStart = false, want true because health is down")
	}
}

func TestManagedFilesRequireCoreRuntimeAndPackageDirectories(t *testing.T) {
	exe := exeSuffix()
	files := managedFiles()
	if !hasManagedFile(files, filepath.Join("bin", "gbrain"+exe), filepath.Join("bin", "gbrain"+exe), true) {
		t.Fatalf("managedFiles() is missing packaged gbrain binary")
	}
	if !hasManagedDir(files, "agents", "agents") {
		t.Fatalf("managedFiles() is missing agents directory")
	}
	if !hasManagedDir(files, "tools", "tools") {
		t.Fatalf("managedFiles() is missing tools directory")
	}
	if !hasManagedDir(files, "skills", "skills") {
		t.Fatalf("managedFiles() is missing skills directory")
	}
}

func TestShouldUpdateRuntimeVersion(t *testing.T) {
	cases := []struct {
		current string
		target  string
		want    bool
	}{
		{"", "0.8.0", true},
		{"0.8.0", "0.8.1", true},
		{"0.8.1", "0.8.0", false},
		{"0.8.0-beta.1", "0.8.0", true},
		{"0.8.0", "0.8.0-beta.1", false},
		{"custom-a", "custom-b", true},
	}
	for _, tc := range cases {
		if got := shouldUpdateRuntimeVersion(tc.current, tc.target); got != tc.want {
			t.Fatalf("shouldUpdateRuntimeVersion(%q, %q) = %v, want %v", tc.current, tc.target, got, tc.want)
		}
	}
}

func TestCopyDirCanSkipTarget(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("MkdirAll(src): %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "keep"), []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile(src keep): %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "skip"), []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile(src skip): %v", err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("MkdirAll(dst): %v", err)
	}
	skipPath := filepath.Join(dst, "skip")
	if err := os.WriteFile(skipPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(dst skip): %v", err)
	}

	if err := copyDir(src, dst, func(target string) bool {
		return samePath(target, skipPath)
	}); err != nil {
		t.Fatalf("copyDir(): %v", err)
	}

	if got := readTestFile(t, filepath.Join(dst, "keep")); got != "new" {
		t.Fatalf("copied keep = %q, want new", got)
	}
	if got := readTestFile(t, skipPath); got != "old" {
		t.Fatalf("skipped file = %q, want old", got)
	}
}

func hasManagedFile(files []managedFile, bundlePath string, targetPath string, executable bool) bool {
	for _, file := range files {
		if !file.Directory && file.BundleRelativePath == bundlePath && file.TargetRelativePath == targetPath && file.Executable == executable {
			return true
		}
	}
	return false
}

func hasManagedDir(files []managedFile, bundlePath string, targetPath string) bool {
	for _, file := range files {
		if file.Directory && file.BundleRelativePath == bundlePath && file.TargetRelativePath == targetPath {
			return true
		}
	}
	return false
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(data)
}
