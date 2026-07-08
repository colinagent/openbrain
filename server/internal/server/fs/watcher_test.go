package fs

import (
	"path/filepath"
	"testing"
)

func TestIsExcludedWatchPathMatchesDirectoryNames(t *testing.T) {
	root := filepath.Join("tmp", "workspace")
	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join(root, ".git", "index"), true},
		{filepath.Join(root, "node_modules", "pkg", "index.js"), true},
		{filepath.Join(root, "notes", "a.md"), false},
	}
	for _, tc := range cases {
		if got := isExcludedWatchPath(tc.path, root, []string{".git", "node_modules"}); got != tc.want {
			t.Fatalf("isExcludedWatchPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
